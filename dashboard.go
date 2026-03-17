package main

import (
	"encoding/csv"
	"encoding/json"
	"errors"
	"net/http"
	"os"
	"strconv"

	_ "embed"
)

//go:embed dashboard.html
var dashboardHTML string

type dashboardData struct {
	Sensors []sensorData `json:"sensors"`
}

type sensorData struct {
	Source   string    `json:"source"`
	SensorID string   `json:"sensor_id"`
	Location string   `json:"location"`
	Readings []reading `json:"readings"`
}

type reading struct {
	Timestamp string  `json:"timestamp"`
	TempC     float64 `json:"temp_c"`
	Humidity  float64 `json:"humidity"`
	DewPointC float64 `json:"dewpoint_c"`
	Pressure  float64 `json:"pressure"`
	PM1       float64 `json:"pm1"`
	PM25      float64 `json:"pm25"`
	PM10      float64 `json:"pm10"`
	AQI       float64 `json:"aqi"`
	VOC       float64 `json:"voc"`
}

func parseCSVData(path string) (*dashboardData, error) {
	f, err := os.Open(path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return &dashboardData{}, nil
		}
		return nil, err
	}
	defer f.Close()

	r := csv.NewReader(f)
	records, err := r.ReadAll()
	if err != nil {
		return nil, err
	}

	if len(records) < 2 {
		return &dashboardData{}, nil
	}

	sensorMap := make(map[string]*sensorData)
	var order []string

	for _, row := range records[1:] {
		if len(row) < 13 {
			continue
		}
		key := row[1] + "/" + row[2]
		sensor, exists := sensorMap[key]
		if !exists {
			sensor = &sensorData{
				Source:   row[1],
				SensorID: row[2],
				Location: row[3],
			}
			sensorMap[key] = sensor
			order = append(order, key)
		}
		sensor.Readings = append(sensor.Readings, reading{
			Timestamp: row[0],
			TempC:     parseCSVFloat(row[4]),
			Humidity:  parseCSVFloat(row[5]),
			DewPointC: parseCSVFloat(row[6]),
			Pressure:  parseCSVFloat(row[7]),
			PM1:       parseCSVFloat(row[8]),
			PM25:      parseCSVFloat(row[9]),
			PM10:      parseCSVFloat(row[10]),
			AQI:       parseCSVFloat(row[11]),
			VOC:       parseCSVFloat(row[12]),
		})
	}

	data := &dashboardData{}
	for _, key := range order {
		data.Sensors = append(data.Sensors, *sensorMap[key])
	}
	return data, nil
}

func parseCSVFloat(s string) float64 {
	if s == "" {
		return 0
	}
	v, _ := strconv.ParseFloat(s, 64)
	return v
}

func dataHandler(csvPath string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		data, err := parseCSVData(csvPath)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(data)
	}
}

func dashboardHandler() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		w.Write([]byte(dashboardHTML))
	}
}
