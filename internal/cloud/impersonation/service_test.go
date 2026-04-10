package impersonation_test

import (
	"context"
	"crypto/sha256"
	"errors"
	"fmt"
	"testing"
	"time"

	"github.com/bluenviron/mediamtx/internal/cloud/audit"
	"github.com/bluenviron/mediamtx/internal/cloud/identity/crosstenant"
	"github.com/bluenviron/mediamtx/internal/cloud/impersonation"
	"github.com/bluenviron/mediamtx/internal/shared/auth"
	"github.com/bluenviron/mediamtx/internal/shared/auth/fake"
	"github.com/bluenviron/mediamtx/internal/shared/permissions"
)

// ---------- test harness ---------------------------------------------------

func testSigningKey(t *testing.T) []byte {
	t.Helper()
	sum := sha256.Sum256([]byte("test-impersonation-key-REPLACE_ME"))
	return sum[:]
}

type fixture struct {
	svc          *impersonation.Service
	identity     *fake.Provider
	relStore     *crosstenant.InMemoryRelationshipStore
	permStore    *permissions.InMemoryRelationshipStore
	sessionStore *impersonation.InMemorySessionStore
	grantStore   *impersonation.InMemoryGrantStore
	recorder     *audit.MemoryRecorder
	notifier     *impersonation.NoopNotificationSender
	integrator   auth.TenantRef
	customer     auth.TenantRef
	now          time.Time
}

func newFixture(t *testing.T) *fixture {
	t.Helper()
	integratorTenant := auth.TenantRef{Type: auth.TenantTypeIntegrator, ID: "int-acme"}
	customerTenant := auth.TenantRef{Type: auth.TenantTypeCustomer, ID: "cust-widgets"}

	identity := fake.New()
	// Create integrator user.
	user, err := identity.CreateUser(context.Background(), integratorTenant, auth.UserSpec{
		Username:    "alice",
		Email:       "alice@acme.test",
		DisplayName: "Alice Integrator",
		Password:    "hunter2!!",
	})
	if err != nil {
		t.Fatalf("create integrator user: %v", err)
	}

	// Create customer admin user.
	_, err = identity.CreateUser(context.Background(), customerTenant, auth.UserSpec{
		Username:    "bob",
		Email:       "bob@widgets.test",
		DisplayName: "Bob Admin",
		Password:    "securepass!!",
	})
	if err != nil {
		t.Fatalf("create customer admin user: %v", err)
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
		ScopedActions: []string{
			permissions.ActionViewLive,
			permissions.ActionViewPlayback,
			permissions.ActionViewThumbnails,
			permissions.ActionCamerasEdit,
			permissions.ActionSystemHealth,
			permissions.ActionAuditRead,
		},
	})

	sessStore := impersonation.NewInMemorySessionStore()
	grantStore := impersonation.NewInMemoryGrantStore()
	rec := audit.NewMemoryRecorder()
	notifier := impersonation.NewNoopNotificationSender()

	now := time.Date(2026, 4, 10, 12, 0, 0, 0, time.UTC)
	seq := 0
	svc, err := impersonation.NewService(
		impersonation.Config{
			SigningKey:            testSigningKey(t),
			DefaultSessionTimeout: 30 * time.Minute,
			DefaultGrantTTL:       1 * time.Hour,
			Now:                   func() time.Time { return now },
			NewID: func() string {
				seq++
				return fmt.Sprintf("id-%d", seq)
			},
		},
		identity, relStore, permStore, sessStore, grantStore, rec, notifier,
	)
	if err != nil {
		t.Fatalf("NewService: %v", err)
	}
	return &fixture{
		svc:          svc,
		identity:     identity,
		relStore:     relStore,
		permStore:    permStore,
		sessionStore: sessStore,
		grantStore:   grantStore,
		recorder:     rec,
		notifier:     notifier,
		integrator:   integratorTenant,
		customer:     customerTenant,
		now:          now,
	}
}

func (f *fixture) integratorUserID() string {
	users, _ := f.identity.ListUsers(context.Background(), f.integrator, auth.ListOptions{})
	if len(users) == 0 {
		return ""
	}
	return string(users[0].ID)
}

func (f *fixture) customerAdminID() string {
	users, _ := f.identity.ListUsers(context.Background(), f.customer, auth.ListOptions{})
	if len(users) == 0 {
		return ""
	}
	return string(users[0].ID)
}

// ---------- NewService validation tests ------------------------------------

func TestNewService_RequiresDependencies(t *testing.T) {
	key := testSigningKey(t)
	id := fake.New()
	rs := crosstenant.NewInMemoryRelationshipStore()
	ps := permissions.NewInMemoryRelationshipStore()
	ss := impersonation.NewInMemorySessionStore()
	gs := impersonation.NewInMemoryGrantStore()
	ar := audit.NewMemoryRecorder()
	ns := impersonation.NewNoopNotificationSender()

	tests := []struct {
		name string
		cfg  impersonation.Config
		id   auth.IdentityProvider
		rs   crosstenant.RelationshipStore
		ps   permissions.IntegratorRelationshipStore
		ss   impersonation.SessionStore
		gs   impersonation.GrantStore
		ar   audit.Recorder
		ns   impersonation.NotificationSender
		want string
	}{
		{"missing signing key", impersonation.Config{}, id, rs, ps, ss, gs, ar, ns, "signing key"},
		{"missing identity", impersonation.Config{SigningKey: key}, nil, rs, ps, ss, gs, ar, ns, "identity provider"},
		{"missing relationship store", impersonation.Config{SigningKey: key}, id, nil, ps, ss, gs, ar, ns, "relationship store"},
		{"missing permission store", impersonation.Config{SigningKey: key}, id, rs, nil, ss, gs, ar, ns, "permission store"},
		{"missing session store", impersonation.Config{SigningKey: key}, id, rs, ps, nil, gs, ar, ns, "session store"},
		{"missing grant store", impersonation.Config{SigningKey: key}, id, rs, ps, ss, nil, ar, ns, "grant store"},
		{"missing audit recorder", impersonation.Config{SigningKey: key}, id, rs, ps, ss, gs, nil, ns, "audit recorder"},
		{"missing notifier", impersonation.Config{SigningKey: key}, id, rs, ps, ss, gs, ar, nil, "notification sender"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			_, err := impersonation.NewService(tc.cfg, tc.id, tc.rs, tc.ps, tc.ss, tc.gs, tc.ar, tc.ns)
			if err == nil {
				t.Fatal("expected error, got nil")
			}
			if got := err.Error(); !contains(got, tc.want) {
				t.Fatalf("error %q should mention %q", got, tc.want)
			}
		})
	}
}

// ---------- Authorization Grant tests --------------------------------------

func TestCreateAuthorizationGrant_Success(t *testing.T) {
	f := newFixture(t)
	ctx := context.Background()

	grant, err := f.svc.CreateAuthorizationGrant(ctx, impersonation.CreateGrantRequest{
		TenantID:        f.customer.ID,
		GrantedByUserID: f.customerAdminID(),
		Reason:          "Support ticket #12345",
	})
	if err != nil {
		t.Fatalf("CreateAuthorizationGrant: %v", err)
	}

	if grant.GrantID == "" {
		t.Fatal("grant ID should not be empty")
	}
	if grant.TenantID != f.customer.ID {
		t.Fatalf("tenant mismatch: got %q want %q", grant.TenantID, f.customer.ID)
	}
	if grant.Consumed {
		t.Fatal("grant should not be consumed yet")
	}
	if grant.Revoked {
		t.Fatal("grant should not be revoked")
	}

	// Verify audit entry was created.
	entries, err := f.recorder.Query(ctx, audit.QueryFilter{
		TenantID:     f.customer.ID,
		ActionPattern: impersonation.AuditActionImpersonationGrantCreate,
	})
	if err != nil {
		t.Fatalf("query audit: %v", err)
	}
	if len(entries) != 1 {
		t.Fatalf("expected 1 audit entry, got %d", len(entries))
	}
	if entries[0].ResourceID != grant.GrantID {
		t.Fatalf("audit resource_id mismatch: got %q want %q", entries[0].ResourceID, grant.GrantID)
	}
}

func TestCreateAuthorizationGrant_RequiresTenantID(t *testing.T) {
	f := newFixture(t)
	_, err := f.svc.CreateAuthorizationGrant(context.Background(), impersonation.CreateGrantRequest{
		GrantedByUserID: "some-user",
	})
	if err == nil {
		t.Fatal("expected error for missing tenant_id")
	}
}

func TestRevokeAuthorizationGrant(t *testing.T) {
	f := newFixture(t)
	ctx := context.Background()

	grant, err := f.svc.CreateAuthorizationGrant(ctx, impersonation.CreateGrantRequest{
		TenantID:        f.customer.ID,
		GrantedByUserID: f.customerAdminID(),
		Reason:          "Support ticket #12345",
	})
	if err != nil {
		t.Fatalf("create grant: %v", err)
	}

	err = f.svc.RevokeAuthorizationGrant(ctx, grant.GrantID, f.customerAdminID())
	if err != nil {
		t.Fatalf("revoke grant: %v", err)
	}

	// Second revoke is idempotent.
	err = f.svc.RevokeAuthorizationGrant(ctx, grant.GrantID, f.customerAdminID())
	if err != nil {
		t.Fatalf("idempotent revoke: %v", err)
	}

	// Verify revoke audit entry.
	entries, err := f.recorder.Query(ctx, audit.QueryFilter{
		TenantID:     f.customer.ID,
		ActionPattern: impersonation.AuditActionImpersonationGrantRevoke,
	})
	if err != nil {
		t.Fatalf("query audit: %v", err)
	}
	if len(entries) != 1 {
		t.Fatalf("expected 1 revoke audit entry, got %d", len(entries))
	}
}

func TestRevokeAuthorizationGrant_NotFound(t *testing.T) {
	f := newFixture(t)
	err := f.svc.RevokeAuthorizationGrant(context.Background(), "nonexistent", "user")
	if !errors.Is(err, impersonation.ErrGrantNotFound) {
		t.Fatalf("expected ErrGrantNotFound, got %v", err)
	}
}

// ---------- Integrator impersonation tests ----------------------------------

func TestCreateSession_Integrator_Success(t *testing.T) {
	f := newFixture(t)
	ctx := context.Background()

	sess, err := f.svc.CreateSession(ctx, impersonation.CreateSessionRequest{
		Mode:                  impersonation.ModeIntegrator,
		ImpersonatingUserID:   f.integratorUserID(),
		ImpersonatingTenantID: f.integrator.ID,
		ImpersonatedTenantID:  f.customer.ID,
	})
	if err != nil {
		t.Fatalf("CreateSession: %v", err)
	}

	if sess.SessionID == "" {
		t.Fatal("session ID should not be empty")
	}
	if sess.Mode != impersonation.ModeIntegrator {
		t.Fatalf("mode: got %q want %q", sess.Mode, impersonation.ModeIntegrator)
	}
	if sess.Status != impersonation.StatusActive {
		t.Fatalf("status: got %q want %q", sess.Status, impersonation.StatusActive)
	}
	if len(sess.ScopedPermissions) == 0 {
		t.Fatal("scoped permissions should not be empty")
	}

	// Verify admin actions are stripped.
	for _, perm := range sess.ScopedPermissions {
		if impersonation.AdminActions[perm] {
			t.Fatalf("admin action %q should be stripped from impersonation scope", perm)
		}
	}

	// Verify audit entry.
	entries, err := f.recorder.Query(ctx, audit.QueryFilter{
		TenantID:     f.customer.ID,
		ActionPattern: impersonation.AuditActionImpersonationStart,
	})
	if err != nil {
		t.Fatalf("query audit: %v", err)
	}
	if len(entries) != 1 {
		t.Fatalf("expected 1 start audit entry, got %d", len(entries))
	}
	if entries[0].ImpersonatingUserID == nil || *entries[0].ImpersonatingUserID != f.integratorUserID() {
		t.Fatal("audit entry should have impersonating_user_id set")
	}
	if entries[0].ImpersonatedTenantID == nil || *entries[0].ImpersonatedTenantID != f.customer.ID {
		t.Fatal("audit entry should have impersonated_tenant_id set")
	}

	// Verify notification was sent.
	startCalls := f.notifier.GetStartCalls()
	if len(startCalls) != 1 {
		t.Fatalf("expected 1 start notification, got %d", len(startCalls))
	}
}

func TestCreateSession_Integrator_NoRelationship(t *testing.T) {
	f := newFixture(t)
	_, err := f.svc.CreateSession(context.Background(), impersonation.CreateSessionRequest{
		Mode:                  impersonation.ModeIntegrator,
		ImpersonatingUserID:   "unknown-user",
		ImpersonatingTenantID: f.integrator.ID,
		ImpersonatedTenantID:  f.customer.ID,
	})
	if !errors.Is(err, impersonation.ErrNoRelationship) {
		t.Fatalf("expected ErrNoRelationship, got %v", err)
	}
}

func TestCreateSession_Integrator_RevokedRelationship(t *testing.T) {
	f := newFixture(t)

	// Revoke the relationship.
	f.relStore.Put(crosstenant.RelationshipRecord{
		IntegratorUserID: f.integratorUserID(),
		IntegratorTenant: f.integrator.ID,
		CustomerTenantID: f.customer.ID,
		Revoked:          true,
	})

	_, err := f.svc.CreateSession(context.Background(), impersonation.CreateSessionRequest{
		Mode:                  impersonation.ModeIntegrator,
		ImpersonatingUserID:   f.integratorUserID(),
		ImpersonatingTenantID: f.integrator.ID,
		ImpersonatedTenantID:  f.customer.ID,
	})
	if !errors.Is(err, impersonation.ErrRelationshipRevoked) {
		t.Fatalf("expected ErrRelationshipRevoked, got %v", err)
	}
}

// ---------- Platform Support impersonation tests ---------------------------

func TestCreateSession_PlatformSupport_Success(t *testing.T) {
	f := newFixture(t)
	ctx := context.Background()

	// Customer admin creates a grant.
	grant, err := f.svc.CreateAuthorizationGrant(ctx, impersonation.CreateGrantRequest{
		TenantID:        f.customer.ID,
		GrantedByUserID: f.customerAdminID(),
		Reason:          "Ticket #42",
		MaxDuration:     15 * time.Minute,
	})
	if err != nil {
		t.Fatalf("create grant: %v", err)
	}

	// Platform support creates an impersonation session.
	sess, err := f.svc.CreateSession(ctx, impersonation.CreateSessionRequest{
		Mode:                  impersonation.ModePlatformSupport,
		ImpersonatingUserID:   "support-agent-1",
		ImpersonatingTenantID: "platform",
		ImpersonatedTenantID:  f.customer.ID,
		AuthorizationGrantID:  grant.GrantID,
	})
	if err != nil {
		t.Fatalf("CreateSession: %v", err)
	}

	if sess.Mode != impersonation.ModePlatformSupport {
		t.Fatalf("mode: got %q want %q", sess.Mode, impersonation.ModePlatformSupport)
	}
	if len(sess.ScopedPermissions) == 0 {
		t.Fatal("scoped permissions should not be empty")
	}

	// Verify admin actions are stripped.
	for _, perm := range sess.ScopedPermissions {
		if impersonation.AdminActions[perm] {
			t.Fatalf("admin action %q should be stripped", perm)
		}
	}

	// Verify the grant is consumed.
	stored, ok, _ := f.grantStore.GetGrant(ctx, grant.GrantID)
	if !ok {
		t.Fatal("grant should still exist")
	}
	if !stored.Consumed {
		t.Fatal("grant should be consumed after session creation")
	}
}

func TestCreateSession_PlatformSupport_NoGrant(t *testing.T) {
	f := newFixture(t)
	_, err := f.svc.CreateSession(context.Background(), impersonation.CreateSessionRequest{
		Mode:                  impersonation.ModePlatformSupport,
		ImpersonatingUserID:   "support-agent-1",
		ImpersonatingTenantID: "platform",
		ImpersonatedTenantID:  f.customer.ID,
		// No AuthorizationGrantID
	})
	if !errors.Is(err, impersonation.ErrMissingGrant) {
		t.Fatalf("expected ErrMissingGrant, got %v", err)
	}
}

func TestCreateSession_PlatformSupport_GrantNotFound(t *testing.T) {
	f := newFixture(t)
	_, err := f.svc.CreateSession(context.Background(), impersonation.CreateSessionRequest{
		Mode:                  impersonation.ModePlatformSupport,
		ImpersonatingUserID:   "support-agent-1",
		ImpersonatingTenantID: "platform",
		ImpersonatedTenantID:  f.customer.ID,
		AuthorizationGrantID:  "nonexistent",
	})
	if !errors.Is(err, impersonation.ErrGrantNotFound) {
		t.Fatalf("expected ErrGrantNotFound, got %v", err)
	}
}

func TestCreateSession_PlatformSupport_GrantAlreadyConsumed(t *testing.T) {
	f := newFixture(t)
	ctx := context.Background()

	grant, _ := f.svc.CreateAuthorizationGrant(ctx, impersonation.CreateGrantRequest{
		TenantID:        f.customer.ID,
		GrantedByUserID: f.customerAdminID(),
		Reason:          "Ticket",
	})

	// First session succeeds.
	_, err := f.svc.CreateSession(ctx, impersonation.CreateSessionRequest{
		Mode:                  impersonation.ModePlatformSupport,
		ImpersonatingUserID:   "support-1",
		ImpersonatingTenantID: "platform",
		ImpersonatedTenantID:  f.customer.ID,
		AuthorizationGrantID:  grant.GrantID,
	})
	if err != nil {
		t.Fatalf("first session: %v", err)
	}

	// Second session with same grant fails.
	_, err = f.svc.CreateSession(ctx, impersonation.CreateSessionRequest{
		Mode:                  impersonation.ModePlatformSupport,
		ImpersonatingUserID:   "support-2",
		ImpersonatingTenantID: "platform",
		ImpersonatedTenantID:  f.customer.ID,
		AuthorizationGrantID:  grant.GrantID,
	})
	if !errors.Is(err, impersonation.ErrGrantConsumed) {
		t.Fatalf("expected ErrGrantConsumed, got %v", err)
	}
}

func TestCreateSession_PlatformSupport_GrantRevoked(t *testing.T) {
	f := newFixture(t)
	ctx := context.Background()

	grant, _ := f.svc.CreateAuthorizationGrant(ctx, impersonation.CreateGrantRequest{
		TenantID:        f.customer.ID,
		GrantedByUserID: f.customerAdminID(),
		Reason:          "Ticket",
	})

	// Customer revokes the grant.
	_ = f.svc.RevokeAuthorizationGrant(ctx, grant.GrantID, f.customerAdminID())

	_, err := f.svc.CreateSession(ctx, impersonation.CreateSessionRequest{
		Mode:                  impersonation.ModePlatformSupport,
		ImpersonatingUserID:   "support-1",
		ImpersonatingTenantID: "platform",
		ImpersonatedTenantID:  f.customer.ID,
		AuthorizationGrantID:  grant.GrantID,
	})
	if !errors.Is(err, impersonation.ErrGrantRevoked) {
		t.Fatalf("expected ErrGrantRevoked, got %v", err)
	}
}

func TestCreateSession_PlatformSupport_GrantTenantMismatch(t *testing.T) {
	f := newFixture(t)
	ctx := context.Background()

	// Grant for a different tenant.
	grant, _ := f.svc.CreateAuthorizationGrant(ctx, impersonation.CreateGrantRequest{
		TenantID:        "other-tenant",
		GrantedByUserID: "admin",
		Reason:          "Ticket",
	})

	_, err := f.svc.CreateSession(ctx, impersonation.CreateSessionRequest{
		Mode:                  impersonation.ModePlatformSupport,
		ImpersonatingUserID:   "support-1",
		ImpersonatingTenantID: "platform",
		ImpersonatedTenantID:  f.customer.ID,
		AuthorizationGrantID:  grant.GrantID,
	})
	if !errors.Is(err, impersonation.ErrUnauthorized) {
		t.Fatalf("expected ErrUnauthorized, got %v", err)
	}
}

// ---------- Session validation tests ----------------------------------------

func TestValidateSession_Active(t *testing.T) {
	f := newFixture(t)
	ctx := context.Background()

	sess, _ := f.svc.CreateSession(ctx, impersonation.CreateSessionRequest{
		Mode:                  impersonation.ModeIntegrator,
		ImpersonatingUserID:   f.integratorUserID(),
		ImpersonatingTenantID: f.integrator.ID,
		ImpersonatedTenantID:  f.customer.ID,
	})

	validated, err := f.svc.ValidateSession(ctx, sess.SessionID)
	if err != nil {
		t.Fatalf("ValidateSession: %v", err)
	}
	if validated.SessionID != sess.SessionID {
		t.Fatal("session ID mismatch")
	}
}

func TestValidateSession_NotFound(t *testing.T) {
	f := newFixture(t)
	_, err := f.svc.ValidateSession(context.Background(), "nonexistent")
	if !errors.Is(err, impersonation.ErrSessionNotFound) {
		t.Fatalf("expected ErrSessionNotFound, got %v", err)
	}
}

func TestValidateSession_Expired_AutoTerminates(t *testing.T) {
	f := newFixture(t)
	ctx := context.Background()

	// Create a session with a very short timeout.
	sess, _ := f.svc.CreateSession(ctx, impersonation.CreateSessionRequest{
		Mode:                  impersonation.ModeIntegrator,
		ImpersonatingUserID:   f.integratorUserID(),
		ImpersonatingTenantID: f.integrator.ID,
		ImpersonatedTenantID:  f.customer.ID,
		Timeout:               1 * time.Minute,
	})

	// Advance time past the expiry by recreating the service with a new Now.
	futureNow := f.now.Add(2 * time.Minute)
	seq := 100
	svc2, _ := impersonation.NewService(
		impersonation.Config{
			SigningKey:            testSigningKey(t),
			DefaultSessionTimeout: 30 * time.Minute,
			Now:                   func() time.Time { return futureNow },
			NewID: func() string {
				seq++
				return fmt.Sprintf("id-%d", seq)
			},
		},
		f.identity, f.relStore, f.permStore, f.sessionStore, f.grantStore, f.recorder, f.notifier,
	)

	_, err := svc2.ValidateSession(ctx, sess.SessionID)
	if !errors.Is(err, impersonation.ErrSessionExpired) {
		t.Fatalf("expected ErrSessionExpired, got %v", err)
	}

	// Verify end notification was sent.
	endCalls := f.notifier.GetEndCalls()
	if len(endCalls) == 0 {
		t.Fatal("expected end notification for expired session")
	}
}

// ---------- RecordAction tests ----------------------------------------------

func TestRecordAction_Success(t *testing.T) {
	f := newFixture(t)
	ctx := context.Background()

	sess, _ := f.svc.CreateSession(ctx, impersonation.CreateSessionRequest{
		Mode:                  impersonation.ModeIntegrator,
		ImpersonatingUserID:   f.integratorUserID(),
		ImpersonatingTenantID: f.integrator.ID,
		ImpersonatedTenantID:  f.customer.ID,
	})

	err := f.svc.RecordAction(ctx, sess.SessionID, permissions.ActionViewLive, "cameras", "cam-1")
	if err != nil {
		t.Fatalf("RecordAction: %v", err)
	}

	// Verify audit entry with impersonation context.
	entries, err := f.recorder.Query(ctx, audit.QueryFilter{
		TenantID:     f.customer.ID,
		ActionPattern: permissions.ActionViewLive,
	})
	if err != nil {
		t.Fatalf("query audit: %v", err)
	}
	if len(entries) != 1 {
		t.Fatalf("expected 1 action audit entry, got %d", len(entries))
	}
	if entries[0].ImpersonatingUserID == nil {
		t.Fatal("action audit entry must have impersonating_user_id")
	}
	if entries[0].ImpersonatedTenantID == nil {
		t.Fatal("action audit entry must have impersonated_tenant_id")
	}
}

func TestRecordAction_AdminAction_Blocked(t *testing.T) {
	f := newFixture(t)
	ctx := context.Background()

	sess, _ := f.svc.CreateSession(ctx, impersonation.CreateSessionRequest{
		Mode:                  impersonation.ModeIntegrator,
		ImpersonatingUserID:   f.integratorUserID(),
		ImpersonatingTenantID: f.integrator.ID,
		ImpersonatedTenantID:  f.customer.ID,
	})

	adminActions := []string{
		permissions.ActionUsersCreate,
		permissions.ActionUsersDelete,
		permissions.ActionUsersImpersonate,
		permissions.ActionPermissionsGrant,
		permissions.ActionPermissionsRevoke,
		permissions.ActionSettingsEdit,
		permissions.ActionBillingChange,
	}

	for _, action := range adminActions {
		t.Run(action, func(t *testing.T) {
			err := f.svc.RecordAction(ctx, sess.SessionID, action, "test", "test-1")
			if !errors.Is(err, impersonation.ErrAdminActionBlocked) {
				t.Fatalf("expected ErrAdminActionBlocked for %q, got %v", action, err)
			}
		})
	}
}

func TestRecordAction_OutOfScope_Denied(t *testing.T) {
	f := newFixture(t)
	ctx := context.Background()

	sess, _ := f.svc.CreateSession(ctx, impersonation.CreateSessionRequest{
		Mode:                  impersonation.ModeIntegrator,
		ImpersonatingUserID:   f.integratorUserID(),
		ImpersonatingTenantID: f.integrator.ID,
		ImpersonatedTenantID:  f.customer.ID,
	})

	// ActionPTZControl is not in the integrator's scoped permissions.
	err := f.svc.RecordAction(ctx, sess.SessionID, permissions.ActionPTZControl, "cameras", "cam-1")
	if !errors.Is(err, impersonation.ErrUnauthorized) {
		t.Fatalf("expected ErrUnauthorized for out-of-scope action, got %v", err)
	}
}

// ---------- TerminateSession tests ------------------------------------------

func TestTerminateSession_Success(t *testing.T) {
	f := newFixture(t)
	ctx := context.Background()

	sess, _ := f.svc.CreateSession(ctx, impersonation.CreateSessionRequest{
		Mode:                  impersonation.ModeIntegrator,
		ImpersonatingUserID:   f.integratorUserID(),
		ImpersonatingTenantID: f.integrator.ID,
		ImpersonatedTenantID:  f.customer.ID,
	})

	err := f.svc.TerminateSession(ctx, sess.SessionID, f.customerAdminID())
	if err != nil {
		t.Fatalf("TerminateSession: %v", err)
	}

	// Verify the session is no longer valid.
	_, err = f.svc.ValidateSession(ctx, sess.SessionID)
	if !errors.Is(err, impersonation.ErrSessionAlreadyTerminated) {
		t.Fatalf("expected ErrSessionAlreadyTerminated, got %v", err)
	}

	// Verify end audit entry.
	entries, err := f.recorder.Query(ctx, audit.QueryFilter{
		TenantID:     f.customer.ID,
		ActionPattern: impersonation.AuditActionImpersonationEnd,
	})
	if err != nil {
		t.Fatalf("query audit: %v", err)
	}
	if len(entries) == 0 {
		t.Fatal("expected at least 1 end audit entry")
	}

	// Verify end notification.
	endCalls := f.notifier.GetEndCalls()
	if len(endCalls) == 0 {
		t.Fatal("expected end notification")
	}
}

func TestTerminateSession_NotFound(t *testing.T) {
	f := newFixture(t)
	err := f.svc.TerminateSession(context.Background(), "nonexistent", "user")
	if !errors.Is(err, impersonation.ErrSessionNotFound) {
		t.Fatalf("expected ErrSessionNotFound, got %v", err)
	}
}

func TestTerminateSession_AlreadyTerminated(t *testing.T) {
	f := newFixture(t)
	ctx := context.Background()

	sess, _ := f.svc.CreateSession(ctx, impersonation.CreateSessionRequest{
		Mode:                  impersonation.ModeIntegrator,
		ImpersonatingUserID:   f.integratorUserID(),
		ImpersonatingTenantID: f.integrator.ID,
		ImpersonatedTenantID:  f.customer.ID,
	})

	_ = f.svc.TerminateSession(ctx, sess.SessionID, "user1")
	err := f.svc.TerminateSession(ctx, sess.SessionID, "user2")
	if !errors.Is(err, impersonation.ErrSessionAlreadyTerminated) {
		t.Fatalf("expected ErrSessionAlreadyTerminated, got %v", err)
	}
}

// ---------- ListActiveSessions tests ----------------------------------------

func TestListActiveSessions(t *testing.T) {
	f := newFixture(t)
	ctx := context.Background()

	// Create two sessions.
	sess1, _ := f.svc.CreateSession(ctx, impersonation.CreateSessionRequest{
		Mode:                  impersonation.ModeIntegrator,
		ImpersonatingUserID:   f.integratorUserID(),
		ImpersonatingTenantID: f.integrator.ID,
		ImpersonatedTenantID:  f.customer.ID,
	})

	// Create a second integrator user for a second session.
	user2, _ := f.identity.CreateUser(context.Background(), f.integrator, auth.UserSpec{
		Username: "carol", Email: "carol@acme.test", DisplayName: "Carol", Password: "pass!!",
	})
	f.relStore.Put(crosstenant.RelationshipRecord{
		IntegratorUserID: string(user2.ID),
		IntegratorTenant: f.integrator.ID,
		CustomerTenantID: f.customer.ID,
	})
	f.permStore.PutDirect(permissions.IntegratorRelationship{
		IntegratorUserID: user2.ID,
		CustomerTenant:   f.customer,
		ScopedActions:    []string{permissions.ActionViewLive},
	})

	_, _ = f.svc.CreateSession(ctx, impersonation.CreateSessionRequest{
		Mode:                  impersonation.ModeIntegrator,
		ImpersonatingUserID:   string(user2.ID),
		ImpersonatingTenantID: f.integrator.ID,
		ImpersonatedTenantID:  f.customer.ID,
	})

	// Terminate one.
	_ = f.svc.TerminateSession(ctx, sess1.SessionID, "admin")

	// Should have 1 active session.
	active, err := f.svc.ListActiveSessions(ctx, f.customer.ID)
	if err != nil {
		t.Fatalf("ListActiveSessions: %v", err)
	}
	if len(active) != 1 {
		t.Fatalf("expected 1 active session, got %d", len(active))
	}
}

// ---------- Security boundary tests ----------------------------------------

func TestCreateSession_InvalidMode(t *testing.T) {
	f := newFixture(t)
	_, err := f.svc.CreateSession(context.Background(), impersonation.CreateSessionRequest{
		Mode:                  "invalid",
		ImpersonatingUserID:   "user",
		ImpersonatingTenantID: "tenant",
		ImpersonatedTenantID:  f.customer.ID,
	})
	if !errors.Is(err, impersonation.ErrInvalidMode) {
		t.Fatalf("expected ErrInvalidMode, got %v", err)
	}
}

func TestCreateSession_MissingRequiredFields(t *testing.T) {
	f := newFixture(t)

	tests := []struct {
		name string
		req  impersonation.CreateSessionRequest
	}{
		{"missing impersonating_user_id", impersonation.CreateSessionRequest{
			Mode: impersonation.ModeIntegrator, ImpersonatingTenantID: "t", ImpersonatedTenantID: "t2",
		}},
		{"missing impersonating_tenant_id", impersonation.CreateSessionRequest{
			Mode: impersonation.ModeIntegrator, ImpersonatingUserID: "u", ImpersonatedTenantID: "t2",
		}},
		{"missing impersonated_tenant_id", impersonation.CreateSessionRequest{
			Mode: impersonation.ModeIntegrator, ImpersonatingUserID: "u", ImpersonatingTenantID: "t",
		}},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			_, err := f.svc.CreateSession(context.Background(), tc.req)
			if err == nil {
				t.Fatal("expected error for missing field")
			}
		})
	}
}

func TestImpersonationScope_NeverIncludesAdminActions(t *testing.T) {
	// Even if the integrator has admin-level permissions in their relationship,
	// the impersonation session must strip them.
	f := newFixture(t)
	ctx := context.Background()

	// Give the integrator user ALL permissions including admin actions.
	f.permStore.PutDirect(permissions.IntegratorRelationship{
		IntegratorUserID: auth.UserID(f.integratorUserID()),
		CustomerTenant:   f.customer,
		ScopedActions: []string{
			permissions.ActionViewLive,
			permissions.ActionUsersCreate,
			permissions.ActionUsersDelete,
			permissions.ActionUsersImpersonate,
			permissions.ActionPermissionsGrant,
			permissions.ActionPermissionsRevoke,
			permissions.ActionSettingsEdit,
			permissions.ActionBillingChange,
		},
	})

	sess, err := f.svc.CreateSession(ctx, impersonation.CreateSessionRequest{
		Mode:                  impersonation.ModeIntegrator,
		ImpersonatingUserID:   f.integratorUserID(),
		ImpersonatingTenantID: f.integrator.ID,
		ImpersonatedTenantID:  f.customer.ID,
	})
	if err != nil {
		t.Fatalf("CreateSession: %v", err)
	}

	// Only view.live should remain.
	if len(sess.ScopedPermissions) != 1 {
		t.Fatalf("expected 1 permission, got %d: %v", len(sess.ScopedPermissions), sess.ScopedPermissions)
	}
	if sess.ScopedPermissions[0] != permissions.ActionViewLive {
		t.Fatalf("expected view.live, got %q", sess.ScopedPermissions[0])
	}
}

func TestPlatformSupport_CannotImpersonateWithoutGrant(t *testing.T) {
	// This is the critical security boundary: platform support MUST have
	// explicit customer authorization.
	f := newFixture(t)

	_, err := f.svc.CreateSession(context.Background(), impersonation.CreateSessionRequest{
		Mode:                  impersonation.ModePlatformSupport,
		ImpersonatingUserID:   "support-agent",
		ImpersonatingTenantID: "platform",
		ImpersonatedTenantID:  f.customer.ID,
		// No grant ID
	})
	if !errors.Is(err, impersonation.ErrMissingGrant) {
		t.Fatalf("expected ErrMissingGrant, got %v", err)
	}

	// Also with a fake grant ID.
	_, err = f.svc.CreateSession(context.Background(), impersonation.CreateSessionRequest{
		Mode:                  impersonation.ModePlatformSupport,
		ImpersonatingUserID:   "support-agent",
		ImpersonatingTenantID: "platform",
		ImpersonatedTenantID:  f.customer.ID,
		AuthorizationGrantID:  "fabricated-grant-id",
	})
	if !errors.Is(err, impersonation.ErrGrantNotFound) {
		t.Fatalf("expected ErrGrantNotFound, got %v", err)
	}
}

func TestAuditTrail_CompleteCoverage(t *testing.T) {
	// Verify the full lifecycle produces the expected audit trail.
	f := newFixture(t)
	ctx := context.Background()

	// 1. Create grant.
	grant, _ := f.svc.CreateAuthorizationGrant(ctx, impersonation.CreateGrantRequest{
		TenantID:        f.customer.ID,
		GrantedByUserID: f.customerAdminID(),
		Reason:          "Support ticket",
	})

	// 2. Create session.
	sess, _ := f.svc.CreateSession(ctx, impersonation.CreateSessionRequest{
		Mode:                  impersonation.ModePlatformSupport,
		ImpersonatingUserID:   "support-1",
		ImpersonatingTenantID: "platform",
		ImpersonatedTenantID:  f.customer.ID,
		AuthorizationGrantID:  grant.GrantID,
	})

	// 3. Record an action.
	_ = f.svc.RecordAction(ctx, sess.SessionID, permissions.ActionViewLive, "cameras", "cam-1")

	// 4. Terminate session.
	_ = f.svc.TerminateSession(ctx, sess.SessionID, f.customerAdminID())

	// Query all audit entries for this tenant.
	entries, err := f.recorder.Query(ctx, audit.QueryFilter{
		TenantID: f.customer.ID,
	})
	if err != nil {
		t.Fatalf("query audit: %v", err)
	}

	// Expect: grant create, session start, action, session end = 4 entries.
	if len(entries) < 4 {
		t.Fatalf("expected at least 4 audit entries, got %d", len(entries))
	}

	// Verify all session-related entries have impersonation context.
	for _, e := range entries {
		if e.Action == impersonation.AuditActionImpersonationStart ||
			e.Action == impersonation.AuditActionImpersonationEnd ||
			e.Action == permissions.ActionViewLive {
			if e.ImpersonatingUserID == nil {
				t.Fatalf("audit entry action=%q missing impersonating_user_id", e.Action)
			}
			if e.ImpersonatedTenantID == nil {
				t.Fatalf("audit entry action=%q missing impersonated_tenant_id", e.Action)
			}
		}
	}
}

// ---------- helpers ---------------------------------------------------------

func contains(s, substr string) bool {
	return len(s) >= len(substr) && containsSubstring(s, substr)
}

func containsSubstring(s, sub string) bool {
	for i := 0; i+len(sub) <= len(s); i++ {
		if s[i:i+len(sub)] == sub {
			return true
		}
	}
	return false
}
