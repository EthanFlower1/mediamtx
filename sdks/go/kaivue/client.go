// Package kaivue provides a Go client for the KaiVue VMS public API.
//
// The client covers all 7 services (cameras, users, recordings, events,
// schedules, retention, integrations) with typed request/response structs,
// automatic pagination, and pluggable authentication (API key or OAuth bearer).
//
// Quick start:
//
//	client := kaivue.NewClient("https://your-instance.kaivue.io",
//	    kaivue.WithAPIKey("your-key"),
//	)
//	cameras, err := client.Cameras.List(ctx, &kaivue.ListCamerasRequest{})
package kaivue

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"
)

const sdkVersion = "0.1.0"

// Client is the top-level KaiVue API client.
type Client struct {
	baseURL    string
	httpClient *http.Client
	auth       AuthProvider

	Cameras      *CameraService
	Users        *UserService
	Recordings   *RecordingService
	Events       *EventService
	Schedules    *ScheduleService
	Retention    *RetentionService
	Integrations *IntegrationService
}

// Option configures the Client.
type Option func(*Client)

// WithHTTPClient sets a custom http.Client.
func WithHTTPClient(hc *http.Client) Option {
	return func(c *Client) { c.httpClient = hc }
}

// WithAPIKey configures API key authentication.
func WithAPIKey(key string) Option {
	return func(c *Client) { c.auth = &APIKeyAuth{Key: key} }
}

// WithOAuth configures OAuth bearer token authentication.
func WithOAuth(token string) Option {
	return func(c *Client) { c.auth = &OAuthAuth{AccessToken: token} }
}

// WithAuth sets a custom AuthProvider.
func WithAuth(auth AuthProvider) Option {
	return func(c *Client) { c.auth = auth }
}

// NewClient creates a new KaiVue API client.
func NewClient(baseURL string, opts ...Option) *Client {
	c := &Client{
		baseURL: strings.TrimRight(baseURL, "/"),
		httpClient: &http.Client{
			Timeout: 30 * time.Second,
		},
	}
	for _, opt := range opts {
		opt(c)
	}

	c.Cameras = &CameraService{client: c}
	c.Users = &UserService{client: c}
	c.Recordings = &RecordingService{client: c}
	c.Events = &EventService{client: c}
	c.Schedules = &ScheduleService{client: c}
	c.Retention = &RetentionService{client: c}
	c.Integrations = &IntegrationService{client: c}

	return c
}

// do executes an HTTP request with auth and returns the response body.
func (c *Client) do(ctx context.Context, method, path string, body any, params url.Values) ([]byte, error) {
	u := c.baseURL + path
	if len(params) > 0 {
		u += "?" + params.Encode()
	}

	var bodyReader io.Reader
	if body != nil {
		b, err := json.Marshal(body)
		if err != nil {
			return nil, fmt.Errorf("kaivue: marshal request: %w", err)
		}
		bodyReader = bytes.NewReader(b)
	}

	req, err := http.NewRequestWithContext(ctx, method, u, bodyReader)
	if err != nil {
		return nil, fmt.Errorf("kaivue: create request: %w", err)
	}

	req.Header.Set("User-Agent", "kaivue-go/"+sdkVersion)
	req.Header.Set("Accept", "application/json")
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}

	if c.auth != nil {
		c.auth.Apply(req)
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("kaivue: http request: %w", err)
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("kaivue: read response: %w", err)
	}

	if resp.StatusCode >= 400 {
		return nil, parseAPIError(resp.StatusCode, respBody, resp.Header.Get("X-Request-Id"))
	}

	return respBody, nil
}

func (c *Client) get(ctx context.Context, path string, params url.Values) ([]byte, error) {
	return c.do(ctx, http.MethodGet, path, nil, params)
}

func (c *Client) post(ctx context.Context, path string, body any) ([]byte, error) {
	return c.do(ctx, http.MethodPost, path, body, nil)
}

func (c *Client) patch(ctx context.Context, path string, body any) ([]byte, error) {
	return c.do(ctx, http.MethodPatch, path, body, nil)
}

func (c *Client) delete(ctx context.Context, path string, params url.Values) ([]byte, error) {
	return c.do(ctx, http.MethodDelete, path, nil, params)
}
