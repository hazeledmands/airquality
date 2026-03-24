// Package openmeteo fetches current weather conditions from Open-Meteo.
package openmeteo

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"time"
)

const defaultBaseURL = "https://api.open-meteo.com"

// Reading holds a single weather observation.
type Reading struct {
	Time            time.Time
	Latitude        float64
	Longitude       float64
	TempC           float64
	Humidity        float64
	DewPointC       float64
	PrecipitationMM float64
	WeatherCode     int
	PressureHPa     float64
	WindSpeedKMH    float64
	UVIndex         float64
	Sunrise         time.Time
	Sunset          time.Time
}

// Client fetches weather from Open-Meteo for a fixed location.
type Client struct {
	Latitude  float64
	Longitude float64
	baseURL   string
	http      *http.Client
}

// NewClient creates a Client for the given coordinates.
func NewClient(lat, lon float64) *Client {
	return &Client{
		Latitude:  lat,
		Longitude: lon,
		baseURL:   defaultBaseURL,
		http:      &http.Client{Timeout: 15 * time.Second},
	}
}

type apiResponse struct {
	Current struct {
		Time            string  `json:"time"`
		TempC           float64 `json:"temperature_2m"`
		Humidity        float64 `json:"relative_humidity_2m"`
		DewPointC       float64 `json:"dew_point_2m"`
		PrecipitationMM float64 `json:"precipitation"`
		WeatherCode     int     `json:"weather_code"`
		PressureHPa     float64 `json:"pressure_msl"`
		WindSpeedKMH    float64 `json:"wind_speed_10m"`
		UVIndex         float64 `json:"uv_index"`
	} `json:"current"`
	Daily struct {
		Sunrise []string `json:"sunrise"`
		Sunset  []string `json:"sunset"`
	} `json:"daily"`
}

// Fetch retrieves current conditions and today's sunrise/sunset.
func (c *Client) Fetch(ctx context.Context) (Reading, error) {
	params := url.Values{}
	params.Set("latitude", fmt.Sprintf("%f", c.Latitude))
	params.Set("longitude", fmt.Sprintf("%f", c.Longitude))
	params.Set("current", "temperature_2m,relative_humidity_2m,dew_point_2m,precipitation,weather_code,pressure_msl,wind_speed_10m,uv_index")
	params.Set("daily", "sunrise,sunset")
	params.Set("timezone", "UTC")
	params.Set("forecast_days", "1")

	u := fmt.Sprintf("%s/v1/forecast?%s", c.baseURL, params.Encode())
	req, err := http.NewRequestWithContext(ctx, "GET", u, nil)
	if err != nil {
		return Reading{}, fmt.Errorf("fetch weather: %w", err)
	}
	resp, err := c.http.Do(req)
	if err != nil {
		return Reading{}, fmt.Errorf("fetch weather: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return Reading{}, fmt.Errorf("read response: %w", err)
	}
	if resp.StatusCode != http.StatusOK {
		return Reading{}, fmt.Errorf("API error %d: %s", resp.StatusCode, body)
	}

	var data apiResponse
	if err := json.Unmarshal(body, &data); err != nil {
		return Reading{}, fmt.Errorf("parse response: %w", err)
	}

	r := Reading{
		Latitude:        c.Latitude,
		Longitude:       c.Longitude,
		TempC:           data.Current.TempC,
		Humidity:        data.Current.Humidity,
		DewPointC:       data.Current.DewPointC,
		PrecipitationMM: data.Current.PrecipitationMM,
		WeatherCode:     data.Current.WeatherCode,
		PressureHPa:     data.Current.PressureHPa,
		WindSpeedKMH:    data.Current.WindSpeedKMH,
		UVIndex:         data.Current.UVIndex,
	}

	// Parse observation time (format "2006-01-02T15:04", no timezone — API returns UTC when timezone=UTC)
	if t, err := time.Parse("2006-01-02T15:04", data.Current.Time); err == nil {
		r.Time = t.UTC()
	} else {
		r.Time = time.Now().UTC()
	}

	const dailyFmt = "2006-01-02T15:04"
	if len(data.Daily.Sunrise) > 0 {
		if t, err := time.Parse(dailyFmt, data.Daily.Sunrise[0]); err == nil {
			r.Sunrise = t.UTC()
		}
	}
	if len(data.Daily.Sunset) > 0 {
		if t, err := time.Parse(dailyFmt, data.Daily.Sunset[0]); err == nil {
			r.Sunset = t.UTC()
		}
	}

	return r, nil
}
