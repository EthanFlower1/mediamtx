package sendgrid_test

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/bluenviron/mediamtx/internal/cloud/notifications"
	"github.com/bluenviron/mediamtx/internal/cloud/notifications/sendgrid"
)

func newTestAdapter(t *testing.T, handler http.HandlerFunc) *sendgrid.Adapter {
	t.Helper()
	srv := httptest.NewServer(handler)
	t.Cleanup(srv.Close)

	a, err := sendgrid.New(sendgrid.Config{
		Sender: &sendgrid.StaticSenderResolver{
			Identity: sendgrid.SenderIdentity{
				APIKey:    "test-key",
				FromEmail: "noreply@kaivue.io",
				FromName:  "KaiVue",
			},
		},
		Client:  srv.Client(),
		BaseURL: srv.URL,
	})
	if err != nil {
		t.Fatalf("new adapter: %v", err)
	}
	return a
}

func TestSendGridName(t *testing.T) {
	a := newTestAdapter(t, func(w http.ResponseWriter, r *http.Request) {})
	if a.Name() != "sendgrid" {
		t.Errorf("expected name sendgrid, got %s", a.Name())
	}
}

func TestSendGridSupportedTypes(t *testing.T) {
	a := newTestAdapter(t, func(w http.ResponseWriter, r *http.Request) {})
	types := a.SupportedTypes()
	if len(types) != 1 || types[0] != notifications.MessageTypeEmail {
		t.Errorf("expected [email], got %v", types)
	}
}

func TestSendGridSendSuccess(t *testing.T) {
	var receivedBody map[string]interface{}
	a := newTestAdapter(t, func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v3/mail/send" {
			t.Errorf("unexpected path: %s", r.URL.Path)
		}
		if r.Header.Get("Authorization") != "Bearer test-key" {
			t.Errorf("missing auth header")
		}
		body, _ := io.ReadAll(r.Body)
		json.Unmarshal(body, &receivedBody)
		w.Header().Set("X-Message-Id", "sg-msg-123")
		w.WriteHeader(http.StatusAccepted)
	})

	msg := notifications.Message{
		ID:       "test-001",
		Type:     notifications.MessageTypeEmail,
		TenantID: "tenant-1",
		To:       []notifications.Recipient{{Address: "user@example.com", Name: "User"}},
		Subject:  "Alert",
		Body:     "Camera offline",
	}

	result, err := a.Send(context.Background(), msg)
	if err != nil {
		t.Fatalf("send: %v", err)
	}
	if result.State != notifications.DeliveryStateDelivered {
		t.Errorf("expected delivered, got %s", result.State)
	}
	if result.ProviderMessageID != "sg-msg-123" {
		t.Errorf("expected provider message ID sg-msg-123, got %s", result.ProviderMessageID)
	}

	// Verify payload structure.
	if receivedBody == nil {
		t.Fatal("expected request body")
	}
	from, ok := receivedBody["from"].(map[string]interface{})
	if !ok {
		t.Fatal("expected from field")
	}
	if from["email"] != "noreply@kaivue.io" {
		t.Errorf("expected from email noreply@kaivue.io, got %v", from["email"])
	}
}

func TestSendGridBatchSend(t *testing.T) {
	a := newTestAdapter(t, func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("X-Message-Id", "sg-batch-456")
		w.WriteHeader(http.StatusAccepted)
	})

	msg := notifications.Message{
		ID:       "batch-001",
		Type:     notifications.MessageTypeEmail,
		TenantID: "tenant-1",
		To: []notifications.Recipient{
			{Address: "a@example.com"},
			{Address: "b@example.com"},
		},
		Subject: "Batch Test",
		Body:    "Hello batch",
	}

	results, err := a.BatchSend(context.Background(), msg)
	if err != nil {
		t.Fatalf("batch send: %v", err)
	}
	if len(results) != 2 {
		t.Fatalf("expected 2 results, got %d", len(results))
	}
	for _, r := range results {
		if r.State != notifications.DeliveryStateDelivered {
			t.Errorf("expected delivered, got %s", r.State)
		}
	}
}

func TestSendGridTemplateSupport(t *testing.T) {
	var receivedBody map[string]interface{}
	a := newTestAdapter(t, func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		json.Unmarshal(body, &receivedBody)
		w.WriteHeader(http.StatusAccepted)
	})

	msg := notifications.Message{
		ID:       "tmpl-001",
		Type:     notifications.MessageTypeEmail,
		TenantID: "tenant-1",
		To:       []notifications.Recipient{{Address: "user@example.com"}},
		Body:     "fallback",
		TemplateID:   "d-abc123",
		TemplateData: map[string]string{"camera_name": "Front Door", "event": "offline"},
	}

	_, err := a.Send(context.Background(), msg)
	if err != nil {
		t.Fatalf("send: %v", err)
	}

	if receivedBody["template_id"] != "d-abc123" {
		t.Errorf("expected template_id d-abc123, got %v", receivedBody["template_id"])
	}
}

func TestSendGridHTMLEmail(t *testing.T) {
	var receivedBody map[string]interface{}
	a := newTestAdapter(t, func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		json.Unmarshal(body, &receivedBody)
		w.WriteHeader(http.StatusAccepted)
	})

	msg := notifications.Message{
		ID:       "html-001",
		Type:     notifications.MessageTypeEmail,
		TenantID: "tenant-1",
		To:       []notifications.Recipient{{Address: "user@example.com"}},
		Subject:  "HTML Test",
		Body:     "Plain text",
		HTMLBody: "<h1>Hello</h1>",
	}

	_, err := a.Send(context.Background(), msg)
	if err != nil {
		t.Fatalf("send: %v", err)
	}

	content, ok := receivedBody["content"].([]interface{})
	if !ok {
		t.Fatal("expected content array")
	}
	if len(content) != 2 {
		t.Fatalf("expected 2 content parts (text + html), got %d", len(content))
	}
}

func TestSendGridRetryableError(t *testing.T) {
	a := newTestAdapter(t, func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/v3/mail/send" {
			w.WriteHeader(http.StatusTooManyRequests)
			w.Write([]byte(`{"errors":[{"message":"rate limit"}]}`))
		}
	})

	msg := notifications.Message{
		ID:       "retry-001",
		Type:     notifications.MessageTypeEmail,
		TenantID: "tenant-1",
		To:       []notifications.Recipient{{Address: "user@example.com"}},
		Body:     "test",
	}

	_, err := a.BatchSend(context.Background(), msg)
	if err == nil {
		t.Fatal("expected error on 429")
	}
	// Should contain "retryable" so the dispatcher knows to retry.
	if got := err.Error(); !contains(got, "retryable") {
		t.Errorf("expected retryable in error, got: %s", got)
	}
}

func TestSendGridPermanentError(t *testing.T) {
	a := newTestAdapter(t, func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/v3/mail/send" {
			w.WriteHeader(http.StatusBadRequest)
			w.Write([]byte(`{"errors":[{"message":"invalid email"}]}`))
		}
	})

	msg := notifications.Message{
		ID:       "perm-001",
		Type:     notifications.MessageTypeEmail,
		TenantID: "tenant-1",
		To:       []notifications.Recipient{{Address: "bad"}},
		Body:     "test",
	}

	results, err := a.BatchSend(context.Background(), msg)
	if err != nil {
		t.Fatalf("expected no error for permanent failure, got: %v", err)
	}
	if len(results) != 1 || results[0].State != notifications.DeliveryStateFailed {
		t.Error("expected failed state on permanent error")
	}
}

func TestSendGridUnsupportedType(t *testing.T) {
	a := newTestAdapter(t, func(w http.ResponseWriter, r *http.Request) {})
	msg := notifications.Message{
		ID:       "sms-001",
		Type:     notifications.MessageTypeSMS,
		TenantID: "tenant-1",
		To:       []notifications.Recipient{{Address: "+1234567890"}},
		Body:     "test",
	}
	_, err := a.BatchSend(context.Background(), msg)
	if err == nil {
		t.Error("expected error for unsupported type")
	}
}

func TestSendGridCheckHealth(t *testing.T) {
	a := newTestAdapter(t, func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/v3/scopes" {
			w.WriteHeader(http.StatusOK)
			w.Write([]byte(`{"scopes":["mail.send"]}`))
		}
	})

	hs, err := a.CheckHealth(context.Background())
	if err != nil {
		t.Fatalf("health check: %v", err)
	}
	if !hs.Healthy {
		t.Error("expected healthy=true")
	}
	if hs.Latency <= 0 {
		t.Error("expected positive latency")
	}
}

func TestSendGridCheckHealthUnhealthy(t *testing.T) {
	a := newTestAdapter(t, func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/v3/scopes" {
			w.WriteHeader(http.StatusInternalServerError)
		}
	})

	hs, err := a.CheckHealth(context.Background())
	if err != nil {
		t.Fatalf("health check: %v", err)
	}
	if hs.Healthy {
		t.Error("expected healthy=false")
	}
}

func contains(s, sub string) bool {
	return len(s) >= len(sub) && (s == sub || len(s) > 0 && containsStr(s, sub))
}

func containsStr(s, sub string) bool {
	for i := 0; i <= len(s)-len(sub); i++ {
		if s[i:i+len(sub)] == sub {
			return true
		}
	}
	return false
}
