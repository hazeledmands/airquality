package metrics

import (
	"math"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/hazel/airquality/calc"
	"github.com/hazel/airquality/tempest"
	"github.com/prometheus/client_golang/prometheus/testutil"
)

func fp(v float64) *float64 { return &v }
func ip(v int) *int         { return &v }

func TestUpdateTempest_SetsEveryField(t *testing.T) {
	m := New()
	ts := time.Unix(1789564860, 0)
	o := tempest.Observation{
		DeviceID: 2002, Time: ts,
		WindLullMS: fp(0.2), WindAvgMS: fp(1.4), WindGustMS: fp(3.1), WindDirectionDeg: fp(270), WindSampleIntervalS: fp(3),
		StationPressureMB: fp(1011.1), AirTempC: fp(15.9), RelativeHumidity: fp(77),
		IlluminanceLux: fp(1610), UVIndex: fp(0.13), SolarRadiationWM2: fp(14),
		RainMM: fp(0.25), PrecipType: ip(1), LightningAvgDistanceKM: fp(12), LightningStrikeCount: ip(3),
		BatteryV: fp(2.61), ReportIntervalMin: ip(1), LocalDayRainMM: fp(0.5),
		RainFinalMM: fp(0.3), LocalDayRainFinalMM: fp(0.6), PrecipAnalysisType: ip(1),
	}
	m.UpdateTempest(o, "Home", "Outdoor", 22)

	l := []string{"tempest", "2002", "Home", "Outdoor"}
	shared := map[string]float64{
		"temperature": 15.9, "humidity": 77, "uv": 0.13,
	}
	got := map[string]float64{
		"temperature": testutil.ToFloat64(m.temperature.WithLabelValues(l...)),
		"humidity":    testutil.ToFloat64(m.humidity.WithLabelValues(l...)),
		"pressure":    testutil.ToFloat64(m.pressure.WithLabelValues(l...)),
		"uv":          testutil.ToFloat64(m.uvindex.WithLabelValues(l...)),
	}
	for k, want := range shared {
		if got[k] != want {
			t.Errorf("%s = %v, want %v", k, got[k], want)
		}
	}
	// The shared pressure gauge holds sea-level pressure, like Open-Meteo's.
	if p := testutil.ToFloat64(m.pressure.WithLabelValues(l...)); math.Abs(p-calc.SeaLevelPressure(1011.1, 22)) > 1e-9 {
		t.Errorf("pressure = %v, want sea-level", p)
	}
	if dp := testutil.ToFloat64(m.dewpoint.WithLabelValues(l...)); math.Abs(dp-calc.DewPoint(15.9, 77)) > 1e-9 {
		t.Errorf("dewpoint = %v", dp)
	}

	want := map[string]float64{
		"airquality_tempest_wind_lull_meters_per_second":      0.2,
		"airquality_tempest_wind_avg_meters_per_second":       1.4,
		"airquality_tempest_wind_gust_meters_per_second":      3.1,
		"airquality_tempest_wind_direction_degrees":           270,
		"airquality_tempest_wind_sample_interval_seconds":     3,
		"airquality_tempest_illuminance_lux":                  1610,
		"airquality_tempest_solar_radiation_watts_per_m2":     14,
		"airquality_tempest_rain_millimeters":                 0.25,
		"airquality_tempest_precipitation_type":               1,
		"airquality_tempest_lightning_avg_distance_km":        12,
		"airquality_tempest_lightning_strikes":                3,
		"airquality_tempest_battery_volts":                    2.61,
		"airquality_tempest_report_interval_minutes":          1,
		"airquality_tempest_rain_local_day_millimeters":       0.5,
		"airquality_tempest_rain_final_millimeters":           0.3,
		"airquality_tempest_rain_local_day_final_millimeters": 0.6,
		"airquality_tempest_precip_analysis_type":             1,
		"airquality_tempest_observation_timestamp_seconds":    1789564860,
	}
	for name, v := range want {
		g, ok := m.tempest[name]
		if !ok {
			t.Errorf("%s: not registered", name)
			continue
		}
		if got := testutil.ToFloat64(g.WithLabelValues(l...)); got != v {
			t.Errorf("%s = %v, want %v", name, got, v)
		}
	}
}

// A null reading must remove the series, not leave the previous value
// standing (or report a fake zero).
func TestUpdateTempest_NullDropsSeries(t *testing.T) {
	m := New()
	m.UpdateTempest(tempest.Observation{DeviceID: 1, Time: time.Now(), LightningAvgDistanceKM: fp(5)}, "Home", "", 0)
	m.UpdateTempest(tempest.Observation{DeviceID: 1, Time: time.Now()}, "Home", "", 0)
	if n := testutil.CollectAndCount(m.tempest["airquality_tempest_lightning_avg_distance_km"]); n != 0 {
		t.Errorf("series = %d, want 0", n)
	}
}

func TestUpdateTempest_NoStrikesNoDistance(t *testing.T) {
	m := New()
	m.UpdateTempest(tempest.Observation{DeviceID: 1, Time: time.Now(), LightningStrikeCount: ip(0), LightningAvgDistanceKM: fp(0)}, "Home", "", 0)
	if n := testutil.CollectAndCount(m.tempest["airquality_tempest_lightning_avg_distance_km"]); n != 0 {
		t.Errorf("series = %d, want 0 when there were no strikes", n)
	}
}

func TestRecordTempestEvent(t *testing.T) {
	m := New()
	ts := time.Unix(1789564900, 0)
	m.RecordTempestEvent(tempest.Event{DeviceID: 2002, Time: ts, Type: tempest.EventLightningStrike, DistanceKM: fp(9), Energy: fp(400)}, "Home", "")
	m.RecordTempestEvent(tempest.Event{DeviceID: 2002, Time: ts.Add(time.Minute), Type: tempest.EventLightningStrike, DistanceKM: fp(7), Energy: fp(100)}, "Home", "")
	m.RecordTempestEvent(tempest.Event{DeviceID: 2002, Time: ts, Type: tempest.EventRainStart}, "Home", "")

	l := []string{"tempest", "2002", "Home", "Home"}
	if v := testutil.ToFloat64(m.strikeEvents.WithLabelValues(l...)); v != 2 {
		t.Errorf("strike events = %v", v)
	}
	if v := testutil.ToFloat64(m.rainStartEvents.WithLabelValues(l...)); v != 1 {
		t.Errorf("rain start events = %v", v)
	}
	if v := testutil.ToFloat64(m.tempest["airquality_tempest_last_strike_distance_km"].WithLabelValues(l...)); v != 7 {
		t.Errorf("last distance = %v", v)
	}
	if v := testutil.ToFloat64(m.tempest["airquality_tempest_last_strike_energy"].WithLabelValues(l...)); v != 100 {
		t.Errorf("last energy = %v", v)
	}
	if v := testutil.ToFloat64(m.tempest["airquality_tempest_last_strike_timestamp_seconds"].WithLabelValues(l...)); v != float64(ts.Add(time.Minute).Unix()) {
		t.Errorf("last strike ts = %v", v)
	}
	if v := testutil.ToFloat64(m.tempest["airquality_tempest_last_rain_start_timestamp_seconds"].WithLabelValues(l...)); v != float64(ts.Unix()) {
		t.Errorf("last rain start ts = %v", v)
	}
}

func TestTempestMetricsServed(t *testing.T) {
	m := New()
	m.UpdateTempest(tempest.Observation{DeviceID: 2002, Time: time.Now(), WindGustMS: fp(4)}, "Home", "", 0)
	w := httptest.NewRecorder()
	m.Handler().ServeHTTP(w, httptest.NewRequest("GET", "/metrics", nil))
	if !strings.Contains(w.Body.String(), `airquality_tempest_wind_gust_meters_per_second{location="Home",room="Home",sensor_id="2002",source="tempest"} 4`) {
		t.Errorf("gust missing from /metrics:\n%s", w.Body.String())
	}
}

// Per-minute gauges can't be summed; the totals count each observation once,
// and only when told it is new.
func TestAddTempestTotals(t *testing.T) {
	m := New()
	l := []string{"tempest", "2002", "Home", "Home"}
	m.AddTempestTotals(tempest.Observation{DeviceID: 2002, RainMM: fp(0.25), LightningStrikeCount: ip(3)}, "Home", "")
	m.AddTempestTotals(tempest.Observation{DeviceID: 2002, RainMM: fp(0.5), LightningStrikeCount: nil}, "Home", "")
	if v := testutil.ToFloat64(m.rainTotal.WithLabelValues(l...)); v != 0.75 {
		t.Errorf("rain total = %v", v)
	}
	if v := testutil.ToFloat64(m.strikesTotal.WithLabelValues(l...)); v != 3 {
		t.Errorf("strikes total = %v", v)
	}
}

func TestSetTempestEnabled(t *testing.T) {
	m := New()
	m.SetTempestEnabled(false)
	if v := testutil.ToFloat64(m.tempestEnabled); v != 0 {
		t.Errorf("enabled = %v", v)
	}
	m.SetTempestEnabled(true)
	if v := testutil.ToFloat64(m.tempestEnabled); v != 1 {
		t.Errorf("enabled = %v", v)
	}
}
