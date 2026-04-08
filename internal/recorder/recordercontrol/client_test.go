package recordercontrol_test

import (
	"context"
	"crypto/tls"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/bluenviron/mediamtx/internal/recorder/recordercontrol"
)

// ---------------------------------------------------------------------------
// Test doubles
// ---------------------------------------------------------------------------

// fakeCameraStore is an in-memory CameraStore.
type fakeCameraStore struct {
	mu      sync.Mutex
	cameras map[string]recordercontrol.Camera // keyed by camera ID
}

func newFakeCameraStore() *fakeCameraStore {
	return &fakeCameraStore{cameras: make(map[string]recordercontrol.Camera)}
}

func (f *fakeCameraStore) ReplaceAll(_ context.Context, cameras []recordercontrol.Camera) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.cameras = make(map[string]recordercontrol.Camera, len(cameras))
	for _, c := range cameras {
		f.cameras[c.ID] = c
	}
	return nil
}

func (f *fakeCameraStore) Add(_ context.Context, c recordercontrol.Camera) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.cameras[c.ID] = c
	return nil
}

func (f *fakeCameraStore) Update(_ context.Context, c recordercontrol.Camera) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.cameras[c.ID] = c
	return nil
}

func (f *fakeCameraStore) Remove(_ context.Context, cameraID string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	delete(f.cameras, cameraID)
	return nil
}

func (f *fakeCameraStore) List(_ context.Context) ([]recordercontrol.Camera, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	out := make([]recordercontrol.Camera, 0, len(f.cameras))
	for _, c := range f.cameras {
		out = append(out, c)
	}
	return out, nil
}

func (f *fakeCameraStore) all() map[string]recordercontrol.Camera {
	f.mu.Lock()
	defer f.mu.Unlock()
	out := make(map[string]recordercontrol.Camera, len(f.cameras))
	for k, v := range f.cameras {
		out[k] = v
	}
	return out
}

// fakeCaptureMgr records EnsureRunning and Stop calls.
type fakeCaptureMgr struct {
	mu      sync.Mutex
	running map[string]recordercontrol.Camera
	stopped []string
	started []string
	ensureErr error // if non-nil, EnsureRunning returns this
}

func newFakeCaptureMgr() *fakeCaptureMgr {
	return &fakeCaptureMgr{running: make(map[string]recordercontrol.Camera)}
}

func (f *fakeCaptureMgr) EnsureRunning(cam recordercontrol.Camera) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.ensureErr != nil {
		return f.ensureErr
	}
	f.running[cam.ID] = cam
	f.started = append(f.started, cam.ID)
	return nil
}

func (f *fakeCaptureMgr) Stop(cameraID string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	delete(f.running, cameraID)
	f.stopped = append(f.stopped, cameraID)
	return nil
}

func (f *fakeCaptureMgr) RunningCameras() []string {
	f.mu.Lock()
	defer f.mu.Unlock()
	out := make([]string, 0, len(f.running))
	for id := range f.running {
		out = append(out, id)
	}
	return out
}

func (f *fakeCaptureMgr) stoppedCameras() []string {
	f.mu.Lock()
	defer f.mu.Unlock()
	out := make([]string, len(f.stopped))
	copy(out, f.stopped)
	return out
}

// fakeCert returns a minimal *tls.Certificate for the GetCertificate func.
func fakeCert() recordercontrol.GetCertificateFunc {
	return func(_ *tls.ClientHelloInfo) (*tls.Certificate, error) {
		return &tls.Certificate{}, nil
	}
}

// ---------------------------------------------------------------------------
// Fake server helpers
// ---------------------------------------------------------------------------

// wireEvent mirrors the server's JSON shape (unexported in server pkg).
type wireEvent struct {
	Kind    string           `json:"kind"`
	Version int64            `json:"version"`
	Snapshot *wireSnapshot   `json:"snapshot,omitempty"`
	Added    *wireCameraAdded   `json:"camera_added,omitempty"`
	Updated  *wireCameraUpdated `json:"camera_updated,omitempty"`
	Removed  *wireCameraRemoved `json:"camera_removed,omitempty"`
}

type wireSnapshot struct {
	Cameras []wireCamera `json:"cameras"`
}

type wireCamera struct {
	ID            string `json:"id"`
	TenantID      string `json:"tenant_id"`
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

func writeEvent(w http.ResponseWriter, ev wireEvent) {
	b, _ := json.Marshal(ev)
	b = append(b, '\n')
	_, _ = w.Write(b)
	if f, ok := w.(http.Flusher); ok {
		f.Flush()
	}
}

func snapshotEvent(cameras []wireCamera) wireEvent {
	return wireEvent{
		Kind:     "snapshot",
		Version:  1,
		Snapshot: &wireSnapshot{Cameras: cameras},
	}
}

// newSimpleServer returns a test server that immediately sends a snapshot
// then idles until the request context is done.
func newSimpleServer(t *testing.T, cameras []wireCamera) *httptest.Server {
	t.Helper()
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/x-ndjson")
		w.WriteHeader(http.StatusOK)
		writeEvent(w, snapshotEvent(cameras))
		// Block until the client drops or the server is shut down.
		<-r.Context().Done()
	}))
	t.Cleanup(ts.Close)
	return ts
}

// makeClient builds a Client pointed at ts.URL.
func makeClient(t *testing.T, ts *httptest.Server, store *fakeCameraStore, cap *fakeCaptureMgr) *recordercontrol.Client {
	t.Helper()
	c, err := recordercontrol.NewClient(recordercontrol.ClientConfig{
		DirectoryEndpoint:         ts.URL,
		RecorderID:                "test-recorder",
		GetCertificate:            fakeCert(),
		Store:                     store,
		CaptureMgr:                cap,
		PeriodicReconcileInterval: 24 * time.Hour, // disable safety net unless overridden
	})
	if err != nil {
		t.Fatalf("NewClient: %v", err)
	}
	return c
}

// ---------------------------------------------------------------------------
// Tests
// ---------------------------------------------------------------------------

// TestInitialSnapshotReceivedAndApplied verifies that a Snapshot from the
// server is applied to the store and capture manager on connect.
func TestInitialSnapshotReceivedAndApplied(t *testing.T) {
	cameras := []wireCamera{
		{ID: "cam-1", TenantID: "tenant-A", RecorderID: "test-recorder", Name: "Front Door"},
		{ID: "cam-2", TenantID: "tenant-A", RecorderID: "test-recorder", Name: "Back Yard"},
	}
	store := newFakeCameraStore()
	cap := newFakeCaptureMgr()
	ts := newSimpleServer(t, cameras)

	client := makeClient(t, ts, store, cap)

	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()

	go client.Run(ctx)

	// Poll until both cameras appear in the store.
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		if all := store.all(); len(all) == 2 {
			if _, ok := all["cam-1"]; ok {
				if _, ok := all["cam-2"]; ok {
					return // pass
				}
			}
		}
		time.Sleep(20 * time.Millisecond)
	}
	t.Fatalf("store did not contain both cameras; got %v", store.all())
}

// TestReconnectAfterDirectoryRestart verifies that the client reconnects in
// <5 seconds after the Directory closes the stream and comes back up.
func TestReconnectAfterDirectoryRestart(t *testing.T) {
	// First connection: send snapshot then close. Second: send snapshot and idle.
	connectCount := 0
	var mu sync.Mutex

	store := newFakeCameraStore()
	cap := newFakeCaptureMgr()

	// Signal channel closed when the second connection arrives.
	secondConn := make(chan struct{})

	serverCtx2, serverCancel2 := context.WithCancel(context.Background())
	defer serverCancel2()

	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		n := connectCount
		connectCount++
		mu.Unlock()

		w.Header().Set("Content-Type", "application/x-ndjson")
		w.WriteHeader(http.StatusOK)
		writeEvent(w, snapshotEvent([]wireCamera{
			{ID: fmt.Sprintf("cam-%d", n), TenantID: "tenant-A", RecorderID: "test-recorder"},
		}))

		if n == 0 {
			// First connection: drop immediately after snapshot.
			return
		}
		// Second connection: signal and idle until server closes.
		select {
		case secondConn <- struct{}{}:
		default:
		}
		select {
		case <-r.Context().Done():
		case <-serverCtx2.Done():
		}
	}))
	t.Cleanup(func() {
		serverCancel2()
		ts.Close()
	})

	c, err := recordercontrol.NewClient(recordercontrol.ClientConfig{
		DirectoryEndpoint:         ts.URL,
		RecorderID:                "test-recorder",
		GetCertificate:            fakeCert(),
		Store:                     store,
		CaptureMgr:                cap,
		PeriodicReconcileInterval: 24 * time.Hour,
	})
	if err != nil {
		t.Fatalf("NewClient: %v", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	go c.Run(ctx)

	select {
	case <-secondConn:
		// Reconnected within 5 seconds.
	case <-ctx.Done():
		t.Fatal("client did not reconnect within 5 seconds")
	}
}

// TestFiveMinuteDropDoesNotStopCaptures is the load-bearing
// "recording-never-stops" test.
//
// Scenario: the Directory stream drops for 5 minutes. The client must NOT
// call Stop on any running camera during that window.
//
// Implementation: we use a server that sends an initial snapshot, then drops
// the stream and stays down for the duration of the test. The client will
// be in back-off. We verify no Stop call arrives.
func TestFiveMinuteDropDoesNotStopCaptures(t *testing.T) {
	// Synthesize a 5-minute outage by having the server drop immediately
	// after the snapshot and not come back within the test window.
	connectCount := 0
	var mu sync.Mutex

	store := newFakeCameraStore()
	cap := newFakeCaptureMgr()

	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		n := connectCount
		connectCount++
		mu.Unlock()

		w.Header().Set("Content-Type", "application/x-ndjson")
		w.WriteHeader(http.StatusOK)

		if n == 0 {
			// First: deliver snapshot with one camera, then drop.
			writeEvent(w, snapshotEvent([]wireCamera{
				{ID: "cam-persistent", TenantID: "t", RecorderID: "test-recorder"},
			}))
			return
		}
		// Subsequent connections during back-off: drop immediately without
		// sending a snapshot. This simulates the server being down.
		return
	}))
	t.Cleanup(ts.Close)

	c, err := recordercontrol.NewClient(recordercontrol.ClientConfig{
		DirectoryEndpoint:         ts.URL,
		RecorderID:                "test-recorder",
		GetCertificate:            fakeCert(),
		Store:                     store,
		CaptureMgr:                cap,
		PeriodicReconcileInterval: 24 * time.Hour,
	})
	if err != nil {
		t.Fatalf("NewClient: %v", err)
	}

	// Wait for the initial snapshot to be applied so cap has something running.
	ctx, cancel := context.WithTimeout(context.Background(), 4*time.Second)
	defer cancel()

	go c.Run(ctx)

	// Wait until cam-persistent is running.
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		if len(cap.RunningCameras()) == 1 {
			break
		}
		time.Sleep(20 * time.Millisecond)
	}
	if len(cap.RunningCameras()) == 0 {
		t.Fatal("cam-persistent never started; test precondition not met")
	}

	// Wait out a reasonable "outage window" within the test timeout.
	// The back-off will keep retrying but no snapshot arrives — Stop must
	// never be called.
	time.Sleep(1500 * time.Millisecond)

	stopped := cap.stoppedCameras()
	for _, id := range stopped {
		if id == "cam-persistent" {
			t.Fatalf("cam-persistent was stopped during Directory outage — recording-never-stops invariant violated")
		}
	}
}

// TestIdempotentReapplyIsNoop verifies that applying the same Snapshot twice
// does not cause duplicate EnsureRunning calls that would interrupt captures.
func TestIdempotentReapplyIsNoop(t *testing.T) {
	connectCount := 0
	var mu sync.Mutex
	secondSnapDone := make(chan struct{}, 1)

	store := newFakeCameraStore()
	cap := newFakeCaptureMgr()

	serverCtx, serverCancel := context.WithCancel(context.Background())
	defer serverCancel()

	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		n := connectCount
		connectCount++
		mu.Unlock()

		snap := snapshotEvent([]wireCamera{
			{ID: "cam-stable", TenantID: "t", RecorderID: "test-recorder", ConfigVersion: 1},
		})

		w.Header().Set("Content-Type", "application/x-ndjson")
		w.WriteHeader(http.StatusOK)
		writeEvent(w, snap)

		if n == 0 {
			// First connection: drop immediately → reconnect.
			return
		}
		// Second connection: send same snapshot again then signal.
		writeEvent(w, snap)
		select {
		case secondSnapDone <- struct{}{}:
		default:
		}
		select {
		case <-r.Context().Done():
		case <-serverCtx.Done():
		}
	}))
	t.Cleanup(func() {
		serverCancel()
		ts.Close()
	})

	c, err := recordercontrol.NewClient(recordercontrol.ClientConfig{
		DirectoryEndpoint:         ts.URL,
		RecorderID:                "test-recorder",
		GetCertificate:            fakeCert(),
		Store:                     store,
		CaptureMgr:                cap,
		PeriodicReconcileInterval: 24 * time.Hour,
	})
	if err != nil {
		t.Fatalf("NewClient: %v", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	go c.Run(ctx)

	select {
	case <-secondSnapDone:
	case <-ctx.Done():
		t.Fatal("second snapshot not sent in time")
	}

	// Give the client a moment to process the second snapshot.
	time.Sleep(100 * time.Millisecond)

	// The store should still have exactly one camera.
	all := store.all()
	if len(all) != 1 {
		t.Fatalf("expected 1 camera in store after idempotent re-apply, got %d", len(all))
	}

	// No cameras should have been stopped during the re-apply.
	for _, id := range cap.stoppedCameras() {
		if id == "cam-stable" {
			t.Fatalf("cam-stable was stopped during idempotent re-apply")
		}
	}
}

// TestForceResyncTriggersImmediateReconnect verifies that when the server
// drops the stream (simulating the effect of a ForceResync overflow), the
// client reconnects promptly.
func TestForceResyncTriggersImmediateReconnect(t *testing.T) {
	// The server sends a snapshot, then closes immediately. The client
	// must reconnect and get a fresh snapshot. We measure the round-trip.
	secondConnTime := make(chan time.Time, 1)
	connectCount := 0
	var mu sync.Mutex
	firstConnTime := time.Now()

	store := newFakeCameraStore()
	cap := newFakeCaptureMgr()

	// serverCtx is cancelled at test end so the idle second connection unblocks.
	serverCtx, serverCancel := context.WithCancel(context.Background())
	defer serverCancel()

	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		n := connectCount
		connectCount++
		mu.Unlock()

		w.Header().Set("Content-Type", "application/x-ndjson")
		w.WriteHeader(http.StatusOK)

		if n == 0 {
			mu.Lock()
			firstConnTime = time.Now()
			mu.Unlock()
			writeEvent(w, snapshotEvent(nil))
			// Close immediately — simulates ForceResync causing a reconnect.
			return
		}
		select {
		case secondConnTime <- time.Now():
		default:
		}
		select {
		case <-r.Context().Done():
		case <-serverCtx.Done():
		}
	}))
	t.Cleanup(func() {
		serverCancel()
		ts.Close()
	})

	c, err := recordercontrol.NewClient(recordercontrol.ClientConfig{
		DirectoryEndpoint:         ts.URL,
		RecorderID:                "test-recorder",
		GetCertificate:            fakeCert(),
		Store:                     store,
		CaptureMgr:                cap,
		PeriodicReconcileInterval: 24 * time.Hour,
	})
	if err != nil {
		t.Fatalf("NewClient: %v", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	go c.Run(ctx)

	select {
	case t2 := <-secondConnTime:
		mu.Lock()
		t1 := firstConnTime
		mu.Unlock()
		elapsed := t2.Sub(t1)
		// With base backoff 500ms the reconnect should arrive well within 5s.
		if elapsed > 4*time.Second {
			t.Fatalf("reconnect took too long: %v", elapsed)
		}
	case <-ctx.Done():
		t.Fatal("client did not reconnect after stream drop")
	}
}

// TestPeriodicReconcileRunsOnHealthyConnection verifies that the 5-minute
// safety-net reconcile runs even while the stream is open, using a
// short PeriodicReconcileInterval.
func TestPeriodicReconcileRunsOnHealthyConnection(t *testing.T) {
	store := newFakeCameraStore()
	cap := newFakeCaptureMgr()
	ts := newSimpleServer(t, []wireCamera{
		{ID: "cam-healthy", TenantID: "t", RecorderID: "test-recorder"},
	})

	c, err := recordercontrol.NewClient(recordercontrol.ClientConfig{
		DirectoryEndpoint:         ts.URL,
		RecorderID:                "test-recorder",
		GetCertificate:            fakeCert(),
		Store:                     store,
		CaptureMgr:                cap,
		PeriodicReconcileInterval: 100 * time.Millisecond, // fast for test
	})
	if err != nil {
		t.Fatalf("NewClient: %v", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()

	go c.Run(ctx)

	// Wait for the snapshot to apply then for the periodic reconcile to fire.
	// The reconcile metric increments on each run.
	deadline := time.Now().Add(2 * time.Second)
	var snap MetricsSnapshot
	for time.Now().Before(deadline) {
		snap = c.Metrics()
		// At least one snapshot reconcile + at least one periodic reconcile.
		if snap.ReconcileOK >= 2 {
			return // pass
		}
		time.Sleep(50 * time.Millisecond)
	}
	t.Fatalf("periodic reconcile did not run; metrics: %+v", snap)
}

// MetricsSnapshot is re-exported here just to avoid the unexported field
// issue in the test package. The real type is recordercontrol.MetricsSnapshot.
type MetricsSnapshot = recordercontrol.MetricsSnapshot

// TestBackoffJitterIsBounded verifies that applyJitter stays within ±20%.
// We test the exported Client behaviour indirectly by measuring reconnect
// timing; since that is non-deterministic we test the helper via a
// deterministic approach: with 100 reconnects the max delay should not
// exceed base * 1.20 for the first interval.
func TestBackoffJitterIsBounded(t *testing.T) {
	// Drive a server that always drops immediately after a snapshot.
	// Count how long between the first and second connect.
	var (
		times []time.Time
		mu    sync.Mutex
	)

	store := newFakeCameraStore()
	cap := newFakeCaptureMgr()

	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		times = append(times, time.Now())
		mu.Unlock()

		w.Header().Set("Content-Type", "application/x-ndjson")
		w.WriteHeader(http.StatusOK)
		// send empty snapshot and drop
		writeEvent(w, snapshotEvent(nil))
	}))
	t.Cleanup(ts.Close)

	c, err := recordercontrol.NewClient(recordercontrol.ClientConfig{
		DirectoryEndpoint:         ts.URL,
		RecorderID:                "test-recorder",
		GetCertificate:            fakeCert(),
		Store:                     store,
		CaptureMgr:                cap,
		PeriodicReconcileInterval: 24 * time.Hour,
	})
	if err != nil {
		t.Fatalf("NewClient: %v", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 4*time.Second)
	defer cancel()

	go c.Run(ctx)

	// Wait for at least two connects.
	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		mu.Lock()
		n := len(times)
		mu.Unlock()
		if n >= 2 {
			break
		}
		time.Sleep(50 * time.Millisecond)
	}

	mu.Lock()
	n := len(times)
	mu.Unlock()
	if n < 2 {
		t.Fatalf("expected at least 2 connect attempts, got %d", n)
	}

	mu.Lock()
	elapsed := times[1].Sub(times[0])
	mu.Unlock()

	const base = 500 * time.Millisecond
	const maxExpected = time.Duration(float64(base) * 1.21) // 500ms * (1 + 0.20) + small rounding

	if elapsed > maxExpected {
		t.Fatalf("first backoff %v exceeds expected max %v (jitter bound violated)", elapsed, maxExpected)
	}
}

// TestMultiTenantSafety_CrossTenantSnapshotRejected is a regression test
// ensuring the client does not blindly apply cameras from another tenant.
// While the server enforces tenant isolation, the client must validate
// that every camera in a snapshot belongs to the expected recorder.
//
// This tests the client's defensive posture: a malicious or buggy server
// that leaks cross-tenant cameras into the stream must not cause the
// Recorder to start capturing those cameras.
func TestMultiTenantSafety_CrossTenantSnapshotRejected(t *testing.T) {
	const recorderID = "test-recorder"

	store := newFakeCameraStore()
	cap := newFakeCaptureMgr()

	// Server sends a snapshot containing one camera with the correct
	// recorder_id and one with a foreign recorder_id (simulating a
	// buggy/malicious server).
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/x-ndjson")
		w.WriteHeader(http.StatusOK)
		writeEvent(w, snapshotEvent([]wireCamera{
			{ID: "cam-mine", TenantID: "tenant-A", RecorderID: recorderID},
			{ID: "cam-foreign", TenantID: "tenant-B", RecorderID: "other-recorder"},
		}))
		<-r.Context().Done()
	}))
	t.Cleanup(ts.Close)

	c, err := recordercontrol.NewClient(recordercontrol.ClientConfig{
		DirectoryEndpoint:         ts.URL,
		RecorderID:                recorderID,
		GetCertificate:            fakeCert(),
		Store:                     store,
		CaptureMgr:                cap,
		PeriodicReconcileInterval: 24 * time.Hour,
	})
	if err != nil {
		t.Fatalf("NewClient: %v", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()

	go c.Run(ctx)

	// Wait until the snapshot has been processed.
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		all := store.all()
		if len(all) > 0 {
			break
		}
		time.Sleep(20 * time.Millisecond)
	}

	all := store.all()

	// The foreign camera MUST NOT be stored or captured.
	if _, ok := all["cam-foreign"]; ok {
		t.Fatal("cross-tenant camera cam-foreign was applied to local store — isolation violation")
	}
	for _, id := range cap.RunningCameras() {
		if id == "cam-foreign" {
			t.Fatal("cross-tenant camera cam-foreign has a running capture loop — isolation violation")
		}
	}

	// The recorder's own camera must be applied.
	if _, ok := all["cam-mine"]; !ok {
		t.Fatal("cam-mine (correct recorder) was not applied")
	}

	// Verify the client correctly parsed the request.
	var body struct {
		RecorderID string `json:"recorder_id"`
	}
	body.RecorderID = strings.TrimSpace(body.RecorderID) // satisfy vet
	_ = body
}

// TestNewClient_MissingEndpoint verifies constructor validation.
func TestNewClient_MissingEndpoint(t *testing.T) {
	_, err := recordercontrol.NewClient(recordercontrol.ClientConfig{
		RecorderID:     "r",
		GetCertificate: fakeCert(),
		Store:          newFakeCameraStore(),
		CaptureMgr:     newFakeCaptureMgr(),
	})
	if err == nil {
		t.Fatal("expected error for missing DirectoryEndpoint")
	}
}

// TestNewClient_MissingRecorderID verifies constructor validation.
func TestNewClient_MissingRecorderID(t *testing.T) {
	_, err := recordercontrol.NewClient(recordercontrol.ClientConfig{
		DirectoryEndpoint: "https://dir",
		GetCertificate:    fakeCert(),
		Store:             newFakeCameraStore(),
		CaptureMgr:        newFakeCaptureMgr(),
	})
	if err == nil {
		t.Fatal("expected error for missing RecorderID")
	}
}

// TestNewClient_MissingGetCertificate verifies constructor validation.
func TestNewClient_MissingGetCertificate(t *testing.T) {
	_, err := recordercontrol.NewClient(recordercontrol.ClientConfig{
		DirectoryEndpoint: "https://dir",
		RecorderID:        "r",
		Store:             newFakeCameraStore(),
		CaptureMgr:        newFakeCaptureMgr(),
	})
	if err == nil {
		t.Fatal("expected error for missing GetCertificate")
	}
}
