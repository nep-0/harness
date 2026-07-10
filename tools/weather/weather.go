// Package weather provides an agent tool backed by wttr.in.
package weather

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/nep-0/harness/agent"
)

const defaultBaseURL = "https://wttr.in"

type Client struct {
	HTTPClient *http.Client
	BaseURL    string
}
type Report struct {
	Location     string `json:"location"`
	Condition    string `json:"condition"`
	TemperatureC string `json:"temperature_c"`
	FeelsLikeC   string `json:"feels_like_c"`
	Humidity     string `json:"humidity"`
	WindKPH      string `json:"wind_kph"`
}

func (c Client) Tool() agent.Tool {
	return agent.Tool{Name: "weather", Description: "Get the current weather for a location.", Parameters: json.RawMessage(`{"type":"object","properties":{"location":{"type":"string","description":"City, region, or other wttr.in location query."}},"required":["location"],"additionalProperties":false}`), Handler: c.handle}
}
func (c Client) Lookup(ctx context.Context, location string) (Report, error) {
	location = strings.TrimSpace(location)
	if location == "" {
		return Report{}, fmt.Errorf("weather: location is required")
	}
	baseURL := c.BaseURL
	if baseURL == "" {
		baseURL = defaultBaseURL
	}
	requestURL := strings.TrimRight(baseURL, "/") + "/" + url.PathEscape(location) + "?format=j1"
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, requestURL, nil)
	if err != nil {
		return Report{}, err
	}
	client := c.HTTPClient
	if client == nil {
		client = &http.Client{Timeout: 10 * time.Second}
	}
	response, err := client.Do(req)
	if err != nil {
		return Report{}, err
	}
	defer response.Body.Close()
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		return Report{}, fmt.Errorf("weather: wttr.in returned %s", response.Status)
	}
	var payload struct {
		Current []struct {
			Desc []struct {
				Value string `json:"value"`
			} `json:"weatherDesc"`
			Temp     string `json:"temp_C"`
			Feels    string `json:"FeelsLikeC"`
			Humidity string `json:"humidity"`
			Wind     string `json:"windspeedKmph"`
		} `json:"current_condition"`
		Area []struct {
			Value string `json:"value"`
		} `json:"nearest_area"`
	}
	if err := json.NewDecoder(response.Body).Decode(&payload); err != nil {
		return Report{}, err
	}
	if len(payload.Current) == 0 {
		return Report{}, fmt.Errorf("weather: wttr.in returned no current conditions")
	}
	report := Report{Location: location, TemperatureC: payload.Current[0].Temp, FeelsLikeC: payload.Current[0].Feels, Humidity: payload.Current[0].Humidity, WindKPH: payload.Current[0].Wind}
	if len(payload.Current[0].Desc) > 0 {
		report.Condition = payload.Current[0].Desc[0].Value
	}
	if len(payload.Area) > 0 && payload.Area[0].Value != "" {
		report.Location = payload.Area[0].Value
	}
	return report, nil
}
func (c Client) handle(ctx context.Context, arguments json.RawMessage) (string, error) {
	var input struct {
		Location string `json:"location"`
	}
	if err := json.Unmarshal(arguments, &input); err != nil {
		return "", err
	}
	report, err := c.Lookup(ctx, input.Location)
	if err != nil {
		return "", err
	}
	output, err := json.Marshal(report)
	return string(output), err
}
