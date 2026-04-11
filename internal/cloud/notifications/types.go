package notifications

import (
	"context"
	"time"
)

// Severity indicates alert urgency for channels that support it (e.g. PagerDuty, Opsgenie).
type Severity string

const (
	SeverityCritical Severity = "critical"
	SeverityHigh     Severity = "high"
	SeverityWarning  Severity = "warning"
	SeverityInfo     Severity = "info"
)

// CommsMessage is the channel-agnostic notification payload handed to a
// CommsDeliveryChannel (Slack, Teams, PagerDuty, Opsgenie, Webhook).
type CommsMessage struct {
	// ID is a unique identifier for deduplication.
	ID string `json:"id"`
	// TenantID scopes the message to a tenant.
	TenantID string `json:"tenant_id"`
	// EventType classifies the alert (e.g. "camera.offline").
	EventType string `json:"event_type"`
	// Summary is a short human-readable title.
	Summary string `json:"summary"`
	// Body is the detailed description (may contain markdown).
	Body string `json:"body"`
	// Severity for incident-management channels.
	Severity Severity `json:"severity"`
	// DedupKey groups related events for PagerDuty/Opsgenie dedup.
	DedupKey string `json:"dedup_key,omitempty"`
	// Action indicates the desired incident lifecycle action.
	// Valid values: "trigger", "acknowledge", "resolve". Defaults to "trigger".
	Action string `json:"action,omitempty"`
	// ActionURL is an optional deep-link shown as a button in rich cards.
	ActionURL string `json:"action_url,omitempty"`
	// Timestamp is when the event occurred.
	Timestamp time.Time `json:"timestamp"`
	// Extra holds channel-specific overrides (e.g. custom fields).
	Extra map[string]string `json:"extra,omitempty"`
}

// CommsDeliveryState is the result state of a comms delivery attempt.
type CommsDeliveryState string

const (
	CommsDeliverySuccess CommsDeliveryState = "success"
	CommsDeliveryFailure CommsDeliveryState = "failure"
)

// CommsDeliveryResult is the outcome of sending a single CommsMessage to a channel.
type CommsDeliveryResult struct {
	MessageID    string             `json:"message_id"`
	ChannelType  ChannelType        `json:"channel_type"`
	State        CommsDeliveryState `json:"state"`
	ErrorMessage string             `json:"error_message,omitempty"`
}

// CommsDeliveryChannel is the adapter interface that every comms notification
// channel (Slack, Teams, PagerDuty, Opsgenie, Webhook) must implement.
type CommsDeliveryChannel interface {
	// Send delivers a single message. Implementations should return a
	// CommsDeliveryResult even on failure so callers can log the outcome.
	Send(ctx context.Context, msg CommsMessage) CommsDeliveryResult

	// BatchSend delivers multiple messages. The default implementation
	// calls Send in a loop but adapters may override for efficiency.
	BatchSend(ctx context.Context, msgs []CommsMessage) []CommsDeliveryResult

	// CheckHealth verifies that the channel endpoint is reachable.
	CheckHealth(ctx context.Context) error

	// Type returns the channel type identifier.
	Type() ChannelType
}

// ChannelType enumerates supported notification delivery channels.
type ChannelType string

const (
	ChannelEmail     ChannelType = "email"
	ChannelPush      ChannelType = "push"
	ChannelSMS       ChannelType = "sms"
	ChannelVoice     ChannelType = "voice"
	ChannelWhatsApp  ChannelType = "whatsapp"
	ChannelWebhook   ChannelType = "webhook"
	ChannelSlack     ChannelType = "slack"
	ChannelTeams     ChannelType = "teams"
	ChannelPagerDuty ChannelType = "pagerduty"
	ChannelOpsgenie  ChannelType = "opsgenie"
)

// Channel represents a configured notification delivery channel for a tenant.
type Channel struct {
	ChannelID   string      `json:"channel_id"`
	TenantID    string      `json:"tenant_id"`
	ChannelType ChannelType `json:"channel_type"`
	Config      string      `json:"config"`
	Enabled     bool        `json:"enabled"`
	CreatedAt   time.Time   `json:"created_at"`
	UpdatedAt   time.Time   `json:"updated_at"`
}

// Preference records whether a specific user wants a specific event type
// delivered via a specific channel type.
type Preference struct {
	PreferenceID string      `json:"preference_id"`
	TenantID     string      `json:"tenant_id"`
	UserID       string      `json:"user_id"`
	EventType    string      `json:"event_type"`
	ChannelType  ChannelType `json:"channel_type"`
	Enabled      bool        `json:"enabled"`
	CreatedAt    time.Time   `json:"created_at"`
	UpdatedAt    time.Time   `json:"updated_at"`
}

// DeliveryStatus tracks the outcome of a notification delivery attempt.
type DeliveryStatus string

const (
	StatusPending    DeliveryStatus = "pending"
	StatusSent       DeliveryStatus = "sent"
	StatusFailed     DeliveryStatus = "failed"
	StatusSuppressed DeliveryStatus = "suppressed"
)

// LogEntry records a single notification delivery attempt.
type LogEntry struct {
	LogID        string         `json:"log_id"`
	TenantID     string         `json:"tenant_id"`
	UserID       string         `json:"user_id"`
	EventType    string         `json:"event_type"`
	ChannelType  ChannelType    `json:"channel_type"`
	Status       DeliveryStatus `json:"status"`
	ErrorMessage string         `json:"error_message"`
	SentAt       *time.Time     `json:"sent_at,omitempty"`
	CreatedAt    time.Time      `json:"created_at"`
}

// DeliveryTarget pairs a user with a channel for a resolved notification.
type DeliveryTarget struct {
	UserID  string
	Channel Channel
}
