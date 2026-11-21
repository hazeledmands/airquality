package main

import (
	"context"
	"encoding/json"
	"errors"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/hazel/airquality/config"
	"github.com/hazel/airquality/openmeteo"
)

type fakeWeatherAPI struct {
	weather, aq, forecast int
	failAQ                bool
	dayLong               bool // answer whole UTC days, like the real APIs
	histCalls, aqHist     [][2]time.Time
}

func (f *fakeWeatherAPI) Fetch(context.Context) (openmeteo.Reading, error) {
	f.weather++
	return openmeteo.Reading{Time: time.Now().UTC().Truncate(15 * time.Minute), Latitude: 1, Longitude: 2, TempC: 10, CloudCover: fp(40)}, nil
}

func (f *fakeWeatherAPI) AirQuality(context.Context) (openmeteo.AirQuality, error) {
	f.aq++
	if f.failAQ {
		return openmeteo.AirQuality{}, errors.New("boom")
	}
	return openmeteo.AirQuality{Time: time.Now().UTC().Truncate(time.Hour), Latitude: 1, Longitude: 2, PM25: fp(12)}, nil
}

func (f *fakeWeatherAPI) Forecast(context.Context) ([]openmeteo.ForecastHour, error) {
	f.forecast++
	return []openmeteo.ForecastHour{{Time: time.Now().UTC().Truncate(time.Hour).Add(time.Hour), TempC: fp(11)}}, nil
}

func (f *fakeWeatherAPI) History(_ context.Context, start, end time.Time) ([]openmeteo.Reading, error) {
	f.histCalls = append(f.histCalls, [2]time.Time{start, end})
	if f.dayLong {
		start = start.Truncate(24 * time.Hour)
		end = end.Truncate(24 * time.Hour).Add(23 * time.Hour)
	}
	var out []openmeteo.Reading
	for t := start; !t.After(end); t = t.Add(time.Hour) {
		out = append(out, openmeteo.Reading{Time: t, Latitude: 1, Longitude: 2, TempC: 5})
	}
	return out, nil
}

func (f *fakeWeatherAPI) AirQualityHistory(_ context.Context, start, end time.Time) ([]openmeteo.AirQuality, error) {
	f.aqHist = append(f.aqHist, [2]time.Time{start, end})
	if f.dayLong {
		var out []openmeteo.AirQuality
		for t := start.Truncate(24 * time.Hour); t.Before(start.Truncate(24 * time.Hour).Add(24 * time.Hour)); t = t.Add(time.Hour) {
			out = append(out, openmeteo.AirQuality{Time: t, Latitude: 1, Longitude: 2, PM25: fp(3)})
		}
		return out, nil
	}
	return []openmeteo.AirQuality{{Time: start, Latitude: 1, Longitude: 2, PM25: fp(3)}}, nil
}

type fakeWeatherStore struct {
	extras        bool
	weather       map[time.Time]bool
	aq            map[time.Time]bool
	forecastCalls int
}

func newFakeWeatherStore(extras bool) *fakeWeatherStore {
	return &fakeWeatherStore{extras: extras, weather: map[time.Time]bool{}, aq: map[time.Time]bool{}}
}

func (s *fakeWeatherStore) OpenMeteoExtras() bool { return s.extras }
func (s *fakeWeatherStore) InsertWeather(_ context.Context, r openmeteo.Reading) error {
	s.weather[r.Time] = true
	return nil
}
func (s *fakeWeatherStore) BatchInsertWeather(_ context.Context, rows []openmeteo.Reading) (int64, error) {
	var n int64
	for _, r := range rows {
		if !s.weather[r.Time] {
			s.weather[r.Time] = true
			n++
		}
	}
	return n, nil
}
func (s *fakeWeatherStore) UpsertAirQuality(_ context.Context, rows []openmeteo.AirQuality) (int64, error) {
	var n int64
	for _, r := range rows {
		if !s.aq[r.Time] {
			s.aq[r.Time] = true
			n++
		}
	}
	return n, nil
}
func (s *fakeWeatherStore) UpsertForecast(context.Context, float64, float64, time.Time, []openmeteo.ForecastHour) (int64, error) {
	s.forecastCalls++
	return 1, nil
}

func newTestWeatherRecorder(api *fakeWeatherAPI, db *fakeWeatherStore, now *time.Time) *weatherRecorder {
	return &weatherRecorder{api: api, db: db, now: func() time.Time { return *now }, cache: newTestCache(), lat: 1, lon: 2}
}

// Polite cadence: current weather every 15 min, air quality and forecast
// hourly, whatever the main --interval.
func TestWeatherRecorder_Cadence(t *testing.T) {
	api := &fakeWeatherAPI{}
	now := time.Date(2026, 11, 21, 12, 0, 0, 0, time.UTC)
	r := newTestWeatherRecorder(api, newFakeWeatherStore(true), &now)
	for i := 0; i < 60; i++ { // one poll a minute for an hour
		r.Poll(t.Context())
		now = now.Add(time.Minute)
	}
	if api.weather != 4 || api.aq != 1 || api.forecast != 1 {
		t.Errorf("calls in an hour: weather %d, air quality %d, forecast %d; want 4, 1, 1", api.weather, api.aq, api.forecast)
	}
}

func TestWeatherRecorder_NoExtrasWithoutMigration(t *testing.T) {
	api := &fakeWeatherAPI{}
	now := time.Date(2026, 11, 21, 12, 0, 0, 0, time.UTC)
	r := newTestWeatherRecorder(api, newFakeWeatherStore(false), &now)
	r.Poll(t.Context())
	if api.weather != 1 || api.aq != 0 || api.forecast != 0 {
		t.Errorf("weather %d aq %d forecast %d", api.weather, api.aq, api.forecast)
	}
}

func TestWeatherRecorder_FailureRetriesAfterFiveMinutes(t *testing.T) {
	api := &fakeWeatherAPI{failAQ: true}
	now := time.Date(2026, 11, 21, 12, 0, 0, 0, time.UTC)
	r := newTestWeatherRecorder(api, newFakeWeatherStore(true), &now)
	for i := 0; i < 10; i++ {
		r.Poll(t.Context())
		now = now.Add(time.Minute)
	}
	if api.aq != 2 { // at 0 and 5 minutes
		t.Errorf("air quality calls in 10 min of failures = %d, want 2", api.aq)
	}
}

func TestWeatherRecorder_FeedsCache(t *testing.T) {
	api := &fakeWeatherAPI{}
	now := time.Now().UTC()
	r := newTestWeatherRecorder(api, newFakeWeatherStore(true), &now)
	r.Poll(t.Context())
	_, _, w, _ := r.cache.snapshotRange(now.Add(-time.Hour), now.Add(time.Hour))
	if len(w) != 1 || w[0].CloudCover == nil {
		t.Errorf("weather in cache = %+v", w)
	}
	if aq := r.cache.snapshotAirQuality(now.Add(-2*time.Hour), now.Add(time.Hour)); len(aq) != 1 {
		t.Errorf("air quality in cache = %d", len(aq))
	}
	if fc := r.cache.forecastFrom(now.Add(-time.Hour)); len(fc) != 1 {
		t.Errorf("forecast in cache = %d", len(fc))
	}
}

func TestWeatherBackfill_ChunkedAndIdempotent(t *testing.T) {
	api := &fakeWeatherAPI{}
	db := newFakeWeatherStore(true)
	from := time.Date(2026, 4, 17, 0, 0, 0, 0, time.UTC)
	to := from.Add(40 * 24 * time.Hour)
	res, err := runWeatherBackfill(t.Context(), api, db, weatherBackfillOptions{from: from, to: to}, func(string, ...any) {})
	if err != nil {
		t.Fatal(err)
	}
	if len(api.histCalls) != 3 || len(api.aqHist) != 3 {
		t.Errorf("requests: weather %d, air quality %d; want 3 two-week chunks each", len(api.histCalls), len(api.aqHist))
	}
	for _, c := range api.histCalls {
		if c[1].Sub(c[0]) > 14*24*time.Hour {
			t.Errorf("chunk %v too long", c)
		}
	}
	first := res.weather
	res, _ = runWeatherBackfill(t.Context(), api, db, weatherBackfillOptions{from: from, to: to}, func(string, ...any) {})
	if first == 0 || res.weather != 0 || res.airQuality != 0 {
		t.Errorf("re-run stored %d weather, %d air quality rows; want 0 (first run %d)", res.weather, res.airQuality, first)
	}
}

func TestWeatherBackfill_WithoutExtrasSkipsAirQuality(t *testing.T) {
	api := &fakeWeatherAPI{}
	from := time.Date(2026, 4, 17, 0, 0, 0, 0, time.UTC)
	runWeatherBackfill(t.Context(), api, newFakeWeatherStore(false), weatherBackfillOptions{from: from, to: from.Add(24 * time.Hour)}, func(string, ...any) {})
	if len(api.aqHist) != 0 || len(api.histCalls) != 1 {
		t.Errorf("weather %d aq %d", len(api.histCalls), len(api.aqHist))
	}
}

func TestParseWeatherBackfillArgs(t *testing.T) {
	now := time.Date(2026, 11, 21, 12, 0, 0, 0, time.UTC)
	o, err := parseWeatherBackfillArgs([]string{"--from", "2026-04-17"}, now)
	if err != nil || !o.from.Equal(time.Date(2026, 4, 17, 0, 0, 0, 0, time.UTC)) || !o.to.Equal(now) {
		t.Errorf("opts = %+v err = %v", o, err)
	}
	if _, err := parseWeatherBackfillArgs(nil, now); err == nil {
		t.Error("missing --from: want error")
	}
}

func TestDownsampleWeather_KeepsCloudCover(t *testing.T) {
	from := time.Date(2026, 11, 21, 0, 0, 0, 0, time.UTC)
	rows := []openmeteo.Reading{
		{Time: from, TempC: 1, CloudCover: fp(20)},
		{Time: from.Add(15 * time.Minute), TempC: 2, CloudCover: fp(60)},
		{Time: from.Add(30 * time.Minute), TempC: 3, CloudCover: fp(80)},
	}
	got := downsampleWeather(rows, from, 3600_000)
	if len(got) != 1 || got[0].CloudCover == nil || *got[0].CloudCover != 60 {
		t.Errorf("got %+v", got)
	}
}

func TestDataHandler_OutdoorAirAndForecast(t *testing.T) {
	c := newTestCache()
	now := time.Now().UTC().Truncate(time.Hour)
	c.addAirQuality([]openmeteo.AirQuality{{Time: now.Add(-time.Hour), Latitude: 12.34, Longitude: -56.78, PM25: fp(12), USAQI: fp(50), GrassPollen: nil}})
	c.setForecast([]openmeteo.ForecastHour{
		{Time: now.Add(-2 * time.Hour), TempC: fp(1)}, // past: not served
		{Time: now.Add(time.Hour), TempC: fp(11), PrecipProbability: fp(30)},
	})
	c.addWeather(openmeteo.Reading{Time: now.Add(-time.Hour), Latitude: 12.34, Longitude: -56.78, TempC: 10, CloudCover: fp(70)})
	from := now.Add(-3 * time.Hour).UnixMilli()
	w := httptest.NewRecorder()
	dataHandler(c, config.Config{}).ServeHTTP(w, httptest.NewRequest("GET", "/api/data?from="+strconv.FormatInt(from, 10), nil))
	body := w.Body.String()
	if strings.Contains(body, "12.34") || strings.Contains(body, "56.78") {
		t.Error("response carries the location")
	}
	var data struct {
		Weather    []map[string]any `json:"weather"`
		OutdoorAir []map[string]any `json:"outdoor_air"`
		Forecast   []map[string]any `json:"forecast"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &data); err != nil {
		t.Fatal(err)
	}
	if len(data.OutdoorAir) != 1 || data.OutdoorAir[0]["pm2_5"] != 12.0 || data.OutdoorAir[0]["us_aqi"] != 50.0 {
		t.Errorf("outdoor_air = %+v", data.OutdoorAir)
	}
	if len(data.Forecast) != 1 || data.Forecast[0]["precip_probability"] != 30.0 {
		t.Errorf("forecast = %+v", data.Forecast)
	}
	if len(data.Weather) != 1 || data.Weather[0]["cloud_cover"] != 70.0 {
		t.Errorf("weather = %+v", data.Weather)
	}
}

// The APIs answer whole days, so the current day comes back with hours that
// haven't happened yet; those are forecasts and must not be stored as
// observations.
func TestWeatherBackfill_DropsHoursAfterTo(t *testing.T) {
	api := &fakeWeatherAPI{}
	db := newFakeWeatherStore(true)
	from := time.Date(2026, 11, 21, 0, 0, 0, 0, time.UTC)
	to := from.Add(20*time.Hour + 30*time.Minute)
	api.dayLong = true
	if _, err := runWeatherBackfill(t.Context(), api, db, weatherBackfillOptions{from: from, to: to}, func(string, ...any) {}); err != nil {
		t.Fatal(err)
	}
	for ts := range db.weather {
		if ts.After(to) {
			t.Errorf("stored weather at %v, after --to %v", ts, to)
		}
	}
	for ts := range db.aq {
		if ts.After(to) {
			t.Errorf("stored air quality at %v, after --to %v", ts, to)
		}
	}
	if len(db.weather) != 21 {
		t.Errorf("weather rows = %d, want 21 (00:00..20:00)", len(db.weather))
	}
}
