package main

import (
	"bytes"
	"compress/gzip"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/hazel/airquality/config"
	"github.com/hazel/airquality/logger"
	"github.com/hazel/airquality/metrics"
	"github.com/hazel/airquality/openmeteo"
	"github.com/hazel/airquality/tempest"
)

func TestGzip_CompressesWhenAccepted(t *testing.T) {
	body := strings.Repeat(`{"x":1},`, 2000)
	h := withGzip(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		io.WriteString(w, body)
	}))

	req := httptest.NewRequest("GET", "/api/data", nil)
	req.Header.Set("Accept-Encoding", "br, gzip, deflate")
	w := httptest.NewRecorder()
	h.ServeHTTP(w, req)
	if w.Header().Get("Content-Encoding") != "gzip" {
		t.Fatalf("Content-Encoding = %q", w.Header().Get("Content-Encoding"))
	}
	if !strings.Contains(w.Header().Get("Vary"), "Accept-Encoding") {
		t.Errorf("Vary = %q", w.Header().Get("Vary"))
	}
	if w.Header().Get("Content-Length") != "" {
		t.Errorf("Content-Length must not describe the uncompressed body")
	}
	zr, err := gzip.NewReader(w.Body)
	if err != nil {
		t.Fatal(err)
	}
	got, _ := io.ReadAll(zr)
	if string(got) != body {
		t.Error("round trip mismatch")
	}
	if w.Body.Len() >= len(body)/4 {
		t.Errorf("compressed %d of %d bytes", w.Body.Len(), len(body))
	}
}

func TestGzip_PlainWhenNotAccepted(t *testing.T) {
	h := withGzip(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { io.WriteString(w, "hello") }))
	for _, ae := range []string{"", "identity", "gzip;q=0"} {
		req := httptest.NewRequest("GET", "/", nil)
		if ae != "" {
			req.Header.Set("Accept-Encoding", ae)
		}
		w := httptest.NewRecorder()
		h.ServeHTTP(w, req)
		if w.Header().Get("Content-Encoding") != "" || w.Body.String() != "hello" {
			t.Errorf("Accept-Encoding %q: encoding %q body %q", ae, w.Header().Get("Content-Encoding"), w.Body.String())
		}
	}
}

// The page never reads these, so they stay in the database and on /metrics
// but not in every /api/data response. The coordinates are also private.
func TestDataHandler_OmitsUnusedAndPrivateFields(t *testing.T) {
	c := newTestCache()
	now := time.Now().UTC().Truncate(time.Minute)
	c.addReading(logger.Record{Timestamp: now.Add(-time.Minute), Source: "purpleair", SensorID: "pa", Location: "Downstairs",
		TempC: 20, Humidity: 40, DewPointC: 6, Pressure: 1012, PM1: 1, PM25: 2, PM10: 3, AQI: 8, VOC: 120, P03: 100})
	c.addWeather(openmeteo.Reading{Time: now.Add(-time.Minute), Latitude: 12.34, Longitude: -56.78, TempC: 10, WindSpeedKMH: 16.09, WeatherCode: 3, PressureHPa: 1010})
	c.addACState(logger.ACStateRecord{Timestamp: now.Add(-time.Minute).Format(time.RFC3339), DeviceID: "d", RoomName: "R", On: true, Mode: "cool", TargetTemp: 22, SensiboID: "x"})
	c.setTempestStation(tempest.Station{ID: 1001, Name: "Home"}, []tempest.Device{{ID: 2002, Type: "ST"}})
	c.addTempest([]tempest.Observation{{DeviceID: 2002, Time: now.Add(-time.Minute), WindSampleIntervalS: fp(3), RainFinalMM: fp(0.1), PrecipAnalysisType: ip(1), AirTempC: fp(9)}})
	c.addTempestEvent(tempest.Event{DeviceID: 2002, Time: now.Add(-time.Minute), Type: tempest.EventLightningStrike, DistanceKM: fp(5)})

	w := httptest.NewRecorder()
	from := now.Add(-time.Hour).UnixMilli()
	dataHandler(c, config.Config{}).ServeHTTP(w, httptest.NewRequest("GET", "/api/data?from="+strconv.FormatInt(from, 10), nil))
	body := w.Body.String()
	for _, field := range []string{
		`"latitude"`, `"longitude"`, "12.34", "-56.78", // private
		`"aqi"`, `"voc"`, `"pm1"`, `"pm10"`, `"pressure"`, `"weather_code"`, `"pressure_hpa"`,
		`"mode"`, `"target_temp"`, `"fan_level"`, `"temp_unit"`, `"reason"`,
		`"device_id":2002`, `"wind_sample_interval_s"`, `"rain_final_mm"`, `"precip_analysis_type"`,
	} {
		if strings.Contains(body, field) {
			t.Errorf("/api/data contains %s", field)
		}
	}
	// What the page does use is still there.
	for _, field := range []string{`"pm25"`, `"p03"`, `"wind_speed_kmh"`, `"air_temp_c"`, `"on":true`, `"distance_km"`} {
		if !strings.Contains(body, field) {
			t.Errorf("/api/data lacks %s", field)
		}
	}
	var parsed map[string]any
	if err := json.Unmarshal(bytes.TrimSpace(w.Body.Bytes()), &parsed); err != nil {
		t.Fatal(err)
	}
}

// The server's routes, not just the middleware: the dashboard HTML and data
// must actually leave compressed.
func TestRoutes_CompressDashboardAndData(t *testing.T) {
	h := routes(metricsForTest(), newTestCache(), config.Config{}, authConfig{}, nil)
	for _, path := range []string{"/", "/api/data"} {
		req := httptest.NewRequest("GET", path, nil)
		req.Header.Set("Accept-Encoding", "gzip")
		w := httptest.NewRecorder()
		h.ServeHTTP(w, req)
		if w.Code != 200 || w.Header().Get("Content-Encoding") != "gzip" {
			t.Errorf("%s: status %d, Content-Encoding %q", path, w.Code, w.Header().Get("Content-Encoding"))
		}
	}
}

func metricsForTest() *metrics.Metrics { return metrics.New() }
