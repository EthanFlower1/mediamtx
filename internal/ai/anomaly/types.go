package anomaly

import (
	"encoding/json"
	"time"
)

// Beta indicates this feature is in beta. API/UI consumers should check this.
const Beta = true

// DefaultLearningDays is the default baseline learning period.
const DefaultLearningDays = 7

// DefaultSensitivity is the default anomaly sensitivity (0.0 = least sensitive,
// 1.0 = most sensitive).
const DefaultSensitivity = 0.5

// HoursPerDay is the number of hourly buckets in a baseline.
const HoursPerDay = 24

// AnomalyEvent is emitted when the anomaly score exceeds the threshold derived
// from the configured sensitivity.
type AnomalyEvent struct {
	// TenantID and CameraID identify the source.
	TenantID string `json:"tenant_id"`
	CameraID string `json:"camera_id"`

	// At is the wall-clock time the anomaly was detected.
	At time.Time `json:"at"`

	// Score is the anomaly score in [0, 1] where 1 is maximum deviation.
	Score float64 `json:"score"`

	// Threshold is the score threshold that was exceeded.
	Threshold float64 `json:"threshold"`

	// HourOfDay is the hour bucket (0-23) in which the anomaly was detected.
	HourOfDay int `json:"hour_of_day"`

	// ObservedCount is the object count observed in this frame.
	ObservedCount int `json:"observed_count"`

	// BaselineMean is the expected mean object count for this hour.
	BaselineMean float64 `json:"baseline_mean"`

	// BaselineStdDev is the standard deviation of object counts for this hour.
	BaselineStdDev float64 `json:"baseline_stddev"`

	// Details contains per-class breakdown if available.
	Details map[string]ClassAnomaly `json:"details,omitempty"`

	// Beta indicates this feature is in beta.
	Beta bool `json:"beta"`
}

// ClassAnomaly holds anomaly detail for a single object class.
type ClassAnomaly struct {
	Observed int     `json:"observed"`
	Mean     float64 `json:"mean"`
	StdDev   float64 `json:"stddev"`
	ZScore   float64 `json:"z_score"`
}

// Config holds per-camera anomaly detection configuration.
type Config struct {
	// ID is the opaque configuration identifier.
	ID string `json:"id"`

	// TenantID and CameraID scope this config.
	TenantID string `json:"tenant_id"`
	CameraID string `json:"camera_id"`

	// Enabled gates the detector.
	Enabled bool `json:"enabled"`

	// Sensitivity is the anomaly sensitivity in [0.0, 1.0].
	// Higher values make the detector more sensitive (lower threshold).
	// 0.0 = only extreme anomalies, 1.0 = flag minor deviations.
	Sensitivity float64 `json:"sensitivity"`

	// LearningDays is the number of days for the baseline learning phase.
	// During learning, no anomalies are emitted. Default: 7.
	LearningDays int `json:"learning_days"`

	// Beta indicates this feature is in beta.
	Beta bool `json:"beta"`
}

// Validate returns an error if the config is invalid.
func (c Config) Validate() error {
	if c.Sensitivity < 0 || c.Sensitivity > 1 {
		return &ValidationError{Field: "sensitivity", Msg: "must be in [0.0, 1.0]"}
	}
	if c.LearningDays < 0 {
		return &ValidationError{Field: "learning_days", Msg: "must be non-negative"}
	}
	return nil
}

// ValidationError is returned when configuration is invalid.
type ValidationError struct {
	Field string `json:"field"`
	Msg   string `json:"msg"`
}

func (e *ValidationError) Error() string {
	return "anomaly config: " + e.Field + ": " + e.Msg
}

// StatusResponse is the API response for anomaly detection status.
type StatusResponse struct {
	CameraID     string  `json:"camera_id"`
	Enabled      bool    `json:"enabled"`
	Sensitivity  float64 `json:"sensitivity"`
	Learning     bool    `json:"learning"`
	LearningDays int     `json:"learning_days"`
	DaysLearned  int     `json:"days_learned"`
	LastAnomaly  *AnomalyEvent `json:"last_anomaly,omitempty"`
	Beta         bool    `json:"beta"`
}

// BaselineSnapshot is a serialisable snapshot of the learned baseline.
type BaselineSnapshot struct {
	CameraID    string              `json:"camera_id"`
	StartedAt   time.Time           `json:"started_at"`
	SampleCount int                 `json:"sample_count"`
	Hours       [HoursPerDay]HourStats `json:"hours"`
	ClassHours  map[string][HoursPerDay]HourStats `json:"class_hours,omitempty"`
	Beta        bool                `json:"beta"`
}

// HourStats holds the running statistics for a single hour bucket.
type HourStats struct {
	Mean   float64 `json:"mean"`
	StdDev float64 `json:"stddev"`
	Count  int     `json:"count"`
}

// MarshalJSON implements json.Marshaler for BaselineSnapshot.
func (b BaselineSnapshot) MarshalJSON() ([]byte, error) {
	type Alias BaselineSnapshot
	return json.Marshal(&struct {
		Alias
	}{Alias: Alias(b)})
}
