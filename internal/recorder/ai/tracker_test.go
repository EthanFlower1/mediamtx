// internal/nvr/ai/tracker_test.go
package ai

import (
	"context"
	"testing"
	"time"
)

func TestIoUBoxes(t *testing.T) {
	a := BoundingBox{0.1, 0.1, 0.5, 0.5}
	if got := iouBoxes(a, a); got < 0.99 {
		t.Errorf("identical boxes IoU = %f, want ~1.0", got)
	}

	b := BoundingBox{0.8, 0.8, 0.1, 0.1}
	if got := iouBoxes(a, b); got != 0 {
		t.Errorf("non-overlapping IoU = %f, want 0", got)
	}

	c := BoundingBox{0.3, 0.3, 0.5, 0.5}
	iou := iouBoxes(a, c)
	if iou < 0.1 || iou > 0.5 {
		t.Errorf("partial overlap IoU = %f, expected between 0.1 and 0.5", iou)
	}
}

func TestTrackerAssignsIDs(t *testing.T) {
	in := make(chan DetectionFrame, 1)
	out := make(chan TrackedFrame, 1)
	tr := NewTracker(in, out, 5)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go tr.Run(ctx)

	now := time.Now()
	in <- DetectionFrame{
		Timestamp: now,
		Detections: []Detection{
			{Class: "person", Confidence: 0.9, Box: BoundingBox{0.1, 0.1, 0.3, 0.5}},
			{Class: "car", Confidence: 0.8, Box: BoundingBox{0.6, 0.6, 0.2, 0.2}},
		},
	}

	tf := <-out
	if len(tf.Objects) != 2 {
		t.Fatalf("expected 2 objects, got %d", len(tf.Objects))
	}
	for _, obj := range tf.Objects {
		if obj.State != ObjectEntered {
			t.Errorf("track %d state = %v, want entered", obj.TrackID, obj.State)
		}
	}
	if tf.Objects[0].TrackID == tf.Objects[1].TrackID {
		t.Error("tracks should have different IDs")
	}
}

func TestTrackerMatchesAcrossFrames(t *testing.T) {
	in := make(chan DetectionFrame, 2)
	out := make(chan TrackedFrame, 2)
	tr := NewTracker(in, out, 5)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go tr.Run(ctx)

	now := time.Now()
	box := BoundingBox{0.1, 0.1, 0.3, 0.5}

	in <- DetectionFrame{
		Timestamp:  now,
		Detections: []Detection{{Class: "person", Confidence: 0.9, Box: box}},
	}
	tf1 := <-out
	enteredID := tf1.Objects[0].TrackID

	movedBox := BoundingBox{0.12, 0.12, 0.3, 0.5}
	in <- DetectionFrame{
		Timestamp:  now.Add(200 * time.Millisecond),
		Detections: []Detection{{Class: "person", Confidence: 0.92, Box: movedBox}},
	}
	tf2 := <-out
	if len(tf2.Objects) != 1 {
		t.Fatalf("expected 1 object, got %d", len(tf2.Objects))
	}
	if tf2.Objects[0].TrackID != enteredID {
		t.Errorf("track ID changed: got %d, want %d", tf2.Objects[0].TrackID, enteredID)
	}
	if tf2.Objects[0].State != ObjectActive {
		t.Errorf("state = %v, want active", tf2.Objects[0].State)
	}
}

func TestTrackerEmitsLeft(t *testing.T) {
	in := make(chan DetectionFrame, 10)
	out := make(chan TrackedFrame, 10)
	tr := NewTracker(in, out, 1)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go tr.Run(ctx)

	now := time.Now()

	in <- DetectionFrame{
		Timestamp:  now,
		Detections: []Detection{{Class: "person", Confidence: 0.9, Box: BoundingBox{0.1, 0.1, 0.3, 0.5}}},
	}
	<-out

	in <- DetectionFrame{
		Timestamp:  now.Add(2 * time.Second),
		Detections: nil,
	}
	tf := <-out

	foundLeft := false
	for _, obj := range tf.Objects {
		if obj.State == ObjectLeft && obj.Class == "person" {
			foundLeft = true
		}
	}
	if !foundLeft {
		t.Error("expected person to be marked as left")
	}
}
