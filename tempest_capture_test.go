package main

import (
	"context"
	"encoding/json"
	"errors"
	"net/http/httptest"
	"sort"
	"strconv"
	"sync"
	"testing"
	"time"

	"github.com/hazel/airquality/config"
	"github.com/hazel/airquality/tempest"
)

func fp(v float64) *float64 { return &v }
func ip(v int) *int         { return &v }

// fakeTempestAPI serves one observation per minute for any range.
type fakeTempestAPI struct {
	mu       sync.Mutex
	stations []tempest.Station
	calls    []tempest.Window
	failObs  error
	// finalMM, when set, is returned as the Rain Check value (simulating
	// WeatherFlow filling it in later).
	finalMM *float64
}

func (f *fakeTempestAPI) Stations(context.Context) ([]tempest.Station, error) {
	return f.stations, nil
}

func (f *fakeTempestAPI) DeviceObservations(_ context.Context, id int, start, end time.Time) ([]tempest.Observation, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.calls = append(f.calls, tempest.Window{Start: start, End: end})
	if f.failObs != nil {
		return nil, f.failObs
	}
	var out []tempest.Observation
	for t := start.Truncate(time.Minute); !t.After(end); t = t.Add(time.Minute) {
		if t.Before(start) {
			continue
		}
		out = append(out, tempest.Observation{DeviceID: id, Time: t.UTC(), AirTempC: fp(10), RainFinalMM: f.finalMM})
	}
	return out, nil
}

// fakeTempestStore is an in-memory tempest_observations keyed like the real
// primary key, with the same merge semantics.
type fakeTempestStore struct {
	mu     sync.Mutex
	rows   map[string]tempest.Observation
	events map[string]tempest.Event
}

func newFakeTempestStore() *fakeTempestStore {
	return &fakeTempestStore{rows: map[string]tempest.Observation{}, events: map[string]tempest.Event{}}
}

func obsKey(o tempest.Observation) string {
	return strconv.Itoa(o.DeviceID) + "/" + strconv.FormatInt(o.Time.Unix(), 10)
}

func (s *fakeTempestStore) UpsertTempestObservations(_ context.Context, obs []tempest.Observation) (int64, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	var n int64
	for _, o := range obs {
		old, ok := s.rows[obsKey(o)]
		if !ok || (o.RainFinalMM != nil && (old.RainFinalMM == nil || *old.RainFinalMM != *o.RainFinalMM)) {
			n++
		}
		if ok && o.RainFinalMM == nil {
			o.RainFinalMM = old.RainFinalMM
		}
		s.rows[obsKey(o)] = o
	}
	return n, nil
}

func (s *fakeTempestStore) LatestTempestObservation(_ context.Context, id int) (time.Time, bool, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	var latest time.Time
	for _, o := range s.rows {
		if o.DeviceID == id && o.Time.After(latest) {
			latest = o.Time
		}
	}
	return latest, !latest.IsZero(), nil
}

func (s *fakeTempestStore) InsertTempestEvents(_ context.Context, evs []tempest.Event) ([]tempest.Event, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	var inserted []tempest.Event
	for _, e := range evs {
		k := tempestEventKey(e)
		if _, ok := s.events[k]; !ok {
			s.events[k] = e
			inserted = append(inserted, e)
		}
	}
	return inserted, nil
}

// uniqueObs counts distinct (device, timestamp) pairs; window boundaries are
// fetched twice, which the store and cache absorb.
func uniqueObs(obs []tempest.Observation) int {
	seen := map[string]bool{}
	for _, o := range obs {
		seen[obsKey(o)] = true
	}
	return len(seen)
}

func homeStation() []tempest.Station {
	return []tempest.Station{{ID: 1001, Name: "Home", Devices: []tempest.Device{{ID: 2001, Type: "HB"}, {ID: 2002, Type: "ST"}}}}
}

func newTestRecorder(api *fakeTempestAPI, db *fakeTempestStore, now *time.Time) (*tempestRecorder, *[]tempest.Observation) {
	var delivered []tempest.Observation
	r := &tempestRecorder{
		onNewObs: func(tempest.Station, []tempest.Observation) {},
		api:      api,
		db:       db,
		now:      func() time.Time { return *now },
		onObs: func(_ tempest.Station, obs []tempest.Observation) {
			delivered = append(delivered, obs...)
		},
		onEvent: func(tempest.Station, tempest.Event) {},
	}
	return r, &delivered
}

func TestTempestRecorder_FirstPollFetchesLastDayThenOnlyNewMinutes(t *testing.T) {
	api := &fakeTempestAPI{stations: homeStation()}
	db := newFakeTempestStore()
	now := time.Date(2026, 11, 20, 12, 0, 30, 0, time.UTC)
	r, delivered := newTestRecorder(api, db, &now)

	if err := r.Poll(t.Context()); err != nil {
		t.Fatal(err)
	}
	// The first poll runs the Rain Check recheck, which covers the last 72h
	// (4321 minute marks inclusive) and so also the startup catch-up.
	if len(db.rows) != 72*60+1 {
		t.Errorf("rows after first poll = %d", len(db.rows))
	}
	if n := uniqueObs(*delivered); n != 72*60+1 {
		t.Errorf("delivered = %d distinct", n)
	}

	// One minute later: exactly one new row, nothing re-delivered.
	*delivered = nil
	api.calls = nil
	now = now.Add(time.Minute)
	if err := r.Poll(t.Context()); err != nil {
		t.Fatal(err)
	}
	if len(*delivered) != 1 || !(*delivered)[0].Time.Equal(time.Date(2026, 11, 20, 12, 1, 0, 0, time.UTC)) {
		t.Errorf("delivered = %+v", *delivered)
	}
	if len(api.calls) != 1 || !api.calls[0].Start.Equal(time.Date(2026, 11, 20, 12, 0, 1, 0, time.UTC)) {
		t.Errorf("calls = %+v", api.calls)
	}
	if len(db.rows) != 72*60+2 {
		t.Errorf("rows = %d", len(db.rows))
	}
}

// Polling faster than the device reports (or a main --interval that differs)
// must never duplicate a row.
func TestTempestRecorder_FastPollingNoDuplicates(t *testing.T) {
	api := &fakeTempestAPI{stations: homeStation()}
	db := newFakeTempestStore()
	now := time.Date(2026, 11, 20, 12, 0, 0, 0, time.UTC)
	r, delivered := newTestRecorder(api, db, &now)
	r.Poll(t.Context())
	*delivered = nil
	for range 4 {
		now = now.Add(15 * time.Second)
		if err := r.Poll(t.Context()); err != nil {
			t.Fatal(err)
		}
	}
	if len(*delivered) != 1 {
		t.Errorf("delivered = %d over one minute of 15 s polls, want 1", len(*delivered))
	}
}

func TestTempestRecorder_ResumesFromStoredLatest(t *testing.T) {
	api := &fakeTempestAPI{stations: homeStation()}
	db := newFakeTempestStore()
	now := time.Date(2026, 11, 20, 12, 0, 0, 0, time.UTC)
	db.UpsertTempestObservations(t.Context(), []tempest.Observation{{DeviceID: 2002, Time: now.Add(-10 * time.Minute)}})
	r, delivered := newTestRecorder(api, db, &now)
	r.lastRecheck = now // skip the recheck for this test
	if err := r.Poll(t.Context()); err != nil {
		t.Fatal(err)
	}
	if len(*delivered) != 10 {
		t.Errorf("delivered = %d, want the 10 missing minutes", len(*delivered))
	}
}

func TestTempestRecorder_RecheckPicksUpRainCheckOncePerDay(t *testing.T) {
	api := &fakeTempestAPI{stations: homeStation()}
	db := newFakeTempestStore()
	now := time.Date(2026, 11, 20, 12, 0, 0, 0, time.UTC)
	r, _ := newTestRecorder(api, db, &now)
	r.Poll(t.Context())

	api.finalMM = fp(1.5)
	now = now.Add(time.Minute)
	api.calls = nil
	r.Poll(t.Context())
	if len(api.calls) != 1 {
		t.Errorf("recheck ran again within a day: %d calls", len(api.calls))
	}
	now = now.Add(24 * time.Hour)
	r.Poll(t.Context())
	yesterday := db.rows[obsKey(tempest.Observation{DeviceID: 2002, Time: time.Date(2026, 11, 20, 6, 0, 0, 0, time.UTC)})]
	if yesterday.RainFinalMM == nil || *yesterday.RainFinalMM != 1.5 {
		t.Errorf("Rain Check value not refetched: %+v", yesterday)
	}
}

func TestTempestRecorder_DiscoveryFailureRetriesNextPoll(t *testing.T) {
	api := &fakeTempestAPI{}
	db := newFakeTempestStore()
	now := time.Date(2026, 11, 20, 12, 0, 0, 0, time.UTC)
	r, _ := newTestRecorder(api, db, &now)
	if err := r.Poll(t.Context()); err == nil {
		t.Fatal("want error with no stations")
	}
	api.stations = homeStation()
	if err := r.Poll(t.Context()); err != nil {
		t.Fatal(err)
	}
	if r.station.ID != 1001 || len(r.devices) != 1 {
		t.Errorf("station = %+v devices = %+v", r.station, r.devices)
	}
}

func TestTempestRecorder_FetchErrorKeepsPosition(t *testing.T) {
	api := &fakeTempestAPI{stations: homeStation()}
	db := newFakeTempestStore()
	now := time.Date(2026, 11, 20, 12, 0, 0, 0, time.UTC)
	r, delivered := newTestRecorder(api, db, &now)
	r.Poll(t.Context())
	*delivered = nil

	api.failObs = errors.New("boom")
	now = now.Add(5 * time.Minute)
	if err := r.Poll(t.Context()); err == nil {
		t.Fatal("want error")
	}
	api.failObs = nil
	now = now.Add(time.Minute)
	r.Poll(t.Context())
	if len(*delivered) != 6 {
		t.Errorf("delivered after recovery = %d, want 6", len(*delivered))
	}
}

func TestTempestBackfill_IdempotentAndChunked(t *testing.T) {
	api := &fakeTempestAPI{stations: homeStation()}
	db := newFakeTempestStore()
	from := time.Date(2026, 11, 1, 0, 0, 0, 0, time.UTC)
	to := from.Add(72 * time.Hour)
	opts := backfillOptions{from: from, to: to, stationID: 0}

	res, err := runTempestBackfill(t.Context(), api, db, opts, func(string, ...any) {})
	if err != nil {
		t.Fatal(err)
	}
	if len(api.calls) != 3 {
		t.Errorf("requests = %d, want 3 one-day windows", len(api.calls))
	}
	for _, c := range api.calls {
		if c.End.Sub(c.Start) > tempest.MaxWindow {
			t.Errorf("window %v exceeds MaxWindow", c)
		}
	}
	rows := len(db.rows)
	if rows != 72*60+1 || res.changed != int64(rows) {
		t.Errorf("rows = %d changed = %d", rows, res.changed)
	}
	res, err = runTempestBackfill(t.Context(), api, db, opts, func(string, ...any) {})
	if err != nil {
		t.Fatal(err)
	}
	if len(db.rows) != rows || res.changed != 0 {
		t.Errorf("re-run: rows = %d changed = %d, want unchanged", len(db.rows), res.changed)
	}
	if res.fetched != int64(rows)+2 { // window boundaries are fetched twice
		t.Errorf("fetched = %d", res.fetched)
	}
}

func TestTempestBackfill_DefaultsToStationCreation(t *testing.T) {
	created := time.Date(2026, 11, 18, 6, 0, 0, 0, time.UTC)
	st := homeStation()
	st[0].Created = created
	api := &fakeTempestAPI{stations: st}
	db := newFakeTempestStore()
	opts := backfillOptions{to: created.Add(36 * time.Hour)}
	if _, err := runTempestBackfill(t.Context(), api, db, opts, func(string, ...any) {}); err != nil {
		t.Fatal(err)
	}
	if len(api.calls) != 2 || !api.calls[0].Start.Equal(created) {
		t.Errorf("calls = %+v", api.calls)
	}

	api = &fakeTempestAPI{stations: homeStation()} // no created_epoch
	if _, err := runTempestBackfill(t.Context(), api, db, opts, func(string, ...any) {}); err == nil {
		t.Error("no --from and no creation date: want error")
	}
}

func TestParseBackfillArgs(t *testing.T) {
	now := time.Date(2026, 11, 20, 12, 0, 0, 0, time.UTC)
	o, err := parseBackfillArgs([]string{"--from", "2026-01-02"}, now)
	if err != nil {
		t.Fatal(err)
	}
	if !o.from.Equal(time.Date(2026, 1, 2, 0, 0, 0, 0, time.UTC)) || !o.to.Equal(now) {
		t.Errorf("opts = %+v", o)
	}
	o, err = parseBackfillArgs([]string{"--from", "2026-01-02T03:04:05Z", "--to", "2026-01-03"}, now)
	if err != nil || !o.to.Equal(time.Date(2026, 1, 3, 0, 0, 0, 0, time.UTC)) || o.from.Hour() != 3 {
		t.Errorf("opts = %+v err = %v", o, err)
	}
	o, err = parseBackfillArgs([]string{}, now)
	if err != nil || !o.from.IsZero() {
		t.Errorf("no --from means 'since the station was created': opts = %+v err = %v", o, err)
	}
	if _, err := parseBackfillArgs([]string{"--from", "2026-02-01", "--to", "2026-01-01"}, now); err == nil {
		t.Error("from after to: want error")
	}
}

func TestTempestRecorder_HandleEventStoresAndNotifies(t *testing.T) {
	api := &fakeTempestAPI{stations: homeStation()}
	db := newFakeTempestStore()
	now := time.Now()
	r, _ := newTestRecorder(api, db, &now)
	var got []tempest.Event
	r.onEvent = func(_ tempest.Station, e tempest.Event) { got = append(got, e) }
	e := tempest.Event{DeviceID: 2002, Time: now, Type: tempest.EventLightningStrike, DistanceKM: fp(5)}
	r.HandleEvent(t.Context(), e)
	r.HandleEvent(t.Context(), e) // replay: stored once, counted once
	if len(db.events) != 1 || len(got) != 1 {
		t.Errorf("stored = %d notified = %d", len(db.events), len(got))
	}
}

// --- downsampling ---

func TestDownsampleTempest_AggregatesByMeaning(t *testing.T) {
	from := time.Date(2026, 11, 20, 0, 0, 0, 0, time.UTC)
	mk := func(min int, o tempest.Observation) tempest.Observation {
		o.DeviceID = 2002
		o.Time = from.Add(time.Duration(min) * time.Minute)
		return o
	}
	obs := []tempest.Observation{
		mk(0, tempest.Observation{AirTempC: fp(10), WindLullMS: fp(1), WindAvgMS: fp(2), WindGustMS: fp(4), WindDirectionDeg: fp(350),
			RainMM: fp(0.2), LightningStrikeCount: ip(1), LightningAvgDistanceKM: fp(10), LocalDayRainMM: fp(5), PrecipType: ip(0)}),
		mk(1, tempest.Observation{AirTempC: fp(12), WindLullMS: fp(0.5), WindAvgMS: fp(2), WindGustMS: fp(7), WindDirectionDeg: fp(10),
			RainMM: fp(0.3), LightningStrikeCount: ip(3), LightningAvgDistanceKM: fp(2), LocalDayRainMM: fp(5.3), PrecipType: ip(1)}),
		mk(2, tempest.Observation{AirTempC: fp(11), RainMM: nil, LightningStrikeCount: ip(0), LocalDayRainMM: fp(0.1)}),
		// next 5-minute bucket
		mk(5, tempest.Observation{AirTempC: fp(20)}),
	}
	got := downsampleTempest(obs, from, 5*60*1000)
	if len(got) != 2 {
		t.Fatalf("buckets = %d", len(got))
	}
	b := got[0]
	check := func(name string, p *float64, want float64) {
		t.Helper()
		if p == nil || absf(*p-want) > 1e-6 {
			t.Errorf("%s = %v, want %v", name, ptrVal(p), want)
		}
	}
	check("temp median", b.AirTempC, 11)
	check("lull min", b.WindLullMS, 0.5)
	check("gust max", b.WindGustMS, 7)
	check("avg median", b.WindAvgMS, 2)
	check("rain sum", b.RainMM, 0.5)
	check("local day rain: day total before the reset", b.LocalDayRainMM, 5.3)
	check("distance strike-weighted", b.LightningAvgDistanceKM, (10*1+2*3)/4.0)
	if b.LightningStrikeCount == nil || *b.LightningStrikeCount != 4 {
		t.Errorf("strikes = %v, want 4", b.LightningStrikeCount)
	}
	if b.PrecipType == nil || *b.PrecipType != 1 {
		t.Errorf("precip type = %v, want max 1", b.PrecipType)
	}
	// 350° and 10° average to north, not south.
	if b.WindDirectionDeg == nil || !(*b.WindDirectionDeg > 359 || *b.WindDirectionDeg < 1) {
		t.Errorf("direction = %v, want ~0", ptrVal(b.WindDirectionDeg))
	}
	if !b.Time.Equal(from) {
		t.Errorf("bucket time = %v", b.Time)
	}
	if got[1].WindGustMS != nil || got[1].RainMM != nil {
		t.Errorf("all-null fields must stay null: %+v", got[1])
	}
}

func absf(v float64) float64 {
	if v < 0 {
		return -v
	}
	return v
}

func ptrVal(p *float64) any {
	if p == nil {
		return nil
	}
	return *p
}

// --- cache and API ---

func newTestCache() *dataCache {
	return &dataCache{
		seenReadings: make(map[string]bool),
		seenACStates: make(map[string]bool),
		seenWeather:  make(map[string]bool),
	}
}

func TestDataCache_TempestReplacesSameMinute(t *testing.T) {
	c := newTestCache()
	ts := time.Now().UTC().Truncate(time.Minute)
	c.addTempest([]tempest.Observation{{DeviceID: 1, Time: ts, RainMM: fp(1)}, {DeviceID: 1, Time: ts.Add(time.Minute)}})
	c.addTempest([]tempest.Observation{{DeviceID: 1, Time: ts, RainMM: fp(1), RainFinalMM: fp(1.2)}})
	obs, _ := c.snapshotTempest(ts.Add(-time.Hour), ts.Add(time.Hour))
	if len(obs) != 2 {
		t.Fatalf("len = %d", len(obs))
	}
	if obs[0].RainFinalMM == nil || *obs[0].RainFinalMM != 1.2 {
		t.Errorf("not replaced: %+v", obs[0])
	}
	c.addTempestEvent(tempest.Event{DeviceID: 1, Time: ts, Type: tempest.EventRainStart})
	c.addTempestEvent(tempest.Event{DeviceID: 1, Time: ts, Type: tempest.EventRainStart})
	_, evs := c.snapshotTempest(ts.Add(-time.Hour), ts.Add(time.Hour))
	if len(evs) != 1 {
		t.Errorf("events = %d, want deduped 1", len(evs))
	}
}

func TestDataHandler_IncludesTempest(t *testing.T) {
	c := newTestCache()
	now := time.Now().UTC().Truncate(time.Minute)
	c.setTempestStation(tempest.Station{ID: 1001, Name: "Home"}, []tempest.Device{{ID: 2002, Type: "ST"}})
	c.addTempest([]tempest.Observation{
		{DeviceID: 2002, Time: now.Add(-2 * time.Minute), WindGustMS: fp(3), AirTempC: fp(10), RelativeHumidity: fp(50)},
		{DeviceID: 2002, Time: now.Add(-time.Minute), WindGustMS: fp(6), AirTempC: fp(11), RelativeHumidity: fp(50)},
	})
	c.addTempestEvent(tempest.Event{DeviceID: 2002, Time: now.Add(-time.Minute), Type: tempest.EventLightningStrike, DistanceKM: fp(8)})

	cfg := config.Config{Sensors: map[string]config.Sensor{"tempest/2002": {Room: "Garden"}}}
	h := dataHandler(c, cfg)
	from := now.Add(-time.Hour).UnixMilli()
	req := httptest.NewRequest("GET", "/api/data?from="+strconv.FormatInt(from, 10)+"&stepSize=300000", nil)
	w := httptest.NewRecorder()
	h.ServeHTTP(w, req)

	var body struct {
		Tempest struct {
			Station struct {
				ID   int    `json:"id"`
				Name string `json:"name"`
				Room string `json:"room"`
			} `json:"station"`
			Observations []map[string]any `json:"observations"`
			Events       []map[string]any `json:"events"`
		} `json:"tempest"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	tp := body.Tempest
	if tp.Station.Name != "Home" || tp.Station.ID != 1001 || tp.Station.Room != "Garden" {
		t.Errorf("station = %+v", tp.Station)
	}
	if len(tp.Observations) == 0 || len(tp.Observations) > 2 {
		t.Fatalf("observations = %d", len(tp.Observations))
	}
	gusts := []float64{}
	for _, o := range tp.Observations {
		gusts = append(gusts, o["wind_gust_ms"].(float64))
		if _, ok := o["dewpoint_c"]; !ok {
			t.Error("dewpoint_c missing")
		}
	}
	sort.Float64s(gusts)
	if gusts[len(gusts)-1] != 6 {
		t.Errorf("gusts = %v; downsampling must keep the max gust", gusts)
	}
	if len(tp.Events) != 1 || tp.Events[0]["type"] != tempest.EventLightningStrike {
		t.Errorf("events = %+v", tp.Events)
	}
}

func TestDataHandler_TempestEmptyIsArrays(t *testing.T) {
	h := dataHandler(newTestCache(), config.Config{})
	w := httptest.NewRecorder()
	h.ServeHTTP(w, httptest.NewRequest("GET", "/api/data", nil))
	var body map[string]json.RawMessage
	json.Unmarshal(w.Body.Bytes(), &body)
	var tp struct {
		Observations json.RawMessage `json:"observations"`
		Events       json.RawMessage `json:"events"`
	}
	json.Unmarshal(body["tempest"], &tp)
	if string(tp.Observations) != "[]" || string(tp.Events) != "[]" {
		t.Errorf("tempest = %s", body["tempest"])
	}
}

func TestResolveTempestStationID(t *testing.T) {
	file := config.Config{Tempest: config.Tempest{StationID: 5}}
	if id, err := resolveTempestStationID("", file); err != nil || id != 5 {
		t.Errorf("file: %d %v", id, err)
	}
	if id, err := resolveTempestStationID("1001", file); err != nil || id != 1001 {
		t.Errorf("env overrides: %d %v", id, err)
	}
	if _, err := resolveTempestStationID("abc", file); err == nil {
		t.Error("non-numeric: want error")
	}
}

// A stored timestamp in the future must not become the resume point forever.
func TestTempestRecorder_FutureLatestIsClamped(t *testing.T) {
	api := &fakeTempestAPI{stations: homeStation()}
	db := newFakeTempestStore()
	now := time.Date(2026, 11, 20, 12, 0, 0, 0, time.UTC)
	db.UpsertTempestObservations(t.Context(), []tempest.Observation{{DeviceID: 2002, Time: now.Add(2 * time.Hour)}})
	r, delivered := newTestRecorder(api, db, &now)
	r.lastRecheck = now
	r.Poll(t.Context())
	now = now.Add(time.Minute)
	if err := r.Poll(t.Context()); err != nil {
		t.Fatal(err)
	}
	if len(*delivered) != 1 || !(*delivered)[0].Time.Equal(now) {
		t.Errorf("delivered = %+v, want the new minute", *delivered)
	}
}

// A recheck that fails is not retried every minute.
func TestTempestRecorder_FailedRecheckDoesNotSpin(t *testing.T) {
	api := &fakeTempestAPI{stations: homeStation(), failObs: errors.New("boom")}
	db := newFakeTempestStore()
	now := time.Date(2026, 11, 20, 12, 0, 0, 0, time.UTC)
	db.UpsertTempestObservations(t.Context(), []tempest.Observation{{DeviceID: 2002, Time: now.Add(-10 * time.Minute)}})
	r, _ := newTestRecorder(api, db, &now)
	if err := r.Poll(t.Context()); err == nil {
		t.Fatal("want error")
	}
	api.failObs = nil
	api.calls = nil
	now = now.Add(time.Minute)
	r.Poll(t.Context())
	for _, c := range api.calls {
		if c.End.Sub(c.Start) > 2*time.Hour {
			t.Errorf("recheck ran again: %+v", c)
		}
	}
}

// Totals count each observation once: rechecked history is not new.
func TestTempestRecorder_NewObsExcludeRecheckedHistory(t *testing.T) {
	api := &fakeTempestAPI{stations: homeStation()}
	db := newFakeTempestStore()
	now := time.Date(2026, 11, 20, 12, 0, 0, 0, time.UTC)
	db.UpsertTempestObservations(t.Context(), []tempest.Observation{{DeviceID: 2002, Time: now.Add(-3 * time.Minute)}})
	r, _ := newTestRecorder(api, db, &now)
	var fresh []tempest.Observation
	r.onNewObs = func(_ tempest.Station, obs []tempest.Observation) { fresh = append(fresh, obs...) }
	r.Poll(t.Context()) // includes the 72h recheck
	if uniqueObs(fresh) != 3 || len(fresh) != 3 {
		t.Errorf("new = %d (%d distinct), want the 3 minutes after the stored row", len(fresh), uniqueObs(fresh))
	}
	fresh = nil
	now = now.Add(24*time.Hour + time.Minute) // next recheck
	r.Poll(t.Context())
	if uniqueObs(fresh) != 24*60+1 {
		t.Errorf("new after a day = %d, want 1441", uniqueObs(fresh))
	}
}

func TestDataCache_TrimsOldEventsAndKeysOnDistance(t *testing.T) {
	c := newTestCache()
	now := time.Now().UTC()
	old := tempest.Event{DeviceID: 1, Time: now.Add(-cacheDuration - time.Hour), Type: tempest.EventRainStart}
	c.addTempestEvent(old)
	a := tempest.Event{DeviceID: 1, Time: now, Type: tempest.EventLightningStrike, DistanceKM: fp(5)}
	b := tempest.Event{DeviceID: 1, Time: now, Type: tempest.EventLightningStrike, DistanceKM: fp(9)}
	c.addTempestEvent(a)
	c.addTempestEvent(b)
	if len(c.tempestEvents) != 2 || len(c.seenTempestEvents) != 2 {
		t.Errorf("events = %d seen = %d, want old trimmed and both same-second strikes kept", len(c.tempestEvents), len(c.seenTempestEvents))
	}
}

func TestDataHandler_TempestLatestIsRawAndEnabled(t *testing.T) {
	c := newTestCache()
	c.tempestEnabled = true
	now := time.Now().UTC().Truncate(time.Minute)
	c.setTempestStation(tempest.Station{ID: 1001, Name: "Home", ElevationM: 22}, []tempest.Device{{ID: 2002, Type: "ST"}})
	var obs []tempest.Observation
	for i := 9; i >= 1; i-- {
		obs = append(obs, tempest.Observation{DeviceID: 2002, Time: now.Add(-time.Duration(i) * time.Minute), RainMM: fp(0.1), ReportIntervalMin: ip(1), StationPressureMB: fp(1013.8)})
	}
	c.addTempest(obs)
	h := dataHandler(c, config.Config{})
	from := now.Add(-time.Hour).UnixMilli()
	w := httptest.NewRecorder()
	h.ServeHTTP(w, httptest.NewRequest("GET", "/api/data?from="+strconv.FormatInt(from, 10)+"&stepSize=600000", nil))
	var body struct {
		Tempest struct {
			Enabled bool             `json:"enabled"`
			Latest  map[string]any   `json:"latest"`
			Obs     []map[string]any `json:"observations"`
			Station map[string]any   `json:"station"`
		} `json:"tempest"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	tp := body.Tempest
	if !tp.Enabled {
		t.Error("enabled = false")
	}
	if tp.Latest["rain_mm"].(float64) != 0.1 {
		t.Errorf("latest rain = %v, want the raw minute, not a bucket sum", tp.Latest["rain_mm"])
	}
	if tp.Latest["timestamp"] != now.Add(-time.Minute).Format(time.RFC3339) {
		t.Errorf("latest timestamp = %v", tp.Latest["timestamp"])
	}
	slp, ok := tp.Latest["sea_level_pressure_mb"].(float64)
	if !ok || absf(slp-1016.44) > 0.02 {
		t.Errorf("sea level = %v", tp.Latest["sea_level_pressure_mb"])
	}
	maxSeen := false
	for _, o := range tp.Obs {
		if m, ok := o["rain_max_mm"].(float64); ok && m == 0.1 {
			maxSeen = true
		}
	}
	if !maxSeen {
		t.Error("downsampled buckets lack rain_max_mm")
	}
}

func TestDownsampleTempest_DailyTotalAcrossMidnightKeepsDayTotal(t *testing.T) {
	from := time.Date(2026, 11, 20, 0, 0, 0, 0, time.UTC)
	obs := []tempest.Observation{
		{DeviceID: 1, Time: from, LocalDayRainMM: fp(12), RainMM: fp(0.2)},
		{DeviceID: 1, Time: from.Add(time.Minute), LocalDayRainMM: fp(12.4), RainMM: fp(0.4)},
		{DeviceID: 1, Time: from.Add(2 * time.Minute), LocalDayRainMM: fp(0.1), RainMM: fp(0.1)}, // local midnight reset
	}
	got := downsampleTempest(obs, from, 5*60*1000)
	if len(got) != 1 {
		t.Fatalf("buckets = %d", len(got))
	}
	if got[0].LocalDayRainMM == nil || *got[0].LocalDayRainMM != 12.4 {
		t.Errorf("daily total = %v, want the day's final 12.4", ptrVal(got[0].LocalDayRainMM))
	}
	if got[0].RainMaxMM == nil || *got[0].RainMaxMM != 0.4 {
		t.Errorf("rain max = %v, want 0.4", ptrVal(got[0].RainMaxMM))
	}
}

// Values from a real 9-minute bucket of light easterly wind: the served mean
// must be the speed-weighted vector mean, ~70.7° (ENE), not mirrored (~290°)
// or rotated. Pins the sin/cos/atan2 argument order and normalisation.
func TestVectorMeanDirection_EasterlyBucket(t *testing.T) {
	dirs := []float64{38, 59, 79, 72, 75, 93, 69, 65, 73}
	speeds := []float64{0.6, 0.6, 0.9, 0.6, 0.4, 0.6, 0.6, 0.6, 0.8}
	var recs []tempest.Observation
	for i := range dirs {
		recs = append(recs, tempest.Observation{WindDirectionDeg: fp(dirs[i]), WindAvgMS: fp(speeds[i])})
	}
	got := vectorMeanDirection(recs)
	if got == nil || *got < 69 || *got > 72 {
		t.Errorf("mean = %v, want ~70.7", ptrVal(got))
	}
	west := vectorMeanDirection([]tempest.Observation{{WindDirectionDeg: fp(260), WindAvgMS: fp(1)}, {WindDirectionDeg: fp(280), WindAvgMS: fp(1)}})
	if west == nil || absf(*west-270) > 1e-6 {
		t.Errorf("west mean = %v, want 270", ptrVal(west))
	}
}
