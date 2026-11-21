package openmeteo

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

// testdata/*.json are real Open-Meteo responses for a public city location,
// with the coordinates replaced.
func fixture(t *testing.T, name string) []byte {
	t.Helper()
	b, err := os.ReadFile("testdata/" + name)
	if err != nil {
		t.Fatal(err)
	}
	return b
}

// testClient points every API at one test server.
func testClient(srv *httptest.Server) *Client {
	c := NewClient(12.345, -65.432)
	c.baseURL, c.aqBaseURL, c.histBaseURL = srv.URL, srv.URL, srv.URL
	c.RetryBase = time.Millisecond
	c.Warn = func(string, ...any) {}
	return c
}

func serve(t *testing.T, path string, body []byte, check func(*http.Request)) *httptest.Server {
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != path {
			t.Errorf("path = %s, want %s", r.URL.Path, path)
		}
		if check != nil {
			check(r)
		}
		w.Write(body)
	}))
}

func TestFetch_CloudCover(t *testing.T) {
	srv := serve(t, "/v1/forecast", fixture(t, "forecast.json"), func(r *http.Request) {
		if !strings.Contains(r.URL.Query().Get("current"), "cloud_cover") {
			t.Errorf("current = %q, want cloud_cover", r.URL.Query().Get("current"))
		}
	})
	defer srv.Close()
	r, err := testClient(srv).Fetch(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	if r.CloudCover == nil || *r.CloudCover != 81 {
		t.Errorf("CloudCover = %v, want 81", r.CloudCover)
	}
}

func TestAirQuality(t *testing.T) {
	srv := serve(t, "/v1/air-quality", fixture(t, "air_quality_current.json"), func(r *http.Request) {
		cur := r.URL.Query().Get("current")
		for _, v := range []string{"pm2_5", "pm10", "ozone", "nitrogen_dioxide", "us_aqi", "grass_pollen"} {
			if !strings.Contains(cur, v) {
				t.Errorf("current lacks %s", v)
			}
		}
	})
	defer srv.Close()
	aq, err := testClient(srv).AirQuality(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	if !aq.Time.Equal(time.Date(2026, 11, 21, 20, 0, 0, 0, time.UTC)) {
		t.Errorf("Time = %v", aq.Time)
	}
	if *aq.PM25 != 24 || *aq.PM10 != 26.7 || *aq.Ozone != 13 || *aq.NO2 != 67.1 || *aq.USAQI != 86 || *aq.EuropeanAQI != 64 {
		t.Errorf("aq = %+v", aq)
	}
	// Pollen is only modelled for Europe; elsewhere the API sends null.
	if aq.GrassPollen != nil || aq.RagweedPollen != nil {
		t.Errorf("pollen should be nil here: %+v", aq)
	}
	if aq.Latitude != 12.345 || aq.Longitude != -65.432 {
		t.Errorf("location = %v,%v (must be the configured one)", aq.Latitude, aq.Longitude)
	}
}

func TestAirQualityHistory(t *testing.T) {
	start := time.Date(2026, 4, 17, 0, 0, 0, 0, time.UTC)
	srv := serve(t, "/v1/air-quality", fixture(t, "air_quality_history.json"), func(r *http.Request) {
		q := r.URL.Query()
		if q.Get("start_date") != "2026-04-17" || q.Get("end_date") != "2026-04-18" || q.Get("hourly") == "" {
			t.Errorf("query = %v", q)
		}
	})
	defer srv.Close()
	rows, err := testClient(srv).AirQualityHistory(t.Context(), start, start.Add(47*time.Hour))
	if err != nil {
		t.Fatal(err)
	}
	if len(rows) != 48 || !rows[0].Time.Equal(start) || rows[0].PM25 == nil {
		t.Fatalf("rows = %d, first = %+v", len(rows), rows[0])
	}
}

func TestForecast(t *testing.T) {
	srv := serve(t, "/v1/forecast", fixture(t, "forecast.json"), func(r *http.Request) {
		if !strings.Contains(r.URL.Query().Get("hourly"), "precipitation_probability") {
			t.Error("hourly lacks precipitation_probability")
		}
	})
	defer srv.Close()
	hours, err := testClient(srv).Forecast(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	if len(hours) != 72 {
		t.Fatalf("hours = %d", len(hours))
	}
	h := hours[0]
	if !h.Time.Equal(time.Date(2026, 11, 21, 0, 0, 0, 0, time.UTC)) || h.TempC == nil || h.PrecipProbability == nil || h.CloudCover == nil {
		t.Errorf("first hour = %+v", h)
	}
}

func TestHistory(t *testing.T) {
	start := time.Date(2026, 4, 17, 0, 0, 0, 0, time.UTC)
	srv := serve(t, "/v1/forecast", fixture(t, "historical_forecast.json"), func(r *http.Request) {
		q := r.URL.Query()
		if q.Get("start_date") != "2026-04-17" || q.Get("end_date") != "2026-04-18" {
			t.Errorf("query = %v", q)
		}
	})
	defer srv.Close()
	rows, err := testClient(srv).History(t.Context(), start, start.Add(47*time.Hour))
	if err != nil {
		t.Fatal(err)
	}
	if len(rows) != 48 {
		t.Fatalf("rows = %d", len(rows))
	}
	r := rows[13]
	if !r.Time.Equal(start.Add(13*time.Hour)) || r.CloudCover == nil || r.Latitude != 12.345 {
		t.Errorf("row = %+v", r)
	}
	// Each hour carries its own day's sunrise and sunset, for daylight shading.
	if r.Sunrise.IsZero() || r.Sunset.IsZero() || r.Sunrise.Format("2006-01-02") != "2026-04-17" {
		t.Errorf("sunrise/sunset = %v / %v", r.Sunrise, r.Sunset)
	}
	if rows[30].Sunrise.Format("2006-01-02") != "2026-04-18" {
		t.Errorf("second day's sunrise = %v", rows[30].Sunrise)
	}
}

// One implausible value is dropped (nil) and reported, without losing the
// rest of the row or flooding the log.
func TestAirQuality_ValidatesValues(t *testing.T) {
	body := `{"current":{"time":"2026-11-21T20:00","pm2_5":-4,"pm10":12,"us_aqi":99999,"ozone":null}}`
	srv := serve(t, "/v1/air-quality", []byte(body), nil)
	defer srv.Close()
	c := testClient(srv)
	var warns []string
	c.Warn = func(f string, a ...any) { warns = append(warns, fmt.Sprintf(f, a...)) }
	for range 3 {
		aq, err := c.AirQuality(t.Context())
		if err != nil {
			t.Fatal(err)
		}
		if aq.PM25 != nil || aq.USAQI != nil || aq.Ozone != nil || aq.PM10 == nil || *aq.PM10 != 12 {
			t.Errorf("aq = %+v", aq)
		}
	}
	if len(warns) != 2 { // pm2_5 and us_aqi, once each
		t.Errorf("warnings = %v", warns)
	}
}

func TestRetriesRateLimitHonouringRetryAfter(t *testing.T) {
	var calls atomic.Int32
	body := fixture(t, "air_quality_current.json")
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if calls.Add(1) == 1 {
			w.Header().Set("Retry-After", "1")
			w.WriteHeader(http.StatusTooManyRequests)
			return
		}
		w.Write(body)
	}))
	defer srv.Close()
	start := time.Now()
	if _, err := testClient(srv).AirQuality(t.Context()); err != nil {
		t.Fatal(err)
	}
	if calls.Load() != 2 || time.Since(start) < 900*time.Millisecond {
		t.Errorf("calls = %d after %v", calls.Load(), time.Since(start))
	}
}

// The request URL carries the location; it must not reach the logs.
func TestErrorsNeverContainCoordinates(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {}))
	srv.Close()
	c := testClient(srv)
	for _, call := range []func() error{
		func() error { _, err := c.Fetch(t.Context()); return err },
		func() error { _, err := c.AirQuality(t.Context()); return err },
		func() error { _, err := c.Forecast(t.Context()); return err },
	} {
		err := call()
		if err == nil {
			t.Fatal("want error")
		}
		if strings.Contains(err.Error(), "12.345") || strings.Contains(err.Error(), "65.432") {
			t.Errorf("error leaks coordinates: %v", err)
		}
	}
}

func TestAPIErrorBody(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusBadRequest)
		w.Write([]byte(`{"error":true,"reason":"Cannot initialize WeatherVariable from invalid String value nope"}`))
	}))
	defer srv.Close()
	_, err := testClient(srv).Forecast(t.Context())
	if err == nil || !strings.Contains(err.Error(), "invalid String value") {
		t.Errorf("err = %v", err)
	}
}

// The weather table's original columns can't hold NULL, so an hour missing
// any of them is skipped rather than stored with a fake zero.
func TestHistory_SkipsHoursWithMissingValues(t *testing.T) {
	body := `{"hourly":{
		"time":["2026-04-17T00:00","2026-04-17T01:00","2026-04-17T02:00"],
		"temperature_2m":[10,11,12],"relative_humidity_2m":[50,null,52],"dew_point_2m":[1,2,3],
		"precipitation":[0,0,0],"weather_code":[1,1,1],"pressure_msl":[1010,1011,null],
		"wind_speed_10m":[5,5,5],"uv_index":[0,0,0],"cloud_cover":[10,null,30]},
		"daily":{"time":["2026-04-17"],"sunrise":["2026-04-17T13:00"],"sunset":["2026-04-18T02:00"]}}`
	srv := serve(t, "/v1/forecast", []byte(body), nil)
	defer srv.Close()
	start := time.Date(2026, 4, 17, 0, 0, 0, 0, time.UTC)
	rows, err := testClient(srv).History(t.Context(), start, start)
	if err != nil {
		t.Fatal(err)
	}
	if len(rows) != 1 || !rows[0].Time.Equal(start) {
		t.Errorf("rows = %+v, want only 00:00 (01:00 lacks humidity, 02:00 pressure)", rows)
	}
}

// The forecast comes in the station's local days (timezone=auto), four of
// them, so the strip's three local days are complete; times are converted
// to UTC with the response's offset.
func TestForecast_LocalDaysConvertedToUTC(t *testing.T) {
	body := `{"utc_offset_seconds":-28800,"hourly":{"time":["2026-11-21T00:00","2026-11-21T01:00"],
		"temperature_2m":[5,6],"precipitation_probability":[10,20],"precipitation":[0,0],
		"cloud_cover":[50,60],"uv_index":[0,0],"wind_speed_10m":[3,3],"weather_code":[3,3]}}`
	srv := serve(t, "/v1/forecast", []byte(body), func(r *http.Request) {
		q := r.URL.Query()
		if q.Get("timezone") != "auto" || q.Get("forecast_days") != "4" {
			t.Errorf("timezone=%q forecast_days=%q, want auto and 4", q.Get("timezone"), q.Get("forecast_days"))
		}
	})
	defer srv.Close()
	hours, err := testClient(srv).Forecast(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	if len(hours) != 2 || !hours[0].Time.Equal(time.Date(2026, 11, 21, 8, 0, 0, 0, time.UTC)) {
		t.Errorf("first hour = %v, want 08:00 UTC (local midnight at UTC-8)", hours[0].Time)
	}
}
