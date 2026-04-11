package provider

import "time"

// ComponentStatus mirrors the Statuspage.io component status enum.
type ComponentStatus string

const (
	ComponentOperational         ComponentStatus = "operational"
	ComponentDegradedPerformance ComponentStatus = "degraded_performance"
	ComponentPartialOutage       ComponentStatus = "partial_outage"
	ComponentMajorOutage         ComponentStatus = "major_outage"
	ComponentUnderMaintenance    ComponentStatus = "under_maintenance"
)

// Component represents a Statuspage.io component.
type Component struct {
	ID                 string          `json:"id,omitempty"`
	PageID             string          `json:"page_id,omitempty"`
	Name               string          `json:"name"`
	Description        string          `json:"description,omitempty"`
	Status             ComponentStatus `json:"status"`
	Position           int             `json:"position,omitempty"`
	Showcase           bool            `json:"showcase"`
	OnlyShowIfDegraded bool            `json:"only_show_if_degraded"`
	GroupID            string          `json:"group_id,omitempty"`
	CreatedAt          time.Time       `json:"created_at,omitempty"`
	UpdatedAt          time.Time       `json:"updated_at,omitempty"`
}

// ComponentGroup represents a Statuspage.io component group.
type ComponentGroup struct {
	ID          string   `json:"id,omitempty"`
	PageID      string   `json:"page_id,omitempty"`
	Name        string   `json:"name"`
	Description string   `json:"description,omitempty"`
	Components  []string `json:"components,omitempty"`
	Position    int      `json:"position,omitempty"`
}

// IncidentImpact mirrors the Statuspage.io incident impact level.
type IncidentImpact string

const (
	ImpactNone        IncidentImpact = "none"
	ImpactMinor       IncidentImpact = "minor"
	ImpactMajor       IncidentImpact = "major"
	ImpactCritical    IncidentImpact = "critical"
	ImpactMaintenance IncidentImpact = "maintenance"
)

// IncidentStatus mirrors the Statuspage.io incident status enum.
type IncidentStatus string

const (
	IncidentInvestigating IncidentStatus = "investigating"
	IncidentIdentified    IncidentStatus = "identified"
	IncidentMonitoring    IncidentStatus = "monitoring"
	IncidentResolved      IncidentStatus = "resolved"
	IncidentPostmortem    IncidentStatus = "postmortem"
)

// Incident represents a Statuspage.io incident.
type Incident struct {
	ID               string          `json:"id,omitempty"`
	PageID           string          `json:"page_id,omitempty"`
	Name             string          `json:"name"`
	Status           IncidentStatus  `json:"status"`
	Impact           IncidentImpact  `json:"impact,omitempty"`
	Body             string          `json:"body,omitempty"`
	ComponentIDs     []string        `json:"component_ids,omitempty"`
	Components       map[string]ComponentStatus `json:"components,omitempty"`
	CreatedAt        time.Time       `json:"created_at,omitempty"`
	UpdatedAt        time.Time       `json:"updated_at,omitempty"`
	ResolvedAt       *time.Time      `json:"resolved_at,omitempty"`
	IncidentUpdates  []IncidentUpdate `json:"incident_updates,omitempty"`
}

// IncidentUpdate represents a single update within an incident timeline.
type IncidentUpdate struct {
	ID         string         `json:"id,omitempty"`
	IncidentID string         `json:"incident_id,omitempty"`
	Status     IncidentStatus `json:"status"`
	Body       string         `json:"body"`
	CreatedAt  time.Time      `json:"created_at,omitempty"`
}

// CreateIncidentRequest is the payload for creating an incident.
type CreateIncidentRequest struct {
	Name             string                     `json:"name"`
	Status           IncidentStatus             `json:"status,omitempty"`
	ImpactOverride   IncidentImpact             `json:"impact_override,omitempty"`
	Body             string                     `json:"body,omitempty"`
	ComponentIDs     []string                   `json:"component_ids,omitempty"`
	Components       map[string]ComponentStatus `json:"components,omitempty"`
	DeliverNotifications bool                   `json:"deliver_notifications"`
}

// UpdateIncidentRequest is the payload for updating an incident.
type UpdateIncidentRequest struct {
	Status     IncidentStatus             `json:"status,omitempty"`
	Body       string                     `json:"body,omitempty"`
	Components map[string]ComponentStatus `json:"components,omitempty"`
}
