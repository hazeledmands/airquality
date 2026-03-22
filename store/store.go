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
