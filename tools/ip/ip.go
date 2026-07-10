// Package ip provides an agent tool backed by api.ipapi.is.
package ip

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

const defaultBaseURL = "https://api.ipapi.is"

type Client struct {
	HTTPClient *http.Client
	BaseURL    string
}

func (c Client) Tool() agent.Tool {
	return agent.Tool{Name: "ip_lookup", Description: "Look up geographic and network information for an IP address.", Parameters: json.RawMessage(`{"type":"object","properties":{"ip":{"type":"string","description":"IPv4 or IPv6 address to look up."}},"required":["ip"],"additionalProperties":false}`), Handler: c.handle}
}
func (c Client) Lookup(ctx context.Context, address string) (json.RawMessage, error) {
	address = strings.TrimSpace(address)
	if address == "" {
		return nil, fmt.Errorf("ip: address is required")
	}
	baseURL := c.BaseURL
	if baseURL == "" {
		baseURL = defaultBaseURL
	}
	requestURL := strings.TrimRight(baseURL, "/") + "/?q=" + url.QueryEscape(address)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, requestURL, nil)
	if err != nil {
		return nil, err
	}
	client := c.HTTPClient
	if client == nil {
		client = &http.Client{Timeout: 10 * time.Second}
	}
	response, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer response.Body.Close()
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		return nil, fmt.Errorf("ip: api.ipapi.is returned %s", response.Status)
	}
	var payload json.RawMessage
	if err := json.NewDecoder(response.Body).Decode(&payload); err != nil {
		return nil, err
	}
	if !json.Valid(payload) {
		return nil, fmt.Errorf("ip: api.ipapi.is returned invalid JSON")
	}
	return payload, nil
}
func (c Client) handle(ctx context.Context, arguments json.RawMessage) (string, error) {
	var input struct {
		IP string `json:"ip"`
	}
	if err := json.Unmarshal(arguments, &input); err != nil {
		return "", err
	}
	output, err := c.Lookup(ctx, input.IP)
	return string(output), err
}
