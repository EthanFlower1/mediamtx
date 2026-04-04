// internal/nvr/ai/dedup_test.go
package ai

import (
	"context"
	"testing"
	"time"
)

func TestDedupSuppressesDuplicateEnteredEvents(t *testing.T) {
	in := make(chan TrackedFrame, 2)
	out := make(chan TrackedFrame, 2)
	d := NewDedup(in, out, DedupConfig{Window: 3 * time.Second, MinIoU: 0.5})

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go d.Run(ctx)

	now := time.Now()
	box := BoundingBox{0.1, 0.1, 0.3, 0.5}

	// First enter event passes through.
	in <- TrackedFrame{
		Timestamp: now,
		Objects: []TrackedObject{
			{TrackID: 1, State: ObjectEntered, Class: "person", Confidence: 0.8, Box: box},
		},
	}
	tf1 := <-out
	if len(tf1.Objects) != 1 {
		t.Fatalf("expected 1 object, got %d", len(tf1.Objects))
	}
	if tf1.Objects[0].TrackID != 1 {
		t.Errorf("expected trackID 1, got %d", tf1.Objects[0].TrackID)
	}

	// Second enter with overlapping box and same class within window is suppressed.
	overlapBox := BoundingBox{0.12, 0.12, 0.3, 0.5}
	in <- TrackedFrame{
		Timestamp: now.Add(500 * time.Millisecond),
		Objects: []TrackedObject{
			{TrackID: 2, State: ObjectEntered, Class: "person", Confidence: 0.85, Box: overlapBox},
		},
	}
	tf2 := <-out
	if len(tf2.Objects) != 0 {
		t.Errorf("expected 0 objects (suppressed), got %d", len(tf2.Objects))
	}
}

func TestDedupKeepsHighestConfidence(t *testing.T) {
	d := &Dedup{
		window: 3 * time.Second,
		minIoU: 0.5,
	}

	now := time.Now()
	box := BoundingBox{0.1, 0.1, 0.3, 0.5}

	// First event.
	tf1 := d.process(TrackedFrame{
		Timestamp: now,
		Objects: []TrackedObject{
			{TrackID: 1, State: ObjectEntered, Class: "person", Confidence: 0.7, Box: box},
		},
	})
	if len(tf1.Objects) != 1 {
		t.Fatalf("expected 1 object, got %d", len(tf1.Objects))
	}

	// Second event with higher confidence -- suppressed but updates representative.
	tf2 := d.process(TrackedFrame{
		Timestamp: now.Add(200 * time.Millisecond),
		Objects: []TrackedObject{
			{TrackID: 2, State: ObjectEntered, Class: "person", Confidence: 0.95, Box: box},
		},
	})
	if len(tf2.Objects) != 0 {
		t.Fatalf("expected suppressed, got %d", len(tf2.Objects))
	}

	// Verify the representative was updated with higher confidence.
	if len(d.recent) != 1 {
		t.Fatalf("expected 1 recent entry, got %d", len(d.recent))
	}
	if d.recent[0].confidence != 0.95 {
		t.Errorf("expected confidence 0.95, got %f", d.recent[0].confidence)
	}
}

func TestDedupAllowsAfterWindowExpires(t *testing.T) {
	d := &Dedup{
		window: 1 * time.Second,
		minIoU: 0.5,
	}

	now := time.Now()
	box := BoundingBox{0.1, 0.1, 0.3, 0.5}

	// First event passes.
	tf1 := d.process(TrackedFrame{
		Timestamp: now,
		Objects: []TrackedObject{
			{TrackID: 1, State: ObjectEntered, Class: "person", Confidence: 0.8, Box: box},
		},
	})
	if len(tf1.Objects) != 1 {
		t.Fatalf("expected 1, got %d", len(tf1.Objects))
	}

	// Same box after window expires should pass through.
	tf2 := d.process(TrackedFrame{
		Timestamp: now.Add(2 * time.Second),
		Objects: []TrackedObject{
			{TrackID: 3, State: ObjectEntered, Class: "person", Confidence: 0.8, Box: box},
		},
	})
	if len(tf2.Objects) != 1 {
		t.Errorf("expected 1 after window expired, got %d", len(tf2.Objects))
	}
}

func TestDedupDifferentClassNotSuppressed(t *testing.T) {
	d := &Dedup{
		window: 3 * time.Second,
		minIoU: 0.5,
	}

	now := time.Now()
	box := BoundingBox{0.1, 0.1, 0.3, 0.5}

	d.process(TrackedFrame{
		Timestamp: now,
		Objects: []TrackedObject{
			{TrackID: 1, State: ObjectEntered, Class: "person", Confidence: 0.8, Box: box},
		},
	})

	// Same box but different class should pass through.
	tf2 := d.process(TrackedFrame{
		Timestamp: now.Add(200 * time.Millisecond),
		Objects: []TrackedObject{
			{TrackID: 2, State: ObjectEntered, Class: "car", Confidence: 0.8, Box: box},
		},
	})
	if len(tf2.Objects) != 1 {
		t.Errorf("expected 1 (different class), got %d", len(tf2.Objects))
	}
}

func TestDedupNonOverlappingBoxNotSuppressed(t *testing.T) {
	d := &Dedup{
		window: 3 * time.Second,
		minIoU: 0.5,
	}

	now := time.Now()

	d.process(TrackedFrame{
		Timestamp: now,
		Objects: []TrackedObject{
			{TrackID: 1, State: ObjectEntered, Class: "person", Confidence: 0.8,
				Box: BoundingBox{0.1, 0.1, 0.2, 0.2}},
		},
	})

	// Same class but non-overlapping box.
	tf2 := d.process(TrackedFrame{
		Timestamp: now.Add(200 * time.Millisecond),
		Objects: []TrackedObject{
			{TrackID: 2, State: ObjectEntered, Class: "person", Confidence: 0.8,
				Box: BoundingBox{0.8, 0.8, 0.1, 0.1}},
		},
	})
	if len(tf2.Objects) != 1 {
		t.Errorf("expected 1 (non-overlapping), got %d", len(tf2.Objects))
	}
}

func TestDedupActiveAndLeftPassThrough(t *testing.T) {
	d := &Dedup{
		window: 3 * time.Second,
		minIoU: 0.5,
	}

	now := time.Now()
	box := BoundingBox{0.1, 0.1, 0.3, 0.5}

	tf := d.process(TrackedFrame{
		Timestamp: now,
		Objects: []TrackedObject{
			{TrackID: 1, State: ObjectActive, Class: "person", Confidence: 0.8, Box: box},
			{TrackID: 2, State: ObjectLeft, Class: "car", Confidence: 0.7, Box: box},
		},
	})
	if len(tf.Objects) != 2 {
		t.Errorf("expected 2 (active+left pass through), got %d", len(tf.Objects))
	}
}

func TestDedupDefaultConfig(t *testing.T) {
	in := make(chan TrackedFrame, 1)
	out := make(chan TrackedFrame, 1)
	d := NewDedup(in, out, DedupConfig{})

	if d.window != DefaultDedupWindow {
		t.Errorf("expected default window %v, got %v", DefaultDedupWindow, d.window)
	}
	if d.minIoU != DefaultDedupIoU {
		t.Errorf("expected default minIoU %f, got %f", DefaultDedupIoU, d.minIoU)
	}
}
