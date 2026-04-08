package permissions_test

import (
	"context"
	"testing"

	"github.com/bluenviron/mediamtx/internal/cloud/permissions"
	"github.com/bluenviron/mediamtx/internal/shared/auth"
)

// These tests are the Wave-1 multi-tenant isolation tests (seam #4). Every
// cloud API PR MUST add one of these before merging — see README.md.

var (
	tenantA = auth.TenantRef{Type: auth.TenantTypeCustomer, ID: "tenant-A"}
	tenantB = auth.TenantRef{Type: auth.TenantTypeCustomer, ID: "tenant-B"}
	tenantI = auth.TenantRef{Type: auth.TenantTypeIntegrator, ID: "tenant-integrator"}
)

func newTestEnforcer(t *testing.T) *permissions.Enforcer {
	t.Helper()
	e, err := permissions.NewEnforcer(permissions.NewInMemoryStore(), nil)
	if err != nil {
		t.Fatalf("NewEnforcer: %v", err)
	}
	return e
}

// --- helpers ------------------------------------------------------------

func mustEnforce(t *testing.T, e *permissions.Enforcer, sub permissions.SubjectRef, obj permissions.ObjectRef, action string) bool {
	t.Helper()
	ok, err := e.Enforce(context.Background(), sub, obj, action)
	if err != nil {
		t.Fatalf("Enforce(%s, %s, %s): %v", sub, obj, action, err)
	}
	return ok
}

// --- TestDirectTenantAccess --------------------------------------------

func TestDirectTenantAccess(t *testing.T) {
	e := newTestEnforcer(t)

	// Seed the admin role in tenant A and bind user REPLACE_ME_user_1.
	roleA, err := permissions.SeedRole(e, permissions.DefaultRoleTemplates[permissions.RoleAdmin], tenantA)
	if err != nil {
		t.Fatalf("SeedRole A: %v", err)
	}
	userA := permissions.NewUserSubject("REPLACE_ME_user_1", tenantA)
	if err := permissions.BindSubjectToRole(e, userA, roleA); err != nil {
		t.Fatalf("BindSubjectToRole A: %v", err)
	}

	// Seed admin in tenant B with a different user.
	roleB, err := permissions.SeedRole(e, permissions.DefaultRoleTemplates[permissions.RoleAdmin], tenantB)
	if err != nil {
		t.Fatalf("SeedRole B: %v", err)
	}
	userB := permissions.NewUserSubject("REPLACE_ME_user_2", tenantB)
	if err := permissions.BindSubjectToRole(e, userB, roleB); err != nil {
		t.Fatalf("BindSubjectToRole B: %v", err)
	}

	camA := permissions.NewObject(tenantA, "cameras", "cam-1")
	camB := permissions.NewObject(tenantB, "cameras", "cam-1")

	if !mustEnforce(t, e, userA, camA, permissions.ActionViewLive) {
		t.Fatalf("userA should see tenantA/cameras/cam-1")
	}
	if mustEnforce(t, e, userA, camB, permissions.ActionViewLive) {
		t.Fatalf("userA must NOT see tenantB resources (cross-tenant leak)")
	}
	if !mustEnforce(t, e, userB, camB, permissions.ActionViewLive) {
		t.Fatalf("userB should see own tenant")
	}
	if mustEnforce(t, e, userB, camA, permissions.ActionViewLive) {
		t.Fatalf("userB must NOT see tenantA resources")
	}
}

// --- TestIntegratorCrossTenantAccess -----------------------------------

func TestIntegratorCrossTenantAccess(t *testing.T) {
	e := newTestEnforcer(t)

	// Integrator staff has a relationship to tenantA, not tenantB.
	supportRole, err := permissions.SeedRole(e,
		permissions.DefaultRoleTemplates[permissions.RoleIntegratorSupport], tenantA)
	if err != nil {
		t.Fatalf("SeedRole integrator_support: %v", err)
	}

	integrator := permissions.NewIntegratorSubject("REPLACE_ME_user_3", tenantA)
	if err := permissions.BindSubjectToRole(e, integrator, supportRole); err != nil {
		t.Fatalf("Bind integrator to support role: %v", err)
	}

	camA := permissions.NewObject(tenantA, "cameras", "cam-1")
	camB := permissions.NewObject(tenantB, "cameras", "cam-1")

	if !mustEnforce(t, e, integrator, camA, permissions.ActionViewLive) {
		t.Fatalf("integrator with A relationship should see tenantA cameras")
	}
	if !mustEnforce(t, e, integrator, camA, permissions.ActionPTZControl) {
		t.Fatalf("integrator_support should have PTZ on tenantA")
	}
	// No relationship to B, no policies bound — must be denied.
	if mustEnforce(t, e, integrator, camB, permissions.ActionViewLive) {
		t.Fatalf("integrator must NOT access tenantB without a relationship")
	}

	// Integrator "view live" on tenantA is allowed, but cameras.delete is not
	// in the support template — make sure the narrow role is actually narrow.
	if mustEnforce(t, e, integrator, camA, permissions.ActionCamerasDelete) {
		t.Fatalf("integrator_support should NOT delete cameras")
	}
}

// --- TestFederationSubject ----------------------------------------------

func TestFederationSubject(t *testing.T) {
	e := newTestEnforcer(t)

	peer := permissions.NewFederationSubject("peer-dir-42")

	// Federation subjects don't get role templates — they get explicit
	// per-resource grants written by federation.configure flows.
	if err := e.AddPolicy(permissions.PolicyRule{
		Sub: peer.String(),
		Obj: permissions.NewObject(tenantA, "cameras", "cam-shared").String(),
		Act: permissions.ActionViewLive,
		Eft: "allow",
	}); err != nil {
		t.Fatalf("AddPolicy federation: %v", err)
	}

	shared := permissions.NewObject(tenantA, "cameras", "cam-shared")
	other := permissions.NewObject(tenantA, "cameras", "cam-private")

	if !mustEnforce(t, e, peer, shared, permissions.ActionViewLive) {
		t.Fatalf("federation peer should view the explicitly granted camera")
	}
	if mustEnforce(t, e, peer, other, permissions.ActionViewLive) {
		t.Fatalf("federation peer must NOT see cameras that were not granted")
	}
	// Even tenant-wide actions shouldn't leak.
	if mustEnforce(t, e, peer, shared, permissions.ActionCamerasDelete) {
		t.Fatalf("federation grant was view.live only")
	}
}

// --- TestSubResellerNarrowing -------------------------------------------

func TestSubResellerNarrowing(t *testing.T) {
	store := permissions.NewInMemoryRelationshipStore()

	// Parent integrator has view.playback + view.live.
	parentRef := auth.IntegratorRelationshipRef("parent-link-1")
	store.PutParent(parentRef, permissions.IntegratorRelationship{
		IntegratorUserID: "REPLACE_ME_user_parent",
		CustomerTenant:   tenantA,
		ScopedActions: []string{
			permissions.ActionViewLive,
			permissions.ActionViewPlayback,
		},
	})

	// Child sub-reseller only grants view.live. The intersection must
	// produce exactly {view.live}.
	store.PutDirect(permissions.IntegratorRelationship{
		IntegratorUserID: "REPLACE_ME_user_child",
		CustomerTenant:   tenantA,
		ParentIntegrator: parentRef,
		ScopedActions:    []string{permissions.ActionViewLive},
	})

	allowed, found, err := permissions.ResolveIntegratorScope(
		context.Background(), store, "REPLACE_ME_user_child", tenantA)
	if err != nil {
		t.Fatalf("ResolveIntegratorScope: %v", err)
	}
	if !found {
		t.Fatalf("expected relationship to be found")
	}
	if len(allowed) != 1 || allowed[0] != permissions.ActionViewLive {
		t.Fatalf("expected only view.live, got %v", allowed)
	}

	// Opposite: child tries to add view.playback that parent already has
	// but parent DOESN'T have view.snapshot. Intersection must exclude it.
	store.PutDirect(permissions.IntegratorRelationship{
		IntegratorUserID: "REPLACE_ME_user_child_broad",
		CustomerTenant:   tenantA,
		ParentIntegrator: parentRef,
		ScopedActions: []string{
			permissions.ActionViewLive,
			permissions.ActionViewPlayback,
			permissions.ActionViewSnapshot, // NOT in parent
		},
	})
	allowed, _, err = permissions.ResolveIntegratorScope(
		context.Background(), store, "REPLACE_ME_user_child_broad", tenantA)
	if err != nil {
		t.Fatalf("ResolveIntegratorScope broad: %v", err)
	}
	for _, a := range allowed {
		if a == permissions.ActionViewSnapshot {
			t.Fatalf("child must NOT broaden parent (view.snapshot leaked)")
		}
	}
	if len(allowed) != 2 {
		t.Fatalf("expected 2 actions after intersect, got %v", allowed)
	}
}

// --- TestFailClosedDefault ----------------------------------------------

func TestFailClosedDefault(t *testing.T) {
	e := newTestEnforcer(t)

	// Brand new enforcer: no policies. Every Enforce must deny.
	user := permissions.NewUserSubject("REPLACE_ME_user_4", tenantA)
	cam := permissions.NewObject(tenantA, "cameras", "cam-1")

	for _, act := range permissions.AllActions {
		if mustEnforce(t, e, user, cam, act) {
			t.Fatalf("fail-closed violated: %s allowed with no policies", act)
		}
	}
}

// --- TestRevocationTakesEffect ------------------------------------------

func TestRevocationTakesEffect(t *testing.T) {
	e := newTestEnforcer(t)
	role, err := permissions.SeedRole(e, permissions.DefaultRoleTemplates[permissions.RoleViewer], tenantA)
	if err != nil {
		t.Fatalf("SeedRole viewer: %v", err)
	}
	user := permissions.NewUserSubject("REPLACE_ME_user_5", tenantA)
	if err := permissions.BindSubjectToRole(e, user, role); err != nil {
		t.Fatalf("bind: %v", err)
	}

	cam := permissions.NewObject(tenantA, "cameras", "cam-1")
	if !mustEnforce(t, e, user, cam, permissions.ActionViewLive) {
		t.Fatalf("viewer should be allowed view.live before revocation")
	}

	if err := permissions.UnbindSubjectFromRole(e, user, role); err != nil {
		t.Fatalf("unbind: %v", err)
	}
	if mustEnforce(t, e, user, cam, permissions.ActionViewLive) {
		t.Fatalf("revocation MUST take effect on next Enforce call")
	}
}

// --- TestRoleTemplateSemantics ------------------------------------------

func TestRoleTemplateSemantics(t *testing.T) {
	e := newTestEnforcer(t)

	adminRole, err := permissions.SeedRole(e, permissions.DefaultRoleTemplates[permissions.RoleAdmin], tenantA)
	if err != nil {
		t.Fatalf("SeedRole admin: %v", err)
	}
	viewerRole, err := permissions.SeedRole(e, permissions.DefaultRoleTemplates[permissions.RoleViewer], tenantA)
	if err != nil {
		t.Fatalf("SeedRole viewer: %v", err)
	}

	admin := permissions.NewUserSubject("REPLACE_ME_user_admin", tenantA)
	viewer := permissions.NewUserSubject("REPLACE_ME_user_viewer", tenantA)
	if err := permissions.BindSubjectToRole(e, admin, adminRole); err != nil {
		t.Fatalf("bind admin: %v", err)
	}
	if err := permissions.BindSubjectToRole(e, viewer, viewerRole); err != nil {
		t.Fatalf("bind viewer: %v", err)
	}

	cam := permissions.NewObject(tenantA, "cameras", "cam-1")

	// Admin: every action must be allowed on every resource in tenant A.
	for _, act := range permissions.AllActions {
		if !mustEnforce(t, e, admin, cam, act) {
			t.Fatalf("admin should be allowed %s on tenantA/cameras/cam-1", act)
		}
	}

	// Viewer: only view.* actions, nothing else.
	viewActions := map[string]bool{
		permissions.ActionViewLive:       true,
		permissions.ActionViewPlayback:   true,
		permissions.ActionViewSnapshot:   true,
		permissions.ActionViewThumbnails: true,
	}
	for _, act := range permissions.AllActions {
		got := mustEnforce(t, e, viewer, cam, act)
		if viewActions[act] && !got {
			t.Fatalf("viewer should be allowed %s", act)
		}
		if !viewActions[act] && got {
			t.Fatalf("viewer should NOT be allowed %s", act)
		}
	}

	// And: admin on tenantB must STILL be denied — role is tenant-scoped.
	camB := permissions.NewObject(tenantB, "cameras", "cam-1")
	if mustEnforce(t, e, admin, camB, permissions.ActionViewLive) {
		t.Fatalf("tenantA admin must not touch tenantB")
	}
}

// --- TestSubjectFromClaims ----------------------------------------------

func TestSubjectFromClaims(t *testing.T) {
	claims := auth.Claims{
		UserID:    "REPLACE_ME_user_6",
		TenantRef: tenantA,
	}
	got := permissions.SubjectFromClaims(claims)
	want := "user:REPLACE_ME_user_6@tenant-A"
	if got.String() != want {
		t.Fatalf("SubjectFromClaims = %q, want %q", got.String(), want)
	}
}

// --- TestObjectValidation -----------------------------------------------

func TestObjectValidation(t *testing.T) {
	e := newTestEnforcer(t)
	user := permissions.NewUserSubject("REPLACE_ME_user_7", tenantA)

	// Missing tenant id -> error, fail-closed.
	bad := permissions.ObjectRef{ResourceType: "cameras", ResourceID: "cam-1"}
	ok, err := e.Enforce(context.Background(), user, bad, permissions.ActionViewLive)
	if err == nil {
		t.Fatalf("expected error for tenantless object")
	}
	if ok {
		t.Fatalf("expected deny for tenantless object")
	}
}

// Ensure the integrator tenant type doesn't accidentally turn into a blanket
// escape hatch — an integrator-type tenant subject still needs a policy.
func TestIntegratorTenantTypeIsNotMagic(t *testing.T) {
	e := newTestEnforcer(t)
	// A user in the integrator tenant, with no relationships seeded, should
	// not be able to touch a customer tenant's cameras.
	_ = tenantI // reference silenced
	user := permissions.NewUserSubject("REPLACE_ME_user_8", tenantI)
	cam := permissions.NewObject(tenantA, "cameras", "cam-1")
	if mustEnforce(t, e, user, cam, permissions.ActionViewLive) {
		t.Fatalf("integrator-tenant user cannot touch customer without explicit policy")
	}
}
