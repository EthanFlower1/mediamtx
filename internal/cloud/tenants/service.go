package tenants

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/bluenviron/mediamtx/internal/cloud/audit"
	clouddb "github.com/bluenviron/mediamtx/internal/cloud/db"
	"github.com/bluenviron/mediamtx/internal/shared/permissions"
	"github.com/bluenviron/mediamtx/internal/shared/auth"
)

// Clock is a minimal now-source so tests can freeze time. Default is
// time.Now().UTC().
type Clock func() time.Time

// IDGen is a minimal id generator. Default is a 16-byte hex random.
type IDGen func() string

// Config bundles the dependencies of a Service. Every field is required
// except Clock / IDGen, which default to sensible values.
type Config struct {
	DB                *clouddb.DB
	Bootstrapper      TenantBootstrapper
	PermissionChecker PermissionChecker
	Enforcer          *permissions.Enforcer
	Jobs              JobEnqueuer
	Audit             audit.Recorder

	Clock Clock
	IDGen IDGen
}

// Service provisions new integrators and customer tenants. See package doc
// and README for the six-step flow and rollback semantics.
type Service struct {
	cfg Config
}

// NewService constructs a Service. It returns an error if any required
// dependency is nil.
func NewService(cfg Config) (*Service, error) {
	if cfg.DB == nil {
		return nil, errors.New("tenants: DB is required")
	}
	if cfg.Bootstrapper == nil {
		return nil, errors.New("tenants: Bootstrapper is required")
	}
	if cfg.PermissionChecker == nil {
		return nil, errors.New("tenants: PermissionChecker is required")
	}
	if cfg.Enforcer == nil {
		return nil, errors.New("tenants: Enforcer is required")
	}
	if cfg.Jobs == nil {
		return nil, errors.New("tenants: Jobs is required")
	}
	if cfg.Audit == nil {
		return nil, errors.New("tenants: Audit is required")
	}
	if cfg.Clock == nil {
		cfg.Clock = func() time.Time { return time.Now().UTC() }
	}
	if cfg.IDGen == nil {
		cfg.IDGen = defaultIDGen
	}
	return &Service{cfg: cfg}, nil
}

func defaultIDGen() string {
	var b [16]byte
	if _, err := rand.Read(b[:]); err != nil {
		panic(fmt.Sprintf("tenants: rand.Read: %v", err))
	}
	return hex.EncodeToString(b[:])
}

// compensation records undo steps in reverse order. Run on any provisioning
// failure after the DB row is inserted.
type compensation struct {
	steps []func()
}

func (c *compensation) push(fn func()) { c.steps = append(c.steps, fn) }

func (c *compensation) runAll() {
	for i := len(c.steps) - 1; i >= 0; i-- {
		c.steps[i]()
	}
}

// ------------------------------------------------------------------
// CreateIntegrator
// ------------------------------------------------------------------

// CreateIntegrator provisions a brand-new root integrator (no parent).
// The caller must have the platform-level integrators.create action.
func (s *Service) CreateIntegrator(
	ctx context.Context,
	caller Caller,
	spec CreateIntegratorSpec,
) (*Integrator, error) {
	if err := caller.Validate(); err != nil {
		return nil, err
	}
	if err := s.authorize(ctx, caller, permissions.ActionIntegratorsCreate); err != nil {
		return nil, err
	}
	return s.createIntegratorInternal(ctx, caller, spec, nil)
}

// CreateSubReseller nests a new integrator under an existing parent. The
// caller must have integrators.create_subreseller AND the parent must exist
// inside the caller's region. The resulting depth must not exceed
// MaxSubResellerDepth.
func (s *Service) CreateSubReseller(
	ctx context.Context,
	caller Caller,
	parentIntegratorID string,
	spec CreateIntegratorSpec,
) (*Integrator, error) {
	if err := caller.Validate(); err != nil {
		return nil, err
	}
	if parentIntegratorID == "" {
		return nil, fmt.Errorf("%w: parent integrator id is required", ErrInvalidSpec)
	}
	if err := s.authorize(ctx, caller, permissions.ActionIntegratorsCreateSubReseller); err != nil {
		return nil, err
	}

	region := spec.Region
	if region == "" {
		region = clouddb.DefaultRegion
	}
	parent, err := s.cfg.DB.GetIntegrator(ctx, parentIntegratorID, region)
	if err != nil || parent == nil {
		return nil, fmt.Errorf("%w: %v", ErrParentIntegratorNotFound, err)
	}

	parentDepth, err := s.cfg.DB.IntegratorDepth(ctx, parent.ID, region)
	if err != nil {
		return nil, fmt.Errorf("tenants: resolve parent depth: %w", err)
	}
	if parentDepth+1 > MaxSubResellerDepth {
		return nil, fmt.Errorf("%w: adding child would make depth %d", ErrMaxDepthExceeded, parentDepth+1)
	}

	return s.createIntegratorInternal(ctx, caller, spec, &parent.ID)
}

func (s *Service) createIntegratorInternal(
	ctx context.Context,
	caller Caller,
	spec CreateIntegratorSpec,
	parentID *string,
) (*Integrator, error) {
	if strings.TrimSpace(spec.DisplayName) == "" {
		return nil, fmt.Errorf("%w: display name is required", ErrInvalidSpec)
	}
	region := spec.Region
	if region == "" {
		region = clouddb.DefaultRegion
	}
	id := spec.ID
	if id == "" {
		id = s.cfg.IDGen()
	}

	// Duplicate-name rejection (seam: single-region for now).
	n, err := s.cfg.DB.CountIntegratorsByDisplayName(ctx, spec.DisplayName, region)
	if err != nil {
		return nil, fmt.Errorf("tenants: count integrators: %w", err)
	}
	if n > 0 {
		return nil, ErrIntegratorExists
	}

	row := clouddb.Integrator{
		ID:                       id,
		ParentIntegratorID:       parentID,
		DisplayName:              spec.DisplayName,
		BillingMode:              BillingModeDirect, // hard-coded per KAI-227 scope
		WholesaleDiscountPercent: 0,
		Status:                   "active",
		Region:                   region,
	}
	if spec.LegalName != "" {
		v := spec.LegalName
		row.LegalName = &v
	}
	if spec.ContactEmail != "" {
		v := spec.ContactEmail
		row.ContactEmail = &v
	}

	// Step 1: insert row.
	if err := s.cfg.DB.InsertIntegrator(ctx, row); err != nil {
		return nil, fmt.Errorf("tenants: insert integrator: %w", err)
	}

	comp := &compensation{}
	comp.push(func() {
		_ = s.cfg.DB.DeleteIntegrator(context.Background(), row.ID, row.Region)
	})

	tenantRef := auth.TenantRef{Type: auth.TenantTypeIntegrator, ID: row.ID}

	// Step 2: identity provider org.
	orgID, err := s.cfg.Bootstrapper.CreateOrg(ctx, OrgSpec{
		Tenant:       tenantRef,
		DisplayName:  spec.DisplayName,
		ContactEmail: spec.ContactEmail,
	})
	if err != nil {
		comp.runAll()
		return nil, fmt.Errorf("tenants: bootstrap org: %w", err)
	}
	comp.push(func() {
		_ = s.cfg.Bootstrapper.DeleteOrg(context.Background(), orgID)
	})

	// Step 3: brand_config_id left NULL (KAI-310).

	// Step 4: seed Casbin.
	seeded, err := s.seedDefaultRoles(tenantRef)
	if err != nil {
		revertSeededRoles(s.cfg.Enforcer, seeded)
		comp.runAll()
		return nil, fmt.Errorf("tenants: seed roles: %w", err)
	}
	comp.push(func() { revertSeededRoles(s.cfg.Enforcer, seeded) })

	// Step 5: welcome email job.
	if spec.InitialAdminEmail != "" {
		if err := s.cfg.Jobs.EnqueueWelcomeEmail(ctx, WelcomeEmailJob{
			TenantID:    row.ID,
			TenantType:  string(auth.TenantTypeIntegrator),
			Email:       spec.InitialAdminEmail,
			DisplayName: spec.DisplayName,
		}); err != nil {
			comp.runAll()
			return nil, fmt.Errorf("tenants: enqueue welcome: %w", err)
		}
	}

	// Step 6: audit entry.
	if err := s.emitProvisionAudit(ctx, caller, row.ID, "integrator", audit.ResultAllow, ""); err != nil {
		comp.runAll()
		return nil, fmt.Errorf("tenants: audit: %w", err)
	}

	result := &Integrator{
		Row:          row,
		ZitadelOrgID: string(orgID),
	}

	// Optional: create initial admin (separate audit entry).
	if spec.InitialAdminEmail != "" {
		inv, err := s.inviteAdminInternal(ctx, caller, tenantRef, orgID, spec.InitialAdminEmail, row.Region)
		if err != nil {
			// Inviting is best-effort once the tenant is live: don't
			// unwind the whole integrator. The caller can retry.
			return result, fmt.Errorf("tenants: invite initial admin: %w", err)
		}
		result.InitialAdmin = inv
	}

	return result, nil
}

// ------------------------------------------------------------------
// CreateCustomerTenant
// ------------------------------------------------------------------

// CreateCustomerTenant provisions a new end-customer tenant.
func (s *Service) CreateCustomerTenant(
	ctx context.Context,
	caller Caller,
	spec CreateCustomerTenantSpec,
) (*CustomerTenant, error) {
	if err := caller.Validate(); err != nil {
		return nil, err
	}
	if err := s.authorize(ctx, caller, permissions.ActionCustomerTenantsCreate); err != nil {
		return nil, err
	}
	if strings.TrimSpace(spec.DisplayName) == "" {
		return nil, fmt.Errorf("%w: display name is required", ErrInvalidSpec)
	}

	billingMode := spec.BillingMode
	if billingMode == "" {
		billingMode = BillingModeDirect
	}
	if billingMode != BillingModeDirect && billingMode != BillingModeViaIntegrator {
		return nil, fmt.Errorf("%w: unknown billing mode %q", ErrInvalidSpec, billingMode)
	}
	if billingMode == BillingModeViaIntegrator && spec.HomeIntegratorID == "" {
		return nil, ErrMissingHomeIntegrator
	}

	region := spec.Region
	if region == "" {
		region = clouddb.DefaultRegion
	}
	id := spec.ID
	if id == "" {
		id = s.cfg.IDGen()
	}

	n, err := s.cfg.DB.CountCustomerTenantsByDisplayName(ctx, spec.DisplayName, region)
	if err != nil {
		return nil, fmt.Errorf("tenants: count customer tenants: %w", err)
	}
	if n > 0 {
		return nil, ErrCustomerTenantExists
	}

	row := clouddb.CustomerTenant{
		ID:          id,
		DisplayName: spec.DisplayName,
		BillingMode: billingMode,
		Status:      "active",
		Region:      region,
	}
	if spec.HomeIntegratorID != "" {
		v := spec.HomeIntegratorID
		row.HomeIntegratorID = &v
	}
	if spec.SignupSource != "" {
		v := spec.SignupSource
		row.SignupSource = &v
	}

	if err := s.cfg.DB.InsertCustomerTenant(ctx, row); err != nil {
		return nil, fmt.Errorf("tenants: insert customer tenant: %w", err)
	}
	comp := &compensation{}
	comp.push(func() {
		_ = s.cfg.DB.DeleteCustomerTenant(context.Background(), row.ID, row.Region)
	})

	tenantRef := auth.TenantRef{Type: auth.TenantTypeCustomer, ID: row.ID}

	orgID, err := s.cfg.Bootstrapper.CreateOrg(ctx, OrgSpec{
		Tenant:       tenantRef,
		DisplayName:  spec.DisplayName,
		ContactEmail: spec.InitialAdminEmail,
	})
	if err != nil {
		comp.runAll()
		return nil, fmt.Errorf("tenants: bootstrap org: %w", err)
	}
	comp.push(func() {
		_ = s.cfg.Bootstrapper.DeleteOrg(context.Background(), orgID)
	})

	seeded, err := s.seedDefaultRoles(tenantRef)
	if err != nil {
		revertSeededRoles(s.cfg.Enforcer, seeded)
		comp.runAll()
		return nil, fmt.Errorf("tenants: seed roles: %w", err)
	}
	comp.push(func() { revertSeededRoles(s.cfg.Enforcer, seeded) })

	if spec.InitialAdminEmail != "" {
		if err := s.cfg.Jobs.EnqueueWelcomeEmail(ctx, WelcomeEmailJob{
			TenantID:    row.ID,
			TenantType:  string(auth.TenantTypeCustomer),
			Email:       spec.InitialAdminEmail,
			DisplayName: spec.DisplayName,
		}); err != nil {
			comp.runAll()
			return nil, fmt.Errorf("tenants: enqueue welcome: %w", err)
		}
	}

	if err := s.emitProvisionAudit(ctx, caller, row.ID, "customer_tenant", audit.ResultAllow, ""); err != nil {
		comp.runAll()
		return nil, fmt.Errorf("tenants: audit: %w", err)
	}

	result := &CustomerTenant{Row: row, ZitadelOrgID: string(orgID)}

	if spec.InitialAdminEmail != "" {
		inv, err := s.inviteAdminInternal(ctx, caller, tenantRef, orgID, spec.InitialAdminEmail, row.Region)
		if err != nil {
			return result, fmt.Errorf("tenants: invite initial admin: %w", err)
		}
		result.InitialAdmin = inv
	}

	return result, nil
}

// ------------------------------------------------------------------
// InviteInitialAdmin
// ------------------------------------------------------------------

// InviteInitialAdmin creates the first admin user for an already-provisioned
// tenant. It is safe to call multiple times; the caller is expected to
// deduplicate on email.
func (s *Service) InviteInitialAdmin(
	ctx context.Context,
	caller Caller,
	tenant TenantRef,
	email string,
) (*Invitation, error) {
	if err := caller.Validate(); err != nil {
		return nil, err
	}
	if err := s.authorize(ctx, caller, permissions.ActionTenantsInviteAdmin); err != nil {
		return nil, err
	}
	if tenant.IsZero() {
		return nil, fmt.Errorf("%w: tenant is required", ErrInvalidSpec)
	}
	if strings.TrimSpace(email) == "" {
		return nil, fmt.Errorf("%w: email is required", ErrInvalidSpec)
	}

	region := clouddb.DefaultRegion
	// Look up the canonical org id via the tenant row so we can pass it
	// to the bootstrapper. In v1 we store the Zitadel org id alongside
	// the tenant row in a future column (KAI-223); for now the in-memory
	// bootstrapper indexes by tenant so we re-derive the OrgID by name.
	// Callers get the OrgID back from the original Create* call, so the
	// production HTTP layer will always have it available.
	orgID := OrgID("") // unknown here — caller must have used Create* path
	return s.inviteAdminInternal(ctx, caller, tenant, orgID, email, region)
}

// inviteAdminInternal is the shared invite-admin path used by both the
// Create* flows and the top-level InviteInitialAdmin. orgID may be empty
// when called via the public endpoint — the MemoryBootstrapper accepts
// that and looks up by tenant.
func (s *Service) inviteAdminInternal(
	ctx context.Context,
	caller Caller,
	tenant TenantRef,
	orgID OrgID,
	email string,
	region string,
) (*Invitation, error) {
	// If we don't have an org id, the public InviteInitialAdmin path was
	// used. The MemoryBootstrapper does not support tenant-lookup of org
	// ids so we return an error — in production the Zitadel adapter
	// will materialise orgID from tenant metadata.
	if orgID == "" {
		return nil, fmt.Errorf("tenants: InviteInitialAdmin requires a tenant previously created via Create*; orgID not resolvable")
	}

	adminUser, err := s.cfg.Bootstrapper.CreateInitialAdmin(ctx, orgID, email)
	if err != nil {
		return nil, fmt.Errorf("tenants: bootstrap admin: %w", err)
	}

	userRow := clouddb.User{
		ID:            s.cfg.IDGen(),
		Email:         email,
		Status:        "invited",
		Region:        region,
	}
	zid := adminUser.UserID
	userRow.ZitadelUserID = &zid
	dbTenant := clouddb.TenantRef{
		Type:   dbTenantType(tenant.Type),
		ID:     tenant.ID,
		Region: region,
	}
	if err := s.cfg.DB.InsertUser(ctx, dbTenant, userRow); err != nil {
		return nil, fmt.Errorf("tenants: insert user: %w", err)
	}

	// Audit entry for the invite.
	if err := s.emitAudit(ctx, caller, "tenant.invite_admin", tenant.ID, "user", userRow.ID, audit.ResultAllow, ""); err != nil {
		_ = s.cfg.DB.DeleteUser(context.Background(), userRow.ID, userRow.Region)
		return nil, fmt.Errorf("tenants: audit invite: %w", err)
	}

	return &Invitation{
		UserRow:       userRow,
		ZitadelUserID: adminUser.UserID,
		Email:         email,
		InviteToken:   adminUser.InviteToken,
		ExpiresAt:     s.cfg.Clock().Add(7 * 24 * time.Hour),
	}, nil
}

func dbTenantType(t auth.TenantType) clouddb.TenantRefType {
	if t == auth.TenantTypeIntegrator {
		return clouddb.TenantIntegrator
	}
	return clouddb.TenantCustomerTenant
}

// ------------------------------------------------------------------
// Casbin seed / revert
// ------------------------------------------------------------------

// seededRole records a (role-id, rules) pair so we can revert on failure.
type seededRole struct {
	Rules []permissions.PolicyRule
}

func (s *Service) seedDefaultRoles(tenant auth.TenantRef) ([]seededRole, error) {
	out := make([]seededRole, 0, len(permissions.DefaultRoleTemplates))
	for _, tmpl := range permissions.DefaultRoleTemplates {
		// Track which rules we are about to add so we can revert exactly
		// those and nothing else.
		before := snapshotPolicies(s.cfg.Enforcer)
		if _, err := permissions.SeedRole(s.cfg.Enforcer, tmpl, tenant); err != nil {
			return out, err
		}
		after := snapshotPolicies(s.cfg.Enforcer)
		out = append(out, seededRole{Rules: diffPolicies(before, after)})
	}
	return out, nil
}

func revertSeededRoles(e *permissions.Enforcer, seeded []seededRole) {
	for i := len(seeded) - 1; i >= 0; i-- {
		for _, r := range seeded[i].Rules {
			_ = e.RemovePolicy(r)
		}
	}
}

func snapshotPolicies(e *permissions.Enforcer) []permissions.PolicyRule {
	src := e.Store().ListPolicies()
	out := make([]permissions.PolicyRule, len(src))
	copy(out, src)
	return out
}

func diffPolicies(before, after []permissions.PolicyRule) []permissions.PolicyRule {
	b := make(map[permissions.PolicyRule]struct{}, len(before))
	for _, r := range before {
		b[r] = struct{}{}
	}
	var out []permissions.PolicyRule
	for _, r := range after {
		if _, ok := b[r]; !ok {
			out = append(out, r)
		}
	}
	return out
}

// ------------------------------------------------------------------
// authorization + audit helpers
// ------------------------------------------------------------------

func (s *Service) authorize(ctx context.Context, caller Caller, action string) error {
	subject := permissions.NewUserSubject(caller.UserID, caller.Tenant)
	object := permissions.NewObjectAll(platformTenant(), "tenants")
	allowed, err := s.cfg.PermissionChecker.Enforce(ctx, subject, object, action)
	if err != nil {
		return fmt.Errorf("tenants: permission check: %w", err)
	}
	if !allowed {
		return fmt.Errorf("%w: %s", ErrPermissionDenied, action)
	}
	return nil
}

func (s *Service) emitProvisionAudit(
	ctx context.Context,
	caller Caller,
	tenantID string,
	resourceType string,
	result audit.Result,
	errorCode string,
) error {
	return s.emitAudit(ctx, caller, "tenant.provision", tenantID, resourceType, tenantID, result, errorCode)
}

func (s *Service) emitAudit(
	ctx context.Context,
	caller Caller,
	action string,
	tenantID string,
	resourceType string,
	resourceID string,
	result audit.Result,
	errorCode string,
) error {
	entry := audit.Entry{
		TenantID:     tenantID,
		ActorUserID:  string(caller.UserID),
		ActorAgent:   audit.AgentCloud,
		Action:       action,
		ResourceType: resourceType,
		ResourceID:   resourceID,
		Result:       result,
		Timestamp:    s.cfg.Clock(),
	}
	if errorCode != "" {
		e := errorCode
		entry.ErrorCode = &e
	}
	return s.cfg.Audit.Record(ctx, entry)
}
