package logger

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestCSVLogger_WritesHeader(t *testing.T) {
	path := filepath.Join(t.TempDir(), "test.csv")
	l, err := NewCSVLogger(path)
	if err != nil {
		t.Fatalf("NewCSVLogger: %v", err)
	}
	l.Close()

	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("ReadFile: %v", err)
	}

	lines := strings.Split(strings.TrimSpace(string(data)), "\n")
	if len(lines) != 1 {
		t.Fatalf("expected 1 line (header), got %d", len(lines))
	}

	want := "timestamp,source,sensor_id,location,temp_c,humidity,dewpoint_c,pressure,pm1,pm25,pm10,aqi,voc"
	if lines[0] != want {
		t.Errorf("header mismatch\n got: %s\nwant: %s", lines[0], want)
	}
}

func TestCSVLogger_WritesRecords(t *testing.T) {
	path := filepath.Join(t.TempDir(), "test.csv")
	l, err := NewCSVLogger(path)
	if err != nil {
		t.Fatalf("NewCSVLogger: %v", err)
	}

	ts := time.Date(2026, 3, 16, 12, 0, 0, 0, time.UTC)

	err = l.Log(Record{
		Timestamp: ts,
		Source:    "sensibo",
		SensorID:  "abc123",
		Location:  "Office",
		TempC:     22.5,
		Humidity:  45.0,
		DewPointC: 10.2,
	})
	if err != nil {
		t.Fatalf("Log: %v", err)
	}

	err = l.Log(Record{
		Timestamp: ts,
		Source:    "purpleair",
		SensorID:  "pa-001",
		Location:  "Garden",
		TempC:     18.3,
		Humidity:  60.0,
		DewPointC: 10.5,
		Pressure:  1013.2,
		PM1:       5.1,
		PM25:      12.3,
		PM10:      15.0,
		AQI:       51,
		VOC:       120.5,
	})
	if err != nil {
		t.Fatalf("Log: %v", err)
	}

	l.Close()

	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("ReadFile: %v", err)
	}

	lines := strings.Split(strings.TrimSpace(string(data)), "\n")
	if len(lines) != 3 {
		t.Fatalf("expected 3 lines (header + 2 records), got %d", len(lines))
	}

	// Check sensibo record
	fields := strings.Split(lines[1], ",")
	if fields[0] != "2026-03-16T12:00:00Z" {
		t.Errorf("timestamp = %q, want 2026-03-16T12:00:00Z", fields[0])
	}
	if fields[1] != "sensibo" {
		t.Errorf("source = %q, want sensibo", fields[1])
	}
	if fields[2] != "abc123" {
		t.Errorf("sensor_id = %q, want abc123", fields[2])
	}
	if fields[7] != "" {
		t.Errorf("pressure should be empty for sensibo, got %q", fields[7])
	}

	// Check purpleair record
	fields = strings.Split(lines[2], ",")
	if fields[1] != "purpleair" {
		t.Errorf("source = %q, want purpleair", fields[1])
	}
	if fields[7] != "1013.2" {
		t.Errorf("pressure = %q, want 1013.2", fields[7])
	}
	if fields[12] != "120.5" {
		t.Errorf("voc = %q, want 120.5", fields[12])
	}
}

func TestCSVLogger_SkipsDuplicateSameMinute(t *testing.T) {
	path := filepath.Join(t.TempDir(), "test.csv")
	l, err := NewCSVLogger(path)
	if err != nil {
		t.Fatalf("NewCSVLogger: %v", err)
	}

	ts1 := time.Date(2026, 3, 16, 12, 5, 0, 0, time.UTC)
	ts2 := time.Date(2026, 3, 16, 12, 5, 30, 0, time.UTC) // same minute, different second

	l.Log(Record{Timestamp: ts1, Source: "sensibo", SensorID: "abc", Location: "Office", TempC: 22.0, Humidity: 45.0, DewPointC: 10.0})
	l.Log(Record{Timestamp: ts2, Source: "sensibo", SensorID: "abc", Location: "Office", TempC: 22.1, Humidity: 44.0, DewPointC: 9.8})
	l.Close()

	data, _ := os.ReadFile(path)
	lines := strings.Split(strings.TrimSpace(string(data)), "\n")
	if len(lines) != 2 {
		t.Fatalf("expected 2 lines (header + 1 record), got %d: %s", len(lines), string(data))
	}
}

func TestCSVLogger_AllowsDifferentMinute(t *testing.T) {
	path := filepath.Join(t.TempDir(), "test.csv")
	l, err := NewCSVLogger(path)
	if err != nil {
		t.Fatalf("NewCSVLogger: %v", err)
	}

	ts1 := time.Date(2026, 3, 16, 12, 5, 0, 0, time.UTC)
	ts2 := time.Date(2026, 3, 16, 12, 6, 0, 0, time.UTC) // different minute

	l.Log(Record{Timestamp: ts1, Source: "sensibo", SensorID: "abc", Location: "Office", TempC: 22.0, Humidity: 45.0, DewPointC: 10.0})
	l.Log(Record{Timestamp: ts2, Source: "sensibo", SensorID: "abc", Location: "Office", TempC: 22.1, Humidity: 44.0, DewPointC: 9.8})
	l.Close()

	data, _ := os.ReadFile(path)
	lines := strings.Split(strings.TrimSpace(string(data)), "\n")
	if len(lines) != 3 {
		t.Fatalf("expected 3 lines (header + 2 records), got %d: %s", len(lines), string(data))
	}
}

func TestCSVLogger_AllowsSameMinuteDifferentSensor(t *testing.T) {
	path := filepath.Join(t.TempDir(), "test.csv")
	l, err := NewCSVLogger(path)
	if err != nil {
		t.Fatalf("NewCSVLogger: %v", err)
	}

	ts := time.Date(2026, 3, 16, 12, 5, 0, 0, time.UTC)

	l.Log(Record{Timestamp: ts, Source: "sensibo", SensorID: "abc", Location: "Office", TempC: 22.0, Humidity: 45.0, DewPointC: 10.0})
	l.Log(Record{Timestamp: ts, Source: "sensibo", SensorID: "def", Location: "Bedroom", TempC: 23.0, Humidity: 50.0, DewPointC: 12.0})
	l.Close()

	data, _ := os.ReadFile(path)
	lines := strings.Split(strings.TrimSpace(string(data)), "\n")
	if len(lines) != 3 {
		t.Fatalf("expected 3 lines (header + 2 records), got %d: %s", len(lines), string(data))
	}
}

func TestCSVLogger_LoadsExistingForDedup(t *testing.T) {
	path := filepath.Join(t.TempDir(), "test.csv")

	// Write one record
	l, err := NewCSVLogger(path)
	if err != nil {
		t.Fatalf("NewCSVLogger: %v", err)
	}
	ts := time.Date(2026, 3, 16, 12, 5, 0, 0, time.UTC)
	l.Log(Record{Timestamp: ts, Source: "sensibo", SensorID: "abc", Location: "Office", TempC: 22.0, Humidity: 45.0, DewPointC: 10.0})
	l.Close()

	// Reopen — should load existing record and skip duplicate
	l, err = NewCSVLogger(path)
	if err != nil {
		t.Fatalf("NewCSVLogger reopen: %v", err)
	}
	ts2 := time.Date(2026, 3, 16, 12, 5, 45, 0, time.UTC) // same minute
	l.Log(Record{Timestamp: ts2, Source: "sensibo", SensorID: "abc", Location: "Office", TempC: 22.1, Humidity: 44.0, DewPointC: 9.8})
	l.Close()

	data, _ := os.ReadFile(path)
	lines := strings.Split(strings.TrimSpace(string(data)), "\n")
	if len(lines) != 2 {
		t.Fatalf("expected 2 lines (header + 1 record, dup skipped), got %d: %s", len(lines), string(data))
	}
}

func TestCSVLogger_AppendsToExistingFile(t *testing.T) {
	path := filepath.Join(t.TempDir(), "test.csv")

	// Write one record
	l, err := NewCSVLogger(path)
	if err != nil {
		t.Fatalf("NewCSVLogger: %v", err)
	}
	ts := time.Date(2026, 3, 16, 12, 0, 0, 0, time.UTC)
	l.Log(Record{Timestamp: ts, Source: "sensibo", SensorID: "abc", Location: "Office", TempC: 22.0, Humidity: 45.0, DewPointC: 10.0})
	l.Close()

	// Reopen and write another
	l, err = NewCSVLogger(path)
	if err != nil {
		t.Fatalf("NewCSVLogger reopen: %v", err)
	}
	ts2 := time.Date(2026, 3, 16, 12, 1, 0, 0, time.UTC)
	l.Log(Record{Timestamp: ts2, Source: "sensibo", SensorID: "abc", Location: "Office", TempC: 22.1, Humidity: 44.0, DewPointC: 9.8})
	l.Close()

	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("ReadFile: %v", err)
	}

	lines := strings.Split(strings.TrimSpace(string(data)), "\n")
	if len(lines) != 3 {
		t.Fatalf("expected 3 lines (header + 2 records), got %d: %s", len(lines), string(data))
	}
}
