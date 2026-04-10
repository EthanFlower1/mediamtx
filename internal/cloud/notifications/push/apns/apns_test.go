package apns_test

import (
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/x509"
	"encoding/json"
	"encoding/pem"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/bluenviron/mediamtx/internal/cloud/notifications"
	"github.com/bluenviron/mediamtx/internal/cloud/notifications/push/apns"
)

// generateTestKey creates a PEM-encoded ECDSA P-256 key for testing.
func generateTestKey(t *testing.T) string {
	t.Helper()
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatalf("generate key: %v", err)
	}
	raw, err := x509.MarshalPKCS8PrivateKey(key)
	if err != nil {
		t.Fatalf("marshal key: %v", err)
	}
	block := &pem.Block{Type: "PRIVATE KEY", Bytes: raw}
	return string(pem.EncodeToMemory(block))
}

func TestSend_Success(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !strings.HasPrefix(r.URL.Path, "/3/device/") {
			t.Errorf("unexpected path: %s", r.URL.Path)
		}
		if r.Header.Get("apns-topic") != "com.kaivue.app" {
			t.Errorf("unexpected topic: %s", r.Header.Get("apns-topic"))
		}
		auth := r.Header.Get("Authorization")
		if !strings.HasPrefix(auth, "bearer ") {
			t.Errorf("unexpected auth: %s", auth)
		}
		w.Header().Set("apns-id", "uuid-apns-123")
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	ch, err := apns.New(apns.Config{
		KeyID:      "KEY123",
		TeamID:     "TEAM456",
		BundleID:   "com.kaivue.app",
		PrivateKey: generateTestKey(t),
		Endpoint:   srv.URL,
	})
	if err != nil {
		t.Fatalf("new: %v", err)
	}

	result, err := ch.Send(context.Background(), notifications.Message{
		MessageID: "msg-1",
		Target:    "apns-device-token-abc",
		Title:     "Camera Alert",
		Body:      "Motion detected",
		Priority:  "high",
	})
	if err != nil {
		t.Fatalf("send: %v", err)
	}
	if result.State != notifications.StateDelivered {
		t.Errorf("expected delivered, got %s", result.State)
	}
	if result.PlatformID != "uuid-apns-123" {
		t.Errorf("unexpected platform id: %s", result.PlatformID)
	}
}

func TestSend_BadDeviceToken(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(map[string]string{
			"reason": "BadDeviceToken",
		})
	}))
	defer srv.Close()

	ch, _ := apns.New(apns.Config{
		KeyID:      "KEY123",
		TeamID:     "TEAM456",
		BundleID:   "com.kaivue.app",
		PrivateKey: generateTestKey(t),
		Endpoint:   srv.URL,
	})

	result, err := ch.Send(context.Background(), notifications.Message{
		Target: "bad-token",
	})
	if err != nil {
		t.Fatalf("send: %v", err)
	}
	if result.State != notifications.StateUnreachable {
		t.Errorf("expected unreachable, got %s", result.State)
	}
	if !result.ShouldRemoveToken {
		t.Error("expected ShouldRemoveToken=true")
	}
}

func TestSend_Unregistered(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusGone)
		json.NewEncoder(w).Encode(map[string]string{
			"reason": "Unregistered",
		})
	}))
	defer srv.Close()

	ch, _ := apns.New(apns.Config{
		KeyID:      "KEY123",
		TeamID:     "TEAM456",
		BundleID:   "com.kaivue.app",
		PrivateKey: generateTestKey(t),
		Endpoint:   srv.URL,
	})

	result, _ := ch.Send(context.Background(), notifications.Message{
		Target: "expired-token",
	})
	if result.State != notifications.StateUnreachable {
		t.Errorf("expected unreachable, got %s", result.State)
	}
	if !result.ShouldRemoveToken {
		t.Error("expected ShouldRemoveToken=true")
	}
}

func TestSend_RateLimited(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusTooManyRequests)
		json.NewEncoder(w).Encode(map[string]string{
			"reason": "TooManyRequests",
		})
	}))
	defer srv.Close()

	ch, _ := apns.New(apns.Config{
		KeyID:      "KEY123",
		TeamID:     "TEAM456",
		BundleID:   "com.kaivue.app",
		PrivateKey: generateTestKey(t),
		Endpoint:   srv.URL,
	})

	result, _ := ch.Send(context.Background(), notifications.Message{
		Target: "token-abc",
	})
	if result.State != notifications.StateThrottled {
		t.Errorf("expected throttled, got %s", result.State)
	}
}

func TestBatchSend(t *testing.T) {
	var count int
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		count++
		w.Header().Set("apns-id", "batch-uuid")
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	ch, _ := apns.New(apns.Config{
		KeyID:      "KEY123",
		TeamID:     "TEAM456",
		BundleID:   "com.kaivue.app",
		PrivateKey: generateTestKey(t),
		Endpoint:   srv.URL,
	})

	results, err := ch.BatchSend(context.Background(), notifications.BatchMessage{
		MessageID: "batch-1",
		Targets: []notifications.Target{
			{UserID: "u1", DeviceToken: "t1", Platform: notifications.PlatformAPNs},
			{UserID: "u2", DeviceToken: "t2", Platform: notifications.PlatformAPNs},
		},
		Title: "Batch",
		Body:  "Test",
	})
	if err != nil {
		t.Fatalf("batch: %v", err)
	}
	if len(results) != 2 {
		t.Fatalf("expected 2 results, got %d", len(results))
	}
	if count != 2 {
		t.Errorf("expected 2 requests, got %d", count)
	}
}

func TestCheckHealth(t *testing.T) {
	ch, _ := apns.New(apns.Config{
		KeyID:      "KEY123",
		TeamID:     "TEAM456",
		BundleID:   "com.kaivue.app",
		PrivateKey: generateTestKey(t),
	})
	if err := ch.CheckHealth(context.Background()); err != nil {
		t.Errorf("health check should pass: %v", err)
	}
}

func TestType(t *testing.T) {
	ch, _ := apns.New(apns.Config{
		KeyID:      "KEY123",
		TeamID:     "TEAM456",
		BundleID:   "com.kaivue.app",
		PrivateKey: generateTestKey(t),
	})
	if ch.Type() != notifications.ChannelPush {
		t.Errorf("expected push, got %s", ch.Type())
	}
}

func TestNew_ValidationErrors(t *testing.T) {
	testKey := generateTestKey(t)
	tests := []struct {
		name string
		cfg  apns.Config
	}{
		{"missing key_id", apns.Config{TeamID: "T", BundleID: "B", PrivateKey: testKey}},
		{"missing team_id", apns.Config{KeyID: "K", BundleID: "B", PrivateKey: testKey}},
		{"missing bundle_id", apns.Config{KeyID: "K", TeamID: "T", PrivateKey: testKey}},
		{"missing private_key", apns.Config{KeyID: "K", TeamID: "T", BundleID: "B"}},
		{"bad pem", apns.Config{KeyID: "K", TeamID: "T", BundleID: "B", PrivateKey: "not-pem"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := apns.New(tt.cfg)
			if err == nil {
				t.Error("expected error")
			}
		})
	}
}

func TestStats(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("apns-id", "ok")
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	ch, _ := apns.New(apns.Config{
		KeyID:      "KEY123",
		TeamID:     "TEAM456",
		BundleID:   "com.kaivue.app",
		PrivateKey: generateTestKey(t),
		Endpoint:   srv.URL,
	})

	ch.Send(context.Background(), notifications.Message{Target: "t1", Title: "x"})

	sent, failed, removed := ch.Stats()
	if sent != 1 {
		t.Errorf("expected 1 sent, got %d", sent)
	}
	if failed != 0 || removed != 0 {
		t.Errorf("expected 0 failed/removed, got %d/%d", failed, removed)
	}
}
