package logger

import "time"

// Record represents a single measurement from any sensor.
type Record struct {
	Timestamp time.Time
	Source    string
	SensorID  string
	Location  string
	TempC     float64
	Humidity  float64
	DewPointC float64
	Pressure  float64 // purpleair only
	PM1       float64 // purpleair only
	PM25      float64 // purpleair only
	PM10      float64 // purpleair only
	AQI       float64 // purpleair only
	VOC       float64 // purpleair only (gas_680 kOhm)
}
