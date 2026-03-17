package logger

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestACStateCSVLogger_WritesHeader(t *testing.T) {
	path := filepath.Join(t.TempDir(), "acstates.csv")
	l, err := NewACStateCSVLogger(path)
	if err != nil {
		t.Fatalf("NewACStateCSVLogger: %v", err)
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

	want := "timestamp,device_id,room,on,mode,target_temp,temp_unit,fan_level,swing,horizontal_swing,changed_properties,reason,sensibo_id"
	if lines[0] != want {
		t.Errorf("header mismatch\n got: %s\nwant: %s", lines[0], want)
	}
}

func TestACStateCSVLogger_WritesRecords(t *testing.T) {
	path := filepath.Join(t.TempDir(), "acstates.csv")
	l, err := NewACStateCSVLogger(path)
	if err != nil {
		t.Fatalf("NewACStateCSVLogger: %v", err)
	}

	err = l.Log(ACStateRecord{
		Timestamp:         "2026-03-16T13:01:02Z",
		DeviceID:          "jFh2Jznm",
		RoomName:          "Office",
		On:                false,
		Mode:              "cool",
		TargetTemp:        70,
		TempUnit:          "F",
		FanLevel:          "quiet",
		Swing:             "fixedMiddle",
		HorizontalSwing:   "fixedCenter",
		ChangedProperties: "on;mode",
		Reason:            "AntiMold",
		SensiboID:         "QRLiZw9jnJ",
	})
	if err != nil {
		t.Fatalf("Log: %v", err)
	}

	err = l.Log(ACStateRecord{
		Timestamp:         "2026-03-16T12:55:48Z",
		DeviceID:          "jFh2Jznm",
		RoomName:          "Office",
		On:                true,
		Mode:              "fan",
		TargetTemp:        70,
		TempUnit:          "F",
		FanLevel:          "low",
		Swing:             "fixedMiddle",
		HorizontalSwing:   "fixedCenter",
		ChangedProperties: "mode",
		Reason:            "UserRequest",
		SensiboID:         "NMmdigja6z",
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

	// Check first record
	fields := strings.Split(lines[1], ",")
	if fields[0] != "2026-03-16T13:01:02Z" {
		t.Errorf("timestamp = %q, want 2026-03-16T13:01:02Z", fields[0])
	}
	if fields[1] != "jFh2Jznm" {
		t.Errorf("device_id = %q, want jFh2Jznm", fields[1])
	}
	if fields[3] != "false" {
		t.Errorf("on = %q, want false", fields[3])
	}
	if fields[12] != "QRLiZw9jnJ" {
		t.Errorf("sensibo_id = %q, want QRLiZw9jnJ", fields[12])
	}

	// Check second record
	fields = strings.Split(lines[2], ",")
	if fields[3] != "true" {
		t.Errorf("on = %q, want true", fields[3])
	}
	if fields[11] != "UserRequest" {
		t.Errorf("reason = %q, want UserRequest", fields[11])
	}
}

func TestLoadExistingACStateIDs(t *testing.T) {
	path := filepath.Join(t.TempDir(), "acstates.csv")
	l, err := NewACStateCSVLogger(path)
	if err != nil {
		t.Fatalf("NewACStateCSVLogger: %v", err)
	}
	l.Log(ACStateRecord{
		Timestamp: "2026-03-16T13:01:02Z",
		DeviceID:  "dev1",
		RoomName:  "Office",
		On:        false,
		Mode:      "cool",
		TargetTemp: 70,
		TempUnit:   "F",
		FanLevel:   "quiet",
		SensiboID:  "id-aaa",
	})
	l.Log(ACStateRecord{
		Timestamp: "2026-03-16T12:55:48Z",
		DeviceID:  "dev1",
		RoomName:  "Office",
		On:        true,
		Mode:      "fan",
		TargetTemp: 70,
		TempUnit:   "F",
		FanLevel:   "low",
		SensiboID:  "id-bbb",
	})
	l.Close()

	ids, err := LoadExistingACStateIDs(path)
	if err != nil {
		t.Fatalf("LoadExistingACStateIDs: %v", err)
	}
	if len(ids) != 2 {
		t.Fatalf("expected 2 IDs, got %d", len(ids))
	}
	if !ids["id-aaa"] {
		t.Error("expected id-aaa to be present")
	}
	if !ids["id-bbb"] {
		t.Error("expected id-bbb to be present")
	}
	if ids["id-ccc"] {
		t.Error("expected id-ccc to be absent")
	}
}

func TestLoadExistingACStateIDs_MissingFile(t *testing.T) {
	ids, err := LoadExistingACStateIDs(filepath.Join(t.TempDir(), "nonexistent.csv"))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(ids) != 0 {
		t.Errorf("expected empty map, got %d entries", len(ids))
	}
}

func TestACStateCSVLogger_AppendsToExistingFile(t *testing.T) {
	path := filepath.Join(t.TempDir(), "acstates.csv")

	// Write one record
	l, err := NewACStateCSVLogger(path)
	if err != nil {
		t.Fatalf("NewACStateCSVLogger: %v", err)
	}
	l.Log(ACStateRecord{
		Timestamp: "2026-03-16T13:01:02Z",
		DeviceID:  "dev1",
		RoomName:  "Office",
		SensiboID: "id-aaa",
	})
	l.Close()

	// Reopen and write another
	l, err = NewACStateCSVLogger(path)
	if err != nil {
		t.Fatalf("NewACStateCSVLogger reopen: %v", err)
	}
	l.Log(ACStateRecord{
		Timestamp: "2026-03-16T14:00:00Z",
		DeviceID:  "dev1",
		RoomName:  "Office",
		SensiboID: "id-bbb",
	})
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
