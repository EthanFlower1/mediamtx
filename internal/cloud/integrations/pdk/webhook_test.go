package pdk_test

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"

	clouddb "github.com/bluenviron/mediamtx/internal/cloud/db"
	"github.com/bluenviron/mediamtx/internal/cloud/integrations/pdk"
)

func newWebhookTestService(t *testing.T) *pdk.Service {
	t.Helper()
	dir := t.TempDir()
	dsn := "sqlite://" + filepath.Join(dir, "cloud.db")
	db, err := clouddb.Open(context.Background(), dsn)
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })

	var whSeq int
	svc, err := pdk.NewService(pdk.Config{
		DB: db,
		IDGen: func() string {
			whSeq++
			return fmt.Sprintf("wh-%04d", whSeq)
		},
	})
	if err != nil {
		t.Fatalf("new service: %v", err)
	}
	return svc
}

func signPayload(body, secret string) string {
	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write([]byte(body))
	return hex.EncodeToString(mac.Sum(nil))
}

func TestWebhookHandler_Success(t *testing.T) {
	svc := newWebhookTestService(t)
	ctx := context.Background()

	// Set up tenant config with webhook secret.
	svc.UpsertConfig(ctx, pdk.IntegrationConfig{
		TenantID:      "tenant-1",
		APIEndpoint:   "https://pdk.example.com",
		WebhookSecret: "test-secret",
		Enabled:       true,
		Status:        pdk.StatusConnected,
	})

	handler := pdk.NewWebhookHandler(svc)

	body := `{"event_id":"ev-1","panel_id":"p1","door_id":"d1","event_type":"access.granted","person_name":"Bob","credential":"card-100","timestamp":"2026-04-10T12:00:00Z"}`
	sig := signPayload(body, "test-secret")

	req := httptest.NewRequest(http.MethodPost, "/webhook/pdk", strings.NewReader(body))
	req.Header.Set("X-PDK-Tenant", "tenant-1")
	req.Header.Set("X-PDK-Signature", sig)
	req.Header.Set("Content-Type", "application/json")

	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rr.Code, rr.Body.String())
	}

	// Verify the event was stored.
	events, err := svc.ListEvents(ctx, "tenant-1", 10)
	if err != nil {
		t.Fatalf("list events: %v", err)
	}
	if len(events) != 1 {
		t.Fatalf("expected 1 event, got %d", len(events))
	}
	if events[0].PersonName != "Bob" {
		t.Errorf("expected Bob, got %s", events[0].PersonName)
	}
}

func TestWebhookHandler_InvalidSignature(t *testing.T) {
	svc := newWebhookTestService(t)
	ctx := context.Background()

	svc.UpsertConfig(ctx, pdk.IntegrationConfig{
		TenantID:      "tenant-1",
		WebhookSecret: "correct-secret",
		Enabled:       true,
		Status:        pdk.StatusConnected,
	})

	handler := pdk.NewWebhookHandler(svc)

	body := `{"event_id":"ev-1"}`
	req := httptest.NewRequest(http.MethodPost, "/webhook/pdk", strings.NewReader(body))
	req.Header.Set("X-PDK-Tenant", "tenant-1")
	req.Header.Set("X-PDK-Signature", "wrong-signature")

	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)

	if rr.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401, got %d", rr.Code)
	}
}

func TestWebhookHandler_MissingTenant(t *testing.T) {
	svc := newWebhookTestService(t)
	handler := pdk.NewWebhookHandler(svc)

	req := httptest.NewRequest(http.MethodPost, "/webhook/pdk", strings.NewReader("{}"))
	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)

	if rr.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", rr.Code)
	}
}

func TestWebhookHandler_MethodNotAllowed(t *testing.T) {
	svc := newWebhookTestService(t)
	handler := pdk.NewWebhookHandler(svc)

	req := httptest.NewRequest(http.MethodGet, "/webhook/pdk", nil)
	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)

	if rr.Code != http.StatusMethodNotAllowed {
		t.Fatalf("expected 405, got %d", rr.Code)
	}
}
