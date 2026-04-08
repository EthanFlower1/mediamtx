// Package crosstenant implements the cross-tenant access service (KAI-224).
//
// When an integrator-staff user accesses a managed customer tenant, the cloud
// control plane mints a short-lived "scoped token" that carries:
//
//   - the integrator user's identity (sub = "integrator:<uid>@<customer_tenant>")
//   - the target customer tenant id
//   - the intersected permission scope derived from the customer_integrator_relationships
//     table and any sub-reseller parent chain (KAI-225 ResolveIntegratorScope)
//   - a short TTL (default 15 minutes)
//   - a revocable session id
//
// Every Mint/Verify call emits an audit.Entry through the injected
// audit.Recorder. This is the only way integrator-staff cross-tenant activity
// is recorded for SOC 2 / HIPAA purposes.
//
// The service intentionally does NOT import the Zitadel adapter (KAI-223) or
// the API server (KAI-226). It speaks to Wave 1 artifacts only via:
//
//   - auth.IdentityProvider         — verifies the integrator user exists
//   - permissions.IntegratorRelationshipStore — walks the sub-reseller chain
//   - audit.Recorder                 — emits cross-tenant audit entries
//   - ScopedSessionStore             — tracks revocable sessions
//
// See README.md for the full flow + fail-closed policy.
package crosstenant

import (
	"errors"
	"time"

	"github.com/bluenviron/mediamtx/internal/shared/auth"
)

// ScopedToken is the opaque result of MintScopedToken. Token is a signed JWT;
// the other fields are metadata the caller needs to surface to the UI / HTTP
// layer (KAI-226).
type ScopedToken struct {
	// Token is the signed JWT (HS256) that the client will present back on
	// every cross-tenant request.
	Token string

	// SessionID is the revocable session id. Store it server-side so the
	// integrator can terminate an active impersonation.
	SessionID string

	// ExpiresAt is the absolute expiry time of the token (UTC).
	ExpiresAt time.Time

	// CustomerTenantID is the tenant whose data the token authorizes
	// (NOT the integrator's own tenant).
	CustomerTenantID string

	// PermissionScope is the intersected action set after walking the
	// sub-reseller parent chain. Order is deterministic (sorted).
	PermissionScope []string
}

// ScopedClaims is the verified, cross-tenant claim set returned by
// VerifyScopedToken. This is the ONLY struct callers should consult when
// authorizing a cross-tenant request — the raw JWT claims never escape the
// package.
type ScopedClaims struct {
	// Subject is the canonical "integrator:<uid>@<customer_tenant>" string
	// used by the Casbin enforcer (KAI-225 SubjectKindIntegrator).
	Subject string

	// IntegratorUserID is the integrator-staff user id.
	IntegratorUserID auth.UserID

	// IntegratorTenant is the integrator's own tenant (audit / logging).
	IntegratorTenant auth.TenantRef

	// CustomerTenant is the customer tenant the token is scoped to.
	CustomerTenant auth.TenantRef

	// PermissionScope is the intersected allow-list of actions.
	PermissionScope []string

	// SessionID is the revocable session id.
	SessionID string

	// IssuedAt / ExpiresAt are the JWT time bounds.
	IssuedAt  time.Time
	ExpiresAt time.Time
}

// Sentinel errors. Callers switch on errors.Is to react to these.
var (
	// ErrNoRelationship is returned when no customer_integrator_relationships
	// row exists for (integrator user, customer tenant). Fail-closed.
	ErrNoRelationship = errors.New("crosstenant: no integrator relationship")

	// ErrRelationshipRevoked is returned when the relationship exists but
	// has been revoked/disabled.
	ErrRelationshipRevoked = errors.New("crosstenant: relationship revoked")

	// ErrScopedTokenExpired is returned by VerifyScopedToken when the JWT
	// exp is in the past.
	ErrScopedTokenExpired = errors.New("crosstenant: scoped token expired")

	// ErrScopedTokenInvalid is the generic "can't trust this token" error
	// (bad signature, malformed, wrong type). Implementations MUST NOT
	// reveal which check failed.
	ErrScopedTokenInvalid = errors.New("crosstenant: scoped token invalid")

	// ErrSessionRevoked is returned by VerifyScopedToken when the session
	// id is marked revoked in the ScopedSessionStore.
	ErrSessionRevoked = errors.New("crosstenant: scoped session revoked")

	// ErrHierarchyTooDeep is returned when the sub-reseller parent chain
	// exceeds maxHierarchyDepth (32).
	ErrHierarchyTooDeep = errors.New("crosstenant: sub-reseller hierarchy too deep")

	// ErrUnknownIntegrator is returned when the injected IdentityProvider
	// cannot find the integrator user within the integrator tenant.
	ErrUnknownIntegrator = errors.New("crosstenant: unknown integrator user")

	// ErrEmptyScope is returned when the resolved intersection is empty.
	// Fail-closed: we refuse to mint a token that would authorize nothing.
	ErrEmptyScope = errors.New("crosstenant: resolved permission scope is empty")
)

// maxHierarchyDepth bounds the parent_integrator walk (matches KAI-225).
const maxHierarchyDepth = 32
