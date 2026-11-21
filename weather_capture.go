package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"log"
	"os"
	"time"

	"github.com/hazel/airquality/config"
	"github.com/hazel/airquality/metrics"
	"github.com/hazel/airquality/openmeteo"
	"github.com/hazel/airquality/store"
	"github.com/hazel/airquality/tempest"
)

// Open-Meteo is polled at the cadence its data changes, independent of
// --interval: current weather is a 15-minute slot, air quality and the
// forecast are hourly. About 150 calls a day, far inside the free tier's
// 10,000 (open-meteo.com/en/terms).
const (
	weatherEvery         = 15 * time.Minute
	airQualityEvery      = time.Hour
	forecastEvery        = time.Hour
	weatherRetryAfterErr = 5 * time.Minute
	// weatherBackfillWindow keeps each historical request at two weeks,
	// which Open-Meteo counts as a single call.
	weatherBackfillWindow = 14 * 24 * time.Hour
)

type weatherAPI interface {
	Fetch(ctx context.Context) (openmeteo.Reading, error)
	AirQuality(ctx context.Context) (openmeteo.AirQuality, error)
	Forecast(ctx context.Context) ([]openmeteo.ForecastHour, error)
	History(ctx context.Context, start, end time.Time) ([]openmeteo.Reading, error)
	AirQualityHistory(ctx context.Context, start, end time.Time) ([]openmeteo.AirQuality, error)
}

type weatherStore interface {
	OpenMeteoExtras() bool
	InsertWeather(ctx context.Context, r openmeteo.Reading) error
	BatchInsertWeather(ctx context.Context, rows []openmeteo.Reading) (int64, error)
	UpsertAirQuality(ctx context.Context, rows []openmeteo.AirQuality) (int64, error)
	UpsertForecast(ctx context.Context, lat, lon float64, issuedAt time.Time, hours []openmeteo.ForecastHour) (int64, error)
}

// weatherRecorder polls Open-Meteo: current weather, and, once the
// migration is applied, outdoor air quality and the forecast.
type weatherRecorder struct {
	api      weatherAPI
	db       weatherStore
	cache    *dataCache
	m        *metrics.Metrics // nil in tests
	now      func() time.Time
	lat, lon float64
	verbose  bool

	nextWeather, nextAQ, nextForecast time.Time
}

// Poll fetches whatever is due. A failed fetch is retried after
// weatherRetryAfterErr rather than at the next poll.
func (r *weatherRecorder) Poll(ctx context.Context) {
	now := r.now()
	if !now.Before(r.nextWeather) {
		r.nextWeather = now.Add(weatherEvery)
		if err := r.pollWeather(ctx); err != nil {
			log.Printf("Weather error: %v", err)
			r.nextWeather = now.Add(weatherRetryAfterErr)
		}
	}
	if !r.db.OpenMeteoExtras() {
		return
	}
	if !now.Before(r.nextAQ) {
		r.nextAQ = now.Add(airQualityEvery)
		if err := r.pollAirQuality(ctx); err != nil {
			log.Printf("Air quality error: %v", err)
			r.nextAQ = now.Add(weatherRetryAfterErr)
		}
	}
	if !now.Before(r.nextForecast) {
		r.nextForecast = now.Add(forecastEvery)
		if err := r.pollForecast(ctx, now); err != nil {
			log.Printf("Forecast error: %v", err)
			r.nextForecast = now.Add(weatherRetryAfterErr)
		}
	}
}

func (r *weatherRecorder) pollWeather(ctx context.Context) error {
	reading, err := r.api.Fetch(ctx)
	if err != nil {
		return err
	}
	if err := r.db.InsertWeather(ctx, reading); err != nil {
		log.Printf("DB weather write error: %v", err)
	}
	r.cache.addWeather(reading)
	if r.m != nil {
		r.m.UpdateWeather(reading)
	}
	if r.verbose {
		log.Printf("  weather: %.1f°C, %.0f%%, dew point %.1f°C, %.1f hPa, wind %.1f km/h, UV %.1f, precip %.1f mm, cloud %v",
			reading.TempC, reading.Humidity, reading.DewPointC, reading.PressureHPa, reading.WindSpeedKMH,
			reading.UVIndex, reading.PrecipitationMM, orDash(reading.CloudCover))
	}
	return nil
}

func (r *weatherRecorder) pollAirQuality(ctx context.Context) error {
	aq, err := r.api.AirQuality(ctx)
	if err != nil {
		return err
	}
	if _, err := r.db.UpsertAirQuality(ctx, []openmeteo.AirQuality{aq}); err != nil {
		log.Printf("DB air quality write error: %v", err)
	}
	r.cache.addAirQuality([]openmeteo.AirQuality{aq})
	if r.m != nil {
		r.m.UpdateAirQuality(aq)
	}
	return nil
}

func (r *weatherRecorder) pollForecast(ctx context.Context, now time.Time) error {
	hours, err := r.api.Forecast(ctx)
	if err != nil {
		return err
	}
	if _, err := r.db.UpsertForecast(ctx, r.lat, r.lon, now, hours); err != nil {
		log.Printf("DB forecast write error: %v", err)
	}
	r.cache.setForecast(hours)
	if r.m != nil {
		r.m.UpdateForecast(r.lat, r.lon, hours, now)
	}
	return nil
}

// runWeather polls every minute (each poll fetches only what is due) until
// ctx is done; once polls a single time.
func runWeather(ctx context.Context, r *weatherRecorder, once bool) {
	r.Poll(ctx)
	if once {
		return
	}
	ticker := time.NewTicker(time.Minute)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			r.Poll(ctx)
		}
	}
}

// --- backfill ---

type weatherBackfillOptions struct {
	from, to   time.Time
	delay      time.Duration
	configPath string
}

type weatherBackfillResult struct {
	weather, airQuality int64
}

func parseWeatherBackfillArgs(args []string, now time.Time) (weatherBackfillOptions, error) {
	fs := flag.NewFlagSet("weather-backfill", flag.ContinueOnError)
	fs.SetOutput(io.Discard)
	var from, to string
	o := weatherBackfillOptions{to: now.UTC()}
	fs.StringVar(&from, "from", "", "")
	fs.StringVar(&to, "to", "", "")
	fs.DurationVar(&o.delay, "delay", time.Second, "")
	fs.StringVar(&o.configPath, "config", os.Getenv("AIRQUALITY_CONFIG"), "")
	if err := fs.Parse(args); err != nil {
		return o, err
	}
	if from == "" {
		return o, errors.New("--from is required")
	}
	var err error
	if o.from, err = parseBackfillTime(from); err != nil {
		return o, fmt.Errorf("--from: %w", err)
	}
	if to != "" {
		if o.to, err = parseBackfillTime(to); err != nil {
			return o, fmt.Errorf("--to: %w", err)
		}
	}
	if !o.from.Before(o.to) {
		return o, errors.New("--from must be before --to")
	}
	return o, nil
}

// runWeatherBackfill loads hourly weather (historical-forecast API) and,
// once the migration is applied, hourly air quality, in two-week requests.
// Weather rows already stored for an hour are kept; air quality is upserted.
// Re-running it is safe.
func runWeatherBackfill(ctx context.Context, api weatherAPI, db weatherStore, o weatherBackfillOptions, logf func(string, ...any)) (weatherBackfillResult, error) {
	var res weatherBackfillResult
	windows := tempest.Windows(o.from, o.to, weatherBackfillWindow)
	wait := func(i int) error {
		if i == 0 || o.delay == 0 {
			return nil
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(o.delay):
			return nil
		}
	}
	logf("Backfilling weather %s to %s in %d request(s)", o.from.Format("2006-01-02"), o.to.Format("2006-01-02"), len(windows))
	for i, w := range windows {
		if err := wait(i); err != nil {
			return res, err
		}
		rows, err := api.History(ctx, w.Start, w.End.Add(-time.Second))
		if err != nil {
			return res, fmt.Errorf("weather %s: %w", w.Start.Format("2006-01-02"), err)
		}
		rows = within(rows, o.from, o.to, func(r openmeteo.Reading) time.Time { return r.Time })
		n, err := db.BatchInsertWeather(ctx, rows)
		if err != nil {
			return res, err
		}
		res.weather += n
		logf("  weather %s: %d hour(s), %d new", w.Start.Format("2006-01-02"), len(rows), n)
	}
	if !db.OpenMeteoExtras() {
		logf("Air quality skipped: apply supabase/migrations/20261121000000_open_meteo_more.sql first")
		return res, nil
	}
	for i, w := range windows {
		if err := wait(i + 1); err != nil {
			return res, err
		}
		rows, err := api.AirQualityHistory(ctx, w.Start, w.End.Add(-time.Second))
		if err != nil {
			return res, fmt.Errorf("air quality %s: %w", w.Start.Format("2006-01-02"), err)
		}
		rows = within(rows, o.from, o.to, func(a openmeteo.AirQuality) time.Time { return a.Time })
		n, err := db.UpsertAirQuality(ctx, rows)
		if err != nil {
			return res, err
		}
		res.airQuality += n
		logf("  air quality %s: %d hour(s), %d new or changed", w.Start.Format("2006-01-02"), len(rows), n)
	}
	return res, nil
}

func weatherBackfillMain(args []string) {
	o, err := parseWeatherBackfillArgs(args, time.Now())
	if err != nil {
		fmt.Fprintf(os.Stderr, `weather-backfill: %v

Usage: airquality weather-backfill --from DATE [--to DATE] [--delay 1s] [--config path]

Loads hourly Open-Meteo weather (and outdoor air quality, once its migration
is applied) for the configured location, in two-week requests. Re-running is
safe. DATE is YYYY-MM-DD (UTC) or RFC 3339; --to defaults to now. Needs
DATABASE_URL and WEATHER_LAT/WEATHER_LON or the config file's weather.lat/lon.
`, err)
		os.Exit(2)
	}
	fileCfg, err := config.Load(o.configPath)
	if err != nil {
		log.Fatalf("Config: %v", err)
	}
	client := resolveWeatherClient(os.Getenv("WEATHER_LAT"), os.Getenv("WEATHER_LON"), fileCfg)
	if client == nil {
		log.Fatal("No weather location: set WEATHER_LAT/WEATHER_LON or weather.lat/lon in the config file.")
	}
	dbURL := os.Getenv("DATABASE_URL")
	if dbURL == "" {
		log.Fatal("DATABASE_URL is required.")
	}
	ctx := context.Background()
	db, err := store.New(ctx, dbURL)
	if err != nil {
		log.Fatalf("Failed to connect to database: %v", err)
	}
	defer db.Close()
	res, err := runWeatherBackfill(ctx, client, db, o, log.Printf)
	if err != nil {
		log.Fatalf("Backfill failed after %d weather and %d air quality row(s): %v", res.weather, res.airQuality, err)
	}
	log.Printf("Backfill done: %d new weather row(s), %d air quality row(s) new or changed", res.weather, res.airQuality)
}

func orDash(p *float64) any {
	if p == nil {
		return "-"
	}
	return *p
}

// within keeps rows in [from, to]. The APIs answer whole days: the first
// day can start before --from, and the current day includes hours that are
// still forecasts, not observations.
func within[T any](rows []T, from, to time.Time, at func(T) time.Time) []T {
	out := rows[:0:0]
	for _, r := range rows {
		if t := at(r); !t.Before(from) && !t.After(to) {
			out = append(out, r)
		}
	}
	return out
}
