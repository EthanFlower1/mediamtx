package auth

import "context"

// IdentityProvider is the multi-tenant identity firewall (architectural
// seam #3). Every identity operation in the cloud control plane and the
// on-prem Directory MUST flow through this interface.
//
// See doc.go for the fail-closed-for-security policy that all
// implementations are required to honor.
type IdentityProvider interface {
	// --- Authentication & sessions -------------------------------------

	// AuthenticateLocal verifies a username/password against the tenant's
	// local user database and returns a fresh Session on success. On any
	// failure (unknown user, wrong password, disabled account) it MUST
	// return ErrInvalidCredentials with a nil Session, with no information
	// leak about which check failed.
	AuthenticateLocal(ctx context.Context, tenant TenantRef, username, password string) (*Session, error)

	// BeginSSOFlow starts an SSO authentication flow against the named
	// external provider for the tenant. The returned SSOBegin contains
	// the URL to redirect the user agent to and the opaque `state` value
	// the implementation will validate during CompleteSSOFlow.
	//
	// redirectURI is the URL the IdP will call back; it MUST match the
	// provider's allow-list configured via ConfigureProvider.
	BeginSSOFlow(ctx context.Context, tenant TenantRef, providerID ProviderID, redirectURI string) (*SSOBegin, error)

	// CompleteSSOFlow exchanges an OIDC/SAML callback (state + code) for
	// a Session. ErrSSOStateInvalid is returned for unknown/expired/replayed
	// state values; this is the canonical CSRF defense for the SSO flow.
	CompleteSSOFlow(ctx context.Context, tenant TenantRef, state, code string) (*Session, error)

	// RefreshSession exchanges a still-valid refresh token for a new
	// Session. Implementations SHOULD rotate the refresh token on every
	// call (refresh-token rotation). ErrSessionNotFound is returned for
	// unknown or already-revoked tokens.
	RefreshSession(ctx context.Context, refreshToken string) (*Session, error)

	// VerifyToken parses, validates, and returns the Claims for an access
	// token (typically a JWT). Any verification failure — bad signature,
	// expired, missing tenant claim, audience mismatch, clock skew —
	// returns ErrTokenInvalid with nil Claims. Implementations MUST NOT
	// reveal which check failed.
	VerifyToken(ctx context.Context, token string) (*Claims, error)

	// RevokeSession revokes a session by id. It is idempotent: revoking an
	// unknown or already-revoked session returns nil, not an error.
	RevokeSession(ctx context.Context, sessionID SessionID) error

	// --- User CRUD ------------------------------------------------------

	// ListUsers returns a tenant-scoped page of users.
	ListUsers(ctx context.Context, tenant TenantRef, opts ListOptions) ([]*User, error)

	// GetUser returns a single user. Returns ErrUserNotFound if the user
	// does not exist within the given tenant, and ErrTenantMismatch if
	// the user exists but in a different tenant.
	GetUser(ctx context.Context, tenant TenantRef, id UserID) (*User, error)

	// CreateUser creates a new local user from a UserSpec. Returns
	// ErrUserExists if username/email collides within the tenant.
	CreateUser(ctx context.Context, tenant TenantRef, spec UserSpec) (*User, error)

	// UpdateUser applies a sparse update. Nil fields in the update are
	// left untouched.
	UpdateUser(ctx context.Context, tenant TenantRef, id UserID, update UserUpdate) (*User, error)

	// DeleteUser removes a user. Implementations SHOULD also revoke any
	// active sessions for the deleted user.
	DeleteUser(ctx context.Context, tenant TenantRef, id UserID) error

	// --- Group CRUD & membership ---------------------------------------

	// ListGroups returns all groups in the tenant.
	ListGroups(ctx context.Context, tenant TenantRef) ([]*Group, error)

	// AddUserToGroup adds the user to the named group. Both must belong
	// to the same tenant; otherwise ErrTenantMismatch is returned.
	AddUserToGroup(ctx context.Context, tenant TenantRef, userID UserID, groupID GroupID) error

	// RemoveUserFromGroup is the inverse of AddUserToGroup. Removing a
	// user that is not a member is a no-op (returns nil).
	RemoveUserFromGroup(ctx context.Context, tenant TenantRef, userID UserID, groupID GroupID) error

	// --- Provider configuration ----------------------------------------

	// ConfigureProvider creates or updates an external IdP configuration
	// (OIDC, SAML, or LDAP) for the tenant. Per the fail-closed policy,
	// implementations MUST refuse to persist a configuration that has not
	// passed TestProvider; callers should call TestProvider first and
	// pass the same ProviderConfig here on success.
	ConfigureProvider(ctx context.Context, tenant TenantRef, cfg ProviderConfig) error

	// TestProvider performs a round-trip probe against the candidate
	// configuration without persisting anything. This is the "Test
	// Connection" step in the Sign-in Methods wizard, and it is a
	// first-class citizen of the interface — no provider should ever be
	// saved without it.
	TestProvider(ctx context.Context, tenant TenantRef, cfg ProviderConfig) (*TestResult, error)
}
