package connect

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"log/slog"

	"github.com/bluenviron/mediamtx/internal/directory/cloudconnector"
	"github.com/gorilla/websocket"
)

func TestBrokerAcceptsDirectoryConnection(t *testing.T) {
	reg := NewRegistry()
	logger := slog.Default()

	broker := NewBroker(BrokerConfig{
		Registry: reg,
		Authenticate: func(token string) (string, bool) {
			if token == "valid-token" {
				return "tenant-1", true
			}
			return "", false
		},
		RelayURL: "wss://relay.raikada.com",
		Logger:   logger,
	})

	srv := httptest.NewServer(broker)
	defer srv.Close()

	wsURL := "ws" + strings.TrimPrefix(srv.URL, "http")
	header := http.Header{}
	header.Set("Authorization", "Bearer valid-token")

	conn, _, err := websocket.DefaultDialer.Dial(wsURL, header)
	if err != nil {
		t.Fatalf("dial failed: %v", err)
	}
	defer conn.Close()

	// Send register message.
	regMsg := cloudconnector.Envelope{
		Type: cloudconnector.MsgTypeRegister,
		Register: &cloudconnector.RegisterPayload{
			SiteID:    "site-abc",
			SiteAlias: "warehouse",
			Version:   "1.0.0",
			PublicIP:  "203.0.113.1",
			LANCIDRs:  []string{"192.168.1.0/24"},
			Capabilities: cloudconnector.Capabilities{
				Streams:  true,
				Playback: true,
			},
		},
	}

	if err := conn.WriteJSON(regMsg); err != nil {
		t.Fatalf("write register failed: %v", err)
	}

	// Read ack.
	var ack cloudconnector.Envelope
	conn.SetReadDeadline(time.Now().Add(5 * time.Second))
	if err := conn.ReadJSON(&ack); err != nil {
		t.Fatalf("read ack failed: %v", err)
	}

	if ack.Type != cloudconnector.MsgTypeRegistered {
		t.Errorf("ack type = %q, want %q", ack.Type, cloudconnector.MsgTypeRegistered)
	}
	if ack.Registered == nil {
		t.Fatal("ack.Registered is nil")
	}
	if !ack.Registered.OK {
		t.Errorf("ack.Registered.OK = false, want true")
	}
	if ack.Registered.RelayURL != "wss://relay.raikada.com" {
		t.Errorf("ack.Registered.RelayURL = %q, want %q", ack.Registered.RelayURL, "wss://relay.raikada.com")
	}

	// Verify registry entry.
	session, ok := reg.LookupByAlias("tenant-1", "warehouse")
	if !ok {
		t.Fatal("expected session in registry after register")
	}
	if session.SiteID != "site-abc" {
		t.Errorf("session.SiteID = %q, want %q", session.SiteID, "site-abc")
	}
	if session.PublicIP != "203.0.113.1" {
		t.Errorf("session.PublicIP = %q, want %q", session.PublicIP, "203.0.113.1")
	}
	if session.Status != StatusOnline {
		t.Errorf("session.Status = %q, want %q", session.Status, StatusOnline)
	}

	// Send heartbeat and verify it updates registry.
	hbMsg := cloudconnector.Envelope{
		Type: cloudconnector.MsgTypeHeartbeat,
		Heartbeat: &cloudconnector.HeartbeatPayload{
			SiteID:        "site-abc",
			Timestamp:     time.Now(),
			CameraCount:   12,
			RecorderCount: 2,
			DiskUsedPct:   55.5,
		},
	}
	if err := conn.WriteJSON(hbMsg); err != nil {
		t.Fatalf("write heartbeat failed: %v", err)
	}

	// Give the broker a moment to process the heartbeat.
	time.Sleep(100 * time.Millisecond)

	session2, _ := reg.LookupByAlias("tenant-1", "warehouse")
	if session2.CameraCount != 12 {
		t.Errorf("CameraCount = %d, want 12", session2.CameraCount)
	}
	if session2.RecorderCount != 2 {
		t.Errorf("RecorderCount = %d, want 2", session2.RecorderCount)
	}

	// Close connection and verify removal from registry.
	conn.Close()
	time.Sleep(200 * time.Millisecond)

	_, ok = reg.LookupByAlias("tenant-1", "warehouse")
	if ok {
		t.Error("expected session removed from registry after disconnect")
	}
}

func TestBrokerRejectsInvalidToken(t *testing.T) {
	reg := NewRegistry()
	logger := slog.Default()

	broker := NewBroker(BrokerConfig{
		Registry: reg,
		Authenticate: func(token string) (string, bool) {
			return "", false
		},
		RelayURL: "wss://relay.raikada.com",
		Logger:   logger,
	})

	srv := httptest.NewServer(broker)
	defer srv.Close()

	wsURL := "ws" + strings.TrimPrefix(srv.URL, "http")
	header := http.Header{}
	header.Set("Authorization", "Bearer bad-token")

	_, resp, err := websocket.DefaultDialer.Dial(wsURL, header)
	if err == nil {
		t.Fatal("expected dial to fail with invalid token")
	}
	if resp != nil && resp.StatusCode != http.StatusUnauthorized {
		t.Errorf("status = %d, want %d", resp.StatusCode, http.StatusUnauthorized)
	}

	// Also test missing Authorization header.
	_, resp2, err2 := websocket.DefaultDialer.Dial(wsURL, nil)
	if err2 == nil {
		t.Fatal("expected dial to fail with missing auth")
	}
	if resp2 != nil && resp2.StatusCode != http.StatusUnauthorized {
		t.Errorf("status = %d, want %d", resp2.StatusCode, http.StatusUnauthorized)
	}

	// Registry should be empty since nothing was registered.
	list := reg.ListByTenant("any")
	if len(list) != 0 {
		t.Errorf("registry should be empty, has %d entries", len(list))
	}
}

// TestBrokerHandlesEventMessage verifies that event messages are accepted
// without error (they are logged but otherwise ignored for now).
func TestBrokerHandlesEventMessage(t *testing.T) {
	reg := NewRegistry()
	logger := slog.Default()

	broker := NewBroker(BrokerConfig{
		Registry: reg,
		Authenticate: func(token string) (string, bool) {
			return "tenant-1", true
		},
		RelayURL: "wss://relay.raikada.com",
		Logger:   logger,
	})

	srv := httptest.NewServer(broker)
	defer srv.Close()

	wsURL := "ws" + strings.TrimPrefix(srv.URL, "http")
	header := http.Header{}
	header.Set("Authorization", "Bearer ok")

	conn, _, err := websocket.DefaultDialer.Dial(wsURL, header)
	if err != nil {
		t.Fatalf("dial failed: %v", err)
	}
	defer conn.Close()

	// Register first.
	conn.WriteJSON(cloudconnector.Envelope{
		Type: cloudconnector.MsgTypeRegister,
		Register: &cloudconnector.RegisterPayload{
			SiteID:    "site-evt",
			SiteAlias: "office",
			Version:   "1.0.0",
		},
	})

	// Read ack.
	var ack cloudconnector.Envelope
	conn.SetReadDeadline(time.Now().Add(5 * time.Second))
	conn.ReadJSON(&ack)

	// Send event.
	evtMsg := cloudconnector.Envelope{
		Type: cloudconnector.MsgTypeEvent,
		Event: &cloudconnector.EventPayload{
			Kind:      "motion_detected",
			CameraID:  "cam-1",
			Timestamp: time.Now(),
			Data:      json.RawMessage(`{"zone":"entrance"}`),
		},
	}
	if err := conn.WriteJSON(evtMsg); err != nil {
		t.Fatalf("write event failed: %v", err)
	}

	// If the broker doesn't crash, the test passes. Give it a moment to process.
	time.Sleep(100 * time.Millisecond)
}
