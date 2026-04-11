// Package notifications — channel.go defines the Channel delivery interface
// (KAI-476). This is the contract consumed by KAI-477 (push) and KAI-478
// (Slack/Teams/PagerDuty/webhook). Changing signatures here requires
// coordinating with those downstream tickets.
package notifications

import (
	"context"
	"errors"
	"fmt"
	"time"
)

// -----------------------------------------------------------------------
// Message types
// -----------------------------------------------------------------------

// MessageType identifies the delivery medium of a message.
type MessageType string

const (
	MessageTypeEmail    MessageType = "email"
	MessageTypeSMS      MessageType = "sms"
	MessageTypeVoice    MessageType = "voice"
	MessageTypeWhatsApp MessageType = "whatsapp"
)

// Priority controls delivery urgency. Channels MAY map this to
// provider-specific priority headers (e.g., SendGrid "high" priority,
// Twilio priority routing).
type Priority int

const (
	PriorityLow      Priority = 0
	PriorityNormal   Priority = 1
	PriorityHigh     Priority = 2
	PriorityCritical Priority = 3
)

// Recipient identifies who should receive the message.
type Recipient struct {
	// Address is the delivery endpoint — email address, phone number
	// (E.164), or provider-specific identifier.
	Address string `json:"address"`
	// Name is an optional display name (used in email "To:" header).
	Name string `json:"name,omitempty"`
	// Metadata carries provider-specific fields that do not warrant
	// first-class struct members (e.g., language preference for
	// Twilio voice).
	Metadata map[string]string `json:"metadata,omitempty"`
}

// Message is the universal payload handed to a Channel for delivery.
// Each Channel adapter maps the relevant fields to its provider's API.
type Message struct {
	// ID is a caller-assigned unique identifier used for idempotent
	// delivery. If empty the Dispatcher will generate one. The same
	// ID submitted twice MUST NOT result in duplicate delivery.
	ID string `json:"id"`

	// Type declares the intended medium. A Channel MUST reject
	// messages whose type it does not support.
	Type MessageType `json:"type"`

	// TenantID scopes the message to a tenant. Channels that
	// support per-tenant sender identity (SendGrid subuser,
	// Twilio messaging service SID, etc.) use this to select
	// the correct credentials.
	TenantID string `json:"tenant_id"`

	// To is the list of recipients. BatchSend fans out one provider
	// call per recipient; Send expects exactly one.
	To []Recipient `json:"to"`

	// Subject is used by email; ignored by SMS/voice/WhatsApp.
	Subject string `json:"subject,omitempty"`

	// Body is the plain-text body (SMS, WhatsApp text, email
	// plain-text part, voice TwiML <Say> text).
	Body string `json:"body"`

	// HTMLBody is the rich HTML body. Used only by email.
	HTMLBody string `json:"html_body,omitempty"`

	// TemplateID is a provider-side template identifier (SendGrid
	// dynamic template, Twilio content template SID). When set the
	// provider renders the template with TemplateData and Body is
	// used as fallback only.
	TemplateID string `json:"template_id,omitempty"`

	// TemplateData is the key-value substitution data for the
	// template.
	TemplateData map[string]string `json:"template_data,omitempty"`

	// Priority controls delivery urgency.
	Priority Priority `json:"priority"`

	// Metadata carries arbitrary KV pairs forwarded to the
	// provider (custom headers, analytics tags, etc.).
	Metadata map[string]string `json:"metadata,omitempty"`

	// CallbackURL is an optional URL the provider should POST
	// delivery status webhooks to. Channels that do not support
	// callbacks ignore this field.
	CallbackURL string `json:"callback_url,omitempty"`
}

// Validate performs cheap client-side validation before hitting a
// provider API.
func (m *Message) Validate() error {
	if m.Type == "" {
		return errors.New("notifications: message type is required")
	}
	if m.TenantID == "" {
		return errors.New("notifications: tenant_id is required")
	}
	if len(m.To) == 0 {
		return errors.New("notifications: at least one recipient is required")
	}
	for i, r := range m.To {
		if r.Address == "" {
			return fmt.Errorf("notifications: recipient %d: address is required", i)
		}
	}
	if m.Body == "" && m.TemplateID == "" {
		return errors.New("notifications: body or template_id is required")
	}
	return nil
}

// -----------------------------------------------------------------------
// Delivery status lifecycle
// -----------------------------------------------------------------------

// DeliveryState is the lifecycle state of a single delivery attempt.
// The state machine is:
//
//	Queued -> Sending -> Delivered
//	                  -> Failed -> (retry) -> Sending
//	                            -> DeadLettered
//	       -> Suppressed (opt-out / bounce list)
type DeliveryState string

const (
	DeliveryStateQueued       DeliveryState = "queued"
	DeliveryStateSending      DeliveryState = "sending"
	DeliveryStateDelivered    DeliveryState = "delivered"
	DeliveryStateFailed       DeliveryState = "failed"
	DeliveryStateDeadLettered DeliveryState = "dead_lettered"
	DeliveryStateSuppressed   DeliveryState = "suppressed"
)

// DeliveryResult is returned by Send / BatchSend with the outcome
// of each recipient.
type DeliveryResult struct {
	// MessageID is the caller-supplied or auto-generated message ID.
	MessageID string `json:"message_id"`

	// ProviderMessageID is the provider's own identifier (e.g.,
	// SendGrid x-message-id, Twilio message SID).
	ProviderMessageID string `json:"provider_message_id,omitempty"`

	// Recipient is the address this result pertains to.
	Recipient string `json:"recipient"`

	// State is the terminal state of this delivery attempt.
	State DeliveryState `json:"state"`

	// Error is non-nil when State == DeliveryStateFailed.
	Error error `json:"-"`

	// ErrorMessage is the string form of Error, safe for JSON
	// serialisation and logging.
	ErrorMessage string `json:"error_message,omitempty"`

	// Timestamp is when the provider acknowledged the request.
	Timestamp time.Time `json:"timestamp"`
}

// -----------------------------------------------------------------------
// Health check
// -----------------------------------------------------------------------

// HealthStatus summarises a channel's operational readiness.
type HealthStatus struct {
	// Healthy is true when the channel can accept traffic.
	Healthy bool `json:"healthy"`

	// Latency is the measured round-trip to the provider health
	// endpoint (or a synthetic probe).
	Latency time.Duration `json:"latency_ms"`

	// Message is a human-readable status summary.
	Message string `json:"message,omitempty"`

	// CheckedAt is when the probe was executed.
	CheckedAt time.Time `json:"checked_at"`
}

// -----------------------------------------------------------------------
// DeliveryChannel interface — THE contract
// -----------------------------------------------------------------------

// DeliveryChannel is the core abstraction that every notification
// provider adapter must implement. It is consumed by the Dispatcher
// (this package) and by downstream channel tickets (KAI-477, KAI-478).
//
// Implementations MUST:
//   - Be safe for concurrent use.
//   - Honour the Message.ID for idempotent delivery.
//   - Return a DeliveryResult per recipient (BatchSend) or a single
//     result (Send).
//   - Map provider errors to the standard DeliveryState values.
//   - Export Prometheus metrics through the shared MetricsCollector.
type DeliveryChannel interface {
	// Name returns a stable, lowercase identifier for the channel
	// (e.g., "sendgrid", "twilio_sms", "twilio_voice"). Used as the
	// Prometheus "channel" label.
	Name() string

	// SupportedTypes returns the MessageTypes this channel can deliver.
	SupportedTypes() []MessageType

	// Send delivers a single message to a single recipient. It is
	// a convenience wrapper; the default implementation delegates
	// to BatchSend.
	Send(ctx context.Context, msg Message) (DeliveryResult, error)

	// BatchSend delivers a message to all recipients in msg.To.
	// Returns one DeliveryResult per recipient. The channel SHOULD
	// use provider batch APIs where available.
	BatchSend(ctx context.Context, msg Message) ([]DeliveryResult, error)

	// CheckHealth probes the provider and returns operational status.
	// Implementations SHOULD complete within 5 seconds.
	CheckHealth(ctx context.Context) (HealthStatus, error)
}

// -----------------------------------------------------------------------
// Channel registry
// -----------------------------------------------------------------------

// ChannelRegistry holds all registered DeliveryChannel implementations
// and routes messages to the appropriate channel.
type ChannelRegistry struct {
	channels map[string]DeliveryChannel
}

// NewChannelRegistry creates an empty registry.
func NewChannelRegistry() *ChannelRegistry {
	return &ChannelRegistry{channels: make(map[string]DeliveryChannel)}
}

// Register adds a channel. Panics on duplicate name (programming error).
func (r *ChannelRegistry) Register(ch DeliveryChannel) {
	name := ch.Name()
	if _, ok := r.channels[name]; ok {
		panic(fmt.Sprintf("notifications: duplicate channel registration: %q", name))
	}
	r.channels[name] = ch
}

// Get returns the channel registered under name, or nil.
func (r *ChannelRegistry) Get(name string) DeliveryChannel {
	return r.channels[name]
}

// ForType returns all channels that support the given message type.
func (r *ChannelRegistry) ForType(mt MessageType) []DeliveryChannel {
	var out []DeliveryChannel
	for _, ch := range r.channels {
		for _, t := range ch.SupportedTypes() {
			if t == mt {
				out = append(out, ch)
				break
			}
		}
	}
	return out
}

// All returns every registered channel.
func (r *ChannelRegistry) All() []DeliveryChannel {
	out := make([]DeliveryChannel, 0, len(r.channels))
	for _, ch := range r.channels {
		out = append(out, ch)
	}
	return out
}

// HealthCheckAll probes every registered channel and returns results
// keyed by channel name.
func (r *ChannelRegistry) HealthCheckAll(ctx context.Context) map[string]HealthStatus {
	results := make(map[string]HealthStatus, len(r.channels))
	for name, ch := range r.channels {
		hs, err := ch.CheckHealth(ctx)
		if err != nil {
			hs = HealthStatus{
				Healthy:   false,
				Message:   err.Error(),
				CheckedAt: time.Now().UTC(),
			}
		}
		results[name] = hs
	}
	return results
}
