package config

import (
	"math"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/hazel/airquality/calc"
	"github.com/hazel/airquality/logger"
)

func writeFile(t *testing.T, body string) string {
	t.Helper()
	p := filepath.Join(t.TempDir(), "config.yaml")
	if err := os.WriteFile(p, []byte(body), 0o600); err != nil {
		t.Fatal(err)
	}
	return p
}

func TestLoad_FullFile(t *testing.T) {
	p := writeFile(t, `
purpleair:
  hosts: [192.168.1.218, 192.168.1.219]
weather:
  lat: 37.83
  lon: -122.28
sensors:
  purpleair/a4:e5:7c:28:49:7:
    room: Downstairs
    offsets:
      temp_c: -4.4
      humidity: 4
`)
	cfg, err := Load(p)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if got := strings.Join(cfg.PurpleAir.Hosts, ","); got != "192.168.1.218,192.168.1.219" {
		t.Errorf("hosts = %q", got)
	}
	if cfg.Weather.Lat == nil || *cfg.Weather.Lat != 37.83 {
		t.Errorf("lat = %v", cfg.Weather.Lat)
	}
	if cfg.Weather.Lon == nil || *cfg.Weather.Lon != -122.28 {
		t.Errorf("lon = %v", cfg.Weather.Lon)
	}
	s := cfg.Sensors["purpleair/a4:e5:7c:28:49:7"]
	if s.Room != "Downstairs" || s.Offsets.TempC != -4.4 || s.Offsets.Humidity != 4 {
		t.Errorf("sensor = %+v", s)
	}
}

func TestLoad_EmptyPathIsZeroConfig(t *testing.T) {
	cfg, err := Load("")
	if err != nil {
		t.Fatalf("Load(\"\"): %v", err)
	}
	if len(cfg.Sensors) != 0 || len(cfg.PurpleAir.Hosts) != 0 {
		t.Errorf("want zero config, got %+v", cfg)
	}
}

func TestLoad_UnknownKeyIsError(t *testing.T) {
	p := writeFile(t, "sensors:\n  sensibo/x:\n    rooom: Typo\n")
	if _, err := Load(p); err == nil {
		t.Fatal("want error for unknown key")
	}
}

func TestLoad_UnknownOffsetIsError(t *testing.T) {
	p := writeFile(t, "sensors:\n  sensibo/x:\n    offsets: {pm25: 1}\n")
	if _, err := Load(p); err == nil {
		t.Fatal("want error for unsupported offset field")
	}
}

func TestLoad_MissingFileIsError(t *testing.T) {
	if _, err := Load(filepath.Join(t.TempDir(), "nope.yaml")); err == nil {
		t.Fatal("want error for missing file")
	}
}

func TestLoad_BadSensorKeyIsError(t *testing.T) {
	p := writeFile(t, "sensors:\n  justanid:\n    room: X\n")
	if _, err := Load(p); err == nil {
		t.Fatal("want error for sensor key without source/ prefix")
	}
}

func TestLoad_LatWithoutLonIsError(t *testing.T) {
	p := writeFile(t, "weather:\n  lat: 1\n")
	if _, err := Load(p); err == nil {
		t.Fatal("want error when only one of lat/lon is set")
	}
}

func TestRoom(t *testing.T) {
	cfg := Config{Sensors: map[string]Sensor{
		"purpleair/abc": {Room: "Downstairs"},
		"sensibo/xyz":   {Offsets: Offsets{TempC: 1}},
	}}
	cases := []struct{ source, id, loc, want string }{
		{"purpleair", "abc", "inside", "Downstairs"},
		{"sensibo", "xyz", "Bedroom", "Bedroom"}, // listed but no room: keep location
		{"sensibo", "other", "Office", "Office"}, // not listed
	}
	for _, c := range cases {
		if got := cfg.Room(c.source, c.id, c.loc); got != c.want {
			t.Errorf("Room(%s/%s, %q) = %q, want %q", c.source, c.id, c.loc, got, c.want)
		}
	}
}

func TestAdjust_AppliesOffsetsAndRecomputesDewPoint(t *testing.T) {
	cfg := Config{Sensors: map[string]Sensor{
		"purpleair/abc": {Offsets: Offsets{TempC: -4.4, Humidity: 4}},
	}}
	rec := logger.Record{Source: "purpleair", SensorID: "abc", TempC: 26.4, Humidity: 40, DewPointC: 11.6, PM25: 3}
	got, adjusted := cfg.Adjust(rec)
	if !adjusted {
		t.Fatal("want adjusted = true")
	}
	if math.Abs(got.TempC-22.0) > 1e-9 || got.Humidity != 44 {
		t.Errorf("temp/humidity = %v/%v, want 22/44", got.TempC, got.Humidity)
	}
	if want := calc.DewPoint(22.0, 44); math.Abs(got.DewPointC-want) > 1e-9 {
		t.Errorf("dewpoint = %v, want %v", got.DewPointC, want)
	}
	if got.PM25 != 3 {
		t.Errorf("PM25 changed: %v", got.PM25)
	}
}

func TestAdjust_ClampsHumidity(t *testing.T) {
	cfg := Config{Sensors: map[string]Sensor{
		"purpleair/abc": {Offsets: Offsets{Humidity: 10}},
	}}
	got, _ := cfg.Adjust(logger.Record{Source: "purpleair", SensorID: "abc", TempC: 20, Humidity: 95})
	if got.Humidity != 100 {
		t.Errorf("humidity = %v, want clamped to 100", got.Humidity)
	}
}

func TestAdjust_NoOffsetsIsUnchanged(t *testing.T) {
	cfg := Config{Sensors: map[string]Sensor{"purpleair/abc": {Room: "X"}}}
	rec := logger.Record{Source: "purpleair", SensorID: "abc", TempC: 20, Humidity: 50, DewPointC: 9.3}
	got, adjusted := cfg.Adjust(rec)
	if adjusted || got != rec {
		t.Errorf("want unchanged, got %+v adjusted=%v", got, adjusted)
	}
}
