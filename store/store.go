// Package store persists sensor readings and AC state changes to PostgreSQL.
package store

import (
	"context"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/hazel/airquality/logger"
	"github.com/hazel/airquality/openmeteo"
)

// Store writes records to a Postgres database.
type Store struct {
	pool *pgxpool.Pool
}

// New connects to the database at the given URL and returns a Store.
func New(ctx context.Context, databaseURL string) (*Store, error) {
	pool, err := pgxpool.New(ctx, databaseURL)
	if err != nil {
		return nil, fmt.Errorf("connect to database: %w", err)
	}
	if err := pool.Ping(ctx); err != nil {
		pool.Close()
		return nil, fmt.Errorf("ping database: %w", err)
	}
	return &Store{pool: pool}, nil
}

// Close releases the connection pool.
func (s *Store) Close() { s.pool.Close() }

// InsertReading writes a sensor reading. Silently skips if an identical
// source+sensor+minute already exists (ON CONFLICT DO NOTHING).
func (s *Store) InsertReading(ctx context.Context, r logger.Record) error {
	_, err := s.pool.Exec(ctx, `
		INSERT INTO readings
			(timestamp, source, sensor_id, location,
			 temp_c, humidity, dewpoint_c,
			 pressure, pm1, pm25, pm10, aqi, voc)
		VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13)
		ON CONFLICT (source, sensor_id, minute_epoch(timestamp)) DO NOTHING`,
		r.Timestamp, r.Source, r.SensorID, r.Location,
		r.TempC, r.Humidity, r.DewPointC,
		nullFloat(r.Pressure), nullFloat(r.PM1), nullFloat(r.PM25),
		nullFloat(r.PM10), nullFloat(r.AQI), nullFloat(r.VOC),
	)
	if err != nil {
		return fmt.Errorf("insert reading: %w", err)
	}
	return nil
}

// InsertACState writes an AC state change. Silently skips if the sensibo_id
// already exists (ON CONFLICT DO NOTHING).
func (s *Store) InsertACState(ctx context.Context, r logger.ACStateRecord) error {
	_, err := s.pool.Exec(ctx, `
		INSERT INTO ac_states
			(timestamp, device_id, room_name, ac_on,
			 mode, target_temp, temp_unit, fan_level,
			 swing, horizontal_swing, changed_properties, reason, sensibo_id)
		VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13)
		ON CONFLICT (sensibo_id) DO NOTHING`,
		r.Timestamp, r.DeviceID, r.RoomName, r.On,
		r.Mode, r.TargetTemp, r.TempUnit, r.FanLevel,
		r.Swing, r.HorizontalSwing, r.ChangedProperties, r.Reason, r.SensiboID,
	)
	if err != nil {
		return fmt.Errorf("insert ac state: %w", err)
	}
	return nil
}

// nullFloat returns nil for zero values so optional columns are stored as NULL.
func nullFloat(v float64) *float64 {
	if v == 0 {
		return nil
	}
	return &v
}

// InsertWeather writes a weather observation. Silently skips if an identical
// location+minute already exists (ON CONFLICT DO NOTHING).
func (s *Store) InsertWeather(ctx context.Context, r openmeteo.Reading) error {
	sunrise := (*time.Time)(nil)
	if !r.Sunrise.IsZero() {
		sunrise = &r.Sunrise
	}
	sunset := (*time.Time)(nil)
	if !r.Sunset.IsZero() {
		sunset = &r.Sunset
	}
	_, err := s.pool.Exec(ctx, `
		INSERT INTO weather
			(timestamp, latitude, longitude,
			 temp_c, humidity, dewpoint_c, precipitation_mm,
			 weather_code, pressure_hpa, wind_speed_kmh, uv_index,
			 sunrise, sunset)
		VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13)
		ON CONFLICT (latitude, longitude, minute_epoch(timestamp)) DO NOTHING`,
		r.Time, r.Latitude, r.Longitude,
		r.TempC, r.Humidity, r.DewPointC, r.PrecipitationMM,
		r.WeatherCode, r.PressureHPa, r.WindSpeedKMH, r.UVIndex,
		sunrise, sunset,
	)
	if err != nil {
		return fmt.Errorf("insert weather: %w", err)
	}
	return nil
}

// QueryWeather returns weather rows between from (inclusive) and to (exclusive),
// ordered ascending. Returns an empty slice (not an error) when the table is empty.
func (s *Store) QueryWeather(ctx context.Context, from, to time.Time) ([]openmeteo.Reading, error) {
	rows, err := s.pool.Query(ctx, `
		SELECT timestamp, latitude, longitude, temp_c, humidity, dewpoint_c,
		       precipitation_mm, weather_code, pressure_hpa,
		       wind_speed_kmh, uv_index, sunrise, sunset
		FROM weather
		WHERE timestamp >= $1 AND timestamp < $2
		ORDER BY timestamp ASC`,
		from, to,
	)
	if err != nil {
		return nil, fmt.Errorf("query weather: %w", err)
	}
	defer rows.Close()

	var result []openmeteo.Reading
	for rows.Next() {
		var r openmeteo.Reading
		var sunrise, sunset *time.Time
		if err := rows.Scan(
			&r.Time, &r.Latitude, &r.Longitude, &r.TempC, &r.Humidity, &r.DewPointC,
			&r.PrecipitationMM, &r.WeatherCode, &r.PressureHPa,
			&r.WindSpeedKMH, &r.UVIndex, &sunrise, &sunset,
		); err != nil {
			return nil, fmt.Errorf("scan weather row: %w", err)
		}
		if sunrise != nil {
			r.Sunrise = *sunrise
		}
		if sunset != nil {
			r.Sunset = *sunset
		}
		result = append(result, r)
	}
	return result, rows.Err()
}

// QueryReadings returns sensor readings between from (inclusive) and to (exclusive),
// ordered ascending. Returns an empty slice (not an error) when the table is empty.
func (s *Store) QueryReadings(ctx context.Context, from, to time.Time) ([]logger.Record, error) {
	rows, err := s.pool.Query(ctx, `
		SELECT timestamp, source, sensor_id, location,
		       temp_c, humidity, dewpoint_c,
		       pressure, pm1, pm25, pm10, aqi, voc
		FROM readings
		WHERE timestamp >= $1 AND timestamp < $2
		ORDER BY timestamp ASC`,
		from, to,
	)
	if err != nil {
		return nil, fmt.Errorf("query readings: %w", err)
	}
	defer rows.Close()

	var result []logger.Record
	for rows.Next() {
		var r logger.Record
		var pressure, pm1, pm25, pm10, aqi, voc *float64
		if err := rows.Scan(
			&r.Timestamp, &r.Source, &r.SensorID, &r.Location,
			&r.TempC, &r.Humidity, &r.DewPointC,
			&pressure, &pm1, &pm25, &pm10, &aqi, &voc,
		); err != nil {
			return nil, fmt.Errorf("scan reading row: %w", err)
		}
		if pressure != nil {
			r.Pressure = *pressure
		}
		if pm1 != nil {
			r.PM1 = *pm1
		}
		if pm25 != nil {
			r.PM25 = *pm25
		}
		if pm10 != nil {
			r.PM10 = *pm10
		}
		if aqi != nil {
			r.AQI = *aqi
		}
		if voc != nil {
			r.VOC = *voc
		}
		result = append(result, r)
	}
	return result, rows.Err()
}

// QueryACStates returns AC state changes between from (inclusive) and to (exclusive),
// ordered ascending. Returns an empty slice (not an error) when the table is empty.
func (s *Store) QueryACStates(ctx context.Context, from, to time.Time) ([]logger.ACStateRecord, error) {
	rows, err := s.pool.Query(ctx, `
		SELECT timestamp, device_id, room_name, ac_on,
		       mode, target_temp, temp_unit, fan_level,
		       swing, horizontal_swing, changed_properties, reason, sensibo_id
		FROM ac_states
		WHERE timestamp >= $1 AND timestamp < $2
		ORDER BY timestamp ASC`,
		from, to,
	)
	if err != nil {
		return nil, fmt.Errorf("query ac states: %w", err)
	}
	defer rows.Close()

	var result []logger.ACStateRecord
	for rows.Next() {
		var r logger.ACStateRecord
		var ts time.Time
		var mode, tempUnit, fanLevel, swing, hSwing, changedProps, reason *string
		var targetTemp *int
		if err := rows.Scan(
			&ts, &r.DeviceID, &r.RoomName, &r.On,
			&mode, &targetTemp, &tempUnit, &fanLevel,
			&swing, &hSwing, &changedProps, &reason, &r.SensiboID,
		); err != nil {
			return nil, fmt.Errorf("scan ac state row: %w", err)
		}
		r.Timestamp = ts.UTC().Format(time.RFC3339)
		if mode != nil {
			r.Mode = *mode
		}
		if targetTemp != nil {
			r.TargetTemp = *targetTemp
		}
		if tempUnit != nil {
			r.TempUnit = *tempUnit
		}
		if fanLevel != nil {
			r.FanLevel = *fanLevel
		}
		if swing != nil {
			r.Swing = *swing
		}
		if hSwing != nil {
			r.HorizontalSwing = *hSwing
		}
		if changedProps != nil {
			r.ChangedProperties = *changedProps
		}
		if reason != nil {
			r.Reason = *reason
		}
		result = append(result, r)
	}
	return result, rows.Err()
}

// Annotation is a user-authored note attached to a specific point in time.
// Room is empty for annotations that appear on all charts.
type Annotation struct {
	ID        int64
	Timestamp time.Time
	Note      string
	Room      string // empty = show on all charts; non-empty = show only on named room
	CreatedAt time.Time
}

// InsertAnnotation writes an annotation and returns the saved record with its ID.
// Pass an empty string for room to create a global annotation visible on all charts.
func (s *Store) InsertAnnotation(ctx context.Context, ts time.Time, note, room string) (Annotation, error) {
	var a Annotation
	var roomArg *string
	if room != "" {
		roomArg = &room
	}
	err := s.pool.QueryRow(ctx, `
		INSERT INTO annotations (timestamp, note, room)
		VALUES ($1, $2, $3)
		RETURNING id, timestamp, note, COALESCE(room, ''), created_at`,
		ts, note, roomArg,
	).Scan(&a.ID, &a.Timestamp, &a.Note, &a.Room, &a.CreatedAt)
	if err != nil {
		return Annotation{}, fmt.Errorf("insert annotation: %w", err)
	}
	return a, nil
}

// QueryAnnotations returns annotations between from (inclusive) and to (exclusive),
// ordered ascending.
func (s *Store) QueryAnnotations(ctx context.Context, from, to time.Time) ([]Annotation, error) {
	rows, err := s.pool.Query(ctx, `
		SELECT id, timestamp, note, COALESCE(room, ''), created_at
		FROM annotations
		WHERE timestamp >= $1 AND timestamp < $2
		ORDER BY timestamp ASC`,
		from, to,
	)
	if err != nil {
		return nil, fmt.Errorf("query annotations: %w", err)
	}
	defer rows.Close()
	var result []Annotation
	for rows.Next() {
		var a Annotation
		if err := rows.Scan(&a.ID, &a.Timestamp, &a.Note, &a.Room, &a.CreatedAt); err != nil {
			return nil, fmt.Errorf("scan annotation row: %w", err)
		}
		result = append(result, a)
	}
	return result, rows.Err()
}

// DeleteAnnotation removes the annotation with the given ID.
func (s *Store) DeleteAnnotation(ctx context.Context, id int64) error {
	_, err := s.pool.Exec(ctx, `DELETE FROM annotations WHERE id = $1`, id)
	if err != nil {
		return fmt.Errorf("delete annotation: %w", err)
	}
	return nil
}

// BatchInsertReadings inserts multiple readings in a single round-trip.
func (s *Store) BatchInsertReadings(ctx context.Context, records []logger.Record) error {
	if len(records) == 0 {
		return nil
	}
	rows := make([][]any, len(records))
	for i, r := range records {
		rows[i] = []any{
			r.Timestamp, r.Source, r.SensorID, r.Location,
			r.TempC, r.Humidity, r.DewPointC,
			nullFloat(r.Pressure), nullFloat(r.PM1), nullFloat(r.PM25),
			nullFloat(r.PM10), nullFloat(r.AQI), nullFloat(r.VOC),
		}
	}
	_, err := s.pool.CopyFrom(ctx,
		pgx.Identifier{"readings"},
		[]string{"timestamp", "source", "sensor_id", "location",
			"temp_c", "humidity", "dewpoint_c",
			"pressure", "pm1", "pm25", "pm10", "aqi", "voc"},
		pgx.CopyFromRows(rows),
	)
	if err != nil {
		return fmt.Errorf("batch insert readings: %w", err)
	}
	return nil
}
