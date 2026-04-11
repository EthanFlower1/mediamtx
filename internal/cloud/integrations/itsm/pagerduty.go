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
	pagerDutyDefaultEndpoint = "https://events.pagerduty.com/v2/enqueue"
	pagerDutyMaxSummaryLen   = 1024
)

// pagerDutyEvent is the Events API v2 payload.
type pagerDutyEvent struct {
	RoutingKey  string                 `json:"routing_key"`
	EventAction string                 `json:"event_action"`
	DedupKey    string                 `json:"dedup_key,omitempty"`
	Payload     *pagerDutyPayload      `json:"payload,omitempty"`
}

type pagerDutyPayload struct {
	Summary   string            `json:"summary"`
	Source    string            `json:"source"`
	Severity  string            `json:"severity"`
	Timestamp string            `json:"timestamp"`
	Group     string            `json:"group,omitempty"`
	Class     string            `json:"class,omitempty"`
	CustomDetails map[string]string `json:"custom_details,omitempty"`
}

type pagerDutyResponse struct {
	Status   string `json:"status"`
	Message  string `json:"message"`
	DedupKey string `json:"dedup_key"`
}

// HTTPDoer abstracts *http.Client for testing.
type HTTPDoer interface {
	Do(req *http.Request) (*http.Response, error)
}

// PagerDutyClient implements Provider for PagerDuty Events API v2.
type PagerDutyClient struct {
	routingKey string
	endpoint   string
	httpClient HTTPDoer
}

// PagerDutyOption configures a PagerDutyClient.
type PagerDutyOption func(*PagerDutyClient)

// WithPagerDutyEndpoint overrides the default PagerDuty events endpoint.
func WithPagerDutyEndpoint(endpoint string) PagerDutyOption {
	return func(c *PagerDutyClient) {
		c.endpoint = endpoint
	}
}

// WithPagerDutyHTTPClient sets a custom HTTP client (useful for testing).
func WithPagerDutyHTTPClient(client HTTPDoer) PagerDutyOption {
	return func(c *PagerDutyClient) {
		c.httpClient = client
	}
}

// NewPagerDutyClient creates a new PagerDuty integration client.
func NewPagerDutyClient(routingKey string, opts ...PagerDutyOption) (*PagerDutyClient, error) {
	if routingKey == "" {
		return nil, fmt.Errorf("%w: pagerduty routing key is required", ErrInvalidConfig)
	}
	c := &PagerDutyClient{
		routingKey: routingKey,
		endpoint:   pagerDutyDefaultEndpoint,
		httpClient: &http.Client{Timeout: 30 * time.Second},
	}
	for _, opt := range opts {
		opt(c)
	}
	return c, nil
}

func (c *PagerDutyClient) Type() ProviderType {
	return ProviderPagerDuty
}

func (c *PagerDutyClient) SendAlert(ctx context.Context, alert Alert) (AlertResult, error) {
	summary := alert.Summary
	if len(summary) > pagerDutyMaxSummaryLen {
		summary = summary[:pagerDutyMaxSummaryLen]
	}

	evt := pagerDutyEvent{
		RoutingKey:  c.routingKey,
		EventAction: "trigger",
		DedupKey:    alert.DedupKey,
		Payload: &pagerDutyPayload{
			Summary:       summary,
			Source:        alert.Source,
			Severity:      pdSeverity(alert.Severity),
			Timestamp:     alert.Timestamp.UTC().Format(time.RFC3339),
			Group:         alert.Group,
			Class:         alert.Class,
			CustomDetails: alert.Details,
		},
	}

	resp, err := c.sendEvent(ctx, evt)
	if err != nil {
		return AlertResult{}, err
	}
	return AlertResult{
		ProviderType: ProviderPagerDuty,
		ExternalID:   resp.DedupKey,
		Status:       resp.Status,
		Message:      resp.Message,
		Timestamp:    time.Now().UTC(),
	}, nil
}

func (c *PagerDutyClient) ResolveAlert(ctx context.Context, dedupKey string) (AlertResult, error) {
	evt := pagerDutyEvent{
		RoutingKey:  c.routingKey,
		EventAction: "resolve",
		DedupKey:    dedupKey,
	}
	resp, err := c.sendEvent(ctx, evt)
	if err != nil {
		return AlertResult{}, err
	}
	return AlertResult{
		ProviderType: ProviderPagerDuty,
		ExternalID:   resp.DedupKey,
		Status:       resp.Status,
		Message:      resp.Message,
		Timestamp:    time.Now().UTC(),
	}, nil
}

func (c *PagerDutyClient) TestConnection(ctx context.Context) error {
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

func (c *PagerDutyClient) sendEvent(ctx context.Context, evt pagerDutyEvent) (pagerDutyResponse, error) {
	body, err := json.Marshal(evt)
	if err != nil {
		return pagerDutyResponse{}, fmt.Errorf("pagerduty: marshal event: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.endpoint, bytes.NewReader(body))
	if err != nil {
		return pagerDutyResponse{}, fmt.Errorf("pagerduty: create request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return pagerDutyResponse{}, fmt.Errorf("%w: pagerduty: %v", ErrAlertFailed, err)
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if err != nil {
		return pagerDutyResponse{}, fmt.Errorf("pagerduty: read response: %w", err)
	}

	if resp.StatusCode == http.StatusTooManyRequests {
		return pagerDutyResponse{}, ErrRateLimited
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return pagerDutyResponse{}, fmt.Errorf("%w: pagerduty: status %d: %s", ErrAlertFailed, resp.StatusCode, string(respBody))
	}

	var pdResp pagerDutyResponse
	if err := json.Unmarshal(respBody, &pdResp); err != nil {
		return pagerDutyResponse{}, fmt.Errorf("pagerduty: decode response: %w", err)
	}
	return pdResp, nil
}

// pdSeverity maps our severity to PagerDuty severity values.
func pdSeverity(s Severity) string {
	switch s {
	case SeverityCritical:
		return "critical"
	case SeverityError:
		return "error"
	case SeverityWarning:
		return "warning"
	case SeverityInfo:
		return "info"
	default:
		return "error"
	}
}
