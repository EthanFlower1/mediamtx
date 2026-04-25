package backchannel

import (
	"testing"
)

func mockCredFunc(cameraID string) (xaddr, user, pass string, err error) {
	return "http://192.168.1.100:80/onvif/device_service", "admin", "pass", nil
}

func TestManagerNewManager(t *testing.T) {
	m := NewManager(mockCredFunc)
	if m == nil {
		t.Fatal("expected non-nil manager")
	}
	if m.sessions == nil {
		t.Fatal("expected initialized sessions map")
	}
}

func TestManagerSessionState(t *testing.T) {
	m := NewManager(mockCredFunc)
	info, exists := m.GetSessionInfo("cam-1")
	if exists {
		t.Fatalf("expected no session, got %+v", info)
	}
}

func TestSessionStateConstants(t *testing.T) {
	if StateIdle != 0 {
		t.Fatalf("expected 0, got %d", StateIdle)
	}
	if StateConnecting != 1 {
		t.Fatalf("expected 1, got %d", StateConnecting)
	}
	if StateActive != 2 {
		t.Fatalf("expected 2, got %d", StateActive)
	}
	if StateClosing != 3 {
		t.Fatalf("expected 3, got %d", StateClosing)
	}
}

func TestManagerStopSessionNotStarted(t *testing.T) {
	m := NewManager(mockCredFunc)
	err := m.StopSession("cam-nonexistent")
	if err == nil {
		t.Fatal("expected error")
	}
	if err != ErrNoSession {
		t.Fatalf("expected ErrNoSession, got %v", err)
	}
}

func TestManagerCloseAll(t *testing.T) {
	m := NewManager(mockCredFunc)
	m.CloseAll() // should not panic on empty manager
}

func TestManagerSendAudioNoSession(t *testing.T) {
	m := NewManager(mockCredFunc)
	err := m.SendAudio("cam-1", []byte{0x01})
	if err == nil {
		t.Fatal("expected error")
	}
	if err != ErrNoSession {
		t.Fatalf("expected ErrNoSession, got %v", err)
	}
}
