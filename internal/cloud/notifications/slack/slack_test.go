package slack

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/bluenviron/mediamtx/internal/cloud/notifications"
)

func TestSend_Success(t *testing.T) {
	var received blockKitPayload
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			t.Errorf("expected POST, got %s", r.Method)
		}
		if ct := r.Header.Get("Content-Type"); ct != "application/json" {
			t.Errorf("expected application/json, got %s", ct)
		}
		if err := json.NewDecoder(r.Body).Decode(&received); err != nil {
			t.Fatalf("decode: %v", err)
		}
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("ok"))
	}))
	defer srv.Close()

	adapter, err := New(Config{WebhookURL: srv.URL, HTTPClient: srv.Client()})
	if err != nil {
		t.Fatalf("new: %v", err)
	}

	msg := notifications.CommsMessage{
		ID:        "msg-1",
		TenantID:  "tenant-1",
		EventType: "camera.offline",
		Summary:   "Camera offline",
		Body:      "Camera *lobby-1* went offline at 14:32 UTC.",
		Severity:  notifications.SeverityHigh,
		ActionURL: "https://app.example.com/cameras/lobby-1",
		Timestamp: time.Now(),
	}

	result := adapter.Send(context.Background(), msg)
	if result.State != notifications.CommsDeliverySuccess {
		t.Fatalf("expected success, got %s: %s", result.State, result.ErrorMessage)
	}
	if result.MessageID != "msg-1" {
		t.Errorf("expected msg-1, got %s", result.MessageID)
	}

	// Verify Block Kit structure
	if len(received.Blocks) < 3 {
		t.Fatalf("expected at least 3 blocks, got %d", len(received.Blocks))
	}
	if received.Blocks[0].Type != "header" {
		t.Errorf("expected header block, got %s", received.Blocks[0].Type)
	}
	// Should have action button since ActionURL is set
	if len(received.Blocks) < 4 || received.Blocks[3].Type != "actions" {
		t.Error("expected actions block with button")
	}
}

func TestSend_NoActionURL(t *testing.T) {
	var received blockKitPayload
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		json.NewDecoder(r.Body).Decode(&received)
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	adapter, _ := New(Config{WebhookURL: srv.URL, HTTPClient: srv.Client()})

	msg := notifications.CommsMessage{
		ID:        "msg-2",
		Summary:   "Test",
		Body:      "No action URL",
		Severity:  notifications.SeverityInfo,
		Timestamp: time.Now(),
	}

	result := adapter.Send(context.Background(), msg)
	if result.State != notifications.CommsDeliverySuccess {
		t.Fatalf("expected success, got %s", result.State)
	}
	// No actions block when no ActionURL
	for _, b := range received.Blocks {
		if b.Type == "actions" {
			t.Error("expected no actions block when ActionURL is empty")
		}
	}
}

func TestSend_HTTPError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer srv.Close()

	adapter, _ := New(Config{WebhookURL: srv.URL, HTTPClient: srv.Client()})
	result := adapter.Send(context.Background(), notifications.CommsMessage{ID: "msg-3"})
	if result.State != notifications.CommsDeliveryFailure {
		t.Fatalf("expected failure, got %s", result.State)
	}
}

func TestBatchSend(t *testing.T) {
	count := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		count++
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	adapter, _ := New(Config{WebhookURL: srv.URL, HTTPClient: srv.Client()})
	msgs := []notifications.CommsMessage{
		{ID: "b-1", Summary: "One"},
		{ID: "b-2", Summary: "Two"},
		{ID: "b-3", Summary: "Three"},
	}
	results := adapter.BatchSend(context.Background(), msgs)
	if len(results) != 3 {
		t.Fatalf("expected 3 results, got %d", len(results))
	}
	if count != 3 {
		t.Errorf("expected 3 HTTP calls, got %d", count)
	}
}

func TestCheckHealth(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusBadRequest) // Slack returns 400 for empty body, but endpoint is reachable
	}))
	defer srv.Close()

	adapter, _ := New(Config{WebhookURL: srv.URL, HTTPClient: srv.Client()})
	if err := adapter.CheckHealth(context.Background()); err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
}

func TestCheckHealth_NotFound(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
	}))
	defer srv.Close()

	adapter, _ := New(Config{WebhookURL: srv.URL, HTTPClient: srv.Client()})
	if err := adapter.CheckHealth(context.Background()); err == nil {
		t.Fatal("expected error for 404")
	}
}

func TestNew_MissingURL(t *testing.T) {
	_, err := New(Config{})
	if err == nil {
		t.Fatal("expected error for missing webhook URL")
	}
}

func TestType(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {}))
	defer srv.Close()

	adapter, _ := New(Config{WebhookURL: srv.URL, HTTPClient: srv.Client()})
	if adapter.Type() != notifications.ChannelSlack {
		t.Errorf("expected slack, got %s", adapter.Type())
	}
}

// Verify interface compliance at compile time.
var _ notifications.CommsDeliveryChannel = (*Adapter)(nil)
