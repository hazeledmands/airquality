package main

import (
	"fmt"
	"log"
	"os"
	"strings"

	"github.com/hazel/airquality/calc"
	"github.com/hazel/airquality/purpleair"
	"github.com/hazel/airquality/sensibo"
)

func main() {
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
	var hosts []string
	for _, h := range strings.Split(hostsStr, ",") {
		h = strings.TrimSpace(h)
		if h != "" {
			hosts = append(hosts, h)
		}
	}

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
