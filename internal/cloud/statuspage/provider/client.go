package provider

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"
)

const defaultBaseURL = "https://api.statuspage.io/v1"

// Provider defines the operations required to manage a Statuspage.io page.
// Mock this interface in tests.
type Provider interface {
	// Components
	ListComponents(ctx context.Context) ([]Component, error)
	GetComponent(ctx context.Context, componentID string) (Component, error)
	CreateComponent(ctx context.Context, c Component) (Component, error)
	UpdateComponent(ctx context.Context, componentID string, c Component) (Component, error)
	UpdateComponentStatus(ctx context.Context, componentID string, status ComponentStatus) error
	DeleteComponent(ctx context.Context, componentID string) error

	// Component Groups
	ListComponentGroups(ctx context.Context) ([]ComponentGroup, error)
	CreateComponentGroup(ctx context.Context, g ComponentGroup) (ComponentGroup, error)

	// Incidents
	ListUnresolvedIncidents(ctx context.Context) ([]Incident, error)
	CreateIncident(ctx context.Context, req CreateIncidentRequest) (Incident, error)
	UpdateIncident(ctx context.Context, incidentID string, req UpdateIncidentRequest) (Incident, error)
}

// ClientConfig holds configuration for the Statuspage.io HTTP client.
type ClientConfig struct {
	// APIKey is the Statuspage.io API key (OAuth or API key).
	APIKey string

	// PageID is the Statuspage.io page identifier.
	PageID string

	// BaseURL overrides the default API base URL (for testing).
	BaseURL string

	// HTTPClient overrides the default HTTP client.
	HTTPClient *http.Client
}

// Client implements Provider against the real Statuspage.io v1 REST API.
type Client struct {
	apiKey     string
	pageID     string
	baseURL    string
	httpClient *http.Client
}

// NewClient constructs a Client from the given config.
func NewClient(cfg ClientConfig) (*Client, error) {
	if cfg.APIKey == "" {
		return nil, fmt.Errorf("statuspage/provider: APIKey is required")
	}
	if cfg.PageID == "" {
		return nil, fmt.Errorf("statuspage/provider: PageID is required")
	}
	base := cfg.BaseURL
	if base == "" {
		base = defaultBaseURL
	}
	hc := cfg.HTTPClient
	if hc == nil {
		hc = &http.Client{Timeout: 30 * time.Second}
	}
	return &Client{
		apiKey:     cfg.APIKey,
		pageID:     cfg.PageID,
		baseURL:    base,
		httpClient: hc,
	}, nil
}

// ------------------------------------------------------------------
// Components
// ------------------------------------------------------------------

func (c *Client) ListComponents(ctx context.Context) ([]Component, error) {
	var out []Component
	err := c.doJSON(ctx, http.MethodGet, c.url("/components"), nil, &out)
	return out, err
}

func (c *Client) GetComponent(ctx context.Context, componentID string) (Component, error) {
	var out Component
	err := c.doJSON(ctx, http.MethodGet, c.url("/components/"+componentID), nil, &out)
	return out, err
}

func (c *Client) CreateComponent(ctx context.Context, comp Component) (Component, error) {
	body := map[string]interface{}{"component": comp}
	var out Component
	err := c.doJSON(ctx, http.MethodPost, c.url("/components"), body, &out)
	return out, err
}

func (c *Client) UpdateComponent(ctx context.Context, componentID string, comp Component) (Component, error) {
	body := map[string]interface{}{"component": comp}
	var out Component
	err := c.doJSON(ctx, http.MethodPatch, c.url("/components/"+componentID), body, &out)
	return out, err
}

func (c *Client) UpdateComponentStatus(ctx context.Context, componentID string, status ComponentStatus) error {
	body := map[string]interface{}{
		"component": map[string]string{
			"status": string(status),
		},
	}
	return c.doJSON(ctx, http.MethodPatch, c.url("/components/"+componentID), body, nil)
}

func (c *Client) DeleteComponent(ctx context.Context, componentID string) error {
	return c.doJSON(ctx, http.MethodDelete, c.url("/components/"+componentID), nil, nil)
}

// ------------------------------------------------------------------
// Component Groups
// ------------------------------------------------------------------

func (c *Client) ListComponentGroups(ctx context.Context) ([]ComponentGroup, error) {
	var out []ComponentGroup
	err := c.doJSON(ctx, http.MethodGet, c.url("/component-groups"), nil, &out)
	return out, err
}

func (c *Client) CreateComponentGroup(ctx context.Context, g ComponentGroup) (ComponentGroup, error) {
	body := map[string]interface{}{"component_group": g}
	var out ComponentGroup
	err := c.doJSON(ctx, http.MethodPost, c.url("/component-groups"), body, &out)
	return out, err
}

// ------------------------------------------------------------------
// Incidents
// ------------------------------------------------------------------

func (c *Client) ListUnresolvedIncidents(ctx context.Context) ([]Incident, error) {
	var out []Incident
	err := c.doJSON(ctx, http.MethodGet, c.url("/incidents/unresolved"), nil, &out)
	return out, err
}

func (c *Client) CreateIncident(ctx context.Context, req CreateIncidentRequest) (Incident, error) {
	body := map[string]interface{}{"incident": req}
	var out Incident
	err := c.doJSON(ctx, http.MethodPost, c.url("/incidents"), body, &out)
	return out, err
}

func (c *Client) UpdateIncident(ctx context.Context, incidentID string, req UpdateIncidentRequest) (Incident, error) {
	body := map[string]interface{}{"incident": req}
	var out Incident
	err := c.doJSON(ctx, http.MethodPatch, c.url("/incidents/"+incidentID), body, &out)
	return out, err
}

// ------------------------------------------------------------------
// HTTP helpers
// ------------------------------------------------------------------

func (c *Client) url(path string) string {
	return fmt.Sprintf("%s/pages/%s%s", c.baseURL, c.pageID, path)
}

// doJSON executes an HTTP request with JSON encoding/decoding.
func (c *Client) doJSON(ctx context.Context, method, url string, reqBody interface{}, respBody interface{}) error {
	var bodyReader io.Reader
	if reqBody != nil {
		b, err := json.Marshal(reqBody)
		if err != nil {
			return fmt.Errorf("statuspage/provider: marshal request: %w", err)
		}
		bodyReader = bytes.NewReader(b)
	}

	req, err := http.NewRequestWithContext(ctx, method, url, bodyReader)
	if err != nil {
		return fmt.Errorf("statuspage/provider: build request: %w", err)
	}
	req.Header.Set("Authorization", "OAuth "+c.apiKey)
	if bodyReader != nil {
		req.Header.Set("Content-Type", "application/json")
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("statuspage/provider: %s %s: %w", method, url, err)
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
		return &APIError{
			StatusCode: resp.StatusCode,
			Method:     method,
			URL:        url,
			Body:       string(body),
		}
	}

	if respBody != nil && resp.StatusCode != http.StatusNoContent {
		if err := json.NewDecoder(resp.Body).Decode(respBody); err != nil {
			return fmt.Errorf("statuspage/provider: decode response: %w", err)
		}
	}
	return nil
}

// APIError represents a non-2xx response from the Statuspage.io API.
type APIError struct {
	StatusCode int
	Method     string
	URL        string
	Body       string
}

func (e *APIError) Error() string {
	return fmt.Sprintf("statuspage/provider: %s %s returned %d: %s", e.Method, e.URL, e.StatusCode, e.Body)
}
