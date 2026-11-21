package store

import (
	"context"
	"testing"
	"time"

	"github.com/hazel/airquality/openmeteo"
)

// Each test uses its own fake location so rows never collide.
func cleanupLocation(t *testing.T, s *Store, lat, lon float64) {
	t.Helper()
	ctx := context.Background()
	clean := func() {
		for _, table := range []string{"weather", "outdoor_air_quality", "weather_forecast"} {
			s.pool.Exec(ctx, `DELETE FROM `+table+` WHERE latitude = $1 AND longitude = $2`, lat, lon)
		}
	}
	clean()
	t.Cleanup(clean)
}

func TestOpenMeteoSchemaDetected(t *testing.T) {
	s := openTestStore(t)
	if !s.OpenMeteoExtras() {
		t.Error("OpenMeteoExtras = false with the migration applied")
	}
}

func TestWeather_CloudCoverRoundTripAndBatchIdempotent(t *testing.T) {
	s := openTestStore(t)
	ctx := context.Background()
	lat, lon := 1.001, -1.001
	cleanupLocation(t, s, lat, lon)
	ts := time.Date(2026, 4, 17, 10, 0, 0, 0, time.UTC)
	rows := []openmeteo.Reading{
		{Time: ts, Latitude: lat, Longitude: lon, TempC: 12, CloudCover: fp(80), Sunrise: ts.Add(-4 * time.Hour), Sunset: ts.Add(8 * time.Hour)},
		{Time: ts.Add(time.Hour), Latitude: lat, Longitude: lon, TempC: 13},
	}
	n, err := s.BatchInsertWeather(ctx, rows)
	if err != nil || n != 2 {
		t.Fatalf("first insert: n=%d err=%v", n, err)
	}
	n, err = s.BatchInsertWeather(ctx, rows)
	if err != nil || n != 0 {
		t.Fatalf("re-insert: n=%d err=%v, want 0 (idempotent)", n, err)
	}
	got, err := s.QueryWeather(ctx, ts.Add(-time.Minute), ts.Add(2*time.Hour))
	if err != nil {
		t.Fatal(err)
	}
	var mine []openmeteo.Reading
	for _, r := range got {
		if r.Latitude == lat && r.Longitude == lon {
			mine = append(mine, r)
		}
	}
	if len(mine) != 2 || mine[0].CloudCover == nil || *mine[0].CloudCover != 80 || mine[1].CloudCover != nil {
		t.Errorf("rows = %+v", mine)
	}
}

// Before the migration is applied, weather capture keeps working without
// the new column.
func TestWeather_LegacySchemaModeStillInserts(t *testing.T) {
	s := openTestStore(t)
	ctx := context.Background()
	lat, lon := 1.002, -1.002
	cleanupLocation(t, s, lat, lon)
	s.extras = false
	t.Cleanup(func() { s.extras = true })
	ts := time.Date(2026, 4, 18, 10, 0, 0, 0, time.UTC)
	if err := s.InsertWeather(ctx, openmeteo.Reading{Time: ts, Latitude: lat, Longitude: lon, TempC: 5, CloudCover: fp(50)}); err != nil {
		t.Fatal(err)
	}
	got, err := s.QueryWeather(ctx, ts, ts.Add(time.Minute))
	if err != nil {
		t.Fatal(err)
	}
	found := false
	for _, r := range got {
		if r.Latitude == lat {
			found = true
		}
	}
	if !found {
		t.Error("row not stored in legacy mode")
	}
}

func TestAirQuality_UpsertMergesAndIsIdempotent(t *testing.T) {
	s := openTestStore(t)
	ctx := context.Background()
	lat, lon := 1.003, -1.003
	cleanupLocation(t, s, lat, lon)
	ts := time.Date(2026, 11, 21, 20, 0, 0, 0, time.UTC)
	aq := openmeteo.AirQuality{Time: ts, Latitude: lat, Longitude: lon, PM25: fp(24), USAQI: fp(86), Ozone: fp(13)}
	if n, err := s.UpsertAirQuality(ctx, []openmeteo.AirQuality{aq}); err != nil || n != 1 {
		t.Fatalf("n=%d err=%v", n, err)
	}
	if n, _ := s.UpsertAirQuality(ctx, []openmeteo.AirQuality{aq}); n != 0 {
		t.Errorf("repeat changed %d rows", n)
	}
	// A later sample with a null never erases a stored value.
	if _, err := s.UpsertAirQuality(ctx, []openmeteo.AirQuality{{Time: ts, Latitude: lat, Longitude: lon, PM25: fp(25)}}); err != nil {
		t.Fatal(err)
	}
	got, err := s.QueryAirQuality(ctx, ts, ts.Add(time.Minute))
	if err != nil {
		t.Fatal(err)
	}
	for _, g := range got {
		if g.Latitude != lat {
			continue
		}
		if *g.PM25 != 25 || g.USAQI == nil || *g.USAQI != 86 || g.GrassPollen != nil {
			t.Errorf("row = %+v", g)
		}
		return
	}
	t.Fatal("row not found")
}

func TestForecast_LatestIssueWins(t *testing.T) {
	s := openTestStore(t)
	ctx := context.Background()
	lat, lon := 1.004, -1.004
	cleanupLocation(t, s, lat, lon)
	valid := time.Date(2026, 11, 22, 12, 0, 0, 0, time.UTC)
	issued := valid.Add(-24 * time.Hour)
	hour := func(temp, prob float64) []openmeteo.ForecastHour {
		return []openmeteo.ForecastHour{{Time: valid, TempC: fp(temp), PrecipProbability: fp(prob)}}
	}
	if _, err := s.UpsertForecast(ctx, lat, lon, issued, hour(10, 20)); err != nil {
		t.Fatal(err)
	}
	if _, err := s.UpsertForecast(ctx, lat, lon, issued.Add(time.Hour), hour(11, 60)); err != nil {
		t.Fatal(err)
	}
	got, err := s.QueryForecast(ctx, valid.Add(-time.Hour), valid.Add(time.Hour))
	if err != nil {
		t.Fatal(err)
	}
	for _, g := range got {
		if g.Latitude != lat {
			continue
		}
		if *g.TempC != 11 || *g.PrecipProbability != 60 || !g.IssuedAt.Equal(issued.Add(time.Hour)) {
			t.Errorf("row = %+v", g)
		}
		return
	}
	t.Fatal("row not found")
}
