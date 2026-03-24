package sensibo

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"math"
	"net/http"
	"strconv"
	"time"

	"github.com/hazel/airquality/calc"
)

const maxRetries = 3

var baseURL = "https://home.sensibo.com/api/v2"

// setBaseURL overrides the API base URL (used in tests).
func setBaseURL(url string) { baseURL = url }

// Client communicates with the Sensibo API.
type Client struct {
	APIKey     string
	HTTPClient *http.Client
	// retryDelay is the base delay for exponential backoff (default 1s; overridable in tests).
	retryDelay time.Duration
}

// NewClient creates a Sensibo API client.
func NewClient(apiKey string) *Client {
	return &Client{
		APIKey: apiKey,
		HTTPClient: &http.Client{
			Timeout: 30 * time.Second,
		},
		retryDelay: time.Second,
	}
}

func (c *Client) get(ctx context.Context, path string) ([]byte, error) {
	url := fmt.Sprintf("%s%s&apiKey=%s", baseURL, path, c.APIKey)

	var lastErr error
	for attempt := 0; attempt <= maxRetries; attempt++ {
		if attempt > 0 {
			delay := c.retryDelay * time.Duration(1<<(attempt-1))
			select {
			case <-time.After(delay):
			case <-ctx.Done():
				return nil, ctx.Err()
			}
		}

		req, err := http.NewRequestWithContext(ctx, "GET", url, nil)
		if err != nil {
			return nil, err
		}
		// Go's default transport handles Accept-Encoding: gzip automatically.

		resp, err := c.HTTPClient.Do(req)
		if err != nil {
			return nil, err
		}

		if resp.StatusCode == http.StatusTooManyRequests {
			body, _ := io.ReadAll(resp.Body)
			resp.Body.Close()
			lastErr = fmt.Errorf("API error %d: %s", resp.StatusCode, string(body))

			// Honor Retry-After header if present.
			if ra := resp.Header.Get("Retry-After"); ra != "" {
				if secs, err := strconv.Atoi(ra); err == nil {
					select {
					case <-time.After(time.Duration(secs) * time.Second):
					case <-ctx.Done():
						return nil, ctx.Err()
					}
					attempt-- // don't count this against the retry budget
					if attempt < -maxRetries {
						break // safety valve
					}
					continue
				}
			}
			continue
		}

		if resp.StatusCode != http.StatusOK {
			body, _ := io.ReadAll(resp.Body)
			resp.Body.Close()
			return nil, fmt.Errorf("API error %d: %s", resp.StatusCode, string(body))
		}

		defer resp.Body.Close()
		return io.ReadAll(resp.Body)
	}

	return nil, fmt.Errorf("API rate limited after %d retries: %w", maxRetries, lastErr)
}

// GetDevices returns all devices associated with the account.
func (c *Client) GetDevices(ctx context.Context) ([]Device, error) {
	data, err := c.get(ctx, "/users/me/pods?fields=id,room")
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
func (c *Client) GetHistoricalMeasurements(ctx context.Context, deviceID string, days int) (HistoricalMeasurements, error) {
	path := fmt.Sprintf("/pods/%s/historicalMeasurements?days=%d", deviceID, days)
	data, err := c.get(ctx, path)
	if err != nil {
		return HistoricalMeasurements{}, fmt.Errorf("get measurements for %s: %w", deviceID, err)
	}

	var resp measurementsResponse
	if err := json.Unmarshal(data, &resp); err != nil {
		return HistoricalMeasurements{}, fmt.Errorf("parse measurements for %s: %w", deviceID, err)
	}

	return resp.Result, nil
}

// GetACStates returns recent AC state changes for a device.
func (c *Client) GetACStates(ctx context.Context, deviceID string, limit int) ([]ACStateChange, error) {
	path := fmt.Sprintf("/pods/%s/acStates?limit=%d", deviceID, limit)
	data, err := c.get(ctx, path)
	if err != nil {
		return nil, fmt.Errorf("get ac states for %s: %w", deviceID, err)
	}

	var resp acStatesResponse
	if err := json.Unmarshal(data, &resp); err != nil {
		return nil, fmt.Errorf("parse ac states for %s: %w", deviceID, err)
	}
	return resp.Result, nil
}

// CtoF converts Celsius to Fahrenheit.
// Deprecated: Use calc.CtoF instead.
func CtoF(c float64) float64 {
	return calc.CtoF(c)
}

// DewPoint calculates the dew point in °C from temperature (°C) and relative
// humidity (%) using the Magnus formula.
// Deprecated: Use calc.DewPoint instead.
func DewPoint(tempC, humidity float64) float64 {
	return calc.DewPoint(tempC, humidity)
}

// Summarize computes aggregate stats from historical measurements.
func Summarize(deviceID, roomName string, m HistoricalMeasurements) DeviceSummary {
	summary := DeviceSummary{
		DeviceID:    deviceID,
		RoomName:    roomName,
		DataPoints:  len(m.Temperature),
		MinTemp:     math.MaxFloat64,
		MinHumid:    math.MaxFloat64,
		MinDewPoint: math.MaxFloat64,
		MaxTemp:     -math.MaxFloat64,
		MaxHumid:    -math.MaxFloat64,
		MaxDewPoint: -math.MaxFloat64,
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

	// Compute dew point for each paired temperature+humidity reading.
	n := len(m.Temperature)
	if len(m.Humidity) < n {
		n = len(m.Humidity)
	}
	var totalDew float64
	for i := 0; i < n; i++ {
		dp := DewPoint(m.Temperature[i].Value, m.Humidity[i].Value)
		totalDew += dp
		if dp < summary.MinDewPoint {
			summary.MinDewPoint = dp
		}
		if dp > summary.MaxDewPoint {
			summary.MaxDewPoint = dp
		}
	}
	if n > 0 {
		summary.AvgDewPoint = totalDew / float64(n)
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
		summary.Latest.DewPoint = DewPoint(summary.Latest.Temperature, summary.Latest.Humidity)
	}

	return summary
}
