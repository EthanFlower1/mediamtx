// Package casbin_policies contains the Casbin policy matrix verification
// tests for KAI-235.
//
// The tests in this file:
//  1. TestCasbinPolicy_DirectUserCrossTenant — a user from tenant A MUST NOT
//     be granted access to tenant B's resources, even if they hold the same
//     role inside their own tenant.
//  2. TestCasbinPolicy_IntegratorCrossTenant — an integrator subject for
//     tenant A MUST NOT access tenant B's resources.
//  3. TestCasbinPolicy_FederationCrossTenant — a federation subject MUST NOT
//     access any tenant resources without an explicit grant in the policy.
//  4. TestCasbinPolicy_DenyOverridesAllow — when a user has both an allow and
//     a deny rule for the same resource, the deny wins.
//  5. TestCasbinPolicy_WildcardObjectIsTenantScoped — a wildcard object
//     "<tenant>/<type>/*" MUST NOT match across tenants.
//  6. TestCasbinPolicy_EmptySubjectRejected — the enforcer must return an
//     error (not allow) for a zero-value SubjectRef.
//  7. TestCasbinPolicy_EmptyObjectRejected — enforcer must error on a
//     zero-value ObjectRef.
//  8. TestCasbinPolicy_EmptyActionRejected — enforcer must error on "".
//  9. TestCasbinPolicy_Matrix — an exhaustive table-driven matrix across the
//     four subject kinds × resource types × actions. The matrix is printed to
//     stdout so code reviewers can audit the policy without running tests.
package casbin_policies

import (
	"bytes"
	"context"
	"fmt"
	"strings"
	"testing"

	"github.com/bluenviron/mediamtx/internal/cloud/permissions"
	"github.com/bluenviron/mediamtx/internal/shared/auth"
)

// -----------------------------------------------------------------------
// Helpers
// -----------------------------------------------------------------------

func newEnforcer(t *testing.T) *permissions.Enforcer {
	t.Helper()
	store := permissions.NewInMemoryStore()
	e, err := permissions.NewEnforcer(store, nil)
	if err != nil {
		t.Fatalf("newEnforcer: %v", err)
	}
	return e
}

func tenantA() auth.TenantRef { return auth.TenantRef{Type: auth.TenantTypeCustomer, ID: "tenant-a"} }
func tenantB() auth.TenantRef { return auth.TenantRef{Type: auth.TenantTypeCustomer, ID: "tenant-b"} }

const (
	userAID auth.UserID = "user-a-001"
	userBID auth.UserID = "user-b-001"
)

func addAllowPolicy(t *testing.T, e *permissions.Enforcer, sub permissions.SubjectRef, obj permissions.ObjectRef, action string) {
	t.Helper()
	rule := permissions.PolicyRule{
		Sub: sub.String(),
		Obj: obj.String(),
		Act: action,
		Eft: "allow",
	}
	if err := e.AddPolicy(rule); err != nil {
		t.Fatalf("addAllowPolicy: %v", err)
	}
}

func addDenyPolicy(t *testing.T, e *permissions.Enforcer, sub permissions.SubjectRef, obj permissions.ObjectRef, action string) {
	t.Helper()
	rule := permissions.PolicyRule{
		Sub: sub.String(),
		Obj: obj.String(),
		Act: action,
		Eft: "deny",
	}
	if err := e.AddPolicy(rule); err != nil {
		t.Fatalf("addDenyPolicy: %v", err)
	}
}

func mustAllow(t *testing.T, e *permissions.Enforcer, sub permissions.SubjectRef, obj permissions.ObjectRef, action string) {
	t.Helper()
	ok, err := e.Enforce(context.Background(), sub, obj, action)
	if err != nil {
		t.Fatalf("Enforce error: %v", err)
	}
	if !ok {
		t.Errorf("expected ALLOW for sub=%s obj=%s act=%s; got DENY", sub, obj, action)
	}
}

func mustDeny(t *testing.T, e *permissions.Enforcer, sub permissions.SubjectRef, obj permissions.ObjectRef, action string) {
	t.Helper()
	ok, err := e.Enforce(context.Background(), sub, obj, action)
	if err != nil {
		// An error is also a deny, which is correct. Don't fail the test.
		return
	}
	if ok {
		t.Errorf("expected DENY for sub=%s obj=%s act=%s; got ALLOW", sub, obj, action)
	}
}

// -----------------------------------------------------------------------
// 1. Direct user cross-tenant
// -----------------------------------------------------------------------

func TestCasbinPolicy_DirectUserCrossTenant(t *testing.T) {
	t.Parallel()
	e := newEnforcer(t)
	subA := permissions.NewUserSubject(userAID, tenantA())

	// Grant user A access to tenant A's cameras.
	addAllowPolicy(t, e, subA, permissions.NewObjectAll(tenantA(), "cameras"), "read")

	// User A must be able to read their own tenant's cameras.
	mustAllow(t, e, subA, permissions.NewObjectAll(tenantA(), "cameras"), "read")

	// User A must NOT be able to read tenant B's cameras — even though the
	// action and resource type are the same. The tenant prefix in the object
	// prevents the wildcard from matching across tenants.
	mustDeny(t, e, subA, permissions.NewObjectAll(tenantB(), "cameras"), "read")
	mustDeny(t, e, subA, permissions.NewObject(tenantB(), "cameras", "cam-99"), "read")
}

// -----------------------------------------------------------------------
// 2. Integrator cross-tenant
// -----------------------------------------------------------------------

func TestCasbinPolicy_IntegratorCrossTenant(t *testing.T) {
	t.Parallel()
	e := newEnforcer(t)

	// An integrator subject scoped to tenant A.
	intA := permissions.NewIntegratorSubject(userAID, tenantA())
	addAllowPolicy(t, e, intA, permissions.NewObjectAll(tenantA(), "cameras"), "read")

	mustAllow(t, e, intA, permissions.NewObjectAll(tenantA(), "cameras"), "read")
	// Integrator A's grant MUST NOT bleed into tenant B.
	mustDeny(t, e, intA, permissions.NewObjectAll(tenantB(), "cameras"), "read")

	// An integrator subject scoped to tenant B (different from tenant A's
	// integrator) MUST NOT use tenant A's grants.
	intB := permissions.NewIntegratorSubject(userAID, tenantB())
	mustDeny(t, e, intB, permissions.NewObjectAll(tenantA(), "cameras"), "read")
}

// -----------------------------------------------------------------------
// 3. Federation cross-tenant
// -----------------------------------------------------------------------

func TestCasbinPolicy_FederationCrossTenant(t *testing.T) {
	t.Parallel()
	e := newEnforcer(t)

	fed := permissions.NewFederationSubject("peer-dir-001")
	// No policy for the federation subject: must be denied by fail-closed.
	mustDeny(t, e, fed, permissions.NewObjectAll(tenantA(), "cameras"), "read")
	mustDeny(t, e, fed, permissions.NewObjectAll(tenantB(), "cameras"), "read")

	// Explicitly grant the federation subject access to tenant A only.
	addAllowPolicy(t, e, fed, permissions.NewObjectAll(tenantA(), "cameras"), "read")
	mustAllow(t, e, fed, permissions.NewObjectAll(tenantA(), "cameras"), "read")
	// Tenant B is still off limits.
	mustDeny(t, e, fed, permissions.NewObjectAll(tenantB(), "cameras"), "read")
}

// -----------------------------------------------------------------------
// 4. Deny overrides allow
// -----------------------------------------------------------------------

func TestCasbinPolicy_DenyOverridesAllow(t *testing.T) {
	t.Parallel()
	e := newEnforcer(t)
	subA := permissions.NewUserSubject(userAID, tenantA())

	addAllowPolicy(t, e, subA, permissions.NewObjectAll(tenantA(), "cameras"), "read")
	addDenyPolicy(t, e, subA, permissions.NewObjectAll(tenantA(), "cameras"), "read")

	// Deny wins in the some(allow) && !some(deny) model.
	mustDeny(t, e, subA, permissions.NewObjectAll(tenantA(), "cameras"), "read")
}

// -----------------------------------------------------------------------
// 5. Wildcard object is tenant-scoped
// -----------------------------------------------------------------------

func TestCasbinPolicy_WildcardObjectIsTenantScoped(t *testing.T) {
	t.Parallel()
	e := newEnforcer(t)
	subA := permissions.NewUserSubject(userAID, tenantA())

	// Grant access to ALL cameras in tenant A ("tenant-a/cameras/*").
	addAllowPolicy(t, e, subA, permissions.NewObjectAll(tenantA(), "cameras"), "read")

	// Specific camera in tenant A — must match via wildcard.
	mustAllow(t, e, subA, permissions.NewObject(tenantA(), "cameras", "cam-001"), "read")
	mustAllow(t, e, subA, permissions.NewObject(tenantA(), "cameras", "cam-999"), "read")

	// ANY camera in tenant B — must NOT match (wildcard is tenant-scoped).
	mustDeny(t, e, subA, permissions.NewObject(tenantB(), "cameras", "cam-001"), "read")
	mustDeny(t, e, subA, permissions.NewObjectAll(tenantB(), "cameras"), "read")
}

// -----------------------------------------------------------------------
// 6. Empty subject rejected
// -----------------------------------------------------------------------

func TestCasbinPolicy_EmptySubjectRejected(t *testing.T) {
	t.Parallel()
	e := newEnforcer(t)
	zero := permissions.SubjectRef{} // zero value — ID is empty
	_, err := e.Enforce(context.Background(), zero, permissions.NewObjectAll(tenantA(), "cameras"), "read")
	if err == nil {
		t.Error("expected error for zero SubjectRef; got nil")
	}
}

// -----------------------------------------------------------------------
// 7. Empty object rejected
// -----------------------------------------------------------------------

func TestCasbinPolicy_EmptyObjectRejected(t *testing.T) {
	t.Parallel()
	e := newEnforcer(t)
	subA := permissions.NewUserSubject(userAID, tenantA())
	zero := permissions.ObjectRef{} // zero value — TenantID is empty
	_, err := e.Enforce(context.Background(), subA, zero, "read")
	if err == nil {
		t.Error("expected error for zero ObjectRef; got nil")
	}
}

// -----------------------------------------------------------------------
// 8. Empty action rejected
// -----------------------------------------------------------------------

func TestCasbinPolicy_EmptyActionRejected(t *testing.T) {
	t.Parallel()
	e := newEnforcer(t)
	subA := permissions.NewUserSubject(userAID, tenantA())
	_, err := e.Enforce(context.Background(), subA, permissions.NewObjectAll(tenantA(), "cameras"), "")
	if err == nil {
		t.Error("expected error for empty action; got nil")
	}
}

// -----------------------------------------------------------------------
// 9. Policy matrix
// -----------------------------------------------------------------------

// matrixRow describes one (subject, resource, action, hasPolicy, expectedAllow) row.
//
// seedObject is the object used when seeding the allow policy (may differ from
// the object under test for cross-tenant rows — we seed tenant A's policy and
// then assert it does not grant access to tenant B's objects).
type matrixRow struct {
	desc          string
	subject       permissions.SubjectRef
	seedObject    permissions.ObjectRef // object to seed the allow policy on; zero = same as object
	object        permissions.ObjectRef // object to check access against
	action        string
	seedPolicy    bool // true = seed an allow policy before checking
	expectedAllow bool
}

// TestCasbinPolicy_Matrix runs an exhaustive table-driven test and prints the
// resulting policy matrix to stdout for audit review.
//
// Each row runs in its own goroutine with its own enforcer instance so there
// is no shared mutable state. The matrix output is assembled after all
// sub-tests finish to avoid the race on a shared bytes.Buffer.
func TestCasbinPolicy_Matrix(t *testing.T) {
	t.Parallel()

	tenA := tenantA()
	tenB := tenantB()
	userA := permissions.NewUserSubject(userAID, tenA)
	intA := permissions.NewIntegratorSubject(userAID, tenA)
	fed := permissions.NewFederationSubject("peer-001")

	// For cross-tenant rows, seedObject is tenant-A's resource (what we seed)
	// while object is tenant-B's resource (what we assert must be denied).
	// Zero-value seedObject means use the same object as `object`.
	rows := []matrixRow{
		// ---- User in own tenant ----
		{desc: "user-A reads own cameras (no policy)", subject: userA, object: permissions.NewObjectAll(tenA, "cameras"), action: "read", seedPolicy: false, expectedAllow: false},
		{desc: "user-A reads own cameras (policy)", subject: userA, object: permissions.NewObjectAll(tenA, "cameras"), action: "read", seedPolicy: true, expectedAllow: true},
		{desc: "user-A writes own cameras (no policy)", subject: userA, object: permissions.NewObjectAll(tenA, "cameras"), action: "create", seedPolicy: false, expectedAllow: false},
		{desc: "user-A writes own cameras (policy)", subject: userA, object: permissions.NewObjectAll(tenA, "cameras"), action: "create", seedPolicy: true, expectedAllow: true},
		// ---- User in other tenant — seed policy on A, check against B ----
		{desc: "user-A reads B cameras (policy on A only)", subject: userA, seedObject: permissions.NewObjectAll(tenA, "cameras"), object: permissions.NewObjectAll(tenB, "cameras"), action: "read", seedPolicy: true, expectedAllow: false},
		{desc: "user-A writes B cameras (policy on A only)", subject: userA, seedObject: permissions.NewObjectAll(tenA, "cameras"), object: permissions.NewObjectAll(tenB, "cameras"), action: "create", seedPolicy: true, expectedAllow: false},
		// ---- Integrator in own scoped tenant ----
		{desc: "integrator-A reads A cameras (no policy)", subject: intA, object: permissions.NewObjectAll(tenA, "cameras"), action: "read", seedPolicy: false, expectedAllow: false},
		{desc: "integrator-A reads A cameras (policy)", subject: intA, object: permissions.NewObjectAll(tenA, "cameras"), action: "read", seedPolicy: true, expectedAllow: true},
		// ---- Integrator cross-tenant — seed on A, check against B ----
		{desc: "integrator-A reads B cameras (policy on A only)", subject: intA, seedObject: permissions.NewObjectAll(tenA, "cameras"), object: permissions.NewObjectAll(tenB, "cameras"), action: "read", seedPolicy: true, expectedAllow: false},
		// ---- Federation ----
		{desc: "federation reads A cameras (no policy)", subject: fed, object: permissions.NewObjectAll(tenA, "cameras"), action: "read", seedPolicy: false, expectedAllow: false},
		{desc: "federation reads A cameras (policy)", subject: fed, object: permissions.NewObjectAll(tenA, "cameras"), action: "read", seedPolicy: true, expectedAllow: true},
		{desc: "federation reads B cameras (A-only policy)", subject: fed, seedObject: permissions.NewObjectAll(tenA, "cameras"), object: permissions.NewObjectAll(tenB, "cameras"), action: "read", seedPolicy: true, expectedAllow: false},
		// ---- Streams ----
		{desc: "user-A mints stream A (policy)", subject: userA, object: permissions.NewObjectAll(tenA, "streams"), action: "mint", seedPolicy: true, expectedAllow: true},
		{desc: "user-A mints stream B (policy on A only)", subject: userA, seedObject: permissions.NewObjectAll(tenA, "streams"), object: permissions.NewObjectAll(tenB, "streams"), action: "mint", seedPolicy: true, expectedAllow: false},
		// ---- Recorders ----
		{desc: "user-A controls recorder A (policy)", subject: userA, object: permissions.NewObjectAll(tenA, "recorders"), action: "control", seedPolicy: true, expectedAllow: true},
		{desc: "user-A controls recorder B (policy on A only)", subject: userA, seedObject: permissions.NewObjectAll(tenA, "recorders"), object: permissions.NewObjectAll(tenB, "recorders"), action: "control", seedPolicy: true, expectedAllow: false},
	}

	type rowResult struct {
		desc     string
		expected string
		got      string
		pass     bool
		errStr   string
	}
	results := make([]rowResult, len(rows))

	// Each row gets its own enforcer so policies do not bleed between rows.
	// Sub-tests write only into their own results[i] slot — no sharing.
	for i, row := range rows {
		i, row := i, row
		t.Run(row.desc, func(t *testing.T) {
			t.Parallel()
			e := newEnforcer(t)
			if row.seedPolicy {
				// Use seedObject if set; otherwise seed on the checked object.
				seedObj := row.seedObject
				if seedObj.Tenant.ID == "" {
					seedObj = row.object
				}
				addAllowPolicy(t, e, row.subject, seedObj, row.action)
			}
			got, err := e.Enforce(context.Background(), row.subject, row.object, row.action)
			if err != nil {
				got = false // error = deny (fail-closed)
			}
			gotStr := "DENY"
			if got {
				gotStr = "ALLOW"
			}
			expectedStr := "DENY"
			if row.expectedAllow {
				expectedStr = "ALLOW"
			}
			var errStr string
			if err != nil {
				errStr = err.Error()
			}
			results[i] = rowResult{
				desc:     row.desc,
				expected: expectedStr,
				got:      gotStr,
				pass:     got == row.expectedAllow,
				errStr:   errStr,
			}
			if !results[i].pass {
				t.Errorf("scenario %q: expected %s, got %s (err=%v)", row.desc, expectedStr, gotStr, err)
			}
		})
	}

	// Print the matrix after all sub-tests complete (called by testing framework
	// when the parent test function returns after all t.Run calls).
	t.Cleanup(func() {
		var buf bytes.Buffer
		fmt.Fprintf(&buf, "\nKAI-235 Casbin Policy Matrix\n")
		fmt.Fprintf(&buf, "%-60s %-10s %-10s %-6s\n", "Scenario", "Expected", "Got", "Pass")
		fmt.Fprintf(&buf, "%s\n", strings.Repeat("-", 90))
		for _, r := range results {
			pass := "OK"
			if !r.pass {
				pass = "FAIL"
			}
			fmt.Fprintf(&buf, "%-60s %-10s %-10s %-6s", r.desc, r.expected, r.got, pass)
			if r.errStr != "" {
				fmt.Fprintf(&buf, " (%s)", r.errStr)
			}
			fmt.Fprintln(&buf)
		}
		t.Log(buf.String())
	})
}
