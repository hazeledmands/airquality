package sensibo

import (
	"math"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestGetDevices(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/users/me/pods" {
			t.Errorf("unexpected path: %s", r.URL.Path)
		}
		if r.URL.Query().Get("apiKey") != "test-key" {
			t.Errorf("unexpected apiKey: %s", r.URL.Query().Get("apiKey"))
		}
		w.Write([]byte(`{"status":"success","result":[{"id":"abc123","room":{"name":"Office","icon":"desk"}}]}`))
	}))
	defer srv.Close()

	c := NewClient("test-key")
	// Point client at test server.
	origBase := baseURL
	defer func() { setBaseURL(origBase) }()
	setBaseURL(srv.URL)

	devices, err := c.GetDevices()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(devices) != 1 {
		t.Fatalf("expected 1 device, got %d", len(devices))
	}
	if devices[0].ID != "abc123" {
		t.Errorf("expected ID abc123, got %s", devices[0].ID)
	}
	if devices[0].Room.Name != "Office" {
		t.Errorf("expected room Office, got %s", devices[0].Room.Name)
	}
}

func TestGetHistoricalMeasurements(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/pods/abc123/historicalMeasurements" {
			t.Errorf("unexpected path: %s", r.URL.Path)
		}
		if r.URL.Query().Get("days") != "1" {
			t.Errorf("unexpected days: %s", r.URL.Query().Get("days"))
		}
		w.Write([]byte(`{"status":"success","result":{
			"temperature":[{"time":"2026-03-15T10:00:00Z","value":22.5},{"time":"2026-03-15T11:00:00Z","value":23.0}],
			"humidity":[{"time":"2026-03-15T10:00:00Z","value":45.0},{"time":"2026-03-15T11:00:00Z","value":50.0}]
		}}`))
	}))
	defer srv.Close()

	c := NewClient("test-key")
	origBase := baseURL
	defer func() { setBaseURL(origBase) }()
	setBaseURL(srv.URL)

	m, err := c.GetHistoricalMeasurements("abc123", 1)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(m.Temperature) != 2 {
		t.Fatalf("expected 2 temperature readings, got %d", len(m.Temperature))
	}
	if len(m.Humidity) != 2 {
		t.Fatalf("expected 2 humidity readings, got %d", len(m.Humidity))
	}
	if m.Temperature[0].Value != 22.5 {
		t.Errorf("expected first temp 22.5, got %f", m.Temperature[0].Value)
	}
}

func TestGetDevices_APIError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusForbidden)
		w.Write([]byte(`{"status":"error","message":"invalid key"}`))
	}))
	defer srv.Close()

	c := NewClient("bad-key")
	origBase := baseURL
	defer func() { setBaseURL(origBase) }()
	setBaseURL(srv.URL)

	_, err := c.GetDevices()
	if err == nil {
		t.Fatal("expected error for 403 response")
	}
}

func TestSummarize(t *testing.T) {
	m := HistoricalMeasurements{
		Temperature: []TimedValue{
			{Time: "2026-03-15T10:00:00Z", Value: 20.0},
			{Time: "2026-03-15T11:00:00Z", Value: 22.0},
			{Time: "2026-03-15T12:00:00Z", Value: 24.0},
		},
		Humidity: []TimedValue{
			{Time: "2026-03-15T10:00:00Z", Value: 40.0},
			{Time: "2026-03-15T11:00:00Z", Value: 50.0},
			{Time: "2026-03-15T12:00:00Z", Value: 60.0},
		},
	}

	s := Summarize("dev1", "Office", m)

	if s.DeviceID != "dev1" {
		t.Errorf("expected DeviceID dev1, got %s", s.DeviceID)
	}
	if s.RoomName != "Office" {
		t.Errorf("expected RoomName Office, got %s", s.RoomName)
	}
	if s.DataPoints != 3 {
		t.Errorf("expected 3 data points, got %d", s.DataPoints)
	}
	if s.MinTemp != 20.0 {
		t.Errorf("expected MinTemp 20.0, got %f", s.MinTemp)
	}
	if s.MaxTemp != 24.0 {
		t.Errorf("expected MaxTemp 24.0, got %f", s.MaxTemp)
	}
	if s.AvgTemp != 22.0 {
		t.Errorf("expected AvgTemp 22.0, got %f", s.AvgTemp)
	}
	if s.MinHumid != 40.0 {
		t.Errorf("expected MinHumid 40.0, got %f", s.MinHumid)
	}
	if s.MaxHumid != 60.0 {
		t.Errorf("expected MaxHumid 60.0, got %f", s.MaxHumid)
	}
	if s.AvgHumid != 50.0 {
		t.Errorf("expected AvgHumid 50.0, got %f", s.AvgHumid)
	}
	// Latest should be the last entry.
	if s.Latest.Temperature != 24.0 {
		t.Errorf("expected latest temp 24.0, got %f", s.Latest.Temperature)
	}
	if s.Latest.Humidity != 60.0 {
		t.Errorf("expected latest humidity 60.0, got %f", s.Latest.Humidity)
	}
}

func TestDewPoint(t *testing.T) {
	tests := []struct {
		name     string
		tempC    float64
		humidity float64
		wantC    float64
	}{
		// Reference values from well-known psychrometric tables.
		{"typical indoor", 22.0, 50.0, 11.1},
		{"hot humid", 35.0, 80.0, 31.0},
		{"cold dry", 5.0, 30.0, -11.2},
		{"saturated", 20.0, 100.0, 20.0},
		{"very dry", 25.0, 10.0, -8.8},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := DewPoint(tc.tempC, tc.humidity)
			if math.Abs(got-tc.wantC) > 0.5 {
				t.Errorf("DewPoint(%.1f, %.1f) = %.1f, want %.1f (±0.5)", tc.tempC, tc.humidity, got, tc.wantC)
			}
		})
	}
}

func TestSummarize_IncludesDewPoint(t *testing.T) {
	m := HistoricalMeasurements{
		Temperature: []TimedValue{
			{Time: "2026-03-15T10:00:00Z", Value: 22.0},
			{Time: "2026-03-15T11:00:00Z", Value: 24.0},
		},
		Humidity: []TimedValue{
			{Time: "2026-03-15T10:00:00Z", Value: 50.0},
			{Time: "2026-03-15T11:00:00Z", Value: 60.0},
		},
	}

	s := Summarize("dev1", "Office", m)

	// Dew points: DewPoint(22,50)≈11.1, DewPoint(24,60)≈15.5
	if s.AvgDewPoint == 0 {
		t.Error("expected AvgDewPoint to be computed")
	}
	if s.MinDewPoint >= s.MaxDewPoint {
		t.Errorf("expected MinDewPoint < MaxDewPoint, got %.1f >= %.1f", s.MinDewPoint, s.MaxDewPoint)
	}
	if s.Latest.DewPoint == 0 {
		t.Error("expected latest DewPoint to be set")
	}
}

func TestSummarize_Empty(t *testing.T) {
	s := Summarize("dev1", "Empty", HistoricalMeasurements{})

	if s.DataPoints != 0 {
		t.Errorf("expected 0 data points, got %d", s.DataPoints)
	}
	if s.MinTemp != math.MaxFloat64 {
		t.Errorf("expected MinTemp to be MaxFloat64 for empty data")
	}
}
