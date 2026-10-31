package main

import (
	"math"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/hazel/airquality/calc"
	"github.com/hazel/airquality/config"
	"github.com/hazel/airquality/logger"
)

func TestDataHandler_ReturnsJSON(t *testing.T) {
	cache := &dataCache{
		seenReadings: make(map[string]bool),
		seenACStates: make(map[string]bool),
		seenWeather:  make(map[string]bool),
	}
	handler := dataHandler(cache, config.Config{})
	req := httptest.NewRequest("GET", "/api/data", nil)
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)

	if w.Code != 200 {
		t.Fatalf("status = %d, want 200", w.Code)
	}
	if ct := w.Header().Get("Content-Type"); ct != "application/json" {
		t.Errorf("content-type = %q, want application/json", ct)
	}
}

func TestRoomsHandler_ReturnsHTML(t *testing.T) {
	handler := roomsHandler()
	req := httptest.NewRequest("GET", "/", nil)
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)

	if w.Code != 200 {
		t.Fatalf("status = %d, want 200", w.Code)
	}
	body := w.Body.String()
	if len(body) < 100 {
		t.Error("response too short to be the rooms HTML")
	}
}

func TestGroupReadings_AppliesRoomAndOffsets(t *testing.T) {
	cfg := config.Config{Sensors: map[string]config.Sensor{
		"purpleair/pa1": {Room: "Downstairs", Offsets: config.Offsets{TempC: -4, Humidity: 4}},
	}}
	ts := time.Date(2026, 10, 31, 12, 0, 0, 0, time.UTC)
	records := []logger.Record{
		{Timestamp: ts, Source: "sensibo", SensorID: "s1", Location: "Downstairs", TempC: 21, Humidity: 45},
		{Timestamp: ts, Source: "purpleair", SensorID: "pa1", Location: "inside", TempC: 25, Humidity: 40, PM25: 2},
	}
	got := groupReadings(records, map[string]string{}, cfg)
	if len(got) != 2 {
		t.Fatalf("len = %d, want 2", len(got))
	}
	sb, pa := got[0], got[1]
	if sb.Location != "Downstairs" || sb.Adjusted {
		t.Errorf("sensibo = %+v", sb)
	}
	if pa.Location != "Downstairs" {
		t.Errorf("purpleair location = %q, want Downstairs", pa.Location)
	}
	if !pa.Adjusted {
		t.Error("purpleair Adjusted = false, want true")
	}
	r := pa.Readings[0]
	if r.TempC != 21 || r.Humidity != 44 || r.PM25 != 2 {
		t.Errorf("purpleair reading = %+v", r)
	}
	if want := calc.DewPoint(21, 44); math.Abs(r.DewPointC-want) > 1e-9 {
		t.Errorf("dewpoint = %v, want %v", r.DewPointC, want)
	}
}

func TestGroupReadings_RoomOverridesLatestLocation(t *testing.T) {
	cfg := config.Config{Sensors: map[string]config.Sensor{"purpleair/pa1": {Room: "Downstairs"}}}
	records := []logger.Record{{Timestamp: time.Now(), Source: "purpleair", SensorID: "pa1", Location: "inside"}}
	got := groupReadings(records, map[string]string{"purpleair/pa1": "outside"}, cfg)
	if got[0].Location != "Downstairs" {
		t.Errorf("location = %q, want Downstairs", got[0].Location)
	}
}
