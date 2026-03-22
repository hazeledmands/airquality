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

	data, err := parseCSVData(f.Name(), "")
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

	data, err := parseCSVData(f.Name(), "")
	if err != nil {
		t.Fatal(err)
	}
	if len(data.Sensors) != 0 {
		t.Errorf("got %d sensors, want 0", len(data.Sensors))
	}
}

func TestParseCSVData_MissingFile(t *testing.T) {
	data, err := parseCSVData("/tmp/nonexistent-airquality-test.csv", "")
	if err != nil {
		t.Fatal(err)
	}
	if len(data.Sensors) != 0 {
		t.Errorf("got %d sensors, want 0 for missing file", len(data.Sensors))
	}
}

func TestParseCSVData_IncludesACStates(t *testing.T) {
	airFile, err := os.CreateTemp("", "air-*.csv")
	if err != nil {
		t.Fatal(err)
	}
	defer os.Remove(airFile.Name())

	acFile, err := os.CreateTemp("", "ac-*.csv")
	if err != nil {
		t.Fatal(err)
	}
	defer os.Remove(acFile.Name())

	if _, err := airFile.WriteString("timestamp,source,sensor_id,location,temp_c,humidity,dewpoint_c,pressure,pm1,pm25,pm10,aqi,voc\n"); err != nil {
		t.Fatal(err)
	}
	if _, err := airFile.WriteString("2026-03-16T14:45:00Z,sensibo,abc123,Office,25.2,50.0,13.2,,,,,,\n"); err != nil {
		t.Fatal(err)
	}
	if err := airFile.Close(); err != nil {
		t.Fatal(err)
	}

	if _, err := acFile.WriteString("timestamp,device_id,room,on,mode,target_temp,temp_unit,fan_level,swing,horizontal_swing,changed_properties,reason,sensibo_id\n"); err != nil {
		t.Fatal(err)
	}
	if _, err := acFile.WriteString("2026-03-16T14:40:00Z,abc123,Office,true,cool,70,F,medium,fixedMiddle,fixedCenter,on,UserRequest,id-1\n"); err != nil {
		t.Fatal(err)
	}
	if _, err := acFile.WriteString("2026-03-16T14:50:00Z,abc123,Office,false,cool,70,F,medium,fixedMiddle,fixedCenter,on,UserRequest,id-2\n"); err != nil {
		t.Fatal(err)
	}
	if err := acFile.Close(); err != nil {
		t.Fatal(err)
	}

	data, err := parseCSVData(airFile.Name(), acFile.Name())
	if err != nil {
		t.Fatal(err)
	}

	if len(data.ACStates) != 1 {
		t.Fatalf("got %d AC state series, want 1", len(data.ACStates))
	}
	if data.ACStates[0].Room != "Office" {
		t.Errorf("room = %q, want Office", data.ACStates[0].Room)
	}
	if len(data.ACStates[0].Changes) != 2 {
		t.Fatalf("got %d AC state changes, want 2", len(data.ACStates[0].Changes))
	}
	if !data.ACStates[0].Changes[0].On {
		t.Errorf("first on = %v, want true", data.ACStates[0].Changes[0].On)
	}
	if data.ACStates[0].Changes[1].Mode != "cool" {
		t.Errorf("second mode = %q, want cool", data.ACStates[0].Changes[1].Mode)
	}
}

func TestDataHandler_ReturnsJSON(t *testing.T) {
	airFile, err := os.CreateTemp("", "air-*.csv")
	if err != nil {
		t.Fatal(err)
	}
	defer os.Remove(airFile.Name())

	acFile, err := os.CreateTemp("", "ac-*.csv")
	if err != nil {
		t.Fatal(err)
	}
	defer os.Remove(acFile.Name())

	if _, err := airFile.WriteString("timestamp,source,sensor_id,location,temp_c,humidity,dewpoint_c,pressure,pm1,pm25,pm10,aqi,voc\n"); err != nil {
		t.Fatal(err)
	}
	if _, err := airFile.WriteString("2026-03-16T14:45:00Z,sensibo,abc123,Office,25.2,50.0,13.2,,,,,,\n"); err != nil {
		t.Fatal(err)
	}
	if err := airFile.Close(); err != nil {
		t.Fatal(err)
	}

	if _, err := acFile.WriteString("timestamp,device_id,room,on,mode,target_temp,temp_unit,fan_level,swing,horizontal_swing,changed_properties,reason,sensibo_id\n"); err != nil {
		t.Fatal(err)
	}
	if _, err := acFile.WriteString("2026-03-16T14:40:00Z,abc123,Office,true,cool,70,F,medium,fixedMiddle,fixedCenter,on,UserRequest,id-1\n"); err != nil {
		t.Fatal(err)
	}
	if err := acFile.Close(); err != nil {
		t.Fatal(err)
	}

	handler := dataHandler(airFile.Name(), acFile.Name(), nil)
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
	if len(data.ACStates) != 1 {
		t.Errorf("got %d ac states, want 1", len(data.ACStates))
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
