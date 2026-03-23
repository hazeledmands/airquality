package main

import (
	"encoding/json"
	"log"
	"net/http"
	"time"

	_ "embed"

	"github.com/hazel/airquality/logger"
	"github.com/hazel/airquality/openmeteo"
	"github.com/hazel/airquality/store"
)

//go:embed dashboard.html
var dashboardHTML string

//go:embed rooms.html
var roomsHTML string

type dashboardData struct {
	Sensors  []sensorData     `json:"sensors"`
	ACStates []acStateData    `json:"ac_states"`
	Weather  []weatherReading `json:"weather"`
}

type weatherReading struct {
	Timestamp       string  `json:"timestamp"`
	Latitude        float64 `json:"latitude"`
	Longitude       float64 `json:"longitude"`
	TempC           float64 `json:"temp_c"`
	Humidity        float64 `json:"humidity"`
	DewPointC       float64 `json:"dewpoint_c"`
	PrecipitationMM float64 `json:"precipitation_mm"`
	WeatherCode     int     `json:"weather_code"`
	PressureHPa     float64 `json:"pressure_hpa"`
	WindSpeedKMH    float64 `json:"wind_speed_kmh"`
	UVIndex         float64 `json:"uv_index"`
	Sunrise         string  `json:"sunrise"` // RFC3339, "" if zero
	Sunset          string  `json:"sunset"`  // RFC3339, "" if zero
}

func weatherReadings(rows []openmeteo.Reading) []weatherReading {
	out := make([]weatherReading, 0, len(rows))
	for _, r := range rows {
		wr := weatherReading{
			Timestamp:       r.Time.UTC().Format(time.RFC3339),
			Latitude:        r.Latitude,
			Longitude:       r.Longitude,
			TempC:           r.TempC,
			Humidity:        r.Humidity,
			DewPointC:       r.DewPointC,
			PrecipitationMM: r.PrecipitationMM,
			WeatherCode:     r.WeatherCode,
			PressureHPa:     r.PressureHPa,
			WindSpeedKMH:    r.WindSpeedKMH,
			UVIndex:         r.UVIndex,
		}
		if !r.Sunrise.IsZero() {
			wr.Sunrise = r.Sunrise.UTC().Format(time.RFC3339)
		}
		if !r.Sunset.IsZero() {
			wr.Sunset = r.Sunset.UTC().Format(time.RFC3339)
		}
		out = append(out, wr)
	}
	return out
}

type sensorData struct {
	Source   string    `json:"source"`
	SensorID string    `json:"sensor_id"`
	Location string    `json:"location"`
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
	Timestamp  string `json:"timestamp"`
	On         bool   `json:"on"`
	Mode       string `json:"mode"`
	TargetTemp int    `json:"target_temp"`
	TempUnit   string `json:"temp_unit"`
	FanLevel   string `json:"fan_level"`
	Reason     string `json:"reason"`
}

func groupReadings(records []logger.Record) []sensorData {
	sensorMap := make(map[string]*sensorData)
	var order []string
	for _, r := range records {
		key := r.Source + "/" + r.SensorID
		sensor, exists := sensorMap[key]
		if !exists {
			sensor = &sensorData{
				Source:   r.Source,
				SensorID: r.SensorID,
				Location: r.Location,
			}
			sensorMap[key] = sensor
			order = append(order, key)
		}
		sensor.Readings = append(sensor.Readings, reading{
			Timestamp: r.Timestamp.UTC().Format(time.RFC3339),
			TempC:     r.TempC,
			Humidity:  r.Humidity,
			DewPointC: r.DewPointC,
			Pressure:  r.Pressure,
			PM1:       r.PM1,
			PM25:      r.PM25,
			PM10:      r.PM10,
			AQI:       r.AQI,
			VOC:       r.VOC,
		})
	}
	result := make([]sensorData, 0, len(order))
	for _, key := range order {
		result = append(result, *sensorMap[key])
	}
	return result
}

func groupACStates(records []logger.ACStateRecord) []acStateData {
	stateMap := make(map[string]*acStateData)
	var order []string
	for _, r := range records {
		state, exists := stateMap[r.DeviceID]
		if !exists {
			state = &acStateData{
				DeviceID: r.DeviceID,
				Room:     r.RoomName,
			}
			stateMap[r.DeviceID] = state
			order = append(order, r.DeviceID)
		}
		state.Changes = append(state.Changes, acStateChange{
			Timestamp:  r.Timestamp,
			On:         r.On,
			Mode:       r.Mode,
			TargetTemp: r.TargetTemp,
			TempUnit:   r.TempUnit,
			FanLevel:   r.FanLevel,
			Reason:     r.Reason,
		})
	}
	result := make([]acStateData, 0, len(order))
	for _, key := range order {
		result = append(result, *stateMap[key])
	}
	return result
}

func dataHandler(db *store.Store) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		to := time.Now().UTC()
		from := to.AddDate(0, 0, -30)

		data := &dashboardData{
			Sensors:  []sensorData{},
			ACStates: []acStateData{},
			Weather:  []weatherReading{},
		}

		if db != nil {
			readings, err := db.QueryReadings(r.Context(), from, to)
			if err != nil {
				log.Printf("Warning: readings query: %v", err)
			} else {
				data.Sensors = groupReadings(readings)
			}

			acRecords, err := db.QueryACStates(r.Context(), from, to)
			if err != nil {
				log.Printf("Warning: ac states query: %v", err)
			} else {
				data.ACStates = groupACStates(acRecords)
			}

			weatherRows, err := db.QueryWeather(r.Context(), from, to)
			if err != nil {
				log.Printf("Warning: weather query: %v", err)
			} else {
				data.Weather = weatherReadings(weatherRows)
			}
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

func roomsHandler() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		w.Write([]byte(roomsHTML))
	}
}
