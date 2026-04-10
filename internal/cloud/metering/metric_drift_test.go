package metering_test

import (
	"testing"

	"github.com/bluenviron/mediamtx/internal/cloud/metering"
)

// TestMetricDrift pins the metric string values and the shape of the
// AllMetrics list. If a metric is added or renamed, this test must be
// updated in the same commit that updates billing rules consuming it.
// Team-wide policy: drift-guard tests pin cross-package constants.
func TestMetricDrift(t *testing.T) {
	cases := []struct {
		name string
		got  metering.Metric
		want string
	}{
		{"camera_hours", metering.MetricCameraHours, "camera_hours"},
		{"recording_bytes", metering.MetricRecordingBytes, "recording_bytes"},
		{"ai_inference_count", metering.MetricAIInferenceCount, "ai_inference_count"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if string(tc.got) != tc.want {
				t.Errorf("metric string drifted: got %q want %q", tc.got, tc.want)
			}
			if !tc.got.Valid() {
				t.Errorf("metric %q reported !Valid", tc.got)
			}
		})
	}

	// Pinning the length keeps reviewers honest when somebody adds a
	// metric: this test breaks, and they must update billing rules in
	// the same PR.
	if len(metering.AllMetrics) != 3 {
		t.Errorf("AllMetrics size drifted: got %d want 3", len(metering.AllMetrics))
	}
}
