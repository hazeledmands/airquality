package openmeteo

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

const sampleResponse = `{
	"latitude": 37.818,
	"longitude": -122.290,
	"timezone": "America/Los_Angeles",
	"current": {
		"time": "2026-03-22T12:45",
		"interval": 900,
		"temperature_2m": 20.5,
		"relative_humidity_2m": 58,
		"dew_point_2m": 11.9,
		"precipitation": 0.2,
		"weather_code": 3,
		"pressure_msl": 1015.1,
		"wind_speed_10m": 7.4,
		"uv_index": 6.25
	},
	"daily": {
		"time": ["2026-03-22"],
		"sunrise": ["2026-03-22T07:08"],
		"sunset":  ["2026-03-22T19:22"]
	}
}`

func TestFetch(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/forecast" {
			t.Errorf("unexpected path: %s", r.URL.Path)
		}
		q := r.URL.Query()
		if q.Get("latitude") == "" || q.Get("longitude") == "" {
			t.Error("missing lat/lon params")
		}
		w.Write([]byte(sampleResponse))
	}))
	defer srv.Close()

	c := NewClient(37.831, -122.285)
	c.baseURL = srv.URL

	reading, err := c.Fetch(t.Context())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if reading.TempC != 20.5 {
		t.Errorf("TempC = %v, want 20.5", reading.TempC)
	}
	if reading.Humidity != 58 {
		t.Errorf("Humidity = %v, want 58", reading.Humidity)
	}
	if reading.DewPointC != 11.9 {
		t.Errorf("DewPointC = %v, want 11.9", reading.DewPointC)
	}
	if reading.PrecipitationMM != 0.2 {
		t.Errorf("PrecipitationMM = %v, want 0.2", reading.PrecipitationMM)
	}
	if reading.WeatherCode != 3 {
		t.Errorf("WeatherCode = %v, want 3", reading.WeatherCode)
	}
	if reading.PressureHPa != 1015.1 {
		t.Errorf("PressureHPa = %v, want 1015.1", reading.PressureHPa)
	}
	if reading.WindSpeedKMH != 7.4 {
		t.Errorf("WindSpeedKMH = %v, want 7.4", reading.WindSpeedKMH)
	}
	if reading.UVIndex != 6.25 {
		t.Errorf("UVIndex = %v, want 6.25", reading.UVIndex)
	}
	if reading.Latitude != 37.831 {
		t.Errorf("Latitude = %v, want 37.831", reading.Latitude)
	}
	if reading.Longitude != -122.285 {
		t.Errorf("Longitude = %v, want -122.285", reading.Longitude)
	}

	// Sunrise/sunset parsed correctly
	wantSunrise := time.Date(2026, 3, 22, 7, 8, 0, 0, time.UTC)
	if !reading.Sunrise.Equal(wantSunrise) {
		t.Errorf("Sunrise = %v, want %v", reading.Sunrise, wantSunrise)
	}
	wantSunset := time.Date(2026, 3, 22, 19, 22, 0, 0, time.UTC)
	if !reading.Sunset.Equal(wantSunset) {
		t.Errorf("Sunset = %v, want %v", reading.Sunset, wantSunset)
	}
}

func TestFetch_APIError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusBadRequest)
		w.Write([]byte(`{"error":true,"reason":"invalid coordinates"}`))
	}))
	defer srv.Close()

	c := NewClient(0, 0)
	c.baseURL = srv.URL

	_, err := c.Fetch(t.Context())
	if err == nil {
		t.Fatal("expected error for 400 response")
	}
}

func TestFetch_EmptyDaily(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(`{
			"current": {"time":"2026-03-22T12:00","temperature_2m":18.0,"relative_humidity_2m":60,
			            "dew_point_2m":10.0,"precipitation":0,"weather_code":0,
			            "pressure_msl":1013.0,"wind_speed_10m":5.0,"uv_index":0},
			"daily": {"time":[],"sunrise":[],"sunset":[]}
		}`))
	}))
	defer srv.Close()

	c := NewClient(37.831, -122.285)
	c.baseURL = srv.URL

	reading, err := c.Fetch(t.Context())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !reading.Sunrise.IsZero() {
		t.Errorf("expected zero Sunrise for empty daily, got %v", reading.Sunrise)
	}
}
