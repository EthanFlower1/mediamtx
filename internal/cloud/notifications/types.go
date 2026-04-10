package notifications

import "time"

// ChannelType enumerates supported notification delivery channels.
type ChannelType string

const (
	ChannelEmail    ChannelType = "email"
	ChannelPush     ChannelType = "push"
	ChannelSMS      ChannelType = "sms"
	ChannelVoice    ChannelType = "voice"
	ChannelWhatsApp ChannelType = "whatsapp"
	ChannelWebhook  ChannelType = "webhook"
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
