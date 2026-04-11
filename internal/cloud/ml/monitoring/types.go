package monitoring

import (
	"errors"
	"time"
)

// -----------------------------------------------------------------------
// Configuration
// -----------------------------------------------------------------------

// Config holds configuration for the model monitoring pipeline.
type Config struct {
	// DriftCheckInterval is how often the detector runs distribution checks.
	DriftCheckInterval time.Duration

	// KLDivergenceThreshold triggers an alert when KL divergence exceeds this
	// value. Typical production threshold: 0.1 - 0.5.
	KLDivergenceThreshold float64

	// PSIThreshold triggers an alert when PSI exceeds this value.
	// < 0.1 = no shift, 0.1-0.25 = moderate, > 0.25 = significant.
	PSIThreshold float64

	// AccuracyFloorPct is the minimum acceptable accuracy percentage.
	// An alert fires when accuracy drops below this value.
	AccuracyFloorPct float64

	// FPRateCeilingPct is the maximum acceptable false positive rate percentage.
	FPRateCeilingPct float64

	// LatencyP99Ceiling is the maximum acceptable p99 inference latency.
	LatencyP99Ceiling time.Duration

	// HistogramBins is the number of bins used for distribution tracking.
	HistogramBins int

	// AlertWebhookURL is the endpoint for alert notifications.
	AlertWebhookURL string

	// OnCallRotationID identifies the on-call rotation for alert routing.
	OnCallRotationID string

	// RetentionDays is how long audit evidence is retained.
	RetentionDays int
}

// DefaultConfig returns a Config with sensible production defaults.
func DefaultConfig() Config {
	return Config{
		DriftCheckInterval:    5 * time.Minute,
		KLDivergenceThreshold: 0.2,
		PSIThreshold:          0.25,
		AccuracyFloorPct:      90.0,
		FPRateCeilingPct:      5.0,
		LatencyP99Ceiling:     100 * time.Millisecond,
		HistogramBins:         20,
		RetentionDays:         365,
	}
}

// Validate checks that the Config has valid values.
func (c Config) Validate() error {
	if c.DriftCheckInterval <= 0 {
		return errors.New("monitoring: DriftCheckInterval must be positive")
	}
	if c.KLDivergenceThreshold <= 0 {
		return errors.New("monitoring: KLDivergenceThreshold must be positive")
	}
	if c.PSIThreshold <= 0 {
		return errors.New("monitoring: PSIThreshold must be positive")
	}
	if c.HistogramBins < 2 {
		return errors.New("monitoring: HistogramBins must be >= 2")
	}
	if c.AccuracyFloorPct < 0 || c.AccuracyFloorPct > 100 {
		return errors.New("monitoring: AccuracyFloorPct must be 0-100")
	}
	if c.FPRateCeilingPct < 0 || c.FPRateCeilingPct > 100 {
		return errors.New("monitoring: FPRateCeilingPct must be 0-100")
	}
	return nil
}

// -----------------------------------------------------------------------
// Domain types
// -----------------------------------------------------------------------

// ModelKey uniquely identifies a model within a tenant.
type ModelKey struct {
	TenantID string
	ModelID  string
	Version  string
}

// String returns a label-safe representation of the model key.
func (k ModelKey) String() string {
	return k.TenantID + "/" + k.ModelID + ":" + k.Version
}

// DriftResult holds the outcome of a single drift check.
type DriftResult struct {
	Key           ModelKey
	Timestamp     time.Time
	KLDivergence  float64
	PSI           float64
	FeatureDrifts map[string]FeatureDrift
	Drifted       bool
	Reason        string
}

// FeatureDrift holds per-feature drift metrics.
type FeatureDrift struct {
	FeatureName  string
	KLDivergence float64
	PSI          float64
	Drifted      bool
}

// AlertSeverity classifies alert urgency.
type AlertSeverity string

const (
	SeverityWarning  AlertSeverity = "warning"
	SeverityCritical AlertSeverity = "critical"
)

// Alert represents a monitoring alert to be dispatched.
type Alert struct {
	ID               string
	Severity         AlertSeverity
	Key              ModelKey
	Type             string // "drift", "accuracy", "fp_rate", "latency"
	Message          string
	Value            float64
	Threshold        float64
	Timestamp        time.Time
	OnCallRotationID string
}

// AuditRecord is a SOC 2-compatible evidence record for model monitoring
// activity.
type AuditRecord struct {
	ID            string    `json:"id"`
	Timestamp     time.Time `json:"timestamp"`
	TenantID      string    `json:"tenant_id"`
	ModelID       string    `json:"model_id"`
	ModelVersion  string    `json:"model_version"`
	EventType     string    `json:"event_type"` // "drift_check", "alert_fired", "baseline_set"
	Details       string    `json:"details"`
	KLDivergence  *float64  `json:"kl_divergence,omitempty"`
	PSI           *float64  `json:"psi,omitempty"`
	Accuracy      *float64  `json:"accuracy,omitempty"`
	FPRate        *float64  `json:"fp_rate,omitempty"`
	LatencyP99Ms  *float64  `json:"latency_p99_ms,omitempty"`
	AlertFired    bool      `json:"alert_fired"`
	ExportedAt    time.Time `json:"exported_at"`
}

// -----------------------------------------------------------------------
// Sentinel errors
// -----------------------------------------------------------------------

var (
	// ErrNoBaseline is returned when drift detection is attempted before a
	// reference distribution has been set.
	ErrNoBaseline = errors.New("monitoring: no baseline distribution set")

	// ErrInvalidTenantID is returned when tenant_id is empty.
	ErrInvalidTenantID = errors.New("monitoring: tenant_id is required")

	// ErrInvalidModelID is returned when model_id is empty.
	ErrInvalidModelID = errors.New("monitoring: model_id is required")
)
