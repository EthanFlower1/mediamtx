package summaries

import (
	"errors"
	"time"
)

// EventCategory classifies events for aggregation.
type EventCategory string

const (
	CategoryMotion     EventCategory = "motion"
	CategoryPerson     EventCategory = "person_detected"
	CategoryVehicle    EventCategory = "vehicle_detected"
	CategoryOffline    EventCategory = "camera_offline"
	CategoryOnline     EventCategory = "camera_online"
	CategoryTamper     EventCategory = "tamper"
	CategoryLoitering  EventCategory = "loitering"
	CategoryLineCross  EventCategory = "line_crossing"
	CategoryCrowd      EventCategory = "crowd_density"
	CategoryTailgating EventCategory = "tailgating"
	CategoryFall       EventCategory = "fall_detection"
	CategoryOther      EventCategory = "other"
)

// Event is a single event from the NVR event stream.
type Event struct {
	EventID   string        `json:"event_id"`
	TenantID  string        `json:"tenant_id"`
	CameraID  string        `json:"camera_id"`
	Category  EventCategory `json:"category"`
	Detail    string        `json:"detail"`
	Timestamp time.Time     `json:"timestamp"`
}

// AggregatedEvents groups events by category for a single tenant within
// a time window. This is the input to the prompt builder.
type AggregatedEvents struct {
	TenantID  string
	StartTime time.Time
	EndTime   time.Time
	// ByCameraCategory maps camera_id -> category -> count.
	ByCameraCategory map[string]map[EventCategory]int
	// TotalByCategory maps category -> total count across all cameras.
	TotalByCategory map[EventCategory]int
	// TotalEvents is the grand total.
	TotalEvents int
	// NotableEvents are events that merit individual mention (e.g. tamper,
	// fall_detection, camera_offline).
	NotableEvents []Event
}

// SummaryPeriod is the schedule cadence for summaries.
type SummaryPeriod string

const (
	PeriodDaily  SummaryPeriod = "daily"
	PeriodWeekly SummaryPeriod = "weekly"
)

// Summary is a generated natural language summary for a single tenant.
type Summary struct {
	SummaryID string        `json:"summary_id"`
	TenantID  string        `json:"tenant_id"`
	Period    SummaryPeriod `json:"period"`
	StartTime time.Time     `json:"start_time"`
	EndTime   time.Time     `json:"end_time"`
	// Text is the generated natural language summary.
	Text string `json:"text"`
	// EventCount is the total number of events summarised.
	EventCount int       `json:"event_count"`
	GeneratedAt time.Time `json:"generated_at"`
	// DeliveredAt records when the summary was sent via notification channels.
	DeliveredAt *time.Time `json:"delivered_at,omitempty"`
}

// TritonConfig holds connection parameters for the Triton Inference Server.
type TritonConfig struct {
	// Endpoint is the gRPC or HTTP URL of the Triton server.
	Endpoint string `json:"endpoint"`
	// ModelName is the name of the LLM model deployed on Triton.
	ModelName string `json:"model_name"`
	// MaxTokens caps the generated summary length.
	MaxTokens int `json:"max_tokens"`
	// Temperature controls sampling randomness (0.0 = greedy).
	Temperature float64 `json:"temperature"`
	// TimeoutSeconds is the per-request timeout for Triton inference.
	TimeoutSeconds int `json:"timeout_seconds"`
}

// DefaultTritonConfig returns sensible defaults for Triton LLM inference.
func DefaultTritonConfig() TritonConfig {
	return TritonConfig{
		Endpoint:       "localhost:8001",
		ModelName:      "llama3-8b",
		MaxTokens:      512,
		Temperature:    0.3,
		TimeoutSeconds: 30,
	}
}

// Sentinel errors.
var (
	ErrInvalidTenantID = errors.New("summaries: tenant_id is required")
	ErrNoEvents        = errors.New("summaries: no events in period")
	ErrTritonUnavail   = errors.New("summaries: triton inference server unavailable")
	ErrInferenceFailed = errors.New("summaries: LLM inference failed")
	ErrNotFound        = errors.New("summaries: summary not found")
)
