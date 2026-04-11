package webhook

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"

	"github.com/bluenviron/mediamtx/internal/cloud/notifications"
)

func TestSend_Success(t *testing.T) {
	var receivedBody []byte
	var sigHeader string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			t.Errorf("expected POST, got %s", r.Method)
		}
		sigHeader = r.Header.Get("X-Kaivue-Signature")
		receivedBody, _ = io.ReadAll(r.Body)
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	adapter, err := New(Config{
		URL:        srv.URL,
		Secret:     "my-secret",
		MaxRetries: 0,
		HTTPClient: srv.Client(),
	})
	if err != nil {
		t.Fatalf("new: %v", err)
	}

	msg := notifications.CommsMessage{
		ID:        "msg-1",
		TenantID:  "tenant-1",
		EventType: "camera.offline",
		Summary:   "Camera offline",
		Severity:  notifications.SeverityHigh,
		Timestamp: time.Date(2026, 4, 10, 12, 0, 0, 0, time.UTC),
	}

	result := adapter.Send(context.Background(), msg)
	if result.State != notifications.CommsDeliverySuccess {
		t.Fatalf("expected success, got %s: %s", result.State, result.ErrorMessage)
	}

	// Verify HMAC signature
	if sigHeader == "" {
		t.Fatal("expected X-Kaivue-Signature header")
	}
	if !VerifyHMAC(receivedBody, "my-secret", sigHeader) {
		t.Error("HMAC verification failed")
	}

	// Verify body is valid JSON message
	var received notifications.Message
	if err := json.Unmarshal(receivedBody, &received); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if received.ID != "msg-1" {
		t.Errorf("expected msg-1, got %s", received.ID)
	}
}

func TestSend_NoSecret(t *testing.T) {
	var sigHeader string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		sigHeader = r.Header.Get("X-Kaivue-Signature")
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	adapter, _ := New(Config{URL: srv.URL, MaxRetries: 0, HTTPClient: srv.Client()})
	result := adapter.Send(context.Background(), notifications.CommsMessage{ID: "msg-2"})
	if result.State != notifications.CommsDeliverySuccess {
		t.Fatalf("expected success, got %s", result.State)
	}
	if sigHeader != "" {
		t.Error("expected no signature when secret is empty")
	}
}

func TestSend_RetryOnServerError(t *testing.T) {
	var attempts int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		n := atomic.AddInt32(&attempts, 1)
		if n <= 2 {
			w.WriteHeader(http.StatusInternalServerError)
			return
		}
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	adapter, _ := New(Config{
		URL:        srv.URL,
		MaxRetries: 2,
		RetryDelay: time.Millisecond, // fast retries for test
		HTTPClient: srv.Client(),
	})

	result := adapter.Send(context.Background(), notifications.CommsMessage{ID: "msg-3"})
	if result.State != notifications.CommsDeliverySuccess {
		t.Fatalf("expected success after retry, got %s: %s", result.State, result.ErrorMessage)
	}
	if atomic.LoadInt32(&attempts) != 3 {
		t.Errorf("expected 3 attempts, got %d", atomic.LoadInt32(&attempts))
	}
}

func TestSend_NoRetryOnClientError(t *testing.T) {
	var attempts int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&attempts, 1)
		w.WriteHeader(http.StatusBadRequest)
	}))
	defer srv.Close()

	adapter, _ := New(Config{
		URL:        srv.URL,
		MaxRetries: 2,
		RetryDelay: time.Millisecond,
		HTTPClient: srv.Client(),
	})

	result := adapter.Send(context.Background(), notifications.CommsMessage{ID: "msg-4"})
	if result.State != notifications.CommsDeliveryFailure {
		t.Fatalf("expected failure, got %s", result.State)
	}
	// Client errors should not be retried
	if atomic.LoadInt32(&attempts) != 1 {
		t.Errorf("expected 1 attempt for client error, got %d", atomic.LoadInt32(&attempts))
	}
}

func TestSend_AllRetriesFail(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer srv.Close()

	adapter, _ := New(Config{
		URL:        srv.URL,
		MaxRetries: 1,
		RetryDelay: time.Millisecond,
		HTTPClient: srv.Client(),
	})

	result := adapter.Send(context.Background(), notifications.CommsMessage{ID: "msg-5"})
	if result.State != notifications.CommsDeliveryFailure {
		t.Fatalf("expected failure, got %s", result.State)
	}
}

func TestBatchSend(t *testing.T) {
	count := int32(0)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&count, 1)
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	adapter, _ := New(Config{URL: srv.URL, MaxRetries: 0, HTTPClient: srv.Client()})
	results := adapter.BatchSend(context.Background(), []notifications.CommsMessage{
		{ID: "b-1"}, {ID: "b-2"}, {ID: "b-3"},
	})
	if len(results) != 3 {
		t.Fatalf("expected 3, got %d", len(results))
	}
}

func TestCheckHealth_OK(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			t.Errorf("expected GET for health, got %s", r.Method)
		}
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	adapter, _ := New(Config{URL: srv.URL, HTTPClient: srv.Client()})
	if err := adapter.CheckHealth(context.Background()); err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
}

func TestCheckHealth_ServerError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer srv.Close()

	adapter, _ := New(Config{URL: srv.URL, HTTPClient: srv.Client()})
	if err := adapter.CheckHealth(context.Background()); err == nil {
		t.Fatal("expected error for server error")
	}
}

func TestComputeAndVerifyHMAC(t *testing.T) {
	body := []byte(`{"id":"test","summary":"hello"}`)
	secret := "super-secret-key"

	sig := ComputeHMAC(body, secret)
	if sig == "" {
		t.Fatal("expected non-empty signature")
	}
	if sig[:7] != "sha256=" {
		t.Errorf("expected sha256= prefix, got %s", sig[:7])
	}

	if !VerifyHMAC(body, secret, sig) {
		t.Error("verification should succeed with correct secret")
	}
	if VerifyHMAC(body, "wrong-secret", sig) {
		t.Error("verification should fail with wrong secret")
	}
	if VerifyHMAC([]byte("tampered"), secret, sig) {
		t.Error("verification should fail with tampered body")
	}
}

func TestNew_MissingURL(t *testing.T) {
	_, err := New(Config{})
	if err == nil {
		t.Fatal("expected error for missing URL")
	}
}

func TestType(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {}))
	defer srv.Close()
	adapter, _ := New(Config{URL: srv.URL, HTTPClient: srv.Client()})
	if adapter.Type() != notifications.ChannelWebhook {
		t.Errorf("expected webhook, got %s", adapter.Type())
	}
}

var _ notifications.CommsDeliveryChannel = (*Adapter)(nil)
