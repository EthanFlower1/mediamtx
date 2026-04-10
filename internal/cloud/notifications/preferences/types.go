package preferences

import (
	"time"

	"github.com/bluenviron/mediamtx/internal/cloud/notifications"
)

// Severity represents the minimum severity level for notification delivery.
type Severity string

const (
	SeverityInfo     Severity = "info"
	SeverityWarning  Severity = "warning"
	SeverityCritical Severity = "critical"
)

// SeverityRank maps severity strings to ordinal values for comparison.
func SeverityRank(s Severity) int {
	switch s {
	case SeverityInfo:
		return 0
	case SeverityWarning:
		return 1
	case SeverityCritical:
		return 2
	default:
		return 0
	}
}

// Pref is a single per-user notification preference entry.
type Pref struct {
	PrefID       string                      `json:"pref_id"`
	TenantID     string                      `json:"tenant_id"`
	UserID       string                      `json:"user_id"`
	CameraID     string                      `json:"camera_id"`     // "" = all cameras
	EventType    string                      `json:"event_type"`    // "" = all event types
	Channels     []notifications.ChannelType `json:"channels"`      // which channels to deliver on
	SeverityMin  Severity                    `json:"severity_min"`  // minimum severity threshold
	QuietStart   string                      `json:"quiet_start"`   // "HH:MM" or "" for no quiet hours
	QuietEnd     string                      `json:"quiet_end"`     // "HH:MM" or ""
	QuietTimezone string                     `json:"quiet_timezone"` // IANA timezone
	QuietDays    []int                       `json:"quiet_days"`    // 0=Sun..6=Sat; empty = every day
	Enabled      bool                        `json:"enabled"`
	CreatedAt    time.Time                   `json:"created_at"`
	UpdatedAt    time.Time                   `json:"updated_at"`
}

// ResolvedDelivery is the result of resolving preferences for a specific
// notification event. It says which channels to use and whether quiet hours
// suppress delivery.
type ResolvedDelivery struct {
	Pref       Pref
	Suppressed bool   // true if quiet hours are active right now
	Reason     string // human-readable reason if suppressed
}
