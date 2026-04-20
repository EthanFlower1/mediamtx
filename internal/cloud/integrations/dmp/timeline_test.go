package dmp

import (
	"testing"
	"time"

	"github.com/bluenviron/mediamtx/internal/recorder/db"
)

func TestTimelineIntegrator_IngestAlarmEvent(t *testing.T) {
	// Open an in-memory SQLite database for testing.
	database, err := db.Open(":memory:")
	if err != nil {
		t.Fatalf("failed to open test database: %v", err)
	}
	defer database.Close()

	// Create a camera so the foreign key constraint is satisfied.
	cam := &db.Camera{
		ID:   "cam-front-door",
		Name: "Front Door",
	}
	if err := database.CreateCamera(cam); err != nil {
		t.Fatalf("failed to create test camera: %v", err)
	}

	zm := NewZoneMapper()
	zm.Add(&ZoneMapping{
		AccountID: "1234",
		Zone:      1,
		Area:      1,
		CameraID:  "cam-front-door",
	})

	ti := &TimelineIntegrator{
		DB:            database,
		ZoneMapper:    zm,
		AlarmDuration: 30 * time.Second,
	}

	event := &AlarmEvent{
		AccountID:      "1234",
		EventCode:      CodeBurglaryAlarm,
		EventQualifier: "E",
		Zone:           1,
		Area:           1,
		Timestamp:      time.Now().UTC(),
		Description:    "Burglary alarm zone 1 area 1",
		Severity:       SeverityWarning,
	}

	camID := ti.IngestAlarmEvent(event)
	if camID != "cam-front-door" {
		t.Errorf("IngestAlarmEvent returned %q, want cam-front-door", camID)
	}

	// Verify the event was inserted into the timeline.
	now := time.Now().UTC()
	events, err := database.QueryMotionEvents("cam-front-door",
		now.Add(-1*time.Minute), now.Add(1*time.Minute))
	if err != nil {
		t.Fatalf("QueryMotionEvents failed: %v", err)
	}

	if len(events) != 1 {
		t.Fatalf("expected 1 timeline event, got %d", len(events))
	}

	me := events[0]
	if me.EventType != "alarm" {
		t.Errorf("EventType = %q, want alarm", me.EventType)
	}
	if me.ObjectClass != "dmp_BA" {
		t.Errorf("ObjectClass = %q, want dmp_BA", me.ObjectClass)
	}
	if me.Confidence != 1.0 {
		t.Errorf("Confidence = %f, want 1.0", me.Confidence)
	}
	if me.Metadata == nil {
		t.Fatal("Metadata should not be nil")
	}
}

func TestTimelineIntegrator_NoMapping(t *testing.T) {
	database, err := db.Open(":memory:")
	if err != nil {
		t.Fatalf("failed to open test database: %v", err)
	}
	defer database.Close()

	zm := NewZoneMapper() // empty mapper

	ti := &TimelineIntegrator{
		DB:         database,
		ZoneMapper: zm,
	}

	event := &AlarmEvent{
		AccountID:      "9999",
		EventCode:      CodeBurglaryAlarm,
		EventQualifier: "E",
		Zone:           1,
		Area:           1,
		Timestamp:      time.Now().UTC(),
		Severity:       SeverityWarning,
	}

	camID := ti.IngestAlarmEvent(event)
	if camID != "" {
		t.Errorf("expected empty camera ID for unmapped zone, got %q", camID)
	}
}

func TestTimelineIntegrator_NilDB(t *testing.T) {
	zm := NewZoneMapper()
	zm.Add(&ZoneMapping{AccountID: "1234", Zone: 1, Area: 1, CameraID: "cam-1"})

	ti := &TimelineIntegrator{
		DB:         nil,
		ZoneMapper: zm,
	}

	event := &AlarmEvent{
		AccountID: "1234",
		EventCode: CodeBurglaryAlarm,
		Zone:      1,
		Area:      1,
	}

	camID := ti.IngestAlarmEvent(event)
	if camID != "" {
		t.Errorf("expected empty camera ID with nil DB, got %q", camID)
	}
}
