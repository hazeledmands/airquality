package store

import (
	"context"
	"errors"
	"fmt"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgconn"

	"github.com/hazel/airquality/tempest"
)

func fp(v float64) *float64 { return &v }
func ip(v int) *int         { return &v }

func cleanupTempest(t *testing.T, s *Store, deviceID int) {
	t.Helper()
	ctx := context.Background()
	clean := func() {
		s.pool.Exec(ctx, `DELETE FROM tempest_observations WHERE device_id = $1`, deviceID)
		s.pool.Exec(ctx, `DELETE FROM tempest_events WHERE device_id = $1`, deviceID)
	}
	clean()
	t.Cleanup(clean)
}

func TestUpsertTempestObservations_RoundTripAndIdempotent(t *testing.T) {
	s := openTestStore(t)
	ctx := context.Background()
	const dev = 990001
	cleanupTempest(t, s, dev)

	ts := time.Date(2026, 11, 1, 12, 0, 0, 0, time.UTC)
	full := tempest.Observation{
		DeviceID: dev, Time: ts,
		WindLullMS: fp(0.2), WindAvgMS: fp(1.4), WindGustMS: fp(3.1), WindDirectionDeg: fp(270), WindSampleIntervalS: fp(3),
		StationPressureMB: fp(1011.1), AirTempC: fp(15.9), RelativeHumidity: fp(77),
		IlluminanceLux: fp(1610), UVIndex: fp(0.13), SolarRadiationWM2: fp(14),
		RainMM: fp(0), PrecipType: ip(0), LightningAvgDistanceKM: fp(12), LightningStrikeCount: ip(3),
		BatteryV: fp(2.61), ReportIntervalMin: ip(1), LocalDayRainMM: fp(0.25),
		RainFinalMM: nil, LocalDayRainFinalMM: nil, PrecipAnalysisType: ip(0),
	}
	sparse := tempest.Observation{DeviceID: dev, Time: ts.Add(time.Minute), AirTempC: fp(16)}

	batch := []tempest.Observation{full, sparse}
	if _, err := s.UpsertTempestObservations(ctx, batch); err != nil {
		t.Fatal(err)
	}
	// Re-running the same batch (a repeated backfill) changes nothing.
	n, err := s.UpsertTempestObservations(ctx, batch)
	if err != nil {
		t.Fatal(err)
	}
	if n != 0 {
		t.Errorf("second upsert changed %d rows, want 0", n)
	}

	got, err := s.QueryTempestObservations(ctx, ts.Add(-time.Second), ts.Add(time.Hour))
	if err != nil {
		t.Fatal(err)
	}
	var mine []tempest.Observation
	for _, o := range got {
		if o.DeviceID == dev {
			mine = append(mine, o)
		}
	}
	if len(mine) != 2 {
		t.Fatalf("rows = %d, want 2", len(mine))
	}
	g := mine[0]
	if !g.Time.Equal(ts) || *g.WindGustMS != 3.1 || *g.LightningStrikeCount != 3 || *g.PrecipType != 0 || *g.RainMM != 0 {
		t.Errorf("full row = %+v", g)
	}
	if g.RainFinalMM != nil {
		t.Errorf("NULL came back as %v", *g.RainFinalMM)
	}
	sp := mine[1]
	if *sp.AirTempC != 16 || sp.WindAvgMS != nil || sp.BatteryV != nil {
		t.Errorf("sparse row = %+v", sp)
	}
}

// WeatherFlow fills in Rain Check values a day later; refetching must update
// them, and a later fetch with nulls must not erase a known value.
func TestUpsertTempestObservations_UpdatesCorrectionsNeverErases(t *testing.T) {
	s := openTestStore(t)
	ctx := context.Background()
	const dev = 990002
	cleanupTempest(t, s, dev)

	ts := time.Date(2026, 11, 2, 0, 0, 0, 0, time.UTC)
	o := tempest.Observation{DeviceID: dev, Time: ts, RainMM: fp(0.5), LocalDayRainMM: fp(1)}
	if _, err := s.UpsertTempestObservations(ctx, []tempest.Observation{o}); err != nil {
		t.Fatal(err)
	}
	o.RainFinalMM = fp(0.6)
	o.LocalDayRainFinalMM = fp(1.2)
	n, err := s.UpsertTempestObservations(ctx, []tempest.Observation{o})
	if err != nil {
		t.Fatal(err)
	}
	if n != 1 {
		t.Errorf("correction changed %d rows, want 1", n)
	}
	if _, err := s.UpsertTempestObservations(ctx, []tempest.Observation{{DeviceID: dev, Time: ts}}); err != nil {
		t.Fatal(err)
	}
	got, err := s.QueryTempestObservations(ctx, ts, ts.Add(time.Second))
	if err != nil {
		t.Fatal(err)
	}
	for _, g := range got {
		if g.DeviceID != dev {
			continue
		}
		if g.RainFinalMM == nil || *g.RainFinalMM != 0.6 || g.RainMM == nil || *g.RainMM != 0.5 {
			t.Errorf("row = %+v", g)
		}
		return
	}
	t.Fatal("row not found")
}

func TestLatestTempestObservation(t *testing.T) {
	s := openTestStore(t)
	ctx := context.Background()
	const dev = 990003
	cleanupTempest(t, s, dev)

	if _, ok, err := s.LatestTempestObservation(ctx, dev); err != nil || ok {
		t.Fatalf("empty: ok=%v err=%v", ok, err)
	}
	ts := time.Date(2026, 11, 3, 0, 0, 0, 0, time.UTC)
	s.UpsertTempestObservations(ctx, []tempest.Observation{
		{DeviceID: dev, Time: ts}, {DeviceID: dev, Time: ts.Add(5 * time.Minute)},
	})
	got, ok, err := s.LatestTempestObservation(ctx, dev)
	if err != nil || !ok || !got.Equal(ts.Add(5*time.Minute)) {
		t.Errorf("latest = %v ok=%v err=%v", got, ok, err)
	}
}

func TestInsertTempestEvents_Dedup(t *testing.T) {
	s := openTestStore(t)
	ctx := context.Background()
	const dev = 990004
	cleanupTempest(t, s, dev)

	ts := time.Date(2026, 11, 4, 0, 0, 0, 0, time.UTC)
	evs := []tempest.Event{
		{DeviceID: dev, Time: ts, Type: tempest.EventLightningStrike, DistanceKM: fp(12), Energy: fp(500)},
		{DeviceID: dev, Time: ts, Type: tempest.EventRainStart},
	}
	for range 2 {
		if _, err := s.InsertTempestEvents(ctx, evs); err != nil {
			t.Fatal(err)
		}
	}
	got, err := s.QueryTempestEvents(ctx, ts.Add(-time.Second), ts.Add(time.Second))
	if err != nil {
		t.Fatal(err)
	}
	var mine []tempest.Event
	for _, e := range got {
		if e.DeviceID == dev {
			mine = append(mine, e)
		}
	}
	if len(mine) != 2 {
		t.Fatalf("events = %d, want 2", len(mine))
	}
	for _, e := range mine {
		switch e.Type {
		case tempest.EventLightningStrike:
			if e.DistanceKM == nil || *e.DistanceKM != 12 || *e.Energy != 500 {
				t.Errorf("strike = %+v", e)
			}
		case tempest.EventRainStart:
			if e.DistanceKM != nil {
				t.Errorf("rain start = %+v", e)
			}
		}
	}
}

// A row the database rejects must not take the rest of the batch with it.
func TestUpsertTempestObservations_BadRowDoesNotFailBatch(t *testing.T) {
	s := openTestStore(t)
	ctx := context.Background()
	const dev = 990005
	cleanupTempest(t, s, dev)
	ts := time.Date(2026, 11, 5, 0, 0, 0, 0, time.UTC)
	batch := []tempest.Observation{
		{DeviceID: dev, Time: ts, AirTempC: fp(1)},
		{DeviceID: dev, Time: ts.Add(time.Minute), PrecipType: ip(40000)}, // overflows smallint
		{DeviceID: dev, Time: ts.Add(2 * time.Minute), AirTempC: fp(3)},
	}
	n, err := s.UpsertTempestObservations(ctx, batch)
	if err != nil {
		t.Fatalf("err = %v, want the good rows stored and the bad one skipped", err)
	}
	if n != 2 {
		t.Errorf("changed = %d, want 2", n)
	}
	got, _ := s.QueryTempestObservations(ctx, ts, ts.Add(time.Hour))
	count := 0
	for _, o := range got {
		if o.DeviceID == dev {
			count++
		}
	}
	if count != 2 {
		t.Errorf("rows = %d, want 2", count)
	}
}

// Two strikes in the same second are two strikes; a replay of the same
// strike is not.
func TestInsertTempestEvents_SameSecondStrikes(t *testing.T) {
	s := openTestStore(t)
	ctx := context.Background()
	const dev = 990006
	cleanupTempest(t, s, dev)
	ts := time.Date(2026, 11, 6, 0, 0, 0, 0, time.UTC)
	a := tempest.Event{DeviceID: dev, Time: ts, Type: tempest.EventLightningStrike, DistanceKM: fp(5), Energy: fp(100)}
	b := tempest.Event{DeviceID: dev, Time: ts, Type: tempest.EventLightningStrike, DistanceKM: fp(9), Energy: fp(300)}
	inserted, err := s.InsertTempestEvents(ctx, []tempest.Event{a, b})
	if err != nil {
		t.Fatal(err)
	}
	if len(inserted) != 2 {
		t.Errorf("inserted = %d, want 2", len(inserted))
	}
	inserted, err = s.InsertTempestEvents(ctx, []tempest.Event{a, {DeviceID: dev, Time: ts, Type: tempest.EventRainStart}})
	if err != nil {
		t.Fatal(err)
	}
	if len(inserted) != 1 || inserted[0].Type != tempest.EventRainStart {
		t.Errorf("replay inserted = %+v, want only the rain start", inserted)
	}
	inserted, _ = s.InsertTempestEvents(ctx, []tempest.Event{{DeviceID: dev, Time: ts, Type: tempest.EventRainStart}})
	if len(inserted) != 0 {
		t.Errorf("rain start replay inserted %d", len(inserted))
	}
}

func TestTempestTablesExist(t *testing.T) {
	s := openTestStore(t)
	ok, err := s.TempestTablesExist(context.Background())
	if err != nil || !ok {
		t.Errorf("ok = %v err = %v, want true with the migration applied", ok, err)
	}
}

// Only data errors (bad values) fall back to row-by-row; a dead connection
// fails fast instead of being retried once per row.
func TestUpsertTempestObservations_ConnectionErrorFailsFast(t *testing.T) {
	s := openTestStore(t)
	broken, err := New(context.Background(), os.Getenv("DATABASE_URL"))
	if err != nil {
		t.Fatal(err)
	}
	broken.Close()
	obs := make([]tempest.Observation, 50)
	for i := range obs {
		obs[i] = tempest.Observation{DeviceID: 990007, Time: time.Date(2026, 11, 7, 0, i, 0, 0, time.UTC)}
	}
	_, err = broken.UpsertTempestObservations(context.Background(), obs)
	if err == nil {
		t.Fatal("want error on a closed pool")
	}
	if strings.Contains(err.Error(), "rows failed") {
		t.Errorf("err = %v: fell back to row-by-row on a connection error", err)
	}
	_ = s
}

func TestIsTempestDataError(t *testing.T) {
	if !isDataError(&pgconn.PgError{Code: "22003"}) || !isDataError(&pgconn.PgError{Code: "23502"}) {
		t.Error("class 22/23 must count as data errors")
	}
	if isDataError(&pgconn.PgError{Code: "57P01"}) || isDataError(errors.New("conn closed")) {
		t.Error("admin shutdown / plain errors are not data errors")
	}
	if !isDataError(fmt.Errorf("failed to encode args[14]: unable to encode 40000 into binary format for int2")) {
		t.Error("pgx encode errors are data errors")
	}
}
