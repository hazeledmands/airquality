package store

import (
	"context"
	"errors"
	"fmt"
	"log"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"

	"github.com/hazel/airquality/tempest"
)

// tempestValueCols are the tempest_observations value columns, in the order
// tempestValues returns them.
var tempestValueCols = []string{
	"wind_lull_ms", "wind_avg_ms", "wind_gust_ms", "wind_direction_deg", "wind_sample_interval_s",
	"station_pressure_mb", "air_temp_c", "relative_humidity",
	"illuminance_lux", "uv_index", "solar_radiation_wm2",
	"rain_mm", "precip_type", "lightning_avg_distance_km", "lightning_strike_count",
	"battery_v", "report_interval_min",
	"local_day_rain_mm", "rain_final_mm", "local_day_rain_final_mm", "precip_analysis_type",
}

func tempestValues(o tempest.Observation) []any {
	return []any{
		o.WindLullMS, o.WindAvgMS, o.WindGustMS, o.WindDirectionDeg, o.WindSampleIntervalS,
		o.StationPressureMB, o.AirTempC, o.RelativeHumidity,
		o.IlluminanceLux, o.UVIndex, o.SolarRadiationWM2,
		o.RainMM, o.PrecipType, o.LightningAvgDistanceKM, o.LightningStrikeCount,
		o.BatteryV, o.ReportIntervalMin,
		o.LocalDayRainMM, o.RainFinalMM, o.LocalDayRainFinalMM, o.PrecipAnalysisType,
	}
}

func tempestScanTargets(o *tempest.Observation) []any {
	return []any{
		&o.WindLullMS, &o.WindAvgMS, &o.WindGustMS, &o.WindDirectionDeg, &o.WindSampleIntervalS,
		&o.StationPressureMB, &o.AirTempC, &o.RelativeHumidity,
		&o.IlluminanceLux, &o.UVIndex, &o.SolarRadiationWM2,
		&o.RainMM, &o.PrecipType, &o.LightningAvgDistanceKM, &o.LightningStrikeCount,
		&o.BatteryV, &o.ReportIntervalMin,
		&o.LocalDayRainMM, &o.RainFinalMM, &o.LocalDayRainFinalMM, &o.PrecipAnalysisType,
	}
}

// upsertTempestSQL inserts a row, or on (device_id, timestamp) conflict
// merges it: a non-NULL incoming value replaces the stored one (Rain Check
// corrections arrive a day later), a NULL never erases a stored value, and
// rows that would not change are not rewritten.
var upsertTempestSQL = func() string {
	n := len(tempestValueCols)
	placeholders := make([]string, n)
	sets := make([]string, n)
	olds := make([]string, n)
	news := make([]string, n)
	for i, c := range tempestValueCols {
		placeholders[i] = fmt.Sprintf("$%d", i+3)
		merged := fmt.Sprintf("COALESCE(EXCLUDED.%s, t.%s)", c, c)
		sets[i] = c + " = " + merged
		olds[i] = "t." + c
		news[i] = merged
	}
	return `INSERT INTO tempest_observations AS t (device_id, timestamp, ` + strings.Join(tempestValueCols, ", ") + `)
		VALUES ($1, $2, ` + strings.Join(placeholders, ", ") + `)
		ON CONFLICT (device_id, timestamp) DO UPDATE SET ` + strings.Join(sets, ", ") + `
		WHERE (` + strings.Join(olds, ", ") + `) IS DISTINCT FROM (` + strings.Join(news, ", ") + `)`
}()

// fallbackTimeout bounds the row-by-row retry of one batch.
const fallbackTimeout = 30 * time.Second

// isDataError reports whether err means a value was bad, not that the
// database was unreachable: SQLSTATE class 22 (data exception) or 23
// (integrity constraint), or a value pgx could not encode.
func isDataError(err error) bool {
	var pgErr *pgconn.PgError
	if errors.As(err, &pgErr) {
		return strings.HasPrefix(pgErr.Code, "22") || strings.HasPrefix(pgErr.Code, "23")
	}
	// pgx reports client-side encode failures only as text.
	return strings.Contains(err.Error(), "failed to encode args")
}

// UpsertTempestObservations writes observations and returns how many rows
// were inserted or changed. Safe to repeat. It tries one round trip; if the
// database rejects the batch for a bad value (which rolls it all back), it
// retries row by row within fallbackTimeout and skips, with a log line, only
// the rows that fail, so one bad row can't stall ingestion. Connection and
// other errors return immediately. It also errors if no row could be written.
func (s *Store) UpsertTempestObservations(ctx context.Context, obs []tempest.Observation) (int64, error) {
	if len(obs) == 0 {
		return 0, nil
	}
	changed, err := s.upsertTempestBatch(ctx, obs)
	if err == nil {
		return changed, nil
	}
	if ctx.Err() != nil || !isDataError(err) {
		return 0, err
	}
	ctx, cancel := context.WithTimeout(ctx, fallbackTimeout)
	defer cancel()
	changed = 0
	failed := 0
	var lastErr error
	for _, o := range obs {
		args := append([]any{o.DeviceID, o.Time}, tempestValues(o)...)
		tag, err := s.pool.Exec(ctx, upsertTempestSQL, args...)
		if err != nil {
			if !isDataError(err) {
				return changed, fmt.Errorf("upsert tempest observations: %w", err)
			}
			failed++
			lastErr = err
			log.Printf("tempest device %d at %s: row skipped: %v", o.DeviceID, o.Time.Format(time.RFC3339), err)
			continue
		}
		changed += tag.RowsAffected()
	}
	if failed == len(obs) {
		return 0, fmt.Errorf("upsert tempest observations: all %d rows failed: %w", failed, lastErr)
	}
	return changed, nil
}

func (s *Store) upsertTempestBatch(ctx context.Context, obs []tempest.Observation) (int64, error) {
	b := &pgx.Batch{}
	for _, o := range obs {
		args := append([]any{o.DeviceID, o.Time}, tempestValues(o)...)
		b.Queue(upsertTempestSQL, args...)
	}
	br := s.pool.SendBatch(ctx, b)
	defer br.Close()
	var changed int64
	for range obs {
		tag, err := br.Exec()
		if err != nil {
			return 0, fmt.Errorf("upsert tempest observation: %w", err)
		}
		changed += tag.RowsAffected()
	}
	return changed, br.Close()
}

// TempestTablesExist reports whether the Tempest migration has been applied.
func (s *Store) TempestTablesExist(ctx context.Context) (bool, error) {
	var obs, evs *string
	err := s.pool.QueryRow(ctx,
		`SELECT to_regclass('tempest_observations')::text, to_regclass('tempest_events')::text`).Scan(&obs, &evs)
	if err != nil {
		return false, fmt.Errorf("check tempest tables: %w", err)
	}
	return obs != nil && evs != nil, nil
}

// QueryTempestObservations returns observations in [from, to), oldest first.
func (s *Store) QueryTempestObservations(ctx context.Context, from, to time.Time) ([]tempest.Observation, error) {
	rows, err := s.pool.Query(ctx, `
		SELECT device_id, timestamp, `+strings.Join(tempestValueCols, ", ")+`
		FROM tempest_observations
		WHERE timestamp >= $1 AND timestamp < $2
		ORDER BY timestamp ASC, device_id ASC`, from, to)
	if err != nil {
		return nil, fmt.Errorf("query tempest observations: %w", err)
	}
	defer rows.Close()
	var out []tempest.Observation
	for rows.Next() {
		var o tempest.Observation
		targets := append([]any{&o.DeviceID, &o.Time}, tempestScanTargets(&o)...)
		if err := rows.Scan(targets...); err != nil {
			return nil, fmt.Errorf("scan tempest observation: %w", err)
		}
		o.Time = o.Time.UTC()
		out = append(out, o)
	}
	return out, rows.Err()
}

// LatestTempestObservation returns the newest stored timestamp for a device.
func (s *Store) LatestTempestObservation(ctx context.Context, deviceID int) (time.Time, bool, error) {
	var ts *time.Time
	err := s.pool.QueryRow(ctx,
		`SELECT max(timestamp) FROM tempest_observations WHERE device_id = $1`, deviceID).Scan(&ts)
	if err != nil && !errors.Is(err, pgx.ErrNoRows) {
		return time.Time{}, false, fmt.Errorf("latest tempest observation: %w", err)
	}
	if ts == nil {
		return time.Time{}, false, nil
	}
	return ts.UTC(), true, nil
}

// InsertTempestEvents writes events and returns the ones that were new;
// replays of stored events are skipped.
func (s *Store) InsertTempestEvents(ctx context.Context, evs []tempest.Event) ([]tempest.Event, error) {
	var inserted []tempest.Event
	for _, e := range evs {
		tag, err := s.pool.Exec(ctx, `INSERT INTO tempest_events (device_id, timestamp, type, distance_km, energy)
			VALUES ($1, $2, $3, $4, $5)
			ON CONFLICT (device_id, type, timestamp, (COALESCE(distance_km, -1)), (COALESCE(energy, -1))) DO NOTHING`,
			e.DeviceID, e.Time, e.Type, e.DistanceKM, e.Energy)
		if err != nil {
			return inserted, fmt.Errorf("insert tempest event: %w", err)
		}
		if tag.RowsAffected() > 0 {
			inserted = append(inserted, e)
		}
	}
	return inserted, nil
}

// QueryTempestEvents returns events in [from, to), oldest first.
func (s *Store) QueryTempestEvents(ctx context.Context, from, to time.Time) ([]tempest.Event, error) {
	rows, err := s.pool.Query(ctx, `
		SELECT device_id, timestamp, type, distance_km, energy
		FROM tempest_events
		WHERE timestamp >= $1 AND timestamp < $2
		ORDER BY timestamp ASC`, from, to)
	if err != nil {
		return nil, fmt.Errorf("query tempest events: %w", err)
	}
	defer rows.Close()
	var out []tempest.Event
	for rows.Next() {
		var e tempest.Event
		if err := rows.Scan(&e.DeviceID, &e.Time, &e.Type, &e.DistanceKM, &e.Energy); err != nil {
			return nil, fmt.Errorf("scan tempest event: %w", err)
		}
		e.Time = e.Time.UTC()
		out = append(out, e)
	}
	return out, rows.Err()
}
