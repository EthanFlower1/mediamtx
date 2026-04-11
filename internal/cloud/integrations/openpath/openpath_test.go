package openpath

import (
	"bytes"
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

// -----------------------------------------------------------------------
// Mock HTTP transport
// -----------------------------------------------------------------------

// roundTripFunc implements http.RoundTripper for test doubles.
type roundTripFunc func(*http.Request) (*http.Response, error)

func (f roundTripFunc) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }

// mockHTTPClient wraps roundTripFunc to implement HTTPClient.
type mockHTTPClient struct {
	fn roundTripFunc
}

func (m *mockHTTPClient) Do(req *http.Request) (*http.Response, error) {
	return m.fn.RoundTrip(req)
}

func newMockHTTPClient(fn roundTripFunc) *mockHTTPClient {
	return &mockHTTPClient{fn: fn}
}

// -----------------------------------------------------------------------
// Test helpers
// -----------------------------------------------------------------------

func testConfig() Config {
	return Config{
		TenantID:      "tenant-1",
		OrgID:         "org-42",
		ClientID:      "client-id",
		ClientSecret:  "client-secret",
		BaseURL:       "https://mock.alta.local",
		WebhookSecret: "webhook-secret-key",
		DoorCameraMappings: []DoorCameraMapping{
			{DoorID: "door-A", DoorName: "Main Entrance", CameraPaths: []string{"cam/lobby", "cam/entrance"}},
			{DoorID: "door-B", DoorName: "Server Room", CameraPaths: []string{"cam/server-room"}},
		},
		Enabled: true,
	}
}

func signPayload(body []byte, secret string) string {
	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write(body)
	return hex.EncodeToString(mac.Sum(nil))
}

func altaTokenHandler(w http.ResponseWriter, _ *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	_, _ = w.Write([]byte(`{"access_token":"test-token","token_type":"Bearer","expires_in":3600}`))
}

// -----------------------------------------------------------------------
// Config validation
// -----------------------------------------------------------------------

func TestConfig_Validate(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name    string
		mutate  func(*Config)
		wantErr string
	}{
		{"valid", func(_ *Config) {}, ""},
		{"missing tenant_id", func(c *Config) { c.TenantID = "" }, "tenant_id"},
		{"missing org_id", func(c *Config) { c.OrgID = "" }, "org_id"},
		{"missing client_id", func(c *Config) { c.ClientID = "" }, "client_id"},
		{"missing client_secret", func(c *Config) { c.ClientSecret = "" }, "client_secret"},
		{"missing webhook_secret", func(c *Config) { c.WebhookSecret = "" }, "webhook_secret"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			cfg := testConfig()
			tt.mutate(&cfg)
			err := cfg.Validate()
			if tt.wantErr == "" {
				if err != nil {
					t.Fatalf("unexpected error: %v", err)
				}
			} else {
				if err == nil {
					t.Fatal("expected error, got nil")
				}
				if !strings.Contains(err.Error(), tt.wantErr) {
					t.Fatalf("error %q does not contain %q", err.Error(), tt.wantErr)
				}
			}
		})
	}
}

func TestConfig_EffectiveBaseURL(t *testing.T) {
	t.Parallel()
	cfg := testConfig()
	cfg.BaseURL = ""
	if got := cfg.EffectiveBaseURL(); got != DefaultBaseURL {
		t.Fatalf("expected default URL, got %q", got)
	}
	cfg.BaseURL = "https://custom.local"
	if got := cfg.EffectiveBaseURL(); got != "https://custom.local" {
		t.Fatalf("expected custom URL, got %q", got)
	}
}

// -----------------------------------------------------------------------
// Token validation
// -----------------------------------------------------------------------

func TestToken_Valid(t *testing.T) {
	t.Parallel()
	tok := Token{AccessToken: "abc", ExpiresAt: time.Now().Add(5 * time.Minute)}
	if !tok.Valid() {
		t.Fatal("expected valid token")
	}

	expired := Token{AccessToken: "abc", ExpiresAt: time.Now().Add(-1 * time.Minute)}
	if expired.Valid() {
		t.Fatal("expected expired token to be invalid")
	}

	empty := Token{}
	if empty.Valid() {
		t.Fatal("expected empty token to be invalid")
	}

	// Token expiring within 30s should be invalid (buffer).
	almostExpired := Token{AccessToken: "abc", ExpiresAt: time.Now().Add(20 * time.Second)}
	if almostExpired.Valid() {
		t.Fatal("expected near-expiry token to be invalid")
	}
}

// -----------------------------------------------------------------------
// Client: OAuth2 authentication
// -----------------------------------------------------------------------

func TestClient_Authenticate(t *testing.T) {
	t.Parallel()
	cfg := testConfig()

	calls := 0
	mock := newMockHTTPClient(func(req *http.Request) (*http.Response, error) {
		calls++
		if req.URL.Path != "/auth/oauth2/token" {
			t.Fatalf("unexpected path: %s", req.URL.Path)
		}
		if req.Method != http.MethodPost {
			t.Fatalf("expected POST, got %s", req.Method)
		}
		body, _ := io.ReadAll(req.Body)
		if !strings.Contains(string(body), "grant_type=client_credentials") {
			t.Fatalf("missing grant_type in body: %s", body)
		}
		rec := httptest.NewRecorder()
		altaTokenHandler(rec, req)
		return rec.Result(), nil
	})

	client := NewClient(cfg, mock, nil)
	tok, err := client.Authenticate(context.Background())
	if err != nil {
		t.Fatalf("authenticate failed: %v", err)
	}
	if tok.AccessToken != "test-token" {
		t.Fatalf("unexpected token: %q", tok.AccessToken)
	}
	if calls != 1 {
		t.Fatalf("expected 1 HTTP call, got %d", calls)
	}

	// Second call should use cached token.
	tok2, err := client.Authenticate(context.Background())
	if err != nil {
		t.Fatalf("second authenticate failed: %v", err)
	}
	if tok2.AccessToken != "test-token" {
		t.Fatalf("unexpected cached token: %q", tok2.AccessToken)
	}
	if calls != 1 {
		t.Fatalf("expected cached, got %d HTTP calls", calls)
	}
}

// -----------------------------------------------------------------------
// Client: ListDoors
// -----------------------------------------------------------------------

func TestClient_ListDoors(t *testing.T) {
	t.Parallel()
	cfg := testConfig()

	mock := newMockHTTPClient(func(req *http.Request) (*http.Response, error) {
		switch {
		case req.URL.Path == "/auth/oauth2/token":
			rec := httptest.NewRecorder()
			altaTokenHandler(rec, req)
			return rec.Result(), nil
		case req.URL.Path == fmt.Sprintf("/orgs/%s/doors", cfg.OrgID):
			rec := httptest.NewRecorder()
			rec.Header().Set("Content-Type", "application/json")
			_, _ = rec.WriteString(`{"data":[{"id":"door-A","name":"Main Entrance"},{"id":"door-B","name":"Server Room"}]}`)
			return rec.Result(), nil
		default:
			t.Fatalf("unexpected request: %s %s", req.Method, req.URL.Path)
			return nil, nil
		}
	})

	client := NewClient(cfg, mock, nil)
	doors, err := client.ListDoors(context.Background())
	if err != nil {
		t.Fatalf("list doors failed: %v", err)
	}
	if len(doors) != 2 {
		t.Fatalf("expected 2 doors, got %d", len(doors))
	}
	if doors[0].ID != "door-A" {
		t.Fatalf("unexpected door ID: %s", doors[0].ID)
	}
}

// -----------------------------------------------------------------------
// Client: retry on 5xx
// -----------------------------------------------------------------------

func TestClient_RetryOn5xx(t *testing.T) {
	t.Parallel()
	cfg := testConfig()

	attempt := 0
	mock := newMockHTTPClient(func(req *http.Request) (*http.Response, error) {
		if req.URL.Path == "/auth/oauth2/token" {
			rec := httptest.NewRecorder()
			altaTokenHandler(rec, req)
			return rec.Result(), nil
		}
		attempt++
		if attempt <= 2 {
			return &http.Response{
				StatusCode: http.StatusServiceUnavailable,
				Body:       io.NopCloser(strings.NewReader("service unavailable")),
			}, nil
		}
		rec := httptest.NewRecorder()
		rec.Header().Set("Content-Type", "application/json")
		_, _ = rec.WriteString(`{"data":[]}`)
		return rec.Result(), nil
	})

	client := NewClient(cfg, mock, nil)
	doors, err := client.ListDoors(context.Background())
	if err != nil {
		t.Fatalf("expected retry to succeed, got: %v", err)
	}
	if attempt != 3 {
		t.Fatalf("expected 3 attempts, got %d", attempt)
	}
	if len(doors) != 0 {
		t.Fatalf("expected 0 doors, got %d", len(doors))
	}
}

// -----------------------------------------------------------------------
// Client: TriggerLockdown
// -----------------------------------------------------------------------

func TestClient_TriggerLockdown(t *testing.T) {
	t.Parallel()
	cfg := testConfig()

	var capturedBody map[string]string
	mock := newMockHTTPClient(func(req *http.Request) (*http.Response, error) {
		if req.URL.Path == "/auth/oauth2/token" {
			rec := httptest.NewRecorder()
			altaTokenHandler(rec, req)
			return rec.Result(), nil
		}
		expectedPath := fmt.Sprintf("/orgs/%s/doors/door-A/actions", cfg.OrgID)
		if req.URL.Path != expectedPath {
			t.Fatalf("unexpected path: %s (expected %s)", req.URL.Path, expectedPath)
		}
		body, _ := io.ReadAll(req.Body)
		_ = json.Unmarshal(body, &capturedBody)
		return &http.Response{
			StatusCode: http.StatusNoContent,
			Body:       io.NopCloser(strings.NewReader("")),
		}, nil
	})

	client := NewClient(cfg, mock, nil)
	err := client.TriggerLockdown(context.Background(), LockdownRequest{
		TenantID: cfg.TenantID,
		OrgID:    cfg.OrgID,
		DoorID:   "door-A",
		Reason:   "motion detected",
	})
	if err != nil {
		t.Fatalf("lockdown failed: %v", err)
	}
	if capturedBody["action"] != "lockdown" {
		t.Fatalf("expected lockdown action, got %q", capturedBody["action"])
	}
	if capturedBody["reason"] != "motion detected" {
		t.Fatalf("unexpected reason: %q", capturedBody["reason"])
	}
}

// -----------------------------------------------------------------------
// Service: webhook handling + correlation
// -----------------------------------------------------------------------

func TestService_WebhookHandling(t *testing.T) {
	t.Parallel()
	store := NewMemoryStore()
	svc, err := NewService(ServiceConfig{Store: store})
	if err != nil {
		t.Fatalf("new service: %v", err)
	}

	cfg := testConfig()
	// Use RegisterWithClient to skip real OAuth.
	mock := newMockHTTPClient(func(_ *http.Request) (*http.Response, error) {
		return nil, fmt.Errorf("should not be called")
	})
	client := NewClient(cfg, mock, nil)
	// Pre-set a valid token to avoid HTTP calls.
	client.token = Token{
		AccessToken: "test",
		TokenType:   "Bearer",
		ExpiresAt:   time.Now().Add(1 * time.Hour),
	}
	svc.RegisterWithClient(cfg, client)

	// Build webhook payload.
	payload := map[string]string{
		"event_id":   "evt-001",
		"org_id":     "org-42",
		"door_id":    "door-A",
		"door_name":  "Main Entrance",
		"event_type": "unlock",
		"user_name":  "alice",
		"timestamp":  time.Now().UTC().Format(time.RFC3339),
	}
	body, _ := json.Marshal(payload)
	sig := signPayload(body, cfg.WebhookSecret)

	req := httptest.NewRequest(http.MethodPost,
		"/api/v1/integrations/openpath/webhook/tenant-1?tenant_id=tenant-1",
		bytes.NewReader(body),
	)
	req.Header.Set("X-OpenPath-Signature", sig)
	rec := httptest.NewRecorder()
	svc.HandleWebhook(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}

	// Wait briefly for the async correlation goroutine.
	time.Sleep(100 * time.Millisecond)

	events := store.Events()
	if len(events) != 1 {
		t.Fatalf("expected 1 event, got %d", len(events))
	}
	if events[0].ID != "evt-001" {
		t.Fatalf("unexpected event ID: %s", events[0].ID)
	}
	if events[0].Type != DoorEventUnlock {
		t.Fatalf("unexpected event type: %s", events[0].Type)
	}

	clips := store.Clips()
	if len(clips) != 2 { // door-A maps to cam/lobby + cam/entrance
		t.Fatalf("expected 2 correlated clips, got %d", len(clips))
	}
	if clips[0].DoorEventID != "evt-001" {
		t.Fatalf("unexpected clip event ID: %s", clips[0].DoorEventID)
	}
}

func TestService_WebhookInvalidSignature(t *testing.T) {
	t.Parallel()
	store := NewMemoryStore()
	svc, err := NewService(ServiceConfig{Store: store})
	if err != nil {
		t.Fatal(err)
	}

	cfg := testConfig()
	mock := newMockHTTPClient(func(_ *http.Request) (*http.Response, error) {
		return nil, nil
	})
	client := NewClient(cfg, mock, nil)
	client.token = Token{AccessToken: "t", TokenType: "Bearer", ExpiresAt: time.Now().Add(1 * time.Hour)}
	svc.RegisterWithClient(cfg, client)

	body := []byte(`{"event_id":"evt-002","door_id":"door-A","event_type":"unlock","timestamp":"2026-01-01T00:00:00Z"}`)
	req := httptest.NewRequest(http.MethodPost,
		"/api/v1/integrations/openpath/webhook/tenant-1?tenant_id=tenant-1",
		bytes.NewReader(body),
	)
	req.Header.Set("X-OpenPath-Signature", "bad-signature")
	rec := httptest.NewRecorder()
	svc.HandleWebhook(rec, req)

	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401, got %d", rec.Code)
	}
}

func TestService_WebhookUnknownTenant(t *testing.T) {
	t.Parallel()
	store := NewMemoryStore()
	svc, err := NewService(ServiceConfig{Store: store})
	if err != nil {
		t.Fatal(err)
	}

	req := httptest.NewRequest(http.MethodPost,
		"/api/v1/integrations/openpath/webhook/unknown-tenant?tenant_id=unknown-tenant",
		strings.NewReader(`{}`),
	)
	rec := httptest.NewRecorder()
	svc.HandleWebhook(rec, req)

	if rec.Code != http.StatusNotFound {
		t.Fatalf("expected 404, got %d", rec.Code)
	}
}

// -----------------------------------------------------------------------
// Service: idempotent event storage
// -----------------------------------------------------------------------

func TestService_IdempotentEventStorage(t *testing.T) {
	t.Parallel()
	store := NewMemoryStore()
	svc, err := NewService(ServiceConfig{Store: store})
	if err != nil {
		t.Fatal(err)
	}

	cfg := testConfig()
	mock := newMockHTTPClient(func(_ *http.Request) (*http.Response, error) { return nil, nil })
	client := NewClient(cfg, mock, nil)
	client.token = Token{AccessToken: "t", TokenType: "Bearer", ExpiresAt: time.Now().Add(1 * time.Hour)}
	svc.RegisterWithClient(cfg, client)

	payload, _ := json.Marshal(map[string]string{
		"event_id":   "evt-dup",
		"door_id":    "door-B",
		"event_type": "denied",
		"timestamp":  time.Now().UTC().Format(time.RFC3339),
	})
	sig := signPayload(payload, cfg.WebhookSecret)

	for i := 0; i < 3; i++ {
		req := httptest.NewRequest(http.MethodPost,
			"/api/v1/integrations/openpath/webhook/tenant-1?tenant_id=tenant-1",
			bytes.NewReader(payload),
		)
		req.Header.Set("X-OpenPath-Signature", sig)
		rec := httptest.NewRecorder()
		svc.HandleWebhook(rec, req)
		if rec.Code != http.StatusOK {
			t.Fatalf("attempt %d: expected 200, got %d", i, rec.Code)
		}
	}

	time.Sleep(100 * time.Millisecond)
	events := store.Events()
	if len(events) != 1 {
		t.Fatalf("expected 1 event (idempotent), got %d", len(events))
	}
}

// -----------------------------------------------------------------------
// Service: TriggerLockdown integration
// -----------------------------------------------------------------------

func TestService_TriggerLockdown(t *testing.T) {
	t.Parallel()
	store := NewMemoryStore()
	svc, err := NewService(ServiceConfig{Store: store})
	if err != nil {
		t.Fatal(err)
	}

	cfg := testConfig()
	lockdownCalled := false
	mock := newMockHTTPClient(func(req *http.Request) (*http.Response, error) {
		if req.URL.Path == "/auth/oauth2/token" {
			rec := httptest.NewRecorder()
			altaTokenHandler(rec, req)
			return rec.Result(), nil
		}
		lockdownCalled = true
		return &http.Response{
			StatusCode: http.StatusNoContent,
			Body:       io.NopCloser(strings.NewReader("")),
		}, nil
	})

	client := NewClient(cfg, mock, nil)
	svc.RegisterWithClient(cfg, client)

	err = svc.TriggerLockdown(context.Background(), LockdownRequest{
		TenantID: "tenant-1",
		DoorID:   "door-A",
		Reason:   "intrusion detected",
	})
	if err != nil {
		t.Fatalf("lockdown failed: %v", err)
	}
	if !lockdownCalled {
		t.Fatal("expected lockdown API call")
	}
}

// -----------------------------------------------------------------------
// Service: ListDoorEvents
// -----------------------------------------------------------------------

func TestService_ListDoorEvents(t *testing.T) {
	t.Parallel()
	store := NewMemoryStore()
	now := time.Now().UTC()

	// Seed events.
	_ = store.SaveDoorEvent(context.Background(), DoorEvent{
		ID: "ev1", TenantID: "tenant-1", DoorID: "door-A", Type: DoorEventUnlock,
		Timestamp: now.Add(-10 * time.Minute),
	})
	_ = store.SaveDoorEvent(context.Background(), DoorEvent{
		ID: "ev2", TenantID: "tenant-1", DoorID: "door-B", Type: DoorEventDenied,
		Timestamp: now.Add(-5 * time.Minute),
	})
	_ = store.SaveDoorEvent(context.Background(), DoorEvent{
		ID: "ev3", TenantID: "tenant-2", DoorID: "door-A", Type: DoorEventUnlock,
		Timestamp: now.Add(-3 * time.Minute),
	})

	svc, _ := NewService(ServiceConfig{Store: store})

	events, err := svc.ListDoorEvents(context.Background(), "tenant-1",
		now.Add(-15*time.Minute), now)
	if err != nil {
		t.Fatalf("list events failed: %v", err)
	}
	if len(events) != 2 {
		t.Fatalf("expected 2 events for tenant-1, got %d", len(events))
	}
	// Should be newest first.
	if events[0].ID != "ev2" {
		t.Fatalf("expected newest first, got %s", events[0].ID)
	}
}

// -----------------------------------------------------------------------
// Service: ListDoors via Alta API
// -----------------------------------------------------------------------

func TestService_ListDoors(t *testing.T) {
	t.Parallel()
	store := NewMemoryStore()
	svc, _ := NewService(ServiceConfig{Store: store})

	cfg := testConfig()
	mock := newMockHTTPClient(func(req *http.Request) (*http.Response, error) {
		if req.URL.Path == "/auth/oauth2/token" {
			rec := httptest.NewRecorder()
			altaTokenHandler(rec, req)
			return rec.Result(), nil
		}
		rec := httptest.NewRecorder()
		_, _ = rec.WriteString(`{"data":[{"id":"d1","name":"Door 1"}]}`)
		return rec.Result(), nil
	})
	client := NewClient(cfg, mock, nil)
	svc.RegisterWithClient(cfg, client)

	doors, err := svc.ListDoors(context.Background(), "tenant-1")
	if err != nil {
		t.Fatalf("list doors failed: %v", err)
	}
	if len(doors) != 1 || doors[0].ID != "d1" {
		t.Fatalf("unexpected doors: %+v", doors)
	}
}

// -----------------------------------------------------------------------
// MemoryStore: basics
// -----------------------------------------------------------------------

func TestMemoryStore_SaveAndList(t *testing.T) {
	t.Parallel()
	store := NewMemoryStore()
	ctx := context.Background()
	now := time.Now().UTC()

	_ = store.SaveDoorEvent(ctx, DoorEvent{ID: "a", TenantID: "t1", Timestamp: now})
	_ = store.SaveDoorEvent(ctx, DoorEvent{ID: "b", TenantID: "t1", Timestamp: now.Add(time.Minute)})
	_ = store.SaveDoorEvent(ctx, DoorEvent{ID: "a", TenantID: "t1", Timestamp: now}) // dup

	events, _ := store.ListDoorEvents(ctx, "t1", now.Add(-time.Hour), now.Add(time.Hour))
	if len(events) != 2 {
		t.Fatalf("expected 2 (deduped), got %d", len(events))
	}

	_ = store.SaveCorrelatedClip(ctx, CorrelatedClip{DoorEventID: "a", CameraPath: "cam/1"})
	clips := store.Clips()
	if len(clips) != 1 {
		t.Fatalf("expected 1 clip, got %d", len(clips))
	}
}

// -----------------------------------------------------------------------
// Door-camera mapping
// -----------------------------------------------------------------------

func TestService_CamerasForDoor(t *testing.T) {
	t.Parallel()
	svc := &Service{}

	mappings := []DoorCameraMapping{
		{DoorID: "d1", CameraPaths: []string{"cam/a", "cam/b"}},
		{DoorID: "d2", CameraPaths: []string{"cam/c"}},
	}

	got := svc.camerasForDoor(mappings, "d1")
	if len(got) != 2 {
		t.Fatalf("expected 2 cameras, got %d", len(got))
	}

	got = svc.camerasForDoor(mappings, "d-unknown")
	if len(got) != 0 {
		t.Fatalf("expected 0 cameras, got %d", len(got))
	}
}

// -----------------------------------------------------------------------
// Handler: connect endpoint
// -----------------------------------------------------------------------

func newTestServiceWithClient(t *testing.T) (*Service, Config) {
	t.Helper()
	store := NewMemoryStore()
	svc, err := NewService(ServiceConfig{Store: store})
	if err != nil {
		t.Fatalf("new service: %v", err)
	}

	cfg := testConfig()
	mock := newMockHTTPClient(func(req *http.Request) (*http.Response, error) {
		if req.URL.Path == "/auth/oauth2/token" {
			rec := httptest.NewRecorder()
			altaTokenHandler(rec, req)
			return rec.Result(), nil
		}
		rec := httptest.NewRecorder()
		rec.Header().Set("Content-Type", "application/json")
		_, _ = rec.WriteString(`{"data":[]}`)
		return rec.Result(), nil
	})
	client := NewClient(cfg, mock, nil)
	svc.RegisterWithClient(cfg, client)

	return svc, cfg
}

func TestHandler_ConnectionStatus(t *testing.T) {
	t.Parallel()
	svc, _ := newTestServiceWithClient(t)
	h := NewHandler(HandlerConfig{
		Service: svc,
		Tenant: func(r *http.Request) string {
			return "tenant-1"
		},
	})

	req := httptest.NewRequest(http.MethodGet, "/integrations/openpath/connection", nil)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}

	var resp map[string]any
	_ = json.Unmarshal(rec.Body.Bytes(), &resp)
	if resp["status"] != "connected" {
		t.Fatalf("expected connected, got %v", resp["status"])
	}
}

func TestHandler_ConnectionStatus_NotFound(t *testing.T) {
	t.Parallel()
	store := NewMemoryStore()
	svc, _ := NewService(ServiceConfig{Store: store})
	h := NewHandler(HandlerConfig{
		Service: svc,
		Tenant: func(r *http.Request) string {
			return "unknown-tenant"
		},
	})

	req := httptest.NewRequest(http.MethodGet, "/integrations/openpath/connection", nil)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)

	if rec.Code != http.StatusNotFound {
		t.Fatalf("expected 404, got %d", rec.Code)
	}
}

func TestHandler_Disconnect(t *testing.T) {
	t.Parallel()
	svc, _ := newTestServiceWithClient(t)
	h := NewHandler(HandlerConfig{
		Service: svc,
		Tenant: func(r *http.Request) string {
			return "tenant-1"
		},
	})

	req := httptest.NewRequest(http.MethodDelete, "/integrations/openpath/connection", nil)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rec.Code)
	}

	// Verify disconnected.
	req2 := httptest.NewRequest(http.MethodGet, "/integrations/openpath/connection", nil)
	rec2 := httptest.NewRecorder()
	h.ServeHTTP(rec2, req2)
	if rec2.Code != http.StatusNotFound {
		t.Fatalf("expected 404 after disconnect, got %d", rec2.Code)
	}
}

func TestHandler_Mappings(t *testing.T) {
	t.Parallel()
	svc, _ := newTestServiceWithClient(t)
	h := NewHandler(HandlerConfig{
		Service: svc,
		Tenant: func(r *http.Request) string {
			return "tenant-1"
		},
	})

	// Create mapping.
	mapping := `{"door_id":"door-C","door_name":"Parking","camera_paths":["cam/parking"]}`
	req := httptest.NewRequest(http.MethodPost, "/integrations/openpath/mappings",
		strings.NewReader(mapping))
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("create mapping: expected 200, got %d: %s", rec.Code, rec.Body.String())
	}

	// List mappings — should include original 2 + new one.
	req2 := httptest.NewRequest(http.MethodGet, "/integrations/openpath/mappings", nil)
	rec2 := httptest.NewRecorder()
	h.ServeHTTP(rec2, req2)

	if rec2.Code != http.StatusOK {
		t.Fatalf("list mappings: expected 200, got %d", rec2.Code)
	}
	var mappings []DoorCameraMapping
	_ = json.Unmarshal(rec2.Body.Bytes(), &mappings)
	if len(mappings) != 3 {
		t.Fatalf("expected 3 mappings, got %d", len(mappings))
	}

	// Delete mapping.
	req3 := httptest.NewRequest(http.MethodDelete, "/integrations/openpath/mappings/door-C", nil)
	rec3 := httptest.NewRecorder()
	h.ServeHTTP(rec3, req3)
	if rec3.Code != http.StatusOK {
		t.Fatalf("delete mapping: expected 200, got %d", rec3.Code)
	}
}

func TestHandler_Lockdown(t *testing.T) {
	t.Parallel()
	store := NewMemoryStore()
	svc, _ := NewService(ServiceConfig{Store: store})
	cfg := testConfig()

	lockdownCalled := false
	mock := newMockHTTPClient(func(req *http.Request) (*http.Response, error) {
		if req.URL.Path == "/auth/oauth2/token" {
			rec := httptest.NewRecorder()
			altaTokenHandler(rec, req)
			return rec.Result(), nil
		}
		lockdownCalled = true
		return &http.Response{
			StatusCode: http.StatusNoContent,
			Body:       io.NopCloser(strings.NewReader("")),
		}, nil
	})
	client := NewClient(cfg, mock, nil)
	svc.RegisterWithClient(cfg, client)

	h := NewHandler(HandlerConfig{
		Service: svc,
		Tenant: func(r *http.Request) string {
			return "tenant-1"
		},
	})

	body := `{"door_id":"door-A","reason":"suspicious motion"}`
	req := httptest.NewRequest(http.MethodPost, "/integrations/openpath/lockdown",
		strings.NewReader(body))
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}
	if !lockdownCalled {
		t.Fatal("expected lockdown API call")
	}
}

func TestHandler_Events(t *testing.T) {
	t.Parallel()
	store := NewMemoryStore()
	now := time.Now().UTC()
	_ = store.SaveDoorEvent(context.Background(), DoorEvent{
		ID: "ev-h1", TenantID: "tenant-1", DoorID: "door-A",
		Type: DoorEventUnlock, Timestamp: now.Add(-1 * time.Hour),
	})

	svc, _ := NewService(ServiceConfig{Store: store})
	cfg := testConfig()
	mock := newMockHTTPClient(func(_ *http.Request) (*http.Response, error) { return nil, nil })
	client := NewClient(cfg, mock, nil)
	client.token = Token{AccessToken: "t", TokenType: "Bearer", ExpiresAt: time.Now().Add(1 * time.Hour)}
	svc.RegisterWithClient(cfg, client)

	h := NewHandler(HandlerConfig{
		Service: svc,
		Tenant: func(r *http.Request) string {
			return "tenant-1"
		},
	})

	req := httptest.NewRequest(http.MethodGet, "/integrations/openpath/events", nil)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}
	var events []DoorEvent
	_ = json.Unmarshal(rec.Body.Bytes(), &events)
	if len(events) != 1 {
		t.Fatalf("expected 1 event, got %d", len(events))
	}
}

// -----------------------------------------------------------------------
// Registry
// -----------------------------------------------------------------------

func TestIntegrationMeta(t *testing.T) {
	t.Parallel()
	meta := IntegrationMeta()
	if meta.ID != "openpath" {
		t.Fatalf("expected ID 'openpath', got %q", meta.ID)
	}
	if meta.Category != "access_control" {
		t.Fatalf("expected category 'access_control', got %q", meta.Category)
	}
	if !meta.Bidirectional {
		t.Fatal("expected bidirectional=true")
	}
}

// -----------------------------------------------------------------------
// Service: SetDoorCameraMapping / DeleteDoorCameraMapping
// -----------------------------------------------------------------------

func TestService_SetDoorCameraMapping(t *testing.T) {
	t.Parallel()
	svc, _ := newTestServiceWithClient(t)

	// Add a new mapping.
	err := svc.SetDoorCameraMapping("tenant-1", DoorCameraMapping{
		DoorID: "door-new", DoorName: "New Door", CameraPaths: []string{"cam/new"},
	})
	if err != nil {
		t.Fatalf("set mapping: %v", err)
	}

	ts, _ := svc.tenant("tenant-1")
	if len(ts.cfg.DoorCameraMappings) != 3 { // 2 original + 1 new
		t.Fatalf("expected 3 mappings, got %d", len(ts.cfg.DoorCameraMappings))
	}

	// Update existing mapping.
	err = svc.SetDoorCameraMapping("tenant-1", DoorCameraMapping{
		DoorID: "door-A", DoorName: "Updated", CameraPaths: []string{"cam/updated"},
	})
	if err != nil {
		t.Fatalf("update mapping: %v", err)
	}
	ts, _ = svc.tenant("tenant-1")
	if len(ts.cfg.DoorCameraMappings) != 3 { // should still be 3
		t.Fatalf("expected 3 mappings after update, got %d", len(ts.cfg.DoorCameraMappings))
	}

	// Unknown tenant.
	err = svc.SetDoorCameraMapping("unknown", DoorCameraMapping{DoorID: "d"})
	if err == nil {
		t.Fatal("expected error for unknown tenant")
	}
}

func TestService_DeleteDoorCameraMapping(t *testing.T) {
	t.Parallel()
	svc, _ := newTestServiceWithClient(t)

	err := svc.DeleteDoorCameraMapping("tenant-1", "door-A")
	if err != nil {
		t.Fatalf("delete mapping: %v", err)
	}

	ts, _ := svc.tenant("tenant-1")
	if len(ts.cfg.DoorCameraMappings) != 1 { // was 2, now 1
		t.Fatalf("expected 1 mapping after delete, got %d", len(ts.cfg.DoorCameraMappings))
	}

	// Delete non-existent is a no-op.
	err = svc.DeleteDoorCameraMapping("tenant-1", "non-existent")
	if err != nil {
		t.Fatalf("delete non-existent should not error: %v", err)
	}
}
