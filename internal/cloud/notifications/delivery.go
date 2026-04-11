package notifications

import (
	"context"
	"time"
)

// DeliveryChannel is the interface that all notification delivery adapters
// (FCM, APNs, Web Push, email, SMS, webhook) must implement. KAI-476 defined
// the contract; KAI-477 adds the push-notification implementations.
type DeliveryChannel interface {
	// Type returns the channel type this adapter handles.
	Type() ChannelType

	// Send delivers a single message to a single recipient device/endpoint.
	Send(ctx context.Context, msg Message) (DeliveryResult, error)

	// BatchSend delivers a message to multiple recipient devices/endpoints.
	// Implementations should return one DeliveryResult per target, in the
	// same order as msg.Targets.
	BatchSend(ctx context.Context, msg BatchMessage) ([]DeliveryResult, error)

	// CheckHealth verifies that the adapter's backend (FCM API, APNs
	// gateway, etc.) is reachable and credentials are valid.
	CheckHealth(ctx context.Context) error
}

// Message is a single push notification to be delivered.
type Message struct {
	// MessageID is a caller-assigned idempotency key.
	MessageID string `json:"message_id"`
	TenantID  string `json:"tenant_id"`
	UserID    string `json:"user_id"`

	// Target is the device token, push subscription, or endpoint URL.
	Target string `json:"target"`

	// Platform disambiguates the target format when routing through a
	// generic dispatcher.
	Platform Platform `json:"platform"`

	Title    string            `json:"title"`
	Body     string            `json:"body"`
	Data     map[string]string `json:"data,omitempty"`
	ImageURL string            `json:"image_url,omitempty"`

	// Priority: "high" or "normal". Adapters map to platform equivalents.
	Priority string `json:"priority,omitempty"`

	// TTL is the time-to-live for the message on the push gateway.
	TTL time.Duration `json:"ttl,omitempty"`

	// CollapseKey groups messages so only the latest is delivered if the
	// device is offline.
	CollapseKey string `json:"collapse_key,omitempty"`

	// Badge is the app icon badge count (APNs / FCM Android).
	Badge *int `json:"badge,omitempty"`

	// Sound is the notification sound name.
	Sound string `json:"sound,omitempty"`
}

// BatchMessage wraps a notification payload with multiple targets.
type BatchMessage struct {
	// MessageID is a caller-assigned idempotency key for the batch.
	MessageID string   `json:"message_id"`
	TenantID  string   `json:"tenant_id"`
	Targets   []Target `json:"targets"`

	Title    string            `json:"title"`
	Body     string            `json:"body"`
	Data     map[string]string `json:"data,omitempty"`
	ImageURL string            `json:"image_url,omitempty"`
	Priority string            `json:"priority,omitempty"`
	TTL      time.Duration     `json:"ttl,omitempty"`

	CollapseKey string `json:"collapse_key,omitempty"`
	Badge       *int   `json:"badge,omitempty"`
	Sound       string `json:"sound,omitempty"`
}

// Target identifies a single push recipient within a batch.
type Target struct {
	UserID      string   `json:"user_id"`
	DeviceToken string   `json:"device_token"`
	Platform    Platform `json:"platform"`
}

// Platform identifies the push notification platform.
type Platform string

const (
	PlatformFCM     Platform = "fcm"
	PlatformAPNs    Platform = "apns"
	PlatformWebPush Platform = "webpush"
)

// DeliveryResult records the outcome of delivering to a single target.
type DeliveryResult struct {
	Target       string        `json:"target"`
	State        DeliveryState `json:"state"`
	PlatformID   string        `json:"platform_id,omitempty"`
	ErrorCode    string        `json:"error_code,omitempty"`
	ErrorMessage string        `json:"error_message,omitempty"`

	// ShouldRemoveToken is true when the push gateway reports the token as
	// invalid or unregistered. The caller should deregister the device.
	ShouldRemoveToken bool `json:"should_remove_token,omitempty"`
}

// DeliveryState is the terminal state after a single send attempt.
type DeliveryState string

const (
	StateDelivered   DeliveryState = "delivered"
	StateFailed      DeliveryState = "failed"
	StateThrottled   DeliveryState = "throttled"
	StateUnreachable DeliveryState = "unreachable"
)

// ChannelRegistry provides type-based routing to registered DeliveryChannel
// implementations.
type ChannelRegistry struct {
	channels map[ChannelType]DeliveryChannel
}

// NewChannelRegistry creates an empty registry.
func NewChannelRegistry() *ChannelRegistry {
	return &ChannelRegistry{channels: make(map[ChannelType]DeliveryChannel)}
}

// Register adds a delivery channel adapter to the registry.
func (r *ChannelRegistry) Register(ch DeliveryChannel) {
	r.channels[ch.Type()] = ch
}

// Get returns the registered channel for the given type, or nil.
func (r *ChannelRegistry) Get(ct ChannelType) DeliveryChannel {
	return r.channels[ct]
}

// All returns every registered channel.
func (r *ChannelRegistry) All() map[ChannelType]DeliveryChannel {
	out := make(map[ChannelType]DeliveryChannel, len(r.channels))
	for k, v := range r.channels {
		out[k] = v
	}
	return out
}
