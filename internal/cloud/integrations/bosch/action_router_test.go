package bosch

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
	"time"
)

// mockDispatcher records camera action calls.
type mockDispatcher struct {
	mu        sync.Mutex
	recordings []recordingCall
	ptzCalls   []ptzCall
	snapshots  []snapshotCall
	err       error // if set, all calls return this error
}

type recordingCall struct {
	TenantID  string
	CameraID  string
	Duration  int
}

type ptzCall struct {
	TenantID   string
	CameraID   string
	PresetName string
}

type snapshotCall struct {
	TenantID string
	CameraID string
}

func (m *mockDispatcher) StartRecording(_ context.Context, tenantID, cameraID string, durationSec int) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.recordings = append(m.recordings, recordingCall{tenantID, cameraID, durationSec})
	return m.err
}

func (m *mockDispatcher) RecallPTZPreset(_ context.Context, tenantID, cameraID, presetName string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.ptzCalls = append(m.ptzCalls, ptzCall{tenantID, cameraID, presetName})
	return m.err
}

func (m *mockDispatcher) TakeSnapshot(_ context.Context, tenantID, cameraID string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.snapshots = append(m.snapshots, snapshotCall{tenantID, cameraID})
	return m.err
}

func TestActionRouter_DispatchesRecordAction(t *testing.T) {
	dispatcher := &mockDispatcher{}
	router := NewActionRouter(dispatcher)

	router.SetMappings("panel-1", []ZoneCameraMapping{
		{
			ID:         "m1",
			TenantID:   "tenant-1",
			PanelID:    "panel-1",
			ZoneNumber: 5,
			CameraIDs:  []string{"cam-a"},
			Actions: []Action{
				{Type: ActionRecord, Duration: 30},
			},
			Enabled: true,
		},
	})

	event := &AlarmEvent{
		PanelID:    "panel-1",
		TenantID:   "tenant-1",
		EventType:  EventBurglary,
		ZoneNumber: 5,
		Timestamp:  time.Now(),
	}

	router.HandleEvent(event)

	// Actions are dispatched in goroutines; wait briefly.
	time.Sleep(50 * time.Millisecond)

	dispatcher.mu.Lock()
	defer dispatcher.mu.Unlock()

	if len(dispatcher.recordings) != 1 {
		t.Fatalf("recordings: got %d want 1", len(dispatcher.recordings))
	}
	if dispatcher.recordings[0].CameraID != "cam-a" {
		t.Errorf("camera: got %q want %q", dispatcher.recordings[0].CameraID, "cam-a")
	}
	if dispatcher.recordings[0].Duration != 30 {
		t.Errorf("duration: got %d want 30", dispatcher.recordings[0].Duration)
	}
}

func TestActionRouter_DispatchesPTZAction(t *testing.T) {
	dispatcher := &mockDispatcher{}
	router := NewActionRouter(dispatcher)

	router.SetMappings("panel-1", []ZoneCameraMapping{
		{
			ID:         "m1",
			TenantID:   "tenant-1",
			PanelID:    "panel-1",
			ZoneNumber: 3,
			CameraIDs:  []string{"cam-b"},
			Actions: []Action{
				{Type: ActionPTZPreset, PTZPreset: "entrance"},
			},
			Enabled: true,
		},
	})

	event := &AlarmEvent{
		PanelID:    "panel-1",
		TenantID:   "tenant-1",
		EventType:  EventPanic,
		ZoneNumber: 3,
		Timestamp:  time.Now(),
	}

	router.HandleEvent(event)
	time.Sleep(50 * time.Millisecond)

	dispatcher.mu.Lock()
	defer dispatcher.mu.Unlock()

	if len(dispatcher.ptzCalls) != 1 {
		t.Fatalf("ptzCalls: got %d want 1", len(dispatcher.ptzCalls))
	}
	if dispatcher.ptzCalls[0].PresetName != "entrance" {
		t.Errorf("preset: got %q want %q", dispatcher.ptzCalls[0].PresetName, "entrance")
	}
}

func TestActionRouter_DispatchesSnapshotAction(t *testing.T) {
	dispatcher := &mockDispatcher{}
	router := NewActionRouter(dispatcher)

	router.SetMappings("panel-1", []ZoneCameraMapping{
		{
			ID:         "m1",
			TenantID:   "tenant-1",
			PanelID:    "panel-1",
			ZoneNumber: 7,
			CameraIDs:  []string{"cam-c"},
			Actions: []Action{
				{Type: ActionSnapshot},
			},
			Enabled: true,
		},
	})

	event := &AlarmEvent{
		PanelID:    "panel-1",
		TenantID:   "tenant-1",
		EventType:  EventFire,
		ZoneNumber: 7,
		Timestamp:  time.Now(),
	}

	router.HandleEvent(event)
	time.Sleep(50 * time.Millisecond)

	dispatcher.mu.Lock()
	defer dispatcher.mu.Unlock()

	if len(dispatcher.snapshots) != 1 {
		t.Fatalf("snapshots: got %d want 1", len(dispatcher.snapshots))
	}
	if dispatcher.snapshots[0].CameraID != "cam-c" {
		t.Errorf("camera: got %q want %q", dispatcher.snapshots[0].CameraID, "cam-c")
	}
}

func TestActionRouter_WebhookAction(t *testing.T) {
	var received bool
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		received = true
		if r.Header.Get("Content-Type") != "application/json" {
			t.Errorf("content-type: got %q want application/json", r.Header.Get("Content-Type"))
		}
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	dispatcher := &mockDispatcher{}
	router := NewActionRouter(dispatcher)

	router.SetMappings("panel-1", []ZoneCameraMapping{
		{
			ID:         "m1",
			TenantID:   "tenant-1",
			PanelID:    "panel-1",
			ZoneNumber: 2,
			CameraIDs:  []string{"cam-d"},
			Actions: []Action{
				{Type: ActionWebhook, WebhookURL: server.URL},
			},
			Enabled: true,
		},
	})

	event := &AlarmEvent{
		PanelID:    "panel-1",
		TenantID:   "tenant-1",
		EventType:  EventTrouble,
		ZoneNumber: 2,
		Timestamp:  time.Now(),
	}

	router.HandleEvent(event)
	time.Sleep(100 * time.Millisecond)

	if !received {
		t.Error("webhook was not called")
	}
}

func TestActionRouter_SkipsDisabledMapping(t *testing.T) {
	dispatcher := &mockDispatcher{}
	router := NewActionRouter(dispatcher)

	router.SetMappings("panel-1", []ZoneCameraMapping{
		{
			ID:         "m1",
			TenantID:   "tenant-1",
			PanelID:    "panel-1",
			ZoneNumber: 5,
			CameraIDs:  []string{"cam-a"},
			Actions:    []Action{{Type: ActionRecord}},
			Enabled:    false, // disabled
		},
	})

	router.HandleEvent(&AlarmEvent{
		PanelID:    "panel-1",
		ZoneNumber: 5,
		Timestamp:  time.Now(),
	})

	time.Sleep(50 * time.Millisecond)

	dispatcher.mu.Lock()
	defer dispatcher.mu.Unlock()
	if len(dispatcher.recordings) != 0 {
		t.Errorf("should not dispatch for disabled mapping")
	}
}

func TestActionRouter_SkipsMismatchedZone(t *testing.T) {
	dispatcher := &mockDispatcher{}
	router := NewActionRouter(dispatcher)

	router.SetMappings("panel-1", []ZoneCameraMapping{
		{
			ID:         "m1",
			TenantID:   "tenant-1",
			PanelID:    "panel-1",
			ZoneNumber: 5,
			CameraIDs:  []string{"cam-a"},
			Actions:    []Action{{Type: ActionRecord}},
			Enabled:    true,
		},
	})

	// Event for zone 10, mapping is for zone 5.
	router.HandleEvent(&AlarmEvent{
		PanelID:    "panel-1",
		ZoneNumber: 10,
		Timestamp:  time.Now(),
	})

	time.Sleep(50 * time.Millisecond)

	dispatcher.mu.Lock()
	defer dispatcher.mu.Unlock()
	if len(dispatcher.recordings) != 0 {
		t.Errorf("should not dispatch for mismatched zone")
	}
}

func TestActionRouter_MultipleCamerasMultipleActions(t *testing.T) {
	dispatcher := &mockDispatcher{}
	router := NewActionRouter(dispatcher)

	router.SetMappings("panel-1", []ZoneCameraMapping{
		{
			ID:         "m1",
			TenantID:   "tenant-1",
			PanelID:    "panel-1",
			ZoneNumber: 1,
			CameraIDs:  []string{"cam-a", "cam-b"},
			Actions: []Action{
				{Type: ActionRecord, Duration: 60},
				{Type: ActionSnapshot},
			},
			Enabled: true,
		},
	})

	router.HandleEvent(&AlarmEvent{
		PanelID:    "panel-1",
		TenantID:   "tenant-1",
		ZoneNumber: 1,
		Timestamp:  time.Now(),
	})

	time.Sleep(100 * time.Millisecond)

	dispatcher.mu.Lock()
	defer dispatcher.mu.Unlock()

	// 2 cameras * 2 actions = 4 total dispatches
	if len(dispatcher.recordings) != 2 {
		t.Errorf("recordings: got %d want 2", len(dispatcher.recordings))
	}
	if len(dispatcher.snapshots) != 2 {
		t.Errorf("snapshots: got %d want 2", len(dispatcher.snapshots))
	}
}

func TestActionRouter_RemoveMappings(t *testing.T) {
	dispatcher := &mockDispatcher{}
	router := NewActionRouter(dispatcher)

	router.SetMappings("panel-1", []ZoneCameraMapping{
		{
			ID:         "m1",
			TenantID:   "tenant-1",
			PanelID:    "panel-1",
			ZoneNumber: 1,
			CameraIDs:  []string{"cam-a"},
			Actions:    []Action{{Type: ActionRecord}},
			Enabled:    true,
		},
	})

	router.RemoveMappings("panel-1")

	router.HandleEvent(&AlarmEvent{
		PanelID:    "panel-1",
		ZoneNumber: 1,
		Timestamp:  time.Now(),
	})

	time.Sleep(50 * time.Millisecond)

	dispatcher.mu.Lock()
	defer dispatcher.mu.Unlock()
	if len(dispatcher.recordings) != 0 {
		t.Errorf("should not dispatch after removing mappings")
	}
}

func TestActionRouter_FailedActionCountsStats(t *testing.T) {
	dispatcher := &mockDispatcher{err: fmt.Errorf("camera offline")}
	router := NewActionRouter(dispatcher)

	router.SetMappings("panel-1", []ZoneCameraMapping{
		{
			ID:         "m1",
			TenantID:   "tenant-1",
			PanelID:    "panel-1",
			ZoneNumber: 1,
			CameraIDs:  []string{"cam-a"},
			Actions:    []Action{{Type: ActionRecord}},
			Enabled:    true,
		},
	})

	router.HandleEvent(&AlarmEvent{
		PanelID:    "panel-1",
		TenantID:   "tenant-1",
		ZoneNumber: 1,
		Timestamp:  time.Now(),
	})

	time.Sleep(50 * time.Millisecond)

	dispatched, failed := router.Stats()
	if dispatched != 0 {
		t.Errorf("dispatched: got %d want 0", dispatched)
	}
	if failed != 1 {
		t.Errorf("failed: got %d want 1", failed)
	}
}
