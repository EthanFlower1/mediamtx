package brivo

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"strings"
	"testing"
	"time"
)

// ---------------------------------------------------------------------------
// Mock BrivoAPIClient
// ---------------------------------------------------------------------------

type mockAPIClient struct {
	exchangeCodeFn  func(ctx context.Context, code, verifier string) (TokenPair, error)
	refreshTokenFn  func(ctx context.Context, refreshToken string) (TokenPair, error)
	listSitesFn     func(ctx context.Context, token string) ([]BrivoSite, error)
	listDoorsFn     func(ctx context.Context, token, siteID string) ([]BrivoDoor, error)
	sendEventFn     func(ctx context.Context, token string, event NVREvent) error
}

func (m *mockAPIClient) ExchangeCode(ctx context.Context, code, verifier string) (TokenPair, error) {
	if m.exchangeCodeFn != nil {
		return m.exchangeCodeFn(ctx, code, verifier)
	}
	return TokenPair{
		AccessToken:  "access-tok",
		RefreshToken: "refresh-tok",
		TokenType:    "Bearer",
		ExpiresAt:    time.Now().Add(time.Hour),
		Scope:        "read write",
	}, nil
}

func (m *mockAPIClient) RefreshToken(ctx context.Context, refreshToken string) (TokenPair, error) {
	if m.refreshTokenFn != nil {
		return m.refreshTokenFn(ctx, refreshToken)
	}
	return TokenPair{
		AccessToken:  "new-access-tok",
		RefreshToken: "new-refresh-tok",
		TokenType:    "Bearer",
		ExpiresAt:    time.Now().Add(time.Hour),
		Scope:        "read write",
	}, nil
}

func (m *mockAPIClient) ListSites(ctx context.Context, token string) ([]BrivoSite, error) {
	if m.listSitesFn != nil {
		return m.listSitesFn(ctx, token)
	}
	return []BrivoSite{{ID: "site-1", Name: "HQ"}}, nil
}

func (m *mockAPIClient) ListDoors(ctx context.Context, token, siteID string) ([]BrivoDoor, error) {
	if m.listDoorsFn != nil {
		return m.listDoorsFn(ctx, token, siteID)
	}
	return []BrivoDoor{
		{ID: "door-1", Name: "Main Entrance", SiteID: siteID, SiteName: "HQ"},
	}, nil
}

func (m *mockAPIClient) SendEvent(ctx context.Context, token string, event NVREvent) error {
	if m.sendEventFn != nil {
		return m.sendEventFn(ctx, token, event)
	}
	return nil
}

// ---------------------------------------------------------------------------
// Mock SnapshotService
// ---------------------------------------------------------------------------

type mockSnapshotService struct {
	captureFn func(ctx context.Context, tenantID, cameraID string, ts time.Time) (string, error)
}

func (m *mockSnapshotService) Capture(ctx context.Context, tenantID, cameraID string, ts time.Time) (string, error) {
	if m.captureFn != nil {
		return m.captureFn(ctx, tenantID, cameraID, ts)
	}
	return fmt.Sprintf("https://snapshots.example.com/%s/%s.jpg", tenantID, cameraID), nil
}

// ---------------------------------------------------------------------------
// Test helpers
// ---------------------------------------------------------------------------

var fixedTime = time.Date(2026, 4, 10, 12, 0, 0, 0, time.UTC)

func fixedClock() time.Time { return fixedTime }

type auditRecord struct {
	events []AuditEvent
}

func (a *auditRecord) hook() AuditHook {
	return func(_ context.Context, ev AuditEvent) {
		a.events = append(a.events, ev)
	}
}

func newTestService(t *testing.T, opts ...func(*Config)) (*Service, *mockAPIClient, *auditRecord) {
	t.Helper()
	api := &mockAPIClient{}
	audit := &auditRecord{}

	cfg := Config{
		OAuth: OAuthConfig{
			ClientID:      "test-client-id",
			ClientSecret:  "test-secret",
			AuthURL:       "https://auth.brivo.com/oauth/authorize",
			TokenURL:      "https://auth.brivo.com/oauth/token",
			RedirectURL:   "https://kaivue.example.com/integrations/brivo/callback",
			WebhookSecret: "webhook-secret-key",
		},
		Tokens:    NewMemoryTokenStore(),
		Conns:     NewMemoryConnectionStore(),
		Mappings:  NewMemoryMappingStore(),
		Events:    NewMemoryEventLog(),
		Snapshots: &mockSnapshotService{},
		API:       api,
		AuditHook: audit.hook(),
		Clock:     fixedClock,
	}

	for _, o := range opts {
		o(&cfg)
	}

	svc, err := NewService(cfg)
	if err != nil {
		t.Fatalf("NewService: %v", err)
	}
	return svc, api, audit
}

// ---------------------------------------------------------------------------
// TokenPair tests
// ---------------------------------------------------------------------------

func TestTokenPair_IsExpired(t *testing.T) {
	t.Run("not expired", func(t *testing.T) {
		tok := TokenPair{ExpiresAt: time.Now().Add(5 * time.Minute)}
		if tok.IsExpired() {
			t.Error("expected token not to be expired")
		}
	})

	t.Run("expired", func(t *testing.T) {
		tok := TokenPair{ExpiresAt: time.Now().Add(-5 * time.Minute)}
		if !tok.IsExpired() {
			t.Error("expected token to be expired")
		}
	})

	t.Run("within safety margin", func(t *testing.T) {
		tok := TokenPair{ExpiresAt: time.Now().Add(30 * time.Second)}
		if !tok.IsExpired() {
			t.Error("expected token within 60s margin to be expired")
		}
	})
}

// ---------------------------------------------------------------------------
// OAuth flow tests
// ---------------------------------------------------------------------------

func TestOAuth_BeginAuthorize(t *testing.T) {
	svc, _, _ := newTestService(t)
	url, err := svc.OAuth().BeginAuthorize(context.Background(), "tenant-1")
	if err != nil {
		t.Fatalf("BeginAuthorize: %v", err)
	}
	if !strings.HasPrefix(url, "https://auth.brivo.com/oauth/authorize?") {
		t.Errorf("unexpected URL prefix: %s", url)
	}
	if !strings.Contains(url, "code_challenge_method=S256") {
		t.Error("missing PKCE S256 challenge method")
	}
	if !strings.Contains(url, "client_id=test-client-id") {
		t.Error("missing client_id")
	}
	if !strings.Contains(url, "response_type=code") {
		t.Error("missing response_type=code")
	}
}

func TestOAuth_CompleteAuthorize(t *testing.T) {
	svc, _, audit := newTestService(t)
	ctx := context.Background()

	// Begin the flow to get a state token.
	url, err := svc.OAuth().BeginAuthorize(ctx, "tenant-1")
	if err != nil {
		t.Fatalf("BeginAuthorize: %v", err)
	}

	// Extract state from URL.
	state := extractQueryParam(url, "state")
	if state == "" {
		t.Fatal("no state param in authorize URL")
	}

	tok, err := svc.OAuth().CompleteAuthorize(ctx, state, "auth-code-123")
	if err != nil {
		t.Fatalf("CompleteAuthorize: %v", err)
	}
	if tok.AccessToken == "" {
		t.Error("expected non-empty access token")
	}

	// Verify audit event.
	found := false
	for _, e := range audit.events {
		if e.Action == "connect" && e.TenantID == "tenant-1" {
			found = true
		}
	}
	if !found {
		t.Error("missing connect audit event")
	}
}

func TestOAuth_CompleteAuthorize_InvalidState(t *testing.T) {
	svc, _, _ := newTestService(t)
	_, err := svc.OAuth().CompleteAuthorize(context.Background(), "bogus-state", "code")
	if err != ErrInvalidState {
		t.Errorf("expected ErrInvalidState, got: %v", err)
	}
}

func TestOAuth_CompleteAuthorize_ExpiredSession(t *testing.T) {
	// Use a clock that's 15 minutes ahead of session creation.
	callCount := 0
	svc, _, _ := newTestService(t, func(cfg *Config) {
		cfg.Clock = func() time.Time {
			callCount++
			// First call (session creation) returns base time.
			// Subsequent calls return 15 min later.
			if callCount <= 1 {
				return fixedTime
			}
			return fixedTime.Add(15 * time.Minute)
		}
	})
	ctx := context.Background()

	url, err := svc.OAuth().BeginAuthorize(ctx, "tenant-1")
	if err != nil {
		t.Fatalf("BeginAuthorize: %v", err)
	}

	state := extractQueryParam(url, "state")
	_, err = svc.OAuth().CompleteAuthorize(ctx, state, "code")
	if err != ErrInvalidState {
		t.Errorf("expected ErrInvalidState for expired session, got: %v", err)
	}
}

func TestOAuth_EnsureValidToken_NotExpired(t *testing.T) {
	svc, _, _ := newTestService(t)
	ctx := context.Background()

	// Pre-store a valid token.
	tok := TokenPair{
		AccessToken:  "valid-token",
		RefreshToken: "refresh",
		ExpiresAt:    time.Now().Add(time.Hour),
	}
	if err := svc.oauth.tokens.StoreToken(ctx, "tenant-1", tok); err != nil {
		t.Fatal(err)
	}

	got, err := svc.oauth.EnsureValidToken(ctx, "tenant-1")
	if err != nil {
		t.Fatalf("EnsureValidToken: %v", err)
	}
	if got != "valid-token" {
		t.Errorf("expected valid-token, got %s", got)
	}
}

func TestOAuth_EnsureValidToken_Refresh(t *testing.T) {
	refreshCalled := false
	svc, api, audit := newTestService(t)
	api.refreshTokenFn = func(_ context.Context, rt string) (TokenPair, error) {
		refreshCalled = true
		return TokenPair{
			AccessToken:  "refreshed-token",
			RefreshToken: "new-refresh",
			ExpiresAt:    time.Now().Add(time.Hour),
		}, nil
	}
	ctx := context.Background()

	// Store an expired token.
	tok := TokenPair{
		AccessToken:  "old-token",
		RefreshToken: "old-refresh",
		ExpiresAt:    time.Now().Add(-time.Hour),
	}
	if err := svc.oauth.tokens.StoreToken(ctx, "tenant-1", tok); err != nil {
		t.Fatal(err)
	}

	got, err := svc.oauth.EnsureValidToken(ctx, "tenant-1")
	if err != nil {
		t.Fatalf("EnsureValidToken: %v", err)
	}
	if !refreshCalled {
		t.Error("expected refresh to be called")
	}
	if got != "refreshed-token" {
		t.Errorf("expected refreshed-token, got %s", got)
	}

	// Verify audit.
	found := false
	for _, e := range audit.events {
		if e.Action == "token_refresh" {
			found = true
		}
	}
	if !found {
		t.Error("missing token_refresh audit event")
	}
}

func TestOAuth_EnsureValidToken_RefreshFails(t *testing.T) {
	svc, api, _ := newTestService(t)
	api.refreshTokenFn = func(_ context.Context, _ string) (TokenPair, error) {
		return TokenPair{}, fmt.Errorf("network error")
	}
	ctx := context.Background()

	tok := TokenPair{
		AccessToken:  "old",
		RefreshToken: "old-refresh",
		ExpiresAt:    time.Now().Add(-time.Hour),
	}
	_ = svc.oauth.tokens.StoreToken(ctx, "tenant-1", tok)

	_, err := svc.oauth.EnsureValidToken(ctx, "tenant-1")
	if err == nil {
		t.Fatal("expected error")
	}
	if !strings.Contains(err.Error(), "token expired") {
		t.Errorf("expected token expired error, got: %v", err)
	}
}

func TestOAuth_EnsureValidToken_NotConnected(t *testing.T) {
	svc, _, _ := newTestService(t)
	_, err := svc.oauth.EnsureValidToken(context.Background(), "unknown-tenant")
	if err != ErrNotConnected {
		t.Errorf("expected ErrNotConnected, got: %v", err)
	}
}

func TestOAuth_Disconnect(t *testing.T) {
	svc, _, audit := newTestService(t)
	ctx := context.Background()

	tok := TokenPair{AccessToken: "tok", RefreshToken: "ref", ExpiresAt: time.Now().Add(time.Hour)}
	_ = svc.oauth.tokens.StoreToken(ctx, "tenant-1", tok)

	if err := svc.oauth.Disconnect(ctx, "tenant-1"); err != nil {
		t.Fatalf("Disconnect: %v", err)
	}

	// Token should be gone.
	_, err := svc.oauth.tokens.GetToken(ctx, "tenant-1")
	if err != ErrNotConnected {
		t.Errorf("expected ErrNotConnected after disconnect, got: %v", err)
	}

	found := false
	for _, e := range audit.events {
		if e.Action == "disconnect" {
			found = true
		}
	}
	if !found {
		t.Error("missing disconnect audit event")
	}
}

// ---------------------------------------------------------------------------
// Service tests
// ---------------------------------------------------------------------------

func TestService_NewService_RequiresTokenStore(t *testing.T) {
	_, err := NewService(Config{API: &mockAPIClient{}})
	if err == nil {
		t.Error("expected error when token store is nil")
	}
}

func TestService_NewService_RequiresAPI(t *testing.T) {
	_, err := NewService(Config{Tokens: NewMemoryTokenStore()})
	if err == nil {
		t.Error("expected error when API client is nil")
	}
}

func TestService_ConnectAndGetConnection(t *testing.T) {
	svc, _, _ := newTestService(t)
	ctx := context.Background()

	// Start OAuth and get state.
	url, _ := svc.OAuth().BeginAuthorize(ctx, "tenant-1")
	state := extractQueryParam(url, "state")

	err := svc.Connect(ctx, "tenant-1", state, "code-123", "site-1", "HQ")
	if err != nil {
		t.Fatalf("Connect: %v", err)
	}

	conn, err := svc.GetConnection(ctx, "tenant-1")
	if err != nil {
		t.Fatalf("GetConnection: %v", err)
	}
	if conn.BrivoSiteID != "site-1" {
		t.Errorf("expected site-1, got %s", conn.BrivoSiteID)
	}
	if conn.Status != ConnStatusActive {
		t.Errorf("expected active, got %s", conn.Status)
	}
}

func TestService_Disconnect(t *testing.T) {
	svc, _, _ := newTestService(t)
	ctx := context.Background()

	// Setup: connect first.
	url, _ := svc.OAuth().BeginAuthorize(ctx, "tenant-1")
	state := extractQueryParam(url, "state")
	_ = svc.Connect(ctx, "tenant-1", state, "code", "site-1", "HQ")

	if err := svc.Disconnect(ctx, "tenant-1"); err != nil {
		t.Fatalf("Disconnect: %v", err)
	}

	_, err := svc.GetConnection(ctx, "tenant-1")
	if err != ErrNotConnected {
		t.Errorf("expected ErrNotConnected, got: %v", err)
	}
}

// ---------------------------------------------------------------------------
// Door-camera mapping tests
// ---------------------------------------------------------------------------

func TestService_DoorCameraMappings(t *testing.T) {
	svc, _, _ := newTestService(t)
	ctx := context.Background()

	mapping := DoorCameraMapping{
		ID:          "map-1",
		TenantID:    "tenant-1",
		BrivoDoorID: "door-1",
		DoorName:    "Main Entrance",
		CameraID:    "cam-1",
		CameraName:  "Lobby Camera",
	}

	if err := svc.SetDoorCameraMapping(ctx, mapping); err != nil {
		t.Fatalf("SetDoorCameraMapping: %v", err)
	}

	mappings, err := svc.ListDoorCameraMappings(ctx, "tenant-1")
	if err != nil {
		t.Fatalf("ListDoorCameraMappings: %v", err)
	}
	if len(mappings) != 1 {
		t.Fatalf("expected 1 mapping, got %d", len(mappings))
	}
	if mappings[0].CameraID != "cam-1" {
		t.Errorf("expected cam-1, got %s", mappings[0].CameraID)
	}

	// Delete.
	if err := svc.DeleteDoorCameraMapping(ctx, "tenant-1", "map-1"); err != nil {
		t.Fatalf("DeleteDoorCameraMapping: %v", err)
	}
	mappings, _ = svc.ListDoorCameraMappings(ctx, "tenant-1")
	if len(mappings) != 0 {
		t.Errorf("expected 0 mappings after delete, got %d", len(mappings))
	}
}

// ---------------------------------------------------------------------------
// Webhook signature tests
// ---------------------------------------------------------------------------

func TestService_VerifyWebhookSignature_Valid(t *testing.T) {
	svc, _, _ := newTestService(t)
	payload := []byte(`{"event":"test"}`)
	mac := hmac.New(sha256.New, []byte("webhook-secret-key"))
	mac.Write(payload)
	sig := hex.EncodeToString(mac.Sum(nil))

	if err := svc.VerifyWebhookSignature(payload, sig); err != nil {
		t.Errorf("expected valid signature, got: %v", err)
	}
}

func TestService_VerifyWebhookSignature_Invalid(t *testing.T) {
	svc, _, _ := newTestService(t)
	payload := []byte(`{"event":"test"}`)

	err := svc.VerifyWebhookSignature(payload, "bad-signature")
	if err != ErrWebhookSignatureInvalid {
		t.Errorf("expected ErrWebhookSignatureInvalid, got: %v", err)
	}
}

func TestService_VerifyWebhookSignature_NoSecret(t *testing.T) {
	svc, _, _ := newTestService(t, func(cfg *Config) {
		cfg.OAuth.WebhookSecret = ""
	})
	// Should pass when no secret is configured (dev mode).
	if err := svc.VerifyWebhookSignature([]byte("anything"), "any"); err != nil {
		t.Errorf("expected nil when no secret, got: %v", err)
	}
}

// ---------------------------------------------------------------------------
// Event ingestion tests
// ---------------------------------------------------------------------------

func TestService_HandleDoorEvent(t *testing.T) {
	snapCalled := 0
	snapSvc := &mockSnapshotService{
		captureFn: func(_ context.Context, tenantID, cameraID string, _ time.Time) (string, error) {
			snapCalled++
			return fmt.Sprintf("https://snap.example.com/%s/%s.jpg", tenantID, cameraID), nil
		},
	}
	svc, _, audit := newTestService(t, func(cfg *Config) {
		cfg.Snapshots = snapSvc
	})
	ctx := context.Background()

	// Create a mapping so the event correlates to a camera.
	_ = svc.SetDoorCameraMapping(ctx, DoorCameraMapping{
		ID:          "map-1",
		TenantID:    "tenant-1",
		BrivoDoorID: "door-1",
		DoorName:    "Main",
		CameraID:    "cam-1",
		CameraName:  "Lobby",
	})

	// Store a connection to verify sync time update.
	_ = svc.conns.Upsert(ctx, Connection{
		ID:       "brivo_tenant-1",
		TenantID: "tenant-1",
		Status:   ConnStatusActive,
	})

	event := DoorEvent{
		EventID:     "evt-1",
		TenantID:    "tenant-1",
		BrivoSiteID: "site-1",
		BrivoDoorID: "door-1",
		DoorName:    "Main",
		EventType:   DoorEventUnlock,
		UserName:    "Alice",
		OccurredAt:  fixedTime,
	}

	result, err := svc.HandleDoorEvent(ctx, event)
	if err != nil {
		t.Fatalf("HandleDoorEvent: %v", err)
	}
	if result == nil {
		t.Fatal("expected non-nil result")
	}
	if len(result.SnapshotURLs) != 1 {
		t.Errorf("expected 1 snapshot URL, got %d", len(result.SnapshotURLs))
	}
	if snapCalled != 1 {
		t.Errorf("expected snapshot called once, got %d", snapCalled)
	}

	// Verify event was logged.
	events, _ := svc.ListEvents(ctx, "tenant-1", fixedTime.Add(-time.Hour), fixedTime.Add(time.Hour), 10)
	if len(events) != 1 {
		t.Errorf("expected 1 logged event, got %d", len(events))
	}

	// Verify audit.
	found := false
	for _, e := range audit.events {
		if e.Action == "event_received" {
			found = true
		}
	}
	if !found {
		t.Error("missing event_received audit event")
	}
}

func TestService_HandleDoorEvent_NoMapping(t *testing.T) {
	svc, _, _ := newTestService(t)
	ctx := context.Background()

	event := DoorEvent{
		EventID:     "evt-2",
		TenantID:    "tenant-1",
		BrivoDoorID: "door-99",
		EventType:   DoorEventLock,
		OccurredAt:  fixedTime,
	}

	result, err := svc.HandleDoorEvent(ctx, event)
	if err != nil {
		t.Fatalf("HandleDoorEvent: %v", err)
	}
	if len(result.SnapshotURLs) != 0 {
		t.Errorf("expected 0 snapshots for unmapped door, got %d", len(result.SnapshotURLs))
	}
}

func TestService_HandleDoorEvent_SnapshotError(t *testing.T) {
	snapSvc := &mockSnapshotService{
		captureFn: func(_ context.Context, _, _ string, _ time.Time) (string, error) {
			return "", fmt.Errorf("camera offline")
		},
	}
	svc, _, _ := newTestService(t, func(cfg *Config) {
		cfg.Snapshots = snapSvc
	})
	ctx := context.Background()

	_ = svc.SetDoorCameraMapping(ctx, DoorCameraMapping{
		ID: "map-1", TenantID: "tenant-1", BrivoDoorID: "door-1", CameraID: "cam-1",
	})

	event := DoorEvent{
		EventID: "evt-3", TenantID: "tenant-1", BrivoDoorID: "door-1",
		EventType: DoorEventUnlock, OccurredAt: fixedTime,
	}

	// Should not fail even when snapshot fails.
	result, err := svc.HandleDoorEvent(ctx, event)
	if err != nil {
		t.Fatalf("HandleDoorEvent should not fail on snapshot error: %v", err)
	}
	if len(result.SnapshotURLs) != 0 {
		t.Error("expected 0 snapshots when capture fails")
	}
}

// ---------------------------------------------------------------------------
// Bidirectional event flow tests
// ---------------------------------------------------------------------------

func TestService_SendNVREvent(t *testing.T) {
	sentEvent := false
	svc, api, audit := newTestService(t)
	api.sendEventFn = func(_ context.Context, token string, ev NVREvent) error {
		sentEvent = true
		if token == "" {
			return fmt.Errorf("empty token")
		}
		return nil
	}
	ctx := context.Background()

	// Store a valid token.
	_ = svc.oauth.tokens.StoreToken(ctx, "tenant-1", TokenPair{
		AccessToken: "tok", RefreshToken: "ref", ExpiresAt: time.Now().Add(time.Hour),
	})

	err := svc.SendNVREvent(ctx, NVREvent{
		TenantID: "tenant-1",
		CameraID: "cam-1",
		DoorID:   "door-1",
		Action:   "lock",
		Reason:   "motion detected",
	})
	if err != nil {
		t.Fatalf("SendNVREvent: %v", err)
	}
	if !sentEvent {
		t.Error("expected event to be sent")
	}

	found := false
	for _, e := range audit.events {
		if e.Action == "event_sent" {
			found = true
		}
	}
	if !found {
		t.Error("missing event_sent audit event")
	}
}

func TestService_SendNVREvent_NoToken(t *testing.T) {
	svc, _, _ := newTestService(t)
	err := svc.SendNVREvent(context.Background(), NVREvent{TenantID: "no-such"})
	if err == nil {
		t.Error("expected error when no token")
	}
}

// ---------------------------------------------------------------------------
// Config UI helpers tests
// ---------------------------------------------------------------------------

func TestService_ListSites(t *testing.T) {
	svc, _, _ := newTestService(t)
	ctx := context.Background()
	_ = svc.oauth.tokens.StoreToken(ctx, "tenant-1", TokenPair{
		AccessToken: "tok", RefreshToken: "ref", ExpiresAt: time.Now().Add(time.Hour),
	})

	sites, err := svc.ListSites(ctx, "tenant-1")
	if err != nil {
		t.Fatalf("ListSites: %v", err)
	}
	if len(sites) != 1 || sites[0].ID != "site-1" {
		t.Errorf("unexpected sites: %+v", sites)
	}
}

func TestService_ListDoors(t *testing.T) {
	svc, _, _ := newTestService(t)
	ctx := context.Background()
	_ = svc.oauth.tokens.StoreToken(ctx, "tenant-1", TokenPair{
		AccessToken: "tok", RefreshToken: "ref", ExpiresAt: time.Now().Add(time.Hour),
	})

	doors, err := svc.ListDoors(ctx, "tenant-1", "site-1")
	if err != nil {
		t.Fatalf("ListDoors: %v", err)
	}
	if len(doors) != 1 || doors[0].ID != "door-1" {
		t.Errorf("unexpected doors: %+v", doors)
	}
}

func TestService_TestConnection(t *testing.T) {
	svc, _, audit := newTestService(t)
	ctx := context.Background()
	_ = svc.oauth.tokens.StoreToken(ctx, "tenant-1", TokenPair{
		AccessToken: "tok", RefreshToken: "ref", ExpiresAt: time.Now().Add(time.Hour),
	})

	if err := svc.TestConnection(ctx, "tenant-1"); err != nil {
		t.Fatalf("TestConnection: %v", err)
	}

	found := false
	for _, e := range audit.events {
		if e.Action == "test" {
			found = true
		}
	}
	if !found {
		t.Error("missing test audit event")
	}
}

func TestService_TestConnection_Fails(t *testing.T) {
	svc, api, _ := newTestService(t)
	api.listSitesFn = func(_ context.Context, _ string) ([]BrivoSite, error) {
		return nil, fmt.Errorf("unauthorized")
	}
	ctx := context.Background()
	_ = svc.oauth.tokens.StoreToken(ctx, "tenant-1", TokenPair{
		AccessToken: "tok", RefreshToken: "ref", ExpiresAt: time.Now().Add(time.Hour),
	})

	err := svc.TestConnection(ctx, "tenant-1")
	if err == nil {
		t.Error("expected error")
	}
}

// ---------------------------------------------------------------------------
// Integration registry tests
// ---------------------------------------------------------------------------

func TestIntegrationMeta(t *testing.T) {
	meta := IntegrationMeta()
	if meta.ID != "brivo" {
		t.Errorf("expected ID=brivo, got %s", meta.ID)
	}
	if meta.Category != "access_control" {
		t.Errorf("expected category access_control, got %s", meta.Category)
	}
	if !meta.Bidirectional {
		t.Error("expected bidirectional=true")
	}
}

// ---------------------------------------------------------------------------
// Memory store tests
// ---------------------------------------------------------------------------

func TestMemoryTokenStore(t *testing.T) {
	store := NewMemoryTokenStore()
	ctx := context.Background()

	_, err := store.GetToken(ctx, "t1")
	if err != ErrNotConnected {
		t.Errorf("expected ErrNotConnected, got %v", err)
	}

	tok := TokenPair{AccessToken: "a", RefreshToken: "r"}
	if err := store.StoreToken(ctx, "t1", tok); err != nil {
		t.Fatal(err)
	}

	got, err := store.GetToken(ctx, "t1")
	if err != nil {
		t.Fatal(err)
	}
	if got.AccessToken != "a" {
		t.Errorf("expected a, got %s", got.AccessToken)
	}

	if err := store.DeleteToken(ctx, "t1"); err != nil {
		t.Fatal(err)
	}
	_, err = store.GetToken(ctx, "t1")
	if err != ErrNotConnected {
		t.Errorf("expected ErrNotConnected after delete, got %v", err)
	}
}

func TestMemoryConnectionStore(t *testing.T) {
	store := NewMemoryConnectionStore()
	ctx := context.Background()

	_, err := store.Get(ctx, "t1")
	if err != ErrNotConnected {
		t.Errorf("expected ErrNotConnected, got %v", err)
	}

	conn := Connection{ID: "c1", TenantID: "t1", Status: ConnStatusActive}
	if err := store.Upsert(ctx, conn); err != nil {
		t.Fatal(err)
	}

	got, err := store.Get(ctx, "t1")
	if err != nil {
		t.Fatal(err)
	}
	if got.Status != ConnStatusActive {
		t.Errorf("expected active, got %s", got.Status)
	}

	now := time.Now()
	if err := store.UpdateSyncTime(ctx, "t1", now); err != nil {
		t.Fatal(err)
	}
	got, _ = store.Get(ctx, "t1")
	if got.LastSyncAt == nil || !got.LastSyncAt.Equal(now) {
		t.Error("sync time not updated")
	}

	if err := store.Delete(ctx, "t1"); err != nil {
		t.Fatal(err)
	}
	_, err = store.Get(ctx, "t1")
	if err != ErrNotConnected {
		t.Errorf("expected ErrNotConnected after delete")
	}
}

func TestMemoryMappingStore(t *testing.T) {
	store := NewMemoryMappingStore()
	ctx := context.Background()

	m := DoorCameraMapping{ID: "m1", TenantID: "t1", BrivoDoorID: "d1", CameraID: "c1"}
	if err := store.Set(ctx, m); err != nil {
		t.Fatal(err)
	}

	// Upsert same ID.
	m.CameraID = "c2"
	if err := store.Set(ctx, m); err != nil {
		t.Fatal(err)
	}

	list, _ := store.ListByTenant(ctx, "t1")
	if len(list) != 1 {
		t.Fatalf("expected 1 mapping after upsert, got %d", len(list))
	}
	if list[0].CameraID != "c2" {
		t.Errorf("expected c2 after upsert, got %s", list[0].CameraID)
	}

	byDoor, _ := store.ListByDoor(ctx, "t1", "d1")
	if len(byDoor) != 1 {
		t.Errorf("expected 1 by door, got %d", len(byDoor))
	}

	_ = store.Delete(ctx, "t1", "m1")
	list, _ = store.ListByTenant(ctx, "t1")
	if len(list) != 0 {
		t.Errorf("expected 0 after delete, got %d", len(list))
	}
}

func TestMemoryEventLog(t *testing.T) {
	log := NewMemoryEventLog()
	ctx := context.Background()
	base := time.Date(2026, 1, 1, 12, 0, 0, 0, time.UTC)

	for i := 0; i < 5; i++ {
		_ = log.Append(ctx, DoorEvent{
			EventID:    fmt.Sprintf("e%d", i),
			TenantID:   "t1",
			OccurredAt: base.Add(time.Duration(i) * time.Hour),
		})
	}

	events, _ := log.ListByTenant(ctx, "t1", base, base.Add(10*time.Hour), 3)
	if len(events) != 3 {
		t.Errorf("expected 3 events with limit, got %d", len(events))
	}
	// Should be descending order.
	if events[0].OccurredAt.Before(events[1].OccurredAt) {
		t.Error("expected descending order")
	}

	// Filter by time range.
	events, _ = log.ListByTenant(ctx, "t1", base.Add(2*time.Hour), base.Add(3*time.Hour), 10)
	if len(events) != 2 {
		t.Errorf("expected 2 events in range, got %d", len(events))
	}
}

// ---------------------------------------------------------------------------
// Helper
// ---------------------------------------------------------------------------

func extractQueryParam(rawURL, key string) string {
	idx := strings.Index(rawURL, "?")
	if idx < 0 {
		return ""
	}
	query := rawURL[idx+1:]
	for _, pair := range strings.Split(query, "&") {
		parts := strings.SplitN(pair, "=", 2)
		if len(parts) == 2 && parts[0] == key {
			return parts[1]
		}
	}
	return ""
}
