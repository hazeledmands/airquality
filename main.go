package main

import (
	"flag"
	"fmt"
	"log"
	"os"
	"os/signal"
	"strings"
	"time"

	"github.com/hazel/airquality/calc"
	"github.com/hazel/airquality/logger"
	"github.com/hazel/airquality/purpleair"
	"github.com/hazel/airquality/sensibo"
)

func main() {
	if len(os.Args) > 1 && os.Args[0] != "-test" && os.Args[1] == "log" {
		runLog(os.Args[2:])
		return
	}

	hasSensibo := false
	if apiKey := os.Getenv("SENSIBO_API_KEY"); apiKey != "" {
		hasSensibo = true
		fetchSensibo(apiKey)
	}

	hasPurpleAir := false
	if hosts := os.Getenv("PURPLEAIR_HOSTS"); hosts != "" {
		hasPurpleAir = true
		fetchPurpleAir(hosts)
	}

	if !hasSensibo && !hasPurpleAir {
		log.Fatal("No sensors configured. Set SENSIBO_API_KEY and/or PURPLEAIR_HOSTS.")
	}
}

func runLog(args []string) {
	fs := flag.NewFlagSet("log", flag.ExitOnError)
	interval := fs.Duration("interval", 1*time.Minute, "capture interval")
	output := fs.String("output", "airquality.csv", "output CSV file path")
	fs.Parse(args)

	sensiboKey := os.Getenv("SENSIBO_API_KEY")
	purpleairHosts := parsePurpleAirHosts(os.Getenv("PURPLEAIR_HOSTS"))

	if sensiboKey == "" && len(purpleairHosts) == 0 {
		log.Fatal("No sensors configured. Set SENSIBO_API_KEY and/or PURPLEAIR_HOSTS.")
	}

	csvLog, err := logger.NewCSVLogger(*output)
	if err != nil {
		log.Fatalf("Failed to open log file: %v", err)
	}
	defer csvLog.Close()

	log.Printf("Logging to %s every %s (Ctrl+C to stop)", *output, *interval)

	// Capture once immediately, then on each tick.
	captureAll(csvLog, sensiboKey, purpleairHosts)

	ticker := time.NewTicker(*interval)
	defer ticker.Stop()

	sig := make(chan os.Signal, 1)
	signal.Notify(sig, os.Interrupt)

	for {
		select {
		case <-ticker.C:
			captureAll(csvLog, sensiboKey, purpleairHosts)
		case <-sig:
			log.Println("Stopping.")
			return
		}
	}
}

func captureAll(csvLog *logger.CSVLogger, sensiboKey string, purpleairHosts []string) {
	now := time.Now()

	if sensiboKey != "" {
		captureSensibo(csvLog, sensiboKey, now)
	}
	if len(purpleairHosts) > 0 {
		capturePurpleAir(csvLog, purpleairHosts, now)
	}
}

func captureSensibo(csvLog *logger.CSVLogger, apiKey string, now time.Time) {
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
		log.Printf("  sensibo/%s (%s): %.1f°C, %.1f%%, dew point %.1f°C", dev.ID, dev.Room.Name, rec.TempC, rec.Humidity, rec.DewPointC)
	}
}

func capturePurpleAir(csvLog *logger.CSVLogger, hosts []string, now time.Time) {
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
		log.Printf("  purpleair/%s (%s): %.1f°C, %.1f%%, dew point %.1f°C, PM2.5=%.1f", s.SensorID, s.Place, s.TempC, s.Humidity, s.DewPointC, s.PM25)
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

func fetchSensibo(apiKey string) {
	client := sensibo.NewClient(apiKey)

	devices, err := client.GetDevices()
	if err != nil {
		log.Printf("Sensibo error: %v", err)
		return
	}

	fmt.Printf("Sensibo: %d device(s)\n\n", len(devices))

	for _, dev := range devices {
		measurements, err := client.GetHistoricalMeasurements(dev.ID, 1)
		if err != nil {
			log.Printf("Warning: failed to get measurements for %s (%s): %v", dev.Room.Name, dev.ID, err)
			continue
		}

		summary := sensibo.Summarize(dev.ID, dev.Room.Name, measurements)
		printSensiboSummary(summary)
	}
}

func fetchPurpleAir(hostsStr string) {
	hosts := parsePurpleAirHosts(hostsStr)
	client := purpleair.NewClient(hosts)

	fmt.Printf("PurpleAir: %d sensor(s)\n\n", len(hosts))

	for _, host := range hosts {
		reading, err := client.Fetch(host)
		if err != nil {
			log.Printf("Warning: PurpleAir %s: %v", host, err)
			continue
		}

		s := reading.Summary()
		printPurpleAirSummary(s)
	}
}

func printSensiboSummary(s sensibo.DeviceSummary) {
	fmt.Printf("=== %s (%s) ===\n", s.RoomName, s.DeviceID)
	if s.DataPoints == 0 {
		fmt.Println("  No data available")
		fmt.Println()
		return
	}

	f := calc.CtoF
	fmt.Printf("  Latest reading (%s):\n", s.Latest.Time.Local().Format("Jan 02 15:04"))
	fmt.Printf("    Temperature: %.1f°C / %.1f°F\n", s.Latest.Temperature, f(s.Latest.Temperature))
	fmt.Printf("    Humidity:    %.1f%%\n", s.Latest.Humidity)
	fmt.Printf("    Dew Point:   %.1f°C / %.1f°F\n", s.Latest.DewPoint, f(s.Latest.DewPoint))
	fmt.Println()
	fmt.Printf("  Last 24h summary (%d data points):\n", s.DataPoints)
	fmt.Printf("    Temperature: %.1f°C / %.1f°F avg (%.1f–%.1f°C / %.1f–%.1f°F)\n",
		s.AvgTemp, f(s.AvgTemp), s.MinTemp, s.MaxTemp, f(s.MinTemp), f(s.MaxTemp))
	fmt.Printf("    Humidity:    %.1f%% avg (%.1f%% – %.1f%%)\n", s.AvgHumid, s.MinHumid, s.MaxHumid)
	fmt.Printf("    Dew Point:   %.1f°C / %.1f°F avg (%.1f–%.1f°C / %.1f–%.1f°F)\n",
		s.AvgDewPoint, f(s.AvgDewPoint), s.MinDewPoint, s.MaxDewPoint, f(s.MinDewPoint), f(s.MaxDewPoint))
	fmt.Println()
}

func printPurpleAirSummary(s purpleair.SensorSummary) {
	fmt.Printf("=== PurpleAir %s (%s) ===\n", s.SensorID, s.Place)

	fmt.Printf("  Environment:\n")
	fmt.Printf("    Temperature: %.1f°C / %.1f°F\n", s.TempC, s.TempF)
	fmt.Printf("    Humidity:    %.1f%%\n", s.Humidity)
	fmt.Printf("    Dew Point:   %.1f°C / %.1f°F\n", s.DewPointC, s.DewPointF)
	fmt.Printf("    Pressure:    %.1f mbar\n", s.Pressure)
	if s.Gas680 != 0 {
		fmt.Printf("    VOC (gas):   %.1f kOhm\n", s.Gas680)
	}
	fmt.Println()

	fmt.Printf("  Air Quality (avg of channels A & B):\n")
	fmt.Printf("    PM1.0:  %.1f µg/m³\n", s.PM1)
	fmt.Printf("    PM2.5:  %.1f µg/m³  (A: %.1f, B: %.1f)\n", s.PM25, s.PM25A, s.PM25B)
	fmt.Printf("    PM10:   %.1f µg/m³\n", s.PM10)
	fmt.Printf("    AQI:    %.0f\n", s.AQI)
	fmt.Println()
}
