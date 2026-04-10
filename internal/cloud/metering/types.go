package metering

import "time"

// Dialect selects the SQL flavour emitted by Store. Mirrors the pattern in
// internal/cloud/audit so tests can exercise the SQL path under SQLite.
type Dialect int

const (
	// DialectPostgres emits Postgres-flavoured SQL with `$1` placeholders
	// and targets the partitioned usage_events parent declared by
	// migration 0016.
	DialectPostgres Dialect = iota
	// DialectSQLite emits SQLite-compatible SQL with `?` placeholders and
	// targets the flat usage_events fallback created by migration 0016
	// under SQLite.
	DialectSQLite
)

// Metric is the closed set of billable metric names. Adding a metric is
// deliberately a two-step change: add here and add to mode_drift_test.go so
// billing stays in sync.
type Metric string

const (
	// MetricCameraHours sums the number of camera-hours the tenant had
	// online during the period. One camera online for one hour == 1.0.
	MetricCameraHours Metric = "camera_hours"

	// MetricRecordingBytes sums the bytes written to archive during the
	// period. Used by the recording-storage add-on in KAI-363.
	MetricRecordingBytes Metric = "recording_bytes"

	// MetricAIInferenceCount sums the number of AI inference calls
	// billed per-unit during the period. Drives the advanced AI add-on.
	MetricAIInferenceCount Metric = "ai_inference_count"
)

// AllMetrics is the canonical closed list. Record rejects unknown metrics.
var AllMetrics = []Metric{
	MetricCameraHours,
	MetricRecordingBytes,
	MetricAIInferenceCount,
}

// Valid reports whether m is a known metric.
func (m Metric) Valid() bool {
	for _, known := range AllMetrics {
		if m == known {
			return true
		}
	}
	return false
}

// Event is a single tenant-scoped usage observation. The Recorder writes one
// Event per Record call. Event values are always non-negative.
type Event struct {
	TenantID string
	Metric   Metric
	Value    float64
	// Timestamp defaults to time.Now().UTC() at Record time when zero.
	Timestamp time.Time
}

// Aggregate is a rollup row for a single (tenant, period, metric) triple.
// Aggregator.Run writes one Aggregate per iteration; Store.ListAggregates
// returns them tenant-scoped.
type Aggregate struct {
	TenantID      string
	PeriodStart   time.Time
	PeriodEnd     time.Time
	Metric        Metric
	Sum           float64
	Max           float64
	SnapshotCount int
}

// QueryFilter restricts reads. TenantID is mandatory.
type QueryFilter struct {
	TenantID string
	Metric   Metric // optional; empty = all metrics
	Since    time.Time
	Until    time.Time
	Limit    int
}
