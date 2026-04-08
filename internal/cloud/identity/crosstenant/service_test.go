package crosstenant_test

import (
	"context"
	"crypto/sha256"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/golang-jwt/jwt/v5"

	"github.com/bluenviron/mediamtx/internal/cloud/audit"
	"github.com/bluenviron/mediamtx/internal/cloud/identity/crosstenant"
	"github.com/bluenviron/mediamtx/internal/cloud/permissions"
	"github.com/bluenviron/mediamtx/internal/shared/auth"
	"github.com/bluenviron/mediamtx/internal/shared/auth/fake"
)

// ---------- test harness ---------------------------------------------------

func testSigningKey(t *testing.T) []byte {
	t.Helper()
	sum := sha256.Sum256([]byte("test-jwt-key-REPLACE_ME"))
	return sum[:]
}

type fixture struct {
	svc          *crosstenant.Service
	identity     *fake.Provider
	relStore     *crosstenant.InMemoryRelationshipStore
	permStore    *permissions.InMemoryRelationshipStore
	sessionStore *crosstenant.InMemorySessionStore
	recorder     *audit.MemoryRecorder
	integrator   auth.TenantRef
	customer     auth.TenantRef
	integratorUser *auth.User
	now          time.Time
}

func newFixture(t *testing.T) *fixture {
	t.Helper()
	integratorTenant := auth.TenantRef{Type: auth.TenantTypeIntegrator, ID: "int-acme"}
	customerTenant := auth.TenantRef{Type: auth.TenantTypeCustomer, ID: "cust-widgets"}

	identity := fake.New()
	// create the integrator user.
	user, err := identity.CreateUser(context.Background(), integratorTenant, auth.UserSpec{
		Username:    "alice",
		Email:       "alice@acme.test",
		DisplayName: "Alice",
		Password:    "hunter2!!",
	})
	if err != nil {
		t.Fatalf("create integrator user: %v", err)
	}

	relStore := crosstenant.NewInMemoryRelationshipStore()
	relStore.Put(crosstenant.RelationshipRecord{
		IntegratorUserID: string(user.ID),
		IntegratorTenant: integratorTenant.ID,
		CustomerTenantID: customerTenant.ID,
		Revoked:          false,
	})

	permStore := permissions.NewInMemoryRelationshipStore()
	permStore.PutDirect(permissions.IntegratorRelationship{
		IntegratorUserID: user.ID,
		CustomerTenant:   customerTenant,
		ScopedActions:    []string{"cameras.read", "cameras.write", "recordings.read"},
	})

	sessStore := crosstenant.NewInMemorySessionStore()
	rec := audit.NewMemoryRecorder()

	now := time.Date(2026, 4, 7, 12, 0, 0, 0, time.UTC)
	seq := 0
	svc, err := crosstenant.NewService(
		crosstenant.Config{
			SigningKey:       testSigningKey(t),
			TTL:              15 * time.Minute,
			IntegratorTenant: integratorTenant,
			Now:              func() time.Time { return now },
			NewSessionID: func() string {
				seq++
				return "sid-" + stringFromInt(seq)
			},
		},
		identity, relStore, permStore, sessStore, rec,
	)
	if err != nil {
		t.Fatalf("NewService: %v", err)
	}
	return &fixture{
		svc:            svc,
		identity:       identity,
		relStore:       relStore,
		permStore:      permStore,
		sessionStore:   sessStore,
		recorder:       rec,
		integrator:     integratorTenant,
		customer:       customerTenant,
		integratorUser: user,
		now:            now,
	}
}

func stringFromInt(i int) string {
	if i == 0 {
		return "0"
	}
	var buf [20]byte
	n := len(buf)
	for i > 0 {
		n--
		buf[n] = byte('0' + i%10)
		i /= 10
	}
	return string(buf[n:])
}

func (f *fixture) entries(t *testing.T, action string) []audit.Entry {
	t.Helper()
	out, err := f.recorder.Query(context.Background(), audit.QueryFilter{
		TenantID:                  f.customer.ID,
		IncludeImpersonatedTenant: true,
	})
	if err != nil {
		t.Fatalf("query audit: %v", err)
	}
	if action == "" {
		return out
	}
	filtered := out[:0]
	for _, e := range out {
		if e.Action == action {
			filtered = append(filtered, e)
		}
	}
	return filtered
}

// ---------- tests ---------------------------------------------------------

func TestMintScopedToken_HappyPath(t *testing.T) {
	f := newFixture(t)
	tok, err := f.svc.MintScopedToken(context.Background(), f.integratorUser.ID, f.customer.ID)
	if err != nil {
		t.Fatalf("mint: %v", err)
	}
	if tok.Token == "" {
		t.Fatal("empty token")
	}
	if tok.CustomerTenantID != f.customer.ID {
		t.Errorf("wrong customer tenant id: %s", tok.CustomerTenantID)
	}
	if tok.SessionID == "" {
		t.Error("empty session id")
	}
	wantScope := []string{"cameras.read", "cameras.write", "recordings.read"}
	if !stringSlicesEqual(tok.PermissionScope, wantScope) {
		t.Errorf("scope mismatch: got %v want %v", tok.PermissionScope, wantScope)
	}
	if !tok.ExpiresAt.Equal(f.now.Add(15 * time.Minute)) {
		t.Errorf("wrong expiry: %v", tok.ExpiresAt)
	}

	// Token parses and carries the right subject.
	parsed, err := jwt.Parse(tok.Token, func(_ *jwt.Token) (interface{}, error) {
		return testSigningKey(t), nil
	}, jwt.WithTimeFunc(func() time.Time { return f.now.Add(time.Second) }))
	if err != nil || !parsed.Valid {
		t.Fatalf("token parse: %v", err)
	}
	mc := parsed.Claims.(jwt.MapClaims)
	if sub, _ := mc["sub"].(string); sub != "integrator:"+string(f.integratorUser.ID)+"@"+f.customer.ID {
		t.Errorf("wrong sub: %s", sub)
	}
}

func TestMintScopedToken_RejectsUnknownIntegrator(t *testing.T) {
	f := newFixture(t)
	_, err := f.svc.MintScopedToken(context.Background(), auth.UserID("nobody"), f.customer.ID)
	if !errors.Is(err, crosstenant.ErrUnknownIntegrator) {
		t.Fatalf("want ErrUnknownIntegrator got %v", err)
	}
}

func TestMintScopedToken_RejectsMissingRelationship(t *testing.T) {
	f := newFixture(t)
	// create a second integrator user that has NO relationship.
	other, err := f.identity.CreateUser(context.Background(), f.integrator, auth.UserSpec{
		Username: "bob", Email: "bob@acme.test", DisplayName: "Bob", Password: "hunter2!!",
	})
	if err != nil {
		t.Fatalf("create user: %v", err)
	}
	_, err = f.svc.MintScopedToken(context.Background(), other.ID, f.customer.ID)
	if !errors.Is(err, crosstenant.ErrNoRelationship) {
		t.Fatalf("want ErrNoRelationship got %v", err)
	}
}

func TestMintScopedToken_RejectsRevokedRelationship(t *testing.T) {
	f := newFixture(t)
	f.relStore.Put(crosstenant.RelationshipRecord{
		IntegratorUserID: string(f.integratorUser.ID),
		IntegratorTenant: f.integrator.ID,
		CustomerTenantID: f.customer.ID,
		Revoked:          true,
	})
	_, err := f.svc.MintScopedToken(context.Background(), f.integratorUser.ID, f.customer.ID)
	if !errors.Is(err, crosstenant.ErrRelationshipRevoked) {
		t.Fatalf("want ErrRelationshipRevoked got %v", err)
	}
}

func TestMintScopedToken_NarrowsViaSubResellerParentChain(t *testing.T) {
	f := newFixture(t)
	// Child grants everything, parent grants only cameras.read.
	f.permStore = permissions.NewInMemoryRelationshipStore()
	f.permStore.PutDirect(permissions.IntegratorRelationship{
		IntegratorUserID: f.integratorUser.ID,
		CustomerTenant:   f.customer,
		ParentIntegrator: "parent-1",
		ScopedActions:    []string{"cameras.read", "cameras.write", "recordings.read"},
	})
	f.permStore.PutParent("parent-1", permissions.IntegratorRelationship{
		IntegratorUserID: "root",
		CustomerTenant:   f.customer,
		ScopedActions:    []string{"cameras.read"},
	})
	svc, err := crosstenant.NewService(
		crosstenant.Config{
			SigningKey:       testSigningKey(t),
			IntegratorTenant: f.integrator,
			Now:              func() time.Time { return f.now },
			NewSessionID:     func() string { return "sid-narrow" },
		},
		f.identity, f.relStore, f.permStore, f.sessionStore, f.recorder,
	)
	if err != nil {
		t.Fatalf("new svc: %v", err)
	}
	tok, err := svc.MintScopedToken(context.Background(), f.integratorUser.ID, f.customer.ID)
	if err != nil {
		t.Fatalf("mint: %v", err)
	}
	want := []string{"cameras.read"}
	if !stringSlicesEqual(tok.PermissionScope, want) {
		t.Errorf("expected intersected scope %v, got %v", want, tok.PermissionScope)
	}
}

func TestMintScopedToken_EmitsAuditEntry(t *testing.T) {
	f := newFixture(t)
	tok, err := f.svc.MintScopedToken(context.Background(), f.integratorUser.ID, f.customer.ID)
	if err != nil {
		t.Fatalf("mint: %v", err)
	}
	entries := f.entries(t, "permissions.cross_tenant_grant")
	if len(entries) != 1 {
		t.Fatalf("want 1 audit entry got %d", len(entries))
	}
	e := entries[0]
	if e.Result != audit.ResultAllow {
		t.Errorf("want allow got %s", e.Result)
	}
	if e.ActorAgent != audit.AgentIntegrator {
		t.Errorf("want agent integrator got %s", e.ActorAgent)
	}
	if e.ResourceID != tok.SessionID {
		t.Errorf("want resource id %q got %q", tok.SessionID, e.ResourceID)
	}
	if e.ImpersonatedTenantID == nil || *e.ImpersonatedTenantID != f.customer.ID {
		t.Errorf("missing impersonated tenant id")
	}
}

func TestVerifyScopedToken_HappyPath(t *testing.T) {
	f := newFixture(t)
	tok, err := f.svc.MintScopedToken(context.Background(), f.integratorUser.ID, f.customer.ID)
	if err != nil {
		t.Fatalf("mint: %v", err)
	}
	claims, err := f.svc.VerifyScopedToken(context.Background(), tok.Token)
	if err != nil {
		t.Fatalf("verify: %v", err)
	}
	if claims.IntegratorUserID != f.integratorUser.ID {
		t.Errorf("integrator uid mismatch")
	}
	if claims.CustomerTenant.ID != f.customer.ID {
		t.Errorf("customer tenant id mismatch")
	}
	if !strings.HasPrefix(claims.Subject, "integrator:") {
		t.Errorf("wrong subject prefix %q", claims.Subject)
	}
	verifyEntries := f.entries(t, "permissions.cross_tenant_verify")
	if len(verifyEntries) != 1 {
		t.Fatalf("want 1 verify audit entry got %d", len(verifyEntries))
	}
}

func TestVerifyScopedToken_RejectsExpiredToken(t *testing.T) {
	f := newFixture(t)
	tok, err := f.svc.MintScopedToken(context.Background(), f.integratorUser.ID, f.customer.ID)
	if err != nil {
		t.Fatalf("mint: %v", err)
	}
	// advance clock past expiry.
	future := f.now.Add(30 * time.Minute)
	svc2, err := crosstenant.NewService(
		crosstenant.Config{
			SigningKey:       testSigningKey(t),
			IntegratorTenant: f.integrator,
			Now:              func() time.Time { return future },
			NewSessionID:     func() string { return "ignored" },
		},
		f.identity, f.relStore, f.permStore, f.sessionStore, f.recorder,
	)
	if err != nil {
		t.Fatalf("new svc: %v", err)
	}
	_, err = svc2.VerifyScopedToken(context.Background(), tok.Token)
	if !errors.Is(err, crosstenant.ErrScopedTokenExpired) {
		t.Fatalf("want ErrScopedTokenExpired got %v", err)
	}
}

func TestVerifyScopedToken_RejectsRevokedSession(t *testing.T) {
	f := newFixture(t)
	tok, err := f.svc.MintScopedToken(context.Background(), f.integratorUser.ID, f.customer.ID)
	if err != nil {
		t.Fatalf("mint: %v", err)
	}
	if err := f.svc.RevokeScopedSession(context.Background(), tok.SessionID); err != nil {
		t.Fatalf("revoke: %v", err)
	}
	_, err = f.svc.VerifyScopedToken(context.Background(), tok.Token)
	if !errors.Is(err, crosstenant.ErrSessionRevoked) {
		t.Fatalf("want ErrSessionRevoked got %v", err)
	}
}

func TestVerifyScopedToken_RejectsWrongSigningKey(t *testing.T) {
	f := newFixture(t)
	tok, err := f.svc.MintScopedToken(context.Background(), f.integratorUser.ID, f.customer.ID)
	if err != nil {
		t.Fatalf("mint: %v", err)
	}
	// Service configured with DIFFERENT signing key.
	otherKey := sha256.Sum256([]byte("other-key-REPLACE_ME"))
	svc2, err := crosstenant.NewService(
		crosstenant.Config{
			SigningKey:       otherKey[:],
			IntegratorTenant: f.integrator,
			Now:              func() time.Time { return f.now },
			NewSessionID:     func() string { return "ignored" },
		},
		f.identity, f.relStore, f.permStore, f.sessionStore, f.recorder,
	)
	if err != nil {
		t.Fatalf("new svc: %v", err)
	}
	_, err = svc2.VerifyScopedToken(context.Background(), tok.Token)
	if !errors.Is(err, crosstenant.ErrScopedTokenInvalid) {
		t.Fatalf("want ErrScopedTokenInvalid got %v", err)
	}
}

func TestChaos_ThreeLevelSubResellerHierarchyIntersection(t *testing.T) {
	f := newFixture(t)
	// Build a 3-level chain: root → mid → leaf(staff user)
	//   root: cameras.*, recordings.*, users.*
	//   mid:  cameras.*, recordings.*
	//   leaf: cameras.read, cameras.write, recordings.read, users.read
	// Expected intersection: cameras.read, cameras.write, recordings.read
	f.permStore = permissions.NewInMemoryRelationshipStore()
	f.permStore.PutDirect(permissions.IntegratorRelationship{
		IntegratorUserID: f.integratorUser.ID,
		CustomerTenant:   f.customer,
		ParentIntegrator: "mid-1",
		ScopedActions:    []string{"cameras.read", "cameras.write", "recordings.read", "users.read"},
	})
	f.permStore.PutParent("mid-1", permissions.IntegratorRelationship{
		IntegratorUserID: "mid",
		CustomerTenant:   f.customer,
		ParentIntegrator: "root-1",
		ScopedActions:    []string{"cameras.read", "cameras.write", "cameras.delete", "recordings.read", "recordings.write"},
	})
	f.permStore.PutParent("root-1", permissions.IntegratorRelationship{
		IntegratorUserID: "root",
		CustomerTenant:   f.customer,
		ScopedActions:    []string{"cameras.read", "cameras.write", "cameras.delete", "recordings.read", "recordings.write", "users.read", "users.write"},
	})
	svc, err := crosstenant.NewService(
		crosstenant.Config{
			SigningKey:       testSigningKey(t),
			IntegratorTenant: f.integrator,
			Now:              func() time.Time { return f.now },
			NewSessionID:     func() string { return "sid-chaos" },
		},
		f.identity, f.relStore, f.permStore, f.sessionStore, f.recorder,
	)
	if err != nil {
		t.Fatalf("new svc: %v", err)
	}
	tok, err := svc.MintScopedToken(context.Background(), f.integratorUser.ID, f.customer.ID)
	if err != nil {
		t.Fatalf("mint: %v", err)
	}
	want := []string{"cameras.read", "cameras.write", "recordings.read"}
	if !stringSlicesEqual(tok.PermissionScope, want) {
		t.Errorf("chaos intersection mismatch: got %v want %v", tok.PermissionScope, want)
	}

	// Verify the minted token carries ONLY the intersection.
	claims, err := svc.VerifyScopedToken(context.Background(), tok.Token)
	if err != nil {
		t.Fatalf("verify: %v", err)
	}
	if !stringSlicesEqual(claims.PermissionScope, want) {
		t.Errorf("verify scope mismatch: got %v want %v", claims.PermissionScope, want)
	}
	// Confirm disallowed actions are absent.
	for _, denied := range []string{"cameras.delete", "recordings.write", "users.read", "users.write"} {
		for _, got := range claims.PermissionScope {
			if got == denied {
				t.Errorf("denied action %q leaked into scope", denied)
			}
		}
	}
}

func TestMintScopedToken_EmptyScopeFailsClosed(t *testing.T) {
	f := newFixture(t)
	// Overwrite permission store: leaf allows cameras.read, parent allows
	// only users.write — intersection is empty.
	f.permStore = permissions.NewInMemoryRelationshipStore()
	f.permStore.PutDirect(permissions.IntegratorRelationship{
		IntegratorUserID: f.integratorUser.ID,
		CustomerTenant:   f.customer,
		ParentIntegrator: "parent-1",
		ScopedActions:    []string{"cameras.read"},
	})
	f.permStore.PutParent("parent-1", permissions.IntegratorRelationship{
		IntegratorUserID: "root",
		CustomerTenant:   f.customer,
		ScopedActions:    []string{"users.write"},
	})
	svc, err := crosstenant.NewService(
		crosstenant.Config{
			SigningKey:       testSigningKey(t),
			IntegratorTenant: f.integrator,
			Now:              func() time.Time { return f.now },
			NewSessionID:     func() string { return "sid-empty" },
		},
		f.identity, f.relStore, f.permStore, f.sessionStore, f.recorder,
	)
	if err != nil {
		t.Fatalf("new svc: %v", err)
	}
	_, err = svc.MintScopedToken(context.Background(), f.integratorUser.ID, f.customer.ID)
	if !errors.Is(err, crosstenant.ErrEmptyScope) {
		t.Fatalf("want ErrEmptyScope got %v", err)
	}
}

// ---------- helpers --------------------------------------------------------

func stringSlicesEqual(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}
