// Package config loads the optional YAML config file: sensor hosts, weather
// location, Tempest station settings, and per-sensor room assignments and
// calibration offsets.
//
// Secrets (SENSIBO_API_KEY, TEMPEST_TOKEN, DATABASE_URL) are never read from
// here; they stay in the environment. Unknown keys are rejected, so a secret
// pasted into the file fails startup instead of being silently accepted.
package config

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"os"
	"strings"
	"time"

	"gopkg.in/yaml.v3"

	"github.com/hazel/airquality/calc"
	"github.com/hazel/airquality/logger"
)

type Config struct {
	PurpleAir PurpleAir `yaml:"purpleair"`
	Weather   Weather   `yaml:"weather"`
	Tempest   Tempest   `yaml:"tempest"`
	// Sensors is keyed by "source/sensor_id", e.g. "purpleair/a4:e5:7c:28:49:7".
	Sensors map[string]Sensor `yaml:"sensors"`
}

type PurpleAir struct {
	Hosts []string `yaml:"hosts"`
}

type Weather struct {
	Lat *float64 `yaml:"lat"`
	Lon *float64 `yaml:"lon"`
}

// Tempest holds the non-secret WeatherFlow Tempest settings. The station is
// recorded only when TEMPEST_TOKEN is set.
type Tempest struct {
	// StationID picks the station; 0 means the first one the token sees with a
	// Tempest device. TEMPEST_STATION_ID overrides it.
	StationID int `yaml:"station_id"`
	// PollInterval is how often the REST API is asked for new observations,
	// independent of --interval. Default 1m, matching the device's report
	// interval; minimum 30s.
	PollInterval time.Duration `yaml:"poll_interval"`
	// Events turns the WebSocket listener for lightning strikes and rain
	// start on or off. Default on.
	Events *bool `yaml:"events"`
}

// PollEvery returns the REST poll interval, defaulting to one minute.
func (t Tempest) PollEvery() time.Duration {
	if t.PollInterval == 0 {
		return time.Minute
	}
	return t.PollInterval
}

// EventsEnabled reports whether to run the WebSocket event listener.
func (t Tempest) EventsEnabled() bool {
	return t.Events == nil || *t.Events
}

type Sensor struct {
	// Room overrides the location the device reports. Empty keeps it.
	Room    string  `yaml:"room"`
	Offsets Offsets `yaml:"offsets"`
}

// Offsets are added to raw readings at display time. Stored data stays raw.
type Offsets struct {
	TempC    float64 `yaml:"temp_c"`
	Humidity float64 `yaml:"humidity"`
}

// Load reads and validates the config file. An empty path yields the zero
// Config, so running without a config file behaves as before.
func Load(path string) (Config, error) {
	var cfg Config
	if path == "" {
		return cfg, nil
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return cfg, err
	}
	dec := yaml.NewDecoder(bytes.NewReader(data))
	dec.KnownFields(true)
	if err := dec.Decode(&cfg); err != nil && !errors.Is(err, io.EOF) {
		return cfg, fmt.Errorf("%s: %w", path, err)
	}
	if err := cfg.validate(); err != nil {
		return cfg, fmt.Errorf("%s: %w", path, err)
	}
	return cfg, nil
}

func (c Config) validate() error {
	if (c.Weather.Lat == nil) != (c.Weather.Lon == nil) {
		return errors.New("weather: lat and lon must be set together")
	}
	if c.Tempest.PollInterval != 0 && c.Tempest.PollInterval < 30*time.Second {
		return errors.New("tempest: poll_interval must be at least 30s")
	}
	for key := range c.Sensors {
		source, id, ok := strings.Cut(key, "/")
		if !ok || source == "" || id == "" {
			return fmt.Errorf("sensors: key %q must be source/sensor_id (e.g. purpleair/a4:e5:...)", key)
		}
	}
	return nil
}

// Room returns the configured room for a sensor, or location if none is set.
func (c Config) Room(source, sensorID, location string) string {
	if s, ok := c.Sensors[source+"/"+sensorID]; ok && s.Room != "" {
		return s.Room
	}
	return location
}

// Adjust applies the sensor's calibration offsets, recomputing dew point from
// the corrected temperature and humidity. It reports whether anything changed.
func (c Config) Adjust(rec logger.Record) (logger.Record, bool) {
	s, ok := c.Sensors[rec.Source+"/"+rec.SensorID]
	if !ok || s.Offsets == (Offsets{}) {
		return rec, false
	}
	rec.TempC += s.Offsets.TempC
	rec.Humidity = min(max(rec.Humidity+s.Offsets.Humidity, 0), 100)
	if rec.Humidity > 0 {
		rec.DewPointC = calc.DewPoint(rec.TempC, rec.Humidity)
	}
	return rec, true
}
