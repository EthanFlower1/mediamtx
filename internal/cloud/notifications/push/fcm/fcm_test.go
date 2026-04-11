package fcm_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/bluenviron/mediamtx/internal/cloud/notifications"
	"github.com/bluenviron/mediamtx/internal/cloud/notifications/push/fcm"
)

type staticToken struct{ token string }

func (s *staticToken) Token(_ context.Context) (string, error) { return s.token, nil }

func TestSend_Success(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer test-token" {
			t.Errorf("unexpected auth header: %s", r.Header.Get("Authorization"))
		}
		if r.Header.Get("Content-Type") != "application/json" {
			t.Errorf("unexpected content-type: %s", r.Header.Get("Content-Type"))
		}

		w.WriteHeader(http.StatusOK)
		json.NewEncoder(w).Encode(map[string]string{
			"name": "projects/test/messages/123",
		})
	}))
	defer srv.Close()

	ch, err := fcm.New(fcm.Config{
		ProjectID:   "test-project",
		TokenSource: &staticToken{token: "test-token"},
		Endpoint:    srv.URL,
	})
	if err != nil {
		t.Fatalf("new: %v", err)
	}

	result, err := ch.Send(context.Background(), notifications.PushMessage{
		MessageID: "msg-1",
		TenantID:  "tenant-1",
		UserID:    "user-1",
		Target:    "device-token-abc",
		Title:     "Test Alert",
		Body:      "Camera offline",
		Priority:  "high",
	})
	if err != nil {
		t.Fatalf("send: %v", err)
	}
	if result.State != notifications.PushStateDelivered {
		t.Errorf("expected delivered, got %s", result.State)
	}
	if result.PlatformID != "projects/test/messages/123" {
		t.Errorf("unexpected platform id: %s", result.PlatformID)
	}
}

func TestSend_InvalidToken(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNotFound)
		json.NewEncoder(w).Encode(map[string]interface{}{
			"error": map[string]interface{}{
				"code":    404,
				"message": "Requested entity was not found.",
				"status":  "NOT_FOUND",
			},
		})
	}))
	defer srv.Close()

	ch, _ := fcm.New(fcm.Config{
		ProjectID:   "test-project",
		TokenSource: &staticToken{token: "test-token"},
		Endpoint:    srv.URL,
	})

	result, err := ch.Send(context.Background(), notifications.PushMessage{
		Target: "invalid-token",
	})
	if err != nil {
		t.Fatalf("send: %v", err)
	}
	if result.State != notifications.PushStateUnreachable {
		t.Errorf("expected unreachable, got %s", result.State)
	}
	if !result.ShouldRemoveToken {
		t.Error("expected ShouldRemoveToken=true")
	}
}

func TestSend_RateLimited(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusTooManyRequests)
		json.NewEncoder(w).Encode(map[string]interface{}{
			"error": map[string]interface{}{
				"code":    429,
				"message": "Quota exceeded.",
				"status":  "RESOURCE_EXHAUSTED",
			},
		})
	}))
	defer srv.Close()

	ch, _ := fcm.New(fcm.Config{
		ProjectID:   "test-project",
		TokenSource: &staticToken{token: "test-token"},
		Endpoint:    srv.URL,
	})

	result, err := ch.Send(context.Background(), notifications.PushMessage{
		Target: "token-abc",
	})
	if err != nil {
		t.Fatalf("send: %v", err)
	}
	if result.State != notifications.PushStateThrottled {
		t.Errorf("expected throttled, got %s", result.State)
	}
}

func TestBatchSend(t *testing.T) {
	var count int
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		count++
		w.WriteHeader(http.StatusOK)
		json.NewEncoder(w).Encode(map[string]string{
			"name": "projects/test/messages/batch",
		})
	}))
	defer srv.Close()

	ch, _ := fcm.New(fcm.Config{
		ProjectID:   "test-project",
		TokenSource: &staticToken{token: "test-token"},
		Endpoint:    srv.URL,
	})

	results, err := ch.BatchSend(context.Background(), notifications.BatchMessage{
		MessageID: "batch-1",
		TenantID:  "tenant-1",
		Targets: []notifications.Target{
			{UserID: "user-1", DeviceToken: "token-1", Platform: notifications.PlatformFCM},
			{UserID: "user-2", DeviceToken: "token-2", Platform: notifications.PlatformFCM},
			{UserID: "user-3", DeviceToken: "token-3", Platform: notifications.PlatformFCM},
		},
		Title: "Batch Test",
		Body:  "Testing batch send",
	})
	if err != nil {
		t.Fatalf("batch send: %v", err)
	}
	if len(results) != 3 {
		t.Fatalf("expected 3 results, got %d", len(results))
	}
	if count != 3 {
		t.Errorf("expected 3 HTTP requests, got %d", count)
	}
	for i, r := range results {
		if r.State != notifications.PushStateDelivered {
			t.Errorf("result[%d]: expected delivered, got %s", i, r.State)
		}
	}
}

func TestCheckHealth(t *testing.T) {
	ch, _ := fcm.New(fcm.Config{
		ProjectID:   "test-project",
		TokenSource: &staticToken{token: "test-token"},
	})
	if err := ch.CheckHealth(context.Background()); err != nil {
		t.Errorf("health check should pass: %v", err)
	}
}

func TestType(t *testing.T) {
	ch, _ := fcm.New(fcm.Config{
		ProjectID:   "test-project",
		TokenSource: &staticToken{token: "test-token"},
	})
	if ch.Type() != notifications.ChannelPush {
		t.Errorf("expected push, got %s", ch.Type())
	}
}

func TestStats(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
		json.NewEncoder(w).Encode(map[string]string{"name": "ok"})
	}))
	defer srv.Close()

	ch, _ := fcm.New(fcm.Config{
		ProjectID:   "test-project",
		TokenSource: &staticToken{token: "test-token"},
		Endpoint:    srv.URL,
	})

	ch.Send(context.Background(), notifications.PushMessage{Target: "t1", Title: "x"})
	ch.Send(context.Background(), notifications.PushMessage{Target: "t2", Title: "x"})

	sent, failed, removed := ch.Stats()
	if sent != 2 {
		t.Errorf("expected 2 sent, got %d", sent)
	}
	if failed != 0 {
		t.Errorf("expected 0 failed, got %d", failed)
	}
	if removed != 0 {
		t.Errorf("expected 0 removed, got %d", removed)
	}
}
