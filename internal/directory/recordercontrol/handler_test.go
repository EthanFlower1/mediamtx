package recordercontrol_test

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/bluenviron/mediamtx/internal/directory/db"
	"github.com/bluenviron/mediamtx/internal/directory/recordercontrol"
)

// -----------------------------------------------------------------------
// Helpers
// -----------------------------------------------------------------------

// setupDB creates a fresh in-memory SQLite database with all migrations applied
// and seeds a recorder for testing.
func setupDB(t *testing.T, recorderID string) *db.DB {
	t.Helper()
	ctx := context.Background()
	d, err := db.Open(ctx, ":memory:")
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	t.Cleanup(func() { _ = d.Close() })

	// Seed a recorder.
	_, err = d.ExecContext(ctx,
		`INSERT INTO recorders (recorder_id, device_pubkey, token_id) VALUES (?, ?, ?)`,
		recorderID, "test-pubkey", "test-token",
	)
	if err != nil {
		t.Fatalf("seed recorder: %v", err)
	}
	return d
}

// makeHandler builds a Handler with fast heartbeat for tests.
func makeHandler(t *testing.T, store *recordercontrol.Store, bus *recordercontrol.EventBus) *recordercontrol.Handler {
	t.Helper()
	h, err := recordercontrol.NewHandler(recordercontrol.Config{
		Bus:               bus,
		Store:             store,
		HeartbeatInterval: 200 * time.Millisecond,
		RecorderAuthenticator: func(r *http.Request) (string, bool) {
			rid := r.Header.Get("X-Recorder-ID")
			return rid, rid != ""
		},
	})
	if err != nil {
		t.Fatalf("NewHandler: %v", err)
	}
	return h
}

// openStream sends a StreamAssignments request and returns the body reader.
// The caller should cancel ctx when done to close the connection.
func openStream(ctx context.Context, t *testing.T, ts *httptest.Server, recorderID string) *bufio.Scanner {
	t.Helper()
	body := fmt.Sprintf(`{"recorder_id":%q}`, recorderID)
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, ts.URL, strings.NewReader(body))
	if err != nil {
		t.Fatalf("new request: %v", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Recorder-ID", recorderID)

	resp, err := ts.Client().Do(req)
	if err != nil {
		t.Fatalf("do request: %v", err)
	}
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200, got %d", resp.StatusCode)
	}
	t.Cleanup(func() { resp.Body.Close() })
	return bufio.NewScanner(resp.Body)
}

// --- Wire types for test deserialization ---

type wireEvent struct {
	Kind     string             `json:"kind"`
	Version  int64              `json:"version"`
	Snapshot *wireSnapshot      `json:"snapshot,omitempty"`
	Added    *wireCameraAdded   `json:"camera_added,omitempty"`
	Updated  *wireCameraUpdated `json:"camera_updated,omitempty"`
	Removed  *wireCameraRemoved `json:"camera_removed,omitempty"`
}

type wireSnapshot struct {
	Cameras []wireCamera `json:"cameras"`
}

type wireCamera struct {
	ID            string `json:"id"`
	RecorderID    string `json:"recorder_id"`
	Name          string `json:"name"`
	CredentialRef string `json:"credential_ref"`
	ConfigJSON    string `json:"config_json"`
	ConfigVersion int64  `json:"config_version"`
}

type wireCameraAdded struct {
	Camera wireCamera `json:"camera"`
}

type wireCameraUpdated struct {
	Camera wireCamera `json:"camera"`
}

type wireCameraRemoved struct {
	CameraID        string `json:"camera_id"`
	PurgeRecordings bool   `json:"purge_recordings"`
	Reason          string `json:"reason,omitempty"`
}

func readEvent(t *testing.T, sc *bufio.Scanner) wireEvent {
	t.Helper()
	if !sc.Scan() {
		t.Fatalf("stream closed before event was received (scan err: %v)", sc.Err())
	}
	line := sc.Text()
	var ev wireEvent
	if err := json.Unmarshal([]byte(line), &ev); err != nil {
		t.Fatalf("unmarshal event %q: %v", line, err)
	}
	return ev
}

// -----------------------------------------------------------------------
// Tests
// -----------------------------------------------------------------------

// TestSnapshotOnConnect verifies that the first event from a newly-opened
// stream is a Snapshot containing all cameras assigned to that recorder.
func TestSnapshotOnConnect(t *testing.T) {
	recorderID := "recorder-1"
	d := setupDB(t, recorderID)
	bus := recordercontrol.NewEventBus()
	store := recordercontrol.NewStore(d.DB)

	// Seed a camera assignment.
	_, err := d.ExecContext(context.Background(),
		`INSERT INTO assigned_cameras (camera_id, recorder_id, name, credential_ref, config_json, config_version)
		 VALUES (?, ?, ?, ?, ?, ?)`,
		"cam-1", recorderID, "Front Door", "cred-ref-abc", "{}", 1,
	)
	if err != nil {
		t.Fatalf("seed camera: %v", err)
	}

	h := makeHandler(t, store, bus)
	ts := httptest.NewServer(h)
	defer ts.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	sc := openStream(ctx, t, ts, recorderID)
	ev := readEvent(t, sc)

	if ev.Kind != "snapshot" {
		t.Fatalf("expected snapshot, got %q", ev.Kind)
	}
	if ev.Snapshot == nil {
		t.Fatal("snapshot payload is nil")
	}
	if len(ev.Snapshot.Cameras) != 1 {
		t.Fatalf("expected 1 camera, got %d", len(ev.Snapshot.Cameras))
	}
	cam := ev.Snapshot.Cameras[0]
	if cam.ID != "cam-1" {
		t.Fatalf("expected cam-1, got %q", cam.ID)
	}
	if cam.CredentialRef != "cred-ref-abc" {
		t.Fatalf("expected cred-ref-abc, got %q", cam.CredentialRef)
	}
	if cam.Name != "Front Door" {
		t.Fatalf("expected Front Door, got %q", cam.Name)
	}
}

// TestEmptySnapshotOnConnect verifies that a recorder with no assigned cameras
// receives an empty snapshot.
func TestEmptySnapshotOnConnect(t *testing.T) {
	recorderID := "recorder-empty"
	d := setupDB(t, recorderID)
	bus := recordercontrol.NewEventBus()
	store := recordercontrol.NewStore(d.DB)

	h := makeHandler(t, store, bus)
	ts := httptest.NewServer(h)
	defer ts.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	sc := openStream(ctx, t, ts, recorderID)
	ev := readEvent(t, sc)

	if ev.Kind != "snapshot" {
		t.Fatalf("expected snapshot, got %q", ev.Kind)
	}
	if ev.Snapshot == nil {
		t.Fatal("snapshot payload is nil")
	}
	if len(ev.Snapshot.Cameras) != 0 {
		t.Fatalf("expected 0 cameras, got %d", len(ev.Snapshot.Cameras))
	}
}

// TestIncrementalCameraAdded verifies that a camera-added event published
// on the bus is delivered to the open stream after the initial Snapshot.
func TestIncrementalCameraAdded(t *testing.T) {
	recorderID := "recorder-2"
	d := setupDB(t, recorderID)
	bus := recordercontrol.NewEventBus()
	store := recordercontrol.NewStore(d.DB)

	h := makeHandler(t, store, bus)
	ts := httptest.NewServer(h)
	defer ts.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	sc := openStream(ctx, t, ts, recorderID)

	// Consume initial empty snapshot.
	snap := readEvent(t, sc)
	if snap.Kind != "snapshot" {
		t.Fatalf("first event must be snapshot, got %q", snap.Kind)
	}

	// Publish a camera-added event.
	bus.Publish(recorderID, recordercontrol.BusEvent{
		Kind: recordercontrol.EventKindCameraAdded,
		Camera: &recordercontrol.CameraRow{
			CameraID:      "cam-new",
			RecorderID:    recorderID,
			Name:          "Back Yard",
			CredentialRef: "cred-back",
		},
	})

	// Skip any heartbeats and wait for the camera_added event.
	for {
		ev := readEvent(t, sc)
		if ev.Kind == "heartbeat" {
			continue
		}
		if ev.Kind != "camera_added" {
			t.Fatalf("expected camera_added, got %q", ev.Kind)
		}
		if ev.Added == nil || ev.Added.Camera.ID != "cam-new" {
			t.Fatalf("unexpected camera_added payload: %+v", ev.Added)
		}
		if ev.Added.Camera.Name != "Back Yard" {
			t.Fatalf("expected Back Yard, got %q", ev.Added.Camera.Name)
		}
		break
	}
}

// TestCameraUpdated verifies camera_updated event delivery.
func TestCameraUpdated(t *testing.T) {
	recorderID := "recorder-upd"
	d := setupDB(t, recorderID)
	bus := recordercontrol.NewEventBus()
	store := recordercontrol.NewStore(d.DB)

	h := makeHandler(t, store, bus)
	ts := httptest.NewServer(h)
	defer ts.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	sc := openStream(ctx, t, ts, recorderID)
	_ = readEvent(t, sc) // initial snapshot

	bus.Publish(recorderID, recordercontrol.BusEvent{
		Kind: recordercontrol.EventKindCameraUpdated,
		Camera: &recordercontrol.CameraRow{
			CameraID:      "cam-1",
			RecorderID:    recorderID,
			Name:          "Updated Camera",
			ConfigVersion: 5,
		},
	})

	for {
		ev := readEvent(t, sc)
		if ev.Kind == "heartbeat" {
			continue
		}
		if ev.Kind != "camera_updated" {
			t.Fatalf("expected camera_updated, got %q", ev.Kind)
		}
		if ev.Updated == nil || ev.Updated.Camera.ID != "cam-1" {
			t.Fatalf("unexpected camera_updated payload: %+v", ev.Updated)
		}
		if ev.Updated.Camera.ConfigVersion != 5 {
			t.Fatalf("expected config_version 5, got %d", ev.Updated.Camera.ConfigVersion)
		}
		break
	}
}

// TestCameraRemoved verifies camera_removed event delivery.
func TestCameraRemoved(t *testing.T) {
	recorderID := "recorder-3"
	d := setupDB(t, recorderID)
	bus := recordercontrol.NewEventBus()
	store := recordercontrol.NewStore(d.DB)

	// Seed a camera to be removed.
	_, _ = d.ExecContext(context.Background(),
		`INSERT INTO assigned_cameras (camera_id, recorder_id, name, credential_ref, config_json, config_version)
		 VALUES (?, ?, ?, ?, ?, ?)`,
		"cam-old", recorderID, "Old Camera", "", "{}", 1,
	)

	h := makeHandler(t, store, bus)
	ts := httptest.NewServer(h)
	defer ts.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	sc := openStream(ctx, t, ts, recorderID)
	_ = readEvent(t, sc) // initial snapshot

	bus.Publish(recorderID, recordercontrol.BusEvent{
		Kind: recordercontrol.EventKindCameraRemoved,
		Removal: &recordercontrol.RemovalPayload{
			CameraID:        "cam-old",
			PurgeRecordings: true,
			Reason:          "decommissioned",
		},
	})

	for {
		ev := readEvent(t, sc)
		if ev.Kind == "heartbeat" {
			continue
		}
		if ev.Kind != "camera_removed" {
			t.Fatalf("expected camera_removed, got %q", ev.Kind)
		}
		if ev.Removed == nil || ev.Removed.CameraID != "cam-old" {
			t.Fatalf("unexpected removal: %+v", ev.Removed)
		}
		if !ev.Removed.PurgeRecordings {
			t.Fatalf("expected purge_recordings=true")
		}
		break
	}
}

// TestRecorderNotFound verifies that a request for an unregistered recorder
// returns 404.
func TestRecorderNotFound(t *testing.T) {
	d := setupDB(t, "other-recorder")
	bus := recordercontrol.NewEventBus()
	store := recordercontrol.NewStore(d.DB)

	h := makeHandler(t, store, bus)
	ts := httptest.NewServer(h)
	defer ts.Close()

	body := `{"recorder_id":"ghost-recorder"}`
	req, err := http.NewRequest(http.MethodPost, ts.URL, strings.NewReader(body))
	if err != nil {
		t.Fatalf("new request: %v", err)
	}
	req.Header.Set("X-Recorder-ID", "ghost-recorder")

	resp, err := ts.Client().Do(req)
	if err != nil {
		t.Fatalf("do: %v", err)
	}
	resp.Body.Close()

	if resp.StatusCode != http.StatusNotFound {
		t.Fatalf("expected 404, got %d", resp.StatusCode)
	}
}

// TestMissingAuth verifies that a request without recorder auth returns 401.
func TestMissingAuth(t *testing.T) {
	recorderID := "recorder-noauth"
	d := setupDB(t, recorderID)
	bus := recordercontrol.NewEventBus()
	store := recordercontrol.NewStore(d.DB)

	h := makeHandler(t, store, bus)
	ts := httptest.NewServer(h)
	defer ts.Close()

	body := `{"recorder_id":"recorder-noauth"}`
	req, err := http.NewRequest(http.MethodPost, ts.URL, strings.NewReader(body))
	if err != nil {
		t.Fatalf("new request: %v", err)
	}
	// No X-Recorder-ID header — auth fails.

	resp, err := ts.Client().Do(req)
	if err != nil {
		t.Fatalf("do: %v", err)
	}
	resp.Body.Close()

	if resp.StatusCode != http.StatusUnauthorized {
		t.Fatalf("expected 401, got %d", resp.StatusCode)
	}
}

// TestRecorderIDMismatch verifies that a mismatch between the authenticated
// recorder and the recorder_id in the request body returns 403.
func TestRecorderIDMismatch(t *testing.T) {
	recorderID := "recorder-real"
	d := setupDB(t, recorderID)
	bus := recordercontrol.NewEventBus()
	store := recordercontrol.NewStore(d.DB)

	h := makeHandler(t, store, bus)
	ts := httptest.NewServer(h)
	defer ts.Close()

	body := `{"recorder_id":"recorder-real"}`
	req, err := http.NewRequest(http.MethodPost, ts.URL, strings.NewReader(body))
	if err != nil {
		t.Fatalf("new request: %v", err)
	}
	// Auth says recorder-fake, body says recorder-real.
	req.Header.Set("X-Recorder-ID", "recorder-fake")

	resp, err := ts.Client().Do(req)
	if err != nil {
		t.Fatalf("do: %v", err)
	}
	resp.Body.Close()

	if resp.StatusCode != http.StatusForbidden {
		t.Fatalf("expected 403, got %d", resp.StatusCode)
	}
}

// TestHeartbeatDelivery verifies that heartbeat events are emitted during
// stream inactivity.
func TestHeartbeatDelivery(t *testing.T) {
	recorderID := "recorder-hb"
	d := setupDB(t, recorderID)
	bus := recordercontrol.NewEventBus()
	store := recordercontrol.NewStore(d.DB)

	h := makeHandler(t, store, bus)
	ts := httptest.NewServer(h)
	defer ts.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	sc := openStream(ctx, t, ts, recorderID)
	_ = readEvent(t, sc) // snapshot

	// Next event should be a heartbeat (200ms interval in tests).
	for {
		ev := readEvent(t, sc)
		if ev.Kind == "heartbeat" {
			return // pass
		}
		if ev.Kind != "snapshot" && ev.Kind != "camera_added" {
			t.Fatalf("unexpected non-heartbeat event: %q", ev.Kind)
		}
	}
}

// TestEventBusPublish_MultiSubscriber verifies that events are delivered to
// multiple subscribers of the same recorder.
func TestEventBusPublish_MultiSubscriber(t *testing.T) {
	bus := recordercontrol.NewEventBus()

	ch1, cancel1 := bus.Subscribe("rec-1")
	defer cancel1()
	ch2, cancel2 := bus.Subscribe("rec-1")
	defer cancel2()

	bus.Publish("rec-1", recordercontrol.BusEvent{
		Kind: recordercontrol.EventKindCameraAdded,
		Camera: &recordercontrol.CameraRow{
			CameraID:   "cam-x",
			RecorderID: "rec-1",
		},
	})

	timeout := time.After(2 * time.Second)

	select {
	case ev := <-ch1:
		if ev.Kind != recordercontrol.EventKindCameraAdded {
			t.Fatalf("ch1: expected CameraAdded, got %v", ev.Kind)
		}
	case <-timeout:
		t.Fatal("ch1: timed out waiting for event")
	}

	select {
	case ev := <-ch2:
		if ev.Kind != recordercontrol.EventKindCameraAdded {
			t.Fatalf("ch2: expected CameraAdded, got %v", ev.Kind)
		}
	case <-timeout:
		t.Fatal("ch2: timed out waiting for event")
	}
}

// TestBusIsolation verifies that publishing to recorder-A does NOT deliver
// to recorder-B's subscriber.
func TestBusIsolation(t *testing.T) {
	bus := recordercontrol.NewEventBus()

	chA, cancelA := bus.Subscribe("recorder-A")
	defer cancelA()
	chB, cancelB := bus.Subscribe("recorder-B")
	defer cancelB()

	// Publish to recorder-A only.
	bus.Publish("recorder-A", recordercontrol.BusEvent{
		Kind:   recordercontrol.EventKindCameraAdded,
		Camera: &recordercontrol.CameraRow{CameraID: "cam-a", RecorderID: "recorder-A"},
	})

	// recorder-A must receive the event.
	select {
	case ev := <-chA:
		if ev.Camera == nil || ev.Camera.CameraID != "cam-a" {
			t.Fatalf("recorder-A received wrong event: %+v", ev)
		}
	case <-time.After(time.Second):
		t.Fatal("recorder-A timed out waiting for event")
	}

	// recorder-B must NOT receive the event.
	select {
	case ev := <-chB:
		t.Fatalf("recorder-B received an event it should not have: %+v", ev)
	case <-time.After(200 * time.Millisecond):
		// correct — nothing arrived
	}
}

// TestNewHandler_MissingBus verifies that NewHandler rejects a zero Config.
func TestNewHandler_MissingBus(t *testing.T) {
	_, err := recordercontrol.NewHandler(recordercontrol.Config{
		Store: recordercontrol.NewStore(nil),
		RecorderAuthenticator: func(r *http.Request) (string, bool) {
			return "", false
		},
	})
	if err == nil {
		t.Fatal("expected error for missing Bus")
	}
}

// TestStoreRoundTrip verifies Insert, List, Update, and Delete operations
// on the SQLite-backed Store.
func TestStoreRoundTrip(t *testing.T) {
	recorderID := "recorder-store"
	d := setupDB(t, recorderID)
	store := recordercontrol.NewStore(d.DB)
	ctx := context.Background()

	// Initially empty.
	cameras, err := store.ListCamerasForRecorder(ctx, recorderID)
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if len(cameras) != 0 {
		t.Fatalf("expected 0 cameras, got %d", len(cameras))
	}

	// Insert.
	err = store.InsertCamera(ctx, recordercontrol.CameraRow{
		CameraID:      "cam-1",
		RecorderID:    recorderID,
		Name:          "Test Cam",
		CredentialRef: "cred-1",
		ConfigJSON:    `{"mode":"continuous"}`,
		ConfigVersion: 1,
	})
	if err != nil {
		t.Fatalf("insert: %v", err)
	}

	// List.
	cameras, err = store.ListCamerasForRecorder(ctx, recorderID)
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if len(cameras) != 1 {
		t.Fatalf("expected 1 camera, got %d", len(cameras))
	}
	if cameras[0].CameraID != "cam-1" {
		t.Fatalf("expected cam-1, got %q", cameras[0].CameraID)
	}
	if cameras[0].Name != "Test Cam" {
		t.Fatalf("expected Test Cam, got %q", cameras[0].Name)
	}

	// Get.
	cam, err := store.GetCamera(ctx, "cam-1")
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	if cam.CredentialRef != "cred-1" {
		t.Fatalf("expected cred-1, got %q", cam.CredentialRef)
	}

	// Update.
	err = store.UpdateCamera(ctx, recordercontrol.CameraRow{
		CameraID:      "cam-1",
		RecorderID:    recorderID,
		Name:          "Updated Cam",
		CredentialRef: "cred-2",
		ConfigJSON:    `{"mode":"motion"}`,
	})
	if err != nil {
		t.Fatalf("update: %v", err)
	}

	cam, err = store.GetCamera(ctx, "cam-1")
	if err != nil {
		t.Fatalf("get after update: %v", err)
	}
	if cam.Name != "Updated Cam" {
		t.Fatalf("expected Updated Cam, got %q", cam.Name)
	}
	if cam.ConfigVersion != 2 {
		t.Fatalf("expected config_version 2 after update, got %d", cam.ConfigVersion)
	}

	// Delete.
	n, err := store.DeleteCamera(ctx, "cam-1")
	if err != nil {
		t.Fatalf("delete: %v", err)
	}
	if n != 1 {
		t.Fatalf("expected 1 row deleted, got %d", n)
	}

	cameras, err = store.ListCamerasForRecorder(ctx, recorderID)
	if err != nil {
		t.Fatalf("list after delete: %v", err)
	}
	if len(cameras) != 0 {
		t.Fatalf("expected 0 cameras after delete, got %d", len(cameras))
	}
}

// TestRecorderExists verifies the recorder existence check.
func TestRecorderExists(t *testing.T) {
	recorderID := "recorder-exists"
	d := setupDB(t, recorderID)
	store := recordercontrol.NewStore(d.DB)
	ctx := context.Background()

	exists, err := store.RecorderExists(ctx, recorderID)
	if err != nil {
		t.Fatalf("exists: %v", err)
	}
	if !exists {
		t.Fatal("expected recorder to exist")
	}

	exists, err = store.RecorderExists(ctx, "nonexistent")
	if err != nil {
		t.Fatalf("exists: %v", err)
	}
	if exists {
		t.Fatal("expected recorder to not exist")
	}
}
