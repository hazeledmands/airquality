package main

import (
	"github.com/hazel/airquality/config"
	"testing"
	"time"
)

func TestParseConfig_Defaults(t *testing.T) {
	cfg := parseConfig([]string{})
	if cfg.interval != time.Minute {
		t.Errorf("interval = %v, want 1m", cfg.interval)
	}
	if cfg.verbose {
		t.Error("verbose should default to false")
	}
	if cfg.once {
		t.Error("once should default to false")
	}
	if cfg.port != 50233 {
		t.Errorf("port = %d, want 50233", cfg.port)
	}
}

func TestParseConfig_ShortFlags(t *testing.T) {
	cfg := parseConfig([]string{"-v", "-i", "30s", "-p", "9090"})
	if !cfg.verbose {
		t.Error("verbose should be true")
	}
	if cfg.interval != 30*time.Second {
		t.Errorf("interval = %v, want 30s", cfg.interval)
	}
	if cfg.port != 9090 {
		t.Errorf("port = %d, want 9090", cfg.port)
	}
}

func TestParseConfig_LongFlags(t *testing.T) {
	cfg := parseConfig([]string{"--verbose", "--once", "--interval", "5m", "--port", "8080"})
	if !cfg.verbose {
		t.Error("verbose should be true")
	}
	if !cfg.once {
		t.Error("once should be true")
	}
	if cfg.interval != 5*time.Minute {
		t.Errorf("interval = %v, want 5m", cfg.interval)
	}
	if cfg.port != 8080 {
		t.Errorf("port = %d, want 8080", cfg.port)
	}
}

func TestParseConfig_OnceAlone(t *testing.T) {
	cfg := parseConfig([]string{"--once"})
	if !cfg.once {
		t.Error("once should be true")
	}
	if cfg.verbose {
		t.Error("verbose should be false")
	}
}

func TestParseConfig_ConfigFlag(t *testing.T) {
	cfg := parseConfig([]string{"--config", "/etc/airquality/config.yaml"})
	if cfg.configPath != "/etc/airquality/config.yaml" {
		t.Errorf("configPath = %q", cfg.configPath)
	}
}

func TestResolvePurpleAirHosts_EnvOverridesFile(t *testing.T) {
	file := config.Config{PurpleAir: config.PurpleAir{Hosts: []string{"10.0.0.1"}}}
	if got := resolvePurpleAirHosts("10.0.0.2, 10.0.0.3", file); len(got) != 2 || got[0] != "10.0.0.2" {
		t.Errorf("env set: got %v", got)
	}
	if got := resolvePurpleAirHosts("", file); len(got) != 1 || got[0] != "10.0.0.1" {
		t.Errorf("env unset: got %v", got)
	}
}

func TestResolveWeatherClient(t *testing.T) {
	lat, lon := 1.5, 2.5
	file := config.Config{Weather: config.Weather{Lat: &lat, Lon: &lon}}
	if c := resolveWeatherClient("", "", file); c == nil {
		t.Error("file lat/lon: want client")
	}
	if c := resolveWeatherClient("", "", config.Config{}); c != nil {
		t.Error("nothing set: want nil")
	}
	if c := resolveWeatherClient("3", "4", config.Config{}); c == nil {
		t.Error("env lat/lon: want client")
	}
}
