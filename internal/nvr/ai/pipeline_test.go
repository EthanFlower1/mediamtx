// internal/nvr/ai/pipeline_test.go
package ai

import (
	"context"
	"testing"
	"time"
)

// mockEventPub implements EventPublisher for testing.
type mockEventPub struct {
	frames     [][]DetectionFrameData
	detections []string
}

func (m *mockEventPub) PublishAIDetection(camera, class string, conf float32) {
	m.detections = append(m.detections, class)
}

func (m *mockEventPub) PublishDetectionFrame(camera string, dets []DetectionFrameData) {
	m.frames = append(m.frames, dets)
}

func TestPipelineChannelWiring(t *testing.T) {
	// Test that data flows through the channel pipeline without a real FFmpeg
	// or YOLO model. We manually push frames into the detection channel and verify
	// tracked output arrives at the tracker output.

	detCh := make(chan DetectionFrame, 1)
	trackCh := make(chan TrackedFrame, 1)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	tracker := NewTracker(detCh, trackCh, 5)
	go tracker.Run(ctx)

	now := time.Now()

	// Simulate detector output.
	detCh <- DetectionFrame{
		Timestamp: now,
		Detections: []Detection{
			{Class: "person", Confidence: 0.9, Box: BoundingBox{0.1, 0.1, 0.3, 0.5}},
		},
	}

	// Read tracked output.
	select {
	case tf := <-trackCh:
		if len(tf.Objects) != 1 {
			t.Fatalf("expected 1 tracked object, got %d", len(tf.Objects))
		}
		if tf.Objects[0].Class != "person" {
			t.Errorf("class = %q, want person", tf.Objects[0].Class)
		}
		if tf.Objects[0].State != ObjectEntered {
			t.Errorf("state = %v, want entered", tf.Objects[0].State)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("timeout waiting for tracked frame")
	}
}
