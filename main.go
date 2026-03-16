package main

import (
	"fmt"
	"log"
	"os"

	"github.com/hazel/airquality/sensibo"
)

func main() {
	apiKey := os.Getenv("SENSIBO_API_KEY")
	if apiKey == "" {
		log.Fatal("SENSIBO_API_KEY environment variable is required")
	}

	client := sensibo.NewClient(apiKey)

	devices, err := client.GetDevices()
	if err != nil {
		log.Fatalf("Failed to get devices: %v", err)
	}

	fmt.Printf("Found %d device(s)\n\n", len(devices))

	for _, dev := range devices {
		measurements, err := client.GetHistoricalMeasurements(dev.ID, 1)
		if err != nil {
			log.Printf("Warning: failed to get measurements for %s (%s): %v", dev.Room.Name, dev.ID, err)
			continue
		}

		summary := sensibo.Summarize(dev.ID, dev.Room.Name, measurements)
		printSummary(summary)
	}
}

func printSummary(s sensibo.DeviceSummary) {
	fmt.Printf("=== %s (%s) ===\n", s.RoomName, s.DeviceID)
	if s.DataPoints == 0 {
		fmt.Println("  No data available")
		fmt.Println()
		return
	}

	fmt.Printf("  Latest reading (%s):\n", s.Latest.Time.Local().Format("Jan 02 15:04"))
	fmt.Printf("    Temperature: %.1f°C\n", s.Latest.Temperature)
	fmt.Printf("    Humidity:    %.1f%%\n", s.Latest.Humidity)
	fmt.Println()
	fmt.Printf("  Last 24h summary (%d data points):\n", s.DataPoints)
	fmt.Printf("    Temperature: %.1f°C avg (%.1f°C – %.1f°C)\n", s.AvgTemp, s.MinTemp, s.MaxTemp)
	fmt.Printf("    Humidity:    %.1f%% avg (%.1f%% – %.1f%%)\n", s.AvgHumid, s.MinHumid, s.MaxHumid)
	fmt.Println()
}
