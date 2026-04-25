// internal/nvr/ai/dedup_test.go
package ai

import (
	"context"
	"testing"
	"time"
)

func TestDedupSuppressesDuplicateEntry(t *testing.T) {
	in := make(chan TrackedFrame, 2)
	out := make(chan TrackedFrame, 2)
	d := NewDeduplicator(in, out, 5, 0.5)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go d.Run(ctx)

	now := time.Now()
	box := BoundingBox{0.1, 0.1, 0.4, 0.4}

	// First person enters.
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

	// Same class at overlapping position enters again within window.
	in <- TrackedFrame{
		Timestamp: now.Add(2 * time.Second),
		Objects: []TrackedObject{
			{TrackID: 2, State: ObjectEntered, Class: "person", Confidence: 0.85, Box: BoundingBox{0.12, 0.12, 0.4, 0.4}},
		},
	}
	tf2 := <-out
	if len(tf2.Objects) != 0 {
		t.Errorf("expected duplicate to be suppressed, got %d objects", len(tf2.Objects))
	}
}

func TestDedupKeepsHigherConfidence(t *testing.T) {
	d := &Deduplicator{
		window: 5 * time.Second,
		minIoU: 0.5,
	}

	now := time.Now()
	box := BoundingBox{0.1, 0.1, 0.4, 0.4}

	// Process first entry.
	tf1 := d.process(TrackedFrame{
		Timestamp: now,
		Objects: []TrackedObject{
			{TrackID: 1, State: ObjectEntered, Class: "person", Confidence: 0.7, Box: box},
		},
	})
	if len(tf1.Objects) != 1 {
		t.Fatalf("expected 1 object, got %d", len(tf1.Objects))
	}

	// Process duplicate with higher confidence.
	tf2 := d.process(TrackedFrame{
		Timestamp: now.Add(1 * time.Second),
		Objects: []TrackedObject{
			{TrackID: 2, State: ObjectEntered, Class: "person", Confidence: 0.95, Box: box},
		},
	})
	if len(tf2.Objects) != 0 {
		t.Errorf("expected suppressed, got %d objects", len(tf2.Objects))
	}

	// Verify confidence was updated in the recent entry.
	if len(d.recent) != 1 {
		t.Fatalf("expected 1 recent entry, got %d", len(d.recent))
	}
	if d.recent[0].confidence != 0.95 {
		t.Errorf("expected confidence 0.95, got %f", d.recent[0].confidence)
	}
}

func TestDedupAllowsAfterWindowExpires(t *testing.T) {
	d := &Deduplicator{
		window: 5 * time.Second,
		minIoU: 0.5,
	}

	now := time.Now()
	box := BoundingBox{0.1, 0.1, 0.4, 0.4}

	// First entry.
	d.process(TrackedFrame{
		Timestamp: now,
		Objects: []TrackedObject{
			{TrackID: 1, State: ObjectEntered, Class: "person", Confidence: 0.8, Box: box},
		},
	})

	// Same class/position after window expires should pass through.
	tf := d.process(TrackedFrame{
		Timestamp: now.Add(6 * time.Second),
		Objects: []TrackedObject{
			{TrackID: 3, State: ObjectEntered, Class: "person", Confidence: 0.8, Box: box},
		},
	})
	if len(tf.Objects) != 1 {
		t.Errorf("expected entry after window expiry, got %d objects", len(tf.Objects))
	}
}

func TestDedupAllowsDifferentClass(t *testing.T) {
	d := &Deduplicator{
		window: 5 * time.Second,
		minIoU: 0.5,
	}

	now := time.Now()
	box := BoundingBox{0.1, 0.1, 0.4, 0.4}

	d.process(TrackedFrame{
		Timestamp: now,
		Objects: []TrackedObject{
			{TrackID: 1, State: ObjectEntered, Class: "person", Confidence: 0.8, Box: box},
		},
	})

	// Different class at same position should pass through.
	tf := d.process(TrackedFrame{
		Timestamp: now.Add(1 * time.Second),
		Objects: []TrackedObject{
			{TrackID: 2, State: ObjectEntered, Class: "car", Confidence: 0.9, Box: box},
		},
	})
	if len(tf.Objects) != 1 {
		t.Errorf("expected different class to pass through, got %d objects", len(tf.Objects))
	}
}

func TestDedupAllowsDifferentPosition(t *testing.T) {
	d := &Deduplicator{
		window: 5 * time.Second,
		minIoU: 0.5,
	}

	now := time.Now()

	d.process(TrackedFrame{
		Timestamp: now,
		Objects: []TrackedObject{
			{TrackID: 1, State: ObjectEntered, Class: "person", Confidence: 0.8, Box: BoundingBox{0.1, 0.1, 0.2, 0.2}},
		},
	})

	// Same class at non-overlapping position should pass through.
	tf := d.process(TrackedFrame{
		Timestamp: now.Add(1 * time.Second),
		Objects: []TrackedObject{
			{TrackID: 2, State: ObjectEntered, Class: "person", Confidence: 0.8, Box: BoundingBox{0.7, 0.7, 0.2, 0.2}},
		},
	})
	if len(tf.Objects) != 1 {
		t.Errorf("expected non-overlapping detection to pass through, got %d objects", len(tf.Objects))
	}
}

func TestDedupPassesThroughActiveAndLeft(t *testing.T) {
	d := &Deduplicator{
		window: 5 * time.Second,
		minIoU: 0.5,
	}

	now := time.Now()
	box := BoundingBox{0.1, 0.1, 0.4, 0.4}

	// Active and Left events should always pass through.
	tf := d.process(TrackedFrame{
		Timestamp: now,
		Objects: []TrackedObject{
			{TrackID: 1, State: ObjectActive, Class: "person", Confidence: 0.8, Box: box},
			{TrackID: 2, State: ObjectLeft, Class: "car", Confidence: 0.7, Box: box},
		},
	})
	if len(tf.Objects) != 2 {
		t.Errorf("expected 2 objects (active+left), got %d", len(tf.Objects))
	}
}
