package auth_test

import (
	"context"
	"errors"
	"testing"

	"github.com/bluenviron/mediamtx/internal/shared/auth"
	"github.com/bluenviron/mediamtx/internal/shared/auth/fake"
)

// Compile-time check: the fake must satisfy the interface. Duplicated here
// (in addition to fake/fake.go) to fail the auth package's own test build
// if the contract drifts.
var _ auth.IdentityProvider = (*fake.Provider)(nil)

func tenantA() auth.TenantRef {
	return auth.TenantRef{Type: auth.TenantTypeCustomer, ID: "cust-a"}
}

func tenantB() auth.TenantRef {
	return auth.TenantRef{Type: auth.TenantTypeCustomer, ID: "cust-b"}
}

func newFakeWithUser(t *testing.T) (*fake.Provider, auth.TenantRef, *auth.User) {
	t.Helper()
	p := fake.New()
	tenant := tenantA()
	u, err := p.CreateUser(context.Background(), tenant, auth.UserSpec{
		Username: "alice",
		Email:    "alice@example.com",
		Password: "hunter2",
	})
	if err != nil {
		t.Fatalf("CreateUser: %v", err)
	}
	return p, tenant, u
}

func TestAuthenticateLocal_Success(t *testing.T) {
	p, tenant, _ := newFakeWithUser(t)
	sess, err := p.AuthenticateLocal(context.Background(), tenant, "alice", "hunter2")
	if err != nil {
		t.Fatalf("AuthenticateLocal: %v", err)
	}
	if sess.AccessToken == "" || sess.RefreshToken == "" {
		t.Fatalf("expected non-empty tokens")
	}
	if !sess.Tenant.Equal(tenant) {
		t.Fatalf("session tenant mismatch: %#v", sess.Tenant)
	}
}

func TestAuthenticateLocal_WrongPassword_FailsClosed(t *testing.T) {
	p, tenant, _ := newFakeWithUser(t)
	_, err := p.AuthenticateLocal(context.Background(), tenant, "alice", "WRONG")
	if !errors.Is(err, auth.ErrInvalidCredentials) {
		t.Fatalf("expected ErrInvalidCredentials, got %v", err)
	}
}

func TestAuthenticateLocal_UnknownUserSameError(t *testing.T) {
	p, tenant, _ := newFakeWithUser(t)
	_, err := p.AuthenticateLocal(context.Background(), tenant, "ghost", "anything")
	if !errors.Is(err, auth.ErrInvalidCredentials) {
		t.Fatalf("expected ErrInvalidCredentials (no user enumeration), got %v", err)
	}
}

func TestVerifyToken_Roundtrip(t *testing.T) {
	p, tenant, u := newFakeWithUser(t)
	sess, err := p.AuthenticateLocal(context.Background(), tenant, "alice", "hunter2")
	if err != nil {
		t.Fatal(err)
	}
	claims, err := p.VerifyToken(context.Background(), sess.AccessToken)
	if err != nil {
		t.Fatalf("VerifyToken: %v", err)
	}
	if claims.UserID != u.ID {
		t.Fatalf("claims user mismatch: got %s want %s", claims.UserID, u.ID)
	}
	if !claims.TenantRef.Equal(tenant) {
		t.Fatalf("claims tenant mismatch")
	}
	if claims.SessionID != sess.ID {
		t.Fatalf("claims session id mismatch")
	}
}

func TestVerifyToken_BadTokenFailsClosed(t *testing.T) {
	p := fake.New()
	_, err := p.VerifyToken(context.Background(), "not-a-real-token")
	if !errors.Is(err, auth.ErrTokenInvalid) {
		t.Fatalf("expected ErrTokenInvalid, got %v", err)
	}
}

func TestRevokeSession_Idempotent(t *testing.T) {
	p, tenant, _ := newFakeWithUser(t)
	sess, _ := p.AuthenticateLocal(context.Background(), tenant, "alice", "hunter2")

	if err := p.RevokeSession(context.Background(), sess.ID); err != nil {
		t.Fatalf("RevokeSession: %v", err)
	}
	// Idempotent — second call also succeeds.
	if err := p.RevokeSession(context.Background(), sess.ID); err != nil {
		t.Fatalf("RevokeSession (second call): %v", err)
	}
	// And unknown session id is also a nil-error no-op.
	if err := p.RevokeSession(context.Background(), auth.SessionID("does-not-exist")); err != nil {
		t.Fatalf("RevokeSession unknown: %v", err)
	}
	// Token must no longer verify.
	if _, err := p.VerifyToken(context.Background(), sess.AccessToken); !errors.Is(err, auth.ErrTokenInvalid) {
		t.Fatalf("expected revoked token to be invalid, got %v", err)
	}
}

func TestRefreshSession_RotatesTokens(t *testing.T) {
	p, tenant, _ := newFakeWithUser(t)
	sess, _ := p.AuthenticateLocal(context.Background(), tenant, "alice", "hunter2")
	oldRefresh := sess.RefreshToken
	oldAccess := sess.AccessToken

	refreshed, err := p.RefreshSession(context.Background(), oldRefresh)
	if err != nil {
		t.Fatalf("RefreshSession: %v", err)
	}
	if refreshed.RefreshToken == oldRefresh {
		t.Fatal("refresh token should have rotated")
	}
	if refreshed.AccessToken == oldAccess {
		t.Fatal("access token should have rotated")
	}
	// Old refresh token must be unusable now.
	if _, err := p.RefreshSession(context.Background(), oldRefresh); !errors.Is(err, auth.ErrSessionNotFound) {
		t.Fatalf("expected ErrSessionNotFound on reuse, got %v", err)
	}
}

func TestSSOFlow_BeginComplete(t *testing.T) {
	p, tenant, _ := newFakeWithUser(t)

	// Configure an OIDC provider via the wizard path.
	cfg := auth.ProviderConfig{
		Kind:        auth.ProviderKindOIDC,
		DisplayName: "Acme OIDC",
		Enabled:     true,
		OIDC: &auth.OIDCConfig{
			IssuerURL:   "https://idp.example.com",
			ClientID:    "abc",
			RedirectURI: "https://app.example.com/cb",
		},
	}
	if res, err := p.TestProvider(context.Background(), tenant, cfg); err != nil || !res.Success {
		t.Fatalf("TestProvider: %v %#v", err, res)
	}
	if err := p.ConfigureProvider(context.Background(), tenant, cfg); err != nil {
		t.Fatalf("ConfigureProvider: %v", err)
	}

	// Find the persisted provider id by listing — the fake assigns it.
	var providerID auth.ProviderID
	// We can begin via a known id by listing. The fake does not expose
	// list, so we walk a fresh begin against the only id we created;
	// instead, attempt with an obviously wrong id and expect not-found.
	if _, err := p.BeginSSOFlow(context.Background(), tenant, "nope", "https://app/cb"); !errors.Is(err, auth.ErrProviderNotFound) {
		t.Fatalf("expected ErrProviderNotFound, got %v", err)
	}
	// Iterate possible ids: the fake assigns idp_N starting at some N.
	for i := 1; i < 20; i++ {
		candidate := auth.ProviderID(fakeIDPID(i))
		if begin, err := p.BeginSSOFlow(context.Background(), tenant, candidate, "https://app/cb"); err == nil {
			providerID = candidate
			if begin.AuthURL == "" || begin.State == "" {
				t.Fatal("expected AuthURL and State")
			}
			// Complete with a code matching the existing username.
			sess, err := p.CompleteSSOFlow(context.Background(), tenant, begin.State, "alice")
			if err != nil {
				t.Fatalf("CompleteSSOFlow: %v", err)
			}
			if sess.UserID == "" {
				t.Fatal("expected non-empty user id")
			}
			// Replay must fail (single-use state).
			if _, err := p.CompleteSSOFlow(context.Background(), tenant, begin.State, "alice"); !errors.Is(err, auth.ErrSSOStateInvalid) {
				t.Fatalf("expected replay to fail, got %v", err)
			}
			break
		}
	}
	if providerID == "" {
		t.Fatal("could not locate fake provider id")
	}
}

func fakeIDPID(n int) string { return "idp_" + itoa(n) }
func itoa(n int) string {
	if n == 0 {
		return "0"
	}
	var buf [12]byte
	i := len(buf)
	for n > 0 {
		i--
		buf[i] = byte('0' + n%10)
		n /= 10
	}
	return string(buf[i:])
}

func TestConfigureProvider_FailsClosedOnBadConfig(t *testing.T) {
	p := fake.New()
	tenant := tenantA()
	bad := auth.ProviderConfig{Kind: auth.ProviderKindOIDC, OIDC: &auth.OIDCConfig{}}
	if err := p.ConfigureProvider(context.Background(), tenant, bad); !errors.Is(err, auth.ErrProviderTestFailed) {
		t.Fatalf("expected ErrProviderTestFailed, got %v", err)
	}
}

func TestUserCRUD(t *testing.T) {
	p := fake.New()
	tenant := tenantA()
	ctx := context.Background()

	u, err := p.CreateUser(ctx, tenant, auth.UserSpec{Username: "bob", Email: "bob@example.com", Password: "pw"})
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	if u.ID == "" {
		t.Fatal("expected user id")
	}
	// Duplicate username -> ErrUserExists.
	if _, err := p.CreateUser(ctx, tenant, auth.UserSpec{Username: "bob"}); !errors.Is(err, auth.ErrUserExists) {
		t.Fatalf("expected ErrUserExists, got %v", err)
	}

	got, err := p.GetUser(ctx, tenant, u.ID)
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	if got.Username != "bob" {
		t.Fatalf("expected bob, got %s", got.Username)
	}

	// List
	users, err := p.ListUsers(ctx, tenant, auth.ListOptions{})
	if err != nil || len(users) != 1 {
		t.Fatalf("list: err=%v len=%d", err, len(users))
	}

	// Update
	newName := "Bob Updated"
	if _, err := p.UpdateUser(ctx, tenant, u.ID, auth.UserUpdate{DisplayName: &newName}); err != nil {
		t.Fatalf("update: %v", err)
	}

	// Cross-tenant get must surface ErrTenantMismatch, not ErrUserNotFound.
	if _, err := p.GetUser(ctx, tenantB(), u.ID); !errors.Is(err, auth.ErrTenantMismatch) {
		t.Fatalf("expected ErrTenantMismatch, got %v", err)
	}

	// Delete
	if err := p.DeleteUser(ctx, tenant, u.ID); err != nil {
		t.Fatalf("delete: %v", err)
	}
	if _, err := p.GetUser(ctx, tenant, u.ID); !errors.Is(err, auth.ErrUserNotFound) {
		t.Fatalf("expected ErrUserNotFound after delete, got %v", err)
	}
}

func TestGroupMembership(t *testing.T) {
	p := fake.New()
	tenant := tenantA()
	ctx := context.Background()

	u, _ := p.CreateUser(ctx, tenant, auth.UserSpec{Username: "carol", Password: "pw"})
	g, err := p.CreateGroup(tenant, "admins", "tenant admins")
	if err != nil {
		t.Fatalf("create group: %v", err)
	}

	if err := p.AddUserToGroup(ctx, tenant, u.ID, g.ID); err != nil {
		t.Fatalf("add: %v", err)
	}
	// Idempotent add.
	if err := p.AddUserToGroup(ctx, tenant, u.ID, g.ID); err != nil {
		t.Fatalf("add idempotent: %v", err)
	}

	got, _ := p.GetUser(ctx, tenant, u.ID)
	if len(got.Groups) != 1 || got.Groups[0] != g.ID {
		t.Fatalf("expected one group membership, got %#v", got.Groups)
	}

	groups, err := p.ListGroups(ctx, tenant)
	if err != nil || len(groups) != 1 {
		t.Fatalf("list groups: err=%v len=%d", err, len(groups))
	}

	if err := p.RemoveUserFromGroup(ctx, tenant, u.ID, g.ID); err != nil {
		t.Fatalf("remove: %v", err)
	}
	got, _ = p.GetUser(ctx, tenant, u.ID)
	if len(got.Groups) != 0 {
		t.Fatalf("expected zero groups after remove, got %#v", got.Groups)
	}

	// Cross-tenant guard.
	if err := p.AddUserToGroup(ctx, tenantB(), u.ID, g.ID); !errors.Is(err, auth.ErrUserNotFound) {
		t.Fatalf("expected ErrUserNotFound across tenants, got %v", err)
	}
}

func TestTenantIsolation_ListUsers(t *testing.T) {
	p := fake.New()
	ctx := context.Background()
	_, _ = p.CreateUser(ctx, tenantA(), auth.UserSpec{Username: "a"})
	_, _ = p.CreateUser(ctx, tenantB(), auth.UserSpec{Username: "b"})

	a, _ := p.ListUsers(ctx, tenantA(), auth.ListOptions{})
	if len(a) != 1 || a[0].Username != "a" {
		t.Fatalf("tenant A leak: %#v", a)
	}
	b, _ := p.ListUsers(ctx, tenantB(), auth.ListOptions{})
	if len(b) != 1 || b[0].Username != "b" {
		t.Fatalf("tenant B leak: %#v", b)
	}
}
