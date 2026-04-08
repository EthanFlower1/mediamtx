// Package testutil provides shared test fixtures for the cloud isolation,
// chaos, and Casbin policy test packages (KAI-235).
//
// All helpers operate with real in-process dependencies (in-memory DB,
// in-memory IdP, in-memory audit recorder) so there is no external
// infrastructure requirement: these tests run on any CI runner that has Go.
package testutil

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/bluenviron/mediamtx/internal/cloud/apiserver"
	"github.com/bluenviron/mediamtx/internal/cloud/audit"
	"github.com/bluenviron/mediamtx/internal/cloud/db"
	"github.com/bluenviron/mediamtx/internal/cloud/permissions"
	"github.com/bluenviron/mediamtx/internal/shared/auth"
	fakeauth "github.com/bluenviron/mediamtx/internal/shared/auth/fake"
)

// Fixture is a fully wired apiserver backed by in-memory dependencies. Every
// isolation and chaos test creates its own Fixture so there is zero shared
// mutable state between parallel sub-tests.
type Fixture struct {
	Server   *apiserver.Server
	HTTP     *httptest.Server
	IDP      *fakeauth.Provider
	Audit    *audit.MemoryRecorder
	Enforcer *permissions.Enforcer
	Store    *permissions.InMemoryStore
	DB       *db.DB
}

// NewFixture constructs a Fixture and registers cleanup with t.
func NewFixture(t *testing.T) *Fixture {
	t.Helper()

	dbHandle, err := db.Open(context.Background(), "sqlite://:memory:")
	if err != nil {
		t.Fatalf("testutil.NewFixture: open db: %v", err)
	}
	t.Cleanup(func() { _ = dbHandle.Close() })

	idp := fakeauth.New()
	recorder := audit.NewMemoryRecorder()
	store := permissions.NewInMemoryStore()
	enforcer, err := permissions.NewEnforcer(store, nil)
	if err != nil {
		t.Fatalf("testutil.NewFixture: new enforcer: %v", err)
	}

	cfg := apiserver.Config{
		ListenAddr:      ":0",
		Region:          "us-east-2",
		DB:              dbHandle,
		Identity:        idp,
		Enforcer:        enforcer,
		AuditRecorder:   recorder,
		ShutdownTimeout: time.Second,
	}
	srv, err := apiserver.New(cfg)
	if err != nil {
		t.Fatalf("testutil.NewFixture: new server: %v", err)
	}

	ts := httptest.NewServer(srv.Handler())
	t.Cleanup(ts.Close)

	return &Fixture{
		Server:   srv,
		HTTP:     ts,
		IDP:      idp,
		Audit:    recorder,
		Enforcer: enforcer,
		Store:    store,
		DB:       dbHandle,
	}
}

// TenantSession holds a minted token and the resolved tenant for one actor.
type TenantSession struct {
	Token    string
	Tenant   auth.TenantRef
	UserID   auth.UserID
	Username string
}

// MintSession creates a user in tenantID and returns a live session token.
// The caller decides the username so it is unique within a test.
func MintSession(t *testing.T, fx *Fixture, tenantID, username string) TenantSession {
	t.Helper()
	ctx := context.Background()
	tenant := auth.TenantRef{Type: auth.TenantTypeCustomer, ID: tenantID}
	user, err := fx.IDP.CreateUser(ctx, tenant, auth.UserSpec{
		Username: username,
		Email:    username + "@test.local",
		Password: "pw",
	})
	if err != nil {
		t.Fatalf("MintSession: create user %q in %q: %v", username, tenantID, err)
	}
	sess, err := fx.IDP.AuthenticateLocal(ctx, tenant, username, "pw")
	if err != nil {
		t.Fatalf("MintSession: authenticate %q: %v", username, err)
	}
	return TenantSession{
		Token:    sess.AccessToken,
		Tenant:   tenant,
		UserID:   user.ID,
		Username: username,
	}
}

// GrantAll seeds a full allow policy for the session's user across all
// resource types that are wired into defaultRouteAuthorizations. It exists so
// tests can set up a "legitimate user A" baseline quickly.
func GrantAll(t *testing.T, fx *Fixture, sess TenantSession) {
	t.Helper()
	resources := []struct {
		resourceType string
		actions      []string
	}{
		{"cameras", []string{"create", "read", "update", "delete"}},
		{"streams", []string{"mint", "revoke"}},
		{"recorders", []string{"control"}},
		{"directory", []string{"ingest"}},
		{"cross_tenant", []string{"read", "mint"}},
		{"tenants", []string{"create", "suspend"}},
	}
	for _, r := range resources {
		for _, action := range r.actions {
			rule := permissions.PolicyRule{
				Sub: permissions.NewUserSubject(sess.UserID, sess.Tenant).String(),
				Obj: permissions.NewObjectAll(sess.Tenant, r.resourceType).String(),
				Act: action,
				Eft: "allow",
			}
			if err := fx.Enforcer.AddPolicy(rule); err != nil {
				t.Fatalf("GrantAll: add policy: %v", err)
			}
		}
	}
}

// DoRequest fires a POST (the wire method for Connect-Go) to path with an
// optional bearer token. It returns the http.Response; callers must close the
// body.
func DoRequest(t *testing.T, fx *Fixture, path, token string) *http.Response {
	t.Helper()
	req, err := http.NewRequest(http.MethodPost, fx.HTTP.URL+path, http.NoBody)
	if err != nil {
		t.Fatalf("DoRequest: new request: %v", err)
	}
	req.Header.Set("Content-Type", "application/json")
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("DoRequest: do: %v", err)
	}
	return resp
}

// WaitForAuditDeny polls the in-memory recorder until at least `want` deny
// entries appear for tenantID, or returns false on timeout. The audit
// middleware records on a detached goroutine.
func WaitForAuditDeny(rec *audit.MemoryRecorder, tenantID string, want int, timeout time.Duration) bool {
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		entries, err := rec.Query(context.Background(), audit.QueryFilter{TenantID: tenantID})
		if err == nil {
			count := 0
			for _, e := range entries {
				if e.Result == audit.ResultDeny {
					count++
				}
			}
			if count >= want {
				return true
			}
		}
		time.Sleep(10 * time.Millisecond)
	}
	return false
}

// UniqueTenantID returns a tenant ID unique within a test run. It encodes
// the test name and a counter so parallel sub-tests don't collide.
func UniqueTenantID(base string, n int) string {
	return fmt.Sprintf("%s-%d", base, n)
}
