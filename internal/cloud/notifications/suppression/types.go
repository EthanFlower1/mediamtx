package suppression

import "time"

// Event represents an incoming alert event to be evaluated for suppression.
type Event struct {
	EventID   string    `json:"event_id"`
	TenantID  string    `json:"tenant_id"`
	CameraID  string    `json:"camera_id"`
	EventType string    `json:"event_type"`
	Timestamp time.Time `json:"timestamp"`
	Payload   string    `json:"payload"` // JSON blob with event-specific data
}

// SuppressionReason indicates why an alert was suppressed.
type SuppressionReason string

const (
	ReasonClustered    SuppressionReason = "clustered"
	ReasonHighActivity SuppressionReason = "high_activity"
	ReasonFalsePos     SuppressionReason = "false_positive"
)

// Decision is the result of evaluating an event through the suppression engine.
type Decision struct {
	Suppress       bool              `json:"suppress"`
	Reason         SuppressionReason `json:"reason,omitempty"`
	ClusterID      string            `json:"cluster_id,omitempty"`
	ClusterSize    int               `json:"cluster_size,omitempty"`
	ClusterSummary string            `json:"cluster_summary,omitempty"`
}

// SuppressedAlert is a persisted record of a suppressed notification.
type SuppressedAlert struct {
	AlertID        string            `json:"alert_id"`
	TenantID       string            `json:"tenant_id"`
	CameraID       string            `json:"camera_id"`
	EventType      string            `json:"event_type"`
	Reason         SuppressionReason `json:"reason"`
	ClusterID      string            `json:"cluster_id"`
	ClusterSize    int               `json:"cluster_size"`
	ClusterSummary string            `json:"cluster_summary"`
	OriginalEvent  string            `json:"original_event"`
	SuppressedAt   time.Time         `json:"suppressed_at"`
}

// Settings holds the per-camera suppression sensitivity.
type Settings struct {
	TenantID    string    `json:"tenant_id"`
	CameraID    string    `json:"camera_id"`
	Sensitivity float64   `json:"sensitivity"` // 0.0 = off, 1.0 = aggressive
	CreatedAt   time.Time `json:"created_at"`
	UpdatedAt   time.Time `json:"updated_at"`
}

// Baseline stores per-camera per-time-slot expected activity levels.
type Baseline struct {
	TenantID    string  `json:"tenant_id"`
	CameraID    string  `json:"camera_id"`
	HourOfDay   int     `json:"hour_of_day"`
	DayOfWeek   int     `json:"day_of_week"`
	EventType   string  `json:"event_type"`
	AvgCount    float64 `json:"avg_count"`
	StddevCount float64 `json:"stddev_count"`
	SampleDays  int     `json:"sample_days"`
}
