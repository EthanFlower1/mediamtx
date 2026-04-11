package pdk_test

import (
	"context"
	"fmt"
	"path/filepath"
	"testing"

	clouddb "github.com/bluenviron/mediamtx/internal/cloud/db"
	"github.com/bluenviron/mediamtx/internal/cloud/integrations/pdk"
)

var seqID int

func testIDGen() string {
	seqID++
	return fmt.Sprintf("test-%04d", seqID)
}

func openTestDB(t *testing.T) *clouddb.DB {
	t.Helper()
	dir := t.TempDir()
	dsn := "sqlite://" + filepath.Join(dir, "cloud.db")
	d, err := clouddb.Open(context.Background(), dsn)
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	t.Cleanup(func() { _ = d.Close() })
	return d
}

func newService(t *testing.T) *pdk.Service {
	t.Helper()
	db := openTestDB(t)
	svc, err := pdk.NewService(pdk.Config{
		DB:    db,
		IDGen: testIDGen,
	})
	if err != nil {
		t.Fatalf("new service: %v", err)
	}
	return svc
}

func TestNewServiceRequiresDB(t *testing.T) {
	_, err := pdk.NewService(pdk.Config{})
	if err == nil {
		t.Fatal("expected error when DB is nil")
	}
}

func TestUpsertAndGetConfig(t *testing.T) {
	svc := newService(t)
	ctx := context.Background()

	cfg := pdk.IntegrationConfig{
		TenantID:      "tenant-1",
		APIEndpoint:   "https://pdk.example.com",
		ClientID:      "cid-123",
		ClientSecret:  "secret-456",
		PanelID:       "panel-A",
		WebhookSecret: "whsec-789",
		Enabled:       true,
		Status:        pdk.StatusConnected,
	}
	created, err := svc.UpsertConfig(ctx, cfg)
	if err != nil {
		t.Fatalf("upsert config: %v", err)
	}
	if created.ConfigID == "" {
		t.Fatal("expected config_id to be set")
	}

	got, err := svc.GetConfig(ctx, "tenant-1")
	if err != nil {
		t.Fatalf("get config: %v", err)
	}
	if got.ClientID != "cid-123" {
		t.Errorf("expected client_id cid-123, got %s", got.ClientID)
	}
	if got.PanelID != "panel-A" {
		t.Errorf("expected panel_id panel-A, got %s", got.PanelID)
	}
}

func TestGetConfigNotFound(t *testing.T) {
	svc := newService(t)
	ctx := context.Background()

	_, err := svc.GetConfig(ctx, "nonexistent")
	if err != pdk.ErrConfigNotFound {
		t.Errorf("expected ErrConfigNotFound, got %v", err)
	}
}

func TestDeleteConfig(t *testing.T) {
	svc := newService(t)
	ctx := context.Background()

	svc.UpsertConfig(ctx, pdk.IntegrationConfig{
		TenantID:    "tenant-1",
		APIEndpoint: "https://pdk.example.com",
		Enabled:     true,
		Status:      pdk.StatusConnected,
	})

	if err := svc.DeleteConfig(ctx, "tenant-1"); err != nil {
		t.Fatalf("delete: %v", err)
	}
	if err := svc.DeleteConfig(ctx, "tenant-1"); err != pdk.ErrConfigNotFound {
		t.Errorf("expected ErrConfigNotFound on second delete, got %v", err)
	}
}

func TestUpsertAndListDoors(t *testing.T) {
	svc := newService(t)
	ctx := context.Background()

	door, err := svc.UpsertDoor(ctx, pdk.Door{
		TenantID:  "tenant-1",
		PDKDoorID: "pdk-door-1",
		Name:      "Front Entrance",
		Location:  "Building A",
	})
	if err != nil {
		t.Fatalf("upsert door: %v", err)
	}
	if door.DoorID == "" {
		t.Fatal("expected door_id to be set")
	}

	doors, err := svc.ListDoors(ctx, "tenant-1")
	if err != nil {
		t.Fatalf("list doors: %v", err)
	}
	if len(doors) != 1 {
		t.Fatalf("expected 1 door, got %d", len(doors))
	}
	if doors[0].Name != "Front Entrance" {
		t.Errorf("expected Front Entrance, got %s", doors[0].Name)
	}
}

func TestGetDoorByPDKID(t *testing.T) {
	svc := newService(t)
	ctx := context.Background()

	svc.UpsertDoor(ctx, pdk.Door{
		TenantID:  "tenant-1",
		PDKDoorID: "pdk-door-42",
		Name:      "Side Door",
	})

	door, err := svc.GetDoorByPDKID(ctx, "tenant-1", "pdk-door-42")
	if err != nil {
		t.Fatalf("get door: %v", err)
	}
	if door.Name != "Side Door" {
		t.Errorf("expected Side Door, got %s", door.Name)
	}

	_, err = svc.GetDoorByPDKID(ctx, "tenant-1", "nonexistent")
	if err != pdk.ErrDoorNotFound {
		t.Errorf("expected ErrDoorNotFound, got %v", err)
	}
}

func TestInsertAndListEvents(t *testing.T) {
	svc := newService(t)
	ctx := context.Background()

	door, _ := svc.UpsertDoor(ctx, pdk.Door{
		TenantID:  "tenant-1",
		PDKDoorID: "pdk-d1",
		Name:      "Main",
	})

	event, err := svc.InsertEvent(ctx, pdk.DoorEvent{
		TenantID:   "tenant-1",
		DoorID:     door.DoorID,
		PDKEventID: "pdk-ev-1",
		EventType:  pdk.EventAccessGranted,
		PersonName: "Alice",
		Credential: "card-001",
	})
	if err != nil {
		t.Fatalf("insert event: %v", err)
	}
	if event.EventID == "" {
		t.Fatal("expected event_id to be set")
	}

	events, err := svc.ListEvents(ctx, "tenant-1", 10)
	if err != nil {
		t.Fatalf("list events: %v", err)
	}
	if len(events) != 1 {
		t.Fatalf("expected 1 event, got %d", len(events))
	}
	if events[0].PersonName != "Alice" {
		t.Errorf("expected Alice, got %s", events[0].PersonName)
	}
}

func TestUpsertAndListMappings(t *testing.T) {
	svc := newService(t)
	ctx := context.Background()

	door, _ := svc.UpsertDoor(ctx, pdk.Door{
		TenantID:  "tenant-1",
		PDKDoorID: "pdk-d1",
		Name:      "Lobby",
	})

	m, err := svc.UpsertMapping(ctx, pdk.DoorCameraMapping{
		TenantID:   "tenant-1",
		DoorID:     door.DoorID,
		CameraPath: "cam-lobby-1",
		PreBuffer:  5,
		PostBuffer: 20,
	})
	if err != nil {
		t.Fatalf("upsert mapping: %v", err)
	}
	if m.MappingID == "" {
		t.Fatal("expected mapping_id to be set")
	}

	mappings, err := svc.ListMappings(ctx, "tenant-1", door.DoorID)
	if err != nil {
		t.Fatalf("list mappings: %v", err)
	}
	if len(mappings) != 1 {
		t.Fatalf("expected 1 mapping, got %d", len(mappings))
	}
	if mappings[0].CameraPath != "cam-lobby-1" {
		t.Errorf("expected cam-lobby-1, got %s", mappings[0].CameraPath)
	}
}

func TestDeleteMapping(t *testing.T) {
	svc := newService(t)
	ctx := context.Background()

	door, _ := svc.UpsertDoor(ctx, pdk.Door{
		TenantID:  "tenant-1",
		PDKDoorID: "pdk-d1",
		Name:      "Gate",
	})
	m, _ := svc.UpsertMapping(ctx, pdk.DoorCameraMapping{
		TenantID:   "tenant-1",
		DoorID:     door.DoorID,
		CameraPath: "cam-gate",
	})

	if err := svc.DeleteMapping(ctx, "tenant-1", m.MappingID); err != nil {
		t.Fatalf("delete: %v", err)
	}
	if err := svc.DeleteMapping(ctx, "tenant-1", m.MappingID); err != pdk.ErrMappingNotFound {
		t.Errorf("expected ErrMappingNotFound, got %v", err)
	}
}

func TestCrossTenantIsolation(t *testing.T) {
	svc := newService(t)
	ctx := context.Background()

	svc.UpsertDoor(ctx, pdk.Door{
		TenantID:  "tenant-1",
		PDKDoorID: "door-a",
		Name:      "Tenant1 Door",
	})
	svc.UpsertDoor(ctx, pdk.Door{
		TenantID:  "tenant-2",
		PDKDoorID: "door-b",
		Name:      "Tenant2 Door",
	})

	d1, _ := svc.ListDoors(ctx, "tenant-1")
	d2, _ := svc.ListDoors(ctx, "tenant-2")

	if len(d1) != 1 || d1[0].Name != "Tenant1 Door" {
		t.Error("tenant-1 should only see its own door")
	}
	if len(d2) != 1 || d2[0].Name != "Tenant2 Door" {
		t.Error("tenant-2 should only see its own door")
	}
}
