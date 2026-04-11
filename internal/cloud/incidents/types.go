package incidents

import "time"

// Severity represents the urgency of an incident for PagerDuty routing.
type Severity string

const (
	SeverityCritical Severity = "critical"
	SeverityError    Severity = "error"
	SeverityWarning  Severity = "warning"
	SeverityInfo     Severity = "info"
)

// IncidentStatus tracks incident lifecycle.
type IncidentStatus string

const (
	StatusTriggered    IncidentStatus = "triggered"
	StatusAcknowledged IncidentStatus = "acknowledged"
	StatusResolved     IncidentStatus = "resolved"
)

// PostMortemStatus tracks the post-mortem lifecycle.
type PostMortemStatus string

const (
	PostMortemDraft     PostMortemStatus = "draft"
	PostMortemInReview  PostMortemStatus = "in_review"
	PostMortemPublished PostMortemStatus = "published"
)

// Incident represents a paging incident linked to a PagerDuty event.
type Incident struct {
	IncidentID        string         `json:"incident_id"`
	TenantID          string         `json:"tenant_id"`
	AlertName         string         `json:"alert_name"`
	Severity          Severity       `json:"severity"`
	Status            IncidentStatus `json:"status"`
	Summary           string         `json:"summary"`
	Source            string         `json:"source"`
	AffectedComponent string        `json:"affected_component"`
	PagerDutyKey      string         `json:"pagerduty_key,omitempty"`
	PagerDutyDedupKey string         `json:"pagerduty_dedup_key,omitempty"`
	RunbookURL        string         `json:"runbook_url,omitempty"`
	TriggeredAt       time.Time      `json:"triggered_at"`
	AcknowledgedAt    *time.Time     `json:"acknowledged_at,omitempty"`
	ResolvedAt        *time.Time     `json:"resolved_at,omitempty"`
	CreatedAt         time.Time      `json:"created_at"`
	UpdatedAt         time.Time      `json:"updated_at"`
}

// RunbookMapping associates an alert name with a runbook URL for a tenant.
type RunbookMapping struct {
	MappingID  string    `json:"mapping_id"`
	TenantID   string    `json:"tenant_id"`
	AlertName  string    `json:"alert_name"`
	RunbookURL string    `json:"runbook_url"`
	CreatedAt  time.Time `json:"created_at"`
	UpdatedAt  time.Time `json:"updated_at"`
}

// OnCallSchedule defines who is on-call for a tenant's service.
type OnCallSchedule struct {
	ScheduleID  string    `json:"schedule_id"`
	TenantID    string    `json:"tenant_id"`
	ServiceName string    `json:"service_name"`
	UserID      string    `json:"user_id"`
	StartTime   time.Time `json:"start_time"`
	EndTime     time.Time `json:"end_time"`
	CreatedAt   time.Time `json:"created_at"`
	UpdatedAt   time.Time `json:"updated_at"`
}

// PostMortem contains the incident review document auto-populated from incident data.
type PostMortem struct {
	PostMortemID       string           `json:"post_mortem_id"`
	IncidentID         string           `json:"incident_id"`
	TenantID           string           `json:"tenant_id"`
	Title              string           `json:"title"`
	Status             PostMortemStatus `json:"status"`
	Summary            string           `json:"summary"`
	Timeline           string           `json:"timeline"`
	AffectedComponents string           `json:"affected_components"`
	RootCause          string           `json:"root_cause"`
	ActionItems        string           `json:"action_items"`
	MetricsSnapshot    string           `json:"metrics_snapshot"`
	CreatedAt          time.Time        `json:"created_at"`
	UpdatedAt          time.Time        `json:"updated_at"`
}

// AlertmanagerPayload represents the Prometheus Alertmanager webhook payload.
type AlertmanagerPayload struct {
	Version           string            `json:"version"`
	GroupKey          string            `json:"groupKey"`
	TruncatedAlerts   int               `json:"truncatedAlerts"`
	Status            string            `json:"status"`
	Receiver          string            `json:"receiver"`
	GroupLabels       map[string]string `json:"groupLabels"`
	CommonLabels      map[string]string `json:"commonLabels"`
	CommonAnnotations map[string]string `json:"commonAnnotations"`
	ExternalURL       string            `json:"externalURL"`
	Alerts            []Alert           `json:"alerts"`
}

// Alert represents a single alert within the Alertmanager webhook payload.
type Alert struct {
	Status       string            `json:"status"`
	Labels       map[string]string `json:"labels"`
	Annotations  map[string]string `json:"annotations"`
	StartsAt     time.Time         `json:"startsAt"`
	EndsAt       time.Time         `json:"endsAt"`
	GeneratorURL string            `json:"generatorURL"`
	Fingerprint  string            `json:"fingerprint"`
}

// PagerDutyEvent represents a PagerDuty Events API v2 payload.
type PagerDutyEvent struct {
	RoutingKey  string              `json:"routing_key"`
	EventAction string             `json:"event_action"`
	DedupKey    string              `json:"dedup_key,omitempty"`
	Payload     PagerDutyPayload    `json:"payload"`
	Links       []PagerDutyLink     `json:"links,omitempty"`
}

// PagerDutyPayload is the payload section of a PagerDuty event.
type PagerDutyPayload struct {
	Summary       string   `json:"summary"`
	Source        string   `json:"source"`
	Severity      string   `json:"severity"`
	Component     string   `json:"component,omitempty"`
	Group         string   `json:"group,omitempty"`
	Class         string   `json:"class,omitempty"`
	CustomDetails any      `json:"custom_details,omitempty"`
}

// PagerDutyLink is a link attached to a PagerDuty event.
type PagerDutyLink struct {
	Href string `json:"href"`
	Text string `json:"text"`
}

// PagerDutyResponse is the response from the PagerDuty Events API v2.
type PagerDutyResponse struct {
	Status   string `json:"status"`
	Message  string `json:"message"`
	DedupKey string `json:"dedup_key"`
}
