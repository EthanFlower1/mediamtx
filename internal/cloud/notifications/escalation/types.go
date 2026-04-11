package escalation

import "time"

// TargetType identifies who receives a notification at a given tier.
type TargetType string

const (
	TargetUser      TargetType = "user"
	TargetGroup     TargetType = "group"
	TargetPagerDuty TargetType = "pagerduty"
)

// ChannelType enumerates delivery mechanisms for an escalation step.
type ChannelType string

const (
	ChannelEmail    ChannelType = "email"
	ChannelPush     ChannelType = "push"
	ChannelSMS      ChannelType = "sms"
	ChannelWebhook  ChannelType = "webhook"
	ChannelPagerDuty ChannelType = "pagerduty"
)

// AlertState is the state of a per-alert escalation tracker.
type AlertState string

const (
	StatePending          AlertState = "pending"
	StateNotified         AlertState = "notified"
	StateTimeout          AlertState = "timeout"
	StateAcknowledged     AlertState = "acknowledged"
	StateResolved         AlertState = "resolved"
	StatePagerDutyFallback AlertState = "pagerduty_fallback"
	StateExhausted        AlertState = "exhausted"
)

// Chain is a named escalation ruleset belonging to a tenant.
type Chain struct {
	ChainID     string    `json:"chain_id"`
	TenantID    string    `json:"tenant_id"`
	Name        string    `json:"name"`
	Description string    `json:"description"`
	Enabled     bool      `json:"enabled"`
	CreatedAt   time.Time `json:"created_at"`
	UpdatedAt   time.Time `json:"updated_at"`
}

// Step is one tier within an escalation chain.
type Step struct {
	StepID         string      `json:"step_id"`
	ChainID        string      `json:"chain_id"`
	TenantID       string      `json:"tenant_id"`
	StepOrder      int         `json:"step_order"`
	TargetType     TargetType  `json:"target_type"`
	TargetID       string      `json:"target_id"`
	ChannelType    ChannelType `json:"channel_type"`
	TimeoutSeconds int         `json:"timeout_seconds"`
	CreatedAt      time.Time   `json:"created_at"`
}

// AlertEscalation tracks the state machine of a single alert being
// escalated through a chain.
type AlertEscalation struct {
	AlertID        string      `json:"alert_id"`
	TenantID       string      `json:"tenant_id"`
	ChainID        string      `json:"chain_id"`
	CurrentStep    int         `json:"current_step"`
	State          AlertState  `json:"state"`
	AckedBy        string      `json:"acked_by,omitempty"`
	AckedAt        *time.Time  `json:"acked_at,omitempty"`
	NextEscalation *time.Time  `json:"next_escalation,omitempty"`
	CreatedAt      time.Time   `json:"created_at"`
	UpdatedAt      time.Time   `json:"updated_at"`
}

// Notifier is the interface for actually sending notifications.
// The escalation service calls Notify when it needs to deliver at a tier.
type Notifier interface {
	// Notify sends a notification for the given alert to the specified step target.
	// It returns an error if delivery fails.
	Notify(alertID string, step Step) error
}

// PagerDutyClient is the interface for creating PagerDuty incidents.
type PagerDutyClient interface {
	// CreateIncident creates a PagerDuty incident for the given alert.
	CreateIncident(alertID, tenantID, description string) error
}
