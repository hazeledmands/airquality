package metrics

import (
	"fmt"
	"strconv"
	"time"

	"github.com/hazel/airquality/openmeteo"
	"github.com/prometheus/client_golang/prometheus"
)

// forecastHorizons are the hours-ahead the forecast gauges report.
var forecastHorizons = []int{1, 3, 6, 12, 24, 48}

func (m *Metrics) registerOpenMeteo() {
	g := func(name, help string, extra ...string) *prometheus.GaugeVec {
		v := prometheus.NewGaugeVec(prometheus.GaugeOpts{Name: name, Help: help}, append(append([]string{}, sensorLabels...), extra...))
		m.registry.MustRegister(v)
		return v
	}
	m.cloudCover = g("airquality_cloud_cover_percent", "Cloud cover (Open-Meteo).")
	m.ozone = g("airquality_ozone_ugm3", "Outdoor ozone (Open-Meteo air quality).")
	m.no2 = g("airquality_no2_ugm3", "Outdoor nitrogen dioxide (Open-Meteo air quality).")
	m.so2 = g("airquality_so2_ugm3", "Outdoor sulphur dioxide (Open-Meteo air quality).")
	m.co = g("airquality_co_ugm3", "Outdoor carbon monoxide (Open-Meteo air quality).")
	m.euAQI = g("airquality_european_aqi", "Outdoor European AQI (Open-Meteo air quality).")
	m.dust = g("airquality_dust_ugm3", "Outdoor dust (Open-Meteo air quality).")
	m.pollen = g("airquality_pollen_grains_m3", "Pollen by type (Open-Meteo air quality; Europe only).", "pollen")
	m.forecastTemp = g("airquality_forecast_temperature_celsius", "Forecast temperature, hours_ahead from now.", "hours_ahead")
	m.forecastPrecipProb = g("airquality_forecast_precip_probability_percent", "Forecast chance of precipitation, hours_ahead from now.", "hours_ahead")
	m.forecastCloud = g("airquality_forecast_cloud_cover_percent", "Forecast cloud cover, hours_ahead from now.", "hours_ahead")
}

func weatherLabels(lat, lon float64) []string {
	return []string{"weather", fmt.Sprintf("%.4f,%.4f", lat, lon), "outdoor", "outdoor"}
}

// UpdateAirQuality sets the outdoor air quality gauges. PM2.5, PM10 and the
// US AQI share the gauges indoor sensors use (source="weather",
// location="outdoor"); a nil value removes its series.
func (m *Metrics) UpdateAirQuality(a openmeteo.AirQuality) {
	l := weatherLabels(a.Latitude, a.Longitude)
	setOrDelete(m.pm25, l, a.PM25)
	setOrDelete(m.pm10, l, a.PM10)
	setOrDelete(m.aqi, l, a.USAQI)
	setOrDelete(m.ozone, l, a.Ozone)
	setOrDelete(m.no2, l, a.NO2)
	setOrDelete(m.so2, l, a.SO2)
	setOrDelete(m.co, l, a.CO)
	setOrDelete(m.euAQI, l, a.EuropeanAQI)
	setOrDelete(m.dust, l, a.Dust)
	for kind, v := range map[string]*float64{
		"alder": a.AlderPollen, "birch": a.BirchPollen, "grass": a.GrassPollen,
		"mugwort": a.MugwortPollen, "olive": a.OlivePollen, "ragweed": a.RagweedPollen,
	} {
		setOrDelete(m.pollen, append(append([]string{}, l...), kind), v)
	}
}

// UpdateForecast sets the forecast gauges for each horizon: the first
// forecast hour starting at or after now + horizon.
func (m *Metrics) UpdateForecast(lat, lon float64, hours []openmeteo.ForecastHour, now time.Time) {
	base := weatherLabels(lat, lon)
	for _, h := range forecastHorizons {
		target := now.Add(time.Duration(h) * time.Hour)
		l := append(append([]string{}, base...), strconv.Itoa(h))
		var hour *openmeteo.ForecastHour
		for i := range hours {
			if !hours[i].Time.Before(target) {
				hour = &hours[i]
				break
			}
		}
		if hour == nil {
			m.forecastTemp.DeleteLabelValues(l...)
			m.forecastPrecipProb.DeleteLabelValues(l...)
			m.forecastCloud.DeleteLabelValues(l...)
			continue
		}
		setOrDelete(m.forecastTemp, l, hour.TempC)
		setOrDelete(m.forecastPrecipProb, l, hour.PrecipProbability)
		setOrDelete(m.forecastCloud, l, hour.CloudCover)
	}
}
