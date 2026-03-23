package store

import (
	"context"
	"os"
	"testing"
	"time"

	"github.com/hazel/airquality/logger"
	"github.com/hazel/airquality/openmeteo"
)

// openTestStore connects to the database using DATABASE_URL. Skips if not set.
func openTestStore(t *testing.T) *Store {
	t.Helper()
	url := os.Getenv("DATABASE_URL")
	if url == "" {
		t.Skip("DATABASE_URL not set")
	}
	s, err := New(context.Background(), url)
	if err != nil {
		t.Fatalf("connect: %v", err)
	}
	t.Cleanup(s.Close)
	return s
}

// TestQueryACStates_NullableColumns verifies that rows with NULL optional fields
// (mode, target_temp, temp_unit, fan_level, swing, horizontal_swing,
// changed_properties, reason) are scanned without error.
func TestQueryACStates_NullableColumns(t *testing.T) {
	s := openTestStore(t)
	ctx := context.Background()

	id := "test-null-cols-" + t.Name()
	ts := time.Now().UTC().Truncate(time.Second)

	// Insert a row with only the NOT NULL columns populated.
	_, err := s.pool.Exec(ctx, `
		INSERT INTO ac_states (timestamp, device_id, room_name, ac_on, sensibo_id)
		VALUES ($1, $2, $3, $4, $5)
		ON CONFLICT (sensibo_id) DO NOTHING`,
		ts, "test-device", "Test Room", false, id,
	)
	if err != nil {
		t.Fatalf("insert: %v", err)
	}
	t.Cleanup(func() {
		s.pool.Exec(ctx, `DELETE FROM ac_states WHERE sensibo_id = $1`, id)
	})

	from := ts.Add(-time.Second)
	to := ts.Add(time.Second)

	records, err := s.QueryACStates(ctx, from, to)
	if err != nil {
		t.Fatalf("QueryACStates returned error on NULL columns: %v", err)
	}

	var got *logger.ACStateRecord
	for i := range records {
		if records[i].SensiboID == id {
			got = &records[i]
			break
		}
	}
	if got == nil {
		t.Fatal("inserted record not found in QueryACStates result")
	}

	if got.DeviceID != "test-device" {
		t.Errorf("DeviceID = %q, want %q", got.DeviceID, "test-device")
	}
	if got.RoomName != "Test Room" {
		t.Errorf("RoomName = %q, want %q", got.RoomName, "Test Room")
	}
	if got.On {
		t.Error("On = true, want false")
	}
	// NULL columns should produce zero values, not a scan error.
	if got.Mode != "" {
		t.Errorf("Mode = %q, want empty string for NULL", got.Mode)
	}
	if got.TargetTemp != 0 {
		t.Errorf("TargetTemp = %d, want 0 for NULL", got.TargetTemp)
	}
}

// TestQueryACStates_AllColumns verifies that rows with all columns populated
// round-trip correctly.
func TestQueryACStates_AllColumns(t *testing.T) {
	s := openTestStore(t)
	ctx := context.Background()

	id := "test-all-cols-" + t.Name()
	ts := time.Now().UTC().Truncate(time.Second)

	rec := logger.ACStateRecord{
		Timestamp:         ts.Format(time.RFC3339),
		DeviceID:          "dev-abc",
		RoomName:          "Office",
		On:                true,
		Mode:              "cool",
		TargetTemp:        72,
		TempUnit:          "F",
		FanLevel:          "medium",
		Swing:             "fixedMiddle",
		HorizontalSwing:   "fixedCenter",
		ChangedProperties: "on;mode",
		Reason:            "UserRequest",
		SensiboID:         id,
	}

	if err := s.InsertACState(ctx, rec); err != nil {
		t.Fatalf("InsertACState: %v", err)
	}
	t.Cleanup(func() {
		s.pool.Exec(ctx, `DELETE FROM ac_states WHERE sensibo_id = $1`, id)
	})

	from := ts.Add(-time.Second)
	to := ts.Add(time.Second)

	records, err := s.QueryACStates(ctx, from, to)
	if err != nil {
		t.Fatalf("QueryACStates: %v", err)
	}

	var got *logger.ACStateRecord
	for i := range records {
		if records[i].SensiboID == id {
			got = &records[i]
			break
		}
	}
	if got == nil {
		t.Fatal("inserted record not found in QueryACStates result")
	}

	if got.Mode != rec.Mode {
		t.Errorf("Mode = %q, want %q", got.Mode, rec.Mode)
	}
	if got.TargetTemp != rec.TargetTemp {
		t.Errorf("TargetTemp = %d, want %d", got.TargetTemp, rec.TargetTemp)
	}
	if got.TempUnit != rec.TempUnit {
		t.Errorf("TempUnit = %q, want %q", got.TempUnit, rec.TempUnit)
	}
	if got.FanLevel != rec.FanLevel {
		t.Errorf("FanLevel = %q, want %q", got.FanLevel, rec.FanLevel)
	}
	if got.Reason != rec.Reason {
		t.Errorf("Reason = %q, want %q", got.Reason, rec.Reason)
	}
	if got.ChangedProperties != rec.ChangedProperties {
		t.Errorf("ChangedProperties = %q, want %q", got.ChangedProperties, rec.ChangedProperties)
	}
}

// TestQueryReadings_NullableColumns verifies that rows with NULL optional sensor
// fields (pressure, pm1, pm25, pm10, aqi, voc) scan without error.
func TestQueryReadings_NullableColumns(t *testing.T) {
	s := openTestStore(t)
	ctx := context.Background()

	ts := time.Now().UTC().Truncate(time.Second)

	rec := logger.Record{
		Timestamp: ts,
		Source:    "sensibo",
		SensorID:  "test-sensor-null",
		Location:  "Test Room",
		TempC:     21.5,
		Humidity:  55.0,
		DewPointC: 11.8,
		// Pressure, PM*, AQI, VOC all zero → stored as NULL
	}

	if err := s.InsertReading(ctx, rec); err != nil {
		t.Fatalf("InsertReading: %v", err)
	}
	t.Cleanup(func() {
		s.pool.Exec(ctx, `DELETE FROM readings WHERE sensor_id = $1 AND source = $2`, rec.SensorID, rec.Source)
	})

	from := ts.Add(-time.Second)
	to := ts.Add(time.Second)

	records, err := s.QueryReadings(ctx, from, to)
	if err != nil {
		t.Fatalf("QueryReadings returned error on NULL columns: %v", err)
	}

	var got *logger.Record
	for i := range records {
		if records[i].SensorID == rec.SensorID && records[i].Source == rec.Source {
			got = &records[i]
			break
		}
	}
	if got == nil {
		t.Fatal("inserted record not found in QueryReadings result")
	}

	if got.TempC != rec.TempC {
		t.Errorf("TempC = %v, want %v", got.TempC, rec.TempC)
	}
	if got.Pressure != 0 {
		t.Errorf("Pressure = %v, want 0 for NULL", got.Pressure)
	}
	if got.PM25 != 0 {
		t.Errorf("PM25 = %v, want 0 for NULL", got.PM25)
	}
}

// TestQueryWeather_RoundTrip verifies that a weather row survives an
// insert/query cycle including nullable sunrise/sunset.
func TestQueryWeather_RoundTrip(t *testing.T) {
	s := openTestStore(t)
	ctx := context.Background()

	ts := time.Now().UTC().Truncate(time.Minute)

	r := openmeteo.Reading{
		Time:            ts,
		Latitude:        37.83,
		Longitude:       -122.28,
		TempC:           18.5,
		Humidity:        60.0,
		DewPointC:       10.5,
		PrecipitationMM: 0.0,
		WeatherCode:     1,
		PressureHPa:     1013.0,
		WindSpeedKMH:    12.0,
		UVIndex:         3.0,
		// Sunrise/Sunset zero → stored as NULL
	}

	if err := s.InsertWeather(ctx, r); err != nil {
		t.Fatalf("InsertWeather: %v", err)
	}
	t.Cleanup(func() {
		s.pool.Exec(ctx, `DELETE FROM weather WHERE latitude = $1 AND longitude = $2 AND minute_epoch(timestamp) = minute_epoch($3)`,
			r.Latitude, r.Longitude, ts)
	})

	rows, err := s.QueryWeather(ctx, ts.Add(-time.Minute), ts.Add(time.Minute))
	if err != nil {
		t.Fatalf("QueryWeather: %v", err)
	}

	var got *openmeteo.Reading
	for i := range rows {
		if rows[i].Latitude == r.Latitude && rows[i].Longitude == r.Longitude {
			got = &rows[i]
			break
		}
	}
	if got == nil {
		t.Fatal("inserted weather row not found in QueryWeather result")
	}

	if got.TempC != r.TempC {
		t.Errorf("TempC = %v, want %v", got.TempC, r.TempC)
	}
	if !got.Sunrise.IsZero() {
		t.Errorf("Sunrise = %v, want zero for NULL", got.Sunrise)
	}
}
