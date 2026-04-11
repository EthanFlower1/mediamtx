package pdk_test

import (
	"context"
	"fmt"
	"path/filepath"
	"testing"
	"time"

	clouddb "github.com/bluenviron/mediamtx/internal/cloud/db"
	"github.com/bluenviron/mediamtx/internal/cloud/integrations/pdk"
)

func newCorrelationService(t *testing.T) *pdk.Service {
	t.Helper()
	dir := t.TempDir()
	dsn := "sqlite://" + filepath.Join(dir, "cloud.db")
	db, err := clouddb.Open(context.Background(), dsn)
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })

	var cSeq int
	svc, err := pdk.NewService(pdk.Config{
		DB: db,
		IDGen: func() string {
			cSeq++
			return fmt.Sprintf("cor-%04d", cSeq)
		},
	})
	if err != nil {
		t.Fatalf("new service: %v", err)
	}
	return svc
}

func TestCorrelateEvent_NoMappings(t *testing.T) {
	svc := newCorrelationService(t)
	ctx := context.Background()

	door, _ := svc.UpsertDoor(ctx, pdk.Door{
		TenantID:  "tenant-1",
		PDKDoorID: "d1",
		Name:      "Unmapped Door",
	})

	event := pdk.DoorEvent{
		EventID:    "ev-1",
		TenantID:   "tenant-1",
		DoorID:     door.DoorID,
		EventType:  pdk.EventAccessGranted,
		OccurredAt: time.Now(),
	}

	correlations, err := svc.CorrelateEvent(ctx, "tenant-1", event)
	if err != nil {
		t.Fatalf("correlate: %v", err)
	}
	if len(correlations) != 0 {
		t.Errorf("expected 0 correlations for unmapped door, got %d", len(correlations))
	}
}

func TestCorrelateEvent_WithMappings(t *testing.T) {
	svc := newCorrelationService(t)
	ctx := context.Background()

	door, _ := svc.UpsertDoor(ctx, pdk.Door{
		TenantID:  "tenant-1",
		PDKDoorID: "d1",
		Name:      "Mapped Door",
	})

	svc.UpsertMapping(ctx, pdk.DoorCameraMapping{
		TenantID:   "tenant-1",
		DoorID:     door.DoorID,
		CameraPath: "cam-lobby",
		PreBuffer:  5,
		PostBuffer: 15,
	})
	svc.UpsertMapping(ctx, pdk.DoorCameraMapping{
		TenantID:   "tenant-1",
		DoorID:     door.DoorID,
		CameraPath: "cam-hallway",
		PreBuffer:  10,
		PostBuffer: 30,
	})

	eventTime := time.Date(2026, 4, 10, 14, 30, 0, 0, time.UTC)
	event := pdk.DoorEvent{
		EventID:    "ev-2",
		TenantID:   "tenant-1",
		DoorID:     door.DoorID,
		EventType:  pdk.EventAccessGranted,
		OccurredAt: eventTime,
	}

	correlations, err := svc.CorrelateEvent(ctx, "tenant-1", event)
	if err != nil {
		t.Fatalf("correlate: %v", err)
	}
	if len(correlations) != 2 {
		t.Fatalf("expected 2 correlations, got %d", len(correlations))
	}

	// Check the first correlation (cam-hallway comes first alphabetically).
	c := correlations[0]
	if c.CameraPath != "cam-hallway" && c.CameraPath != "cam-lobby" {
		t.Errorf("unexpected camera path: %s", c.CameraPath)
	}
	if c.EventID != "ev-2" {
		t.Errorf("expected event ev-2, got %s", c.EventID)
	}

	// Verify clip window for the lobby camera (5s pre, 15s post).
	for _, cor := range correlations {
		if cor.CameraPath == "cam-lobby" {
			expectedStart := eventTime.Add(-5 * time.Second)
			expectedEnd := eventTime.Add(15 * time.Second)
			if !cor.ClipStart.Equal(expectedStart) {
				t.Errorf("clip start: expected %v, got %v", expectedStart, cor.ClipStart)
			}
			if !cor.ClipEnd.Equal(expectedEnd) {
				t.Errorf("clip end: expected %v, got %v", expectedEnd, cor.ClipEnd)
			}
		}
	}
}

func TestIngestWebhookEvent_AutoCreatesDoor(t *testing.T) {
	svc := newCorrelationService(t)
	ctx := context.Background()

	// Configure tenant.
	svc.UpsertConfig(ctx, pdk.IntegrationConfig{
		TenantID:      "tenant-1",
		WebhookSecret: "secret",
		Enabled:       true,
		Status:        pdk.StatusConnected,
	})

	// Ingest an event for a door that does not exist yet.
	payload := pdk.WebhookPayload{
		EventID:    "ev-new",
		DoorID:     "unknown-door",
		EventType:  "access.granted",
		PersonName: "Charlie",
		Credential: "card-999",
		Timestamp:  time.Now().UTC(),
		Raw:        `{"event_id":"ev-new"}`,
	}

	err := svc.IngestWebhookEvent(ctx, "tenant-1", payload)
	if err != nil {
		t.Fatalf("ingest: %v", err)
	}

	// The door should have been auto-created.
	door, err := svc.GetDoorByPDKID(ctx, "tenant-1", "unknown-door")
	if err != nil {
		t.Fatalf("door not auto-created: %v", err)
	}
	if door.Name != "Door unknown-door" {
		t.Errorf("expected auto-name 'Door unknown-door', got %s", door.Name)
	}

	// The event should be persisted.
	events, err := svc.ListEvents(ctx, "tenant-1", 10)
	if err != nil {
		t.Fatalf("list events: %v", err)
	}
	if len(events) != 1 {
		t.Fatalf("expected 1 event, got %d", len(events))
	}
}

func TestGetCorrelations(t *testing.T) {
	svc := newCorrelationService(t)
	ctx := context.Background()

	svc.UpsertConfig(ctx, pdk.IntegrationConfig{
		TenantID:      "tenant-1",
		WebhookSecret: "s",
		Enabled:       true,
		Status:        pdk.StatusConnected,
	})

	door, _ := svc.UpsertDoor(ctx, pdk.Door{
		TenantID:  "tenant-1",
		PDKDoorID: "d1",
		Name:      "Door1",
	})
	svc.UpsertMapping(ctx, pdk.DoorCameraMapping{
		TenantID:   "tenant-1",
		DoorID:     door.DoorID,
		CameraPath: "cam-1",
	})

	payload := pdk.WebhookPayload{
		EventID:   "ev-cor",
		DoorID:    "d1",
		EventType: "access.granted",
		Timestamp: time.Now().UTC(),
		Raw:       "{}",
	}
	svc.IngestWebhookEvent(ctx, "tenant-1", payload)

	// Find the event to get its ID.
	events, _ := svc.ListEvents(ctx, "tenant-1", 10)
	if len(events) == 0 {
		t.Fatal("no events found")
	}

	cors, err := svc.GetCorrelations(ctx, "tenant-1", events[0].EventID)
	if err != nil {
		t.Fatalf("get correlations: %v", err)
	}
	if len(cors) != 1 {
		t.Fatalf("expected 1 correlation, got %d", len(cors))
	}
	if cors[0].CameraPath != "cam-1" {
		t.Errorf("expected cam-1, got %s", cors[0].CameraPath)
	}
}
