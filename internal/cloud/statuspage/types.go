package statuspage

import "time"

// ServiceStatus represents the health status of a service.
type ServiceStatus string

const (
	StatusOperational  ServiceStatus = "operational"
	StatusDegraded     ServiceStatus = "degraded"
	StatusPartialOut   ServiceStatus = "partial_outage"
	StatusMajorOut     ServiceStatus = "major_outage"
)

// IncidentSeverity represents how severe an incident is.
type IncidentSeverity string

const (
	SeverityMinor    IncidentSeverity = "minor"
	SeverityMajor    IncidentSeverity = "major"
	SeverityCritical IncidentSeverity = "critical"
)

// IncidentStatus represents the lifecycle state of an incident.
type IncidentStatus string

const (
	IncidentInvestigating IncidentStatus = "investigating"
	IncidentIdentified    IncidentStatus = "identified"
	IncidentMonitoring    IncidentStatus = "monitoring"
	IncidentResolved      IncidentStatus = "resolved"
)

// HealthCheck represents a monitored service for a tenant.
type HealthCheck struct {
	CheckID       string        `json:"check_id"`
	TenantID      string        `json:"tenant_id"`
	ServiceName   string        `json:"service_name"`
	DisplayName   string        `json:"display_name"`
	Status        ServiceStatus `json:"status"`
	LastCheckedAt *time.Time    `json:"last_checked_at,omitempty"`
	Metadata      string        `json:"metadata"`
	Enabled       bool          `json:"enabled"`
	CreatedAt     time.Time     `json:"created_at"`
	UpdatedAt     time.Time     `json:"updated_at"`
}

// Incident represents a service incident for a tenant.
type Incident struct {
	IncidentID       string           `json:"incident_id"`
	TenantID         string           `json:"tenant_id"`
	Title            string           `json:"title"`
	Severity         IncidentSeverity `json:"severity"`
	Status           IncidentStatus   `json:"status"`
	AffectedServices string           `json:"affected_services"`
	StartedAt        time.Time        `json:"started_at"`
	ResolvedAt       *time.Time       `json:"resolved_at,omitempty"`
	CreatedAt        time.Time        `json:"created_at"`
	UpdatedAt        time.Time        `json:"updated_at"`
}

// IncidentUpdate represents a timestamped update on an incident.
type IncidentUpdate struct {
	UpdateID   string         `json:"update_id"`
	IncidentID string         `json:"incident_id"`
	TenantID   string         `json:"tenant_id"`
	Status     IncidentStatus `json:"status"`
	Message    string         `json:"message"`
	CreatedAt  time.Time      `json:"created_at"`
}

// StatusSummary is the aggregated status page view for a tenant.
type StatusSummary struct {
	OverallStatus ServiceStatus `json:"overall_status"`
	Services      []HealthCheck `json:"services"`
	ActiveIncidents []Incident  `json:"active_incidents"`
}
