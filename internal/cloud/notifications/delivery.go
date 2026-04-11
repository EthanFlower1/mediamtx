package notifications

import (
	"context"
	"time"
)

// PushDeliveryChannel is the interface that all push notification delivery
// adapters (FCM, APNs, Web Push) must implement. This is distinct from
// DeliveryChannel (channel.go) which covers email/SMS/voice channels.
type PushDeliveryChannel interface {
	// Type returns the channel type this adapter handles.
	Type() ChannelType

	// Send delivers a single message to a single recipient device/endpoint.
	Send(ctx context.Context, msg PushMessage) (PushDeliveryResult, error)

	// BatchSend delivers a message to multiple recipient devices/endpoints.
	// Implementations should return one PushDeliveryResult per target, in the
	// same order as msg.Targets.
	BatchSend(ctx context.Context, msg BatchMessage) ([]PushDeliveryResult, error)

	// CheckHealth verifies that the adapter's backend (FCM API, APNs
	// gateway, etc.) is reachable and credentials are valid.
	CheckHealth(ctx context.Context) error
}

// PushMessage is a single push notification to be delivered.
type PushMessage struct {
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

// PushDeliveryResult records the outcome of delivering to a single target.
type PushDeliveryResult struct {
	Target       string             `json:"target"`
	State        PushDeliveryState  `json:"state"`
	PlatformID   string             `json:"platform_id,omitempty"`
	ErrorCode    string             `json:"error_code,omitempty"`
	ErrorMessage string             `json:"error_message,omitempty"`

	// ShouldRemoveToken is true when the push gateway reports the token as
	// invalid or unregistered. The caller should deregister the device.
	ShouldRemoveToken bool `json:"should_remove_token,omitempty"`
}

// PushDeliveryState is the terminal state after a single send attempt.
type PushDeliveryState string

const (
	PushStateDelivered   PushDeliveryState = "delivered"
	PushStateFailed      PushDeliveryState = "failed"
	PushStateThrottled   PushDeliveryState = "throttled"
	PushStateUnreachable PushDeliveryState = "unreachable"
)

// PushChannelRegistry provides type-based routing to registered PushDeliveryChannel
// implementations.
type PushChannelRegistry struct {
	channels map[ChannelType]PushDeliveryChannel
}

// NewPushChannelRegistry creates an empty registry.
func NewPushChannelRegistry() *PushChannelRegistry {
	return &PushChannelRegistry{channels: make(map[ChannelType]PushDeliveryChannel)}
}

// Register adds a delivery channel adapter to the registry.
func (r *PushChannelRegistry) Register(ch PushDeliveryChannel) {
	r.channels[ch.Type()] = ch
}

// Get returns the registered channel for the given type, or nil.
func (r *PushChannelRegistry) Get(ct ChannelType) PushDeliveryChannel {
	return r.channels[ct]
}

// All returns every registered channel.
func (r *PushChannelRegistry) All() map[ChannelType]PushDeliveryChannel {
	out := make(map[ChannelType]PushDeliveryChannel, len(r.channels))
	for k, v := range r.channels {
		out[k] = v
	}
	return out
}
