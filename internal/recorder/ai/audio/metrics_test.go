package audio

import (
	"strings"
	"testing"
	"time"
)

func TestMetrics_RecordInference(t *testing.T) {
	m := NewMetrics()

	m.RecordInference("cam1", "Camera 1", EventGunshot, 50*time.Millisecond)
	m.RecordInference("cam1", "Camera 1", EventGunshot, 60*time.Millisecond)
	m.RecordInference("cam1", "Camera 1", EventGlassBreak, 45*time.Millisecond)

	snap := m.Snapshot()
	if snap.TotalInferences != 3 {
		t.Errorf("expected 3 total inferences, got %d", snap.TotalInferences)
	}
	if len(snap.EventTypes) != 2 {
		t.Errorf("expected 2 event types, got %d", len(snap.EventTypes))
	}
}

func TestMetrics_RecordDetection(t *testing.T) {
	m := NewMetrics()

	m.RecordDetection("cam1", "Camera 1", EventGunshot, 0.92, 800*time.Millisecond)
	m.RecordDetection("cam1", "Camera 1", EventGunshot, 0.88, 1200*time.Millisecond)

	snap := m.Snapshot()
	if snap.TotalDetections != 2 {
		t.Errorf("expected 2 total detections, got %d", snap.TotalDetections)
	}
	if len(snap.Cameras) != 1 {
		t.Fatalf("expected 1 camera, got %d", len(snap.Cameras))
	}
	cam := snap.Cameras[0]
	if cam.DetectionCounts[EventGunshot] != 2 {
		t.Errorf("expected 2 gunshot detections, got %d", cam.DetectionCounts[EventGunshot])
	}
}

func TestMetrics_FalsePositiveRate(t *testing.T) {
	m := NewMetrics()

	// 10 detections, 1 false positive = 10% FPR.
	for i := 0; i < 10; i++ {
		m.RecordDetection("cam1", "Camera 1", EventGunshot, 0.80, 500*time.Millisecond)
	}
	m.RecordFalsePositive("cam1", "Camera 1", EventGunshot)

	snap := m.Snapshot()
	cam := snap.Cameras[0]
	if cam.FPRates[EventGunshot] != 0.1 {
		t.Errorf("expected FPR 0.1, got %f", cam.FPRates[EventGunshot])
	}
}

func TestMetrics_E2ELatency(t *testing.T) {
	m := NewMetrics()

	m.RecordDetection("cam1", "Camera 1", EventSirenHorn, 0.85, 500*time.Millisecond)
	m.RecordDetection("cam1", "Camera 1", EventSirenHorn, 0.90, 1500*time.Millisecond)

	snap := m.Snapshot()
	cam := snap.Cameras[0]
	if cam.E2ELatency.Count != 2 {
		t.Errorf("expected 2 latency samples, got %d", cam.E2ELatency.Count)
	}
	if cam.E2ELatency.Mean < 0.5 || cam.E2ELatency.Mean > 1.5 {
		t.Errorf("expected mean latency around 1.0s, got %f", cam.E2ELatency.Mean)
	}
}

func TestMetrics_PrometheusExport(t *testing.T) {
	m := NewMetrics()

	m.RecordInference("cam1", "Camera 1", EventGunshot, 50*time.Millisecond)
	m.RecordDetection("cam1", "Camera 1", EventGunshot, 0.92, 800*time.Millisecond)

	output := m.PrometheusExport()

	expectedMetrics := []string{
		"nvr_audio_inferences_total",
		"nvr_audio_detections_total",
		"nvr_audio_event_inference_latency_seconds",
		"nvr_audio_camera_detections_total",
		"nvr_audio_camera_e2e_latency_seconds",
	}

	for _, metric := range expectedMetrics {
		if !strings.Contains(output, metric) {
			t.Errorf("Prometheus output missing metric: %s", metric)
		}
	}
}
