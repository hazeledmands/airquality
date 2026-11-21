package main

import (
	"encoding/json"
	"log"
	"math"
	"net/http"
	"reflect"
	"strconv"
	"time"

	_ "embed"

	"github.com/hazel/airquality/calc"
	"github.com/hazel/airquality/config"
	"github.com/hazel/airquality/logger"
	"github.com/hazel/airquality/openmeteo"
	"github.com/hazel/airquality/store"
	"github.com/hazel/airquality/tempest"
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
	Tempest     tempestData      `json:"tempest"`
}

type tempestData struct {
	// Enabled is true when Tempest recording runs, so the dashboard can say
	// "no observations in this range" rather than hide the section.
	Enabled      bool                `json:"enabled"`
	Station      *tempestStationJSON `json:"station"` // null until discovered
	Observations []tempestObsJSON    `json:"observations"`
	// Latest is the newest raw (never downsampled) observation in range,
	// for the latest-values strip.
	Latest *tempestObsJSON    `json:"latest"`
	Events []tempestEventJSON `json:"events"`
}

type tempestStationJSON struct {
	ID         int     `json:"id"`
	Name       string  `json:"name"`
	Room       string  `json:"room"` // config-file room for the first device, else the name
	ElevationM float64 `json:"elevation_m"`
	Devices    []int   `json:"devices"`
}

// tempestObsJSON is what the dashboard draws from an observation (or
// bucket): the fields the page reads, plus dew point (shared with indoor
// sensors) and sea-level pressure (comparable to Open-Meteo's). Device ID,
// wind sample interval, Rain Check per-interval rain and the analysis type
// stay in the database and on /metrics.
type tempestObsJSON struct {
	Timestamp              time.Time `json:"timestamp"`
	WindLullMS             *float64  `json:"wind_lull_ms"`
	WindAvgMS              *float64  `json:"wind_avg_ms"`
	WindGustMS             *float64  `json:"wind_gust_ms"`
	WindDirectionDeg       *float64  `json:"wind_direction_deg"`
	StationPressureMB      *float64  `json:"station_pressure_mb"`
	AirTempC               *float64  `json:"air_temp_c"`
	RelativeHumidity       *float64  `json:"relative_humidity"`
	IlluminanceLux         *float64  `json:"illuminance_lux"`
	UVIndex                *float64  `json:"uv_index"`
	SolarRadiationWM2      *float64  `json:"solar_radiation_wm2"`
	RainMM                 *float64  `json:"rain_mm"`
	RainMaxMM              *float64  `json:"rain_max_mm"`
	PrecipType             *int      `json:"precip_type"`
	LightningAvgDistanceKM *float64  `json:"lightning_avg_distance_km"`
	LightningStrikeCount   *int      `json:"lightning_strike_count"`
	BatteryV               *float64  `json:"battery_v"`
	ReportIntervalMin      *int      `json:"report_interval_min"`
	LocalDayRainMM         *float64  `json:"local_day_rain_mm"`
	LocalDayRainFinalMM    *float64  `json:"local_day_rain_final_mm"`
	DewPointC              *float64  `json:"dewpoint_c"`
	SeaLevelPressureMB     *float64  `json:"sea_level_pressure_mb"`
}

// tempestEventJSON is an event as the dashboard draws it.
type tempestEventJSON struct {
	Timestamp  time.Time `json:"timestamp"`
	Type       string    `json:"type"`
	DistanceKM *float64  `json:"distance_km"`
	Energy     *float64  `json:"energy"`
}

func tempestJSON(b tempestBucket, st *tempest.Station) tempestObsJSON {
	scrubNonFinite(&b.Observation)
	if b.RainMaxMM != nil {
		b.RainMaxMM = finite(*b.RainMaxMM)
	}
	o := b.Observation
	j := tempestObsJSON{
		Timestamp: o.Time, WindLullMS: o.WindLullMS, WindAvgMS: o.WindAvgMS, WindGustMS: o.WindGustMS,
		WindDirectionDeg: o.WindDirectionDeg, StationPressureMB: o.StationPressureMB, AirTempC: o.AirTempC,
		RelativeHumidity: o.RelativeHumidity, IlluminanceLux: o.IlluminanceLux, UVIndex: o.UVIndex,
		SolarRadiationWM2: o.SolarRadiationWM2, RainMM: o.RainMM, RainMaxMM: b.RainMaxMM, PrecipType: o.PrecipType,
		LightningAvgDistanceKM: o.LightningAvgDistanceKM, LightningStrikeCount: o.LightningStrikeCount,
		BatteryV: o.BatteryV, ReportIntervalMin: o.ReportIntervalMin, LocalDayRainMM: o.LocalDayRainMM,
		LocalDayRainFinalMM: o.LocalDayRainFinalMM,
	}
	if b.AirTempC != nil && b.RelativeHumidity != nil && *b.RelativeHumidity > 0 {
		j.DewPointC = finite(calc.DewPoint(*b.AirTempC, *b.RelativeHumidity))
	}
	if b.StationPressureMB != nil && st != nil {
		j.SeaLevelPressureMB = finite(calc.SeaLevelPressure(*b.StationPressureMB, st.ElevationM))
	}
	return j
}

// scrubNonFinite nils every NaN/Inf *float64 field. The client validates
// values on the way in; this keeps the response encodable whatever reaches
// the cache.
func scrubNonFinite(o *tempest.Observation) {
	v := reflect.ValueOf(o).Elem()
	for i := 0; i < v.NumField(); i++ {
		f := v.Field(i)
		if p, ok := f.Interface().(*float64); ok && p != nil {
			f.Set(reflect.ValueOf(finite(*p)))
		}
	}
}

// finite returns &v, or nil for NaN/Inf: encoding/json refuses non-finite
// floats, and one would fail the whole /api/data response.
func finite(v float64) *float64 {
	if math.IsNaN(v) || math.IsInf(v, 0) {
		return nil
	}
	return &v
}

func buildTempestData(enabled bool, st *tempest.Station, devs []tempest.Device, obs []tempestBucket, latest *tempest.Observation, evs []tempest.Event, cfg config.Config) tempestData {
	td := tempestData{Enabled: enabled, Observations: make([]tempestObsJSON, 0, len(obs)), Events: make([]tempestEventJSON, 0, len(evs))}
	for _, e := range evs {
		td.Events = append(td.Events, tempestEventJSON{Timestamp: e.Time, Type: e.Type, DistanceKM: e.DistanceKM, Energy: e.Energy})
	}
	if st != nil {
		js := &tempestStationJSON{ID: st.ID, Name: st.Name, Room: st.Name, ElevationM: st.ElevationM, Devices: []int{}}
		for _, d := range devs {
			js.Devices = append(js.Devices, d.ID)
		}
		if len(devs) > 0 {
			js.Room = cfg.Room("tempest", strconv.Itoa(devs[0].ID), st.Name)
		}
		td.Station = js
	}
	for _, b := range obs {
		td.Observations = append(td.Observations, tempestJSON(b, st))
	}
	if latest != nil {
		j := tempestJSON(tempestBucket{Observation: *latest, RainMaxMM: latest.RainMM}, st)
		td.Latest = &j
	}
	return td
}

// weatherReading is what the dashboard draws from Open-Meteo. The location
// is deliberately absent: the page is public. Weather code and pressure stay
// in the database and on /metrics.
type weatherReading struct {
	Timestamp       string  `json:"timestamp"`
	TempC           float64 `json:"temp_c"`
	Humidity        float64 `json:"humidity"`
	DewPointC       float64 `json:"dewpoint_c"`
	PrecipitationMM float64 `json:"precipitation_mm"`
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
			TempC:           r.TempC,
			Humidity:        r.Humidity,
			DewPointC:       r.DewPointC,
			PrecipitationMM: r.PrecipitationMM,
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
	Source   string `json:"source"`
	SensorID string `json:"sensor_id"`
	Location string `json:"location"`
	// Adjusted is true when calibration offsets from the config file were
	// applied to temp/humidity/dew point.
	Adjusted bool      `json:"adjusted"`
	Readings []reading `json:"readings"`
}

// reading is what the dashboard draws per sensor. Pressure, PM1, PM10, AQI
// and VOC stay in the database and on /metrics; the page never reads them.
type reading struct {
	Timestamp string  `json:"timestamp"`
	TempC     float64 `json:"temp_c"`
	Humidity  float64 `json:"humidity"`
	DewPointC float64 `json:"dewpoint_c"`
	PM25      float64 `json:"pm25"`
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

// acStateChange is what the AC band draws: when the unit was on. The other
// state fields stay in the database.
type acStateChange struct {
	Timestamp string `json:"timestamp"`
	On        bool   `json:"on"`
}

// groupReadings groups records per sensor, resolving each sensor's room and
// applying calibration offsets from cfg. Stored records are never modified.
func groupReadings(records []logger.Record, latestLocs map[string]string, cfg config.Config) []sensorData {
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
				Location: cfg.Room(r.Source, r.SensorID, loc),
			}
			sensorMap[key] = sensor
			order = append(order, key)
		}
		r, adjusted := cfg.Adjust(r)
		sensor.Adjusted = sensor.Adjusted || adjusted
		sensor.Readings = append(sensor.Readings, reading{
			Timestamp: r.Timestamp.UTC().Format(time.RFC3339),
			TempC:     r.TempC,
			Humidity:  r.Humidity,
			DewPointC: r.DewPointC,
			PM25:      r.PM25,
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

func groupACStates(records []logger.ACStateRecord, latestRooms map[string]string) []acStateData {
	stateMap := make(map[string]*acStateData)
	var order []string
	for _, r := range records {
		state, exists := stateMap[r.DeviceID]
		if !exists {
			room := r.RoomName
			if latest, ok := latestRooms[r.DeviceID]; ok {
				room = latest
			}
			state = &acStateData{
				DeviceID: r.DeviceID,
				Room:     room,
			}
			stateMap[r.DeviceID] = state
			order = append(order, r.DeviceID)
		}
		state.Changes = append(state.Changes, acStateChange{Timestamp: r.Timestamp, On: r.On})
	}
	result := make([]acStateData, 0, len(order))
	for _, key := range order {
		result = append(result, *stateMap[key])
	}
	return result
}

func dataHandler(cache *dataCache, cfg config.Config) http.HandlerFunc {
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
		tempestObs, tempestEvents := cache.snapshotTempest(from, to)
		tempestRows := rawBuckets(tempestObs)

		if s := r.URL.Query().Get("stepSize"); s != "" {
			if ms, err := strconv.ParseInt(s, 10, 64); err == nil && ms > 0 {
				readings = downsampleReadings(readings, from, ms)
				weatherRows = downsampleWeather(weatherRows, from, ms)
				tempestRows = downsampleTempest(tempestObs, from, ms)
			}
		}
		station, devices := cache.tempestStationInfo()

		data := &dashboardData{
			Sensors:     groupReadings(readings, cache.latestLocations(), cfg),
			ACStates:    groupACStates(acRecords, cache.latestACRooms()),
			Weather:     weatherReadings(weatherRows),
			Annotations: make([]annotationJSON, 0, len(annotationRows)),
			Tempest:     buildTempestData(cache.tempestEnabled, station, devices, tempestRows, cache.latestTempest(from, to), tempestEvents, cfg),
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
