package ai

import (
	"strings"
	"testing"
	"time"
)

func TestDetectionMetrics_RecordInference(t *testing.T) {
	m := NewDetectionMetrics()

	m.RecordInference("yolov8n", "cam1", "Front Door", 10*time.Millisecond, 3)
	m.RecordInference("yolov8n", "cam1", "Front Door", 20*time.Millisecond, 1)
	m.RecordInference("yolov8s", "cam2", "Backyard", 50*time.Millisecond, 2)

	snap := m.Snapshot()

	if snap.TotalInferences != 3 {
		t.Errorf("expected 3 total inferences, got %d", snap.TotalInferences)
	}
	if snap.TotalDetections != 6 {
		t.Errorf("expected 6 total detections, got %d", snap.TotalDetections)
	}

	if len(snap.Models) != 2 {
		t.Fatalf("expected 2 model entries, got %d", len(snap.Models))
	}
	if len(snap.Cameras) != 2 {
		t.Fatalf("expected 2 camera entries, got %d", len(snap.Cameras))
	}

	// Check per-camera stats.
	var cam1 *CameraMetrics
	for i := range snap.Cameras {
		if snap.Cameras[i].CameraID == "cam1" {
			cam1 = &snap.Cameras[i]
		}
	}
	if cam1 == nil {
		t.Fatal("cam1 not found in snapshot")
	}
	if cam1.InferenceCount != 2 {
		t.Errorf("expected cam1 inference_count=2, got %d", cam1.InferenceCount)
	}
	if cam1.TotalDetections != 4 {
		t.Errorf("expected cam1 total_detections=4, got %d", cam1.TotalDetections)
	}
	if cam1.TotalFrames != 2 {
		t.Errorf("expected cam1 total_frames=2, got %d", cam1.TotalFrames)
	}
}

func TestDetectionMetrics_QueueDepth(t *testing.T) {
	m := NewDetectionMetrics()

	m.SetQueueDepth(5)
	snap := m.Snapshot()
	if snap.QueueDepth != 5 {
		t.Errorf("expected queue_depth=5, got %d", snap.QueueDepth)
	}

	m.SetQueueDepth(3)
	snap = m.Snapshot()
	if snap.QueueDepth != 3 {
		t.Errorf("expected queue_depth=3 after update, got %d", snap.QueueDepth)
	}
}

func TestDetectionMetrics_FrameDrops(t *testing.T) {
	m := NewDetectionMetrics()

	m.RecordFrameDrop("cam1", "Front Door")
	m.RecordFrameDrop("cam1", "Front Door")
	m.RecordFrameDrop("cam2", "Backyard")

	snap := m.Snapshot()

	if snap.TotalFrameDrops != 3 {
		t.Errorf("expected 3 total frame drops, got %d", snap.TotalFrameDrops)
	}

	var cam1 *CameraMetrics
	for i := range snap.Cameras {
		if snap.Cameras[i].CameraID == "cam1" {
			cam1 = &snap.Cameras[i]
		}
	}
	if cam1 == nil {
		t.Fatal("cam1 not found in snapshot")
	}
	if cam1.DroppedFrames != 2 {
		t.Errorf("expected cam1 dropped_frames=2, got %d", cam1.DroppedFrames)
	}
	if cam1.TotalFrames != 2 {
		t.Errorf("expected cam1 total_frames=2, got %d", cam1.TotalFrames)
	}
	if cam1.DropRate != 1.0 {
		t.Errorf("expected cam1 drop_rate=1.0, got %f", cam1.DropRate)
	}
}

func TestDetectionMetrics_Percentiles(t *testing.T) {
	m := NewDetectionMetrics()

	// Add 100 samples with known latencies.
	for i := 1; i <= 100; i++ {
		m.RecordInference("yolov8n", "cam1", "Test", time.Duration(i)*time.Millisecond, 0)
	}

	snap := m.Snapshot()

	// Check global latency percentiles (in seconds).
	if snap.InferenceLatency.P50 < 0.045 || snap.InferenceLatency.P50 > 0.055 {
		t.Errorf("expected p50 ~0.050s, got %g", snap.InferenceLatency.P50)
	}
	if snap.InferenceLatency.P95 < 0.090 || snap.InferenceLatency.P95 > 0.100 {
		t.Errorf("expected p95 ~0.095s, got %g", snap.InferenceLatency.P95)
	}
	if snap.InferenceLatency.Count != 100 {
		t.Errorf("expected count=100, got %d", snap.InferenceLatency.Count)
	}
}

func TestDetectionMetrics_MixedInferencesAndDrops(t *testing.T) {
	m := NewDetectionMetrics()

	m.RecordInference("yolov8n", "cam1", "Front Door", 10*time.Millisecond, 2)
	m.RecordFrameDrop("cam1", "Front Door")
	m.RecordInference("yolov8n", "cam1", "Front Door", 15*time.Millisecond, 1)

	snap := m.Snapshot()

	var cam1 *CameraMetrics
	for i := range snap.Cameras {
		if snap.Cameras[i].CameraID == "cam1" {
			cam1 = &snap.Cameras[i]
		}
	}
	if cam1 == nil {
		t.Fatal("cam1 not found")
	}

	if cam1.TotalFrames != 3 {
		t.Errorf("expected total_frames=3, got %d", cam1.TotalFrames)
	}
	if cam1.DroppedFrames != 1 {
		t.Errorf("expected dropped_frames=1, got %d", cam1.DroppedFrames)
	}
	expectedRate := 1.0 / 3.0
	if cam1.DropRate < expectedRate-0.01 || cam1.DropRate > expectedRate+0.01 {
		t.Errorf("expected drop_rate ~%.3f, got %f", expectedRate, cam1.DropRate)
	}
}

func TestDetectionMetrics_PrometheusExport(t *testing.T) {
	m := NewDetectionMetrics()

	m.RecordInference("yolov8n", "cam1", "Front Door", 10*time.Millisecond, 2)
	m.SetQueueDepth(3)
	m.RecordFrameDrop("cam2", "Backyard")

	output := m.PrometheusExport()

	expected := []string{
		"# TYPE nvr_ai_inferences_total counter",
		"nvr_ai_inferences_total 1",
		"# TYPE nvr_ai_detections_total counter",
		"nvr_ai_detections_total 2",
		"# TYPE nvr_ai_frame_drops_total counter",
		"nvr_ai_frame_drops_total 1",
		"# TYPE nvr_ai_queue_depth gauge",
		"nvr_ai_queue_depth 3",
		"# TYPE nvr_ai_inference_latency_seconds summary",
		"# TYPE nvr_ai_model_inference_latency_seconds summary",
		`model="yolov8n"`,
		"# TYPE nvr_ai_camera_frames_total counter",
		"# TYPE nvr_ai_camera_frame_drops_total counter",
		"# TYPE nvr_ai_camera_detections_total counter",
		"# TYPE nvr_ai_camera_drop_rate gauge",
	}

	for _, s := range expected {
		if !strings.Contains(output, s) {
			t.Errorf("prometheus output missing: %s", s)
		}
	}
}

func TestDetectionMetrics_EmptySnapshot(t *testing.T) {
	m := NewDetectionMetrics()
	snap := m.Snapshot()

	if snap.TotalInferences != 0 {
		t.Errorf("expected 0 inferences, got %d", snap.TotalInferences)
	}
	if snap.TotalDetections != 0 {
		t.Errorf("expected 0 detections, got %d", snap.TotalDetections)
	}
	if snap.TotalFrameDrops != 0 {
		t.Errorf("expected 0 frame drops, got %d", snap.TotalFrameDrops)
	}
	if snap.QueueDepth != 0 {
		t.Errorf("expected 0 queue depth, got %d", snap.QueueDepth)
	}
	if len(snap.Models) != 0 {
		t.Errorf("expected 0 models, got %d", len(snap.Models))
	}
	if len(snap.Cameras) != 0 {
		t.Errorf("expected 0 cameras, got %d", len(snap.Cameras))
	}
}
