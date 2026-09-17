package tempest

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

const sampleObservationResponse = `{
	"status": {
		"status_code": 0,
		"status_message": "SUCCESS"
	},
	"station_id": 233167,
	"station_name": "SkySnout",
	"latitude": 37.84126,
	"longitude": -122.27347,
	"obs": [
		{
			"timestamp": 1789564887,
			"air_temperature": 15.8,
			"relative_humidity": 78,
			"dew_point": 12.0,
			"station_pressure": 1011.0,
			"sea_level_pressure": 1013.9,
			"wind_avg": 0.4,
			"wind_direction": 75,
			"wind_gust": 0.5,
			"wind_lull": 0.3,
			"precip": 0.0,
			"precip_accum_local_day": 0.0,
			"uv": 0.0,
			"solar_radiation": 0,
			"lightning_strike_count": 0,
			"feels_like": 15.8,
			"heat_index": 15.8,
			"wind_chill": 15.8
		}
	]
}`

func TestFetch(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/swd/rest/observations/station/233167" {
			t.Errorf("unexpected path: %s", r.URL.Path)
		}
		if r.URL.Query().Get("token") != "test-token" {
			t.Errorf("unexpected token: %s", r.URL.Query().Get("token"))
		}
		w.Write([]byte(sampleObservationResponse))
	}))
	defer srv.Close()

	c := NewClient("test-token", "233167")
	c.baseURL = srv.URL

	obs, err := c.Fetch(t.Context())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if obs.TempC != 15.8 {
		t.Errorf("TempC = %v, want 15.8", obs.TempC)
	}
	if obs.Humidity != 78 {
		t.Errorf("Humidity = %v, want 78", obs.Humidity)
	}
	if obs.DewPointC != 12.0 {
		t.Errorf("DewPointC = %v, want 12.0", obs.DewPointC)
	}
	if obs.PressureHPa != 1011.0 {
		t.Errorf("PressureHPa = %v, want 1011.0", obs.PressureHPa)
	}
	if obs.WindSpeedMS != 0.4 {
		t.Errorf("WindSpeedMS = %v, want 0.4", obs.WindSpeedMS)
	}
	if obs.WindDirection != 75 {
		t.Errorf("WindDirection = %v, want 75", obs.WindDirection)
	}
	if obs.UVIndex != 0.0 {
		t.Errorf("UVIndex = %v, want 0.0", obs.UVIndex)
	}
	if obs.SolarRadiation != 0 {
		t.Errorf("SolarRadiation = %v, want 0", obs.SolarRadiation)
	}
	if obs.PrecipitationMM != 0.0 {
		t.Errorf("PrecipitationMM = %v, want 0.0", obs.PrecipitationMM)
	}
	if obs.LightningStrikeCount != 0 {
		t.Errorf("LightningStrikeCount = %v, want 0", obs.LightningStrikeCount)
	}
	wantTime := time.Unix(1789564887, 0).UTC()
	if !obs.Time.Equal(wantTime) {
		t.Errorf("Time = %v, want %v", obs.Time, wantTime)
	}
	if obs.StationName != "SkySnout" {
		t.Errorf("StationName = %v, want SkySnout", obs.StationName)
	}
}

func TestFetch_APIError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
		w.Write([]byte(`{"status":{"status_code":401,"status_message":"UNAUTHORIZED"}}`))
	}))
	defer srv.Close()

	c := NewClient("bad-token", "233167")
	c.baseURL = srv.URL

	_, err := c.Fetch(t.Context())
	if err == nil {
		t.Fatal("expected error for 401 response")
	}
}

func TestFetch_EmptyObs(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(`{"status":{"status_code":0,"status_message":"SUCCESS"},"obs":[]}`))
	}))
	defer srv.Close()

	c := NewClient("test-token", "233167")
	c.baseURL = srv.URL

	_, err := c.Fetch(t.Context())
	if err == nil {
		t.Fatal("expected error for empty obs")
	}
}

const sampleStationsResponse = `{
	"stations": [
		{
			"station_id": 233167,
			"station_name": "SkySnout",
			"latitude": 37.84126,
			"longitude": -122.27347
		}
	]
}`

func TestDiscoverStationID(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/swd/rest/stations/" {
			t.Errorf("unexpected path: %s", r.URL.Path)
		}
		if r.URL.Query().Get("token") != "test-token" {
			t.Errorf("unexpected token: %s", r.URL.Query().Get("token"))
		}
		w.Write([]byte(sampleStationsResponse))
	}))
	defer srv.Close()

	id, err := DiscoverStationID(t.Context(), srv.URL, "test-token", srv.Client())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if id != "233167" {
		t.Errorf("StationID = %q, want 233167", id)
	}
}

func TestDiscoverStationID_NoStations(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(`{"stations":[]}`))
	}))
	defer srv.Close()

	_, err := DiscoverStationID(t.Context(), srv.URL, "test-token", srv.Client())
	if err == nil {
		t.Fatal("expected error for empty stations list")
	}
}

func TestDiscoverStationID_APIError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
	}))
	defer srv.Close()

	_, err := DiscoverStationID(t.Context(), srv.URL, "test-token", srv.Client())
	if err == nil {
		t.Fatal("expected error for 401 response")
	}
}
