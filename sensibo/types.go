package sensibo

import "time"

// Device represents a Sensibo device (pod).
type Device struct {
	ID   string `json:"id"`
	Room Room   `json:"room"`
}

type Room struct {
	Name string `json:"name"`
	Icon string `json:"icon"`
}

// TimedValue is a single data point from the historical measurements API.
type TimedValue struct {
	Time  string  `json:"time"`
	Value float64 `json:"value"`
}

// HistoricalMeasurements holds the separate temperature and humidity series.
type HistoricalMeasurements struct {
	Temperature []TimedValue `json:"temperature"`
	Humidity    []TimedValue `json:"humidity"`
}

// ParsedMeasurement is a combined reading with a parsed Go time.
type ParsedMeasurement struct {
	Temperature float64
	Humidity    float64
	DewPoint    float64
	Time        time.Time
}

// DeviceSummary holds aggregated stats for a device.
type DeviceSummary struct {
	DeviceID   string
	RoomName   string
	Latest     ParsedMeasurement
	AvgTemp    float64
	MinTemp    float64
	MaxTemp    float64
	AvgHumid    float64
	MinHumid    float64
	MaxHumid    float64
	AvgDewPoint float64
	MinDewPoint float64
	MaxDewPoint float64
	DataPoints  int
}

// API response wrappers.

type devicesResponse struct {
	Status string   `json:"status"`
	Result []Device `json:"result"`
}

type measurementsResponse struct {
	Status string                 `json:"status"`
	Result HistoricalMeasurements `json:"result"`
}
