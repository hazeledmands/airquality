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

// ACState represents the state of an AC unit at a point in time.
type ACState struct {
	On                bool   `json:"on"`
	Mode              string `json:"mode"`
	TargetTemperature int    `json:"targetTemperature"`
	TemperatureUnit   string `json:"temperatureUnit"`
	FanLevel          string `json:"fanLevel"`
	Swing             string `json:"swing"`
	HorizontalSwing   string `json:"horizontalSwing"`
}

// ACStateChange is a single state change event from the API.
type ACStateChange struct {
	ID                string            `json:"id"`
	Time              ACStateTime       `json:"time"`
	Status            string            `json:"status"`
	ACState           ACState           `json:"acState"`
	ChangedProperties []string          `json:"changedProperties"`
	Reason            string            `json:"reason"`
}

// ACStateTime wraps the nested time object from the API.
type ACStateTime struct {
	Time       string `json:"time"`
	SecondsAgo int    `json:"secondsAgo"`
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

type acStatesResponse struct {
	Status    string          `json:"status"`
	Result    []ACStateChange `json:"result"`
	IsPartial bool            `json:"isPartial"`
}
