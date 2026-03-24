package sensibo

import (
	"math"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
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

	devices, err := c.GetDevices(t.Context())
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

	m, err := c.GetHistoricalMeasurements(t.Context(), "abc123", 1)
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

	_, err := c.GetDevices(t.Context())
	if err == nil {
		t.Fatal("expected error for 403 response")
	}
}

func TestGet_RetriesOn429(t *testing.T) {
	attempts := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		attempts++
		if attempts < 3 {
			w.WriteHeader(http.StatusTooManyRequests)
			w.Write([]byte(`{"status":429,"message":"API Limit exceeded"}`))
			return
		}
		w.Write([]byte(`{"status":"success","result":[]}`))
	}))
	defer srv.Close()

	c := NewClient("test-key")
	c.retryDelay = 1 * time.Millisecond
	origBase := baseURL
	defer func() { setBaseURL(origBase) }()
	setBaseURL(srv.URL)

	_, err := c.GetDevices(t.Context())
	if err != nil {
		t.Fatalf("expected success after retries, got: %v", err)
	}
	if attempts != 3 {
		t.Errorf("expected 3 attempts, got %d", attempts)
	}
}

func TestGet_RespectsRetryAfterHeader(t *testing.T) {
	attempts := 0
	var retryAfterSeen string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		attempts++
		if attempts == 1 {
			w.Header().Set("Retry-After", "0")
			w.WriteHeader(http.StatusTooManyRequests)
			w.Write([]byte(`{"status":429,"message":"API Limit exceeded"}`))
			return
		}
		retryAfterSeen = "waited"
		w.Write([]byte(`{"status":"success","result":[]}`))
	}))
	defer srv.Close()

	c := NewClient("test-key")
	c.retryDelay = 1 * time.Millisecond
	origBase := baseURL
	defer func() { setBaseURL(origBase) }()
	setBaseURL(srv.URL)

	_, err := c.GetDevices(t.Context())
	if err != nil {
		t.Fatalf("expected success after retry, got: %v", err)
	}
	if retryAfterSeen != "waited" {
		t.Error("expected second attempt to be made")
	}
}

func TestGet_GivesUpAfterMaxRetries(t *testing.T) {
	attempts := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		attempts++
		w.WriteHeader(http.StatusTooManyRequests)
		w.Write([]byte(`{"status":429,"message":"API Limit exceeded"}`))
	}))
	defer srv.Close()

	c := NewClient("test-key")
	c.retryDelay = 1 * time.Millisecond
	origBase := baseURL
	defer func() { setBaseURL(origBase) }()
	setBaseURL(srv.URL)

	_, err := c.GetDevices(t.Context())
	if err == nil {
		t.Fatal("expected error after max retries")
	}
	if attempts != maxRetries+1 {
		t.Errorf("expected %d attempts, got %d", maxRetries+1, attempts)
	}
}

func TestGetACStates(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/pods/abc123/acStates" {
			t.Errorf("unexpected path: %s", r.URL.Path)
		}
		if r.URL.Query().Get("limit") != "20" {
			t.Errorf("unexpected limit: %s", r.URL.Query().Get("limit"))
		}
		if r.URL.Query().Get("apiKey") != "test-key" {
			t.Errorf("unexpected apiKey: %s", r.URL.Query().Get("apiKey"))
		}
		w.Write([]byte(`{"status":"success","result":[
			{"id":"QRLiZw9jnJ","time":{"time":"2026-03-16T13:01:02Z","secondsAgo":1000},"status":"Success",
			 "acState":{"on":false,"mode":"cool","targetTemperature":70,"temperatureUnit":"F","fanLevel":"quiet","swing":"fixedMiddle","horizontalSwing":"fixedCenter"},
			 "changedProperties":["on","mode"],"reason":"AntiMold"},
			{"id":"NMmdigja6z","time":{"time":"2026-03-16T12:55:48Z","secondsAgo":1300},"status":"Success",
			 "acState":{"on":true,"mode":"fan","targetTemperature":70,"temperatureUnit":"F","fanLevel":"low","swing":"fixedMiddle","horizontalSwing":"fixedCenter"},
			 "changedProperties":["mode"],"reason":"UserRequest"}
		],"isPartial":false}`))
	}))
	defer srv.Close()

	c := NewClient("test-key")
	origBase := baseURL
	defer func() { setBaseURL(origBase) }()
	setBaseURL(srv.URL)

	states, err := c.GetACStates(t.Context(), "abc123", 20)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(states) != 2 {
		t.Fatalf("expected 2 states, got %d", len(states))
	}

	first := states[0]
	if first.ID != "QRLiZw9jnJ" {
		t.Errorf("expected ID QRLiZw9jnJ, got %s", first.ID)
	}
	if first.Time.Time != "2026-03-16T13:01:02Z" {
		t.Errorf("expected time 2026-03-16T13:01:02Z, got %s", first.Time.Time)
	}
	if first.ACState.On != false {
		t.Errorf("expected on=false")
	}
	if first.ACState.Mode != "cool" {
		t.Errorf("expected mode cool, got %s", first.ACState.Mode)
	}
	if first.ACState.TargetTemperature != 70 {
		t.Errorf("expected targetTemperature 70, got %d", first.ACState.TargetTemperature)
	}
	if first.ACState.FanLevel != "quiet" {
		t.Errorf("expected fanLevel quiet, got %s", first.ACState.FanLevel)
	}
	if first.Reason != "AntiMold" {
		t.Errorf("expected reason AntiMold, got %s", first.Reason)
	}
	if len(first.ChangedProperties) != 2 {
		t.Errorf("expected 2 changed properties, got %d", len(first.ChangedProperties))
	}

	second := states[1]
	if second.ACState.On != true {
		t.Errorf("expected on=true for second state")
	}
	if second.Reason != "UserRequest" {
		t.Errorf("expected reason UserRequest, got %s", second.Reason)
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

func TestCtoF(t *testing.T) {
	tests := []struct {
		name  string
		c     float64
		wantF float64
	}{
		{"freezing", 0.0, 32.0},
		{"boiling", 100.0, 212.0},
		{"body temp", 37.0, 98.6},
		{"negative", -40.0, -40.0},
		{"room temp", 22.0, 71.6},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := CtoF(tc.c)
			if math.Abs(got-tc.wantF) > 0.1 {
				t.Errorf("CtoF(%.1f) = %.1f, want %.1f", tc.c, got, tc.wantF)
			}
		})
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
