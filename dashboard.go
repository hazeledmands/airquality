package main

import (
	"encoding/json"
	"log"
	"net/http"
	"strconv"
	"time"

	_ "embed"

	"github.com/hazel/airquality/logger"
	"github.com/hazel/airquality/openmeteo"
	"github.com/hazel/airquality/store"
)

//go:embed rooms.html
var roomsHTML string

type annotationJSON struct {
	ID        int64  `json:"id"`
	Timestamp string `json:"timestamp"`
	Note      string `json:"note"`
	Room      string `json:"room"` // empty = global (all charts)
}

type dashboardData struct {
	Sensors     []sensorData     `json:"sensors"`
	ACStates    []acStateData    `json:"ac_states"`
	Weather     []weatherReading `json:"weather"`
	Annotations []annotationJSON `json:"annotations"`
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
	P03       float64 `json:"p03"`
	P05       float64 `json:"p05"`
	P10       float64 `json:"p10"`
	P25       float64 `json:"p25"`
	P50       float64 `json:"p50"`
	P100      float64 `json:"p100"`
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

func groupReadings(records []logger.Record, latestLocs map[string]string) []sensorData {
	sensorMap := make(map[string]*sensorData)
	var order []string
	for _, r := range records {
		key := r.Source + "/" + r.SensorID
		sensor, exists := sensorMap[key]
		if !exists {
			loc := r.Location
			if latest, ok := latestLocs[key]; ok {
				loc = latest
			}
			sensor = &sensorData{
				Source:   r.Source,
				SensorID: r.SensorID,
				Location: loc,
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
			P03:       r.P03,
			P05:       r.P05,
			P10:       r.P10,
			P25:       r.P25,
			P50:       r.P50,
			P100:      r.P100,
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

func dataHandler(cache *dataCache) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		to := time.Now().UTC()
		from := to.Add(-cacheDuration)

		if s := r.URL.Query().Get("from"); s != "" {
			if ms, err := strconv.ParseInt(s, 10, 64); err == nil {
				from = time.UnixMilli(ms).UTC()
			}
		}
		if s := r.URL.Query().Get("to"); s != "" {
			if ms, err := strconv.ParseInt(s, 10, 64); err == nil {
				to = time.UnixMilli(ms).UTC()
			}
		}

		readings, acRecords, weatherRows, annotationRows := cache.snapshotRange(from, to)

		if s := r.URL.Query().Get("stepSize"); s != "" {
			if ms, err := strconv.ParseInt(s, 10, 64); err == nil && ms > 0 {
				readings = downsampleReadings(readings, from, ms)
				weatherRows = downsampleWeather(weatherRows, from, ms)
			}
		}

		data := &dashboardData{
			Sensors:     groupReadings(readings, cache.latestLocations()),
			ACStates:    groupACStates(acRecords),
			Weather:     weatherReadings(weatherRows),
			Annotations: make([]annotationJSON, 0, len(annotationRows)),
		}
		for _, a := range annotationRows {
			data.Annotations = append(data.Annotations, annotationJSON{
				ID:        a.ID,
				Timestamp: a.Timestamp.UTC().Format(time.RFC3339),
				Note:      a.Note,
				Room:      a.Room,
			})
		}

		w.Header().Set("Content-Type", "application/json")
		if err := json.NewEncoder(w).Encode(data); err != nil {
			log.Printf("encode data response: %v", err)
		}
	}
}

func annotationsWriteHandler(db *store.Store, cache *dataCache) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var body struct {
			Timestamp string `json:"timestamp"`
			Note      string `json:"note"`
			Room      string `json:"room"` // optional; empty = global
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			http.Error(w, "bad request", http.StatusBadRequest)
			return
		}
		ts, err := time.Parse(time.RFC3339, body.Timestamp)
		if err != nil {
			http.Error(w, "invalid timestamp", http.StatusBadRequest)
			return
		}
		if body.Note == "" {
			http.Error(w, "note is required", http.StatusBadRequest)
			return
		}
		a, err := db.InsertAnnotation(r.Context(), ts, body.Note, body.Room)
		if err != nil {
			log.Printf("insert annotation: %v", err)
			http.Error(w, "internal error", http.StatusInternalServerError)
			return
		}
		cache.addAnnotation(a)
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusCreated)
		if err := json.NewEncoder(w).Encode(annotationJSON{
			ID:        a.ID,
			Timestamp: a.Timestamp.UTC().Format(time.RFC3339),
			Note:      a.Note,
			Room:      a.Room,
		}); err != nil {
			log.Printf("encode annotation response: %v", err)
		}
	}
}

func annotationsDeleteHandler(db *store.Store, cache *dataCache) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
		if err != nil {
			http.Error(w, "invalid id", http.StatusBadRequest)
			return
		}
		if err := db.DeleteAnnotation(r.Context(), id); err != nil {
			log.Printf("delete annotation: %v", err)
			http.Error(w, "internal error", http.StatusInternalServerError)
			return
		}
		cache.deleteAnnotation(id)
		w.WriteHeader(http.StatusNoContent)
	}
}

func roomsHandler() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		w.Write([]byte(roomsHTML))
	}
}
