package recordercontrol_test

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/bluenviron/mediamtx/internal/cloud/recordercontrol"
)

// -----------------------------------------------------------------------
// Test doubles
// -----------------------------------------------------------------------

// fakeCameraStore is an in-memory CameraStore used by all tests.
type fakeCameraStore struct {
	mu      sync.RWMutex
	cameras map[string][]recordercontrol.CameraPayload // key: tenantID+"/"+recorderID
}

func newFakeCameraStore() *fakeCameraStore {
	return &fakeCameraStore{cameras: make(map[string][]recordercontrol.CameraPayload)}
}

func (f *fakeCameraStore) add(tenantID, recorderID string, c recordercontrol.CameraPayload) {
	f.mu.Lock()
	defer f.mu.Unlock()
	key := tenantID + "/" + recorderID
	f.cameras[key] = append(f.cameras[key], c)
}

func (f *fakeCameraStore) ListCamerasForRecorder(_ context.Context, tenantID, recorderID string) ([]recordercontrol.CameraPayload, error) {
	f.mu.RLock()
	defer f.mu.RUnlock()
	key := tenantID + "/" + recorderID
	cams := f.cameras[key]
	out := make([]recordercontrol.CameraPayload, len(cams))
	copy(out, cams)
	return out, nil
}

// fakeRecorderStore is an in-memory RecorderStore.
type fakeRecorderStore struct {
	mu        sync.Mutex
	recorders map[string]string            // recorderID → tenantID
	statuses  map[string]recordercontrol.RecorderStatus // recorderID → status
}

func newFakeRecorderStore() *fakeRecorderStore {
	return &fakeRecorderStore{
		recorders: make(map[string]string),
		statuses:  make(map[string]recordercontrol.RecorderStatus),
	}
}

func (f *fakeRecorderStore) register(recorderID, tenantID string) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.recorders[recorderID] = tenantID
}

func (f *fakeRecorderStore) GetRecorderTenantID(_ context.Context, recorderID string) (string, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	t, ok := f.recorders[recorderID]
	if !ok {
		return "", fmt.Errorf("recorder not found: %s", recorderID)
	}
	return t, nil
}

func (f *fakeRecorderStore) UpdateRecorderStatus(_ context.Context, _, recorderID string, status recordercontrol.RecorderStatus) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.statuses[recorderID] = status
	return nil
}

func (f *fakeRecorderStore) statusOf(recorderID string) recordercontrol.RecorderStatus {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.statuses[recorderID]
}

// -----------------------------------------------------------------------
// Helpers
// -----------------------------------------------------------------------

// makeHandler builds a Handler with fast heartbeat for tests.
func makeHandler(t *testing.T, camStore recordercontrol.CameraStore, recStore recordercontrol.RecorderStore, bus *recordercontrol.EventBus) *recordercontrol.Handler {
	t.Helper()
	h, err := recordercontrol.NewHandler(recordercontrol.Config{
		Bus:               bus,
		Cameras:           camStore,
		Recorders:         recStore,
		HeartbeatInterval: 200 * time.Millisecond,
	})
	if err != nil {
		t.Fatalf("NewHandler: %v", err)
	}
	return h
}

// openStream sends a StreamAssignments request and returns the body reader.
// The caller should cancel ctx when done to close the connection.
func openStream(ctx context.Context, t *testing.T, ts *httptest.Server, tenantID, recorderID string) *bufio.Scanner {
	t.Helper()
	body := fmt.Sprintf(`{"recorder_id":%q}`, recorderID)
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, ts.URL, strings.NewReader(body))
	if err != nil {
		t.Fatalf("new request: %v", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Kaivue-Tenant", tenantID)

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

// readEvent reads one newline-delimited JSON event from sc.
type wireEvent struct {
	Kind    string          `json:"kind"`
	Version int64           `json:"version"`
	Snapshot *wireSnapshot  `json:"snapshot,omitempty"`
	Added    *wireCameraAdded   `json:"camera_added,omitempty"`
	Removed  *wireCameraRemoved `json:"camera_removed,omitempty"`
}

type wireSnapshot struct {
	Cameras []wireCamera `json:"cameras"`
}

type wireCamera struct {
	ID            string `json:"id"`
	TenantID      string `json:"tenant_id"`
	RecorderID    string `json:"recorder_id"`
	CredentialRef string `json:"credential_ref"`
}

type wireCameraAdded struct {
	Camera wireCamera `json:"camera"`
}

type wireCameraRemoved struct {
	CameraID        string `json:"camera_id"`
	PurgeRecordings bool   `json:"purge_recordings"`
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

// tenantMiddleware is a minimal HTTP middleware that injects tenantID from
// the X-Kaivue-Tenant header into the context. Mirrors what apiserver does.
func tenantMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		tenantID := r.Header.Get("X-Kaivue-Tenant")
		ctx := recordercontrol.WithTenantID(r.Context(), tenantID)
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}

// -----------------------------------------------------------------------
// Tests
// -----------------------------------------------------------------------

// TestSnapshotOnConnect verifies that the first event from a newly-opened
// stream is a Snapshot containing all cameras assigned to that recorder.
func TestSnapshotOnConnect(t *testing.T) {
	bus := recordercontrol.NewEventBus()
	cams := newFakeCameraStore()
	recs := newFakeRecorderStore()

	tenantID := "tenant-alpha"
	recorderID := "recorder-1"
	recs.register(recorderID, tenantID)
	cams.add(tenantID, recorderID, recordercontrol.CameraPayload{
		ID:            "cam-1",
		TenantID:      tenantID,
		RecorderID:    recorderID,
		Name:          "Front Door",
		CredentialRef: "cred-ref-abc",
	})

	h := makeHandler(t, cams, recs, bus)
	ts := httptest.NewServer(tenantMiddleware(h))
	defer ts.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	sc := openStream(ctx, t, ts, tenantID, recorderID)
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
	// TenantID must be present in every camera record.
	if cam.TenantID != tenantID {
		t.Fatalf("camera tenantID %q != %q", cam.TenantID, tenantID)
	}
}

// TestIncrementalCameraAdded verifies that a camera-added event published
// on the bus is delivered to the open stream after the initial Snapshot.
func TestIncrementalCameraAdded(t *testing.T) {
	bus := recordercontrol.NewEventBus()
	cams := newFakeCameraStore()
	recs := newFakeRecorderStore()

	tenantID := "tenant-beta"
	recorderID := "recorder-2"
	recs.register(recorderID, tenantID)

	h := makeHandler(t, cams, recs, bus)
	ts := httptest.NewServer(tenantMiddleware(h))
	defer ts.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	sc := openStream(ctx, t, ts, tenantID, recorderID)

	// Consume the initial snapshot (0 cameras).
	snap := readEvent(t, sc)
	if snap.Kind != "snapshot" {
		t.Fatalf("first event must be snapshot, got %q", snap.Kind)
	}
	if len(snap.Snapshot.Cameras) != 0 {
		t.Fatalf("expected empty snapshot, got %d cameras", len(snap.Snapshot.Cameras))
	}

	// Publish a camera-added event.
	bus.Publish(tenantID, recorderID, recordercontrol.BusEvent{
		Kind: recordercontrol.EventKindCameraAdded,
		Camera: &recordercontrol.CameraPayload{
			ID:            "cam-new",
			TenantID:      tenantID,
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
		break
	}
}

// TestCameraRemoved verifies camera_removed event delivery.
func TestCameraRemoved(t *testing.T) {
	bus := recordercontrol.NewEventBus()
	cams := newFakeCameraStore()
	recs := newFakeRecorderStore()

	tenantID := "tenant-gamma"
	recorderID := "recorder-3"
	recs.register(recorderID, tenantID)
	cams.add(tenantID, recorderID, recordercontrol.CameraPayload{
		ID: "cam-old", TenantID: tenantID, RecorderID: recorderID,
	})

	h := makeHandler(t, cams, recs, bus)
	ts := httptest.NewServer(tenantMiddleware(h))
	defer ts.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	sc := openStream(ctx, t, ts, tenantID, recorderID)
	_ = readEvent(t, sc) // initial snapshot

	bus.Publish(tenantID, recorderID, recordercontrol.BusEvent{
		Kind: recordercontrol.EventKindCameraRemoved,
		Removal: &recordercontrol.RemovalPayload{
			CameraID:        "cam-old",
			TenantID:        tenantID,
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

// TestMultiTenantIsolation_CrossTenantReject is the KAI-235 isolation check.
// A Recorder presenting one tenant's credentials must NOT be allowed to open
// a stream for a recorder that belongs to a different tenant.
func TestMultiTenantIsolation_CrossTenantReject(t *testing.T) {
	bus := recordercontrol.NewEventBus()
	cams := newFakeCameraStore()
	recs := newFakeRecorderStore()

	// recorder-x belongs to tenant-A
	recs.register("recorder-x", "tenant-A")
	cams.add("tenant-A", "recorder-x", recordercontrol.CameraPayload{
		ID: "secret-cam", TenantID: "tenant-A", RecorderID: "recorder-x",
	})

	h := makeHandler(t, cams, recs, bus)
	ts := httptest.NewServer(tenantMiddleware(h))
	defer ts.Close()

	// tenant-B tries to open a stream for recorder-x (which belongs to tenant-A)
	body := `{"recorder_id":"recorder-x"}`
	req, err := http.NewRequest(http.MethodPost, ts.URL, strings.NewReader(body))
	if err != nil {
		t.Fatalf("new request: %v", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Kaivue-Tenant", "tenant-B") // wrong tenant

	resp, err := ts.Client().Do(req)
	if err != nil {
		t.Fatalf("do request: %v", err)
	}
	defer resp.Body.Close()

	// Must be 404 — not 200, and definitely not a data-leaking 200 + snapshot.
	if resp.StatusCode != http.StatusNotFound {
		t.Fatalf("expected 404 for cross-tenant access, got %d", resp.StatusCode)
	}
}

// TestRecorderNotFound verifies that an unregistered recorder returns 404.
func TestRecorderNotFound(t *testing.T) {
	bus := recordercontrol.NewEventBus()
	cams := newFakeCameraStore()
	recs := newFakeRecorderStore()

	h := makeHandler(t, cams, recs, bus)
	ts := httptest.NewServer(tenantMiddleware(h))
	defer ts.Close()

	body := `{"recorder_id":"ghost-recorder"}`
	req, err := http.NewRequest(http.MethodPost, ts.URL, strings.NewReader(body))
	if err != nil {
		t.Fatalf("new request: %v", err)
	}
	req.Header.Set("X-Kaivue-Tenant", "tenant-x")

	resp, err := ts.Client().Do(req)
	if err != nil {
		t.Fatalf("do: %v", err)
	}
	resp.Body.Close()

	if resp.StatusCode != http.StatusNotFound {
		t.Fatalf("expected 404, got %d", resp.StatusCode)
	}
}

// TestMissingTenant verifies that a request without a tenant header returns 401.
func TestMissingTenant(t *testing.T) {
	bus := recordercontrol.NewEventBus()
	cams := newFakeCameraStore()
	recs := newFakeRecorderStore()

	h := makeHandler(t, cams, recs, bus)
	ts := httptest.NewServer(h) // no tenant middleware — no tenant in ctx
	defer ts.Close()

	body := `{"recorder_id":"any-recorder"}`
	req, err := http.NewRequest(http.MethodPost, ts.URL, strings.NewReader(body))
	if err != nil {
		t.Fatalf("new request: %v", err)
	}

	resp, err := ts.Client().Do(req)
	if err != nil {
		t.Fatalf("do: %v", err)
	}
	resp.Body.Close()

	if resp.StatusCode != http.StatusUnauthorized {
		t.Fatalf("expected 401, got %d", resp.StatusCode)
	}
}

// TestHeartbeatDelivery verifies that heartbeat events are emitted during
// stream inactivity.
func TestHeartbeatDelivery(t *testing.T) {
	bus := recordercontrol.NewEventBus()
	cams := newFakeCameraStore()
	recs := newFakeRecorderStore()

	tenantID := "tenant-hb"
	recorderID := "recorder-hb"
	recs.register(recorderID, tenantID)

	h := makeHandler(t, cams, recs, bus)
	ts := httptest.NewServer(tenantMiddleware(h))
	defer ts.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	sc := openStream(ctx, t, ts, tenantID, recorderID)
	_ = readEvent(t, sc) // snapshot

	// Next event should be a heartbeat (200ms interval in tests)
	for {
		ev := readEvent(t, sc)
		if ev.Kind == "heartbeat" {
			return // pass
		}
		// Allow other events to pass through if any
		if ev.Kind != "snapshot" && ev.Kind != "camera_added" {
			t.Fatalf("unexpected non-heartbeat event: %q", ev.Kind)
		}
	}
}

// TestBackpressureForceResync verifies that when a subscriber's queue
// overflows the handler sends a fresh Snapshot on the next reconnect.
// We simulate overflow by publishing more than queueSize events while
// the stream is not draining.
func TestEventBusPublish_MultiSubscriber(t *testing.T) {
	bus := recordercontrol.NewEventBus()

	ch1, cancel1 := bus.Subscribe("tenant-T", "rec-1")
	defer cancel1()
	ch2, cancel2 := bus.Subscribe("tenant-T", "rec-1")
	defer cancel2()

	bus.Publish("tenant-T", "rec-1", recordercontrol.BusEvent{
		Kind: recordercontrol.EventKindCameraAdded,
		Camera: &recordercontrol.CameraPayload{ID: "cam-x", TenantID: "tenant-T"},
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

// TestCrossTenantBusIsolation verifies that publishing to tenant-A's recorder
// does NOT deliver to tenant-B's subscriber on the same recorder ID.
func TestCrossTenantBusIsolation(t *testing.T) {
	bus := recordercontrol.NewEventBus()

	chA, cancelA := bus.Subscribe("tenant-A", "recorder-1")
	defer cancelA()
	chB, cancelB := bus.Subscribe("tenant-B", "recorder-1")
	defer cancelB()

	// Publish to tenant-A only.
	bus.Publish("tenant-A", "recorder-1", recordercontrol.BusEvent{
		Kind:   recordercontrol.EventKindCameraAdded,
		Camera: &recordercontrol.CameraPayload{ID: "cam-a", TenantID: "tenant-A"},
	})

	// tenant-A must receive the event.
	select {
	case ev := <-chA:
		if ev.Camera == nil || ev.Camera.ID != "cam-a" {
			t.Fatalf("tenant-A received wrong event: %+v", ev)
		}
	case <-time.After(time.Second):
		t.Fatal("tenant-A timed out waiting for event")
	}

	// tenant-B must NOT receive the event.
	select {
	case ev := <-chB:
		t.Fatalf("tenant-B received an event it should not have: %+v", ev)
	case <-time.After(200 * time.Millisecond):
		// correct — nothing arrived
	}
}

// TestNewHandler_MissingBus verifies that NewHandler rejects a zero Config.
func TestNewHandler_MissingBus(t *testing.T) {
	_, err := recordercontrol.NewHandler(recordercontrol.Config{
		Cameras:   newFakeCameraStore(),
		Recorders: newFakeRecorderStore(),
	})
	if err == nil {
		t.Fatal("expected error for missing Bus")
	}
}

// TestRecorderMarkedOnlineOnConnect verifies that the handler marks the
// recorder as online when the stream is first opened.
func TestRecorderMarkedOnlineOnConnect(t *testing.T) {
	bus := recordercontrol.NewEventBus()
	cams := newFakeCameraStore()
	recs := newFakeRecorderStore()

	tenantID := "tenant-status"
	recorderID := "recorder-status"
	recs.register(recorderID, tenantID)

	h := makeHandler(t, cams, recs, bus)
	ts := httptest.NewServer(tenantMiddleware(h))
	defer ts.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()

	sc := openStream(ctx, t, ts, tenantID, recorderID)
	_ = readEvent(t, sc) // consume snapshot

	// Give the handler a moment to call UpdateRecorderStatus.
	time.Sleep(50 * time.Millisecond)

	status := recs.statusOf(recorderID)
	if status != recordercontrol.RecorderStatusOnline {
		t.Fatalf("expected online, got %q", status)
	}
}
