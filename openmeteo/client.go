// Package openmeteo fetches weather, air quality and forecasts from
// Open-Meteo's free, keyless APIs (https://open-meteo.com): current
// conditions, the Air Quality API (CAMS), hourly forecasts, and the
// historical-forecast API for backfilling.
//
// Free use is limited to 600 calls a minute, 5,000 an hour and 10,000 a day
// per IP; requests for many variables or long ranges count as several calls.
// Callers poll at the data's own cadence (15 min for current weather, hourly
// for air quality and forecasts) and backfill in two-week chunks.
package openmeteo

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"math"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"sync"
	"time"
)

const (
	defaultBaseURL     = "https://api.open-meteo.com"
	defaultAQBaseURL   = "https://air-quality-api.open-meteo.com"
	defaultHistBaseURL = "https://historical-forecast-api.open-meteo.com"
)

// Reading holds a single weather observation.
type Reading struct {
	Time            time.Time
	Latitude        float64
	Longitude       float64
	TempC           float64
	Humidity        float64
	DewPointC       float64
	PrecipitationMM float64
	WeatherCode     int
	PressureHPa     float64
	WindSpeedKMH    float64
	UVIndex         float64
	CloudCover      *float64 // %; nil when not reported
	Sunrise         time.Time
	Sunset          time.Time
}

// AirQuality is one Air Quality API sample (CAMS model, hourly). Pollen is
// modelled only for Europe and is nil elsewhere. Units: µg/m³ except the
// AQIs (index) and pollen (grains/m³).
type AirQuality struct {
	Time          time.Time
	Latitude      float64
	Longitude     float64
	PM25          *float64
	PM10          *float64
	Ozone         *float64
	NO2           *float64
	SO2           *float64
	CO            *float64
	USAQI         *float64
	EuropeanAQI   *float64
	Dust          *float64
	AlderPollen   *float64
	BirchPollen   *float64
	GrassPollen   *float64
	MugwortPollen *float64
	OlivePollen   *float64
	RagweedPollen *float64
}

// ForecastHour is one hour of the forecast.
type ForecastHour struct {
	Time              time.Time
	TempC             *float64
	PrecipProbability *float64 // %
	PrecipMM          *float64
	CloudCover        *float64 // %
	UVIndex           *float64
	WindSpeedKMH      *float64
	WeatherCode       *int
}

// Client fetches from Open-Meteo for a fixed location.
type Client struct {
	Latitude  float64
	Longitude float64
	// RetryBase is the first backoff after a 429 or 5xx; it doubles.
	RetryBase time.Duration
	Retries   int
	// Warn reports implausible values (default log.Printf), at most once per
	// variable per hour.
	Warn func(format string, args ...any)

	baseURL     string
	aqBaseURL   string
	histBaseURL string
	http        *http.Client

	warnMu   sync.Mutex
	lastWarn map[string]time.Time
}

// NewClient creates a Client for the given coordinates.
func NewClient(lat, lon float64) *Client {
	return &Client{
		Latitude:    lat,
		Longitude:   lon,
		RetryBase:   5 * time.Second,
		Retries:     2,
		Warn:        log.Printf,
		baseURL:     defaultBaseURL,
		aqBaseURL:   defaultAQBaseURL,
		histBaseURL: defaultHistBaseURL,
		http:        &http.Client{Timeout: 30 * time.Second},
	}
}

// statusError is a non-200 response.
type statusError struct {
	code       int
	body       string
	retryAfter time.Duration
}

func (e *statusError) Error() string {
	return fmt.Sprintf("Open-Meteo HTTP %d: %s", e.code, e.body)
}

// get fetches base+path with the location and params, retrying 429/5xx and
// transport errors. Errors never include the URL: it carries the location.
func (c *Client) get(ctx context.Context, base, path string, params url.Values) ([]byte, error) {
	q := url.Values{}
	for k, v := range params {
		q[k] = v
	}
	q.Set("latitude", strconv.FormatFloat(c.Latitude, 'f', -1, 64))
	q.Set("longitude", strconv.FormatFloat(c.Longitude, 'f', -1, 64))
	q.Set("timezone", "GMT")
	u := strings.TrimRight(base, "/") + path + "?" + q.Encode()

	backoff := c.RetryBase
	for attempt := 0; ; attempt++ {
		body, err := c.getOnce(ctx, path, u)
		if err == nil {
			return body, nil
		}
		var se *statusError
		isStatus := errors.As(err, &se)
		retryable := !isStatus || se.code == http.StatusTooManyRequests || se.code >= 500
		if attempt >= c.Retries || !retryable || ctx.Err() != nil {
			return nil, err
		}
		wait := backoff
		if isStatus && se.retryAfter > wait {
			wait = min(se.retryAfter, time.Minute)
		}
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case <-time.After(wait):
		}
		backoff *= 2
	}
}

func (c *Client) getOnce(ctx context.Context, path, u string) ([]byte, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u, nil)
	if err != nil {
		return nil, fmt.Errorf("Open-Meteo %s: bad request", path)
	}
	resp, err := c.http.Do(req)
	if err != nil {
		var ue *url.Error
		if errors.As(err, &ue) {
			err = ue.Err
		}
		return nil, fmt.Errorf("Open-Meteo %s: %w", path, err)
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(io.LimitReader(resp.Body, 16<<20))
	if err != nil {
		return nil, fmt.Errorf("Open-Meteo %s: read body: %w", path, err)
	}
	if resp.StatusCode != http.StatusOK {
		var e struct {
			Reason string `json:"reason"`
		}
		msg := strings.TrimSpace(string(body))
		if json.Unmarshal(body, &e) == nil && e.Reason != "" {
			msg = e.Reason
		}
		if len(msg) > 200 {
			msg = msg[:200]
		}
		se := &statusError{code: resp.StatusCode, body: msg}
		if secs, err := strconv.Atoi(resp.Header.Get("Retry-After")); err == nil && secs > 0 {
			se.retryAfter = time.Duration(secs) * time.Second
		}
		return nil, fmt.Errorf("%s: %w", path, se)
	}
	return body, nil
}

// --- validation ---

type valueRange struct{ min, max float64 }

var ranges = map[string]valueRange{
	"temperature_2m": {-90, 70}, "relative_humidity_2m": {0, 100}, "dew_point_2m": {-90, 50},
	"precipitation": {0, 500}, "pressure_msl": {800, 1100}, "wind_speed_10m": {0, 500},
	"uv_index": {0, 25}, "cloud_cover": {0, 100}, "precipitation_probability": {0, 100},
	"weather_code": {0, 99},
	"pm2_5":        {0, 2000}, "pm10": {0, 5000}, "ozone": {0, 1500}, "nitrogen_dioxide": {0, 2000},
	"sulphur_dioxide": {0, 5000}, "carbon_monoxide": {0, 100000}, "us_aqi": {0, 500},
	"european_aqi": {0, 1000}, "dust": {0, 20000},
	"alder_pollen": {0, 1e6}, "birch_pollen": {0, 1e6}, "grass_pollen": {0, 1e6},
	"mugwort_pollen": {0, 1e6}, "olive_pollen": {0, 1e6}, "ragweed_pollen": {0, 1e6},
}

// check returns v if it is finite and plausible for the variable, else nil
// (reported at most once per variable per hour).
func (c *Client) check(name string, v *float64) *float64 {
	if v == nil {
		return nil
	}
	r, ok := ranges[name]
	if !ok || (!math.IsNaN(*v) && !math.IsInf(*v, 0) && *v >= r.min && *v <= r.max) {
		return v
	}
	c.warnMu.Lock()
	defer c.warnMu.Unlock()
	if c.lastWarn == nil {
		c.lastWarn = map[string]time.Time{}
	}
	if time.Since(c.lastWarn[name]) >= time.Hour {
		c.lastWarn[name] = time.Now()
		c.Warn("Open-Meteo: implausible %s %v stored as NULL", name, *v)
	}
	return nil
}

func (c *Client) checkInt(name string, v *float64) *int {
	p := c.check(name, v)
	if p == nil {
		return nil
	}
	n := int(math.Round(*p))
	return &n
}

func parseTime(s string) (time.Time, bool) {
	t, err := time.Parse("2006-01-02T15:04", s)
	return t.UTC(), err == nil
}

// --- current weather ---

// currentVars is what Fetch asks for.
const currentVars = "temperature_2m,relative_humidity_2m,dew_point_2m,precipitation,weather_code,pressure_msl,wind_speed_10m,uv_index,cloud_cover"

type apiResponse struct {
	Current struct {
		Time            string   `json:"time"`
		TempC           float64  `json:"temperature_2m"`
		Humidity        float64  `json:"relative_humidity_2m"`
		DewPointC       float64  `json:"dew_point_2m"`
		PrecipitationMM float64  `json:"precipitation"`
		WeatherCode     int      `json:"weather_code"`
		PressureHPa     float64  `json:"pressure_msl"`
		WindSpeedKMH    float64  `json:"wind_speed_10m"`
		UVIndex         float64  `json:"uv_index"`
		CloudCover      *float64 `json:"cloud_cover"`
	} `json:"current"`
	Daily struct {
		Sunrise []string `json:"sunrise"`
		Sunset  []string `json:"sunset"`
	} `json:"daily"`
}

// Fetch retrieves current conditions and today's sunrise/sunset.
func (c *Client) Fetch(ctx context.Context) (Reading, error) {
	body, err := c.get(ctx, c.baseURL, "/v1/forecast", url.Values{
		"current":       {currentVars},
		"daily":         {"sunrise,sunset"},
		"forecast_days": {"1"},
	})
	if err != nil {
		return Reading{}, fmt.Errorf("fetch weather: %w", err)
	}
	var data apiResponse
	if err := json.Unmarshal(body, &data); err != nil {
		return Reading{}, fmt.Errorf("parse weather: %w", err)
	}
	r := Reading{
		Latitude:        c.Latitude,
		Longitude:       c.Longitude,
		TempC:           data.Current.TempC,
		Humidity:        data.Current.Humidity,
		DewPointC:       data.Current.DewPointC,
		PrecipitationMM: data.Current.PrecipitationMM,
		WeatherCode:     data.Current.WeatherCode,
		PressureHPa:     data.Current.PressureHPa,
		WindSpeedKMH:    data.Current.WindSpeedKMH,
		UVIndex:         data.Current.UVIndex,
		CloudCover:      c.check("cloud_cover", data.Current.CloudCover),
	}
	if t, ok := parseTime(data.Current.Time); ok {
		r.Time = t
	} else {
		r.Time = time.Now().UTC()
	}
	if len(data.Daily.Sunrise) > 0 {
		if t, ok := parseTime(data.Daily.Sunrise[0]); ok {
			r.Sunrise = t
		}
	}
	if len(data.Daily.Sunset) > 0 {
		if t, ok := parseTime(data.Daily.Sunset[0]); ok {
			r.Sunset = t
		}
	}
	return r, nil
}

// --- air quality ---

const aqVars = "pm2_5,pm10,ozone,nitrogen_dioxide,sulphur_dioxide,carbon_monoxide,us_aqi,european_aqi,dust,alder_pollen,birch_pollen,grass_pollen,mugwort_pollen,olive_pollen,ragweed_pollen"

func (c *Client) airQualityFrom(get func(string) *float64, t time.Time) AirQuality {
	v := func(name string) *float64 { return c.check(name, get(name)) }
	return AirQuality{
		Time: t, Latitude: c.Latitude, Longitude: c.Longitude,
		PM25: v("pm2_5"), PM10: v("pm10"), Ozone: v("ozone"), NO2: v("nitrogen_dioxide"),
		SO2: v("sulphur_dioxide"), CO: v("carbon_monoxide"), USAQI: v("us_aqi"),
		EuropeanAQI: v("european_aqi"), Dust: v("dust"),
		AlderPollen: v("alder_pollen"), BirchPollen: v("birch_pollen"), GrassPollen: v("grass_pollen"),
		MugwortPollen: v("mugwort_pollen"), OlivePollen: v("olive_pollen"), RagweedPollen: v("ragweed_pollen"),
	}
}

// AirQuality returns the current (hourly) air quality.
func (c *Client) AirQuality(ctx context.Context) (AirQuality, error) {
	body, err := c.get(ctx, c.aqBaseURL, "/v1/air-quality", url.Values{"current": {aqVars}})
	if err != nil {
		return AirQuality{}, fmt.Errorf("fetch air quality: %w", err)
	}
	var data struct {
		Current map[string]json.RawMessage `json:"current"`
	}
	if err := json.Unmarshal(body, &data); err != nil {
		return AirQuality{}, fmt.Errorf("parse air quality: %w", err)
	}
	var ts string
	json.Unmarshal(data.Current["time"], &ts)
	t, ok := parseTime(ts)
	if !ok {
		return AirQuality{}, errors.New("parse air quality: no time")
	}
	get := func(name string) *float64 {
		var v *float64
		if raw, ok := data.Current[name]; ok {
			json.Unmarshal(raw, &v)
		}
		return v
	}
	return c.airQualityFrom(get, t), nil
}

// hourly is an Open-Meteo "hourly" block: a time array and one array per variable.
type hourly map[string]json.RawMessage

func (h hourly) times() ([]time.Time, error) {
	var raw []string
	if err := json.Unmarshal(h["time"], &raw); err != nil {
		return nil, fmt.Errorf("hourly time: %w", err)
	}
	out := make([]time.Time, len(raw))
	for i, s := range raw {
		t, ok := parseTime(s)
		if !ok {
			return nil, fmt.Errorf("hourly time %q", s)
		}
		out[i] = t
	}
	return out, nil
}

func (h hourly) column(name string) []*float64 {
	var col []*float64
	if raw, ok := h[name]; ok {
		json.Unmarshal(raw, &col)
	}
	return col
}

func at(col []*float64, i int) *float64 {
	if i < len(col) {
		return col[i]
	}
	return nil
}

func dateParams(start, end time.Time) url.Values {
	return url.Values{
		"start_date": {start.UTC().Format("2006-01-02")},
		"end_date":   {end.UTC().Format("2006-01-02")},
	}
}

// AirQualityHistory returns hourly air quality for the UTC days from start to end.
func (c *Client) AirQualityHistory(ctx context.Context, start, end time.Time) ([]AirQuality, error) {
	params := dateParams(start, end)
	params.Set("hourly", aqVars)
	body, err := c.get(ctx, c.aqBaseURL, "/v1/air-quality", params)
	if err != nil {
		return nil, fmt.Errorf("fetch air quality history: %w", err)
	}
	var data struct {
		Hourly hourly `json:"hourly"`
	}
	if err := json.Unmarshal(body, &data); err != nil {
		return nil, fmt.Errorf("parse air quality history: %w", err)
	}
	times, err := data.Hourly.times()
	if err != nil {
		return nil, err
	}
	cols := map[string][]*float64{}
	for _, name := range strings.Split(aqVars, ",") {
		cols[name] = data.Hourly.column(name)
	}
	out := make([]AirQuality, 0, len(times))
	for i, t := range times {
		out = append(out, c.airQualityFrom(func(name string) *float64 { return at(cols[name], i) }, t))
	}
	return out, nil
}

// --- forecast ---

const forecastVars = "temperature_2m,precipitation_probability,precipitation,cloud_cover,uv_index,wind_speed_10m,weather_code"

// Forecast returns the hourly forecast for today and the next two days (UTC).
func (c *Client) Forecast(ctx context.Context) ([]ForecastHour, error) {
	body, err := c.get(ctx, c.baseURL, "/v1/forecast", url.Values{
		"hourly":        {forecastVars},
		"forecast_days": {"3"},
	})
	if err != nil {
		return nil, fmt.Errorf("fetch forecast: %w", err)
	}
	var data struct {
		Hourly hourly `json:"hourly"`
	}
	if err := json.Unmarshal(body, &data); err != nil {
		return nil, fmt.Errorf("parse forecast: %w", err)
	}
	times, err := data.Hourly.times()
	if err != nil {
		return nil, err
	}
	col := data.Hourly.column
	temp, prob, precip := col("temperature_2m"), col("precipitation_probability"), col("precipitation")
	cloud, uv, wind, code := col("cloud_cover"), col("uv_index"), col("wind_speed_10m"), col("weather_code")
	out := make([]ForecastHour, 0, len(times))
	for i, t := range times {
		out = append(out, ForecastHour{
			Time:              t,
			TempC:             c.check("temperature_2m", at(temp, i)),
			PrecipProbability: c.check("precipitation_probability", at(prob, i)),
			PrecipMM:          c.check("precipitation", at(precip, i)),
			CloudCover:        c.check("cloud_cover", at(cloud, i)),
			UVIndex:           c.check("uv_index", at(uv, i)),
			WindSpeedKMH:      c.check("wind_speed_10m", at(wind, i)),
			WeatherCode:       c.checkInt("weather_code", at(code, i)),
		})
	}
	return out, nil
}

// --- history ---

const historyVars = "temperature_2m,relative_humidity_2m,dew_point_2m,precipitation,weather_code,pressure_msl,wind_speed_10m,uv_index,cloud_cover"

// History returns hourly weather for the UTC days from start to end from
// the historical-forecast API (the model's best estimate, available up to
// today, unlike the ERA5 archive which lags about five days). Each reading
// carries its day's sunrise and sunset. Hours with no temperature are
// skipped, since the weather table needs one.
func (c *Client) History(ctx context.Context, start, end time.Time) ([]Reading, error) {
	params := dateParams(start, end)
	params.Set("hourly", historyVars)
	params.Set("daily", "sunrise,sunset")
	body, err := c.get(ctx, c.histBaseURL, "/v1/forecast", params)
	if err != nil {
		return nil, fmt.Errorf("fetch weather history: %w", err)
	}
	var data struct {
		Hourly hourly `json:"hourly"`
		Daily  struct {
			Time    []string `json:"time"`
			Sunrise []string `json:"sunrise"`
			Sunset  []string `json:"sunset"`
		} `json:"daily"`
	}
	if err := json.Unmarshal(body, &data); err != nil {
		return nil, fmt.Errorf("parse weather history: %w", err)
	}
	times, err := data.Hourly.times()
	if err != nil {
		return nil, err
	}
	type sun struct{ rise, set time.Time }
	days := map[string]sun{}
	for i, d := range data.Daily.Time {
		var s sun
		if i < len(data.Daily.Sunrise) {
			s.rise, _ = parseTime(data.Daily.Sunrise[i])
		}
		if i < len(data.Daily.Sunset) {
			s.set, _ = parseTime(data.Daily.Sunset[i])
		}
		days[d] = s
	}
	col := data.Hourly.column
	temp, rh, dew, precip := col("temperature_2m"), col("relative_humidity_2m"), col("dew_point_2m"), col("precipitation")
	code, pres, wind, uv, cloud := col("weather_code"), col("pressure_msl"), col("wind_speed_10m"), col("uv_index"), col("cloud_cover")
	val := func(name string, p *float64) float64 {
		if v := c.check(name, p); v != nil {
			return *v
		}
		return 0
	}
	out := make([]Reading, 0, len(times))
	for i, t := range times {
		if c.check("temperature_2m", at(temp, i)) == nil {
			continue
		}
		s := days[t.Format("2006-01-02")]
		out = append(out, Reading{
			Time: t, Latitude: c.Latitude, Longitude: c.Longitude,
			TempC:           val("temperature_2m", at(temp, i)),
			Humidity:        val("relative_humidity_2m", at(rh, i)),
			DewPointC:       val("dew_point_2m", at(dew, i)),
			PrecipitationMM: val("precipitation", at(precip, i)),
			WeatherCode:     int(val("weather_code", at(code, i))),
			PressureHPa:     val("pressure_msl", at(pres, i)),
			WindSpeedKMH:    val("wind_speed_10m", at(wind, i)),
			UVIndex:         val("uv_index", at(uv, i)),
			CloudCover:      c.check("cloud_cover", at(cloud, i)),
			Sunrise:         s.rise,
			Sunset:          s.set,
		})
	}
	return out, nil
}
