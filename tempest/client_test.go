package tempest

import (
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

func fixture(t *testing.T, name string) []byte {
	t.Helper()
	b, err := os.ReadFile("testdata/" + name)
	if err != nil {
		t.Fatal(err)
	}
	return b
}

func newTestClient(srv *httptest.Server, token string) *Client {
	c := NewClient(token)
	c.BaseURL = srv.URL
	c.RetryBase = time.Millisecond
	c.Now = func() time.Time { return allEnd }
	c.Warn = func(string, ...any) {}
	return c
}

// allStart..allEnd covers every fixture timestamp.
var allStart, allEnd = time.Unix(0, 0), time.Unix(4e9, 0)

func TestStations(t *testing.T) {
	body := fixture(t, "stations.json")
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/swd/rest/stations" {
			t.Errorf("path = %s", r.URL.Path)
		}
		if r.URL.Query().Get("token") != "tok" {
			t.Errorf("token = %q", r.URL.Query().Get("token"))
		}
		w.Write(body)
	}))
	defer srv.Close()

	stations, err := newTestClient(srv, "tok").Stations(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	if len(stations) != 1 {
		t.Fatalf("len = %d", len(stations))
	}
	s := stations[0]
	if s.ID != 1001 || s.Name != "Home" || s.Timezone != "America/Los_Angeles" || s.ElevationM != 50 {
		t.Errorf("station = %+v", s)
	}
	if len(s.Devices) != 2 || s.Devices[1].ID != 2002 || s.Devices[1].Type != "ST" || s.Devices[1].Serial != "ST-00000002" {
		t.Errorf("devices = %+v", s.Devices)
	}
}

// The live API names the list "stations"; the published swagger calls it
// "locations". Accept either.
func TestStations_LocationsKey(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(`{"status":{"status_code":0},"locations":[{"station_id":7,"name":"X","devices":[{"device_id":8,"device_type":"ST"}]}]}`))
	}))
	defer srv.Close()
	stations, err := newTestClient(srv, "tok").Stations(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	if len(stations) != 1 || stations[0].ID != 7 {
		t.Fatalf("stations = %+v", stations)
	}
}

func TestSelectDevices(t *testing.T) {
	stations := []Station{
		{ID: 1, Name: "Hub only", Devices: []Device{{ID: 10, Type: "HB"}}},
		{ID: 2, Name: "Home", Devices: []Device{{ID: 20, Type: "HB"}, {ID: 21, Type: "ST"}}},
		{ID: 3, Name: "Cabin", Devices: []Device{{ID: 31, Type: "ST"}}},
	}
	st, devs, err := SelectDevices(stations, 0)
	if err != nil {
		t.Fatal(err)
	}
	if st.ID != 2 || len(devs) != 1 || devs[0].ID != 21 {
		t.Errorf("auto: station %d devices %+v", st.ID, devs)
	}
	st, devs, err = SelectDevices(stations, 3)
	if err != nil || st.ID != 3 || devs[0].ID != 31 {
		t.Errorf("explicit: station %d devices %+v err %v", st.ID, devs, err)
	}
	if _, _, err := SelectDevices(stations, 99); err == nil {
		t.Error("unknown station: want error")
	}
	if _, _, err := SelectDevices(stations, 1); err == nil {
		t.Error("station without a Tempest device: want error")
	}
	if _, _, err := SelectDevices(nil, 0); err == nil {
		t.Error("no stations: want error")
	}
}

func f(v float64) *float64 { return &v }
func i(v int) *int         { return &v }

func eqF(a, b *float64) bool { return (a == nil && b == nil) || (a != nil && b != nil && *a == *b) }
func eqI(a, b *int) bool     { return (a == nil && b == nil) || (a != nil && b != nil && *a == *b) }

func TestDeviceObservations(t *testing.T) {
	body := fixture(t, "device_obs.json")
	start := time.Unix(1789564800, 0)
	end := start.Add(time.Hour)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/swd/rest/observations/device/2002" {
			t.Errorf("path = %s", r.URL.Path)
		}
		q := r.URL.Query()
		if q.Get("time_start") != "1789564800" || q.Get("time_end") != "1789568400" {
			t.Errorf("range = %s..%s", q.Get("time_start"), q.Get("time_end"))
		}
		w.Write(body)
	}))
	defer srv.Close()

	obs, err := newTestClient(srv, "tok").DeviceObservations(t.Context(), 2002, start, end)
	if err != nil {
		t.Fatal(err)
	}
	if len(obs) != 3 {
		t.Fatalf("len = %d", len(obs))
	}
	o := obs[1]
	if o.DeviceID != 2002 || !o.Time.Equal(time.Unix(1789564860, 0)) || o.Time.Location() != time.UTC {
		t.Errorf("identity = %d %v", o.DeviceID, o.Time)
	}
	checksF := []struct {
		name      string
		got, want *float64
	}{
		{"lull", o.WindLullMS, f(0.2)}, {"avg", o.WindAvgMS, f(1.4)}, {"gust", o.WindGustMS, f(3.1)},
		{"dir", o.WindDirectionDeg, f(270)}, {"sample", o.WindSampleIntervalS, f(3)},
		{"pressure", o.StationPressureMB, f(1011.1)}, {"temp", o.AirTempC, f(15.9)}, {"rh", o.RelativeHumidity, f(77)},
		{"lux", o.IlluminanceLux, f(1610)}, {"uv", o.UVIndex, f(0.13)}, {"solar", o.SolarRadiationWM2, f(14)},
		{"rain", o.RainMM, f(0.25)}, {"strikeDist", o.LightningAvgDistanceKM, f(12)}, {"battery", o.BatteryV, f(2.61)},
		{"dayRain", o.LocalDayRainMM, f(0.25)}, {"final", o.RainFinalMM, nil}, {"dayFinal", o.LocalDayRainFinalMM, nil},
	}
	for _, c := range checksF {
		if !eqF(c.got, c.want) {
			t.Errorf("%s = %v, want %v", c.name, deref(c.got), deref(c.want))
		}
	}
	if !eqI(o.PrecipType, i(1)) || !eqI(o.LightningStrikeCount, i(3)) || !eqI(o.ReportIntervalMin, i(1)) || !eqI(o.PrecipAnalysisType, i(0)) {
		t.Errorf("ints = %v %v %v %v", o.PrecipType, o.LightningStrikeCount, o.ReportIntervalMin, o.PrecipAnalysisType)
	}

	// Nulls stay nil (not zero), and short rows leave the REST-only fields nil.
	last := obs[2]
	if last.WindAvgMS != nil || last.LightningAvgDistanceKM != nil {
		t.Errorf("nulls decoded as values: %+v", last)
	}
	if last.LocalDayRainMM != nil || last.PrecipAnalysisType != nil {
		t.Errorf("18-field row filled REST-only fields: %+v", last)
	}
	if !eqF(last.AirTempC, f(16)) {
		t.Errorf("temp = %v", deref(last.AirTempC))
	}
}

func deref(p *float64) any {
	if p == nil {
		return nil
	}
	return *p
}

func TestDeviceObservations_NullObs(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(`{"status":{"status_code":0,"status_message":"SUCCESS"},"device_id":2002,"obs":null}`))
	}))
	defer srv.Close()
	obs, err := newTestClient(srv, "tok").DeviceObservations(t.Context(), 2002, allStart, allEnd)
	if err != nil {
		t.Fatal(err)
	}
	if len(obs) != 0 {
		t.Errorf("len = %d", len(obs))
	}
}

func TestDeviceObservations_APIStatusError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(`{"status":{"status_code":2,"status_message":"BAD DEVICE"}}`))
	}))
	defer srv.Close()
	_, err := newTestClient(srv, "tok").DeviceObservations(t.Context(), 1, allStart, allEnd)
	if err == nil || !strings.Contains(err.Error(), "BAD DEVICE") {
		t.Errorf("err = %v", err)
	}
}

func TestDeviceObservations_RetriesRateLimit(t *testing.T) {
	var calls atomic.Int32
	body := fixture(t, "device_obs.json")
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if calls.Add(1) < 3 {
			w.WriteHeader(http.StatusTooManyRequests)
			return
		}
		w.Write(body)
	}))
	defer srv.Close()
	obs, err := newTestClient(srv, "tok").DeviceObservations(t.Context(), 2002, allStart, allEnd)
	if err != nil {
		t.Fatal(err)
	}
	if calls.Load() != 3 || len(obs) != 3 {
		t.Errorf("calls = %d, obs = %d", calls.Load(), len(obs))
	}
}

func TestDeviceObservations_NoRetryOnUnauthorized(t *testing.T) {
	var calls atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		w.WriteHeader(http.StatusUnauthorized)
	}))
	defer srv.Close()
	_, err := newTestClient(srv, "tok").DeviceObservations(t.Context(), 2002, allStart, allEnd)
	if err == nil {
		t.Fatal("want error")
	}
	if calls.Load() != 1 {
		t.Errorf("calls = %d, want 1", calls.Load())
	}
}

// The token rides in the query string, and net/http puts the full URL in
// transport errors. None of it may reach a log line.
func TestErrorsNeverContainToken(t *testing.T) {
	const token = "s3cr3t-token-value"
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {}))
	srv.Close() // connection refused

	c := NewClient(token)
	c.BaseURL = srv.URL
	c.RetryBase = time.Millisecond
	_, err := c.DeviceObservations(t.Context(), 1, allStart, allEnd)
	if err == nil {
		t.Fatal("want error")
	}
	if strings.Contains(err.Error(), token) {
		t.Errorf("error leaks token: %v", err)
	}
	if _, err := c.Stations(t.Context()); err == nil || strings.Contains(err.Error(), token) {
		t.Errorf("stations error = %v", err)
	}
}

func TestWindows(t *testing.T) {
	from := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	to := from.Add(50 * time.Hour)
	got := Windows(from, to, 24*time.Hour)
	if len(got) != 3 {
		t.Fatalf("len = %d: %v", len(got), got)
	}
	if !got[0].Start.Equal(from) || !got[0].End.Equal(from.Add(24*time.Hour)) {
		t.Errorf("w0 = %v", got[0])
	}
	if !got[2].Start.Equal(from.Add(48*time.Hour)) || !got[2].End.Equal(to) {
		t.Errorf("w2 = %v", got[2])
	}
	if len(Windows(to, from, time.Hour)) != 0 {
		t.Error("reversed range should yield no windows")
	}
}

// live_*.json are real responses (token stripped; IDs, serials, names and
// location replaced) captured from a Tempest on 2026-11-20.
func TestStations_LiveFixture(t *testing.T) {
	body := fixture(t, "live_stations.json")
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { w.Write(body) }))
	defer srv.Close()
	stations, err := newTestClient(srv, "tok").Stations(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	st, devs, err := SelectDevices(stations, 0)
	if err != nil {
		t.Fatal(err)
	}
	if st.ID != 100001 || st.Timezone != "America/Los_Angeles" || len(devs) != 1 || devs[0].ID != 200002 || devs[0].Firmware != "193" {
		t.Errorf("station = %+v devices = %+v", st, devs)
	}
	if !st.Created.Equal(time.Unix(1700000000, 0)) {
		t.Errorf("created = %v", st.Created)
	}
}

func TestDeviceObservations_LiveFixture(t *testing.T) {
	body := fixture(t, "live_device_obs.json")
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { w.Write(body) }))
	defer srv.Close()
	obs, err := newTestClient(srv, "tok").DeviceObservations(t.Context(), 200002, allStart, allEnd)
	if err != nil {
		t.Fatal(err)
	}
	if len(obs) != 10 {
		t.Fatalf("len = %d", len(obs))
	}
	for _, o := range obs {
		if o.AirTempC == nil || o.StationPressureMB == nil || o.BatteryV == nil || o.LocalDayRainMM == nil || o.PrecipAnalysisType == nil {
			t.Errorf("missing fields: %+v", o)
		}
	}
	for i := 1; i < len(obs); i++ {
		if d := obs[i].Time.Sub(obs[i-1].Time); d != time.Minute {
			t.Errorf("step %d = %v", i, d)
		}
	}
}

// Ranges longer than about five days come back in 30-minute buckets
// (verified live: 6 days gave bucket_step_minutes 30). Those are aggregates,
// not minute rows, and must never be stored as if they were.
func TestDeviceObservations_RejectsCoarseBuckets(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(`{"status":{"status_code":0},"device_id":1,"bucket_step_minutes":30,"obs":[[1789564800,0,0,0,0,3,1000,10,50,0,0,0,0,0,0,0,2.6,30]]}`))
	}))
	defer srv.Close()
	_, err := newTestClient(srv, "tok").DeviceObservations(t.Context(), 1, allStart, allEnd)
	if err == nil || !strings.Contains(err.Error(), "30-minute") {
		t.Errorf("err = %v", err)
	}
}
