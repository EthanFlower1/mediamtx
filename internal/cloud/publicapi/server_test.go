package publicapi

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/bluenviron/mediamtx/internal/shared/auth"
	fakeauth "github.com/bluenviron/mediamtx/internal/shared/auth/fake"
)

// ---------------------------------------------------------------------------
// Test helpers
// ---------------------------------------------------------------------------

type testFixture struct {
	server  *Server
	http    *httptest.Server
	idp     *fakeauth.Provider
	keyStore *fakeAPIKeyStore
}

func newTestServer(t *testing.T, tweak func(*Config)) *testFixture {
	t.Helper()

	idp := fakeauth.New()
	keyStore := &fakeAPIKeyStore{keys: make(map[string]*APIKey)}

	cfg := Config{
		ListenAddr: ":0",
		Identity:   idp,
		APIKeyStore: keyStore,
		ShutdownTimeout: time.Second,
	}
	if tweak != nil {
		tweak(&cfg)
	}

	srv, err := New(cfg)
	if err != nil {
		t.Fatalf("new server: %v", err)
	}

	ts := httptest.NewServer(srv.Handler())
	t.Cleanup(ts.Close)

	return &testFixture{
		server:   srv,
		http:     ts,
		idp:      idp,
		keyStore: keyStore,
	}
}

// fakeAPIKeyStore is a test-only in-memory API key store.
type fakeAPIKeyStore struct {
	keys         map[string]*APIKey
	touchedKeys  []string
}

func (s *fakeAPIKeyStore) Validate(_ context.Context, rawKey string) (*APIKey, error) {
	key, ok := s.keys[rawKey]
	if !ok {
		return nil, ErrInvalidAPIKey
	}
	if key.IsExpired() {
		return nil, ErrAPIKeyExpired
	}
	if key.IsRevoked() {
		return nil, ErrAPIKeyRevoked
	}
	return key, nil
}

func (s *fakeAPIKeyStore) TouchLastUsed(_ context.Context, keyID string) error {
	s.touchedKeys = append(s.touchedKeys, keyID)
	return nil
}

func (s *fakeAPIKeyStore) AddKey(rawKey string, key *APIKey) {
	s.keys[rawKey] = key
}

func mintBearerToken(t *testing.T, idp *fakeauth.Provider, tenantID, username string) string {
	t.Helper()
	ctx := context.Background()
	tenant := auth.TenantRef{Type: auth.TenantTypeCustomer, ID: tenantID}
	_, err := idp.CreateUser(ctx, tenant, auth.UserSpec{
		Username: username,
		Email:    username + "@test.local",
		Password: "pw",
	})
	if err != nil {
		t.Fatalf("create user: %v", err)
	}
	sess, err := idp.AuthenticateLocal(ctx, tenant, username, "pw")
	if err != nil {
		t.Fatalf("authenticate: %v", err)
	}
	return sess.AccessToken
}

// ---------------------------------------------------------------------------
// Health endpoint
// ---------------------------------------------------------------------------

func TestHealthEndpoint(t *testing.T) {
	fx := newTestServer(t, nil)

	resp, err := http.Get(fx.http.URL + "/api/v1/health")
	if err != nil {
		t.Fatalf("GET /api/v1/health: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Errorf("status = %d; want 200", resp.StatusCode)
	}

	var body map[string]string
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		t.Fatalf("decode body: %v", err)
	}
	if body["status"] != "ok" {
		t.Errorf("status = %q; want ok", body["status"])
	}
	if body["version"] != "v1" {
		t.Errorf("version = %q; want v1", body["version"])
	}
}

// ---------------------------------------------------------------------------
// OpenAPI spec endpoint
// ---------------------------------------------------------------------------

func TestOpenAPISpecEndpoint(t *testing.T) {
	fx := newTestServer(t, nil)

	resp, err := http.Get(fx.http.URL + "/api/v1/openapi.json")
	if err != nil {
		t.Fatalf("GET /api/v1/openapi.json: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Errorf("status = %d; want 200", resp.StatusCode)
	}
	if ct := resp.Header.Get("Content-Type"); ct != "application/json" {
		t.Errorf("Content-Type = %q; want application/json", ct)
	}

	body, _ := io.ReadAll(resp.Body)
	var spec map[string]interface{}
	if err := json.Unmarshal(body, &spec); err != nil {
		t.Fatalf("OpenAPI spec is not valid JSON: %v", err)
	}

	if spec["openapi"] != "3.0.3" {
		t.Errorf("openapi version = %v; want 3.0.3", spec["openapi"])
	}

	info, ok := spec["info"].(map[string]interface{})
	if !ok {
		t.Fatal("missing info block")
	}
	if info["version"] != "1.0.0" {
		t.Errorf("info.version = %v; want 1.0.0", info["version"])
	}
}

// ---------------------------------------------------------------------------
// API Version header
// ---------------------------------------------------------------------------

func TestAPIVersionHeader(t *testing.T) {
	fx := newTestServer(t, nil)

	// Even unauthenticated requests on Connect paths should get the version header.
	resp, err := http.Post(
		fx.http.URL+PublicServicePath("PublicCamerasService", "ListCameras"),
		"application/json",
		strings.NewReader("{}"),
	)
	if err != nil {
		t.Fatalf("POST: %v", err)
	}
	defer resp.Body.Close()

	if v := resp.Header.Get("X-API-Version"); v != "v1" {
		t.Errorf("X-API-Version = %q; want v1", v)
	}
}

// ---------------------------------------------------------------------------
// Authentication: unauthenticated requests
// ---------------------------------------------------------------------------

func TestUnauthenticatedReturns401(t *testing.T) {
	fx := newTestServer(t, nil)

	resp, err := http.Post(
		fx.http.URL+PublicServicePath("PublicCamerasService", "ListCameras"),
		"application/json",
		strings.NewReader("{}"),
	)
	if err != nil {
		t.Fatalf("POST: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusUnauthorized {
		t.Errorf("status = %d; want 401", resp.StatusCode)
	}

	var env map[string]string
	if err := json.NewDecoder(resp.Body).Decode(&env); err != nil {
		t.Fatalf("decode body: %v", err)
	}
	if env["code"] != "unauthenticated" {
		t.Errorf("code = %q; want unauthenticated", env["code"])
	}
}

// ---------------------------------------------------------------------------
// Authentication: bearer token
// ---------------------------------------------------------------------------

func TestBearerTokenAuth(t *testing.T) {
	fx := newTestServer(t, nil)
	token := mintBearerToken(t, fx.idp, "acme", "alice")

	req, _ := http.NewRequest(http.MethodPost,
		fx.http.URL+PublicServicePath("PublicCamerasService", "ListCameras"),
		strings.NewReader("{}"))
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Content-Type", "application/json")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("POST: %v", err)
	}
	defer resp.Body.Close()

	// The stub handler returns 501 Unimplemented — the important thing is
	// that we got past auth (not 401).
	if resp.StatusCode == http.StatusUnauthorized {
		t.Errorf("expected to pass auth, got 401")
	}
	// Expect 501 from the stub.
	if resp.StatusCode != http.StatusNotImplemented {
		t.Errorf("status = %d; want 501 (unimplemented stub)", resp.StatusCode)
	}
}

// ---------------------------------------------------------------------------
// Authentication: API key
// ---------------------------------------------------------------------------

func TestAPIKeyAuth(t *testing.T) {
	fx := newTestServer(t, nil)
	fx.keyStore.AddKey("kvue_test123456789012345678901234567890", &APIKey{
		ID:       "key-1",
		TenantID: "acme",
		Tier:     TierPro,
		Name:     "test-key",
	})

	req, _ := http.NewRequest(http.MethodPost,
		fx.http.URL+PublicServicePath("PublicCamerasService", "ListCameras"),
		strings.NewReader("{}"))
	req.Header.Set(APIKeyHeader, "kvue_test123456789012345678901234567890")
	req.Header.Set("Content-Type", "application/json")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("POST: %v", err)
	}
	defer resp.Body.Close()

	// Should not be 401 — API key auth succeeded.
	if resp.StatusCode == http.StatusUnauthorized {
		body, _ := io.ReadAll(resp.Body)
		t.Fatalf("expected to pass API key auth, got 401: %s", body)
	}
	// Expect 501 from the stub handler.
	if resp.StatusCode != http.StatusNotImplemented {
		t.Errorf("status = %d; want 501", resp.StatusCode)
	}
}

func TestInvalidAPIKeyReturns401(t *testing.T) {
	fx := newTestServer(t, nil)

	req, _ := http.NewRequest(http.MethodPost,
		fx.http.URL+PublicServicePath("PublicCamerasService", "ListCameras"),
		strings.NewReader("{}"))
	req.Header.Set(APIKeyHeader, "kvue_invalid_key_here")
	req.Header.Set("Content-Type", "application/json")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("POST: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusUnauthorized {
		t.Errorf("status = %d; want 401", resp.StatusCode)
	}
}

func TestExpiredAPIKeyReturns401(t *testing.T) {
	fx := newTestServer(t, nil)
	fx.keyStore.AddKey("kvue_expired_key", &APIKey{
		ID:        "key-expired",
		TenantID:  "acme",
		Tier:      TierFree,
		ExpiresAt: time.Now().Add(-1 * time.Hour),
	})

	req, _ := http.NewRequest(http.MethodPost,
		fx.http.URL+PublicServicePath("PublicCamerasService", "ListCameras"),
		strings.NewReader("{}"))
	req.Header.Set(APIKeyHeader, "kvue_expired_key")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("POST: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusUnauthorized {
		t.Errorf("status = %d; want 401", resp.StatusCode)
	}
}

func TestRevokedAPIKeyReturns401(t *testing.T) {
	fx := newTestServer(t, nil)
	fx.keyStore.AddKey("kvue_revoked_key", &APIKey{
		ID:        "key-revoked",
		TenantID:  "acme",
		Tier:      TierFree,
		RevokedAt: time.Now().Add(-1 * time.Hour),
	})

	req, _ := http.NewRequest(http.MethodPost,
		fx.http.URL+PublicServicePath("PublicCamerasService", "ListCameras"),
		strings.NewReader("{}"))
	req.Header.Set(APIKeyHeader, "kvue_revoked_key")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("POST: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusUnauthorized {
		t.Errorf("status = %d; want 401", resp.StatusCode)
	}
}

// ---------------------------------------------------------------------------
// Rate limiting
// ---------------------------------------------------------------------------

func TestTieredRateLimiting(t *testing.T) {
	fx := newTestServer(t, func(c *Config) {
		c.TierResolver = func(string) TenantTier { return TierFree }
	})
	fx.keyStore.AddKey("kvue_ratelimit_key", &APIKey{
		ID:       "key-rl",
		TenantID: "acme",
		Tier:     TierFree,
		Name:     "rate-test",
	})

	do := func() int {
		req, _ := http.NewRequest(http.MethodPost,
			fx.http.URL+PublicServicePath("PublicCamerasService", "ListCameras"),
			strings.NewReader("{}"))
		req.Header.Set(APIKeyHeader, "kvue_ratelimit_key")
		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			t.Fatalf("POST: %v", err)
		}
		resp.Body.Close()
		return resp.StatusCode
	}

	// Free tier: burst=5. First 5 requests should not be 429.
	for i := 0; i < 5; i++ {
		code := do()
		if code == http.StatusTooManyRequests {
			t.Fatalf("request %d hit rate limit prematurely (free tier burst=5)", i)
		}
	}

	// 6th request should be rate-limited.
	code := do()
	if code != http.StatusTooManyRequests {
		t.Errorf("6th request = %d; want 429", code)
	}
}

func TestRateLimitHeaders(t *testing.T) {
	fx := newTestServer(t, nil)
	fx.keyStore.AddKey("kvue_header_key", &APIKey{
		ID:       "key-hdr",
		TenantID: "acme",
		Tier:     TierPro,
		Name:     "header-test",
	})

	req, _ := http.NewRequest(http.MethodPost,
		fx.http.URL+PublicServicePath("PublicCamerasService", "ListCameras"),
		strings.NewReader("{}"))
	req.Header.Set(APIKeyHeader, "kvue_header_key")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("POST: %v", err)
	}
	defer resp.Body.Close()

	if tier := resp.Header.Get("X-RateLimit-Tier"); tier != "pro" {
		t.Errorf("X-RateLimit-Tier = %q; want pro", tier)
	}
	if limit := resp.Header.Get("X-RateLimit-Limit"); limit == "" {
		t.Error("missing X-RateLimit-Limit header")
	}
}

// ---------------------------------------------------------------------------
// Scope enforcement
// ---------------------------------------------------------------------------

func TestAPIKeyScopeEnforcement(t *testing.T) {
	fx := newTestServer(t, nil)

	// Key with limited scope: only cameras:read.
	fx.keyStore.AddKey("kvue_scoped_key", &APIKey{
		ID:       "key-scoped",
		TenantID: "acme",
		Tier:     TierStarter,
		Scopes:   []string{"cameras:read"},
	})

	// cameras:read (ListCameras) should succeed past auth.
	req, _ := http.NewRequest(http.MethodPost,
		fx.http.URL+PublicServicePath("PublicCamerasService", "ListCameras"),
		strings.NewReader("{}"))
	req.Header.Set(APIKeyHeader, "kvue_scoped_key")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("POST: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusForbidden {
		t.Error("cameras:read should be allowed by scope cameras:read")
	}

	// cameras:create (CreateCamera) should be forbidden.
	req2, _ := http.NewRequest(http.MethodPost,
		fx.http.URL+PublicServicePath("PublicCamerasService", "CreateCamera"),
		strings.NewReader("{}"))
	req2.Header.Set(APIKeyHeader, "kvue_scoped_key")

	resp2, err := http.DefaultClient.Do(req2)
	if err != nil {
		t.Fatalf("POST: %v", err)
	}
	defer resp2.Body.Close()

	if resp2.StatusCode != http.StatusForbidden {
		t.Errorf("cameras:create with scope cameras:read = %d; want 403", resp2.StatusCode)
	}
}

func TestAPIKeyWildcardScope(t *testing.T) {
	fx := newTestServer(t, nil)

	// Key with wildcard scope: cameras:*
	fx.keyStore.AddKey("kvue_wildcard_key", &APIKey{
		ID:       "key-wild",
		TenantID: "acme",
		Tier:     TierStarter,
		Scopes:   []string{"cameras:*"},
	})

	// cameras:create should be allowed by wildcard.
	req, _ := http.NewRequest(http.MethodPost,
		fx.http.URL+PublicServicePath("PublicCamerasService", "CreateCamera"),
		strings.NewReader("{}"))
	req.Header.Set(APIKeyHeader, "kvue_wildcard_key")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("POST: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusForbidden {
		t.Error("cameras:create should be allowed by wildcard scope cameras:*")
	}
}

// ---------------------------------------------------------------------------
// All public service endpoints are registered
// ---------------------------------------------------------------------------

func TestAllServicesRegistered(t *testing.T) {
	fx := newTestServer(t, nil)
	token := mintBearerToken(t, fx.idp, "acme", "coverage-user")

	paths := AllPublicPaths()
	if len(paths) == 0 {
		t.Fatal("no public paths registered")
	}

	for _, path := range paths {
		req, _ := http.NewRequest(http.MethodPost, fx.http.URL+path, strings.NewReader("{}"))
		req.Header.Set("Authorization", "Bearer "+token)
		req.Header.Set("Content-Type", "application/json")

		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			t.Fatalf("POST %s: %v", path, err)
		}
		resp.Body.Close()

		// We expect 501 (unimplemented stub), not 404 (unregistered).
		if resp.StatusCode == http.StatusNotFound {
			t.Errorf("path %s returned 404 — not registered", path)
		}
	}
}

// ---------------------------------------------------------------------------
// Service + method count contract
// ---------------------------------------------------------------------------

func TestPublicMethodCount(t *testing.T) {
	// This is a contract assertion. If the count changes, downstream tickets
	// need to be updated. The expected count is:
	//   cameras:5 + users:5 + recordings:3 + events:4 + schedules:5 +
	//   retention:5 + integrations:6 = 33
	want := 33
	got := TotalPublicMethodCount()
	if got != want {
		t.Errorf("TotalPublicMethodCount() = %d; want %d", got, want)
	}
}

// ---------------------------------------------------------------------------
// Route authorization coverage
// ---------------------------------------------------------------------------

func TestAllPathsHaveRouteAuth(t *testing.T) {
	routes := PublicRouteAuthorizations()
	paths := AllPublicPaths()

	for _, p := range paths {
		if _, ok := routes[p]; !ok {
			t.Errorf("path %s has no route authorization entry", p)
		}
	}
}

// ---------------------------------------------------------------------------
// REST gateway endpoints registered
// ---------------------------------------------------------------------------

func TestRESTGatewayEndpoints(t *testing.T) {
	fx := newTestServer(t, nil)
	token := mintBearerToken(t, fx.idp, "acme", "rest-user")

	resources := []string{
		"cameras", "users", "recordings", "events",
		"schedules", "retention-policies", "integrations",
	}
	for _, r := range resources {
		req, _ := http.NewRequest(http.MethodGet, fx.http.URL+"/api/v1/"+r, nil)
		req.Header.Set("Authorization", "Bearer "+token)

		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			t.Fatalf("GET /api/v1/%s: %v", r, err)
		}
		resp.Body.Close()

		// Expect 501 (not implemented), not 404 (not registered).
		if resp.StatusCode == http.StatusNotFound {
			t.Errorf("/api/v1/%s returned 404 — not registered", r)
		}
	}
}

// ---------------------------------------------------------------------------
// Server lifecycle
// ---------------------------------------------------------------------------

func TestServerStartAndShutdown(t *testing.T) {
	idp := fakeauth.New()
	srv, err := New(Config{
		ListenAddr: "127.0.0.1:0",
		Identity:   idp,
	})
	if err != nil {
		t.Fatalf("new: %v", err)
	}

	done := make(chan error, 1)
	go func() { done <- srv.Start(context.Background()) }()
	time.Sleep(50 * time.Millisecond)

	if err := srv.Shutdown(context.Background()); err != nil {
		t.Fatalf("shutdown: %v", err)
	}
	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("start returned: %v", err)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("server did not stop within 3s")
	}
}

// ---------------------------------------------------------------------------
// Config validation
// ---------------------------------------------------------------------------

func TestConfigValidation(t *testing.T) {
	_, err := New(Config{})
	if err == nil {
		t.Fatal("expected error for empty config")
	}

	_, err = New(Config{ListenAddr: ":0"})
	if err == nil {
		t.Fatal("expected error for missing Identity")
	}
}
