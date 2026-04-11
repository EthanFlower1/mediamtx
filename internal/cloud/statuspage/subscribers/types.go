package subscribers

import "time"

// ChannelType enumerates the delivery channels subscribers can choose.
type ChannelType string

const (
	ChannelEmail   ChannelType = "email"
	ChannelSMS     ChannelType = "sms"
	ChannelWebhook ChannelType = "webhook"
	ChannelRSS     ChannelType = "rss"
	ChannelSlack   ChannelType = "slack"
	ChannelTeams   ChannelType = "teams"
)

// Subscriber represents a single subscription to status updates.
type Subscriber struct {
	SubscriberID    string      `json:"subscriber_id"`
	TenantID        string      `json:"tenant_id"`
	ChannelType     ChannelType `json:"channel_type"`
	ChannelConfig   string      `json:"channel_config"`   // JSON: email address, phone, webhook URL, etc.
	ComponentFilter string      `json:"component_filter"` // JSON array of component names to watch; empty = all
	Confirmed       bool        `json:"confirmed"`
	CreatedAt       time.Time   `json:"created_at"`
	UpdatedAt       time.Time   `json:"updated_at"`
}

// EventType categorises the kind of status event.
type EventType string

const (
	EventStatusChange     EventType = "status_change"
	EventIncidentCreated  EventType = "incident_created"
	EventIncidentUpdated  EventType = "incident_updated"
	EventIncidentResolved EventType = "incident_resolved"
)

// StatusEvent is a denormalised record of a status change or incident
// update, used for fan-out dispatch and RSS generation.
type StatusEvent struct {
	EventID             string    `json:"event_id"`
	TenantID            string    `json:"tenant_id"`
	EventType           EventType `json:"event_type"`
	Title               string    `json:"title"`
	Description         string    `json:"description"`
	AffectedComponents  string    `json:"affected_components"` // JSON array
	Severity            string    `json:"severity"`
	CreatedAt           time.Time `json:"created_at"`
}

// Dispatcher is the seam for actually delivering a notification to a
// subscriber. Production adapters call Twilio, send emails, post to
// Slack/Teams webhooks, etc. Tests use a recording fake.
type Dispatcher interface {
	Dispatch(sub Subscriber, evt StatusEvent) error
}
