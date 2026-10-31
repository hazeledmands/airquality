package metrics

import (
	"fmt"
	"net/http"

	"github.com/hazel/airquality/logger"
	"github.com/hazel/airquality/openmeteo"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promhttp"
)

// location is what the device reports; room is the resolved room from the
// config file (equal to location when no override is configured).
var sensorLabels = []string{"source", "sensor_id", "location", "room"}

type Metrics struct {
	registry      *prometheus.Registry
	temperature   *prometheus.GaugeVec
	humidity      *prometheus.GaugeVec
	dewpoint      *prometheus.GaugeVec
	pressure      *prometheus.GaugeVec
	pm1           *prometheus.GaugeVec
	pm25          *prometheus.GaugeVec
	pm10          *prometheus.GaugeVec
	aqi           *prometheus.GaugeVec
	voc           *prometheus.GaugeVec
	p03           *prometheus.GaugeVec
	p05           *prometheus.GaugeVec
	p10           *prometheus.GaugeVec
	p25           *prometheus.GaugeVec
	p50           *prometheus.GaugeVec
	p100          *prometheus.GaugeVec
	precipitation *prometheus.GaugeVec
	windspeed     *prometheus.GaugeVec
	uvindex       *prometheus.GaugeVec
	weathercode   *prometheus.GaugeVec
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
		p03: prometheus.NewGaugeVec(prometheus.GaugeOpts{
			Name: "airquality_particles_03_per_dl",
			Help: "Particle count ≥0.3 µm per deciliter (avg A+B channels).",
		}, sensorLabels),
		p05: prometheus.NewGaugeVec(prometheus.GaugeOpts{
			Name: "airquality_particles_05_per_dl",
			Help: "Particle count ≥0.5 µm per deciliter (avg A+B channels).",
		}, sensorLabels),
		p10: prometheus.NewGaugeVec(prometheus.GaugeOpts{
			Name: "airquality_particles_10_per_dl",
			Help: "Particle count ≥1.0 µm per deciliter (avg A+B channels).",
		}, sensorLabels),
		p25: prometheus.NewGaugeVec(prometheus.GaugeOpts{
			Name: "airquality_particles_25_per_dl",
			Help: "Particle count ≥2.5 µm per deciliter (avg A+B channels).",
		}, sensorLabels),
		p50: prometheus.NewGaugeVec(prometheus.GaugeOpts{
			Name: "airquality_particles_50_per_dl",
			Help: "Particle count ≥5.0 µm per deciliter (avg A+B channels).",
		}, sensorLabels),
		p100: prometheus.NewGaugeVec(prometheus.GaugeOpts{
			Name: "airquality_particles_100_per_dl",
			Help: "Particle count ≥10.0 µm per deciliter (avg A+B channels).",
		}, sensorLabels),
		precipitation: prometheus.NewGaugeVec(prometheus.GaugeOpts{
			Name: "airquality_precipitation_mm",
			Help: "Precipitation in millimeters.",
		}, sensorLabels),
		windspeed: prometheus.NewGaugeVec(prometheus.GaugeOpts{
			Name: "airquality_wind_speed_kmh",
			Help: "Wind speed in kilometers per hour.",
		}, sensorLabels),
		uvindex: prometheus.NewGaugeVec(prometheus.GaugeOpts{
			Name: "airquality_uv_index",
			Help: "UV index.",
		}, sensorLabels),
		weathercode: prometheus.NewGaugeVec(prometheus.GaugeOpts{
			Name: "airquality_weather_code",
			Help: "WMO weather interpretation code.",
		}, sensorLabels),
	}

	reg.MustRegister(
		m.temperature, m.humidity, m.dewpoint, m.pressure,
		m.pm1, m.pm25, m.pm10, m.aqi, m.voc,
		m.p03, m.p05, m.p10, m.p25, m.p50, m.p100,
		m.precipitation, m.windspeed, m.uvindex, m.weathercode,
	)
	return m
}

// Update sets gauges from a raw record. room is the resolved room; empty means
// use the record's location. Values are always raw (no calibration offsets).
func (m *Metrics) Update(rec logger.Record, room string) {
	if room == "" {
		room = rec.Location
	}
	l := []string{rec.Source, rec.SensorID, rec.Location, room}
	m.temperature.WithLabelValues(l...).Set(rec.TempC)
	m.humidity.WithLabelValues(l...).Set(rec.Humidity)
	m.dewpoint.WithLabelValues(l...).Set(rec.DewPointC)
	m.pressure.WithLabelValues(l...).Set(rec.Pressure)
	m.pm1.WithLabelValues(l...).Set(rec.PM1)
	m.pm25.WithLabelValues(l...).Set(rec.PM25)
	m.pm10.WithLabelValues(l...).Set(rec.PM10)
	m.aqi.WithLabelValues(l...).Set(rec.AQI)
	m.voc.WithLabelValues(l...).Set(rec.VOC)
	m.p03.WithLabelValues(l...).Set(rec.P03)
	m.p05.WithLabelValues(l...).Set(rec.P05)
	m.p10.WithLabelValues(l...).Set(rec.P10)
	m.p25.WithLabelValues(l...).Set(rec.P25)
	m.p50.WithLabelValues(l...).Set(rec.P50)
	m.p100.WithLabelValues(l...).Set(rec.P100)
}

// UpdateWeather updates metrics from an Open-Meteo weather reading.
// Shared measurements (temp, humidity, dewpoint, pressure) reuse the existing
// gauges with source="weather". Weather-only fields get dedicated gauges.
func (m *Metrics) UpdateWeather(r openmeteo.Reading) {
	sensorID := fmt.Sprintf("%.4f,%.4f", r.Latitude, r.Longitude)
	l := []string{"weather", sensorID, "outdoor", "outdoor"}
	m.temperature.WithLabelValues(l...).Set(r.TempC)
	m.humidity.WithLabelValues(l...).Set(r.Humidity)
	m.dewpoint.WithLabelValues(l...).Set(r.DewPointC)
	m.pressure.WithLabelValues(l...).Set(r.PressureHPa)
	m.precipitation.WithLabelValues(l...).Set(r.PrecipitationMM)
	m.windspeed.WithLabelValues(l...).Set(r.WindSpeedKMH)
	m.uvindex.WithLabelValues(l...).Set(r.UVIndex)
	m.weathercode.WithLabelValues(l...).Set(float64(r.WeatherCode))
}

func (m *Metrics) Handler() http.Handler {
	return promhttp.HandlerFor(m.registry, promhttp.HandlerOpts{})
}
