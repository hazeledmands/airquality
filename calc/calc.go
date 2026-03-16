// Package calc provides shared temperature and humidity calculations.
package calc

import "math"

// CtoF converts Celsius to Fahrenheit.
func CtoF(c float64) float64 {
	return c*9.0/5.0 + 32.0
}

// FtoC converts Fahrenheit to Celsius.
func FtoC(f float64) float64 {
	return (f - 32.0) * 5.0 / 9.0
}

// DewPoint calculates the dew point in °C from temperature (°C) and relative
// humidity (%) using the Magnus formula.
func DewPoint(tempC, humidity float64) float64 {
	const (
		b = 17.67
		c = 243.5
	)
	gamma := math.Log(humidity/100.0) + (b*tempC)/(c+tempC)
	return (c * gamma) / (b - gamma)
}
