package db

import (
	"testing"
	"time"
)

func TestDetectionEventCRUD(t *testing.T) {
	d := openTestDB(t)

	// Create a camera for FK.
	_, err := d.Exec(`INSERT INTO cameras (id, name) VALUES ('cam1', 'Test Camera')`)
	if err != nil {
		t.Fatalf("insert camera: %v", err)
	}

	ev := &DetectionEvent{
		CameraID:       "cam1",
		ZoneID:         "zone1",
		Class:          "person",
		StartTime:      "2026-04-01T10:00:00.000Z",
		EndTime:        "2026-04-01T10:00:05.000Z",
		PeakConfidence: 0.85,
		ThumbnailPath:  "/thumbs/1.jpg",
		DetectionCount: 3,
	}
	if err := d.InsertDetectionEvent(ev); err != nil {
		t.Fatalf("insert: %v", err)
	}
	if ev.ID == 0 {
		t.Fatal("expected non-zero ID after insert")
	}

	// Update.
	ev.EndTime = "2026-04-01T10:00:10.000Z"
	ev.PeakConfidence = 0.95
	ev.DetectionCount = 5
	if err := d.UpdateDetectionEvent(ev); err != nil {
		t.Fatalf("update: %v", err)
	}

	// Query.
	start, _ := time.Parse(time.RFC3339, "2026-04-01T09:00:00Z")
	end, _ := time.Parse(time.RFC3339, "2026-04-01T11:00:00Z")

	events, err := d.QueryDetectionEvents("cam1", "", start, end)
	if err != nil {
		t.Fatalf("query: %v", err)
	}
	if len(events) != 1 {
		t.Fatalf("expected 1 event, got %d", len(events))
	}
	if events[0].PeakConfidence != 0.95 {
		t.Errorf("expected peak confidence 0.95, got %f", events[0].PeakConfidence)
	}
	if events[0].DetectionCount != 5 {
		t.Errorf("expected detection count 5, got %d", events[0].DetectionCount)
	}

	// Query with class filter.
	events, err = d.QueryDetectionEvents("cam1", "car", start, end)
	if err != nil {
		t.Fatalf("query with class: %v", err)
	}
	if len(events) != 0 {
		t.Fatalf("expected 0 events for class car, got %d", len(events))
	}

	// GetLatest.
	latest, err := d.GetLatestDetectionEvent("cam1", "person", "zone1")
	if err != nil {
		t.Fatalf("get latest: %v", err)
	}
	if latest.ID != ev.ID {
		t.Errorf("expected latest ID %d, got %d", ev.ID, latest.ID)
	}
}

func TestDetectionAggregator(t *testing.T) {
	d := openTestDB(t)

	_, err := d.Exec(`INSERT INTO cameras (id, name) VALUES ('cam1', 'Test Camera')`)
	if err != nil {
		t.Fatalf("insert camera: %v", err)
	}

	agg := NewDetectionAggregator(d, 10*time.Second)

	// First detection — should create a new event.
	ev1, err := agg.Aggregate(AggregateInput{
		CameraID:   "cam1",
		Class:      "person",
		Confidence: 0.80,
		FrameTime:  "2026-04-01T10:00:00.000Z",
	})
	if err != nil {
		t.Fatalf("aggregate 1: %v", err)
	}
	if ev1.DetectionCount != 1 {
		t.Errorf("expected count 1, got %d", ev1.DetectionCount)
	}

	// Second detection within gap tolerance — should extend the event.
	ev2, err := agg.Aggregate(AggregateInput{
		CameraID:   "cam1",
		Class:      "person",
		Confidence: 0.90,
		FrameTime:  "2026-04-01T10:00:05.000Z",
	})
	if err != nil {
		t.Fatalf("aggregate 2: %v", err)
	}
	if ev2.ID != ev1.ID {
		t.Errorf("expected same event ID %d, got %d", ev1.ID, ev2.ID)
	}
	if ev2.DetectionCount != 2 {
		t.Errorf("expected count 2, got %d", ev2.DetectionCount)
	}
	if ev2.PeakConfidence != 0.90 {
		t.Errorf("expected peak 0.90, got %f", ev2.PeakConfidence)
	}

	// Third detection outside gap tolerance — should create new event.
	ev3, err := agg.Aggregate(AggregateInput{
		CameraID:   "cam1",
		Class:      "person",
		Confidence: 0.75,
		FrameTime:  "2026-04-01T10:00:20.000Z",
	})
	if err != nil {
		t.Fatalf("aggregate 3: %v", err)
	}
	if ev3.ID == ev1.ID {
		t.Error("expected new event after gap exceeded")
	}
	if ev3.DetectionCount != 1 {
		t.Errorf("expected count 1, got %d", ev3.DetectionCount)
	}

	// Different class — should create a separate event.
	ev4, err := agg.Aggregate(AggregateInput{
		CameraID:   "cam1",
		Class:      "car",
		Confidence: 0.60,
		FrameTime:  "2026-04-01T10:00:05.000Z",
	})
	if err != nil {
		t.Fatalf("aggregate 4: %v", err)
	}
	if ev4.ID == ev1.ID || ev4.ID == ev3.ID {
		t.Error("expected different event for different class")
	}
}

func TestDeleteDetectionEventsOlderThan(t *testing.T) {
	d := openTestDB(t)

	_, err := d.Exec(`INSERT INTO cameras (id, name) VALUES ('cam1', 'Test Camera')`)
	if err != nil {
		t.Fatalf("insert camera: %v", err)
	}

	// Insert an old event.
	old := &DetectionEvent{
		CameraID:       "cam1",
		Class:          "person",
		StartTime:      "2026-01-01T00:00:00.000Z",
		EndTime:        "2026-01-01T00:01:00.000Z",
		PeakConfidence: 0.5,
		DetectionCount: 1,
	}
	if err := d.InsertDetectionEvent(old); err != nil {
		t.Fatalf("insert old: %v", err)
	}

	// Insert a recent event.
	recent := &DetectionEvent{
		CameraID:       "cam1",
		Class:          "person",
		StartTime:      "2026-04-01T00:00:00.000Z",
		EndTime:        "2026-04-01T00:01:00.000Z",
		PeakConfidence: 0.9,
		DetectionCount: 1,
	}
	if err := d.InsertDetectionEvent(recent); err != nil {
		t.Fatalf("insert recent: %v", err)
	}

	cutoff, _ := time.Parse(time.RFC3339, "2026-03-01T00:00:00Z")
	n, err := d.DeleteDetectionEventsOlderThan(cutoff)
	if err != nil {
		t.Fatalf("delete: %v", err)
	}
	if n != 1 {
		t.Errorf("expected 1 deleted, got %d", n)
	}

	// Verify only recent remains.
	start, _ := time.Parse(time.RFC3339, "2025-01-01T00:00:00Z")
	end, _ := time.Parse(time.RFC3339, "2027-01-01T00:00:00Z")
	events, err := d.QueryDetectionEvents("cam1", "", start, end)
	if err != nil {
		t.Fatalf("query: %v", err)
	}
	if len(events) != 1 {
		t.Errorf("expected 1 event remaining, got %d", len(events))
	}
}
