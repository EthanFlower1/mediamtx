package metering

import "context"

// Recorder is the write-side interface used by the hot path (Directory
// ingest, camera state handlers, AI inference call sites). It is a single
// method on purpose — callers should not need to know anything about
// storage to emit a usage event.
type Recorder interface {
	Record(ctx context.Context, e Event) error
}

// Compile-time check that *Store satisfies Recorder.
var _ Recorder = (*Store)(nil)

// RecordCameraHours is a small convenience wrapper so call sites do not
// need to build Event literals for the most common metric.
func RecordCameraHours(ctx context.Context, r Recorder, tenantID string, hours float64) error {
	return r.Record(ctx, Event{TenantID: tenantID, Metric: MetricCameraHours, Value: hours})
}

// RecordRecordingBytes is the wrapper for the recording-bytes metric.
func RecordRecordingBytes(ctx context.Context, r Recorder, tenantID string, bytes float64) error {
	return r.Record(ctx, Event{TenantID: tenantID, Metric: MetricRecordingBytes, Value: bytes})
}

// RecordAIInferences is the wrapper for the AI-inference-count metric.
func RecordAIInferences(ctx context.Context, r Recorder, tenantID string, count float64) error {
	return r.Record(ctx, Event{TenantID: tenantID, Metric: MetricAIInferenceCount, Value: count})
}
