package main

import (
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

func TestParseTempestClient(t *testing.T) {
	if parseTempestClient("", "123") != nil {
		t.Error("expected nil when token is empty")
	}
	if parseTempestClient("token", "") != nil {
		t.Error("expected nil when stationID is empty")
	}
	if parseTempestClient("", "") != nil {
		t.Error("expected nil when both are empty")
	}
	c := parseTempestClient("my-token", "12345")
	if c == nil {
		t.Fatal("expected client when both args are set")
	}
	if c.Token != "my-token" {
		t.Errorf("Token = %q, want my-token", c.Token)
	}
	if c.StationID != "12345" {
		t.Errorf("StationID = %q, want 12345", c.StationID)
	}
}
