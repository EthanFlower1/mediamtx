package recorder

// boot_smoke_test.go — end-to-end Phase 1 smoke tests.
//
// These tests assemble the capture-loop components directly (state.Store +
// mediamtxsupervisor + capturemanager + recordinghealth watchdog + recovery)
// without driving the full Boot() function.  Boot() requires a pairing state,
// mesh node, Directory TLS setup, and running tsnet — too many fixtures for a
// CI smoke test.  The seams exercised here are the same ones Boot() wires; the
// gap (pairing/mesh) is documented as a follow-up.
//
// Scenario coverage:
//   1. TestSmoke_CameraAssignment_PushesToMediaMTX
//   2. TestSmoke_Watchdog_DetectsDriftAndReloads
//   3. TestSmoke_RecoveryScan_ReconcilesDBToDisk
//   4. TestSmoke_NoopMediaMTX_BootCompletes
//   5. TestSmoke_DiskMonitor_TriggersRetention (skipped — covered by diskmonitor/monitor_test.go)

import (
	"bytes"
	"context"
	"encoding/binary"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/bluenviron/mediamtx/internal/recorder/capturemanager"
	"github.com/bluenviron/mediamtx/internal/recorder/mediamtxsupervisor"
	"github.com/bluenviron/mediamtx/internal/recorder/recordercontrol"
	"github.com/bluenviron/mediamtx/internal/recorder/recordinghealth"
	"github.com/bluenviron/mediamtx/internal/recorder/recovery"
	"github.com/bluenviron/mediamtx/internal/recorder/state"
)

// ---- helpers ---------------------------------------------------------------

// discardSlog returns a logger that discards all output.
func discardSlog() *slog.Logger {
	return slog.New(slog.NewTextHandler(io.Discard, nil))
}

// waitCondition polls cond every 5 ms until it returns true or the deadline
// (2 s) is reached.
func waitCondition(t *testing.T, cond func() bool) {
	t.Helper()
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		if cond() {
			return
		}
		time.Sleep(5 * time.Millisecond)
	}
	t.Fatal("condition not met within 2 s")
}

// openTestStore opens a fresh state.Store backed by a temp SQLite file.
func openTestStore(t *testing.T) *state.Store {
	t.Helper()
	path := filepath.Join(t.TempDir(), "state.db")
	st, err := state.Open(path, state.Options{})
	if err != nil {
		t.Fatalf("open state store: %v", err)
	}
	t.Cleanup(func() { st.Close() })
	return st
}

// upsertCam is a convenience wrapper for seeding a camera into a Store.
func upsertCam(t *testing.T, st *state.Store, id, rtspURL string) {
	t.Helper()
	cam := state.AssignedCamera{
		CameraID: id,
		Config: state.CameraConfig{
			ID:      id,
			Name:    id,
			RTSPURL: rtspURL,
		},
	}
	if err := st.UpsertCamera(context.Background(), cam); err != nil {
		t.Fatalf("upsert camera %s: %v", id, err)
	}
}

// fakeMTXServer spins up an httptest.Server that implements the subset of the
// mediamtx v3 API needed by the supervisor's HTTPController.
//
// It tracks all POST /v3/config/paths/add/{name} calls so tests can assert
// against them.
type fakeMTXServer struct {
	srv *httptest.Server

	mu      sync.Mutex
	added   []addedPath
	patched []string
	deleted []string

	// configPaths is the server-side "database" of configured paths.
	configPaths map[string]struct{}

	// runtimeReadyByName controls the runtime /v3/paths/list response.
	// If a name is in this set, it is returned as ready:true.
	runtimeReadyByName map[string]bool
}

type addedPath struct {
	Name string
	Body []byte
}

func newFakeMTXServer(t *testing.T) *fakeMTXServer {
	t.Helper()
	f := &fakeMTXServer{
		configPaths:        make(map[string]struct{}),
		runtimeReadyByName: make(map[string]bool),
	}

	mux := http.NewServeMux()

	// health probe
	mux.HandleFunc("/v3/config/global/get", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{}`))
	})

	// config paths list — used by HTTPController to diff existing vs desired
	mux.HandleFunc("/v3/config/paths/list", func(w http.ResponseWriter, r *http.Request) {
		f.mu.Lock()
		names := make([]string, 0, len(f.configPaths))
		for n := range f.configPaths {
			names = append(names, n)
		}
		f.mu.Unlock()

		w.Header().Set("Content-Type", "application/json")
		var sb strings.Builder
		sb.WriteString(fmt.Sprintf(`{"itemCount":%d,"pageCount":1,"items":[`, len(names)))
		for i, n := range names {
			if i > 0 {
				sb.WriteString(",")
			}
			fmt.Fprintf(&sb, `{"name":%q}`, n)
		}
		sb.WriteString(`]}`)
		_, _ = w.Write([]byte(sb.String()))
	})

	// add path
	mux.HandleFunc("/v3/config/paths/add/", func(w http.ResponseWriter, r *http.Request) {
		name := strings.TrimPrefix(r.URL.Path, "/v3/config/paths/add/")
		body, _ := io.ReadAll(r.Body)
		f.mu.Lock()
		f.added = append(f.added, addedPath{Name: name, Body: body})
		f.configPaths[name] = struct{}{}
		f.mu.Unlock()
		w.WriteHeader(http.StatusOK)
	})

	// patch path
	mux.HandleFunc("/v3/config/paths/patch/", func(w http.ResponseWriter, r *http.Request) {
		name := strings.TrimPrefix(r.URL.Path, "/v3/config/paths/patch/")
		f.mu.Lock()
		f.patched = append(f.patched, name)
		f.mu.Unlock()
		w.WriteHeader(http.StatusOK)
	})

	// delete path
	mux.HandleFunc("/v3/config/paths/delete/", func(w http.ResponseWriter, r *http.Request) {
		name := strings.TrimPrefix(r.URL.Path, "/v3/config/paths/delete/")
		f.mu.Lock()
		f.deleted = append(f.deleted, name)
		delete(f.configPaths, name)
		f.mu.Unlock()
		w.WriteHeader(http.StatusOK)
	})

	// runtime paths list — used by watchdog's HTTPRuntimeClient
	mux.HandleFunc("/v3/paths/list", func(w http.ResponseWriter, r *http.Request) {
		f.mu.Lock()
		var items []string
		for name, ready := range f.runtimeReadyByName {
			readyStr := "false"
			if ready {
				readyStr = "true"
			}
			items = append(items, fmt.Sprintf(`{"name":%q,"ready":%s}`, name, readyStr))
		}
		f.mu.Unlock()

		body := fmt.Sprintf(`{"itemCount":%d,"pageCount":1,"items":[%s]}`,
			len(items), strings.Join(items, ","))
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(body))
	})

	f.srv = httptest.NewServer(mux)
	t.Cleanup(f.srv.Close)
	return f
}

// snapAdded returns a copy of the added-path list under the lock.
func (f *fakeMTXServer) snapAdded() []addedPath {
	f.mu.Lock()
	defer f.mu.Unlock()
	out := make([]addedPath, len(f.added))
	copy(out, f.added)
	return out
}

// ---- fMP4 builder (inlined from recovery/repair_test.go) -------------------

func smokeBox(boxType string, payload []byte) []byte {
	size := uint32(8 + len(payload))
	buf := make([]byte, size)
	binary.BigEndian.PutUint32(buf[0:4], size)
	copy(buf[4:8], boxType)
	copy(buf[8:], payload)
	return buf
}

func smokeFtyp() []byte {
	payload := make([]byte, 12)
	copy(payload[0:4], "isom")
	return smokeBox("ftyp", payload)
}

func smokeMoov() []byte {
	mvhd := smokeBox("mvhd", make([]byte, 100))
	return smokeBox("moov", mvhd)
}

func smokeMoof(seqNum uint32) []byte {
	mfhdPayload := make([]byte, 8)
	binary.BigEndian.PutUint32(mfhdPayload[4:8], seqNum)
	mfhd := smokeBox("mfhd", mfhdPayload)

	trunPayload := make([]byte, 16)
	trunPayload[3] = 0x01
	trunPayload[2] = 0x01
	binary.BigEndian.PutUint32(trunPayload[4:8], 1)
	binary.BigEndian.PutUint32(trunPayload[8:12], 1000)
	binary.BigEndian.PutUint32(trunPayload[12:16], 100)

	tfhd := smokeBox("tfhd", make([]byte, 4))
	traf := smokeBox("traf", append(tfhd, smokeBox("trun", trunPayload)...))

	return smokeBox("moof", append(mfhd, traf...))
}

func smokeMdat(size int) []byte {
	return smokeBox("mdat", make([]byte, size))
}

func buildSmokeValidFMP4(numFragments int) []byte {
	var data []byte
	data = append(data, smokeFtyp()...)
	data = append(data, smokeMoov()...)
	for i := 0; i < numFragments; i++ {
		data = append(data, smokeMoof(uint32(i+1))...)
		data = append(data, smokeMdat(256)...)
	}
	return data
}

// ---- captureHandler for log capture ----------------------------------------

type smokeCaptureHandler struct {
	fn func(slog.Record)
}

func (h *smokeCaptureHandler) Enabled(_ context.Context, _ slog.Level) bool { return true }
func (h *smokeCaptureHandler) Handle(_ context.Context, r slog.Record) error {
	h.fn(r)
	return nil
}
func (h *smokeCaptureHandler) WithAttrs(_ []slog.Attr) slog.Handler { return h }
func (h *smokeCaptureHandler) WithGroup(_ string) slog.Handler      { return h }

// ============================================================================
// Scenario 1 — Camera assignment → supervisor push to fake mediamtx
// ============================================================================

// TestSmoke_CameraAssignment_PushesToMediaMTX verifies the full seam:
//
//  1. Open a real state.Store.
//  2. Upsert a camera with a valid RTSP URL.
//  3. Construct a MediaMTXSupervisor wired to a real HTTPController targeting
//     the fake server.
//  4. Construct a capturemanager.Manager and call EnsureRunning.
//  5. Assert the fake server received POST /v3/config/paths/add/cam_<id>
//     containing the expected source URL.
func TestSmoke_CameraAssignment_PushesToMediaMTX(t *testing.T) {
	t.Parallel()

	const camID = "smoke-cam-1"
	const rtspURL = "rtsp://10.0.1.1/stream"

	// 1. State store
	st := openTestStore(t)
	upsertCam(t, st, camID, rtspURL)

	// 2. Fake mediamtx server
	fake := newFakeMTXServer(t)

	// 3. Supervisor
	ctrl := &mediamtxsupervisor.HTTPController{
		BaseURL:    fake.srv.URL,
		PathPrefix: "cam_",
	}
	sup, err := mediamtxsupervisor.New(mediamtxsupervisor.Config{
		Source:       mediamtxsupervisor.StoreSource{Store: st},
		Controller:   ctrl,
		PollInterval: -1, // no autonomous polling; we drive via Reload
		Render: mediamtxsupervisor.RenderOptions{
			PathPrefix: "cam_",
		},
		Logger: discardSlog(),
	})
	if err != nil {
		t.Fatalf("new supervisor: %v", err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	if err := sup.Start(ctx); err != nil {
		// Fail-open: supervisor logs but returns error; initial reload hit the
		// empty store so there might be nothing to apply.  Continue.
		t.Logf("supervisor start (initial reload) warning: %v", err)
	}
	t.Cleanup(sup.Close)

	// 4. CaptureManager triggers a Reload when a new camera is registered.
	capMgr := capturemanager.New(capturemanager.Config{
		Reload: sup.Reload,
		Logger: discardSlog(),
	})

	if err := capMgr.EnsureRunning(recordercontrol.Camera{
		ID:            camID,
		ConfigVersion: 1,
	}); err != nil {
		t.Fatalf("EnsureRunning: %v", err)
	}

	// 5. Wait for the fake server to receive the ADD for cam_smoke-cam-1.
	waitCondition(t, func() bool {
		for _, ap := range fake.snapAdded() {
			if ap.Name == "cam_"+camID {
				return true
			}
		}
		return false
	})

	// Assert the request body contained the expected source URL.
	added := fake.snapAdded()
	var found *addedPath
	for i := range added {
		if added[i].Name == "cam_"+camID {
			found = &added[i]
			break
		}
	}
	if found == nil {
		t.Fatalf("expected ADD for cam_%s, got %v", camID, added)
	}

	// The body should contain the RTSP URL (credentials may be embedded, but
	// the host/path must appear).
	if !bytes.Contains(found.Body, []byte("10.0.1.1")) {
		t.Errorf("body %s: expected rtsp host 10.0.1.1", found.Body)
	}

	// Verify the supervisor stats reflect the reload.
	stats := sup.Stats()
	if stats.PathsApplied < 1 {
		t.Errorf("PathsApplied=%d want >= 1", stats.PathsApplied)
	}
}

// ============================================================================
// Scenario 2 — Watchdog detects drift and triggers Reload
// ============================================================================

// TestSmoke_Watchdog_DetectsDriftAndReloads builds a watchdog with a real
// state.Store (one camera) and a fake RuntimeClient that always reports zero
// publishing cameras.  After 2 drift cycles the watchdog should call Reload
// and emit a Warn log containing the camera ID.
func TestSmoke_Watchdog_DetectsDriftAndReloads(t *testing.T) {
	t.Parallel()

	const camID = "drift-cam-1"

	st := openTestStore(t)
	upsertCam(t, st, camID, "rtsp://10.0.2.1/stream")

	// Fake runtime client: never reports any camera as publishing.
	rtClient := &fakeSmokeRuntimeClient{ids: nil}

	var reloadCount atomic.Int64
	var warnCamID atomic.Value // string

	// Logger that captures Warn records.
	handler := &smokeCaptureHandler{fn: func(r slog.Record) {
		if r.Level == slog.LevelWarn {
			r.Attrs(func(a slog.Attr) bool {
				if a.Key == "camera_id" {
					warnCamID.Store(a.Value.String())
				}
				return true
			})
		}
	}}
	log := slog.New(handler)

	// Use the production New() with a very short interval so drift
	// is detected quickly without needing an injectable tick channel.
	wd := recordinghealth.New(recordinghealth.Config{
		Store:                     st,
		MediaMTX:                  rtClient,
		Reload:                    func() { reloadCount.Add(1) },
		Interval:                  10 * time.Millisecond,
		DriftAcknowledgmentCycles: 2,
		Logger:                    log,
	})

	ctx, cancel := context.WithCancel(context.Background())
	go wd.Run(ctx)

	// Wait for at least one Reload.
	waitCondition(t, func() bool {
		return reloadCount.Load() >= 1
	})
	cancel()

	if n := reloadCount.Load(); n < 1 {
		t.Errorf("reloadCount=%d, want >= 1", n)
	}
	// Warn log should have been emitted with camera_id = camID.
	if got, _ := warnCamID.Load().(string); got != camID {
		t.Errorf("warn camera_id=%q, want %q", got, camID)
	}
}

// fakeSmokeRuntimeClient implements recordinghealth.RuntimeClient.
type fakeSmokeRuntimeClient struct {
	mu  sync.Mutex
	ids []string
}

func (f *fakeSmokeRuntimeClient) ListPublishingCameras(_ context.Context) ([]string, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	out := make([]string, len(f.ids))
	copy(out, f.ids)
	return out, nil
}

// ============================================================================
// Scenario 3 — Recovery scan reconciles DB against disk
// ============================================================================

// TestSmoke_RecoveryScan_ReconcilesDBToDisk builds a temp directory with a
// valid fMP4 file that has no DB entry, runs recovery.Run with mock adapters,
// and asserts Scanned > 0 and Inserted > 0.
func TestSmoke_RecoveryScan_ReconcilesDBToDisk(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	camDir := filepath.Join(dir, "nvr", "cam_x", "main")
	if err := os.MkdirAll(camDir, 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}

	segPath := filepath.Join(camDir, "2026-04-25_12-00-00-000.mp4")
	if err := os.WriteFile(segPath, buildSmokeValidFMP4(1), 0o644); err != nil {
		t.Fatalf("write fmp4: %v", err)
	}

	// Empty DB — no known recordings.
	mockDB := &smokeDBQuerier{paths: map[string]int64{}, unindexed: map[string]int64{}}
	// Reconciler that records inserts.
	rec := &smokeReconciler{cameraID: "cam_x", streamID: "main"}

	result, err := recovery.Run(recovery.RunConfig{
		RecordDirs: []string{dir},
		DB:         mockDB,
		Reconciler: rec,
	})
	if err != nil {
		t.Fatalf("recovery.Run: %v", err)
	}

	if result.Scanned < 1 {
		t.Errorf("Scanned=%d, want >= 1", result.Scanned)
	}
	if result.Reconcile.Inserted < 1 {
		t.Errorf("Reconcile.Inserted=%d, want >= 1 (file should be inserted into DB)", result.Reconcile.Inserted)
	}
}

// smokeDBQuerier implements recovery.DBQuerier.
type smokeDBQuerier struct {
	paths     map[string]int64
	unindexed map[string]int64
}

func (q *smokeDBQuerier) GetAllRecordingPaths() (map[string]int64, error) {
	return q.paths, nil
}

func (q *smokeDBQuerier) GetUnindexedRecordingPaths() (map[string]int64, error) {
	return q.unindexed, nil
}

// smokeReconciler implements recovery.Reconciler and records calls.
type smokeReconciler struct {
	cameraID string
	streamID string

	mu       sync.Mutex
	inserted []string
}

func (r *smokeReconciler) InsertRecording(cameraID, _ string, _, _ time.Time, _, _ int64, filePath, _ string) (int64, error) {
	r.mu.Lock()
	r.inserted = append(r.inserted, filePath)
	r.mu.Unlock()
	return int64(len(r.inserted)), nil
}

func (r *smokeReconciler) UpdateRecordingFileSize(_ int64, _ int64) error { return nil }

func (r *smokeReconciler) UpdateRecordingStatus(_ int64, _ string, _ *string, _ string) error {
	return nil
}

func (r *smokeReconciler) MatchCameraFromPath(filePath string) (string, string, bool) {
	if strings.Contains(filePath, r.cameraID) {
		return r.cameraID, r.streamID, true
	}
	return "", "", false
}

// ============================================================================
// Scenario 4 — Supervisor with unreachable mediamtx starts in degraded state
// ============================================================================

// TestSmoke_NoopMediaMTX_BootCompletes verifies that a supervisor pointing at
// a non-existent mediamtx URL starts in degraded (fail-open) mode and does not
// crash capturemanager.
func TestSmoke_NoopMediaMTX_BootCompletes(t *testing.T) {
	t.Parallel()

	st := openTestStore(t)
	upsertCam(t, st, "noop-cam", "rtsp://192.0.2.1/stream")

	ctrl := &mediamtxsupervisor.HTTPController{
		BaseURL:    "http://127.0.0.1:19997", // nothing listening here
		PathPrefix: "cam_",
	}
	sup, err := mediamtxsupervisor.New(mediamtxsupervisor.Config{
		Source:       mediamtxsupervisor.StoreSource{Store: st},
		Controller:   ctrl,
		PollInterval: -1, // no polling
		Logger:       discardSlog(),
	})
	if err != nil {
		t.Fatalf("new supervisor: %v", err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Start should fail-open: it returns an error but does not panic.
	startErr := sup.Start(ctx)
	t.Cleanup(sup.Close)

	// Fail-open: either an error or success is acceptable; what matters is
	// that capMgr and the watchdog can still be constructed and exercised
	// without crashing.
	if startErr != nil {
		t.Logf("supervisor Start returned (expected) error: %v", startErr)
	}

	// Stats should record the error.
	stats := sup.Stats()
	if startErr != nil && stats.LastError == nil {
		t.Errorf("LastError should be set when Start fails")
	}

	// capturemanager must not crash on a reload-triggering EnsureRunning even
	// when the supervisor's last attempt failed.
	capMgr := capturemanager.New(capturemanager.Config{
		Reload: sup.Reload,
		Logger: discardSlog(),
	})
	if err := capMgr.EnsureRunning(recordercontrol.Camera{
		ID:            "noop-cam",
		ConfigVersion: 1,
	}); err != nil {
		t.Fatalf("EnsureRunning on degraded supervisor: %v", err)
	}

	// Watchdog construction must also succeed and Run must not crash on the
	// first failed mediamtx poll.
	rtClient := recordinghealth.NewHTTPRuntimeClient(
		"http://127.0.0.1:19997", // unreachable
		"cam_",
		&http.Client{Timeout: 50 * time.Millisecond},
	)
	wd := recordinghealth.New(recordinghealth.Config{
		Store:                     st,
		MediaMTX:                  rtClient,
		Reload:                    sup.Reload,
		Interval:                  10 * time.Millisecond,
		DriftAcknowledgmentCycles: 100, // prevent spurious reloads
		Logger:                    discardSlog(),
	})

	wdCtx, wdCancel := context.WithTimeout(context.Background(), 80*time.Millisecond)
	defer wdCancel()

	done := make(chan struct{})
	go func() {
		defer close(done)
		wd.Run(wdCtx)
	}()
	select {
	case <-done:
		// Run exited cleanly after context cancel — pass.
	case <-time.After(300 * time.Millisecond):
		t.Error("watchdog.Run did not exit within 300ms after context timeout")
	}
}

// ============================================================================
// Scenario 5 — DiskMonitor retention (skipped — already covered)
// ============================================================================

// TestSmoke_DiskMonitor_TriggersRetention is SKIPPED because the identical
// scenario (95% disk + expired recordings → DeleteRecording called) is already
// thoroughly exercised by internal/recorder/diskmonitor/monitor_test.go
// (TestRun_AboveThreshold_DeletesOldest and TestRun_HysteresisStopsRetention).
// Duplicating the scenario here would add test time without additional seam coverage.
func TestSmoke_DiskMonitor_TriggersRetention(t *testing.T) {
	t.Skip("already covered by internal/recorder/diskmonitor/monitor_test.go")
}

// ============================================================================
// Verify that the watchdog exported fields allow direct construction (used in
// TestSmoke_Watchdog_DetectsDriftAndReloads above via package-level accessor).
// ============================================================================

// Ensure the recordinghealth.Watchdog type's exported fields are accessible.
// This blank assignment keeps the import alive even if the struct is unused.
var _ = recordinghealth.Config{}

// validateSmokeJSON is used to sanity-check fake-server request bodies inline.
func validateSmokeJSON(t *testing.T, data []byte, field, wantSubstr string) {
	t.Helper()
	var m map[string]interface{}
	if err := json.Unmarshal(data, &m); err != nil {
		t.Fatalf("invalid JSON body: %v", err)
	}
	v, ok := m[field]
	if !ok {
		t.Errorf("field %q missing from body %s", field, data)
		return
	}
	got := fmt.Sprintf("%v", v)
	if !strings.Contains(got, wantSubstr) {
		t.Errorf("field %q = %q; want substring %q", field, got, wantSubstr)
	}
}

// TestSmoke_CameraAssignment_BodyContainsSource is a tighter variant of
// Scenario 1 that also validates the JSON body fields.
func TestSmoke_CameraAssignment_BodyContainsSource(t *testing.T) {
	t.Parallel()

	const camID = "body-cam-1"
	const rtspURL = "rtsp://10.0.3.1/stream"

	st := openTestStore(t)
	upsertCam(t, st, camID, rtspURL)

	fake := newFakeMTXServer(t)

	ctrl := &mediamtxsupervisor.HTTPController{
		BaseURL:    fake.srv.URL,
		PathPrefix: "cam_",
	}
	sup, err := mediamtxsupervisor.New(mediamtxsupervisor.Config{
		Source:       mediamtxsupervisor.StoreSource{Store: st},
		Controller:   ctrl,
		PollInterval: -1,
		Render: mediamtxsupervisor.RenderOptions{
			PathPrefix: "cam_",
		},
		Logger: discardSlog(),
	})
	if err != nil {
		t.Fatalf("new supervisor: %v", err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	_ = sup.Start(ctx)
	t.Cleanup(sup.Close)

	waitCondition(t, func() bool {
		for _, ap := range fake.snapAdded() {
			if ap.Name == "cam_"+camID {
				return true
			}
		}
		return false
	})

	var body []byte
	for _, ap := range fake.snapAdded() {
		if ap.Name == "cam_"+camID {
			body = ap.Body
			break
		}
	}
	if body == nil {
		t.Fatal("no ADD request captured")
	}

	validateSmokeJSON(t, body, "source", "10.0.3.1")
	validateSmokeJSON(t, body, "record", "true")
}
