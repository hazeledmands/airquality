package metrics

import (
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/hazel/airquality/logger"
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
}
