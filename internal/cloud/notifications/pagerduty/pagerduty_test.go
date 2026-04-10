package pagerduty

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/bluenviron/mediamtx/internal/cloud/notifications"
)

func TestSend_Trigger(t *testing.T) {
	var received pdEvent
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			t.Errorf("expected POST, got %s", r.Method)
		}
		if err := json.NewDecoder(r.Body).Decode(&received); err != nil {
			t.Fatalf("decode: %v", err)
		}
		w.WriteHeader(http.StatusAccepted)
		w.Write([]byte(`{"status":"success","dedup_key":"test"}`))
	}))
	defer srv.Close()

	adapter, err := New(Config{
		RoutingKey: "test-routing-key",
		EventsURL:  srv.URL,
		HTTPClient: srv.Client(),
	})
	if err != nil {
		t.Fatalf("new: %v", err)
	}

	msg := notifications.Message{
		ID:        "msg-1",
		TenantID:  "tenant-1",
		EventType: "camera.offline",
		Summary:   "Camera offline: lobby-1",
		Body:      "Camera lobby-1 has been offline for 5 minutes.",
		Severity:  notifications.SeverityCritical,
		DedupKey:  "camera-offline-lobby-1",
		Timestamp: time.Date(2026, 4, 10, 12, 0, 0, 0, time.UTC),
	}

	result := adapter.Send(context.Background(), msg)
	if result.State != notifications.DeliverySuccess {
		t.Fatalf("expected success, got %s: %s", result.State, result.ErrorMessage)
	}

	// Verify PagerDuty event structure
	if received.RoutingKey != "test-routing-key" {
		t.Errorf("expected routing key, got %s", received.RoutingKey)
	}
	if received.EventAction != "trigger" {
		t.Errorf("expected trigger, got %s", received.EventAction)
	}
	if received.DedupKey != "camera-offline-lobby-1" {
		t.Errorf("expected dedup key, got %s", received.DedupKey)
	}
	if received.Payload == nil {
		t.Fatal("expected payload for trigger event")
	}
	if received.Payload.Severity != "critical" {
		t.Errorf("expected critical, got %s", received.Payload.Severity)
	}
	if received.Payload.Class != "camera.offline" {
		t.Errorf("expected camera.offline class, got %s", received.Payload.Class)
	}
}

func TestSend_Acknowledge(t *testing.T) {
	var received pdEvent
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		json.NewDecoder(r.Body).Decode(&received)
		w.WriteHeader(http.StatusAccepted)
	}))
	defer srv.Close()

	adapter, _ := New(Config{
		RoutingKey: "test-key",
		EventsURL:  srv.URL,
		HTTPClient: srv.Client(),
	})

	result := adapter.Send(context.Background(), notifications.Message{
		ID:       "msg-2",
		Action:   "acknowledge",
		DedupKey: "camera-offline-lobby-1",
	})
	if result.State != notifications.DeliverySuccess {
		t.Fatalf("expected success, got %s", result.State)
	}
	if received.EventAction != "acknowledge" {
		t.Errorf("expected acknowledge, got %s", received.EventAction)
	}
	// Acknowledge should not have a payload
	if received.Payload != nil {
		t.Error("acknowledge should not have payload")
	}
}

func TestSend_Resolve(t *testing.T) {
	var received pdEvent
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		json.NewDecoder(r.Body).Decode(&received)
		w.WriteHeader(http.StatusAccepted)
	}))
	defer srv.Close()

	adapter, _ := New(Config{
		RoutingKey: "test-key",
		EventsURL:  srv.URL,
		HTTPClient: srv.Client(),
	})

	result := adapter.Send(context.Background(), notifications.Message{
		ID:       "msg-3",
		Action:   "resolve",
		DedupKey: "camera-offline-lobby-1",
	})
	if result.State != notifications.DeliverySuccess {
		t.Fatalf("expected success, got %s", result.State)
	}
	if received.EventAction != "resolve" {
		t.Errorf("expected resolve, got %s", received.EventAction)
	}
	if received.Payload != nil {
		t.Error("resolve should not have payload")
	}
}

func TestSend_HTTPError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer srv.Close()

	adapter, _ := New(Config{
		RoutingKey: "test-key",
		EventsURL:  srv.URL,
		HTTPClient: srv.Client(),
	})

	result := adapter.Send(context.Background(), notifications.Message{ID: "msg-4"})
	if result.State != notifications.DeliveryFailure {
		t.Fatalf("expected failure, got %s", result.State)
	}
}

func TestSend_SeverityMapping(t *testing.T) {
	tests := []struct {
		severity notifications.Severity
		expected string
	}{
		{notifications.SeverityCritical, "critical"},
		{notifications.SeverityHigh, "error"},
		{notifications.SeverityWarning, "warning"},
		{notifications.SeverityInfo, "info"},
	}

	for _, tt := range tests {
		t.Run(string(tt.severity), func(t *testing.T) {
			var received pdEvent
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				json.NewDecoder(r.Body).Decode(&received)
				w.WriteHeader(http.StatusAccepted)
			}))
			defer srv.Close()

			adapter, _ := New(Config{
				RoutingKey: "test-key",
				EventsURL:  srv.URL,
				HTTPClient: srv.Client(),
			})
			adapter.Send(context.Background(), notifications.Message{
				ID:       "sev-test",
				Severity: tt.severity,
				Summary:  "test",
			})
			if received.Payload.Severity != tt.expected {
				t.Errorf("expected %s, got %s", tt.expected, received.Payload.Severity)
			}
		})
	}
}

func TestBatchSend(t *testing.T) {
	count := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		count++
		w.WriteHeader(http.StatusAccepted)
	}))
	defer srv.Close()

	adapter, _ := New(Config{
		RoutingKey: "test-key",
		EventsURL:  srv.URL,
		HTTPClient: srv.Client(),
	})

	results := adapter.BatchSend(context.Background(), []notifications.Message{
		{ID: "b-1", Summary: "one"}, {ID: "b-2", Summary: "two"},
	})
	if len(results) != 2 {
		t.Fatalf("expected 2, got %d", len(results))
	}
}

func TestCheckHealth_OK(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusAccepted)
	}))
	defer srv.Close()

	adapter, _ := New(Config{
		RoutingKey: "test-key",
		EventsURL:  srv.URL,
		HTTPClient: srv.Client(),
	})
	if err := adapter.CheckHealth(context.Background()); err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
}

func TestCheckHealth_InvalidKey(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusForbidden)
	}))
	defer srv.Close()

	adapter, _ := New(Config{
		RoutingKey: "bad-key",
		EventsURL:  srv.URL,
		HTTPClient: srv.Client(),
	})
	if err := adapter.CheckHealth(context.Background()); err == nil {
		t.Fatal("expected error for invalid key")
	}
}

func TestNew_MissingKey(t *testing.T) {
	_, err := New(Config{})
	if err == nil {
		t.Fatal("expected error for missing routing key")
	}
}

func TestType(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {}))
	defer srv.Close()
	adapter, _ := New(Config{RoutingKey: "k", EventsURL: srv.URL, HTTPClient: srv.Client()})
	if adapter.Type() != notifications.ChannelPagerDuty {
		t.Errorf("expected pagerduty, got %s", adapter.Type())
	}
}

var _ notifications.DeliveryChannel = (*Adapter)(nil)
