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
	if cfg.output != "airquality.csv" {
		t.Errorf("output = %q, want %q", cfg.output, "airquality.csv")
	}
	if cfg.verbose {
		t.Error("verbose should default to false")
	}
	if cfg.once {
		t.Error("once should default to false")
	}
}

func TestParseConfig_ShortFlags(t *testing.T) {
	cfg := parseConfig([]string{"-v", "-i", "30s", "-o", "test.csv"})
	if !cfg.verbose {
		t.Error("verbose should be true")
	}
	if cfg.interval != 30*time.Second {
		t.Errorf("interval = %v, want 30s", cfg.interval)
	}
	if cfg.output != "test.csv" {
		t.Errorf("output = %q, want %q", cfg.output, "test.csv")
	}
}

func TestParseConfig_LongFlags(t *testing.T) {
	cfg := parseConfig([]string{"--verbose", "--once", "--interval", "5m", "--output", "data.csv"})
	if !cfg.verbose {
		t.Error("verbose should be true")
	}
	if !cfg.once {
		t.Error("once should be true")
	}
	if cfg.interval != 5*time.Minute {
		t.Errorf("interval = %v, want 5m", cfg.interval)
	}
	if cfg.output != "data.csv" {
		t.Errorf("output = %q, want %q", cfg.output, "data.csv")
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
