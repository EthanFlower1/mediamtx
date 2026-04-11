// Package teams implements a DeliveryChannel adapter for Microsoft Teams
// using connector webhooks with Adaptive Card formatting.
package teams

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"

	"github.com/bluenviron/mediamtx/internal/cloud/notifications"
)

// Config configures the Teams adapter.
type Config struct {
	// WebhookURL is the Teams connector / incoming webhook URL.
	WebhookURL string
	// HTTPClient overrides the default HTTP client.
	HTTPClient *http.Client
}

// Adapter delivers notifications to Microsoft Teams via connector webhooks.
type Adapter struct {
	webhookURL string
	client     *http.Client
}

// New creates a Teams adapter.
func New(cfg Config) (*Adapter, error) {
	if cfg.WebhookURL == "" {
		return nil, fmt.Errorf("teams: webhook URL is required")
	}
	client := cfg.HTTPClient
	if client == nil {
		client = http.DefaultClient
	}
	return &Adapter{
		webhookURL: cfg.WebhookURL,
		client:     client,
	}, nil
}

// Type returns the channel type.
func (a *Adapter) Type() notifications.ChannelType {
	return notifications.ChannelTeams
}

// Send delivers a single message to Teams.
func (a *Adapter) Send(ctx context.Context, msg notifications.CommsMessage) notifications.CommsDeliveryResult {
	payload := buildAdaptiveCard(msg)
	body, err := json.Marshal(payload)
	if err != nil {
		return fail(msg.ID, fmt.Sprintf("marshal: %v", err))
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, a.webhookURL, bytes.NewReader(body))
	if err != nil {
		return fail(msg.ID, fmt.Sprintf("new request: %v", err))
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := a.client.Do(req)
	if err != nil {
		return fail(msg.ID, fmt.Sprintf("post: %v", err))
	}
	defer resp.Body.Close()
	io.Copy(io.Discard, resp.Body) //nolint:errcheck

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return fail(msg.ID, fmt.Sprintf("teams returned HTTP %d", resp.StatusCode))
	}

	return notifications.CommsDeliveryResult{
		MessageID:   msg.ID,
		ChannelType: notifications.ChannelTeams,
		State:       notifications.CommsDeliverySuccess,
	}
}

// BatchSend delivers multiple messages sequentially.
func (a *Adapter) BatchSend(ctx context.Context, msgs []notifications.CommsMessage) []notifications.CommsDeliveryResult {
	results := make([]notifications.CommsDeliveryResult, 0, len(msgs))
	for _, msg := range msgs {
		results = append(results, a.Send(ctx, msg))
	}
	return results
}

// CheckHealth verifies the webhook URL is reachable.
func (a *Adapter) CheckHealth(ctx context.Context) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, a.webhookURL, bytes.NewReader([]byte(`{}`)))
	if err != nil {
		return fmt.Errorf("teams health: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := a.client.Do(req)
	if err != nil {
		return fmt.Errorf("teams health: %w", err)
	}
	defer resp.Body.Close()
	io.Copy(io.Discard, resp.Body) //nolint:errcheck
	if resp.StatusCode == http.StatusNotFound {
		return fmt.Errorf("teams health: webhook not found (404)")
	}
	return nil
}

func fail(msgID, errMsg string) notifications.CommsDeliveryResult {
	return notifications.CommsDeliveryResult{
		MessageID:    msgID,
		ChannelType:  notifications.ChannelTeams,
		State:        notifications.CommsDeliveryFailure,
		ErrorMessage: errMsg,
	}
}

// Adaptive Card types for Microsoft Teams.

type adaptiveCardEnvelope struct {
	Type        string         `json:"type"`
	Attachments []attachment   `json:"attachments"`
}

type attachment struct {
	ContentType string       `json:"contentType"`
	ContentURL  *string      `json:"contentUrl"`
	Content     adaptiveCard `json:"content"`
}

type adaptiveCard struct {
	Schema  string        `json:"$schema"`
	Type    string        `json:"type"`
	Version string        `json:"version"`
	Body    []cardElement `json:"body"`
	Actions []cardAction  `json:"actions,omitempty"`
}

type cardElement struct {
	Type    string `json:"type"`
	Text    string `json:"text,omitempty"`
	Size    string `json:"size,omitempty"`
	Weight  string `json:"weight,omitempty"`
	Color   string `json:"color,omitempty"`
	Wrap    bool   `json:"wrap,omitempty"`
	Spacing string `json:"spacing,omitempty"`
}

type cardAction struct {
	Type  string `json:"type"`
	Title string `json:"title"`
	URL   string `json:"url"`
}

func severityColor(s notifications.Severity) string {
	switch s {
	case notifications.SeverityCritical:
		return "attention"
	case notifications.SeverityHigh:
		return "warning"
	case notifications.SeverityWarning:
		return "warning"
	default:
		return "default"
	}
}

func buildAdaptiveCard(msg notifications.CommsMessage) adaptiveCardEnvelope {
	card := adaptiveCard{
		Schema:  "http://adaptivecards.io/schemas/adaptive-card.json",
		Type:    "AdaptiveCard",
		Version: "1.4",
		Body: []cardElement{
			{
				Type:   "TextBlock",
				Text:   msg.Summary,
				Size:   "Large",
				Weight: "Bolder",
				Color:  severityColor(msg.Severity),
				Wrap:   true,
			},
			{
				Type:    "TextBlock",
				Text:    msg.Body,
				Wrap:    true,
				Spacing: "Medium",
			},
			{
				Type:    "TextBlock",
				Text:    fmt.Sprintf("**Event:** %s | **Severity:** %s | **Tenant:** %s", msg.EventType, msg.Severity, msg.TenantID),
				Wrap:    true,
				Spacing: "Small",
				Size:    "Small",
				Color:   "default",
			},
		},
	}

	if msg.ActionURL != "" {
		card.Actions = []cardAction{
			{
				Type:  "Action.OpenUrl",
				Title: "View Details",
				URL:   msg.ActionURL,
			},
		}
	}

	return adaptiveCardEnvelope{
		Type: "message",
		Attachments: []attachment{
			{
				ContentType: "application/vnd.microsoft.card.adaptive",
				ContentURL:  nil,
				Content:     card,
			},
		},
	}
}
