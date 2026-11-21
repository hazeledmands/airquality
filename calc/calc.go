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

// SeaLevelPressure reduces station pressure (hPa) at elevation z (m) to sea
// level, with WeatherFlow's formula
// (https://weatherflow.github.io/Tempest/api/derived-metric-formulas.html).
func SeaLevelPressure(stationHPa, z float64) float64 {
	const (
		p0     = 1013.25 // standard sea-level pressure, hPa
		rd     = 287.05  // gas constant for dry air, J/(kg·K)
		gammaS = 0.0065  // standard lapse rate, K/m
		g      = 9.80665 // gravity, m/s²
		t0     = 288.15  // standard sea-level temperature, K
	)
	k := rd * gammaS / g
	return stationHPa * math.Pow(1+math.Pow(p0/stationHPa, k)*(gammaS*z/t0), 1/k)
}

// HPaToInHg converts hectopascals to inches of mercury.
func HPaToInHg(hpa float64) float64 {
	return hpa / 33.8639
}
