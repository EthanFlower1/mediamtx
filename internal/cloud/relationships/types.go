package relationships

import (
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/bluenviron/mediamtx/internal/shared/auth"
)

// Status values mirror the database CHECK constraint in migration 0003.
const (
	StatusPendingAcceptance = "pending_acceptance"
	StatusActive            = "active"
	StatusRevoked           = "revoked"
)

// RoleTemplate values mirror the database CHECK constraint.
const (
	RoleFullManagement = "full_management"
	RoleMonitoringOnly = "monitoring_only"
	RoleEmergencyAccess = "emergency_access"
	RoleCustom          = "custom"
)

// Relationship is the service-layer view of a customer_integrator_relationships
// row. It decodes scoped_permissions JSON into a typed slice.
type Relationship struct {
	CustomerTenantID  string
	IntegratorID      string
	// ScopedPermissions is the explicit allowlist of Casbin action strings this
	// relationship grants. An empty slice with RoleTemplate != "custom" means
	// "derive from the role template". A non-empty slice with RoleTemplate ==
	// "custom" is the explicit override (KAI-362 markup flows through markup_percent
	// instead).
	ScopedPermissions []string
	RoleTemplate      string
	// MarkupPercent is reserved for via_integrator billing (KAI-362). Set to 0
	// until that ticket is implemented.
	MarkupPercent float64
	Status        string
	GrantedAt     time.Time
	GrantedByUserID *string
	RevokedAt      *time.Time
}

// Initiator is who started the relationship creation request.
type Initiator string

const (
	// InitiatorCustomer means the customer admin directly granted access.
	// The initial status is "active".
	InitiatorCustomer Initiator = "customer"
	// InitiatorIntegrator means the integrator requested access.
	// The initial status is "pending_acceptance".
	InitiatorIntegrator Initiator = "integrator"
)

// CreateSpec is the input to Service.Create.
type CreateSpec struct {
	// CustomerTenantID is always required.
	CustomerTenantID string
	// IntegratorID is always required.
	IntegratorID string
	// RoleTemplate selects the built-in permission set. Defaults to
	// RoleFullManagement when empty.
	RoleTemplate string
	// ScopedPermissions overrides the role template when RoleTemplate ==
	// RoleCustom. Must be non-nil if RoleTemplate is RoleCustom.
	ScopedPermissions []string
	// Initiator controls who started the flow and thus the initial status.
	Initiator Initiator
}

// UpdateSpec is the input to Service.Update.
type UpdateSpec struct {
	// RoleTemplate replaces the current template. Empty means no change.
	RoleTemplate string
	// ScopedPermissions replaces the current allowlist. Nil means no change;
	// an explicitly empty non-nil slice clears it (for custom templates).
	ScopedPermissions *[]string
	// MarkupPercent replaces the current markup. Nil means no change.
	MarkupPercent *float64
}

// ApproveSpec is the input to Service.Approve.  Only a customer admin with
// relationships.grant may call this; the relationship must be in
// pending_acceptance.
type ApproveSpec struct {
	CustomerTenantID string
	IntegratorID     string
}

// RevokeSpec is the input to Service.Revoke.
type RevokeSpec struct {
	CustomerTenantID string
	IntegratorID     string
}

// Caller identifies the authenticated actor making the request. Derive it
// exclusively from verified auth.Claims — never from request body fields.
type Caller struct {
	UserID auth.UserID
	// Tenant is the caller's own tenant. For customer-initiated flows this is
	// the customer tenant. For integrator-initiated flows this is the
	// integrator's tenant.
	Tenant auth.TenantRef
}

// Validate checks structural completeness.
func (c Caller) Validate() error {
	if c.UserID == "" {
		return errors.New("relationships: caller user id is required")
	}
	if c.Tenant.IsZero() {
		return errors.New("relationships: caller tenant is required")
	}
	return nil
}

// HierarchyNode is a node in the integrator sub-reseller tree.
type HierarchyNode struct {
	IntegratorID       string
	ParentIntegratorID *string
	// Children holds the direct children of this node. Empty for leaf nodes.
	Children []*HierarchyNode
	// EffectiveActions is the intersected action set at this level (KAI-229).
	// nil means "all actions from the parent are inherited".
	EffectiveActions []string
}

// visibleCustomersResult is the result of ListVisibleCustomers.
type visibleCustomersResult struct {
	CustomerTenantID string
	// IntegratorID is the direct owner of the relationship (may be a
	// sub-reseller, not the root).
	IntegratorID string
	Status       string
}

// ---------- JSON helpers for scoped_permissions column ----------

// encodeScopedPermissions serialises a []string to a compact JSON array.
// An empty or nil slice serialises to "[]", not "null".
func encodeScopedPermissions(actions []string) (string, error) {
	if actions == nil {
		actions = []string{}
	}
	b, err := json.Marshal(actions)
	if err != nil {
		return "", fmt.Errorf("relationships: encode permissions: %w", err)
	}
	return string(b), nil
}

// decodeScopedPermissions parses the raw JSON column back to []string.
func decodeScopedPermissions(raw string) ([]string, error) {
	if raw == "" || raw == "{}" || raw == "null" {
		return nil, nil
	}
	var out []string
	if err := json.Unmarshal([]byte(raw), &out); err != nil {
		return nil, fmt.Errorf("relationships: decode permissions %q: %w", raw, err)
	}
	return out, nil
}

// nullableTime is a tiny sql.Scanner adapter that converts a *time.Time to/from
// sql.NullTime, so the service layer can work with idiomatic Go pointers.
type nullableTime struct{ v *time.Time }

func (n *nullableTime) Scan(src interface{}) error {
	var nt sql.NullTime
	if err := nt.Scan(src); err != nil {
		return err
	}
	if nt.Valid {
		t := nt.Time
		n.v = &t
	}
	return nil
}
