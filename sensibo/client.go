package sensibo

import (
	"encoding/json"
	"fmt"
	"io"
	"math"
	"net/http"
	"time"
)

var baseURL = "https://home.sensibo.com/api/v2"

// setBaseURL overrides the API base URL (used in tests).
func setBaseURL(url string) { baseURL = url }

// Client communicates with the Sensibo API.
type Client struct {
	APIKey     string
	HTTPClient *http.Client
}

// NewClient creates a Sensibo API client.
func NewClient(apiKey string) *Client {
	return &Client{
		APIKey: apiKey,
		HTTPClient: &http.Client{
			Timeout: 30 * time.Second,
		},
	}
}

func (c *Client) get(path string) ([]byte, error) {
	url := fmt.Sprintf("%s%s&apiKey=%s", baseURL, path, c.APIKey)
	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return nil, err
	}
	// Go's default transport handles Accept-Encoding: gzip automatically.

	resp, err := c.HTTPClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("API error %d: %s", resp.StatusCode, string(body))
	}

	return io.ReadAll(resp.Body)
}

// GetDevices returns all devices associated with the account.
func (c *Client) GetDevices() ([]Device, error) {
	data, err := c.get("/users/me/pods?fields=id,room")
	if err != nil {
		return nil, fmt.Errorf("get devices: %w", err)
	}

	var resp devicesResponse
	if err := json.Unmarshal(data, &resp); err != nil {
		return nil, fmt.Errorf("parse devices: %w", err)
	}
	return resp.Result, nil
}

// GetHistoricalMeasurements returns sensor data for a device over the given number of days.
func (c *Client) GetHistoricalMeasurements(deviceID string, days int) (HistoricalMeasurements, error) {
	path := fmt.Sprintf("/pods/%s/historicalMeasurements?days=%d", deviceID, days)
	data, err := c.get(path)
	if err != nil {
		return HistoricalMeasurements{}, fmt.Errorf("get measurements for %s: %w", deviceID, err)
	}

	var resp measurementsResponse
	if err := json.Unmarshal(data, &resp); err != nil {
		return HistoricalMeasurements{}, fmt.Errorf("parse measurements for %s: %w", deviceID, err)
	}

	return resp.Result, nil
}

// Summarize computes aggregate stats from historical measurements.
func Summarize(deviceID, roomName string, m HistoricalMeasurements) DeviceSummary {
	summary := DeviceSummary{
		DeviceID:   deviceID,
		RoomName:   roomName,
		DataPoints: len(m.Temperature),
		MinTemp:    math.MaxFloat64,
		MinHumid:   math.MaxFloat64,
		MaxTemp:    -math.MaxFloat64,
		MaxHumid:   -math.MaxFloat64,
	}

	if len(m.Temperature) == 0 {
		return summary
	}

	var totalTemp float64
	for _, tv := range m.Temperature {
		totalTemp += tv.Value
		if tv.Value < summary.MinTemp {
			summary.MinTemp = tv.Value
		}
		if tv.Value > summary.MaxTemp {
			summary.MaxTemp = tv.Value
		}
	}
	summary.AvgTemp = totalTemp / float64(len(m.Temperature))

	var totalHumid float64
	for _, hv := range m.Humidity {
		totalHumid += hv.Value
		if hv.Value < summary.MinHumid {
			summary.MinHumid = hv.Value
		}
		if hv.Value > summary.MaxHumid {
			summary.MaxHumid = hv.Value
		}
	}
	if len(m.Humidity) > 0 {
		summary.AvgHumid = totalHumid / float64(len(m.Humidity))
	}

	// Latest reading is the last entry (data is chronological).
	latestTemp := m.Temperature[len(m.Temperature)-1]
	if t, err := time.Parse(time.RFC3339, latestTemp.Time); err == nil {
		summary.Latest = ParsedMeasurement{
			Temperature: latestTemp.Value,
			Time:        t,
		}
	}
	if len(m.Humidity) > 0 {
		summary.Latest.Humidity = m.Humidity[len(m.Humidity)-1].Value
	}

	return summary
}
