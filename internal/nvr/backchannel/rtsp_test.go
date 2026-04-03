package backchannel

import "testing"

func TestRTSPConnStateTransitions(t *testing.T) {
	conn := &RTSPConn{state: rtspStateDisconnected}
	if conn.State() != rtspStateDisconnected {
		t.Fatalf("expected disconnected, got %d", conn.State())
	}
	conn.state = rtspStateConnected
	if conn.State() != rtspStateConnected {
		t.Fatalf("expected connected, got %d", conn.State())
	}
}

func TestRTSPConnDefaults(t *testing.T) {
	conn := NewRTSPConn("rtsp://192.168.1.100:554/backchannel", "admin", "pass123")
	if conn.uri != "rtsp://192.168.1.100:554/backchannel" {
		t.Fatalf("unexpected URI: %s", conn.uri)
	}
	if conn.State() != rtspStateDisconnected {
		t.Fatalf("expected disconnected, got %d", conn.State())
	}
}

func TestRTSPConnSendWithoutConnect(t *testing.T) {
	conn := NewRTSPConn("rtsp://192.168.1.100:554/backchannel", "admin", "pass123")
	err := conn.SendAudio([]byte{0x01, 0x02})
	if err == nil {
		t.Fatal("expected error sending without connection")
	}
	if err != ErrNotConnected {
		t.Fatalf("expected ErrNotConnected, got %v", err)
	}
}
