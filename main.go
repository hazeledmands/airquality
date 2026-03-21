package main

import (
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
	"github.com/hazel/airquality/purpleair"
	"github.com/hazel/airquality/sensibo"
)

type config struct {
	interval time.Duration
	output   string
	acOutput string
	port     int
	verbose  bool
	once     bool
}

func parseConfig(args []string) config {
	var cfg config
	fs := flag.NewFlagSet("airquality", flag.ExitOnError)

	fs.DurationVar(&cfg.interval, "interval", 1*time.Minute, "")
	fs.DurationVar(&cfg.interval, "i", 1*time.Minute, "")
	fs.StringVar(&cfg.output, "output", "airquality.csv", "")
	fs.StringVar(&cfg.output, "o", "airquality.csv", "")
	fs.StringVar(&cfg.acOutput, "ac-output", "acstates.csv", "")
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

Continuously logs air quality sensor data to a CSV file.

Options:
  -i, --interval duration   capture interval (default 1m)
  -o, --output string       output CSV file path (default "airquality.csv")
      --ac-output string    AC state changes CSV file path (default "acstates.csv")
  -p, --port int            prometheus metrics port (default 50233)
  -v, --verbose             print detailed sensor data on each capture
      --once                capture once and exit
  -h, --help                show this help

Environment:
  SENSIBO_API_KEY     Sensibo cloud API key
  PURPLEAIR_HOSTS     comma-separated PurpleAir sensor IPs

At least one sensor must be configured.
`)
}

func main() {
	cfg := parseConfig(os.Args[1:])

	sensiboKey := os.Getenv("SENSIBO_API_KEY")
	purpleairHosts := parsePurpleAirHosts(os.Getenv("PURPLEAIR_HOSTS"))

	if sensiboKey == "" && len(purpleairHosts) == 0 {
		log.Fatal("No sensors configured. Set SENSIBO_API_KEY and/or PURPLEAIR_HOSTS.")
	}

	csvLog, err := logger.NewCSVLogger(cfg.output)
	if err != nil {
		log.Fatalf("Failed to open log file: %v", err)
	}
	defer csvLog.Close()

	var acLog *logger.ACStateCSVLogger
	var acExistingIDs map[string]bool
	if sensiboKey != "" {
		acExistingIDs, err = logger.LoadExistingACStateIDs(cfg.acOutput)
		if err != nil {
			log.Fatalf("Failed to load AC state log: %v", err)
		}
		acLog, err = logger.NewACStateCSVLogger(cfg.acOutput)
		if err != nil {
			log.Fatalf("Failed to open AC state log: %v", err)
		}
		defer acLog.Close()
		log.Printf("Tracking AC state changes in %s (%d existing entries)", cfg.acOutput, len(acExistingIDs))
	}

	m := metrics.New()

	if cfg.once {
		captureAll(csvLog, acLog, acExistingIDs, m, sensiboKey, purpleairHosts, cfg.verbose)
		return
	}

	go func() {
		addr := fmt.Sprintf(":%d", cfg.port)
		mux := http.NewServeMux()
		mux.Handle("/metrics", m.Handler())
		mux.HandleFunc("/api/data", dataHandler(cfg.output, cfg.acOutput))
		mux.HandleFunc("/by-metric", dashboardHandler())
		mux.HandleFunc("/", roomsHandler())
		log.Printf("Dashboard at http://0.0.0.0:%d/ (accessible from any machine on the network)", cfg.port)
		log.Printf("Prometheus metrics at http://0.0.0.0:%d/metrics", cfg.port)
		if err := http.ListenAndServe(addr, mux); err != nil {
			log.Fatalf("Metrics server: %v", err)
		}
	}()

	log.Printf("Logging to %s every %s (Ctrl+C to stop)", cfg.output, cfg.interval)
	captureAll(csvLog, acLog, acExistingIDs, m, sensiboKey, purpleairHosts, cfg.verbose)

	ticker := time.NewTicker(cfg.interval)
	defer ticker.Stop()

	sig := make(chan os.Signal, 1)
	signal.Notify(sig, os.Interrupt)

	for {
		select {
		case <-ticker.C:
			captureAll(csvLog, acLog, acExistingIDs, m, sensiboKey, purpleairHosts, cfg.verbose)
		case <-sig:
			log.Println("Stopping.")
			return
		}
	}
}

func captureAll(csvLog *logger.CSVLogger, acLog *logger.ACStateCSVLogger, acExistingIDs map[string]bool, m *metrics.Metrics, sensiboKey string, purpleairHosts []string, verbose bool) {
	now := time.Now()

	if sensiboKey != "" {
		captureSensibo(csvLog, m, sensiboKey, now, verbose)
		captureACStates(acLog, acExistingIDs, sensiboKey, verbose)
	}
	if len(purpleairHosts) > 0 {
		capturePurpleAir(csvLog, m, purpleairHosts, now, verbose)
	}
}

func captureSensibo(csvLog *logger.CSVLogger, m *metrics.Metrics, apiKey string, now time.Time, verbose bool) {
	client := sensibo.NewClient(apiKey)
	devices, err := client.GetDevices()
	if err != nil {
		log.Printf("Sensibo error: %v", err)
		return
	}
	for _, dev := range devices {
		measurements, err := client.GetHistoricalMeasurements(dev.ID, 1)
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
		if err := csvLog.Log(rec); err != nil {
			log.Printf("Log write error: %v", err)
		}
		m.Update(rec)
		if verbose {
			printVerboseRecord(rec)
		} else {
			log.Printf("  sensibo/%s (%s): %.1f°C, %.1f%%, dew point %.1f°C", dev.ID, dev.Room.Name, rec.TempC, rec.Humidity, rec.DewPointC)
		}
	}
}

func capturePurpleAir(csvLog *logger.CSVLogger, m *metrics.Metrics, hosts []string, now time.Time, verbose bool) {
	client := purpleair.NewClient(hosts)
	for _, host := range hosts {
		reading, err := client.Fetch(host)
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
		}
		if err := csvLog.Log(rec); err != nil {
			log.Printf("Log write error: %v", err)
		}
		m.Update(rec)
		if verbose {
			printVerboseRecord(rec)
		} else {
			log.Printf("  purpleair/%s (%s): %.1f°C, %.1f%%, dew point %.1f°C, PM2.5=%.1f", s.SensorID, s.Place, s.TempC, s.Humidity, s.DewPointC, s.PM25)
		}
	}
}

func captureACStates(acLog *logger.ACStateCSVLogger, existingIDs map[string]bool, apiKey string, verbose bool) {
	client := sensibo.NewClient(apiKey)
	devices, err := client.GetDevices()
	if err != nil {
		log.Printf("Sensibo error (AC states): %v", err)
		return
	}
	for _, dev := range devices {
		states, err := client.GetACStates(dev.ID, 20)
		if err != nil {
			log.Printf("Sensibo AC states %s (%s): %v", dev.Room.Name, dev.ID, err)
			continue
		}
		newCount := 0
		for _, sc := range states {
			if existingIDs[sc.ID] {
				continue
			}
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
			if err := acLog.Log(rec); err != nil {
				log.Printf("AC state log write error: %v", err)
				continue
			}
			existingIDs[sc.ID] = true
			newCount++
			if verbose {
				log.Printf("  AC state: %s (%s) on=%v mode=%s temp=%d%s fan=%s reason=%s [%s]",
					dev.Room.Name, dev.ID, sc.ACState.On, sc.ACState.Mode,
					sc.ACState.TargetTemperature, sc.ACState.TemperatureUnit,
					sc.ACState.FanLevel, sc.Reason,
					strings.Join(sc.ChangedProperties, ","))
			}
		}
		if newCount > 0 {
			log.Printf("  AC states: %s — %d new state change(s) recorded", dev.Room.Name, newCount)
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
	}
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
