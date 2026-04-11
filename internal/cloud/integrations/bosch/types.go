package bosch

import (
	"time"
)

// PanelSeries identifies which Bosch panel family is connected.
type PanelSeries string

const (
	PanelSeriesB PanelSeries = "B"
	PanelSeriesG PanelSeries = "G"
)

// PanelConfig holds the connection and authentication parameters for a
// single Bosch B/G-Series alarm panel within a tenant.
type PanelConfig struct {
	ID          string      `json:"id"`
	TenantID    string      `json:"tenant_id"`
	DisplayName string      `json:"display_name"`
	Host        string      `json:"host"`        // IP or hostname of the panel
	Port        int         `json:"port"`         // TCP port (default 7700)
	AuthCode    string      `json:"auth_code"`    // panel automation passcode
	Series      PanelSeries `json:"series"`       // "B" or "G"
	Enabled     bool        `json:"enabled"`      // whether the integration is active
	CreatedAt   time.Time   `json:"created_at"`
	UpdatedAt   time.Time   `json:"updated_at"`
}

// DefaultPort is the standard TCP port for Bosch Mode2 automation protocol.
const DefaultPort = 7700

// ZoneCameraMapping maps a panel zone to one or more cameras and the
// action(s) to take when an alarm fires on that zone.
type ZoneCameraMapping struct {
	ID          string   `json:"id"`
	TenantID    string   `json:"tenant_id"`
	PanelID     string   `json:"panel_id"`
	ZoneNumber  int      `json:"zone_number"`
	ZoneName    string   `json:"zone_name,omitempty"`
	CameraIDs   []string `json:"camera_ids"`
	Actions     []Action `json:"actions"`
	Enabled     bool     `json:"enabled"`
}

// Action defines what happens on a camera when an alarm event fires.
type Action struct {
	Type       ActionType `json:"type"`
	PTZPreset  string     `json:"ptz_preset,omitempty"`   // for ActionPTZPreset
	WebhookURL string     `json:"webhook_url,omitempty"`  // for ActionWebhook
	Duration   int        `json:"duration,omitempty"`     // seconds for ActionRecord
}

// ActionType enumerates the camera actions triggered by alarm events.
type ActionType string

const (
	ActionRecord    ActionType = "record"
	ActionPTZPreset ActionType = "ptz_preset"
	ActionWebhook   ActionType = "webhook"
	ActionSnapshot  ActionType = "snapshot"
)

// AlarmEvent is the normalized representation of an event received from a
// Bosch panel. It is produced by the EventIngester and consumed by the
// ActionRouter.
type AlarmEvent struct {
	PanelID    string    `json:"panel_id"`
	TenantID   string    `json:"tenant_id"`
	EventType  EventType `json:"event_type"`
	ZoneNumber int       `json:"zone_number"`
	AreaNumber int       `json:"area_number"`
	UserNumber int       `json:"user_number,omitempty"`
	Priority   int       `json:"priority"`
	RawCode    uint16    `json:"raw_code"`
	Message    string    `json:"message"`
	Timestamp  time.Time `json:"timestamp"`
}

// EventType classifies alarm panel events.
type EventType string

const (
	EventBurglary    EventType = "burglary"
	EventFire        EventType = "fire"
	EventPanic       EventType = "panic"
	EventTrouble     EventType = "trouble"
	EventArmDisarm   EventType = "arm_disarm"
	EventZoneFault   EventType = "zone_fault"
	EventZoneRestore EventType = "zone_restore"
	EventSupervisory EventType = "supervisory"
	EventUnknown     EventType = "unknown"
)

// ConnectionState represents the panel TCP connection lifecycle.
type ConnectionState string

const (
	StateDisconnected ConnectionState = "disconnected"
	StateConnecting   ConnectionState = "connecting"
	StateAuthenticating ConnectionState = "authenticating"
	StateConnected    ConnectionState = "connected"
	StateReconnecting ConnectionState = "reconnecting"
)
