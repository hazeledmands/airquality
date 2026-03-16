package purpleair

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"

	"github.com/hazel/airquality/calc"
)

// Client fetches readings from local PurpleAir sensors.
type Client struct {
	Hosts      []string
	HTTPClient *http.Client
}

// NewClient creates a PurpleAir client for the given local sensor hosts (ip:port or ip).
func NewClient(hosts []string) *Client {
	return &Client{
		Hosts: hosts,
		HTTPClient: &http.Client{
			Timeout: 10 * time.Second,
		},
	}
}

// Fetch retrieves the current reading from a single sensor host.
func (c *Client) Fetch(host string) (SensorReading, error) {
	url := fmt.Sprintf("http://%s/json", host)
	resp, err := c.HTTPClient.Get(url)
	if err != nil {
		return SensorReading{}, fmt.Errorf("fetch %s: %w", host, err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return SensorReading{}, fmt.Errorf("fetch %s: HTTP %d: %s", host, resp.StatusCode, string(body))
	}

	data, err := io.ReadAll(resp.Body)
	if err != nil {
		return SensorReading{}, fmt.Errorf("read %s: %w", host, err)
	}

	var reading SensorReading
	if err := json.Unmarshal(data, &reading); err != nil {
		return SensorReading{}, fmt.Errorf("parse %s: %w", host, err)
	}

	return reading, nil
}

// FetchAll retrieves readings from all configured sensor hosts.
// Returns an error if any sensor fails.
func (c *Client) FetchAll() ([]SensorReading, error) {
	var readings []SensorReading
	for _, host := range c.Hosts {
		r, err := c.Fetch(host)
		if err != nil {
			return nil, err
		}
		readings = append(readings, r)
	}
	return readings, nil
}

// SensorSummary holds computed/converted values for display.
type SensorSummary struct {
	SensorID string
	Place    string

	TempC     float64
	TempF     float64
	Humidity  float64
	DewPointC float64
	DewPointF float64
	Pressure  float64

	// PM2.5 average of channels A and B (CF=1, µg/m³)
	PM1  float64
	PM25 float64
	PM10 float64
	AQI  float64

	// Per-channel PM (CF=1)
	PM25A float64
	PM25B float64

	// Gas/VOC (BME680, kOhm)
	Gas680 float64
}

// Summary computes a display-ready summary from the raw reading,
// including unit conversions and dew point calculation.
func (r *SensorReading) Summary() SensorSummary {
	tempC := calc.FtoC(r.TempF)
	dewC := calc.DewPoint(tempC, r.Humidity)

	return SensorSummary{
		SensorID:  r.SensorID,
		Place:     r.Place,
		TempC:     tempC,
		TempF:     r.TempF,
		Humidity:  r.Humidity,
		DewPointC: dewC,
		DewPointF: calc.CtoF(dewC),
		Pressure:  r.Pressure,
		PM1:       (r.PM1CF1 + r.PM1CF1B) / 2,
		PM25:      (r.PM25CF1 + r.PM25CF1B) / 2,
		PM10:      (r.PM10CF1 + r.PM10CF1B) / 2,
		AQI:       (r.PM25AQI + r.PM25AQIB) / 2,
		PM25A:     r.PM25CF1,
		PM25B:     r.PM25CF1B,
		Gas680:    r.Gas680,
	}
}
