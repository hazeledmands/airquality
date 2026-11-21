// Package tempest reads a WeatherFlow Tempest station through the WeatherFlow
// cloud: the REST API for one-minute device observations (live and
// historical) and the WebSocket API for events the REST API does not keep
// (individual lightning strikes, rain start).
package tempest

import "time"

// Observation is one obs_st row: everything a Tempest reports per interval
// (normally one minute). Every value is a pointer because the API sends null
// for readings the device did not produce (e.g. lightning distance when there
// were no strikes, wind while the sonic sensor is iced), and null must stay
// distinguishable from zero.
//
// Fields follow the obs_st array order documented at
// https://weatherflow.github.io/Tempest/api/ws.html; indices 18–21 only come
// from the REST API.
type Observation struct {
	DeviceID int       `json:"device_id"`
	Time     time.Time `json:"timestamp"` // [0]

	WindLullMS             *float64 `json:"wind_lull_ms"`           // [1] minimum 3 s sample
	WindAvgMS              *float64 `json:"wind_avg_ms"`            // [2] average over the interval
	WindGustMS             *float64 `json:"wind_gust_ms"`           // [3] maximum 3 s sample
	WindDirectionDeg       *float64 `json:"wind_direction_deg"`     // [4]
	WindSampleIntervalS    *float64 `json:"wind_sample_interval_s"` // [5]
	StationPressureMB      *float64 `json:"station_pressure_mb"`    // [6] not sea-level corrected
	AirTempC               *float64 `json:"air_temp_c"`             // [7]
	RelativeHumidity       *float64 `json:"relative_humidity"`      // [8] %
	IlluminanceLux         *float64 `json:"illuminance_lux"`        // [9]
	UVIndex                *float64 `json:"uv_index"`               // [10]
	SolarRadiationWM2      *float64 `json:"solar_radiation_wm2"`    // [11]
	RainMM                 *float64 `json:"rain_mm"`                // [12] accumulated over the interval
	PrecipType             *int     `json:"precip_type"`            // [13] 0 none, 1 rain, 2 hail, 3 rain+hail
	LightningAvgDistanceKM *float64 `json:"lightning_avg_distance_km"`
	LightningStrikeCount   *int     `json:"lightning_strike_count"`  // [15]
	BatteryV               *float64 `json:"battery_v"`               // [16]
	ReportIntervalMin      *int     `json:"report_interval_min"`     // [17]
	LocalDayRainMM         *float64 `json:"local_day_rain_mm"`       // [18] since station-local midnight
	RainFinalMM            *float64 `json:"rain_final_mm"`           // [19] Rain Check corrected [12]
	LocalDayRainFinalMM    *float64 `json:"local_day_rain_final_mm"` // [20] Rain Check corrected [18]
	PrecipAnalysisType     *int     `json:"precip_analysis_type"`    // [21] 0 none, 1 Rain Check on, 2 Rain Check off
}

// Event kinds stored from the WebSocket API.
const (
	EventLightningStrike = "lightning_strike"
	EventRainStart       = "rain_start"
)

// Event is a single occurrence the station pushes as it happens.
type Event struct {
	DeviceID   int       `json:"device_id"`
	Time       time.Time `json:"timestamp"`
	Type       string    `json:"type"`
	DistanceKM *float64  `json:"distance_km"` // lightning only
	Energy     *float64  `json:"energy"`      // lightning only; unitless
}

// Station is a WeatherFlow station (a location) and its devices.
type Station struct {
	ID         int
	Name       string
	Timezone   string
	ElevationM float64
	Created    time.Time // zero if the API omits created_epoch
	Devices    []Device
}

// Device is one piece of hardware on a station. Type "ST" is a Tempest,
// "HB" a hub.
type Device struct {
	ID       int
	Type     string
	Serial   string
	Firmware string
}
