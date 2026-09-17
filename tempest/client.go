// Package tempest fetches weather observations from a WeatherFlow Tempest station
// via the cloud REST API.
package tempest

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"
)

const DefaultBaseURL = "https://swd.weatherflow.com"

// Observation holds a single observation from a Tempest station.
type Observation struct {
	Time      time.Time
	StationID int
	StationName string
	Latitude  float64
	Longitude float64

	// Temperature and humidity
	TempC     float64
	Humidity  float64
	DewPointC float64

	// Pressure
	PressureHPa    float64
	SeaLevelHPa    float64

	// Wind
	WindSpeedMS   float64
	WindDirection int
	WindGustMS    float64
	WindLullMS    float64

	// Precipitation
	PrecipitationMM     float64
	PrecipDayAccumMM    float64

	// Solar / UV
	UVIndex        float64
	SolarRadiation int

	// Lightning
	LightningStrikeCount int

	// Derived temps
	FeelsLikeC float64
	HeatIndexC float64
	WindChillC float64
}

// Client fetches observations from the WeatherFlow API.
type Client struct {
	Token     string
	StationID string
	baseURL   string
	http      *http.Client
}

// NewClient creates a Tempest API client for the given token and station ID.
func NewClient(token, stationID string) *Client {
	return &Client{
		Token:     token,
		StationID: stationID,
		baseURL:   DefaultBaseURL,
		http:      &http.Client{Timeout: 15 * time.Second},
	}
}

type apiResponse struct {
	Status struct {
		StatusCode    int    `json:"status_code"`
		StatusMessage string `json:"status_message"`
	} `json:"status"`
	StationID   int     `json:"station_id"`
	StationName string  `json:"station_name"`
	Latitude    float64 `json:"latitude"`
	Longitude   float64 `json:"longitude"`
	Obs         []struct {
		Timestamp            int64   `json:"timestamp"`
		AirTemperature       float64 `json:"air_temperature"`
		RelativeHumidity     float64 `json:"relative_humidity"`
		DewPoint             float64 `json:"dew_point"`
		StationPressure      float64 `json:"station_pressure"`
		SeaLevelPressure     float64 `json:"sea_level_pressure"`
		WindAvg              float64 `json:"wind_avg"`
		WindDirection        int     `json:"wind_direction"`
		WindGust             float64 `json:"wind_gust"`
		WindLull             float64 `json:"wind_lull"`
		Precip               float64 `json:"precip"`
		PrecipAccumLocalDay  float64 `json:"precip_accum_local_day"`
		UV                   float64 `json:"uv"`
		SolarRadiation       int     `json:"solar_radiation"`
		LightningStrikeCount int     `json:"lightning_strike_count"`
		FeelsLike            float64 `json:"feels_like"`
		HeatIndex            float64 `json:"heat_index"`
		WindChill            float64 `json:"wind_chill"`
	} `json:"obs"`
}

type stationsResponse struct {
	Stations []struct {
		StationID   int     `json:"station_id"`
		StationName string  `json:"station_name"`
		Latitude    float64 `json:"latitude"`
		Longitude   float64 `json:"longitude"`
	} `json:"stations"`
}

// DiscoverStationID calls the stations endpoint and returns the first
// station ID associated with the given token, or an empty string if none
// are found.
func DiscoverStationID(ctx context.Context, baseURL, token string, httpClient *http.Client) (string, error) {
	if token == "" {
		return "", fmt.Errorf("token is empty")
	}
	u := fmt.Sprintf("%s/swd/rest/stations/?token=%s", baseURL, token)

	req, err := http.NewRequestWithContext(ctx, "GET", u, nil)
	if err != nil {
		return "", fmt.Errorf("discover tempest station: %w", err)
	}

	resp, err := httpClient.Do(req)
	if err != nil {
		return "", fmt.Errorf("discover tempest station: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("tempest API error %d", resp.StatusCode)
	}

	var data stationsResponse
	if err := json.NewDecoder(resp.Body).Decode(&data); err != nil {
		return "", fmt.Errorf("parse tempest stations: %w", err)
	}

	if len(data.Stations) == 0 {
		return "", fmt.Errorf("no stations found for token")
	}

	return fmt.Sprintf("%d", data.Stations[0].StationID), nil
}

// Fetch retrieves the latest observation from the configured station.
func (c *Client) Fetch(ctx context.Context) (Observation, error) {
	u := fmt.Sprintf("%s/swd/rest/observations/station/%s?token=%s",
		c.baseURL, c.StationID, c.Token)

	req, err := http.NewRequestWithContext(ctx, "GET", u, nil)
	if err != nil {
		return Observation{}, fmt.Errorf("fetch tempest: %w", err)
	}

	resp, err := c.http.Do(req)
	if err != nil {
		return Observation{}, fmt.Errorf("fetch tempest: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return Observation{}, fmt.Errorf("read tempest response: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		return Observation{}, fmt.Errorf("tempest API error %d: %s", resp.StatusCode, body)
	}

	var data apiResponse
	if err := json.Unmarshal(body, &data); err != nil {
		return Observation{}, fmt.Errorf("parse tempest response: %w", err)
	}

	if data.Status.StatusCode != 0 {
		return Observation{}, fmt.Errorf("tempest API status %d: %s",
			data.Status.StatusCode, data.Status.StatusMessage)
	}

	if len(data.Obs) == 0 {
		return Observation{}, fmt.Errorf("tempest API returned no observations")
	}

	latest := data.Obs[0]

	return Observation{
		Time:                 time.Unix(latest.Timestamp, 0).UTC(),
		StationID:            data.StationID,
		StationName:          data.StationName,
		Latitude:             data.Latitude,
		Longitude:            data.Longitude,
		TempC:                latest.AirTemperature,
		Humidity:             latest.RelativeHumidity,
		DewPointC:            latest.DewPoint,
		PressureHPa:          latest.StationPressure,
		SeaLevelHPa:          latest.SeaLevelPressure,
		WindSpeedMS:          latest.WindAvg,
		WindDirection:        latest.WindDirection,
		WindGustMS:           latest.WindGust,
		WindLullMS:           latest.WindLull,
		PrecipitationMM:      latest.Precip,
		PrecipDayAccumMM:     latest.PrecipAccumLocalDay,
		UVIndex:              latest.UV,
		SolarRadiation:       latest.SolarRadiation,
		LightningStrikeCount: latest.LightningStrikeCount,
		FeelsLikeC:           latest.FeelsLike,
		HeatIndexC:           latest.HeatIndex,
		WindChillC:           latest.WindChill,
	}, nil
}
