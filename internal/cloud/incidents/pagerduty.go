package incidents

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
)

// PagerDutyClient sends events to the PagerDuty Events API v2.
type PagerDutyClient interface {
	// SendEvent sends a trigger/acknowledge/resolve event to PagerDuty.
	SendEvent(ctx context.Context, event PagerDutyEvent) (PagerDutyResponse, error)
}

// HTTPPagerDutyClient implements PagerDutyClient using the PagerDuty Events API v2.
type HTTPPagerDutyClient struct {
	httpClient *http.Client
	baseURL    string
}

const defaultPagerDutyURL = "https://events.pagerduty.com/v2/enqueue"

// NewHTTPPagerDutyClient creates a new PagerDuty client.
// If httpClient is nil, http.DefaultClient is used.
// If baseURL is empty, the default PagerDuty events URL is used.
func NewHTTPPagerDutyClient(httpClient *http.Client, baseURL string) *HTTPPagerDutyClient {
	if httpClient == nil {
		httpClient = http.DefaultClient
	}
	if baseURL == "" {
		baseURL = defaultPagerDutyURL
	}
	return &HTTPPagerDutyClient{httpClient: httpClient, baseURL: baseURL}
}

// SendEvent sends an event to the PagerDuty Events API v2.
func (c *HTTPPagerDutyClient) SendEvent(ctx context.Context, event PagerDutyEvent) (PagerDutyResponse, error) {
	body, err := json.Marshal(event)
	if err != nil {
		return PagerDutyResponse{}, fmt.Errorf("marshal pagerduty event: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.baseURL, bytes.NewReader(body))
	if err != nil {
		return PagerDutyResponse{}, fmt.Errorf("create pagerduty request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return PagerDutyResponse{}, fmt.Errorf("send pagerduty event: %w", err)
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return PagerDutyResponse{}, fmt.Errorf("read pagerduty response: %w", err)
	}

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return PagerDutyResponse{}, fmt.Errorf("%w: status %d: %s", ErrPagerDutyAPI, resp.StatusCode, string(respBody))
	}

	var pdResp PagerDutyResponse
	if err := json.Unmarshal(respBody, &pdResp); err != nil {
		return PagerDutyResponse{}, fmt.Errorf("unmarshal pagerduty response: %w", err)
	}
	return pdResp, nil
}
