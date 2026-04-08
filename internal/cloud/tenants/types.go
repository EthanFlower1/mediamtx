package tenants

import (
	"errors"
	"time"

	clouddb "github.com/bluenviron/mediamtx/internal/cloud/db"
	"github.com/bluenviron/mediamtx/internal/shared/auth"
)

// MaxSubResellerDepth is the hard cap on the integrator chain depth (inclusive
// of the root). See README.md for rationale.
const MaxSubResellerDepth = 3

// BillingModeDirect is the platform-bills-customer mode; KAI-227 hard-codes
// this for every customer tenant. KAI-361 (Stripe) wires the real flow.
const BillingModeDirect = "direct"

// BillingModeViaIntegrator is the markup-via-reseller mode. KAI-227 only
// validates the invariant that a via_integrator tenant names a home
// integrator; the actual billing logic is KAI-362.
const BillingModeViaIntegrator = "via_integrator"

// CreateIntegratorSpec is the input to CreateIntegrator / CreateSubReseller.
// All fields except DisplayName are optional; Region defaults to
// clouddb.DefaultRegion. Provisioning is idempotent on ID: callers that want
// a specific UUID may set it (tests do), otherwise the service generates one.
type CreateIntegratorSpec struct {
	ID           string
	DisplayName  string
	LegalName    string
	ContactEmail string
	Region       string
	// InitialAdminEmail, if non-empty, seeds the first admin user and
	// returns the Invitation as part of the Integrator result.
	InitialAdminEmail string
}

// CreateCustomerTenantSpec is the input to CreateCustomerTenant.
type CreateCustomerTenantSpec struct {
	ID                string
	DisplayName       string
	BillingMode       string // "direct" (default) or "via_integrator"
	HomeIntegratorID  string // required iff BillingMode == "via_integrator"
	SignupSource      string
	Region            string
	InitialAdminEmail string
}

// Integrator is the provisioning result for CreateIntegrator /
// CreateSubReseller. It bundles the DB row with the optional initial-admin
// invitation so callers that supplied InitialAdminEmail don't need a second
// round-trip.
type Integrator struct {
	Row           clouddb.Integrator
	ZitadelOrgID  string
	InitialAdmin  *Invitation
}

// CustomerTenant is the provisioning result for CreateCustomerTenant.
type CustomerTenant struct {
	Row           clouddb.CustomerTenant
	ZitadelOrgID  string
	InitialAdmin  *Invitation
}

// Invitation is the result of InviteInitialAdmin — a newly created user plus
// the metadata needed to send an invite email. The actual send is handled by
// the River job enqueued in step 5.
type Invitation struct {
	UserRow       clouddb.User
	ZitadelUserID string
	Email         string
	InviteToken   string
	ExpiresAt     time.Time
}

// Caller identifies the authenticated principal making the provisioning
// request. The service uses it for permission checks, audit actor, and the
// parent-reseller link in sub-reseller creation.
//
// Per the architectural ground rule, callers MUST derive this from a
// verified auth.Claims — never from a request body.
type Caller struct {
	UserID auth.UserID
	// Tenant is the caller's own tenant (integrator or platform). For
	// platform staff creating a fresh root integrator, Type is
	// TenantIntegrator with a well-known "platform" id.
	Tenant auth.TenantRef
	// IsPlatformStaff marks the caller as a platform operator (the only
	// principal that can create root integrators). KAI-224's scoped-token
	// flow will set this from a signed token claim.
	IsPlatformStaff bool
}

// Validate reports whether the Caller is structurally well-formed.
func (c Caller) Validate() error {
	if c.UserID == "" {
		return errors.New("tenants: caller user id is required")
	}
	if c.Tenant.IsZero() {
		return errors.New("tenants: caller tenant is required")
	}
	return nil
}

// TenantRef re-exports the shared auth TenantRef so API handlers don't have
// to import two packages when they pass around a tenant pointer.
type TenantRef = auth.TenantRef
