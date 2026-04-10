// Package opsgenie implements a DeliveryChannel adapter for Opsgenie
// using the Alert API for creating and closing alerts.
package opsgenie

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"

	"github.com/bluenviron/mediamtx/internal/cloud/notifications"
)

const defaultAPIURL = "https://api.opsgenie.com/v2/alerts"

// Config configures the Opsgenie adapter.
type Config struct {
	// APIKey is the Opsgenie API integration key.
	APIKey string
	// APIURL overrides the Alert API endpoint (for testing).
	APIURL string
	// HTTPClient overrides the default HTTP client.
	HTTPClient *http.Client
}

// Adapter delivers notifications to Opsgenie via the Alert API.
type Adapter struct {
	apiKey string
	apiURL string
	client *http.Client
}

// New creates an Opsgenie adapter.
func New(cfg Config) (*Adapter, error) {
	if cfg.APIKey == "" {
		return nil, fmt.Errorf("opsgenie: API key is required")
	}
	apiURL := cfg.APIURL
	if apiURL == "" {
		apiURL = defaultAPIURL
	}
	client := cfg.HTTPClient
	if client == nil {
		client = http.DefaultClient
	}
	return &Adapter{
		apiKey: cfg.APIKey,
		apiURL: apiURL,
		client: client,
	}, nil
}

// Type returns the channel type.
func (a *Adapter) Type() notifications.ChannelType {
	return notifications.ChannelOpsgenie
}

// Send delivers a single message to Opsgenie.
func (a *Adapter) Send(ctx context.Context, msg notifications.Message) notifications.DeliveryResult {
	action := msg.Action
	if action == "" {
		action = "trigger"
	}

	switch action {
	case "resolve":
		return a.closeAlert(ctx, msg)
	case "acknowledge":
		return a.acknowledgeAlert(ctx, msg)
	default:
		return a.createAlert(ctx, msg)
	}
}

// BatchSend delivers multiple messages sequentially.
func (a *Adapter) BatchSend(ctx context.Context, msgs []notifications.Message) []notifications.DeliveryResult {
	results := make([]notifications.DeliveryResult, 0, len(msgs))
	for _, msg := range msgs {
		results = append(results, a.Send(ctx, msg))
	}
	return results
}

// CheckHealth verifies the API key by listing alerts (limit 1).
func (a *Adapter) CheckHealth(ctx context.Context) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, a.apiURL+"?limit=1", nil)
	if err != nil {
		return fmt.Errorf("opsgenie health: %w", err)
	}
	req.Header.Set("Authorization", "GenieKey "+a.apiKey)

	resp, err := a.client.Do(req)
	if err != nil {
		return fmt.Errorf("opsgenie health: %w", err)
	}
	defer resp.Body.Close()
	io.Copy(io.Discard, resp.Body) //nolint:errcheck

	if resp.StatusCode == http.StatusUnauthorized || resp.StatusCode == http.StatusForbidden {
		return fmt.Errorf("opsgenie health: invalid API key (HTTP %d)", resp.StatusCode)
	}
	return nil
}

func (a *Adapter) createAlert(ctx context.Context, msg notifications.Message) notifications.DeliveryResult {
	alert := opsgenieAlert{
		Message:     msg.Summary,
		Description: msg.Body,
		Alias:       msg.DedupKey,
		Priority:    mapPriority(msg.Severity),
		Source:      fmt.Sprintf("kaivue/%s", msg.TenantID),
		Tags:        []string{msg.EventType, string(msg.Severity)},
		Details:     msg.Extra,
	}

	return a.doPost(ctx, a.apiURL, msg.ID, alert)
}

func (a *Adapter) closeAlert(ctx context.Context, msg notifications.Message) notifications.DeliveryResult {
	if msg.DedupKey == "" {
		return fail(msg.ID, "opsgenie: dedup_key required for resolve")
	}
	url := fmt.Sprintf("%s/%s/close?identifierType=alias", a.apiURL, msg.DedupKey)
	body := map[string]string{"source": fmt.Sprintf("kaivue/%s", msg.TenantID)}
	return a.doPost(ctx, url, msg.ID, body)
}

func (a *Adapter) acknowledgeAlert(ctx context.Context, msg notifications.Message) notifications.DeliveryResult {
	if msg.DedupKey == "" {
		return fail(msg.ID, "opsgenie: dedup_key required for acknowledge")
	}
	url := fmt.Sprintf("%s/%s/acknowledge?identifierType=alias", a.apiURL, msg.DedupKey)
	body := map[string]string{"source": fmt.Sprintf("kaivue/%s", msg.TenantID)}
	return a.doPost(ctx, url, msg.ID, body)
}

func (a *Adapter) doPost(ctx context.Context, url, msgID string, payload interface{}) notifications.DeliveryResult {
	body, err := json.Marshal(payload)
	if err != nil {
		return fail(msgID, fmt.Sprintf("marshal: %v", err))
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(body))
	if err != nil {
		return fail(msgID, fmt.Sprintf("new request: %v", err))
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "GenieKey "+a.apiKey)

	resp, err := a.client.Do(req)
	if err != nil {
		return fail(msgID, fmt.Sprintf("post: %v", err))
	}
	defer resp.Body.Close()
	io.Copy(io.Discard, resp.Body) //nolint:errcheck

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return fail(msgID, fmt.Sprintf("opsgenie returned HTTP %d", resp.StatusCode))
	}

	return notifications.DeliveryResult{
		MessageID:   msgID,
		ChannelType: notifications.ChannelOpsgenie,
		State:       notifications.DeliverySuccess,
	}
}

func fail(msgID, errMsg string) notifications.DeliveryResult {
	return notifications.DeliveryResult{
		MessageID:    msgID,
		ChannelType:  notifications.ChannelOpsgenie,
		State:        notifications.DeliveryFailure,
		ErrorMessage: errMsg,
	}
}

// Opsgenie Alert API types.

type opsgenieAlert struct {
	Message     string            `json:"message"`
	Description string            `json:"description,omitempty"`
	Alias       string            `json:"alias,omitempty"`
	Priority    string            `json:"priority"`
	Source      string            `json:"source,omitempty"`
	Tags        []string          `json:"tags,omitempty"`
	Details     map[string]string `json:"details,omitempty"`
}

func mapPriority(s notifications.Severity) string {
	switch s {
	case notifications.SeverityCritical:
		return "P1"
	case notifications.SeverityHigh:
		return "P2"
	case notifications.SeverityWarning:
		return "P3"
	default:
		return "P5"
	}
}
