package delivery

import "time"

// ProviderName identifies an external delivery provider.
type ProviderName string

const (
	ProviderSES    ProviderName = "ses"
	ProviderTwilio ProviderName = "twilio"
)

// Message is the payload passed to a Sender.
type Message struct {
	TenantID  string
	UserID    string
	EventType string
	To        string // email address or phone number
	Subject   string // email only
	Body      string
}

// Result is the outcome of a single delivery attempt.
type Result struct {
	MessageID string // provider-assigned message ID (e.g. SES message ID)
	Status    string // "sent" or "failed"
	Error     error
}

// RateLimit defines the per-tenant, per-channel rate limit configuration.
type RateLimit struct {
	RateLimitID   string    `json:"rate_limit_id"`
	TenantID      string    `json:"tenant_id"`
	ChannelType   string    `json:"channel_type"`
	WindowSeconds int       `json:"window_seconds"`
	MaxCount      int       `json:"max_count"`
	Burst         int       `json:"burst"`
	Enabled       bool      `json:"enabled"`
	CreatedAt     time.Time `json:"created_at"`
	UpdatedAt     time.Time `json:"updated_at"`
}

// ProviderConfig holds the per-tenant delivery provider credentials.
type ProviderConfig struct {
	ProviderID   string       `json:"provider_id"`
	TenantID     string       `json:"tenant_id"`
	ChannelType  string       `json:"channel_type"`
	ProviderName ProviderName `json:"provider_name"`
	Credentials  string       `json:"credentials"`
	FromAddress  string       `json:"from_address"`
	Enabled      bool         `json:"enabled"`
	CreatedAt    time.Time    `json:"created_at"`
	UpdatedAt    time.Time    `json:"updated_at"`
}
