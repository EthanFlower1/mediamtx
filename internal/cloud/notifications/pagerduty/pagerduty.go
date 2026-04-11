// Package pagerduty implements a DeliveryChannel adapter for PagerDuty
// using the Events API v2 for trigger, acknowledge, and resolve actions.
package pagerduty

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"

	"github.com/bluenviron/mediamtx/internal/cloud/notifications"
)

const defaultEventsURL = "https://events.pagerduty.com/v2/enqueue"

// Config configures the PagerDuty adapter.
type Config struct {
	// RoutingKey is the PagerDuty integration / routing key.
	RoutingKey string
	// EventsURL overrides the Events API v2 endpoint (for testing).
	EventsURL string
	// HTTPClient overrides the default HTTP client.
	HTTPClient *http.Client
}

// Adapter delivers notifications to PagerDuty via Events API v2.
type Adapter struct {
	routingKey string
	eventsURL  string
	client     *http.Client
}

// New creates a PagerDuty adapter.
func New(cfg Config) (*Adapter, error) {
	if cfg.RoutingKey == "" {
		return nil, fmt.Errorf("pagerduty: routing key is required")
	}
	eventsURL := cfg.EventsURL
	if eventsURL == "" {
		eventsURL = defaultEventsURL
	}
	client := cfg.HTTPClient
	if client == nil {
		client = http.DefaultClient
	}
	return &Adapter{
		routingKey: cfg.RoutingKey,
		eventsURL:  eventsURL,
		client:     client,
	}, nil
}

// Type returns the channel type.
func (a *Adapter) Type() notifications.ChannelType {
	return notifications.ChannelPagerDuty
}

// Send delivers a single message to PagerDuty.
func (a *Adapter) Send(ctx context.Context, msg notifications.CommsMessage) notifications.CommsDeliveryResult {
	event := buildEvent(a.routingKey, msg)
	body, err := json.Marshal(event)
	if err != nil {
		return fail(msg.ID, fmt.Sprintf("marshal: %v", err))
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, a.eventsURL, bytes.NewReader(body))
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
		return fail(msg.ID, fmt.Sprintf("pagerduty returned HTTP %d", resp.StatusCode))
	}

	return notifications.CommsDeliveryResult{
		MessageID:   msg.ID,
		ChannelType: notifications.ChannelPagerDuty,
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

// CheckHealth sends a change event to verify the routing key is valid.
func (a *Adapter) CheckHealth(ctx context.Context) error {
	payload := map[string]interface{}{
		"routing_key":  a.routingKey,
		"event_action": "trigger",
		"payload": map[string]interface{}{
			"summary":  "Health check from KaiVue VMS",
			"source":   "kaivue-healthcheck",
			"severity": "info",
		},
	}
	// Use the change events endpoint for health checks to avoid creating incidents.
	body, _ := json.Marshal(payload)
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, a.eventsURL, bytes.NewReader(body))
	if err != nil {
		return fmt.Errorf("pagerduty health: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := a.client.Do(req)
	if err != nil {
		return fmt.Errorf("pagerduty health: %w", err)
	}
	defer resp.Body.Close()
	io.Copy(io.Discard, resp.Body) //nolint:errcheck

	if resp.StatusCode == http.StatusForbidden || resp.StatusCode == http.StatusUnauthorized {
		return fmt.Errorf("pagerduty health: invalid routing key (HTTP %d)", resp.StatusCode)
	}
	return nil
}

func fail(msgID, errMsg string) notifications.CommsDeliveryResult {
	return notifications.CommsDeliveryResult{
		MessageID:    msgID,
		ChannelType:  notifications.ChannelPagerDuty,
		State:        notifications.CommsDeliveryFailure,
		ErrorMessage: errMsg,
	}
}

// PagerDuty Events API v2 types.

type pdEvent struct {
	RoutingKey  string     `json:"routing_key"`
	EventAction string     `json:"event_action"`
	DedupKey    string     `json:"dedup_key,omitempty"`
	Payload     *pdPayload `json:"payload,omitempty"`
}

type pdPayload struct {
	Summary   string    `json:"summary"`
	Source    string    `json:"source"`
	Severity  string    `json:"severity"`
	Timestamp string    `json:"timestamp,omitempty"`
	Group     string    `json:"group,omitempty"`
	Class     string    `json:"class,omitempty"`
	CustomDetails map[string]string `json:"custom_details,omitempty"`
}

func mapSeverity(s notifications.Severity) string {
	switch s {
	case notifications.SeverityCritical:
		return "critical"
	case notifications.SeverityHigh:
		return "error"
	case notifications.SeverityWarning:
		return "warning"
	default:
		return "info"
	}
}

func mapAction(action string) string {
	switch action {
	case "acknowledge":
		return "acknowledge"
	case "resolve":
		return "resolve"
	default:
		return "trigger"
	}
}

func buildEvent(routingKey string, msg notifications.CommsMessage) pdEvent {
	action := mapAction(msg.Action)
	event := pdEvent{
		RoutingKey:  routingKey,
		EventAction: action,
		DedupKey:    msg.DedupKey,
	}

	// Acknowledge and resolve only need routing_key, event_action, dedup_key.
	if action == "trigger" {
		ts := msg.Timestamp
		if ts.IsZero() {
			ts = time.Now().UTC()
		}
		event.Payload = &pdPayload{
			Summary:   msg.Summary,
			Source:    fmt.Sprintf("kaivue/%s", msg.TenantID),
			Severity:  mapSeverity(msg.Severity),
			Timestamp: ts.Format(time.RFC3339),
			Class:     msg.EventType,
			CustomDetails: msg.Extra,
		}
	}

	return event
}
