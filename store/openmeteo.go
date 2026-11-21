package store

import (
	"context"
	"fmt"
	"log"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/hazel/airquality/openmeteo"
)

func (s *Store) detectOpenMeteoExtras(ctx context.Context) bool {
	var col bool
	var aq, fc *string
	err := s.pool.QueryRow(ctx, `
		SELECT EXISTS (SELECT 1 FROM information_schema.columns
		               WHERE table_name = 'weather' AND column_name = 'cloud_cover'),
		       to_regclass('outdoor_air_quality')::text,
		       to_regclass('weather_forecast')::text`).Scan(&col, &aq, &fc)
	return err == nil && col && aq != nil && fc != nil
}

// OpenMeteoExtras reports whether the Open-Meteo migration is applied:
// cloud cover, outdoor air quality and forecasts can be stored.
func (s *Store) OpenMeteoExtras() bool { return s.extras }

func timeOrNil(t time.Time) *time.Time {
	if t.IsZero() {
		return nil
	}
	return &t
}

func (s *Store) weatherInsert(r openmeteo.Reading) (string, []any) {
	cols := "timestamp, latitude, longitude, temp_c, humidity, dewpoint_c, precipitation_mm, weather_code, pressure_hpa, wind_speed_kmh, uv_index, sunrise, sunset"
	args := []any{r.Time, r.Latitude, r.Longitude, r.TempC, r.Humidity, r.DewPointC, r.PrecipitationMM,
		r.WeatherCode, r.PressureHPa, r.WindSpeedKMH, r.UVIndex, timeOrNil(r.Sunrise), timeOrNil(r.Sunset)}
	if s.extras {
		cols += ", cloud_cover"
		args = append(args, r.CloudCover)
	}
	ph := make([]string, len(args))
	for i := range ph {
		ph[i] = fmt.Sprintf("$%d", i+1)
	}
	return `INSERT INTO weather (` + cols + `) VALUES (` + strings.Join(ph, ",") + `)
		ON CONFLICT (latitude, longitude, minute_epoch(timestamp)) DO NOTHING`, args
}

// InsertWeather writes a weather observation. Silently skips if an identical
// location+minute already exists (ON CONFLICT DO NOTHING).
func (s *Store) InsertWeather(ctx context.Context, r openmeteo.Reading) error {
	sql, args := s.weatherInsert(r)
	if _, err := s.pool.Exec(ctx, sql, args...); err != nil {
		return fmt.Errorf("insert weather: %w", err)
	}
	return nil
}

// BatchInsertWeather writes readings, skipping ones already stored for the
// same location and minute, and returns how many were new. A row the database
// rejects is skipped with a log line rather than failing the batch.
func (s *Store) BatchInsertWeather(ctx context.Context, rows []openmeteo.Reading) (int64, error) {
	return s.batchExec(ctx, len(rows), func(i int) (string, []any) { return s.weatherInsert(rows[i]) },
		func(i int) string { return "weather at " + rows[i].Time.Format(time.RFC3339) })
}

// batchExec runs n statements in one round trip. If the database rejects the
// batch for a bad value, it retries one by one and skips only failing rows.
func (s *Store) batchExec(ctx context.Context, n int, stmt func(int) (string, []any), name func(int) string) (int64, error) {
	if n == 0 {
		return 0, nil
	}
	b := &pgx.Batch{}
	for i := 0; i < n; i++ {
		sql, args := stmt(i)
		b.Queue(sql, args...)
	}
	br := s.pool.SendBatch(ctx, b)
	var changed int64
	var batchErr error
	for i := 0; i < n; i++ {
		tag, err := br.Exec()
		if err != nil {
			batchErr = err
			break
		}
		changed += tag.RowsAffected()
	}
	closeErr := br.Close()
	if batchErr == nil && closeErr == nil {
		return changed, nil
	}
	if batchErr == nil {
		batchErr = closeErr
	}
	if ctx.Err() != nil || !isDataError(batchErr) {
		return 0, batchErr
	}
	ctx, cancel := context.WithTimeout(ctx, fallbackTimeout)
	defer cancel()
	changed = 0
	for i := 0; i < n; i++ {
		sql, args := stmt(i)
		tag, err := s.pool.Exec(ctx, sql, args...)
		if err != nil {
			if !isDataError(err) {
				return changed, err
			}
			log.Printf("%s: row skipped: %v", name(i), err)
			continue
		}
		changed += tag.RowsAffected()
	}
	return changed, nil
}

// QueryWeather returns weather rows between from (inclusive) and to
// (exclusive), ordered ascending.
func (s *Store) QueryWeather(ctx context.Context, from, to time.Time) ([]openmeteo.Reading, error) {
	cols := `timestamp, latitude, longitude, temp_c, humidity, dewpoint_c,
		       precipitation_mm, weather_code, pressure_hpa,
		       wind_speed_kmh, uv_index, sunrise, sunset`
	if s.extras {
		cols += ", cloud_cover"
	}
	rows, err := s.pool.Query(ctx, `SELECT `+cols+` FROM weather
		WHERE timestamp >= $1 AND timestamp < $2 ORDER BY timestamp ASC`, from, to)
	if err != nil {
		return nil, fmt.Errorf("query weather: %w", err)
	}
	defer rows.Close()
	var result []openmeteo.Reading
	for rows.Next() {
		var r openmeteo.Reading
		var sunrise, sunset *time.Time
		targets := []any{&r.Time, &r.Latitude, &r.Longitude, &r.TempC, &r.Humidity, &r.DewPointC,
			&r.PrecipitationMM, &r.WeatherCode, &r.PressureHPa, &r.WindSpeedKMH, &r.UVIndex, &sunrise, &sunset}
		if s.extras {
			targets = append(targets, &r.CloudCover)
		}
		if err := rows.Scan(targets...); err != nil {
			return nil, fmt.Errorf("scan weather row: %w", err)
		}
		r.Time = r.Time.UTC()
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

// --- outdoor air quality ---

var aqCols = []string{
	"pm2_5", "pm10", "ozone", "nitrogen_dioxide", "sulphur_dioxide", "carbon_monoxide",
	"us_aqi", "european_aqi", "dust",
	"alder_pollen", "birch_pollen", "grass_pollen", "mugwort_pollen", "olive_pollen", "ragweed_pollen",
}

func aqValues(a openmeteo.AirQuality) []any {
	return []any{a.PM25, a.PM10, a.Ozone, a.NO2, a.SO2, a.CO, a.USAQI, a.EuropeanAQI, a.Dust,
		a.AlderPollen, a.BirchPollen, a.GrassPollen, a.MugwortPollen, a.OlivePollen, a.RagweedPollen}
}

func aqTargets(a *openmeteo.AirQuality) []any {
	return []any{&a.PM25, &a.PM10, &a.Ozone, &a.NO2, &a.SO2, &a.CO, &a.USAQI, &a.EuropeanAQI, &a.Dust,
		&a.AlderPollen, &a.BirchPollen, &a.GrassPollen, &a.MugwortPollen, &a.OlivePollen, &a.RagweedPollen}
}

// mergeUpsertSQL inserts a row or, on key conflict, lets non-null incoming
// values replace stored ones; a NULL never erases a value, and rows that
// would not change are not rewritten.
func mergeUpsertSQL(table string, keys, cols []string) string {
	all := append(append([]string{}, keys...), cols...)
	ph := make([]string, len(all))
	for i := range ph {
		ph[i] = fmt.Sprintf("$%d", i+1)
	}
	sets, olds, news := make([]string, len(cols)), make([]string, len(cols)), make([]string, len(cols))
	for i, c := range cols {
		merged := fmt.Sprintf("COALESCE(EXCLUDED.%s, t.%s)", c, c)
		sets[i], olds[i], news[i] = c+" = "+merged, "t."+c, merged
	}
	return `INSERT INTO ` + table + ` AS t (` + strings.Join(all, ", ") + `) VALUES (` + strings.Join(ph, ", ") + `)
		ON CONFLICT (` + strings.Join(keys, ", ") + `) DO UPDATE SET ` + strings.Join(sets, ", ") + `
		WHERE (` + strings.Join(olds, ", ") + `) IS DISTINCT FROM (` + strings.Join(news, ", ") + `)`
}

var upsertAQSQL = mergeUpsertSQL("outdoor_air_quality", []string{"latitude", "longitude", "timestamp"}, aqCols)

// UpsertAirQuality writes samples (idempotent; later non-null values win)
// and returns how many rows were inserted or changed.
func (s *Store) UpsertAirQuality(ctx context.Context, rows []openmeteo.AirQuality) (int64, error) {
	return s.batchExec(ctx, len(rows), func(i int) (string, []any) {
		a := rows[i]
		return upsertAQSQL, append([]any{a.Latitude, a.Longitude, a.Time}, aqValues(a)...)
	}, func(i int) string { return "air quality at " + rows[i].Time.Format(time.RFC3339) })
}

// QueryAirQuality returns samples in [from, to), oldest first.
func (s *Store) QueryAirQuality(ctx context.Context, from, to time.Time) ([]openmeteo.AirQuality, error) {
	rows, err := s.pool.Query(ctx, `SELECT latitude, longitude, timestamp, `+strings.Join(aqCols, ", ")+`
		FROM outdoor_air_quality WHERE timestamp >= $1 AND timestamp < $2 ORDER BY timestamp ASC`, from, to)
	if err != nil {
		return nil, fmt.Errorf("query air quality: %w", err)
	}
	defer rows.Close()
	var out []openmeteo.AirQuality
	for rows.Next() {
		var a openmeteo.AirQuality
		if err := rows.Scan(append([]any{&a.Latitude, &a.Longitude, &a.Time}, aqTargets(&a)...)...); err != nil {
			return nil, fmt.Errorf("scan air quality: %w", err)
		}
		a.Time = a.Time.UTC()
		out = append(out, a)
	}
	return out, rows.Err()
}

// --- forecast ---

// ForecastRow is a stored forecast hour: the latest forecast for that hour.
type ForecastRow struct {
	openmeteo.ForecastHour
	Latitude  float64
	Longitude float64
	IssuedAt  time.Time
}

// UpsertForecast stores a forecast issued at issuedAt; for each hour it
// replaces whatever an earlier forecast said.
func (s *Store) UpsertForecast(ctx context.Context, lat, lon float64, issuedAt time.Time, hours []openmeteo.ForecastHour) (int64, error) {
	return s.batchExec(ctx, len(hours), func(i int) (string, []any) {
		h := hours[i]
		return `INSERT INTO weather_forecast AS t (latitude, longitude, valid_time, issued_at, temp_c,
				precip_probability, precip_mm, cloud_cover, uv_index, wind_speed_kmh, weather_code)
			VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11)
			ON CONFLICT (latitude, longitude, valid_time) DO UPDATE SET
				issued_at = EXCLUDED.issued_at, temp_c = EXCLUDED.temp_c,
				precip_probability = EXCLUDED.precip_probability, precip_mm = EXCLUDED.precip_mm,
				cloud_cover = EXCLUDED.cloud_cover, uv_index = EXCLUDED.uv_index,
				wind_speed_kmh = EXCLUDED.wind_speed_kmh, weather_code = EXCLUDED.weather_code
			WHERE EXCLUDED.issued_at >= t.issued_at`,
			[]any{lat, lon, h.Time, issuedAt, h.TempC, h.PrecipProbability, h.PrecipMM, h.CloudCover, h.UVIndex, h.WindSpeedKMH, h.WeatherCode}
	}, func(i int) string { return "forecast for " + hours[i].Time.Format(time.RFC3339) })
}

// QueryForecast returns stored forecast hours with valid_time in [from, to).
func (s *Store) QueryForecast(ctx context.Context, from, to time.Time) ([]ForecastRow, error) {
	rows, err := s.pool.Query(ctx, `SELECT latitude, longitude, valid_time, issued_at, temp_c,
			precip_probability, precip_mm, cloud_cover, uv_index, wind_speed_kmh, weather_code
		FROM weather_forecast WHERE valid_time >= $1 AND valid_time < $2 ORDER BY valid_time ASC`, from, to)
	if err != nil {
		return nil, fmt.Errorf("query forecast: %w", err)
	}
	defer rows.Close()
	var out []ForecastRow
	for rows.Next() {
		var f ForecastRow
		if err := rows.Scan(&f.Latitude, &f.Longitude, &f.Time, &f.IssuedAt, &f.TempC, &f.PrecipProbability,
			&f.PrecipMM, &f.CloudCover, &f.UVIndex, &f.WindSpeedKMH, &f.WeatherCode); err != nil {
			return nil, fmt.Errorf("scan forecast: %w", err)
		}
		f.Time, f.IssuedAt = f.Time.UTC(), f.IssuedAt.UTC()
		out = append(out, f)
	}
	return out, rows.Err()
}
