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
	Sensors  []sensorData   `json:"sensors"`
	ACStates []acStateData `json:"ac_states"`
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

type acStateData struct {
	DeviceID string          `json:"device_id"`
	Room     string          `json:"room"`
	Changes  []acStateChange `json:"changes"`
}

type acStateChange struct {
	Timestamp string `json:"timestamp"`
	On        bool   `json:"on"`
	Mode      string `json:"mode"`
	TargetTemp int   `json:"target_temp"`
	TempUnit  string `json:"temp_unit"`
	FanLevel  string `json:"fan_level"`
	Reason    string `json:"reason"`
}

func parseCSVData(path string, acPath string) (*dashboardData, error) {
	data, err := parseSensorCSVData(path)
	if err != nil {
		return nil, err
	}

	acStates, err := parseACStateCSVData(acPath)
	if err != nil {
		return nil, err
	}
	data.ACStates = acStates
	return data, nil
}

func parseSensorCSVData(path string) (*dashboardData, error) {
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

func parseACStateCSVData(path string) ([]acStateData, error) {
	f, err := os.Open(path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil, nil
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
		return nil, nil
	}

	stateMap := make(map[string]*acStateData)
	var order []string

	for _, row := range records[1:] {
		if len(row) < 13 {
			continue
		}
		key := row[1]
		state, exists := stateMap[key]
		if !exists {
			state = &acStateData{
				DeviceID: row[1],
				Room:     row[2],
			}
			stateMap[key] = state
			order = append(order, key)
		}
		state.Changes = append(state.Changes, acStateChange{
			Timestamp:  row[0],
			On:         parseCSVBool(row[3]),
			Mode:       row[4],
			TargetTemp: parseCSVInt(row[5]),
			TempUnit:   row[6],
			FanLevel:   row[7],
			Reason:     row[11],
		})
	}

	var states []acStateData
	for _, key := range order {
		states = append(states, *stateMap[key])
	}
	return states, nil
}

func parseCSVFloat(s string) float64 {
	if s == "" {
		return 0
	}
	v, _ := strconv.ParseFloat(s, 64)
	return v
}

func parseCSVInt(s string) int {
	if s == "" {
		return 0
	}
	v, _ := strconv.Atoi(s)
	return v
}

func parseCSVBool(s string) bool {
	v, _ := strconv.ParseBool(s)
	return v
}

func dataHandler(csvPath string, acPath string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		data, err := parseCSVData(csvPath, acPath)
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
