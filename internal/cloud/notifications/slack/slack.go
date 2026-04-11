// Package slack implements a DeliveryChannel adapter for Slack
// using incoming webhooks and Block Kit message formatting.
package slack

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"

	"github.com/bluenviron/mediamtx/internal/cloud/notifications"
)

// Config configures the Slack adapter.
type Config struct {
	// WebhookURL is the Slack incoming webhook URL.
	WebhookURL string
	// HTTPClient overrides the default HTTP client (useful for tests).
	HTTPClient *http.Client
}

// Adapter delivers notifications to Slack via incoming webhooks
// using Block Kit formatting.
type Adapter struct {
	webhookURL string
	client     *http.Client
}

// New creates a Slack adapter.
func New(cfg Config) (*Adapter, error) {
	if cfg.WebhookURL == "" {
		return nil, fmt.Errorf("slack: webhook URL is required")
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
	return notifications.ChannelSlack
}

// Send delivers a single message to Slack.
func (a *Adapter) Send(ctx context.Context, msg notifications.CommsMessage) notifications.CommsDeliveryResult {
	payload := buildBlockKit(msg)
	body, err := json.Marshal(payload)
	if err != nil {
		return notifications.CommsDeliveryResult{
			MessageID:    msg.ID,
			ChannelType:  notifications.ChannelSlack,
			State:        notifications.CommsDeliveryFailure,
			ErrorMessage: fmt.Sprintf("marshal: %v", err),
		}
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, a.webhookURL, bytes.NewReader(body))
	if err != nil {
		return notifications.CommsDeliveryResult{
			MessageID:    msg.ID,
			ChannelType:  notifications.ChannelSlack,
			State:        notifications.CommsDeliveryFailure,
			ErrorMessage: fmt.Sprintf("new request: %v", err),
		}
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := a.client.Do(req)
	if err != nil {
		return notifications.CommsDeliveryResult{
			MessageID:    msg.ID,
			ChannelType:  notifications.ChannelSlack,
			State:        notifications.CommsDeliveryFailure,
			ErrorMessage: fmt.Sprintf("post: %v", err),
		}
	}
	defer resp.Body.Close()
	io.Copy(io.Discard, resp.Body) //nolint:errcheck

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return notifications.CommsDeliveryResult{
			MessageID:    msg.ID,
			ChannelType:  notifications.ChannelSlack,
			State:        notifications.CommsDeliveryFailure,
			ErrorMessage: fmt.Sprintf("slack returned HTTP %d", resp.StatusCode),
		}
	}

	return notifications.CommsDeliveryResult{
		MessageID:   msg.ID,
		ChannelType: notifications.ChannelSlack,
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
		return fmt.Errorf("slack health: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := a.client.Do(req)
	if err != nil {
		return fmt.Errorf("slack health: %w", err)
	}
	defer resp.Body.Close()
	io.Copy(io.Discard, resp.Body) //nolint:errcheck
	// Slack returns 400 for empty payloads but the endpoint is reachable.
	// A 404 or network error means the webhook is invalid.
	if resp.StatusCode == http.StatusNotFound {
		return fmt.Errorf("slack health: webhook not found (404)")
	}
	return nil
}

// blockKitPayload is the Slack Block Kit message structure.
type blockKitPayload struct {
	Blocks []block `json:"blocks"`
}

type block struct {
	Type     string      `json:"type"`
	Text     *textObj    `json:"text,omitempty"`
	Elements []blockElem `json:"elements,omitempty"`
}

type textObj struct {
	Type string `json:"type"`
	Text string `json:"text"`
}

type blockElem struct {
	Type     string   `json:"type"`
	Text     *textObj `json:"text,omitempty"`
	URL      string   `json:"url,omitempty"`
	ActionID string   `json:"action_id,omitempty"`
}

func severityEmoji(s notifications.Severity) string {
	switch s {
	case notifications.SeverityCritical:
		return ":red_circle:"
	case notifications.SeverityHigh:
		return ":large_orange_circle:"
	case notifications.SeverityWarning:
		return ":large_yellow_circle:"
	default:
		return ":large_blue_circle:"
	}
}

func buildBlockKit(msg notifications.CommsMessage) blockKitPayload {
	blocks := []block{
		{
			Type: "header",
			Text: &textObj{Type: "plain_text", Text: fmt.Sprintf("%s %s", severityEmoji(msg.Severity), msg.Summary)},
		},
		{
			Type: "section",
			Text: &textObj{Type: "mrkdwn", Text: msg.Body},
		},
		{
			Type: "context",
			Elements: []blockElem{
				{Type: "mrkdwn", Text: &textObj{Type: "mrkdwn", Text: fmt.Sprintf("*Event:* %s | *Tenant:* %s", msg.EventType, msg.TenantID)}},
			},
		},
	}

	if msg.ActionURL != "" {
		blocks = append(blocks, block{
			Type: "actions",
			Elements: []blockElem{
				{
					Type:     "button",
					Text:     &textObj{Type: "plain_text", Text: "View Details"},
					URL:      msg.ActionURL,
					ActionID: "view_details",
				},
			},
		})
	}

	return blockKitPayload{Blocks: blocks}
}
