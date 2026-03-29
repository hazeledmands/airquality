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

	// Particle count bins (purpleair only, avg A+B, per deciliter)
	P03  float64 // ≥0.3 µm
	P05  float64 // ≥0.5 µm
	P10  float64 // ≥1.0 µm
	P25  float64 // ≥2.5 µm
	P50  float64 // ≥5.0 µm
	P100 float64 // ≥10.0 µm
}
