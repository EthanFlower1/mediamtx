package impersonation

import (
	"errors"
	"time"
)

// ImpersonationMode distinguishes the two impersonation flows.
type ImpersonationMode string

const (
	// ModeIntegrator is used when integrator staff impersonate a managed
	// customer. The scope is derived from the customer-integrator relationship.
	ModeIntegrator ImpersonationMode = "integrator"

	// ModePlatformSupport is used when platform support staff impersonate
	// any tenant with explicit customer authorization.
	ModePlatformSupport ImpersonationMode = "platform_support"
)

// Valid returns true if the mode is well-known.
func (m ImpersonationMode) Valid() bool {
	return m == ModeIntegrator || m == ModePlatformSupport
}

// SessionStatus tracks the lifecycle of an impersonation session.
type SessionStatus string

const (
	StatusActive     SessionStatus = "active"
	StatusExpired    SessionStatus = "expired"
	StatusTerminated SessionStatus = "terminated"
)

// Session represents an active or completed impersonation session.
type Session struct {
	// SessionID is the unique identifier for this impersonation session.
	SessionID string

	// Mode identifies whether this is an integrator or platform support session.
	Mode ImpersonationMode

	// ImpersonatingUserID is the user performing the impersonation.
	ImpersonatingUserID string

	// ImpersonatingTenantID is the tenant the impersonator belongs to.
	ImpersonatingTenantID string

	// ImpersonatedTenantID is the customer tenant being impersonated.
	ImpersonatedTenantID string

	// ScopedPermissions is the set of actions allowed during the session.
	// This is always a subset of the customer's permissions, with admin
	// actions stripped.
	ScopedPermissions []string

	// AuthorizationToken is the customer-issued authorization token
	// (platform support mode only). Empty for integrator mode.
	AuthorizationToken string

	// Status is the current lifecycle state.
	Status SessionStatus

	// CreatedAt is when the session was created.
	CreatedAt time.Time

	// ExpiresAt is the absolute expiry time.
	ExpiresAt time.Time

	// TerminatedAt is when the session was explicitly terminated, or nil.
	TerminatedAt *time.Time

	// TerminatedBy is who terminated the session (user id), or empty.
	TerminatedBy string
}

// IsExpired reports whether the session has passed its expiry time.
func (s Session) IsExpired(now time.Time) bool {
	return now.After(s.ExpiresAt)
}

// IsActive reports whether the session is currently active and not expired.
func (s Session) IsActive(now time.Time) bool {
	return s.Status == StatusActive && !s.IsExpired(now)
}

// AuthorizationGrant is the token a customer admin creates to authorize
// platform support to impersonate their tenant.
type AuthorizationGrant struct {
	// GrantID is the unique identifier for this grant.
	GrantID string

	// TenantID is the customer tenant granting authorization.
	TenantID string

	// GrantedByUserID is the customer admin who created the grant.
	GrantedByUserID string

	// Reason is the customer-provided reason for the grant (e.g., ticket ID).
	Reason string

	// MaxDuration is the maximum session duration the customer authorized.
	MaxDuration time.Duration

	// CreatedAt is when the grant was created.
	CreatedAt time.Time

	// ExpiresAt is when the grant itself expires (not the session).
	ExpiresAt time.Time

	// Consumed indicates whether the grant has been used to create a session.
	Consumed bool

	// ConsumedAt is when the grant was consumed, or nil.
	ConsumedAt *time.Time

	// ConsumedBySessionID is the session that consumed this grant, or empty.
	ConsumedBySessionID string

	// Revoked indicates the customer admin revoked the grant before use.
	Revoked bool
}

// IsValid reports whether the grant can be consumed right now.
func (g AuthorizationGrant) IsValid(now time.Time) bool {
	return !g.Consumed && !g.Revoked && now.Before(g.ExpiresAt)
}

// CreateSessionRequest is the input to CreateSession.
type CreateSessionRequest struct {
	// Mode is required.
	Mode ImpersonationMode

	// ImpersonatingUserID is the user performing the impersonation.
	ImpersonatingUserID string

	// ImpersonatingTenantID is the impersonator's own tenant.
	ImpersonatingTenantID string

	// ImpersonatedTenantID is the target customer tenant.
	ImpersonatedTenantID string

	// AuthorizationGrantID is required for ModePlatformSupport.
	// It references a valid AuthorizationGrant created by the customer admin.
	AuthorizationGrantID string

	// Timeout overrides the default session timeout if positive.
	Timeout time.Duration
}

// CreateGrantRequest is the input to CreateAuthorizationGrant.
type CreateGrantRequest struct {
	// TenantID is the customer tenant.
	TenantID string

	// GrantedByUserID is the customer admin creating the grant.
	GrantedByUserID string

	// Reason is why the grant is being created (e.g., support ticket ID).
	Reason string

	// MaxDuration limits the impersonation session duration. Zero means
	// use DefaultTimeout.
	MaxDuration time.Duration

	// GrantTTL is how long the grant itself is valid before being consumed.
	// Zero means use DefaultGrantTTL.
	GrantTTL time.Duration
}

// Sentinel errors. Callers switch on errors.Is.
var (
	// ErrUnauthorized is returned when impersonation is attempted without
	// proper authorization.
	ErrUnauthorized = errors.New("impersonation: unauthorized")

	// ErrSessionNotFound is returned when a session ID does not resolve.
	ErrSessionNotFound = errors.New("impersonation: session not found")

	// ErrSessionExpired is returned when a session has passed its expiry.
	ErrSessionExpired = errors.New("impersonation: session expired")

	// ErrSessionAlreadyTerminated is returned when terminating an already
	// terminated or expired session.
	ErrSessionAlreadyTerminated = errors.New("impersonation: session already terminated")

	// ErrGrantNotFound is returned when an authorization grant ID does not resolve.
	ErrGrantNotFound = errors.New("impersonation: grant not found")

	// ErrGrantExpired is returned when an authorization grant has expired.
	ErrGrantExpired = errors.New("impersonation: grant expired")

	// ErrGrantConsumed is returned when an authorization grant has already been used.
	ErrGrantConsumed = errors.New("impersonation: grant already consumed")

	// ErrGrantRevoked is returned when an authorization grant has been revoked.
	ErrGrantRevoked = errors.New("impersonation: grant revoked")

	// ErrNoRelationship is returned when integrator mode is requested but
	// no customer-integrator relationship exists.
	ErrNoRelationship = errors.New("impersonation: no integrator relationship")

	// ErrRelationshipRevoked is returned when the relationship exists but
	// has been revoked.
	ErrRelationshipRevoked = errors.New("impersonation: relationship revoked")

	// ErrInvalidMode is returned for an unknown ImpersonationMode.
	ErrInvalidMode = errors.New("impersonation: invalid mode")

	// ErrMissingGrant is returned when platform support mode is requested
	// without an authorization grant ID.
	ErrMissingGrant = errors.New("impersonation: authorization grant required for platform support mode")

	// ErrAdminActionBlocked is returned when an impersonation session
	// attempts an admin-level action.
	ErrAdminActionBlocked = errors.New("impersonation: admin actions are blocked during impersonation")
)
