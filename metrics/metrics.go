package metrics

import (
	"net/http"

	"github.com/hazel/airquality/logger"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promhttp"
)

var sensorLabels = []string{"source", "sensor_id", "location"}

type Metrics struct {
	registry    *prometheus.Registry
	temperature *prometheus.GaugeVec
	humidity    *prometheus.GaugeVec
	dewpoint    *prometheus.GaugeVec
	pressure    *prometheus.GaugeVec
	pm1         *prometheus.GaugeVec
	pm25        *prometheus.GaugeVec
	pm10        *prometheus.GaugeVec
	aqi         *prometheus.GaugeVec
	voc         *prometheus.GaugeVec
}

func New() *Metrics {
	reg := prometheus.NewRegistry()
	m := &Metrics{
		registry: reg,
		temperature: prometheus.NewGaugeVec(prometheus.GaugeOpts{
			Name: "airquality_temperature_celsius",
			Help: "Temperature in degrees Celsius.",
		}, sensorLabels),
		humidity: prometheus.NewGaugeVec(prometheus.GaugeOpts{
			Name: "airquality_humidity_percent",
			Help: "Relative humidity percentage.",
		}, sensorLabels),
		dewpoint: prometheus.NewGaugeVec(prometheus.GaugeOpts{
			Name: "airquality_dewpoint_celsius",
			Help: "Dew point in degrees Celsius.",
		}, sensorLabels),
		pressure: prometheus.NewGaugeVec(prometheus.GaugeOpts{
			Name: "airquality_pressure_mbar",
			Help: "Atmospheric pressure in millibars.",
		}, sensorLabels),
		pm1: prometheus.NewGaugeVec(prometheus.GaugeOpts{
			Name: "airquality_pm1_ugm3",
			Help: "PM1.0 concentration in micrograms per cubic meter.",
		}, sensorLabels),
		pm25: prometheus.NewGaugeVec(prometheus.GaugeOpts{
			Name: "airquality_pm25_ugm3",
			Help: "PM2.5 concentration in micrograms per cubic meter.",
		}, sensorLabels),
		pm10: prometheus.NewGaugeVec(prometheus.GaugeOpts{
			Name: "airquality_pm10_ugm3",
			Help: "PM10 concentration in micrograms per cubic meter.",
		}, sensorLabels),
		aqi: prometheus.NewGaugeVec(prometheus.GaugeOpts{
			Name: "airquality_aqi",
			Help: "Air Quality Index.",
		}, sensorLabels),
		voc: prometheus.NewGaugeVec(prometheus.GaugeOpts{
			Name: "airquality_voc_kohm",
			Help: "VOC gas resistance in kilo-ohms.",
		}, sensorLabels),
	}

	reg.MustRegister(
		m.temperature, m.humidity, m.dewpoint, m.pressure,
		m.pm1, m.pm25, m.pm10, m.aqi, m.voc,
	)
	return m
}

func (m *Metrics) Update(rec logger.Record) {
	l := []string{rec.Source, rec.SensorID, rec.Location}
	m.temperature.WithLabelValues(l...).Set(rec.TempC)
	m.humidity.WithLabelValues(l...).Set(rec.Humidity)
	m.dewpoint.WithLabelValues(l...).Set(rec.DewPointC)
	m.pressure.WithLabelValues(l...).Set(rec.Pressure)
	m.pm1.WithLabelValues(l...).Set(rec.PM1)
	m.pm25.WithLabelValues(l...).Set(rec.PM25)
	m.pm10.WithLabelValues(l...).Set(rec.PM10)
	m.aqi.WithLabelValues(l...).Set(rec.AQI)
	m.voc.WithLabelValues(l...).Set(rec.VOC)
}

func (m *Metrics) Handler() http.Handler {
	return promhttp.HandlerFor(m.registry, promhttp.HandlerOpts{})
}
