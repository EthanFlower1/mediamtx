package comms

import "time"

// Platform identifies a messaging platform.
type Platform string

const (
	PlatformSlack Platform = "slack"
	PlatformTeams Platform = "teams"
)

// Alert represents a normalised NVR alert that can be posted to any
// messaging platform.
type Alert struct {
	AlertID   string    `json:"alert_id"`
	TenantID  string    `json:"tenant_id"`
	EventType string    `json:"event_type"` // e.g. "camera.offline", "motion.detected"
	CameraID  string    `json:"camera_id"`
	Title     string    `json:"title"`
	Body      string    `json:"body"`
	ClipURL   string    `json:"clip_url,omitempty"` // deep-link to clip viewer
	Timestamp time.Time `json:"timestamp"`
}

// RoutingRule maps an event type to a destination channel on a specific
// platform.
type RoutingRule struct {
	RuleID       string   `json:"rule_id"`
	TenantID     string   `json:"tenant_id"`
	EventTypes   []string `json:"event_types"`   // match these event types ("*" = all)
	Platform     Platform `json:"platform"`       // "slack" or "teams"
	ChannelRef   string   `json:"channel_ref"`    // platform-specific channel ID
	IntegrationID string  `json:"integration_id"` // references Slack/Teams config
	Enabled      bool     `json:"enabled"`
	CreatedAt    time.Time `json:"created_at"`
	UpdatedAt    time.Time `json:"updated_at"`
}

// ActionType enumerates the interactive actions users can trigger from
// message cards.
type ActionType string

const (
	ActionAcknowledge ActionType = "ack"
	ActionTriage      ActionType = "triage"
	ActionWatchClip   ActionType = "watch_clip"
)

// CardAction represents an interactive action callback from a messaging
// platform.
type CardAction struct {
	ActionType    ActionType `json:"action_type"`
	AlertID       string     `json:"alert_id"`
	UserID        string     `json:"user_id"`        // platform user ID
	UserName      string     `json:"user_name"`       // display name
	Platform      Platform   `json:"platform"`
	ChannelRef    string     `json:"channel_ref"`
	IntegrationID string     `json:"integration_id"`
	Timestamp     time.Time  `json:"timestamp"`
}

// ActionResult is returned after handling a CardAction.
type ActionResult struct {
	OK      bool   `json:"ok"`
	Message string `json:"message,omitempty"`
}

// PostResult records the outcome of posting an alert to a channel.
type PostResult struct {
	Platform   Platform `json:"platform"`
	ChannelRef string   `json:"channel_ref"`
	MessageID  string   `json:"message_id,omitempty"` // platform message ID
	Err        error    `json:"-"`
}
