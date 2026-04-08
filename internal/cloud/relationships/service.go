package relationships

import (
	"context"
	"crypto/rand"
	"database/sql"
	"encoding/hex"
	"errors"
	"fmt"
	"time"

	"github.com/bluenviron/mediamtx/internal/cloud/audit"
	clouddb "github.com/bluenviron/mediamtx/internal/cloud/db"
	"github.com/bluenviron/mediamtx/internal/cloud/permissions"
	"github.com/bluenviron/mediamtx/internal/shared/auth"
)

// PermissionChecker is the narrow Casbin seam this service depends on.
// Production code uses tenants.NewEnforcerChecker; tests may pass allow/deny
// stubs.
type PermissionChecker interface {
	Enforce(ctx context.Context, subject permissions.SubjectRef, object permissions.ObjectRef, action string) (bool, error)
}

// Clock is a now-source for tests to freeze time.
type Clock func() time.Time

// IDGen generates a random string id. Default is 16-byte hex.
type IDGen func() string

// Config bundles all dependencies of Service.
type Config struct {
	DB                *clouddb.DB
	PermissionChecker PermissionChecker
	Audit             audit.Recorder
	Clock             Clock
	IDGen             IDGen
}

// Service implements the customer-integrator relationship API (KAI-228) and
// the sub-reseller hierarchy management (KAI-229).
type Service struct {
	cfg Config
}

// NewService constructs a Service. All fields are required.
func NewService(cfg Config) (*Service, error) {
	if cfg.DB == nil {
		return nil, errors.New("relationships: DB is required")
	}
	if cfg.PermissionChecker == nil {
		return nil, errors.New("relationships: PermissionChecker is required")
	}
	if cfg.Audit == nil {
		return nil, errors.New("relationships: Audit is required")
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
		panic(fmt.Sprintf("relationships: rand.Read: %v", err))
	}
	return hex.EncodeToString(b[:])
}

// ------------------------------------------------------------------
// KAI-228: Create
// ------------------------------------------------------------------

// Create inserts a new customer-integrator relationship. The flow depends on
// spec.Initiator:
//
//   - InitiatorCustomer:    customer admin grants integrator; status → active.
//   - InitiatorIntegrator:  integrator requests access; status → pending_acceptance.
//
// The caller.Tenant must match spec.CustomerTenantID (customer-initiated) or
// must be the integrator's own tenant (integrator-initiated). The Casbin check
// is relationships.grant for customer-initiated, relationships.write for
// integrator-initiated (creates only a pending request).
func (s *Service) Create(ctx context.Context, caller Caller, spec CreateSpec) (*Relationship, error) {
	if err := caller.Validate(); err != nil {
		return nil, err
	}
	if err := validateCreateSpec(spec); err != nil {
		return nil, err
	}

	region := clouddb.DefaultRegion

	// Determine required action and initial status.
	var requiredAction string
	var initialStatus string
	switch spec.Initiator {
	case InitiatorCustomer:
		// Customer admin directly grants — caller must own the customer tenant.
		if caller.Tenant.Type != auth.TenantTypeCustomer || caller.Tenant.ID != spec.CustomerTenantID {
			return nil, fmt.Errorf("%w: customer-initiated grant requires caller to be the customer tenant", ErrPermissionDenied)
		}
		requiredAction = permissions.ActionRelationshipsGrant
		initialStatus = StatusActive
	case InitiatorIntegrator:
		// Integrator requests access — caller must be in the integrator's tenant.
		if caller.Tenant.Type != auth.TenantTypeIntegrator || caller.Tenant.ID != spec.IntegratorID {
			return nil, fmt.Errorf("%w: integrator-initiated request requires caller to be the integrator", ErrPermissionDenied)
		}
		requiredAction = permissions.ActionRelationshipsWrite
		initialStatus = StatusPendingAcceptance
	default:
		return nil, fmt.Errorf("%w: unknown initiator %q", ErrInvalidSpec, spec.Initiator)
	}

	// Casbin check. Object is scoped to the customer tenant being granted.
	customerTenantRef := auth.TenantRef{Type: auth.TenantTypeCustomer, ID: spec.CustomerTenantID}
	if err := s.checkPermission(ctx, caller, customerTenantRef, "relationships", requiredAction); err != nil {
		return nil, err
	}

	// Idempotency: reject if a row already exists.
	existing, err := s.cfg.DB.GetCustomerIntegratorRelationship(ctx, spec.CustomerTenantID, spec.IntegratorID, region)
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		return nil, fmt.Errorf("relationships: check existing: %w", err)
	}
	if existing != nil {
		return nil, ErrAlreadyExists
	}

	roleTemplate := spec.RoleTemplate
	if roleTemplate == "" {
		roleTemplate = RoleFullManagement
	}

	permsJSON, err := encodeScopedPermissions(spec.ScopedPermissions)
	if err != nil {
		return nil, err
	}

	now := s.cfg.Clock()
	grantedByStr := string(caller.UserID)
	row := clouddb.CustomerIntegratorRelationship{
		CustomerTenantID:  spec.CustomerTenantID,
		IntegratorID:      spec.IntegratorID,
		ScopedPermissions: permsJSON,
		RoleTemplate:      roleTemplate,
		MarkupPercent:     0,
		Status:            initialStatus,
		GrantedAt:         now,
		GrantedByUserID:   &grantedByStr,
	}

	customerRef := clouddb.TenantRef{
		Type:   clouddb.TenantCustomerTenant,
		ID:     spec.CustomerTenantID,
		Region: region,
	}
	if err := s.cfg.DB.InsertCustomerIntegratorRelationship(ctx, customerRef, row); err != nil {
		return nil, fmt.Errorf("relationships: insert: %w", err)
	}

	auditAction := "relationship.grant"
	if spec.Initiator == InitiatorIntegrator {
		auditAction = "relationship.request"
	}
	if auditErr := s.emitAudit(ctx, caller, auditAction, spec.CustomerTenantID,
		"relationship", relationshipID(spec.CustomerTenantID, spec.IntegratorID),
		audit.ResultAllow, ""); auditErr != nil {
		// Audit failure is non-fatal but logged via the audit system itself.
		_ = auditErr
	}

	return s.toRelationship(row)
}

// ------------------------------------------------------------------
// KAI-228: Approve (pending → active)
// ------------------------------------------------------------------

// Approve transitions a pending_acceptance relationship to active. Only the
// customer tenant's admin may approve (relationships.grant).
func (s *Service) Approve(ctx context.Context, caller Caller, spec ApproveSpec) (*Relationship, error) {
	if err := caller.Validate(); err != nil {
		return nil, err
	}
	if spec.CustomerTenantID == "" || spec.IntegratorID == "" {
		return nil, fmt.Errorf("%w: customer_tenant_id and integrator_id are required", ErrInvalidSpec)
	}
	if caller.Tenant.Type != auth.TenantTypeCustomer || caller.Tenant.ID != spec.CustomerTenantID {
		return nil, fmt.Errorf("%w: only the customer tenant may approve", ErrPermissionDenied)
	}

	customerTenantRef := auth.TenantRef{Type: auth.TenantTypeCustomer, ID: spec.CustomerTenantID}
	if err := s.checkPermission(ctx, caller, customerTenantRef, "relationships", permissions.ActionRelationshipsGrant); err != nil {
		return nil, err
	}

	region := clouddb.DefaultRegion
	existing, err := s.cfg.DB.GetCustomerIntegratorRelationship(ctx, spec.CustomerTenantID, spec.IntegratorID, region)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, ErrNotFound
		}
		return nil, fmt.Errorf("relationships: get: %w", err)
	}
	if existing.Status != StatusPendingAcceptance {
		return nil, fmt.Errorf("%w: current status %q cannot transition to active", ErrInvalidTransition, existing.Status)
	}

	existing.Status = StatusActive
	if err := s.cfg.DB.UpdateCustomerIntegratorRelationship(ctx, region, *existing); err != nil {
		return nil, fmt.Errorf("relationships: approve update: %w", err)
	}

	_ = s.emitAudit(ctx, caller, "relationship.approve", spec.CustomerTenantID,
		"relationship", relationshipID(spec.CustomerTenantID, spec.IntegratorID),
		audit.ResultAllow, "")

	return s.toRelationship(*existing)
}

// ------------------------------------------------------------------
// KAI-228: Update
// ------------------------------------------------------------------

// Update patches the mutable fields of an existing active relationship.
// Requires relationships.write on the customer tenant.
func (s *Service) Update(ctx context.Context, caller Caller, customerTenantID, integratorID string, spec UpdateSpec) (*Relationship, error) {
	if err := caller.Validate(); err != nil {
		return nil, err
	}
	if customerTenantID == "" || integratorID == "" {
		return nil, fmt.Errorf("%w: customer_tenant_id and integrator_id are required", ErrInvalidSpec)
	}

	customerTenantRef := auth.TenantRef{Type: auth.TenantTypeCustomer, ID: customerTenantID}
	if err := s.checkPermission(ctx, caller, customerTenantRef, "relationships", permissions.ActionRelationshipsWrite); err != nil {
		return nil, err
	}

	region := clouddb.DefaultRegion
	existing, err := s.cfg.DB.GetCustomerIntegratorRelationship(ctx, customerTenantID, integratorID, region)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, ErrNotFound
		}
		return nil, fmt.Errorf("relationships: get for update: %w", err)
	}
	if existing.Status == StatusRevoked {
		return nil, fmt.Errorf("%w: cannot update a revoked relationship", ErrInvalidTransition)
	}

	// Apply spec fields.
	if spec.RoleTemplate != "" {
		existing.RoleTemplate = spec.RoleTemplate
	}
	if spec.ScopedPermissions != nil {
		p, err := encodeScopedPermissions(*spec.ScopedPermissions)
		if err != nil {
			return nil, err
		}
		existing.ScopedPermissions = p
	}
	if spec.MarkupPercent != nil {
		existing.MarkupPercent = *spec.MarkupPercent
	}

	if err := s.cfg.DB.UpdateCustomerIntegratorRelationship(ctx, region, *existing); err != nil {
		return nil, fmt.Errorf("relationships: update: %w", err)
	}

	_ = s.emitAudit(ctx, caller, "relationship.update", customerTenantID,
		"relationship", relationshipID(customerTenantID, integratorID),
		audit.ResultAllow, "")

	return s.toRelationship(*existing)
}

// ------------------------------------------------------------------
// KAI-228: Revoke
// ------------------------------------------------------------------

// Revoke transitions a relationship to revoked. Both the customer admin and
// the integrator may revoke (relationships.grant from either side). The
// tenant-scoping check differs: customer callers are checked against the
// customer tenant; integrator callers are checked against their own tenant.
func (s *Service) Revoke(ctx context.Context, caller Caller, spec RevokeSpec) error {
	if err := caller.Validate(); err != nil {
		return err
	}
	if spec.CustomerTenantID == "" || spec.IntegratorID == "" {
		return fmt.Errorf("%w: customer_tenant_id and integrator_id are required", ErrInvalidSpec)
	}

	// Determine the object tenant for the Casbin check.
	var checkTenant auth.TenantRef
	switch caller.Tenant.Type {
	case auth.TenantTypeCustomer:
		if caller.Tenant.ID != spec.CustomerTenantID {
			return fmt.Errorf("%w: customer caller may only revoke their own relationships", ErrPermissionDenied)
		}
		checkTenant = auth.TenantRef{Type: auth.TenantTypeCustomer, ID: spec.CustomerTenantID}
	case auth.TenantTypeIntegrator:
		if caller.Tenant.ID != spec.IntegratorID {
			return fmt.Errorf("%w: integrator caller may only revoke their own relationships", ErrPermissionDenied)
		}
		// Object is the customer tenant being accessed.
		checkTenant = auth.TenantRef{Type: auth.TenantTypeCustomer, ID: spec.CustomerTenantID}
	default:
		return fmt.Errorf("%w: unsupported caller tenant type %q", ErrPermissionDenied, caller.Tenant.Type)
	}

	if err := s.checkPermission(ctx, caller, checkTenant, "relationships", permissions.ActionRelationshipsGrant); err != nil {
		return err
	}

	region := clouddb.DefaultRegion
	existing, err := s.cfg.DB.GetCustomerIntegratorRelationship(ctx, spec.CustomerTenantID, spec.IntegratorID, region)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return ErrNotFound
		}
		return fmt.Errorf("relationships: get for revoke: %w", err)
	}
	if existing.Status == StatusRevoked {
		return nil // idempotent
	}

	now := s.cfg.Clock()
	existing.Status = StatusRevoked
	existing.RevokedAt = &now

	if err := s.cfg.DB.UpdateCustomerIntegratorRelationship(ctx, region, *existing); err != nil {
		return fmt.Errorf("relationships: revoke update: %w", err)
	}

	_ = s.emitAudit(ctx, caller, "relationship.revoke", spec.CustomerTenantID,
		"relationship", relationshipID(spec.CustomerTenantID, spec.IntegratorID),
		audit.ResultAllow, "")

	return nil
}

// ------------------------------------------------------------------
// KAI-228: List
// ------------------------------------------------------------------

// ListForCustomer returns all relationships for a customer tenant.
// Requires relationships.read on the customer tenant.
func (s *Service) ListForCustomer(ctx context.Context, caller Caller, customerTenantID string) ([]Relationship, error) {
	if err := caller.Validate(); err != nil {
		return nil, err
	}
	customerTenantRef := auth.TenantRef{Type: auth.TenantTypeCustomer, ID: customerTenantID}
	if err := s.checkPermission(ctx, caller, customerTenantRef, "relationships", permissions.ActionRelationshipsRead); err != nil {
		return nil, err
	}

	region := clouddb.DefaultRegion
	rows, err := s.cfg.DB.ListRelationshipsForCustomer(ctx, clouddb.TenantRef{
		Type:   clouddb.TenantCustomerTenant,
		ID:     customerTenantID,
		Region: region,
	})
	if err != nil {
		return nil, fmt.Errorf("relationships: list for customer: %w", err)
	}
	return s.toRelationshipSlice(rows)
}

// ListForIntegrator returns all relationships visible to an integrator on its
// direct customers. Requires relationships.read on the integrator's own tenant.
func (s *Service) ListForIntegrator(ctx context.Context, caller Caller, integratorID string) ([]Relationship, error) {
	if err := caller.Validate(); err != nil {
		return nil, err
	}
	if caller.Tenant.Type != auth.TenantTypeIntegrator || caller.Tenant.ID != integratorID {
		return nil, fmt.Errorf("%w: integrator caller required", ErrPermissionDenied)
	}
	integratorRef := auth.TenantRef{Type: auth.TenantTypeIntegrator, ID: integratorID}
	if err := s.checkPermission(ctx, caller, integratorRef, "relationships", permissions.ActionRelationshipsRead); err != nil {
		return nil, err
	}

	region := clouddb.DefaultRegion
	rows, err := s.cfg.DB.ListRelationshipsForIntegrator(ctx, integratorID, region)
	if err != nil {
		return nil, fmt.Errorf("relationships: list for integrator: %w", err)
	}
	return s.toRelationshipSlice(rows)
}

// ------------------------------------------------------------------
// KAI-229: ListVisibleCustomers
// ------------------------------------------------------------------

// ListVisibleCustomers returns every customer tenant reachable from the given
// integrator: its own direct relationships PLUS all relationships owned by
// its sub-reseller descendants (KAI-229 recursive walk).
// Requires relationships.read on the integrator's own tenant.
func (s *Service) ListVisibleCustomers(ctx context.Context, caller Caller, rootIntegratorID string) ([]visibleCustomersResult, error) {
	if err := caller.Validate(); err != nil {
		return nil, err
	}
	if caller.Tenant.Type != auth.TenantTypeIntegrator || caller.Tenant.ID != rootIntegratorID {
		return nil, fmt.Errorf("%w: integrator caller required", ErrPermissionDenied)
	}
	integratorRef := auth.TenantRef{Type: auth.TenantTypeIntegrator, ID: rootIntegratorID}
	if err := s.checkPermission(ctx, caller, integratorRef, "relationships", permissions.ActionRelationshipsRead); err != nil {
		return nil, err
	}

	region := clouddb.DefaultRegion
	rows, err := s.cfg.DB.ListRelationshipsForIntegratorSubtree(ctx, rootIntegratorID, region)
	if err != nil {
		return nil, fmt.Errorf("relationships: list visible customers: %w", err)
	}

	out := make([]visibleCustomersResult, len(rows))
	for i, r := range rows {
		out[i] = visibleCustomersResult{
			CustomerTenantID: r.CustomerTenantID,
			IntegratorID:     r.IntegratorID,
			Status:           r.Status,
		}
	}
	return out, nil
}

// ------------------------------------------------------------------
// KAI-229: HierarchyForIntegrator
// ------------------------------------------------------------------

// HierarchyForIntegrator returns the full sub-reseller tree rooted at the
// given integrator. The tree is limited to 3 levels (MaxSubResellerDepth).
// Requires relationships.read.
func (s *Service) HierarchyForIntegrator(ctx context.Context, caller Caller, rootIntegratorID string) (*HierarchyNode, error) {
	if err := caller.Validate(); err != nil {
		return nil, err
	}
	if caller.Tenant.Type != auth.TenantTypeIntegrator || caller.Tenant.ID != rootIntegratorID {
		return nil, fmt.Errorf("%w: integrator caller required for hierarchy", ErrPermissionDenied)
	}
	integratorRef := auth.TenantRef{Type: auth.TenantTypeIntegrator, ID: rootIntegratorID}
	if err := s.checkPermission(ctx, caller, integratorRef, "relationships", permissions.ActionRelationshipsRead); err != nil {
		return nil, err
	}

	region := clouddb.DefaultRegion
	return s.buildHierarchy(ctx, rootIntegratorID, nil, region, 0)
}

// buildHierarchy recursively builds the tree.  parentPerms carries the
// ancestor permission set to intersect at each level; nil means root.
func (s *Service) buildHierarchy(
	ctx context.Context,
	integratorID string,
	parentIntegratorID *string,
	region string,
	depth int,
) (*HierarchyNode, error) {
	const maxDepth = 3 // mirrors MaxSubResellerDepth in the tenants package
	if depth > maxDepth {
		return nil, nil // safety guard; schema enforces this at write time
	}

	node := &HierarchyNode{
		IntegratorID:       integratorID,
		ParentIntegratorID: parentIntegratorID,
	}

	children, err := s.cfg.DB.ListChildIntegrators(ctx, integratorID, region)
	if err != nil {
		return nil, fmt.Errorf("relationships: list children of %s: %w", integratorID, err)
	}
	for _, child := range children {
		childID := child.ID
		childNode, err := s.buildHierarchy(ctx, childID, &integratorID, region, depth+1)
		if err != nil {
			return nil, err
		}
		if childNode != nil {
			node.Children = append(node.Children, childNode)
		}
	}
	return node, nil
}

// ------------------------------------------------------------------
// KAI-229: ValidateSubResellerScope
// ------------------------------------------------------------------

// ValidateSubResellerScope verifies that a sub-reseller's requested
// scoped_permissions do not exceed its parent's scoped_permissions. It
// returns ErrScopeEscalation when the child tries to claim an action the
// parent does not possess.
//
// parentActions == nil means the parent is a root integrator with full access,
// so any child scope is valid.
func ValidateSubResellerScope(parentActions, childActions []string) error {
	if parentActions == nil {
		// Root level: full scope — any child scope is valid.
		return nil
	}
	parent := toActionSet(parentActions)
	for _, a := range childActions {
		if _, ok := parent[a]; !ok {
			return fmt.Errorf("%w: action %q not in parent scope", ErrScopeEscalation, a)
		}
	}
	return nil
}

func toActionSet(actions []string) map[string]struct{} {
	m := make(map[string]struct{}, len(actions))
	for _, a := range actions {
		m[a] = struct{}{}
	}
	return m
}

// ------------------------------------------------------------------
// Internal helpers
// ------------------------------------------------------------------

// checkPermission runs a Casbin enforce call, scoped to the given object tenant.
func (s *Service) checkPermission(
	ctx context.Context,
	caller Caller,
	objectTenant auth.TenantRef,
	resourceType string,
	action string,
) error {
	sub := permissions.NewUserSubject(caller.UserID, caller.Tenant)
	obj := permissions.NewObjectAll(objectTenant, resourceType)
	allowed, err := s.cfg.PermissionChecker.Enforce(ctx, sub, obj, action)
	if err != nil {
		return fmt.Errorf("relationships: permission check: %w", err)
	}
	if !allowed {
		_ = s.emitAudit(ctx, caller, action, objectTenant.ID, resourceType, "*", audit.ResultDeny, "")
		return fmt.Errorf("%w: %s", ErrPermissionDenied, action)
	}
	return nil
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
		ActorAgent:   agentForTenant(caller.Tenant),
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

func agentForTenant(t auth.TenantRef) audit.ActorAgent {
	if t.Type == auth.TenantTypeIntegrator {
		return audit.AgentIntegrator
	}
	return audit.AgentCloud
}

func (s *Service) toRelationship(row clouddb.CustomerIntegratorRelationship) (*Relationship, error) {
	perms, err := decodeScopedPermissions(row.ScopedPermissions)
	if err != nil {
		return nil, err
	}
	r := &Relationship{
		CustomerTenantID:  row.CustomerTenantID,
		IntegratorID:      row.IntegratorID,
		ScopedPermissions: perms,
		RoleTemplate:      row.RoleTemplate,
		MarkupPercent:     row.MarkupPercent,
		Status:            row.Status,
		GrantedAt:         row.GrantedAt,
		GrantedByUserID:   row.GrantedByUserID,
		RevokedAt:         row.RevokedAt,
	}
	return r, nil
}

func (s *Service) toRelationshipSlice(rows []clouddb.CustomerIntegratorRelationship) ([]Relationship, error) {
	out := make([]Relationship, 0, len(rows))
	for _, row := range rows {
		r, err := s.toRelationship(row)
		if err != nil {
			return nil, err
		}
		out = append(out, *r)
	}
	return out, nil
}

// relationshipID builds a composite resource-id string for audit entries.
func relationshipID(customerTenantID, integratorID string) string {
	return customerTenantID + "/" + integratorID
}

func validateCreateSpec(spec CreateSpec) error {
	if spec.CustomerTenantID == "" {
		return fmt.Errorf("%w: customer_tenant_id is required", ErrInvalidSpec)
	}
	if spec.IntegratorID == "" {
		return fmt.Errorf("%w: integrator_id is required", ErrInvalidSpec)
	}
	if spec.Initiator != InitiatorCustomer && spec.Initiator != InitiatorIntegrator {
		return fmt.Errorf("%w: initiator must be %q or %q", ErrInvalidSpec, InitiatorCustomer, InitiatorIntegrator)
	}
	switch spec.RoleTemplate {
	case "", RoleFullManagement, RoleMonitoringOnly, RoleEmergencyAccess, RoleCustom:
		// valid
	default:
		return fmt.Errorf("%w: unknown role template %q", ErrInvalidSpec, spec.RoleTemplate)
	}
	if spec.RoleTemplate == RoleCustom && len(spec.ScopedPermissions) == 0 {
		return fmt.Errorf("%w: custom role requires at least one scoped permission", ErrInvalidSpec)
	}
	return nil
}
