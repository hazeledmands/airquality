package metrics

import (
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/hazel/airquality/logger"
	"github.com/hazel/airquality/openmeteo"
	"github.com/prometheus/client_golang/prometheus/testutil"
)

func TestUpdate_SensiboRecord(t *testing.T) {
	m := New()
	rec := logger.Record{
		Timestamp: time.Now(),
		Source:    "sensibo",
		SensorID:  "abc123",
		Location:  "Office",
		TempC:     25.2,
		Humidity:  50.0,
		DewPointC: 13.2,
	}
	m.Update(rec)

	labels := []string{"sensibo", "abc123", "Office"}
	if v := testutil.ToFloat64(m.temperature.WithLabelValues(labels...)); v != 25.2 {
		t.Errorf("temperature = %v, want 25.2", v)
	}
	if v := testutil.ToFloat64(m.humidity.WithLabelValues(labels...)); v != 50.0 {
		t.Errorf("humidity = %v, want 50.0", v)
	}
	if v := testutil.ToFloat64(m.dewpoint.WithLabelValues(labels...)); v != 13.2 {
		t.Errorf("dewpoint = %v, want 13.2", v)
	}
}

func TestUpdate_PurpleAirRecord(t *testing.T) {
	m := New()
	rec := logger.Record{
		Timestamp: time.Now(),
		Source:    "purpleair",
		SensorID:  "sensor1",
		Location:  "outside",
		TempC:     35.0,
		Humidity:  20.0,
		DewPointC: 8.7,
		Pressure:  1014.9,
		PM1:       1.5,
		PM25:      2.5,
		PM10:      2.9,
		AQI:       10,
		VOC:       87.9,
		P03:       1234.5,
		P05:       567.8,
		P10:       89.1,
		P25:       12.3,
		P50:       4.5,
		P100:      0.6,
	}
	m.Update(rec)

	labels := []string{"purpleair", "sensor1", "outside"}
	if v := testutil.ToFloat64(m.temperature.WithLabelValues(labels...)); v != 35.0 {
		t.Errorf("temperature = %v, want 35.0", v)
	}
	if v := testutil.ToFloat64(m.pressure.WithLabelValues(labels...)); v != 1014.9 {
		t.Errorf("pressure = %v, want 1014.9", v)
	}
	if v := testutil.ToFloat64(m.pm25.WithLabelValues(labels...)); v != 2.5 {
		t.Errorf("pm25 = %v, want 2.5", v)
	}
	if v := testutil.ToFloat64(m.aqi.WithLabelValues(labels...)); v != 10 {
		t.Errorf("aqi = %v, want 10", v)
	}
	if v := testutil.ToFloat64(m.voc.WithLabelValues(labels...)); v != 87.9 {
		t.Errorf("voc = %v, want 87.9", v)
	}
	if v := testutil.ToFloat64(m.p03.WithLabelValues(labels...)); v != 1234.5 {
		t.Errorf("p03 = %v, want 1234.5", v)
	}
	if v := testutil.ToFloat64(m.p05.WithLabelValues(labels...)); v != 567.8 {
		t.Errorf("p05 = %v, want 567.8", v)
	}
	if v := testutil.ToFloat64(m.p10.WithLabelValues(labels...)); v != 89.1 {
		t.Errorf("p10 = %v, want 89.1", v)
	}
	if v := testutil.ToFloat64(m.p25.WithLabelValues(labels...)); v != 12.3 {
		t.Errorf("p25 = %v, want 12.3", v)
	}
	if v := testutil.ToFloat64(m.p50.WithLabelValues(labels...)); v != 4.5 {
		t.Errorf("p50 = %v, want 4.5", v)
	}
	if v := testutil.ToFloat64(m.p100.WithLabelValues(labels...)); v != 0.6 {
		t.Errorf("p100 = %v, want 0.6", v)
	}
}

func TestUpdateWeather(t *testing.T) {
	m := New()
	r := openmeteo.Reading{
		Latitude:        1.3521,
		Longitude:       103.8198,
		TempC:           28.5,
		Humidity:        75.0,
		DewPointC:       23.4,
		PressureHPa:     1011.2,
		PrecipitationMM: 2.4,
		WindSpeedKMH:    15.0,
		UVIndex:         6.5,
		WeatherCode:     61,
	}
	m.UpdateWeather(r)

	labels := []string{"weather", "1.3521,103.8198", "outdoor"}
	if v := testutil.ToFloat64(m.temperature.WithLabelValues(labels...)); v != 28.5 {
		t.Errorf("temperature = %v, want 28.5", v)
	}
	if v := testutil.ToFloat64(m.humidity.WithLabelValues(labels...)); v != 75.0 {
		t.Errorf("humidity = %v, want 75.0", v)
	}
	if v := testutil.ToFloat64(m.dewpoint.WithLabelValues(labels...)); v != 23.4 {
		t.Errorf("dewpoint = %v, want 23.4", v)
	}
	if v := testutil.ToFloat64(m.pressure.WithLabelValues(labels...)); v != 1011.2 {
		t.Errorf("pressure = %v, want 1011.2", v)
	}
	if v := testutil.ToFloat64(m.precipitation.WithLabelValues(labels...)); v != 2.4 {
		t.Errorf("precipitation = %v, want 2.4", v)
	}
	if v := testutil.ToFloat64(m.windspeed.WithLabelValues(labels...)); v != 15.0 {
		t.Errorf("windspeed = %v, want 15.0", v)
	}
	if v := testutil.ToFloat64(m.uvindex.WithLabelValues(labels...)); v != 6.5 {
		t.Errorf("uvindex = %v, want 6.5", v)
	}
	if v := testutil.ToFloat64(m.weathercode.WithLabelValues(labels...)); v != 61 {
		t.Errorf("weathercode = %v, want 61", v)
	}
}

func TestUpdate_OverwritesPreviousValue(t *testing.T) {
	m := New()
	labels := []string{"sensibo", "abc123", "Office"}

	m.Update(logger.Record{
		Source: "sensibo", SensorID: "abc123", Location: "Office",
		TempC: 20.0,
	})
	m.Update(logger.Record{
		Source: "sensibo", SensorID: "abc123", Location: "Office",
		TempC: 25.0,
	})

	if v := testutil.ToFloat64(m.temperature.WithLabelValues(labels...)); v != 25.0 {
		t.Errorf("temperature = %v, want 25.0 (latest)", v)
	}
}

func TestHandler_ServesPrometheusFormat(t *testing.T) {
	m := New()
	m.Update(logger.Record{
		Timestamp: time.Now(),
		Source:    "sensibo",
		SensorID:  "abc123",
		Location:  "Office",
		TempC:     25.2,
		Humidity:  50.0,
		DewPointC: 13.2,
	})

	req := httptest.NewRequest("GET", "/metrics", nil)
	w := httptest.NewRecorder()
	m.Handler().ServeHTTP(w, req)

	if w.Code != 200 {
		t.Fatalf("status = %d, want 200", w.Code)
	}
	body := w.Body.String()
	if !strings.Contains(body, "airquality_temperature_celsius") {
		t.Error("response should contain airquality_temperature_celsius")
	}
	if !strings.Contains(body, `sensor_id="abc123"`) {
		t.Error("response should contain sensor_id label")
	}
	if !strings.Contains(body, `location="Office"`) {
		t.Error("response should contain location label")
	}
	if !strings.Contains(body, "25.2") {
		t.Error("response should contain temperature value")
	}
	if !strings.Contains(body, "airquality_particles_03_per_dl") {
		t.Error("response should contain airquality_particles_03_per_dl metric definition")
	}
}
