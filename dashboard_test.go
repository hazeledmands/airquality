package main

import (
	"encoding/json"
	"net/http/httptest"
	"os"
	"testing"
)

func TestParseCSVData_GroupsBySensor(t *testing.T) {
	f, err := os.CreateTemp("", "test-*.csv")
	if err != nil {
		t.Fatal(err)
	}
	defer os.Remove(f.Name())

	f.WriteString("timestamp,source,sensor_id,location,temp_c,humidity,dewpoint_c,pressure,pm1,pm25,pm10,aqi,voc\n")
	f.WriteString("2026-03-16T14:45:00Z,sensibo,abc123,Office,25.2,50.0,13.2,,,,,,\n")
	f.WriteString("2026-03-16T14:46:00Z,sensibo,abc123,Office,25.3,49.8,13.1,,,,,,\n")
	f.WriteString("2026-03-16T14:45:00Z,purpleair,sensor1,outside,35.0,20.0,8.7,1014.9,1.5,2.5,2.9,10,87.9\n")
	f.Close()

	data, err := parseCSVData(f.Name())
	if err != nil {
		t.Fatal(err)
	}
	if len(data.Sensors) != 2 {
		t.Fatalf("got %d sensors, want 2", len(data.Sensors))
	}

	var office, outside *sensorData
	for i := range data.Sensors {
		switch data.Sensors[i].SensorID {
		case "abc123":
			office = &data.Sensors[i]
		case "sensor1":
			outside = &data.Sensors[i]
		}
	}

	if office == nil {
		t.Fatal("office sensor not found")
	}
	if len(office.Readings) != 2 {
		t.Errorf("office readings = %d, want 2", len(office.Readings))
	}
	if office.Readings[0].TempC != 25.2 {
		t.Errorf("first temp = %v, want 25.2", office.Readings[0].TempC)
	}
	if office.Location != "Office" {
		t.Errorf("location = %q, want Office", office.Location)
	}

	if outside == nil {
		t.Fatal("outside sensor not found")
	}
	if outside.Readings[0].PM25 != 2.5 {
		t.Errorf("pm25 = %v, want 2.5", outside.Readings[0].PM25)
	}
	if outside.Readings[0].Pressure != 1014.9 {
		t.Errorf("pressure = %v, want 1014.9", outside.Readings[0].Pressure)
	}
}

func TestParseCSVData_EmptyFile(t *testing.T) {
	f, err := os.CreateTemp("", "test-*.csv")
	if err != nil {
		t.Fatal(err)
	}
	defer os.Remove(f.Name())
	f.WriteString("timestamp,source,sensor_id,location,temp_c,humidity,dewpoint_c,pressure,pm1,pm25,pm10,aqi,voc\n")
	f.Close()

	data, err := parseCSVData(f.Name())
	if err != nil {
		t.Fatal(err)
	}
	if len(data.Sensors) != 0 {
		t.Errorf("got %d sensors, want 0", len(data.Sensors))
	}
}

func TestParseCSVData_MissingFile(t *testing.T) {
	data, err := parseCSVData("/tmp/nonexistent-airquality-test.csv")
	if err != nil {
		t.Fatal(err)
	}
	if len(data.Sensors) != 0 {
		t.Errorf("got %d sensors, want 0 for missing file", len(data.Sensors))
	}
}

func TestDataHandler_ReturnsJSON(t *testing.T) {
	f, err := os.CreateTemp("", "test-*.csv")
	if err != nil {
		t.Fatal(err)
	}
	defer os.Remove(f.Name())

	f.WriteString("timestamp,source,sensor_id,location,temp_c,humidity,dewpoint_c,pressure,pm1,pm25,pm10,aqi,voc\n")
	f.WriteString("2026-03-16T14:45:00Z,sensibo,abc123,Office,25.2,50.0,13.2,,,,,,\n")
	f.Close()

	handler := dataHandler(f.Name())
	req := httptest.NewRequest("GET", "/api/data", nil)
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)

	if w.Code != 200 {
		t.Fatalf("status = %d, want 200", w.Code)
	}
	if ct := w.Header().Get("Content-Type"); ct != "application/json" {
		t.Errorf("content-type = %q, want application/json", ct)
	}

	var data dashboardData
	if err := json.NewDecoder(w.Body).Decode(&data); err != nil {
		t.Fatalf("invalid json: %v", err)
	}
	if len(data.Sensors) != 1 {
		t.Errorf("got %d sensors, want 1", len(data.Sensors))
	}
}

func TestDashboardHandler_ReturnsHTML(t *testing.T) {
	handler := dashboardHandler()
	req := httptest.NewRequest("GET", "/", nil)
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)

	if w.Code != 200 {
		t.Fatalf("status = %d, want 200", w.Code)
	}
	body := w.Body.String()
	if len(body) < 100 {
		t.Error("response too short to be the dashboard HTML")
	}
}
