package directoryingest_test

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/bluenviron/mediamtx/internal/cloud/directoryingest"
)

// -----------------------------------------------------------------------
// Fake stores
// -----------------------------------------------------------------------

type fakeCameraStateStore struct {
	mu     sync.Mutex
	states []directoryingest.CameraState
	errOn  string // if non-empty, return error for this camera_id
}

func (f *fakeCameraStateStore) UpsertCameraState(_ context.Context, s directoryingest.CameraState) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.errOn != "" && s.CameraID == f.errOn {
		return errStore("forced upsert error")
	}
	f.states = append(f.states, s)
	return nil
}

func (f *fakeCameraStateStore) count() int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return len(f.states)
}

type fakeSegmentIndexStore struct {
	mu      sync.Mutex
	entries []directoryingest.SegmentIndexEntry
	seen    map[string]bool // segment_id → true (duplicate detection)
}

func newFakeSegmentIndexStore() *fakeSegmentIndexStore {
	return &fakeSegmentIndexStore{seen: make(map[string]bool)}
}

func (f *fakeSegmentIndexStore) UpsertSegmentEntries(_ context.Context, entries []directoryingest.SegmentIndexEntry) (accepted, rejectedDuplicate int64, err error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	for _, e := range entries {
		if f.seen[e.SegmentID] {
			rejectedDuplicate++
			continue
		}
		f.seen[e.SegmentID] = true
		f.entries = append(f.entries, e)
		accepted++
	}
	return accepted, rejectedDuplicate, nil
}

func (f *fakeSegmentIndexStore) count() int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return len(f.entries)
}

type fakeAIEventStore struct {
	mu     sync.Mutex
	events []directoryingest.AIEvent
}

func (f *fakeAIEventStore) InsertAIEvents(_ context.Context, events []directoryingest.AIEvent) (accepted, rejectedUnknown int64, err error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.events = append(f.events, events...)
	return int64(len(events)), 0, nil
}

func (f *fakeAIEventStore) count() int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return len(f.events)
}

type fakeAuthStore struct {
	// recorderID → tenantID
	tenants map[string]string
}

func (f *fakeAuthStore) GetRecorderTenantID(_ context.Context, recorderID string) (string, error) {
	tid, ok := f.tenants[recorderID]
	if !ok {
		return "", errStore("recorder not found: " + recorderID)
	}
	return tid, nil
}

type errStore string

func (e errStore) Error() string { return string(e) }

// -----------------------------------------------------------------------
// Test helpers
// -----------------------------------------------------------------------

func newHandler(t *testing.T,
	cs directoryingest.CameraStateStore,
	si directoryingest.SegmentIndexStore,
	ae directoryingest.AIEventStore,
	auth directoryingest.RecorderAuthStore,
) *directoryingest.Handler {
	t.Helper()
	h, err := directoryingest.NewHandler(directoryingest.Config{
		CameraState:  cs,
		SegmentIndex: si,
		AIEvents:     ae,
		Auth:         auth,
	})
	if err != nil {
		t.Fatalf("NewHandler: %v", err)
	}
	return h
}

func withTenant(req *http.Request, tid string) *http.Request {
	ctx := directoryingest.WithTenantID(req.Context(), tid)
	return req.WithContext(ctx)
}

const (
	testTenant   = "tenant-alpha"
	testRecorder = "recorder-001"
)

var authStore = &fakeAuthStore{
	tenants: map[string]string{
		testRecorder:    testTenant,
		"recorder-002":  "tenant-beta",
	},
}

// ndjsonBody encodes values as newline-delimited JSON into a reader.
func ndjsonBody(t *testing.T, vals ...any) io.Reader {
	t.Helper()
	var buf bytes.Buffer
	for _, v := range vals {
		b, err := json.Marshal(v)
		if err != nil {
			t.Fatalf("marshal: %v", err)
		}
		buf.Write(b)
		buf.WriteByte('\n')
	}
	return &buf
}

// -----------------------------------------------------------------------
// StreamCameraState tests
// -----------------------------------------------------------------------

func TestStreamCameraState_RoundTrip(t *testing.T) {
	cs := &fakeCameraStateStore{}
	si := newFakeSegmentIndexStore()
	ae := &fakeAIEventStore{}
	h := newHandler(t, cs, si, ae, authStore)

	now := time.Now().UTC()
	batch := map[string]any{
		"recorder_id": testRecorder,
		"updates": []map[string]any{
			{
				"camera_id":   "cam-1",
				"state":       "online",
				"observed_at": now.Format(time.RFC3339),
				"current_bitrate_kbps": 3000,
				"current_framerate":    25,
			},
			{
				"camera_id":   "cam-2",
				"state":       "degraded",
				"observed_at": now.Format(time.RFC3339),
				"error_message": "packet loss",
			},
		},
	}

	body := ndjsonBody(t, batch)
	req := httptest.NewRequest(http.MethodPost, directoryingest.StreamCameraStatePath, body)
	req = withTenant(req, testTenant)
	rec := httptest.NewRecorder()

	h.StreamCameraState().ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200; body = %s", rec.Code, rec.Body.String())
	}

	var resp struct {
		Accepted        int64 `json:"accepted"`
		RejectedUnknown int64 `json:"rejected_unknown"`
	}
	if err := json.NewDecoder(rec.Body).Decode(&resp); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if resp.Accepted != 2 {
		t.Errorf("accepted = %d, want 2", resp.Accepted)
	}
	if cs.count() != 2 {
		t.Errorf("store count = %d, want 2", cs.count())
	}
}

func TestStreamCameraState_MultiBatch(t *testing.T) {
	cs := &fakeCameraStateStore{}
	si := newFakeSegmentIndexStore()
	ae := &fakeAIEventStore{}
	h := newHandler(t, cs, si, ae, authStore)

	now := time.Now().UTC()
	batch1 := map[string]any{
		"recorder_id": testRecorder,
		"updates": []map[string]any{
			{"camera_id": "cam-1", "state": "online", "observed_at": now.Format(time.RFC3339)},
		},
	}
	batch2 := map[string]any{
		"recorder_id": testRecorder,
		"updates": []map[string]any{
			{"camera_id": "cam-2", "state": "online", "observed_at": now.Format(time.RFC3339)},
			{"camera_id": "cam-3", "state": "offline", "observed_at": now.Format(time.RFC3339)},
		},
	}

	body := ndjsonBody(t, batch1, batch2)
	req := httptest.NewRequest(http.MethodPost, directoryingest.StreamCameraStatePath, body)
	req = withTenant(req, testTenant)
	rec := httptest.NewRecorder()

	h.StreamCameraState().ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d", rec.Code)
	}
	if cs.count() != 3 {
		t.Errorf("store count = %d, want 3", cs.count())
	}
}

func TestStreamCameraState_MissingTenantCtx(t *testing.T) {
	cs := &fakeCameraStateStore{}
	si := newFakeSegmentIndexStore()
	ae := &fakeAIEventStore{}
	h := newHandler(t, cs, si, ae, authStore)

	req := httptest.NewRequest(http.MethodPost, directoryingest.StreamCameraStatePath,
		strings.NewReader(`{"recorder_id":"recorder-001","updates":[]}`+"\n"))
	// No tenant in context.
	rec := httptest.NewRecorder()
	h.StreamCameraState().ServeHTTP(rec, req)

	if rec.Code != http.StatusUnauthorized {
		t.Errorf("status = %d, want 401", rec.Code)
	}
}

func TestStreamCameraState_CrossTenant(t *testing.T) {
	cs := &fakeCameraStateStore{}
	si := newFakeSegmentIndexStore()
	ae := &fakeAIEventStore{}
	h := newHandler(t, cs, si, ae, authStore)

	now := time.Now().UTC()
	batch := map[string]any{
		// recorder-002 belongs to tenant-beta, but we claim tenant-alpha.
		"recorder_id": "recorder-002",
		"updates": []map[string]any{
			{"camera_id": "cam-x", "state": "online", "observed_at": now.Format(time.RFC3339)},
		},
	}
	body := ndjsonBody(t, batch)
	req := httptest.NewRequest(http.MethodPost, directoryingest.StreamCameraStatePath, body)
	req = withTenant(req, testTenant) // tenant-alpha claiming recorder-002 (owned by tenant-beta)
	rec := httptest.NewRecorder()

	h.StreamCameraState().ServeHTTP(rec, req)

	if rec.Code != http.StatusUnauthorized {
		t.Errorf("status = %d, want 401 (cross-tenant rejection)", rec.Code)
	}
	if cs.count() != 0 {
		t.Errorf("store count = %d, want 0 (nothing persisted)", cs.count())
	}
}

// -----------------------------------------------------------------------
// PublishSegmentIndex tests
// -----------------------------------------------------------------------

func TestPublishSegmentIndex_RoundTrip(t *testing.T) {
	cs := &fakeCameraStateStore{}
	si := newFakeSegmentIndexStore()
	ae := &fakeAIEventStore{}
	h := newHandler(t, cs, si, ae, authStore)

	now := time.Now().UTC()
	batch := map[string]any{
		"recorder_id": testRecorder,
		"entries": []map[string]any{
			{
				"camera_id":  "cam-1",
				"segment_id": "seg-abc",
				"start_time": now.Add(-30 * time.Second).Format(time.RFC3339),
				"end_time":   now.Format(time.RFC3339),
				"bytes":      1024000,
				"codec":      "h264",
				"sequence":   42,
			},
			{
				"camera_id":  "cam-1",
				"segment_id": "seg-def",
				"start_time": now.Format(time.RFC3339),
				"end_time":   now.Add(30 * time.Second).Format(time.RFC3339),
				"bytes":      2048000,
				"codec":      "h264",
				"sequence":   43,
			},
		},
	}

	body := ndjsonBody(t, batch)
	req := httptest.NewRequest(http.MethodPost, directoryingest.PublishSegmentIndexPath, body)
	req = withTenant(req, testTenant)
	rec := httptest.NewRecorder()

	h.PublishSegmentIndex().ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d; body = %s", rec.Code, rec.Body.String())
	}

	var resp struct {
		Accepted          int64 `json:"accepted"`
		RejectedDuplicate int64 `json:"rejected_duplicate"`
	}
	if err := json.NewDecoder(rec.Body).Decode(&resp); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if resp.Accepted != 2 {
		t.Errorf("accepted = %d, want 2", resp.Accepted)
	}
	if si.count() != 2 {
		t.Errorf("store count = %d, want 2", si.count())
	}
}

func TestPublishSegmentIndex_DuplicateIdempotent(t *testing.T) {
	cs := &fakeCameraStateStore{}
	si := newFakeSegmentIndexStore()
	ae := &fakeAIEventStore{}
	h := newHandler(t, cs, si, ae, authStore)

	now := time.Now().UTC()
	makeBatch := func() map[string]any {
		return map[string]any{
			"recorder_id": testRecorder,
			"entries": []map[string]any{
				{
					"camera_id":  "cam-1",
					"segment_id": "seg-dup",
					"start_time": now.Format(time.RFC3339),
					"end_time":   now.Add(30 * time.Second).Format(time.RFC3339),
					"bytes":      1024,
				},
			},
		}
	}

	for i := 0; i < 3; i++ {
		body := ndjsonBody(t, makeBatch())
		req := httptest.NewRequest(http.MethodPost, directoryingest.PublishSegmentIndexPath, body)
		req = withTenant(req, testTenant)
		rec := httptest.NewRecorder()
		h.PublishSegmentIndex().ServeHTTP(rec, req)
		if rec.Code != http.StatusOK {
			t.Fatalf("iteration %d: status = %d", i, rec.Code)
		}
	}

	// The store should contain exactly 1 unique segment.
	if si.count() != 1 {
		t.Errorf("store count = %d, want 1 (idempotent dedup)", si.count())
	}
}

// -----------------------------------------------------------------------
// PublishAIEvents tests
// -----------------------------------------------------------------------

func TestPublishAIEvents_RoundTrip(t *testing.T) {
	cs := &fakeCameraStateStore{}
	si := newFakeSegmentIndexStore()
	ae := &fakeAIEventStore{}
	h := newHandler(t, cs, si, ae, authStore)

	now := time.Now().UTC()
	batch := map[string]any{
		"recorder_id": testRecorder,
		"events": []map[string]any{
			{
				"event_id":   "evt-001",
				"camera_id":  "cam-1",
				"kind":       "AI_EVENT_KIND_PERSON",
				"observed_at": now.Format(time.RFC3339),
				"confidence": 0.95,
				"bbox":       map[string]float32{"x": 0.1, "y": 0.2, "width": 0.3, "height": 0.4},
				"track_id":   "track-99",
				"attributes": map[string]string{"age_group": "adult"},
			},
		},
	}

	body := ndjsonBody(t, batch)
	req := httptest.NewRequest(http.MethodPost, directoryingest.PublishAIEventsPath, body)
	req = withTenant(req, testTenant)
	rec := httptest.NewRecorder()

	h.PublishAIEvents().ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d; body = %s", rec.Code, rec.Body.String())
	}

	var resp struct {
		Accepted int64 `json:"accepted"`
	}
	if err := json.NewDecoder(rec.Body).Decode(&resp); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if resp.Accepted != 1 {
		t.Errorf("accepted = %d, want 1", resp.Accepted)
	}
	if ae.count() != 1 {
		t.Errorf("store count = %d, want 1", ae.count())
	}
}

func TestPublishAIEvents_BehavioralEvent(t *testing.T) {
	// Simulates a behavioral analysis event from KAI-284 arriving via
	// the PublishAIEvents stream.
	cs := &fakeCameraStateStore{}
	si := newFakeSegmentIndexStore()
	ae := &fakeAIEventStore{}
	h := newHandler(t, cs, si, ae, authStore)

	now := time.Now().UTC()
	batch := map[string]any{
		"recorder_id": testRecorder,
		"events": []map[string]any{
			{
				"event_id":   "behavioral-001",
				"camera_id":  "cam-entrance",
				"kind":       "AI_EVENT_KIND_LOITERING",
				"observed_at": now.Format(time.RFC3339),
				"confidence": 0.88,
				"bbox":       map[string]float32{"x": 0.0, "y": 0.0, "width": 1.0, "height": 1.0},
				"track_id":   "loiter-track-1",
				"attributes": map[string]string{
					"duration_seconds": "120",
					"zone_id":          "zone-entrance-north",
				},
			},
		},
	}

	body := ndjsonBody(t, batch)
	req := httptest.NewRequest(http.MethodPost, directoryingest.PublishAIEventsPath, body)
	req = withTenant(req, testTenant)
	rec := httptest.NewRecorder()

	h.PublishAIEvents().ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d", rec.Code)
	}
	if ae.count() != 1 {
		t.Errorf("behavioral event not stored: count = %d, want 1", ae.count())
	}
}

func TestPublishAIEvents_CrossTenant(t *testing.T) {
	cs := &fakeCameraStateStore{}
	si := newFakeSegmentIndexStore()
	ae := &fakeAIEventStore{}
	h := newHandler(t, cs, si, ae, authStore)

	now := time.Now().UTC()
	batch := map[string]any{
		"recorder_id": "recorder-002", // belongs to tenant-beta
		"events": []map[string]any{
			{
				"event_id":    "evt-x",
				"camera_id":   "cam-x",
				"kind":        "AI_EVENT_KIND_PERSON",
				"observed_at": now.Format(time.RFC3339),
			},
		},
	}

	body := ndjsonBody(t, batch)
	req := httptest.NewRequest(http.MethodPost, directoryingest.PublishAIEventsPath, body)
	req = withTenant(req, testTenant) // tenant-alpha claiming tenant-beta's recorder
	rec := httptest.NewRecorder()

	h.PublishAIEvents().ServeHTTP(rec, req)

	if rec.Code != http.StatusUnauthorized {
		t.Errorf("status = %d, want 401", rec.Code)
	}
	if ae.count() != 0 {
		t.Errorf("store count = %d, want 0", ae.count())
	}
}

// -----------------------------------------------------------------------
// Config validation
// -----------------------------------------------------------------------

func TestNewHandler_MissingRequiredField(t *testing.T) {
	cs := &fakeCameraStateStore{}
	si := newFakeSegmentIndexStore()
	ae := &fakeAIEventStore{}

	cases := []struct {
		name string
		cfg  directoryingest.Config
	}{
		{"missing CameraState", directoryingest.Config{SegmentIndex: si, AIEvents: ae, Auth: authStore}},
		{"missing SegmentIndex", directoryingest.Config{CameraState: cs, AIEvents: ae, Auth: authStore}},
		{"missing AIEvents", directoryingest.Config{CameraState: cs, SegmentIndex: si, Auth: authStore}},
		{"missing Auth", directoryingest.Config{CameraState: cs, SegmentIndex: si, AIEvents: ae}},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			_, err := directoryingest.NewHandler(c.cfg)
			if err == nil {
				t.Fatal("expected error, got nil")
			}
		})
	}
}
