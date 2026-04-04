package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/signal"
	"strings"
	"time"

	"github.com/hazel/airquality/calc"
	"github.com/hazel/airquality/logger"
	"github.com/hazel/airquality/metrics"
	"github.com/hazel/airquality/openmeteo"
	"github.com/hazel/airquality/purpleair"
	"github.com/hazel/airquality/sensibo"
	"github.com/hazel/airquality/store"
)

type config struct {
	interval time.Duration
	port     int
	verbose  bool
	once     bool
}

func parseConfig(args []string) config {
	var cfg config
	fs := flag.NewFlagSet("airquality", flag.ExitOnError)

	fs.DurationVar(&cfg.interval, "interval", 1*time.Minute, "")
	fs.DurationVar(&cfg.interval, "i", 1*time.Minute, "")
	fs.BoolVar(&cfg.verbose, "verbose", false, "")
	fs.BoolVar(&cfg.verbose, "v", false, "")
	fs.BoolVar(&cfg.once, "once", false, "")
	fs.IntVar(&cfg.port, "port", 50233, "")
	fs.IntVar(&cfg.port, "p", 50233, "")

	fs.Usage = func() { printUsage(fs) }
	fs.Parse(args)
	return cfg
}

func printUsage(_ *flag.FlagSet) {
	fmt.Fprintf(os.Stderr, `Usage: airquality [options]

Continuously logs air quality sensor data to PostgreSQL.

Options:
  -i, --interval duration   capture interval (default 1m)
  -p, --port int            prometheus metrics port (default 50233)
  -v, --verbose             print detailed sensor data on each capture
      --once                capture once and exit
  -h, --help                show this help

Environment:
  SENSIBO_API_KEY     Sensibo cloud API key
  PURPLEAIR_HOSTS     comma-separated PurpleAir sensor IPs
  DATABASE_URL        PostgreSQL connection string (required)

At least one sensor must be configured.
`)
}

func main() {
	cfg := parseConfig(os.Args[1:])

	sensiboKey := os.Getenv("SENSIBO_API_KEY")
	purpleairHosts := parsePurpleAirHosts(os.Getenv("PURPLEAIR_HOSTS"))
	weatherClient := parseWeatherClient(os.Getenv("WEATHER_LAT"), os.Getenv("WEATHER_LON"))

	if sensiboKey == "" && len(purpleairHosts) == 0 {
		log.Fatal("No sensors configured. Set SENSIBO_API_KEY and/or PURPLEAIR_HOSTS.")
	}

	dbURL := os.Getenv("DATABASE_URL")
	if dbURL == "" {
		log.Fatal("DATABASE_URL is required.")
	}
	db, err := store.New(context.Background(), dbURL)
	if err != nil {
		log.Fatalf("Failed to connect to database: %v", err)
	}
	defer db.Close()
	log.Printf("Connected to database")

	cache, err := newDataCache(context.Background(), db)
	if err != nil {
		log.Fatalf("Failed to backfill cache: %v", err)
	}

	m := metrics.New()

	if cfg.once {
		captureAll(context.Background(), m, db, cache, sensiboKey, purpleairHosts, weatherClient, cfg.verbose)
		return
	}

	go func() {
		addr := fmt.Sprintf(":%d", cfg.port)
		mux := http.NewServeMux()
		mux.Handle("/metrics", m.Handler())
		mux.HandleFunc("/api/data", dataHandler(cache))
		mux.HandleFunc("POST /api/annotations", annotationsWriteHandler(db, cache))
		mux.HandleFunc("DELETE /api/annotations/{id}", annotationsDeleteHandler(db, cache))
		mux.HandleFunc("/", roomsHandler())
		log.Printf("Dashboard at http://0.0.0.0:%d/ (accessible from any machine on the network)", cfg.port)
		log.Printf("Prometheus metrics at http://0.0.0.0:%d/metrics", cfg.port)
		if err := http.ListenAndServe(addr, mux); err != nil {
			log.Fatalf("Metrics server: %v", err)
		}
	}()

	log.Printf("Logging to database every %s (Ctrl+C to stop)", cfg.interval)
	captureAll(context.Background(), m, db, cache, sensiboKey, purpleairHosts, weatherClient, cfg.verbose)

	ticker := time.NewTicker(cfg.interval)
	defer ticker.Stop()

	sig := make(chan os.Signal, 1)
	signal.Notify(sig, os.Interrupt)

	for {
		select {
		case <-ticker.C:
			captureAll(context.Background(), m, db, cache, sensiboKey, purpleairHosts, weatherClient, cfg.verbose)
		case <-sig:
			log.Println("Stopping.")
			return
		}
	}
}

func captureAll(ctx context.Context, m *metrics.Metrics, db *store.Store, cache *dataCache, sensiboKey string, purpleairHosts []string, weatherClient *openmeteo.Client, verbose bool) {
	now := time.Now()

	if sensiboKey != "" {
		captureSensibo(ctx, m, db, cache, sensiboKey, now, verbose)
		captureACStates(ctx, db, cache, sensiboKey, verbose)
	}
	if len(purpleairHosts) > 0 {
		capturePurpleAir(ctx, m, db, cache, purpleairHosts, now, verbose)
	}
	if weatherClient != nil {
		captureWeather(ctx, m, db, cache, weatherClient, verbose)
	}
}

func captureSensibo(ctx context.Context, m *metrics.Metrics, db *store.Store, cache *dataCache, apiKey string, now time.Time, verbose bool) {
	client := sensibo.NewClient(apiKey)
	devices, err := client.GetDevices(ctx)
	if err != nil {
		log.Printf("Sensibo error: %v", err)
		return
	}
	for _, dev := range devices {
		measurements, err := client.GetHistoricalMeasurements(ctx, dev.ID, 1)
		if err != nil {
			log.Printf("Sensibo %s (%s): %v", dev.Room.Name, dev.ID, err)
			continue
		}
		if len(measurements.Temperature) == 0 {
			continue
		}
		latest := measurements.Temperature[len(measurements.Temperature)-1]
		rec := logger.Record{
			Timestamp: now,
			Source:    "sensibo",
			SensorID:  dev.ID,
			Location:  dev.Room.Name,
			TempC:     latest.Value,
		}
		if len(measurements.Humidity) > 0 {
			h := measurements.Humidity[len(measurements.Humidity)-1].Value
			rec.Humidity = h
			rec.DewPointC = calc.DewPoint(latest.Value, h)
		}
		if err := db.InsertReading(context.Background(), rec); err != nil {
			log.Printf("DB write error: %v", err)
		}
		cache.addReading(rec)
		m.Update(rec)
		if verbose {
			printVerboseRecord(rec)
		} else {
			log.Printf("  sensibo/%s (%s): %.1f°C, %.1f%%, dew point %.1f°C", dev.ID, dev.Room.Name, rec.TempC, rec.Humidity, rec.DewPointC)
		}
	}
}

func capturePurpleAir(ctx context.Context, m *metrics.Metrics, db *store.Store, cache *dataCache, hosts []string, now time.Time, verbose bool) {
	client := purpleair.NewClient(hosts)
	for _, host := range hosts {
		reading, err := client.Fetch(ctx, host)
		if err != nil {
			log.Printf("PurpleAir %s: %v", host, err)
			continue
		}
		s := reading.Summary()
		rec := logger.Record{
			Timestamp: now,
			Source:    "purpleair",
			SensorID:  s.SensorID,
			Location:  s.Place,
			TempC:     s.TempC,
			Humidity:  s.Humidity,
			DewPointC: s.DewPointC,
			Pressure:  s.Pressure,
			PM1:       s.PM1,
			PM25:      s.PM25,
			PM10:      s.PM10,
			AQI:       s.AQI,
			VOC:       s.Gas680,
			P03:       s.P03,
			P05:       s.P05,
			P10:       s.P10,
			P25:       s.P25,
			P50:       s.P50,
			P100:      s.P100,
		}
		if err := db.InsertReading(context.Background(), rec); err != nil {
			log.Printf("DB write error: %v", err)
		}
		cache.addReading(rec)
		m.Update(rec)
		if verbose {
			printVerboseRecord(rec)
		} else {
			log.Printf("  purpleair/%s (%s): %.1f°C, %.1f%%, dew point %.1f°C, PM2.5=%.1f", s.SensorID, s.Place, s.TempC, s.Humidity, s.DewPointC, s.PM25)
		}
	}
}

func captureACStates(ctx context.Context, db *store.Store, cache *dataCache, apiKey string, verbose bool) {
	client := sensibo.NewClient(apiKey)
	devices, err := client.GetDevices(ctx)
	if err != nil {
		log.Printf("Sensibo error (AC states): %v", err)
		return
	}
	for _, dev := range devices {
		states, err := client.GetACStates(ctx, dev.ID, 20)
		if err != nil {
			log.Printf("Sensibo AC states %s (%s): %v", dev.Room.Name, dev.ID, err)
			continue
		}
		for _, sc := range states {
			rec := logger.ACStateRecord{
				Timestamp:         sc.Time.Time,
				DeviceID:          dev.ID,
				RoomName:          dev.Room.Name,
				On:                sc.ACState.On,
				Mode:              sc.ACState.Mode,
				TargetTemp:        sc.ACState.TargetTemperature,
				TempUnit:          sc.ACState.TemperatureUnit,
				FanLevel:          sc.ACState.FanLevel,
				Swing:             sc.ACState.Swing,
				HorizontalSwing:   sc.ACState.HorizontalSwing,
				ChangedProperties: strings.Join(sc.ChangedProperties, ";"),
				Reason:            sc.Reason,
				SensiboID:         sc.ID,
			}
			if err := db.InsertACState(context.Background(), rec); err != nil {
				log.Printf("DB AC state write error: %v", err)
			}
			cache.addACState(rec)
			if verbose {
				log.Printf("  AC state: %s (%s) on=%v mode=%s temp=%d%s fan=%s reason=%s [%s]",
					dev.Room.Name, dev.ID, sc.ACState.On, sc.ACState.Mode,
					sc.ACState.TargetTemperature, sc.ACState.TemperatureUnit,
					sc.ACState.FanLevel, sc.Reason,
					strings.Join(sc.ChangedProperties, ","))
			}
		}
	}
}

func printVerboseRecord(rec logger.Record) {
	f := calc.CtoF
	fmt.Printf("  %s/%s (%s)\n", rec.Source, rec.SensorID, rec.Location)
	fmt.Printf("    Temperature: %.1f°C / %.1f°F\n", rec.TempC, f(rec.TempC))
	fmt.Printf("    Humidity:    %.1f%%\n", rec.Humidity)
	fmt.Printf("    Dew Point:   %.1f°C / %.1f°F\n", rec.DewPointC, f(rec.DewPointC))
	if rec.Pressure != 0 {
		fmt.Printf("    Pressure:    %.1f mbar\n", rec.Pressure)
	}
	if rec.VOC != 0 {
		fmt.Printf("    VOC (gas):   %.1f kOhm\n", rec.VOC)
	}
	if rec.PM1 != 0 || rec.PM25 != 0 || rec.PM10 != 0 {
		fmt.Printf("    PM1.0:       %.1f µg/m³\n", rec.PM1)
		fmt.Printf("    PM2.5:       %.1f µg/m³\n", rec.PM25)
		fmt.Printf("    PM10:        %.1f µg/m³\n", rec.PM10)
		fmt.Printf("    AQI:         %.0f\n", rec.AQI)
		fmt.Printf("    ≥0.3µm:      %.1f /dL\n", rec.P03)
		fmt.Printf("    ≥0.5µm:      %.1f /dL\n", rec.P05)
		fmt.Printf("    ≥1.0µm:      %.1f /dL\n", rec.P10)
		fmt.Printf("    ≥2.5µm:      %.1f /dL\n", rec.P25)
		fmt.Printf("    ≥5.0µm:      %.1f /dL\n", rec.P50)
		fmt.Printf("    ≥10µm:       %.1f /dL\n", rec.P100)
	}
}

func captureWeather(ctx context.Context, m *metrics.Metrics, db *store.Store, cache *dataCache, client *openmeteo.Client, verbose bool) {
	reading, err := client.Fetch(ctx)
	if err != nil {
		log.Printf("Weather error: %v", err)
		return
	}
	if err := db.InsertWeather(context.Background(), reading); err != nil {
		log.Printf("DB weather write error: %v", err)
	}
	cache.addWeather(reading)
	m.UpdateWeather(reading)
	if verbose {
		log.Printf("  weather (%.4f,%.4f): %.1f°C, %.0f%%, dew point %.1f°C, pressure %.1f hPa, wind %.1f km/h, UV %.1f, precip %.1f mm",
			reading.Latitude, reading.Longitude,
			reading.TempC, reading.Humidity, reading.DewPointC,
			reading.PressureHPa, reading.WindSpeedKMH, reading.UVIndex, reading.PrecipitationMM)
	} else {
		log.Printf("  weather: %.1f°C, %.0f%% humidity, %.1f hPa", reading.TempC, reading.Humidity, reading.PressureHPa)
	}
}

func parseWeatherClient(latStr, lonStr string) *openmeteo.Client {
	if latStr == "" || lonStr == "" {
		return nil
	}
	var lat, lon float64
	if _, err := fmt.Sscanf(latStr, "%f", &lat); err != nil {
		log.Printf("Invalid WEATHER_LAT %q: %v", latStr, err)
		return nil
	}
	if _, err := fmt.Sscanf(lonStr, "%f", &lon); err != nil {
		log.Printf("Invalid WEATHER_LON %q: %v", lonStr, err)
		return nil
	}
	return openmeteo.NewClient(lat, lon)
}

func parsePurpleAirHosts(hostsStr string) []string {
	var hosts []string
	for _, h := range strings.Split(hostsStr, ",") {
		h = strings.TrimSpace(h)
		if h != "" {
			hosts = append(hosts, h)
		}
	}
	return hosts
}
