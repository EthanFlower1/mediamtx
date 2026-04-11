package itsm

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"
)

const (
	opsgenieDefaultEndpoint = "https://api.opsgenie.com/v2/alerts"
	opsgenieEUEndpoint      = "https://api.eu.opsgenie.com/v2/alerts"
)

// opsgenieCreateAlert is the Opsgenie Create Alert payload.
type opsgenieCreateAlert struct {
	Message     string            `json:"message"`
	Alias       string            `json:"alias,omitempty"`
	Description string            `json:"description,omitempty"`
	Entity      string            `json:"entity,omitempty"`
	Source      string            `json:"source,omitempty"`
	Priority    string            `json:"priority"`
	Tags        []string          `json:"tags,omitempty"`
	Details     map[string]string `json:"details,omitempty"`
}

// opsgenieCloseAlert is the Opsgenie Close Alert payload.
type opsgenieCloseAlert struct {
	Source string `json:"source,omitempty"`
	Note   string `json:"note,omitempty"`
}

type opsgenieResponse struct {
	Result    string `json:"result"`
	RequestID string `json:"requestId"`
}

// OpsgenieClient implements Provider for Atlassian Opsgenie.
type OpsgenieClient struct {
	apiKey     string
	endpoint   string
	httpClient HTTPDoer
}

// OpsgenieOption configures an OpsgenieClient.
type OpsgenieOption func(*OpsgenieClient)

// WithOpsgenieEndpoint overrides the default Opsgenie API endpoint.
func WithOpsgenieEndpoint(endpoint string) OpsgenieOption {
	return func(c *OpsgenieClient) {
		c.endpoint = endpoint
	}
}

// WithOpsgenieHTTPClient sets a custom HTTP client for testing.
func WithOpsgenieHTTPClient(client HTTPDoer) OpsgenieOption {
	return func(c *OpsgenieClient) {
		c.httpClient = client
	}
}

// WithOpsgenieEU configures the client to use the EU Opsgenie endpoint.
func WithOpsgenieEU() OpsgenieOption {
	return func(c *OpsgenieClient) {
		c.endpoint = opsgenieEUEndpoint
	}
}

// NewOpsgenieClient creates a new Opsgenie integration client.
func NewOpsgenieClient(apiKey string, opts ...OpsgenieOption) (*OpsgenieClient, error) {
	if apiKey == "" {
		return nil, fmt.Errorf("%w: opsgenie API key is required", ErrInvalidConfig)
	}
	c := &OpsgenieClient{
		apiKey:     apiKey,
		endpoint:   opsgenieDefaultEndpoint,
		httpClient: &http.Client{Timeout: 30 * time.Second},
	}
	for _, opt := range opts {
		opt(c)
	}
	return c, nil
}

func (c *OpsgenieClient) Type() ProviderType {
	return ProviderOpsgenie
}

func (c *OpsgenieClient) SendAlert(ctx context.Context, alert Alert) (AlertResult, error) {
	payload := opsgenieCreateAlert{
		Message:  truncate(alert.Summary, 130), // Opsgenie message limit
		Alias:    alert.DedupKey,
		Entity:   alert.Group,
		Source:   alert.Source,
		Priority: ogPriority(alert.Severity),
		Details:  alert.Details,
	}
	if alert.Class != "" {
		payload.Tags = append(payload.Tags, alert.Class)
	}

	body, err := json.Marshal(payload)
	if err != nil {
		return AlertResult{}, fmt.Errorf("opsgenie: marshal alert: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.endpoint, bytes.NewReader(body))
	if err != nil {
		return AlertResult{}, fmt.Errorf("opsgenie: create request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "GenieKey "+c.apiKey)

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return AlertResult{}, fmt.Errorf("%w: opsgenie: %v", ErrAlertFailed, err)
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if err != nil {
		return AlertResult{}, fmt.Errorf("opsgenie: read response: %w", err)
	}

	if resp.StatusCode == http.StatusTooManyRequests {
		return AlertResult{}, ErrRateLimited
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return AlertResult{}, fmt.Errorf("%w: opsgenie: status %d: %s", ErrAlertFailed, resp.StatusCode, string(respBody))
	}

	var ogResp opsgenieResponse
	if err := json.Unmarshal(respBody, &ogResp); err != nil {
		return AlertResult{}, fmt.Errorf("opsgenie: decode response: %w", err)
	}

	return AlertResult{
		ProviderType: ProviderOpsgenie,
		ExternalID:   ogResp.RequestID,
		Status:       "success",
		Message:      ogResp.Result,
		Timestamp:    time.Now().UTC(),
	}, nil
}

func (c *OpsgenieClient) ResolveAlert(ctx context.Context, dedupKey string) (AlertResult, error) {
	closeURL := fmt.Sprintf("%s/%s/close?identifierType=alias", c.endpoint, dedupKey)

	payload := opsgenieCloseAlert{
		Source: "mediamtx-nvr",
		Note:   "Resolved by MediaMTX NVR",
	}
	body, err := json.Marshal(payload)
	if err != nil {
		return AlertResult{}, fmt.Errorf("opsgenie: marshal close: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, closeURL, bytes.NewReader(body))
	if err != nil {
		return AlertResult{}, fmt.Errorf("opsgenie: create close request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "GenieKey "+c.apiKey)

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return AlertResult{}, fmt.Errorf("%w: opsgenie: %v", ErrAlertFailed, err)
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if err != nil {
		return AlertResult{}, fmt.Errorf("opsgenie: read close response: %w", err)
	}

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return AlertResult{}, fmt.Errorf("%w: opsgenie close: status %d: %s", ErrAlertFailed, resp.StatusCode, string(respBody))
	}

	var ogResp opsgenieResponse
	_ = json.Unmarshal(respBody, &ogResp) // best-effort parse

	return AlertResult{
		ProviderType: ProviderOpsgenie,
		ExternalID:   ogResp.RequestID,
		Status:       "success",
		Message:      "resolved",
		Timestamp:    time.Now().UTC(),
	}, nil
}

func (c *OpsgenieClient) TestConnection(ctx context.Context) error {
	alert := Alert{
		Summary:   "MediaMTX NVR integration test",
		Source:    "mediamtx-itsm-test",
		Severity:  SeverityInfo,
		DedupKey:  "mediamtx-test-connection",
		Timestamp: time.Now().UTC(),
		Details:   map[string]string{"type": "connection_test"},
	}
	result, err := c.SendAlert(ctx, alert)
	if err != nil {
		return err
	}
	// Immediately resolve the test alert.
	_, _ = c.ResolveAlert(ctx, result.ExternalID)
	return nil
}

// ogPriority maps our severity to Opsgenie priority levels (P1-P5).
func ogPriority(s Severity) string {
	switch s {
	case SeverityCritical:
		return "P1"
	case SeverityError:
		return "P2"
	case SeverityWarning:
		return "P3"
	case SeverityInfo:
		return "P5"
	default:
		return "P3"
	}
}

func truncate(s string, max int) string {
	if len(s) <= max {
		return s
	}
	return s[:max]
}
