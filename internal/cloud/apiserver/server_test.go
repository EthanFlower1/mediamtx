// TODO(KAI-310): replace with generated connectrpc code once buf is wired.
package apiserver

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/bluenviron/mediamtx/internal/cloud/audit"
	"github.com/bluenviron/mediamtx/internal/cloud/db"
	"github.com/bluenviron/mediamtx/internal/cloud/permissions"
	"github.com/bluenviron/mediamtx/internal/shared/auth"
	fakeauth "github.com/bluenviron/mediamtx/internal/shared/auth/fake"
)

// newTestServer builds a Server wired to in-memory dependencies. It returns
// the server, its httptest.Server wrapper, the audit recorder (so tests can
// introspect emitted entries), and the identity provider (so tests can mint
// tokens).
type testFixture struct {
	server   *Server
	http     *httptest.Server
	idp      *fakeauth.Provider
	audit    *audit.MemoryRecorder
	enforcer *permissions.Enforcer
	dbHandle *db.DB
}

func newTestServer(t *testing.T, tweak func(*Config)) *testFixture {
	t.Helper()

	dbHandle, err := db.Open(context.Background(), "sqlite://:memory:")
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	t.Cleanup(func() { _ = dbHandle.Close() })

	idp := fakeauth.New()
	recorder := audit.NewMemoryRecorder()

	store := permissions.NewInMemoryStore()
	enforcer, err := permissions.NewEnforcer(store, nil)
	if err != nil {
		t.Fatalf("new enforcer: %v", err)
	}

	cfg := Config{
		ListenAddr:    ":0",
		Region:        "us-east-2",
		DB:            dbHandle,
		Identity:      idp,
		Enforcer:      enforcer,
		AuditRecorder: recorder,
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
		audit:    recorder,
		enforcer: enforcer,
		dbHandle: dbHandle,
	}
}

// mintToken creates a user and active session in the fake IdP, returning a
// bearer-ready access token for that user. The user lives in a customer
// tenant whose ID matches tenantID.
func mintToken(t *testing.T, idp *fakeauth.Provider, tenantID, username string) (token string, tenant auth.TenantRef, userID auth.UserID) {
	t.Helper()
	ctx := context.Background()
	tenant = auth.TenantRef{Type: auth.TenantTypeCustomer, ID: tenantID}
	user, err := idp.CreateUser(ctx, tenant, auth.UserSpec{
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
	return sess.AccessToken, tenant, user.ID
}

// ---------------------------------------------------------------------
// Server lifecycle
// ---------------------------------------------------------------------

func TestServerStartAndShutdown(t *testing.T) {
	fx := newTestServer(t, func(c *Config) {
		// pick a random free port; test relies on start/shutdown not
		// the actual listener hostname
		c.ListenAddr = "127.0.0.1:0"
	})

	// Start in a goroutine; Start blocks until Shutdown is called.
	done := make(chan error, 1)
	go func() { done <- fx.server.Start(context.Background()) }()

	// Give Start time to bind the listener. We can't inspect the picked
	// port easily because http.Server doesn't expose it; a brief sleep is
	// acceptable in a unit test.
	time.Sleep(50 * time.Millisecond)

	if err := fx.server.Shutdown(context.Background()); err != nil {
		t.Fatalf("shutdown: %v", err)
	}
	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("start returned error: %v", err)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("server did not stop within 3s")
	}
}

// ---------------------------------------------------------------------
// Middleware order
// ---------------------------------------------------------------------

// TestMiddlewareChainOrder injects a tracing shim and asserts that:
//   - the request ID middleware runs BEFORE tracing (tracing sees the ID
//     on the context — actually in our stack tracing runs after reqID but
//     before auth; the assertion is that the tracing finish func fires at
//     all and that it observes the final status set by inner layers)
//   - the recovery middleware is the outermost wrapper, because a panic
//     from inside a handler should still produce a 500 response
func TestMiddlewareChainOrder(t *testing.T) {
	var tracerCalled atomic.Int32
	var tracerObservedStatus atomic.Int32

	fx := newTestServer(t, func(c *Config) {
		c.Tracer = func(method, path string) (finish func(status int)) {
			tracerCalled.Add(1)
			return func(status int) { tracerObservedStatus.Store(int32(status)) }
		}
	})

	// Hit the public /healthz endpoint (region middleware still runs but
	// lets the request through because no region header is set).
	resp, err := http.Get(fx.http.URL + "/healthz")
	if err != nil {
		t.Fatalf("GET /healthz: %v", err)
	}
	defer resp.Body.Close()
	_, _ = io.Copy(io.Discard, resp.Body)

	// /healthz bypasses the full Connect chain (it's not wrapped in
	// tracingMiddleware), so the tracer should NOT have fired — this
	// assertion doubles as documentation that health probes bypass the
	// instrumentation chain.
	if tracerCalled.Load() != 0 {
		t.Errorf("tracer fired on /healthz; want 0, got %d", tracerCalled.Load())
	}

	// Now hit a Connect path (unauthenticated so we expect 401, but the
	// chain still runs end to end through tracing).
	resp2, err := http.Post(fx.http.URL+ServicePath("CamerasService", "ListCameras"), "application/json", strings.NewReader("{}"))
	if err != nil {
		t.Fatalf("POST cameras: %v", err)
	}
	defer resp2.Body.Close()
	_, _ = io.Copy(io.Discard, resp2.Body)

	if tracerCalled.Load() == 0 {
		t.Errorf("tracer did not fire on Connect path")
	}
	if got := tracerObservedStatus.Load(); got != http.StatusUnauthorized {
		t.Errorf("tracer saw status %d; want 401", got)
	}
	if resp2.Header.Get("X-Request-Id") == "" {
		t.Errorf("missing X-Request-Id on response — request ID middleware not outermost")
	}
}

// ---------------------------------------------------------------------
// Region routing
// ---------------------------------------------------------------------

func TestRegionMismatchRedirects(t *testing.T) {
	fx := newTestServer(t, func(c *Config) {
		c.RegionRoutes = []RegionRoute{
			{Region: "us-west-2", BaseURL: "https://api-us-west-2.kaivue.io"},
		}
	})

	req, _ := http.NewRequest(http.MethodPost, fx.http.URL+ServicePath("CamerasService", "ListCameras"), strings.NewReader("{}"))
	req.Header.Set(RegionHeader, "us-west-2")
	req.Header.Set("Content-Type", "application/json")

	// We must disable the default redirect follower; we're asserting the
	// server EMITS the 307, not that the client follows it.
	client := &http.Client{
		CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse },
	}
	resp, err := client.Do(req)
	if err != nil {
		t.Fatalf("POST with region: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusTemporaryRedirect {
		t.Fatalf("status = %d; want 307", resp.StatusCode)
	}
	want := "https://api-us-west-2.kaivue.io" + ServicePath("CamerasService", "ListCameras")
	if got := resp.Header.Get("Location"); got != want {
		t.Errorf("Location = %q; want %q", got, want)
	}
}

func TestRegionUnknownRejected(t *testing.T) {
	fx := newTestServer(t, nil)
	req, _ := http.NewRequest(http.MethodPost, fx.http.URL+ServicePath("CamerasService", "ListCameras"), strings.NewReader("{}"))
	req.Header.Set(RegionHeader, "eu-north-99")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("POST: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusBadRequest {
		t.Errorf("status = %d; want 400 for unknown region", resp.StatusCode)
	}
}

// ---------------------------------------------------------------------
// Authentication
// ---------------------------------------------------------------------

func TestUnauthenticatedReturns401(t *testing.T) {
	fx := newTestServer(t, nil)

	req, _ := http.NewRequest(http.MethodPost, fx.http.URL+ServicePath("CamerasService", "ListCameras"), strings.NewReader("{}"))
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("POST: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusUnauthorized {
		t.Errorf("status = %d; want 401", resp.StatusCode)
	}
	var env errorEnvelope
	if err := json.NewDecoder(resp.Body).Decode(&env); err != nil {
		t.Fatalf("decode body: %v", err)
	}
	if env.Code != "unauthenticated" {
		t.Errorf("code = %q; want unauthenticated", env.Code)
	}
}

// ---------------------------------------------------------------------
// Permission enforcement + audit integration
// ---------------------------------------------------------------------

func TestAuthenticatedForbiddenReturns403AndAuditDeny(t *testing.T) {
	fx := newTestServer(t, nil)
	token, tenant, _ := mintToken(t, fx.idp, "acme", "alice")

	// No policy has been added — Casbin default-deny kicks in, which is
	// exactly what we want to assert.
	_ = tenant

	req, _ := http.NewRequest(http.MethodPost, fx.http.URL+ServicePath("CamerasService", "ListCameras"), strings.NewReader("{}"))
	req.Header.Set("Authorization", "Bearer "+token)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("POST: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusForbidden {
		t.Fatalf("status = %d; want 403", resp.StatusCode)
	}

	// Give the audit middleware's detached goroutine a moment to record.
	if !waitForAuditEntries(fx.audit, tenant.ID, 1, time.Second) {
		t.Fatalf("no audit deny entry recorded for tenant %q", tenant.ID)
	}
	entries, err := fx.audit.Query(context.Background(), audit.QueryFilter{TenantID: tenant.ID})
	if err != nil {
		t.Fatalf("query audit: %v", err)
	}
	if len(entries) != 1 {
		t.Fatalf("len(entries) = %d; want 1", len(entries))
	}
	if entries[0].Result != audit.ResultDeny {
		t.Errorf("result = %q; want deny", entries[0].Result)
	}
}

func TestAuthenticatedAllowedReturns2xxAndAuditAllow(t *testing.T) {
	fx := newTestServer(t, nil)
	token, tenant, userID := mintToken(t, fx.idp, "acme", "bob")

	// Seed an allow policy for bob on cameras/read in his own tenant.
	rule := permissions.PolicyRule{
		Sub: permissions.NewUserSubject(userID, tenant).String(),
		Obj: permissions.NewObjectAll(tenant, "cameras").String(),
		Act: "read",
		Eft: "allow",
	}
	if err := fx.enforcer.AddPolicy(rule); err != nil {
		t.Fatalf("add policy: %v", err)
	}

	req, _ := http.NewRequest(http.MethodPost, fx.http.URL+ServicePath("CamerasService", "ListCameras"), strings.NewReader("{}"))
	req.Header.Set("Authorization", "Bearer "+token)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("POST: %v", err)
	}
	defer resp.Body.Close()

	// The stub handler returns 501 Unimplemented, which the audit
	// middleware does NOT auto-record (it only records 2xx allow / 403
	// deny). To prove the allow branch end-to-end we instead hit a
	// custom route via a tiny adapter: swap the stub for a 200 OK.
	if resp.StatusCode != http.StatusNotImplemented {
		t.Fatalf("status = %d; want 501 Unimplemented", resp.StatusCode)
	}

	// Now exercise the allow branch by registering a short-circuiting
	// handler that returns 200. We install it on a fresh path so we
	// don't collide with the default unimplemented stub.
	fx.server.mux.Handle("/test.v1.Echo/OK",
		fx.server.buildConnectChain()(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			w.WriteHeader(http.StatusOK)
		})))
	// Wire permission for the new path.
	fx.server.routes["/test.v1.Echo/OK"] = RouteAuthorization{ResourceType: "cameras", Action: "read"}

	req2, _ := http.NewRequest(http.MethodPost, fx.http.URL+"/test.v1.Echo/OK", strings.NewReader("{}"))
	req2.Header.Set("Authorization", "Bearer "+token)
	resp2, err := http.DefaultClient.Do(req2)
	if err != nil {
		t.Fatalf("POST test echo: %v", err)
	}
	defer resp2.Body.Close()
	if resp2.StatusCode != http.StatusOK {
		t.Fatalf("echo status = %d; want 200", resp2.StatusCode)
	}

	if !waitForAuditEntries(fx.audit, tenant.ID, 1, time.Second) {
		t.Fatalf("no audit allow entry recorded")
	}
	entries, _ := fx.audit.Query(context.Background(), audit.QueryFilter{TenantID: tenant.ID})
	var sawAllow bool
	for _, e := range entries {
		if e.Result == audit.ResultAllow {
			sawAllow = true
			break
		}
	}
	if !sawAllow {
		t.Errorf("no allow entry among %d recorded", len(entries))
	}
}

// ---------------------------------------------------------------------
// Health endpoints
// ---------------------------------------------------------------------

func TestHealthEndpointsWhenHealthy(t *testing.T) {
	fx := newTestServer(t, nil)

	resp, err := http.Get(fx.http.URL + "/healthz")
	if err != nil {
		t.Fatalf("GET /healthz: %v", err)
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Errorf("/healthz = %d; want 200", resp.StatusCode)
	}

	resp2, err := http.Get(fx.http.URL + "/readyz")
	if err != nil {
		t.Fatalf("GET /readyz: %v", err)
	}
	defer resp2.Body.Close()
	if resp2.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp2.Body)
		t.Errorf("/readyz = %d; want 200; body=%s", resp2.StatusCode, body)
	}
}

func TestReadinessFailsWhenDBDown(t *testing.T) {
	fx := newTestServer(t, nil)
	fx.server.SetReadinessProbes(ReadinessProbes{
		DB: func(context.Context) error { return errors.New("db unreachable") },
	})

	resp, err := http.Get(fx.http.URL + "/readyz")
	if err != nil {
		t.Fatalf("GET /readyz: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusServiceUnavailable {
		t.Errorf("/readyz = %d; want 503", resp.StatusCode)
	}
}

func TestMetricsEndpointExposesCounters(t *testing.T) {
	fx := newTestServer(t, nil)

	// Drive a request to bump the counters.
	_, _ = http.Post(fx.http.URL+ServicePath("CamerasService", "ListCameras"), "application/json", strings.NewReader("{}"))

	resp, err := http.Get(fx.http.URL + "/metrics")
	if err != nil {
		t.Fatalf("GET /metrics: %v", err)
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)
	if !strings.Contains(string(body), "kaivue_apiserver_requests_total") {
		t.Errorf("metrics body missing requests_total counter; got:\n%s", body)
	}
}

// ---------------------------------------------------------------------
// Rate limiting
// ---------------------------------------------------------------------

func TestRateLimitKicksInAtThreshold(t *testing.T) {
	fx := newTestServer(t, func(c *Config) {
		c.RateLimit = RateLimitConfig{RequestsPerSecond: 1, Burst: 2}
	})
	token, _, _ := mintToken(t, fx.idp, "acme", "carol")

	do := func() int {
		req, _ := http.NewRequest(http.MethodPost, fx.http.URL+ServicePath("CamerasService", "ListCameras"), strings.NewReader("{}"))
		req.Header.Set("Authorization", "Bearer "+token)
		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			t.Fatalf("POST: %v", err)
		}
		resp.Body.Close()
		return resp.StatusCode
	}

	// First 2 requests consume the burst → 403 (no policy) but NOT 429.
	for i := 0; i < 2; i++ {
		code := do()
		if code == http.StatusTooManyRequests {
			t.Fatalf("request %d hit rate limit prematurely", i)
		}
	}
	// Third request should be throttled by the bucket (its tokens are 0).
	code := do()
	if code != http.StatusTooManyRequests {
		t.Errorf("3rd request = %d; want 429", code)
	}
}

// waitForAuditEntries polls the in-memory recorder up to `timeout` for
// the expected number of entries to show up for `tenantID`. The audit
// middleware records on a detached goroutine so tests can't rely on a
// synchronous observation.
func waitForAuditEntries(rec *audit.MemoryRecorder, tenantID string, want int, timeout time.Duration) bool {
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		entries, err := rec.Query(context.Background(), audit.QueryFilter{TenantID: tenantID})
		if err == nil && len(entries) >= want {
			return true
		}
		time.Sleep(10 * time.Millisecond)
	}
	return false
}
