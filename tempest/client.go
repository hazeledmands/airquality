package tempest

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"math"
	"net"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"sync"
	"time"
)

// DefaultBaseURL is the WeatherFlow REST API host.
const DefaultBaseURL = "https://swd.weatherflow.com"

// MaxWindow is the range each historical request asks for. The REST API
// returns one-minute rows only for short ranges (documented as five days or
// less; a six-day request returned 30-minute buckets), so history is fetched
// a day at a time.
const MaxWindow = 24 * time.Hour

// Client calls the WeatherFlow REST API with a personal access token.
type Client struct {
	BaseURL string
	// RetryBase is the first backoff after a 429 or 5xx; it doubles per retry.
	RetryBase time.Duration
	Retries   int
	// Now bounds accepted observation timestamps (default time.Now).
	Now func() time.Time
	// Warn reports values dropped as implausible (default log.Printf).
	Warn func(format string, args ...any)

	token string
	http  *http.Client

	warnMu    sync.Mutex
	warnState map[string]*warnState
}

// warnWindow is how often each kind of data warning may be logged.
const warnWindow = time.Hour

type warnState struct {
	last       time.Time
	suppressed int
}

// warnLimited logs at most once per key per warnWindow, reporting how many
// were suppressed in between, so one bad field can't flood the log.
func (c *Client) warnLimited(key, format string, args ...any) {
	c.warnMu.Lock()
	defer c.warnMu.Unlock()
	if c.warnState == nil {
		c.warnState = make(map[string]*warnState)
	}
	now := c.Now()
	st, ok := c.warnState[key]
	if ok && now.Sub(st.last) < warnWindow {
		st.suppressed++
		return
	}
	if ok && st.suppressed > 0 {
		format += fmt.Sprintf(" (%d similar suppressed in the last %s)", st.suppressed, warnWindow)
	}
	c.warnState[key] = &warnState{last: now}
	c.Warn(format, args...)
}

// NewClient returns a client for the given personal access token.
func NewClient(token string) *Client {
	return &Client{
		BaseURL:   DefaultBaseURL,
		RetryBase: 2 * time.Second,
		Retries:   3,
		Now:       time.Now,
		Warn:      log.Printf,
		token:     token,
		http:      &http.Client{Timeout: 30 * time.Second},
	}
}

// StatusError is a non-200 HTTP response.
type StatusError struct {
	Code int
	Body string
	// RetryAfter is the server's Retry-After on a 429, if any.
	RetryAfter time.Duration
}

// transportError is a failed request with the token scrubbed from its text.
// permanent marks failures retrying cannot fix (unknown host, bad
// certificate).
type transportError struct {
	msg       string
	permanent bool
}

func (e *transportError) Error() string { return e.msg }

func permanentTransport(err error) bool {
	var dnsErr *net.DNSError
	if errors.As(err, &dnsErr) && dnsErr.IsNotFound {
		return true
	}
	var unknownCA x509.UnknownAuthorityError
	var hostErr x509.HostnameError
	var invalid x509.CertificateInvalidError
	var verify *tls.CertificateVerificationError
	return errors.As(err, &unknownCA) || errors.As(err, &hostErr) || errors.As(err, &invalid) || errors.As(err, &verify)
}

func (e *StatusError) Error() string {
	if e.Body == "" {
		return fmt.Sprintf("tempest API HTTP %d", e.Code)
	}
	return fmt.Sprintf("tempest API HTTP %d: %s", e.Code, e.Body)
}

func retryable(err error) bool {
	var se *StatusError
	if errors.As(err, &se) {
		return se.Code == http.StatusTooManyRequests || se.Code >= 500
	}
	var te *transportError
	if errors.As(err, &te) && te.permanent {
		return false
	}
	// Other transport errors (timeouts, resets) are worth another try.
	return !errors.Is(err, context.Canceled) && !errors.Is(err, context.DeadlineExceeded)
}

// get fetches path with params and the token, retrying 429/5xx/transport
// failures. Errors never contain the token: net/http puts the full request
// URL in transport errors, so those are unwrapped and scrubbed.
func (c *Client) get(ctx context.Context, path string, params url.Values) ([]byte, error) {
	var err error
	backoff := c.RetryBase
	for attempt := 0; ; attempt++ {
		var body []byte
		body, err = c.getOnce(ctx, path, params)
		if err == nil {
			return body, nil
		}
		if attempt >= c.Retries || !retryable(err) {
			return nil, err
		}
		wait := backoff
		var se *StatusError
		if errors.As(err, &se) && se.RetryAfter > wait {
			wait = min(se.RetryAfter, time.Minute)
		}
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case <-time.After(wait):
		}
		backoff *= 2
	}
}

func (c *Client) getOnce(ctx context.Context, path string, params url.Values) ([]byte, error) {
	q := url.Values{}
	for k, v := range params {
		q[k] = v
	}
	q.Set("token", c.token)
	u := strings.TrimRight(c.BaseURL, "/") + path + "?" + q.Encode()

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u, nil)
	if err != nil {
		return nil, fmt.Errorf("GET %s: %s", path, redact(err, c.token))
	}
	resp, err := c.http.Do(req)
	if err != nil {
		inner := err
		var ue *url.Error
		if errors.As(err, &ue) {
			inner = ue.Err
		}
		return nil, &transportError{msg: fmt.Sprintf("GET %s: %s", path, redact(inner, c.token)), permanent: permanentTransport(err)}
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(io.LimitReader(resp.Body, 32<<20))
	if err != nil {
		return nil, fmt.Errorf("GET %s: read body: %s", path, redact(err, c.token))
	}
	if resp.StatusCode != http.StatusOK {
		// Redact the whole body first: truncating first could cut a token
		// in half and leave the first half unredacted.
		snippet := strings.TrimSpace(redactString(string(body), c.token))
		if len(snippet) > 200 {
			snippet = snippet[:200]
		}
		se := &StatusError{Code: resp.StatusCode, Body: snippet}
		if secs, err := strconv.Atoi(resp.Header.Get("Retry-After")); err == nil && secs > 0 {
			se.RetryAfter = time.Duration(secs) * time.Second
		}
		return nil, se
	}
	return body, nil
}

// redact renders err with every occurrence of token replaced.
func redact(err error, token string) string {
	return redactString(err.Error(), token)
}

func redactString(s, token string) string {
	if token != "" {
		s = strings.ReplaceAll(s, token, "REDACTED")
		s = strings.ReplaceAll(s, url.QueryEscape(token), "REDACTED")
	}
	return s
}

type apiStatus struct {
	StatusCode    int    `json:"status_code"`
	StatusMessage string `json:"status_message"`
}

func (s apiStatus) err() error {
	if s.StatusCode != 0 {
		return fmt.Errorf("tempest API status %d: %s", s.StatusCode, s.StatusMessage)
	}
	return nil
}

type stationJSON struct {
	StationID    int    `json:"station_id"`
	Name         string `json:"name"`
	Timezone     string `json:"timezone"`
	CreatedEpoch int64  `json:"created_epoch"`
	StationMeta  struct {
		Elevation float64 `json:"elevation"`
	} `json:"station_meta"`
	Devices []struct {
		DeviceID         int    `json:"device_id"`
		DeviceType       string `json:"device_type"`
		SerialNumber     string `json:"serial_number"`
		FirmwareRevision string `json:"firmware_revision"`
	} `json:"devices"`
}

// Stations lists the stations the token can see, with their devices.
func (c *Client) Stations(ctx context.Context) ([]Station, error) {
	body, err := c.get(ctx, "/swd/rest/stations", nil)
	if err != nil {
		return nil, fmt.Errorf("tempest stations: %w", err)
	}
	var data struct {
		Status    apiStatus     `json:"status"`
		Stations  []stationJSON `json:"stations"`
		Locations []stationJSON `json:"locations"`
	}
	if err := json.Unmarshal(body, &data); err != nil {
		return nil, fmt.Errorf("tempest stations: parse: %w", err)
	}
	if err := data.Status.err(); err != nil {
		return nil, err
	}
	raw := data.Stations
	if len(raw) == 0 {
		raw = data.Locations
	}
	out := make([]Station, 0, len(raw))
	for _, s := range raw {
		// Clamped to the range of inhabited elevations: a wild value would
		// make sea-level pressure non-finite.
		elev := s.StationMeta.Elevation
		if math.IsNaN(elev) {
			elev = 0
		}
		elev = min(max(elev, -500), 9000)
		st := Station{ID: s.StationID, Name: s.Name, Timezone: s.Timezone, ElevationM: elev}
		if s.CreatedEpoch > 0 {
			st.Created = time.Unix(s.CreatedEpoch, 0).UTC()
		}
		for _, d := range s.Devices {
			st.Devices = append(st.Devices, Device{ID: d.DeviceID, Type: d.DeviceType, Serial: d.SerialNumber, Firmware: d.FirmwareRevision})
		}
		out = append(out, st)
	}
	return out, nil
}

// SelectDevices picks the station to record and its Tempest ("ST") devices.
// stationID 0 picks the first station that has a Tempest.
func SelectDevices(stations []Station, stationID int) (Station, []Device, error) {
	if len(stations) == 0 {
		return Station{}, nil, errors.New("tempest: token sees no stations")
	}
	for _, s := range stations {
		if stationID != 0 && s.ID != stationID {
			continue
		}
		var devs []Device
		for _, d := range s.Devices {
			if d.Type == "ST" {
				devs = append(devs, d)
			}
		}
		if len(devs) > 0 {
			return s, devs, nil
		}
		if stationID != 0 {
			return Station{}, nil, fmt.Errorf("tempest: station %d has no Tempest (ST) device", stationID)
		}
	}
	if stationID != 0 {
		return Station{}, nil, fmt.Errorf("tempest: token cannot see station %d", stationID)
	}
	return Station{}, nil, errors.New("tempest: no station has a Tempest (ST) device")
}

// DeviceObservations returns the device's observations with timestamps in
// [start, end], oldest first. It refuses responses aggregated into buckets
// longer than a minute; keep end-start within MaxWindow.
func (c *Client) DeviceObservations(ctx context.Context, deviceID int, start, end time.Time) ([]Observation, error) {
	params := url.Values{
		"time_start": {strconv.FormatInt(start.Unix(), 10)},
		"time_end":   {strconv.FormatInt(end.Unix(), 10)},
	}
	path := "/swd/rest/observations/device/" + strconv.Itoa(deviceID)
	body, err := c.get(ctx, path, params)
	if err != nil {
		return nil, fmt.Errorf("tempest device %d observations: %w", deviceID, err)
	}
	var data struct {
		Status       apiStatus    `json:"status"`
		DeviceID     int          `json:"device_id"`
		BucketMinute int          `json:"bucket_step_minutes"`
		Obs          [][]*float64 `json:"obs"`
	}
	if err := json.Unmarshal(body, &data); err != nil {
		return nil, fmt.Errorf("tempest device %d observations: parse: %w", deviceID, err)
	}
	if err := data.Status.err(); err != nil {
		return nil, err
	}
	if data.BucketMinute > 1 {
		return nil, fmt.Errorf("tempest device %d observations: API returned %d-minute buckets for %s..%s; request a shorter range",
			deviceID, data.BucketMinute, start.UTC().Format(time.RFC3339), end.UTC().Format(time.RFC3339))
	}
	// Rows outside the requested window or in the future are dropped: a
	// future timestamp would otherwise become the poller's resume point and
	// stop live polling.
	lo := start.Add(-time.Minute)
	hi := end
	if now := c.Now(); now.Before(hi) {
		hi = now
	}
	hi = hi.Add(5 * time.Minute)
	out := make([]Observation, 0, len(data.Obs))
	for _, row := range data.Obs {
		o, rejected, ok := ParseObsST(deviceID, row)
		if !ok || o.Time.Before(lo) || o.Time.After(hi) {
			if ok {
				c.warnLimited(fmt.Sprintf("window/%d", deviceID), "tempest device %d: dropped observation at %s, outside %s..%s",
					deviceID, o.Time.Format(time.RFC3339), lo.UTC().Format(time.RFC3339), hi.UTC().Format(time.RFC3339))
			}
			continue
		}
		for _, field := range rejected {
			c.warnLimited(fmt.Sprintf("field/%d/%s", deviceID, field), "tempest device %d at %s: implausible %s stored as NULL",
				deviceID, o.Time.Format(time.RFC3339), field)
		}
		out = append(out, o)
	}
	return out, nil
}

// fieldRange is the plausible range of an obs_st field; values outside it
// (or non-finite) are stored as NULL. Integer fields must fit their column.
type fieldRange struct {
	name     string
	min, max float64
}

var obsRanges = [22]fieldRange{
	1: {"wind_lull_ms", 0, 100}, 2: {"wind_avg_ms", 0, 100}, 3: {"wind_gust_ms", 0, 150},
	4: {"wind_direction_deg", 0, 360}, 5: {"wind_sample_interval_s", 0, 3600},
	6: {"station_pressure_mb", 300, 1200}, 7: {"air_temp_c", -90, 70}, 8: {"relative_humidity", 0, 100},
	9: {"illuminance_lux", 0, 250000}, 10: {"uv_index", 0, 25}, 11: {"solar_radiation_wm2", 0, 2000},
	12: {"rain_mm", 0, 500}, 13: {"precip_type", 0, 3}, 14: {"lightning_avg_distance_km", 0, 100},
	15: {"lightning_strike_count", 0, 100000}, 16: {"battery_v", 0, 10}, 17: {"report_interval_min", 0, 1440},
	18: {"local_day_rain_mm", 0, 5000}, 19: {"rain_final_mm", 0, 500}, 20: {"local_day_rain_final_mm", 0, 5000},
	21: {"precip_analysis_type", 0, 3},
}

// ParseObsST converts one obs_st array to an Observation. It reports false
// for rows without a usable timestamp. Missing trailing fields stay nil, and
// implausible values become nil and are named in rejected.
func ParseObsST(deviceID int, row []*float64) (o Observation, rejected []string, ok bool) {
	if len(row) == 0 || row[0] == nil {
		return Observation{}, nil, false
	}
	if ts := *row[0]; math.IsNaN(ts) || ts < 0 || ts > 1e11 {
		return Observation{}, nil, false
	}
	at := func(i int) *float64 {
		if i >= len(row) || i >= len(obsRanges) || row[i] == nil {
			return nil
		}
		v := *row[i]
		r := obsRanges[i]
		if math.IsNaN(v) || math.IsInf(v, 0) || v < r.min || v > r.max {
			rejected = append(rejected, r.name)
			return nil
		}
		return &v
	}
	atInt := func(i int) *int {
		v := at(i)
		if v == nil {
			return nil
		}
		n := int(*v)
		return &n
	}
	o = Observation{
		DeviceID:               deviceID,
		Time:                   time.Unix(int64(*row[0]), 0).UTC(),
		WindLullMS:             at(1),
		WindAvgMS:              at(2),
		WindGustMS:             at(3),
		WindDirectionDeg:       at(4),
		WindSampleIntervalS:    at(5),
		StationPressureMB:      at(6),
		AirTempC:               at(7),
		RelativeHumidity:       at(8),
		IlluminanceLux:         at(9),
		UVIndex:                at(10),
		SolarRadiationWM2:      at(11),
		RainMM:                 at(12),
		PrecipType:             atInt(13),
		LightningAvgDistanceKM: at(14),
		LightningStrikeCount:   atInt(15),
		BatteryV:               at(16),
		ReportIntervalMin:      atInt(17),
		LocalDayRainMM:         at(18),
		RainFinalMM:            at(19),
		LocalDayRainFinalMM:    at(20),
		PrecipAnalysisType:     atInt(21),
	}
	return o, rejected, true
}

// Window is a half-open slice of a longer time range.
type Window struct{ Start, End time.Time }

// Windows splits [from, to) into consecutive windows no longer than max.
func Windows(from, to time.Time, max time.Duration) []Window {
	var out []Window
	for s := from; s.Before(to); s = s.Add(max) {
		e := s.Add(max)
		if e.After(to) {
			e = to
		}
		out = append(out, Window{Start: s, End: e})
	}
	return out
}
