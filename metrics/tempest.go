package metrics

import (
	"math"
	"strconv"

	"github.com/hazel/airquality/calc"
	"github.com/hazel/airquality/tempest"
	"github.com/prometheus/client_golang/prometheus"
)

// tempestGauge describes one Tempest-only gauge and where its value comes from.
type tempestGauge struct {
	name, help string
	value      func(tempest.Observation) *float64
}

func intVal(p *int) *float64 {
	if p == nil {
		return nil
	}
	v := float64(*p)
	return &v
}

var tempestGauges = []tempestGauge{
	{"airquality_tempest_wind_lull_meters_per_second", "Tempest minimum 3 s wind sample over the report interval.", func(o tempest.Observation) *float64 { return o.WindLullMS }},
	{"airquality_tempest_wind_avg_meters_per_second", "Tempest average wind speed over the report interval.", func(o tempest.Observation) *float64 { return o.WindAvgMS }},
	{"airquality_tempest_wind_gust_meters_per_second", "Tempest maximum 3 s wind sample over the report interval.", func(o tempest.Observation) *float64 { return o.WindGustMS }},
	{"airquality_tempest_wind_direction_degrees", "Tempest wind direction (degrees from north).", func(o tempest.Observation) *float64 { return o.WindDirectionDeg }},
	{"airquality_tempest_wind_sample_interval_seconds", "Tempest wind sample interval.", func(o tempest.Observation) *float64 { return o.WindSampleIntervalS }},
	{"airquality_tempest_station_pressure_mbar", "Tempest station pressure (not sea-level corrected).", func(o tempest.Observation) *float64 { return o.StationPressureMB }},
	{"airquality_tempest_illuminance_lux", "Tempest illuminance.", func(o tempest.Observation) *float64 { return o.IlluminanceLux }},
	{"airquality_tempest_solar_radiation_watts_per_m2", "Tempest solar radiation.", func(o tempest.Observation) *float64 { return o.SolarRadiationWM2 }},
	{"airquality_tempest_rain_millimeters", "Tempest rain accumulated over the report interval.", func(o tempest.Observation) *float64 { return o.RainMM }},
	{"airquality_tempest_precipitation_type", "Tempest precipitation type: 0 none, 1 rain, 2 hail, 3 rain and hail.", func(o tempest.Observation) *float64 { return intVal(o.PrecipType) }},
	{"airquality_tempest_lightning_avg_distance_km", "Tempest average lightning strike distance over the report interval.", func(o tempest.Observation) *float64 { return o.LightningAvgDistanceKM }},
	{"airquality_tempest_lightning_strikes", "Tempest lightning strikes counted in the report interval.", func(o tempest.Observation) *float64 { return intVal(o.LightningStrikeCount) }},
	{"airquality_tempest_battery_volts", "Tempest battery voltage.", func(o tempest.Observation) *float64 { return o.BatteryV }},
	{"airquality_tempest_report_interval_minutes", "Tempest report interval.", func(o tempest.Observation) *float64 { return intVal(o.ReportIntervalMin) }},
	{"airquality_tempest_rain_local_day_millimeters", "Tempest rain since station-local midnight.", func(o tempest.Observation) *float64 { return o.LocalDayRainMM }},
	{"airquality_tempest_rain_final_millimeters", "Tempest Rain Check corrected rain over the report interval.", func(o tempest.Observation) *float64 { return o.RainFinalMM }},
	{"airquality_tempest_rain_local_day_final_millimeters", "Tempest Rain Check corrected rain since station-local midnight.", func(o tempest.Observation) *float64 { return o.LocalDayRainFinalMM }},
	{"airquality_tempest_precip_analysis_type", "Tempest precipitation analysis: 0 none, 1 Rain Check on, 2 Rain Check off.", func(o tempest.Observation) *float64 { return intVal(o.PrecipAnalysisType) }},
	{"airquality_tempest_observation_timestamp_seconds", "Unix time of the latest Tempest observation.", func(o tempest.Observation) *float64 {
		v := float64(o.Time.Unix())
		return &v
	}},
}

// Gauges set from WebSocket events rather than observations.
var tempestEventGauges = [][2]string{
	{"airquality_tempest_last_strike_distance_km", "Distance of the latest lightning strike."},
	{"airquality_tempest_last_strike_energy", "Energy of the latest lightning strike (unitless)."},
	{"airquality_tempest_last_strike_timestamp_seconds", "Unix time of the latest lightning strike."},
	{"airquality_tempest_last_rain_start_timestamp_seconds", "Unix time of the latest rain start event."},
}

func (m *Metrics) registerTempest() {
	m.tempest = make(map[string]*prometheus.GaugeVec)
	add := func(name, help string) {
		g := prometheus.NewGaugeVec(prometheus.GaugeOpts{Name: name, Help: help}, sensorLabels)
		m.tempest[name] = g
		m.registry.MustRegister(g)
	}
	for _, g := range tempestGauges {
		add(g.name, g.help)
	}
	for _, g := range tempestEventGauges {
		add(g[0], g[1])
	}
	m.strikeEvents = prometheus.NewCounterVec(prometheus.CounterOpts{
		Name: "airquality_tempest_lightning_strike_events_total",
		Help: "Lightning strike events received over the Tempest WebSocket since start.",
	}, sensorLabels)
	m.rainStartEvents = prometheus.NewCounterVec(prometheus.CounterOpts{
		Name: "airquality_tempest_rain_start_events_total",
		Help: "Rain start events received over the Tempest WebSocket since start.",
	}, sensorLabels)
	m.rainTotal = prometheus.NewCounterVec(prometheus.CounterOpts{
		Name: "airquality_tempest_rain_millimeters_total",
		Help: "Rain the Tempest reported in observations recorded since start; use increase() for totals over a range.",
	}, sensorLabels)
	m.strikesTotal = prometheus.NewCounterVec(prometheus.CounterOpts{
		Name: "airquality_tempest_lightning_strikes_total",
		Help: "Lightning strikes the Tempest counted in observations recorded since start.",
	}, sensorLabels)
	m.tempestEnabled = prometheus.NewGauge(prometheus.GaugeOpts{
		Name: "airquality_tempest_enabled",
		Help: "1 when Tempest recording is running, 0 when configured but disabled (e.g. migration not applied).",
	})
	m.registry.MustRegister(m.strikeEvents, m.rainStartEvents, m.rainTotal, m.strikesTotal, m.tempestEnabled)
}

// SetTempestEnabled reports whether Tempest recording is running.
func (m *Metrics) SetTempestEnabled(on bool) {
	if on {
		m.tempestEnabled.Set(1)
	} else {
		m.tempestEnabled.Set(0)
	}
}

// AddTempestTotals adds an observation's rain and strikes to the running
// totals. Call it once per observation, only for ones not counted before
// (not for rechecked history).
func (m *Metrics) AddTempestTotals(o tempest.Observation, station, room string) {
	l := tempestLabels(o.DeviceID, station, room)
	if o.RainMM != nil {
		m.rainTotal.WithLabelValues(l...).Add(*o.RainMM)
	}
	if o.LightningStrikeCount != nil {
		m.strikesTotal.WithLabelValues(l...).Add(float64(*o.LightningStrikeCount))
	}
}

func tempestLabels(deviceID int, station, room string) []string {
	if room == "" {
		room = station
	}
	return []string{"tempest", strconv.Itoa(deviceID), station, room}
}

func setOrDelete(g *prometheus.GaugeVec, l []string, v *float64) {
	if v == nil {
		g.DeleteLabelValues(l...)
		return
	}
	g.WithLabelValues(l...).Set(*v)
}

// UpdateTempest sets gauges from the latest observation. Temperature,
// humidity, dew point, sea-level pressure (from station pressure and
// elevationM, comparable to Open-Meteo's) and UV share the gauges other
// sources use (source="tempest"); the rest are Tempest-only. A null value
// removes its series rather than leaving a stale one. room empty means the
// station name.
func (m *Metrics) UpdateTempest(o tempest.Observation, station, room string, elevationM float64) {
	l := tempestLabels(o.DeviceID, station, room)
	setOrDelete(m.temperature, l, o.AirTempC)
	setOrDelete(m.humidity, l, o.RelativeHumidity)
	var slp *float64
	if o.StationPressureMB != nil {
		if v := calc.SeaLevelPressure(*o.StationPressureMB, elevationM); !math.IsNaN(v) && !math.IsInf(v, 0) {
			slp = &v
		}
	}
	setOrDelete(m.pressure, l, slp)
	setOrDelete(m.uvindex, l, o.UVIndex)
	var dew *float64
	if o.AirTempC != nil && o.RelativeHumidity != nil && *o.RelativeHumidity > 0 {
		v := calc.DewPoint(*o.AirTempC, *o.RelativeHumidity)
		dew = &v
	}
	setOrDelete(m.dewpoint, l, dew)
	for _, g := range tempestGauges {
		setOrDelete(m.tempest[g.name], l, g.value(o))
	}
	// The API reports distance 0 for minutes without strikes; a 0 km gauge
	// would read as a strike overhead.
	if o.LightningStrikeCount != nil && *o.LightningStrikeCount == 0 {
		m.tempest["airquality_tempest_lightning_avg_distance_km"].DeleteLabelValues(l...)
	}
}

// RecordTempestEvent counts a WebSocket event and records its details.
func (m *Metrics) RecordTempestEvent(e tempest.Event, station, room string) {
	l := tempestLabels(e.DeviceID, station, room)
	ts := float64(e.Time.Unix())
	switch e.Type {
	case tempest.EventLightningStrike:
		m.strikeEvents.WithLabelValues(l...).Inc()
		m.tempest["airquality_tempest_last_strike_timestamp_seconds"].WithLabelValues(l...).Set(ts)
		setOrDelete(m.tempest["airquality_tempest_last_strike_distance_km"], l, e.DistanceKM)
		setOrDelete(m.tempest["airquality_tempest_last_strike_energy"], l, e.Energy)
	case tempest.EventRainStart:
		m.rainStartEvents.WithLabelValues(l...).Inc()
		m.tempest["airquality_tempest_last_rain_start_timestamp_seconds"].WithLabelValues(l...).Set(ts)
	}
}
