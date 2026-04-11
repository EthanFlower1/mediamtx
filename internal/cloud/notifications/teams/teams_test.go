package teams

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
	var received adaptiveCardEnvelope
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
		Body:      "Camera lobby-1 went offline.",
		Severity:  notifications.SeverityCritical,
		ActionURL: "https://app.example.com/cameras/lobby-1",
		Timestamp: time.Now(),
	}

	result := adapter.Send(context.Background(), msg)
	if result.State != notifications.CommsDeliverySuccess {
		t.Fatalf("expected success, got %s: %s", result.State, result.ErrorMessage)
	}

	// Verify Adaptive Card structure
	if received.Type != "message" {
		t.Errorf("expected message type, got %s", received.Type)
	}
	if len(received.Attachments) != 1 {
		t.Fatalf("expected 1 attachment, got %d", len(received.Attachments))
	}
	card := received.Attachments[0].Content
	if card.Version != "1.4" {
		t.Errorf("expected version 1.4, got %s", card.Version)
	}
	if len(card.Body) < 3 {
		t.Fatalf("expected at least 3 body elements, got %d", len(card.Body))
	}
	// Title should be Large + Bolder
	if card.Body[0].Size != "Large" || card.Body[0].Weight != "Bolder" {
		t.Errorf("title should be Large+Bolder")
	}
	// Critical severity -> attention color
	if card.Body[0].Color != "attention" {
		t.Errorf("expected attention color for critical, got %s", card.Body[0].Color)
	}
	// Should have action button
	if len(card.Actions) != 1 || card.Actions[0].Type != "Action.OpenUrl" {
		t.Error("expected OpenUrl action")
	}
}

func TestSend_NoActionURL(t *testing.T) {
	var received adaptiveCardEnvelope
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		json.NewDecoder(r.Body).Decode(&received)
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	adapter, _ := New(Config{WebhookURL: srv.URL, HTTPClient: srv.Client()})
	result := adapter.Send(context.Background(), notifications.CommsMessage{
		ID:       "msg-2",
		Summary:  "Test",
		Severity: notifications.SeverityInfo,
	})
	if result.State != notifications.CommsDeliverySuccess {
		t.Fatalf("expected success, got %s", result.State)
	}
	card := received.Attachments[0].Content
	if len(card.Actions) != 0 {
		t.Error("expected no actions when ActionURL is empty")
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
	results := adapter.BatchSend(context.Background(), []notifications.CommsMessage{
		{ID: "b-1"}, {ID: "b-2"},
	})
	if len(results) != 2 {
		t.Fatalf("expected 2 results, got %d", len(results))
	}
	if count != 2 {
		t.Errorf("expected 2 calls, got %d", count)
	}
}

func TestCheckHealth(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusBadRequest)
	}))
	defer srv.Close()

	adapter, _ := New(Config{WebhookURL: srv.URL, HTTPClient: srv.Client()})
	if err := adapter.CheckHealth(context.Background()); err != nil {
		t.Fatalf("expected no error, got %v", err)
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
	if adapter.Type() != notifications.ChannelTeams {
		t.Errorf("expected teams, got %s", adapter.Type())
	}
}

var _ notifications.CommsDeliveryChannel = (*Adapter)(nil)
