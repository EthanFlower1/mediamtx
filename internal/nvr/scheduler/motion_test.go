package scheduler

import (
	"sync"
	"testing"
	"time"
)

// mockSetRecording records calls to setRecording for test assertions.
type mockSetRecording struct {
	mu    sync.Mutex
	calls []bool
}

func (m *mockSetRecording) fn(path string, record bool) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.calls = append(m.calls, record)
}

func (m *mockSetRecording) callCount() int {
	m.mu.Lock()
	defer m.mu.Unlock()
	return len(m.calls)
}

func (m *mockSetRecording) lastCall() bool {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.calls[len(m.calls)-1]
}

func TestMotion_IdleToRecording(t *testing.T) {
	mock := &mockSetRecording{}
	sm := NewMotionSM("cam1", "mypath", 5*time.Second, mock.fn)

	if sm.State() != "idle" {
		t.Fatalf("expected idle, got %s", sm.State())
	}

	sm.OnMotion(true)

	if sm.State() != "recording" {
		t.Fatalf("expected recording, got %s", sm.State())
	}
	if mock.callCount() != 1 {
		t.Fatalf("expected 1 call, got %d", mock.callCount())
	}
	if !mock.lastCall() {
		t.Fatal("expected setRecording(true)")
	}
}

func TestMotion_RecordingToPostBuffer(t *testing.T) {
	mock := &mockSetRecording{}
	sm := NewMotionSM("cam1", "mypath", 5*time.Second, mock.fn)

	sm.OnMotion(true)
	sm.OnMotion(false)

	if sm.State() != "post_buffer" {
		t.Fatalf("expected post_buffer, got %s", sm.State())
	}
	// No new setRecording call -- still recording
	if mock.callCount() != 1 {
		t.Fatalf("expected 1 call, got %d", mock.callCount())
	}
}

func TestMotion_PostBufferToHysteresis(t *testing.T) {
	mock := &mockSetRecording{}
	sm := NewMotionSM("cam1", "mypath", 5*time.Second, mock.fn)

	sm.OnMotion(true)
	sm.OnMotion(false)

	// Simulate post_buffer timer expiring.
	sm.expirePostBuffer()

	if sm.State() != "hysteresis" {
		t.Fatalf("expected hysteresis, got %s", sm.State())
	}
	// Still no new setRecording call.
	if mock.callCount() != 1 {
		t.Fatalf("expected 1 call, got %d", mock.callCount())
	}
}

func TestMotion_HysteresisToIdle(t *testing.T) {
	mock := &mockSetRecording{}
	sm := NewMotionSM("cam1", "mypath", 5*time.Second, mock.fn)

	sm.OnMotion(true)
	sm.OnMotion(false)
	sm.expirePostBuffer()
	sm.expireHysteresis()

	if sm.State() != "idle" {
		t.Fatalf("expected idle, got %s", sm.State())
	}
	if mock.callCount() != 2 {
		t.Fatalf("expected 2 calls, got %d", mock.callCount())
	}
	if mock.lastCall() {
		t.Fatal("expected setRecording(false)")
	}
}

func TestMotion_PostBufferReMotion(t *testing.T) {
	mock := &mockSetRecording{}
	sm := NewMotionSM("cam1", "mypath", 5*time.Second, mock.fn)

	sm.OnMotion(true)  // idle -> recording
	sm.OnMotion(false) // recording -> post_buffer
	sm.OnMotion(true)  // post_buffer -> recording (re-motion)

	if sm.State() != "recording" {
		t.Fatalf("expected recording, got %s", sm.State())
	}
	// Only the initial setRecording(true) call -- no new write needed.
	if mock.callCount() != 1 {
		t.Fatalf("expected 1 call (no new write on re-motion), got %d", mock.callCount())
	}
}

func TestMotion_HysteresisReMotion(t *testing.T) {
	mock := &mockSetRecording{}
	sm := NewMotionSM("cam1", "mypath", 5*time.Second, mock.fn)

	sm.OnMotion(true)  // idle -> recording
	sm.OnMotion(false) // recording -> post_buffer
	sm.expirePostBuffer()
	sm.OnMotion(true) // hysteresis -> recording (re-motion)

	if sm.State() != "recording" {
		t.Fatalf("expected recording, got %s", sm.State())
	}
	// Only the initial setRecording(true) call -- no new write needed.
	if mock.callCount() != 1 {
		t.Fatalf("expected 1 call (no new write on re-motion), got %d", mock.callCount())
	}
}

func TestMotion_StopFromRecording(t *testing.T) {
	mock := &mockSetRecording{}
	sm := NewMotionSM("cam1", "mypath", 5*time.Second, mock.fn)

	sm.OnMotion(true) // idle -> recording
	sm.Stop()

	if sm.State() != "idle" {
		t.Fatalf("expected idle after Stop, got %s", sm.State())
	}
	if mock.callCount() != 2 {
		t.Fatalf("expected 2 calls, got %d", mock.callCount())
	}
	if mock.lastCall() {
		t.Fatal("expected setRecording(false) on Stop")
	}
}

func TestMotion_StopFromPostBuffer(t *testing.T) {
	mock := &mockSetRecording{}
	sm := NewMotionSM("cam1", "mypath", 5*time.Second, mock.fn)

	sm.OnMotion(true)
	sm.OnMotion(false) // -> post_buffer
	sm.Stop()

	if sm.State() != "idle" {
		t.Fatalf("expected idle after Stop, got %s", sm.State())
	}
	if mock.callCount() != 2 {
		t.Fatalf("expected 2 calls, got %d", mock.callCount())
	}
	if mock.lastCall() {
		t.Fatal("expected setRecording(false) on Stop")
	}
}

func TestMotion_StopFromHysteresis(t *testing.T) {
	mock := &mockSetRecording{}
	sm := NewMotionSM("cam1", "mypath", 5*time.Second, mock.fn)

	sm.OnMotion(true)
	sm.OnMotion(false)
	sm.expirePostBuffer()
	sm.Stop()

	if sm.State() != "idle" {
		t.Fatalf("expected idle after Stop, got %s", sm.State())
	}
	if mock.callCount() != 2 {
		t.Fatalf("expected 2 calls, got %d", mock.callCount())
	}
	if mock.lastCall() {
		t.Fatal("expected setRecording(false) on Stop")
	}
}

func TestMotion_StopFromIdle(t *testing.T) {
	mock := &mockSetRecording{}
	sm := NewMotionSM("cam1", "mypath", 5*time.Second, mock.fn)

	sm.Stop()

	if sm.State() != "idle" {
		t.Fatalf("expected idle after Stop, got %s", sm.State())
	}
	// No calls -- was already idle with recording off.
	if mock.callCount() != 0 {
		t.Fatalf("expected 0 calls, got %d", mock.callCount())
	}
}

func TestMotion_DoubleMotionTrueIsNoop(t *testing.T) {
	mock := &mockSetRecording{}
	sm := NewMotionSM("cam1", "mypath", 5*time.Second, mock.fn)

	sm.OnMotion(true)
	sm.OnMotion(true) // already recording, should be no-op

	if sm.State() != "recording" {
		t.Fatalf("expected recording, got %s", sm.State())
	}
	if mock.callCount() != 1 {
		t.Fatalf("expected 1 call, got %d", mock.callCount())
	}
}

func TestMotion_MotionFalseFromIdleIsNoop(t *testing.T) {
	mock := &mockSetRecording{}
	sm := NewMotionSM("cam1", "mypath", 5*time.Second, mock.fn)

	sm.OnMotion(false) // already idle, should be no-op

	if sm.State() != "idle" {
		t.Fatalf("expected idle, got %s", sm.State())
	}
	if mock.callCount() != 0 {
		t.Fatalf("expected 0 calls, got %d", mock.callCount())
	}
}
