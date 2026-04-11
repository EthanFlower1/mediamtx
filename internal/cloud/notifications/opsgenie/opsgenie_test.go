package opsgenie

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/bluenviron/mediamtx/internal/cloud/notifications"
)

func TestSend_CreateAlert(t *testing.T) {
	var received opsgenieAlert
	var authHeader string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		authHeader = r.Header.Get("Authorization")
		if r.Method != http.MethodPost {
			t.Errorf("expected POST, got %s", r.Method)
		}
		json.NewDecoder(r.Body).Decode(&received)
		w.WriteHeader(http.StatusAccepted)
		w.Write([]byte(`{"requestId":"abc","result":"created"}`))
	}))
	defer srv.Close()

	adapter, err := New(Config{
		APIKey:     "test-api-key",
		APIURL:     srv.URL,
		HTTPClient: srv.Client(),
	})
	if err != nil {
		t.Fatalf("new: %v", err)
	}

	msg := notifications.CommsMessage{
		ID:        "msg-1",
		TenantID:  "tenant-1",
		EventType: "camera.offline",
		Summary:   "Camera offline: lobby-1",
		Body:      "Camera lobby-1 has been offline for 5 minutes.",
		Severity:  notifications.SeverityCritical,
		DedupKey:  "camera-offline-lobby-1",
		Timestamp: time.Now(),
	}

	result := adapter.Send(context.Background(), msg)
	if result.State != notifications.CommsDeliverySuccess {
		t.Fatalf("expected success, got %s: %s", result.State, result.ErrorMessage)
	}

	if authHeader != "GenieKey test-api-key" {
		t.Errorf("expected GenieKey auth, got %s", authHeader)
	}
	if received.Message != "Camera offline: lobby-1" {
		t.Errorf("expected summary in message field, got %s", received.Message)
	}
	if received.Priority != "P1" {
		t.Errorf("expected P1, got %s", received.Priority)
	}
	if received.Alias != "camera-offline-lobby-1" {
		t.Errorf("expected alias, got %s", received.Alias)
	}
}

func TestSend_ResolveAlert(t *testing.T) {
	var requestURL string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requestURL = r.URL.String()
		w.WriteHeader(http.StatusAccepted)
	}))
	defer srv.Close()

	adapter, _ := New(Config{
		APIKey:     "test-key",
		APIURL:     srv.URL,
		HTTPClient: srv.Client(),
	})

	result := adapter.Send(context.Background(), notifications.CommsMessage{
		ID:       "msg-2",
		Action:   "resolve",
		DedupKey: "camera-offline-lobby-1",
		TenantID: "t1",
	})
	if result.State != notifications.CommsDeliverySuccess {
		t.Fatalf("expected success, got %s: %s", result.State, result.ErrorMessage)
	}
	if !strings.Contains(requestURL, "camera-offline-lobby-1/close") {
		t.Errorf("expected close URL, got %s", requestURL)
	}
}

func TestSend_AcknowledgeAlert(t *testing.T) {
	var requestURL string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requestURL = r.URL.String()
		w.WriteHeader(http.StatusAccepted)
	}))
	defer srv.Close()

	adapter, _ := New(Config{
		APIKey:     "test-key",
		APIURL:     srv.URL,
		HTTPClient: srv.Client(),
	})

	result := adapter.Send(context.Background(), notifications.CommsMessage{
		ID:       "msg-3",
		Action:   "acknowledge",
		DedupKey: "camera-offline-lobby-1",
		TenantID: "t1",
	})
	if result.State != notifications.CommsDeliverySuccess {
		t.Fatalf("expected success, got %s", result.State)
	}
	if !strings.Contains(requestURL, "camera-offline-lobby-1/acknowledge") {
		t.Errorf("expected acknowledge URL, got %s", requestURL)
	}
}

func TestSend_ResolveMissingDedupKey(t *testing.T) {
	adapter, _ := New(Config{
		APIKey:     "test-key",
		APIURL:     "http://localhost",
		HTTPClient: http.DefaultClient,
	})

	result := adapter.Send(context.Background(), notifications.CommsMessage{
		ID:     "msg-4",
		Action: "resolve",
	})
	if result.State != notifications.CommsDeliveryFailure {
		t.Fatalf("expected failure, got %s", result.State)
	}
}

func TestSend_HTTPError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer srv.Close()

	adapter, _ := New(Config{
		APIKey:     "test-key",
		APIURL:     srv.URL,
		HTTPClient: srv.Client(),
	})

	result := adapter.Send(context.Background(), notifications.CommsMessage{ID: "msg-5", Summary: "test"})
	if result.State != notifications.CommsDeliveryFailure {
		t.Fatalf("expected failure, got %s", result.State)
	}
}

func TestSend_PriorityMapping(t *testing.T) {
	tests := []struct {
		severity notifications.Severity
		expected string
	}{
		{notifications.SeverityCritical, "P1"},
		{notifications.SeverityHigh, "P2"},
		{notifications.SeverityWarning, "P3"},
		{notifications.SeverityInfo, "P5"},
	}

	for _, tt := range tests {
		t.Run(string(tt.severity), func(t *testing.T) {
			var received opsgenieAlert
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				json.NewDecoder(r.Body).Decode(&received)
				w.WriteHeader(http.StatusAccepted)
			}))
			defer srv.Close()

			adapter, _ := New(Config{
				APIKey:     "k",
				APIURL:     srv.URL,
				HTTPClient: srv.Client(),
			})
			adapter.Send(context.Background(), notifications.CommsMessage{
				ID:       "p-test",
				Severity: tt.severity,
				Summary:  "test",
			})
			if received.Priority != tt.expected {
				t.Errorf("expected %s, got %s", tt.expected, received.Priority)
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
		APIKey:     "k",
		APIURL:     srv.URL,
		HTTPClient: srv.Client(),
	})

	results := adapter.BatchSend(context.Background(), []notifications.CommsMessage{
		{ID: "b-1", Summary: "one"}, {ID: "b-2", Summary: "two"},
	})
	if len(results) != 2 {
		t.Fatalf("expected 2, got %d", len(results))
	}
}

func TestCheckHealth_OK(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			t.Errorf("expected GET, got %s", r.Method)
		}
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	adapter, _ := New(Config{
		APIKey:     "test-key",
		APIURL:     srv.URL,
		HTTPClient: srv.Client(),
	})
	if err := adapter.CheckHealth(context.Background()); err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
}

func TestCheckHealth_InvalidKey(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
	}))
	defer srv.Close()

	adapter, _ := New(Config{
		APIKey:     "bad-key",
		APIURL:     srv.URL,
		HTTPClient: srv.Client(),
	})
	if err := adapter.CheckHealth(context.Background()); err == nil {
		t.Fatal("expected error for invalid key")
	}
}

func TestNew_MissingKey(t *testing.T) {
	_, err := New(Config{})
	if err == nil {
		t.Fatal("expected error for missing API key")
	}
}

func TestType(t *testing.T) {
	adapter, _ := New(Config{APIKey: "k", APIURL: "http://localhost"})
	if adapter.Type() != notifications.ChannelOpsgenie {
		t.Errorf("expected opsgenie, got %s", adapter.Type())
	}
}

var _ notifications.CommsDeliveryChannel = (*Adapter)(nil)
