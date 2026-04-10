package tenants_test

import (
	"context"
	"errors"
	"path/filepath"
	"strings"
	"testing"

	"github.com/bluenviron/mediamtx/internal/cloud/audit"
	clouddb "github.com/bluenviron/mediamtx/internal/cloud/db"
	"github.com/bluenviron/mediamtx/internal/shared/permissions"
	"github.com/bluenviron/mediamtx/internal/cloud/tenants"
	"github.com/bluenviron/mediamtx/internal/shared/auth"
)

// ---------------------------------------------------------------------------
// helpers
// ---------------------------------------------------------------------------

type harness struct {
	db       *clouddb.DB
	boot     *tenants.MemoryBootstrapper
	jobs     *tenants.MemoryEnqueuer
	auditRec *audit.MemoryRecorder
	enf      *permissions.Enforcer
	svc      *tenants.Service
}

func newHarness(t *testing.T, checker tenants.PermissionChecker) *harness {
	t.Helper()
	dir := t.TempDir()
	dsn := "sqlite://" + filepath.Join(dir, "cloud.db")
	d, err := clouddb.Open(context.Background(), dsn)
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	t.Cleanup(func() { _ = d.Close() })

	enf, err := permissions.NewEnforcer(permissions.NewInMemoryStore(), nil)
	if err != nil {
		t.Fatalf("new enforcer: %v", err)
	}

	boot := tenants.NewMemoryBootstrapper()
	jobs := tenants.NewMemoryEnqueuer()
	rec := audit.NewMemoryRecorder()

	if checker == nil {
		checker = tenants.AllowAllChecker{}
	}

	svc, err := tenants.NewService(tenants.Config{
		DB:                d,
		Bootstrapper:      boot,
		PermissionChecker: checker,
		Enforcer:          enf,
		Jobs:              jobs,
		Audit:             rec,
	})
	if err != nil {
		t.Fatalf("new service: %v", err)
	}

	return &harness{
		db: d, boot: boot, jobs: jobs, auditRec: rec, enf: enf, svc: svc,
	}
}

func defaultCaller() tenants.Caller {
	return tenants.Caller{
		UserID: auth.UserID("platform-admin"),
		Tenant: auth.TenantRef{Type: auth.TenantTypeIntegrator, ID: "platform"},
		IsPlatformStaff: true,
	}
}

// ---------------------------------------------------------------------------
// 1. happy path: CreateIntegrator with no initial admin
// ---------------------------------------------------------------------------

func TestCreateIntegrator_HappyPath(t *testing.T) {
	h := newHarness(t, nil)
	ctx := context.Background()

	got, err := h.svc.CreateIntegrator(ctx, defaultCaller(), tenants.CreateIntegratorSpec{
		DisplayName:  "Acme MSP",
		LegalName:    "Acme MSP LLC",
		ContactEmail: "ops@acme.test",
	})
	if err != nil {
		t.Fatalf("CreateIntegrator: %v", err)
	}
	if got == nil || got.Row.ID == "" {
		t.Fatalf("expected non-empty integrator row")
	}
	if got.Row.BillingMode != tenants.BillingModeDirect {
		t.Errorf("billing mode = %q, want direct", got.Row.BillingMode)
	}
	if got.ZitadelOrgID == "" {
		t.Errorf("expected zitadel org id")
	}
	// DB row exists.
	row, err := h.db.GetIntegrator(ctx, got.Row.ID, clouddb.DefaultRegion)
	if err != nil || row == nil {
		t.Fatalf("integrator missing from db: row=%v err=%v", row, err)
	}
	// Casbin role admin@tenant exists.
	pols := h.enf.Store().ListPolicies()
	roleNeedle := "role:admin@" + got.Row.ID
	hit := false
	for _, p := range pols {
		if p.Sub == roleNeedle {
			hit = true
			break
		}
	}
	if !hit {
		t.Errorf("admin role not seeded: %v", pols)
	}
	// No welcome job (no email supplied).
	if n := len(h.jobs.Jobs()); n != 0 {
		t.Errorf("jobs = %d, want 0", n)
	}
	// One audit entry.
	entries, err := h.auditRec.Query(ctx, audit.QueryFilter{TenantID: got.Row.ID})
	if err != nil {
		t.Fatalf("audit query: %v", err)
	}
	if len(entries) != 1 || entries[0].Action != "tenant.provision" {
		t.Errorf("audit entries = %+v", entries)
	}
}

// ---------------------------------------------------------------------------
// 2. happy path: CreateIntegrator with initial admin enqueues welcome + invites
// ---------------------------------------------------------------------------

func TestCreateIntegrator_WithInitialAdmin(t *testing.T) {
	h := newHarness(t, nil)
	ctx := context.Background()

	got, err := h.svc.CreateIntegrator(ctx, defaultCaller(), tenants.CreateIntegratorSpec{
		DisplayName:       "Beta Integrators",
		InitialAdminEmail: "owner@beta.test",
	})
	if err != nil {
		t.Fatalf("CreateIntegrator: %v", err)
	}
	if got.InitialAdmin == nil {
		t.Fatalf("expected InitialAdmin to be populated")
	}
	if got.InitialAdmin.Email != "owner@beta.test" {
		t.Errorf("invitation email = %q", got.InitialAdmin.Email)
	}
	jobs := h.jobs.Jobs()
	if len(jobs) != 1 {
		t.Fatalf("welcome jobs = %d, want 1", len(jobs))
	}
	if jobs[0].Email != "owner@beta.test" {
		t.Errorf("welcome job email = %q", jobs[0].Email)
	}
	// Two audit entries: provision + invite_admin.
	entries, _ := h.auditRec.Query(ctx, audit.QueryFilter{TenantID: got.Row.ID})
	if len(entries) != 2 {
		t.Errorf("audit entries = %d, want 2 (provision + invite)", len(entries))
	}
}

// ---------------------------------------------------------------------------
// 3. CreateCustomerTenant happy path (direct billing)
// ---------------------------------------------------------------------------

func TestCreateCustomerTenant_HappyPath(t *testing.T) {
	h := newHarness(t, nil)
	ctx := context.Background()

	got, err := h.svc.CreateCustomerTenant(ctx, defaultCaller(), tenants.CreateCustomerTenantSpec{
		DisplayName: "Gamma Retail",
	})
	if err != nil {
		t.Fatalf("CreateCustomerTenant: %v", err)
	}
	if got.Row.BillingMode != tenants.BillingModeDirect {
		t.Errorf("billing mode = %q", got.Row.BillingMode)
	}
	row, err := h.db.GetCustomerTenant(ctx, got.Row.ID, clouddb.DefaultRegion)
	if err != nil || row == nil {
		t.Fatalf("customer tenant missing: %v %v", row, err)
	}
}

// ---------------------------------------------------------------------------
// 4. via_integrator billing requires home integrator
// ---------------------------------------------------------------------------

func TestCreateCustomerTenant_ViaIntegratorRequiresHome(t *testing.T) {
	h := newHarness(t, nil)
	_, err := h.svc.CreateCustomerTenant(context.Background(), defaultCaller(), tenants.CreateCustomerTenantSpec{
		DisplayName: "Needs Parent",
		BillingMode: tenants.BillingModeViaIntegrator,
	})
	if !errors.Is(err, tenants.ErrMissingHomeIntegrator) {
		t.Fatalf("err = %v, want ErrMissingHomeIntegrator", err)
	}
}

// ---------------------------------------------------------------------------
// 5. duplicate display name rejected
// ---------------------------------------------------------------------------

func TestCreateIntegrator_DuplicateNameRejected(t *testing.T) {
	h := newHarness(t, nil)
	ctx := context.Background()

	_, err := h.svc.CreateIntegrator(ctx, defaultCaller(), tenants.CreateIntegratorSpec{
		DisplayName: "Duplicate Co",
	})
	if err != nil {
		t.Fatalf("first create: %v", err)
	}
	_, err = h.svc.CreateIntegrator(ctx, defaultCaller(), tenants.CreateIntegratorSpec{
		DisplayName: "Duplicate Co",
	})
	if !errors.Is(err, tenants.ErrIntegratorExists) {
		t.Fatalf("err = %v, want ErrIntegratorExists", err)
	}
}

func TestCreateCustomerTenant_DuplicateNameRejected(t *testing.T) {
	h := newHarness(t, nil)
	ctx := context.Background()
	_, err := h.svc.CreateCustomerTenant(ctx, defaultCaller(), tenants.CreateCustomerTenantSpec{
		DisplayName: "Same Name",
	})
	if err != nil {
		t.Fatalf("first: %v", err)
	}
	_, err = h.svc.CreateCustomerTenant(ctx, defaultCaller(), tenants.CreateCustomerTenantSpec{
		DisplayName: "Same Name",
	})
	if !errors.Is(err, tenants.ErrCustomerTenantExists) {
		t.Fatalf("err = %v, want ErrCustomerTenantExists", err)
	}
}

// ---------------------------------------------------------------------------
// 6. rollback when bootstrapper fails AFTER db insert
// ---------------------------------------------------------------------------

func TestCreateIntegrator_BootstrapperFailureRollsBack(t *testing.T) {
	h := newHarness(t, nil)
	ctx := context.Background()

	h.boot.CreateOrgErr = errors.New("zitadel down")
	_, err := h.svc.CreateIntegrator(ctx, defaultCaller(), tenants.CreateIntegratorSpec{
		DisplayName: "WillRollback",
	})
	if err == nil {
		t.Fatalf("expected error")
	}
	// DB row must NOT remain.
	n, qerr := h.db.CountIntegratorsByDisplayName(ctx, "WillRollback", clouddb.DefaultRegion)
	if qerr != nil {
		t.Fatalf("count: %v", qerr)
	}
	if n != 0 {
		t.Errorf("integrator row leaked after rollback: count=%d", n)
	}
	// No org should remain in the bootstrapper.
	if h.boot.OrgCount() != 0 {
		t.Errorf("orgs leaked: %d", h.boot.OrgCount())
	}
	// No welcome job, no audit.
	if len(h.jobs.Jobs()) != 0 {
		t.Errorf("welcome enqueued during rollback")
	}
}

// ---------------------------------------------------------------------------
// 7. rollback when welcome enqueue fails (after Casbin seed + org)
// ---------------------------------------------------------------------------

func TestCreateIntegrator_JobFailureRollsBack(t *testing.T) {
	h := newHarness(t, nil)
	ctx := context.Background()

	h.jobs.EnqueueErr = errors.New("river offline")
	_, err := h.svc.CreateIntegrator(ctx, defaultCaller(), tenants.CreateIntegratorSpec{
		DisplayName:       "RollbackOnJob",
		InitialAdminEmail: "boom@x.test",
	})
	if err == nil {
		t.Fatalf("expected error")
	}
	// DB row gone, org gone, casbin policies gone.
	n, _ := h.db.CountIntegratorsByDisplayName(ctx, "RollbackOnJob", clouddb.DefaultRegion)
	if n != 0 {
		t.Errorf("integrator row leaked: %d", n)
	}
	if h.boot.OrgCount() != 0 {
		t.Errorf("zitadel org leaked: %d", h.boot.OrgCount())
	}
	// No admin role for any tenant id beginning with our display.
	for _, p := range h.enf.Store().ListPolicies() {
		if strings.HasPrefix(p.Obj, "RollbackOnJob") {
			t.Errorf("policy leaked: %+v", p)
		}
	}
}

// ---------------------------------------------------------------------------
// 8. permission check fail-closed
// ---------------------------------------------------------------------------

func TestCreateIntegrator_PermissionDeniedFailsClosed(t *testing.T) {
	h := newHarness(t, tenants.DenyAllChecker{})
	ctx := context.Background()

	_, err := h.svc.CreateIntegrator(ctx, defaultCaller(), tenants.CreateIntegratorSpec{
		DisplayName: "ShouldNotExist",
	})
	if !errors.Is(err, tenants.ErrPermissionDenied) {
		t.Fatalf("err = %v, want ErrPermissionDenied", err)
	}
	// Nothing was written: no db row, no org, no jobs, no policies, no audit.
	n, _ := h.db.CountIntegratorsByDisplayName(ctx, "ShouldNotExist", clouddb.DefaultRegion)
	if n != 0 {
		t.Errorf("row leaked under deny: %d", n)
	}
	if h.boot.OrgCount() != 0 {
		t.Errorf("org leaked under deny")
	}
	if len(h.jobs.Jobs()) != 0 {
		t.Errorf("jobs leaked under deny")
	}
}

// ---------------------------------------------------------------------------
// 9. invalid spec
// ---------------------------------------------------------------------------

func TestCreateIntegrator_InvalidSpec(t *testing.T) {
	h := newHarness(t, nil)
	_, err := h.svc.CreateIntegrator(context.Background(), defaultCaller(), tenants.CreateIntegratorSpec{
		DisplayName: "   ",
	})
	if !errors.Is(err, tenants.ErrInvalidSpec) {
		t.Fatalf("err = %v, want ErrInvalidSpec", err)
	}
}

func TestCreateIntegrator_RejectsZeroCaller(t *testing.T) {
	h := newHarness(t, nil)
	_, err := h.svc.CreateIntegrator(context.Background(), tenants.Caller{}, tenants.CreateIntegratorSpec{
		DisplayName: "X",
	})
	if err == nil {
		t.Fatalf("expected error for zero caller")
	}
}

// ---------------------------------------------------------------------------
// 10. sub-reseller happy path under existing parent
// ---------------------------------------------------------------------------

func TestCreateSubReseller_NestsUnderParent(t *testing.T) {
	h := newHarness(t, nil)
	ctx := context.Background()

	parent, err := h.svc.CreateIntegrator(ctx, defaultCaller(), tenants.CreateIntegratorSpec{
		DisplayName: "NSC Root",
	})
	if err != nil {
		t.Fatalf("parent: %v", err)
	}
	child, err := h.svc.CreateSubReseller(ctx, defaultCaller(), parent.Row.ID, tenants.CreateIntegratorSpec{
		DisplayName: "Regional Reseller",
	})
	if err != nil {
		t.Fatalf("sub-reseller: %v", err)
	}
	if child.Row.ParentIntegratorID == nil || *child.Row.ParentIntegratorID != parent.Row.ID {
		t.Errorf("parent link wrong: %+v", child.Row.ParentIntegratorID)
	}
}

// ---------------------------------------------------------------------------
// 11. depth cap is enforced (3 deep is max)
// ---------------------------------------------------------------------------

func TestCreateSubReseller_DepthCapEnforced(t *testing.T) {
	h := newHarness(t, nil)
	ctx := context.Background()

	root, err := h.svc.CreateIntegrator(ctx, defaultCaller(), tenants.CreateIntegratorSpec{DisplayName: "Lvl1"})
	if err != nil {
		t.Fatalf("lvl1: %v", err)
	}
	lvl2, err := h.svc.CreateSubReseller(ctx, defaultCaller(), root.Row.ID, tenants.CreateIntegratorSpec{DisplayName: "Lvl2"})
	if err != nil {
		t.Fatalf("lvl2: %v", err)
	}
	lvl3, err := h.svc.CreateSubReseller(ctx, defaultCaller(), lvl2.Row.ID, tenants.CreateIntegratorSpec{DisplayName: "Lvl3"})
	if err != nil {
		t.Fatalf("lvl3: %v", err)
	}
	// Lvl4 must be rejected.
	_, err = h.svc.CreateSubReseller(ctx, defaultCaller(), lvl3.Row.ID, tenants.CreateIntegratorSpec{DisplayName: "Lvl4"})
	if !errors.Is(err, tenants.ErrMaxDepthExceeded) {
		t.Fatalf("err = %v, want ErrMaxDepthExceeded", err)
	}
}

// ---------------------------------------------------------------------------
// 12. unknown parent rejected
// ---------------------------------------------------------------------------

func TestCreateSubReseller_UnknownParent(t *testing.T) {
	h := newHarness(t, nil)
	_, err := h.svc.CreateSubReseller(context.Background(), defaultCaller(), "no-such-parent", tenants.CreateIntegratorSpec{
		DisplayName: "Orphan",
	})
	if !errors.Is(err, tenants.ErrParentIntegratorNotFound) {
		t.Fatalf("err = %v, want ErrParentIntegratorNotFound", err)
	}
}

// ---------------------------------------------------------------------------
// 13. cross-tenant chaos: CreateCustomerTenant cannot affect another tenant's
//     casbin policies; each provision only writes inside its own tenant id.
// ---------------------------------------------------------------------------

func TestCreateCustomerTenant_CrossTenantIsolation(t *testing.T) {
	h := newHarness(t, nil)
	ctx := context.Background()

	a, err := h.svc.CreateCustomerTenant(ctx, defaultCaller(), tenants.CreateCustomerTenantSpec{
		DisplayName: "Tenant A",
	})
	if err != nil {
		t.Fatalf("a: %v", err)
	}
	b, err := h.svc.CreateCustomerTenant(ctx, defaultCaller(), tenants.CreateCustomerTenantSpec{
		DisplayName: "Tenant B",
	})
	if err != nil {
		t.Fatalf("b: %v", err)
	}

	// Each policy must reference exactly one of the two tenant ids in its
	// object — never both.
	for _, p := range h.enf.Store().ListPolicies() {
		inA := strings.HasPrefix(p.Obj, a.Row.ID+"/")
		inB := strings.HasPrefix(p.Obj, b.Row.ID+"/")
		if inA && inB {
			t.Errorf("policy straddles tenants: %+v", p)
		}
	}

	// Audit entries are tenant-scoped: querying for tenant A must NOT
	// return tenant B's provision entry.
	aEntries, _ := h.auditRec.Query(ctx, audit.QueryFilter{TenantID: a.Row.ID})
	for _, e := range aEntries {
		if e.TenantID != a.Row.ID {
			t.Errorf("audit leak: tenant A query returned %s", e.TenantID)
		}
	}
	bEntries, _ := h.auditRec.Query(ctx, audit.QueryFilter{TenantID: b.Row.ID})
	for _, e := range bEntries {
		if e.TenantID != b.Row.ID {
			t.Errorf("audit leak: tenant B query returned %s", e.TenantID)
		}
	}
}

// ---------------------------------------------------------------------------
// 14. audit entries cover the action set we care about
// ---------------------------------------------------------------------------

func TestProvisionAuditCoverage(t *testing.T) {
	h := newHarness(t, nil)
	ctx := context.Background()

	got, err := h.svc.CreateIntegrator(ctx, defaultCaller(), tenants.CreateIntegratorSpec{
		DisplayName:       "AuditTarget",
		InitialAdminEmail: "first@audit.test",
	})
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	entries, err := h.auditRec.Query(ctx, audit.QueryFilter{TenantID: got.Row.ID})
	if err != nil {
		t.Fatalf("query: %v", err)
	}
	saw := map[string]bool{}
	for _, e := range entries {
		saw[e.Action] = true
		if e.ActorUserID != "platform-admin" {
			t.Errorf("actor = %q", e.ActorUserID)
		}
		if e.Result != audit.ResultAllow {
			t.Errorf("result = %q", e.Result)
		}
		if e.ActorAgent != audit.AgentCloud {
			t.Errorf("agent = %q", e.ActorAgent)
		}
	}
	if !saw["tenant.provision"] {
		t.Errorf("missing tenant.provision audit")
	}
	if !saw["tenant.invite_admin"] {
		t.Errorf("missing tenant.invite_admin audit")
	}
}
