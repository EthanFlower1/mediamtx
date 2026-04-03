package connmgr

import (
	"testing"
	"time"
)

func TestNextBackoff(t *testing.T) {
	tests := []struct {
		current  time.Duration
		expected time.Duration
	}{
		{2 * time.Second, 4 * time.Second},
		{4 * time.Second, 8 * time.Second},
		{8 * time.Second, 16 * time.Second},
		{3 * time.Minute, 5 * time.Minute}, // capped at MaxBackoff
		{5 * time.Minute, 5 * time.Minute}, // stays at cap
	}

	for _, tt := range tests {
		got := nextBackoff(tt.current)
		if got != tt.expected {
			t.Errorf("nextBackoff(%v) = %v, want %v", tt.current, got, tt.expected)
		}
	}
}

func TestNextBackoffCap(t *testing.T) {
	b := InitialBackoff
	for i := 0; i < 20; i++ {
		b = nextBackoff(b)
		if b > MaxBackoff {
			t.Fatalf("backoff %v exceeded MaxBackoff %v at iteration %d", b, MaxBackoff, i)
		}
	}
	if b != MaxBackoff {
		t.Errorf("after 20 iterations, backoff = %v, want %v", b, MaxBackoff)
	}
}

func TestManagerStateTracking(t *testing.T) {
	m := &Manager{
		cameras: make(map[string]*CameraState),
		queues:  make(map[string][]QueuedCommand),
	}

	// No state for unknown camera.
	if s := m.GetState("unknown"); s != nil {
		t.Fatal("expected nil for unknown camera")
	}

	// Add a camera state manually.
	m.cameras["cam1"] = &CameraState{
		CameraID: "cam1",
		State:    StateDisconnected,
		Backoff:  InitialBackoff,
	}

	s := m.GetState("cam1")
	if s == nil {
		t.Fatal("expected state for cam1")
	}
	if s.State != StateDisconnected {
		t.Errorf("state = %s, want %s", s.State, StateDisconnected)
	}

	// GetAllStates returns copies.
	all := m.GetAllStates()
	if len(all) != 1 {
		t.Errorf("GetAllStates() returned %d states, want 1", len(all))
	}
}

func TestEnqueueCommand(t *testing.T) {
	m := &Manager{
		cameras: make(map[string]*CameraState),
		queues:  make(map[string][]QueuedCommand),
	}

	// Unknown camera returns false.
	cmd := QueuedCommand{ID: "cmd1", CameraID: "unknown", Type: "ptz"}
	if m.EnqueueCommand(cmd) {
		t.Fatal("expected false for unknown camera")
	}

	// Connected camera returns false (execute directly).
	m.cameras["cam1"] = &CameraState{CameraID: "cam1", State: StateConnected}
	cmd.CameraID = "cam1"
	if m.EnqueueCommand(cmd) {
		t.Fatal("expected false for connected camera")
	}

	// Disconnected camera queues the command.
	m.cameras["cam1"].State = StateDisconnected
	if !m.EnqueueCommand(cmd) {
		t.Fatal("expected true for disconnected camera")
	}

	queue := m.GetQueue("cam1")
	if len(queue) != 1 {
		t.Fatalf("queue length = %d, want 1", len(queue))
	}
	if queue[0].ID != "cmd1" {
		t.Errorf("queued command ID = %s, want cmd1", queue[0].ID)
	}
}

func TestRemoveCamera(t *testing.T) {
	m := &Manager{
		cameras: make(map[string]*CameraState),
		queues:  make(map[string][]QueuedCommand),
	}

	m.cameras["cam1"] = &CameraState{CameraID: "cam1", State: StateDisconnected}
	m.queues["cam1"] = []QueuedCommand{{ID: "cmd1"}}

	m.RemoveCamera("cam1")

	if s := m.GetState("cam1"); s != nil {
		t.Fatal("expected nil after removal")
	}
	if q := m.GetQueue("cam1"); len(q) != 0 {
		t.Fatal("expected empty queue after removal")
	}
}

func TestNotifyOnlineResetsBackoff(t *testing.T) {
	m := &Manager{
		cameras: make(map[string]*CameraState),
		queues:  make(map[string][]QueuedCommand),
	}

	m.cameras["cam1"] = &CameraState{
		CameraID: "cam1",
		State:    StateError,
		Backoff:  2 * time.Minute,
	}

	m.NotifyOnline("cam1")

	s := m.GetState("cam1")
	if s.Backoff != InitialBackoff {
		t.Errorf("backoff = %v, want %v after NotifyOnline", s.Backoff, InitialBackoff)
	}
}

func TestGetClientNilWhenDisconnected(t *testing.T) {
	m := &Manager{
		cameras: make(map[string]*CameraState),
		queues:  make(map[string][]QueuedCommand),
	}

	m.cameras["cam1"] = &CameraState{CameraID: "cam1", State: StateDisconnected}

	if c := m.GetClient("cam1"); c != nil {
		t.Fatal("expected nil client for disconnected camera")
	}
}
