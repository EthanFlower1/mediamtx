package auth

import (
	"errors"
	"time"
)

// TenantType distinguishes the two kinds of tenants in the multi-tenant
// control plane. Every identity record is owned by exactly one of these.
type TenantType string

const (
	// TenantTypeIntegrator is a reseller / MSP tenant. Integrator users may
	// be granted scoped cross-tenant access to their downstream customers
	// via integrator-relationship records.
	TenantTypeIntegrator TenantType = "integrator"

	// TenantTypeCustomer is an end-customer tenant. Customer users only
	// see their own tenant unless an integrator scoped token says otherwise.
	TenantTypeCustomer TenantType = "customer"
)

// TenantRef identifies a tenant unambiguously. It is the first argument
// (after ctx) of nearly every IdentityProvider method.
//
// IMPORTANT: Callers MUST derive TenantRef from the authenticated session,
// never from a request body. Trusting a client-supplied tenant id is the
// canonical multi-tenant isolation bug.
type TenantRef struct {
	Type TenantType
	ID   string
}

// IsZero reports whether the TenantRef is the zero value.
func (t TenantRef) IsZero() bool { return t.Type == "" && t.ID == "" }

// Equal reports whether two TenantRefs refer to the same tenant.
func (t TenantRef) Equal(o TenantRef) bool { return t.Type == o.Type && t.ID == o.ID }

// Strongly-typed identifiers. Strings under the hood, but the named types
// prevent accidental cross-assignment (e.g. passing a GroupID where a
// UserID was expected).
type (
	UserID                      string
	GroupID                     string
	ProviderID                  string
	SessionID                   string
	RecorderID                  string
	IntegratorRelationshipRef   string
)

// Session is the result of any successful authentication or refresh. It is
// what the caller hands back to the user agent (typically as cookies or
// bearer tokens).
type Session struct {
	ID           SessionID
	UserID       UserID
	Tenant       TenantRef
	AccessToken  string
	RefreshToken string
	IDToken      string // optional; populated for OIDC flows
	IssuedAt     time.Time
	ExpiresAt    time.Time
}

// Claims is the verified, tenant-scoped result of VerifyToken. It is the
// only struct callers should consult when making authorization decisions —
// raw JWT claims must never escape the auth package.
type Claims struct {
	UserID                  UserID
	TenantRef               TenantRef
	Groups                  []GroupID
	IssuedAt                time.Time
	ExpiresAt               time.Time
	NotBefore               time.Time
	SessionID               SessionID
	SiteScope               []RecorderID
	IntegratorRelationships []IntegratorRelationshipRef
}

// TokenClaims is an alias kept for callers that prefer the longer name.
// The canonical type is Claims; both refer to the same struct.
type TokenClaims = Claims

// User is a tenant-scoped identity record.
type User struct {
	ID            UserID
	Tenant        TenantRef
	Username      string
	Email         string
	DisplayName   string
	Groups        []GroupID
	ExternalIDs   map[ProviderID]string // populated for SSO-linked accounts
	Disabled      bool
	CreatedAt     time.Time
	UpdatedAt     time.Time
	LastLoginAt   time.Time
}

// UserSpec is the input to CreateUser.
type UserSpec struct {
	Username    string
	Email       string
	DisplayName string
	Password    string // optional; if empty the user must use SSO
	Groups      []GroupID
	Disabled    bool
}

// UserUpdate is a sparse update; nil fields are left untouched.
type UserUpdate struct {
	Email       *string
	DisplayName *string
	Password    *string
	Disabled    *bool
}

// Group is a tenant-scoped collection of users used by Casbin policy.
type Group struct {
	ID          GroupID
	Tenant      TenantRef
	Name        string
	Description string
	CreatedAt   time.Time
	UpdatedAt   time.Time
}

// ListOptions is a generic pagination/filter struct for list endpoints.
type ListOptions struct {
	Search   string
	PageSize int
	Cursor   string
}

// ProviderKind distinguishes the supported external IdP types.
type ProviderKind string

const (
	ProviderKindOIDC ProviderKind = "oidc"
	ProviderKindSAML ProviderKind = "saml"
	ProviderKindLDAP ProviderKind = "ldap"
)

// ProviderConfig is the union over all external IdP configurations. Exactly
// one of OIDC, SAML, or LDAP must be set, matching Kind.
type ProviderConfig struct {
	ID          ProviderID
	Tenant      TenantRef
	Kind        ProviderKind
	DisplayName string
	Enabled     bool
	OIDC        *OIDCConfig
	SAML        *SAMLConfig
	LDAP        *LDAPConfig
	CreatedAt   time.Time
	UpdatedAt   time.Time
}

// OIDCConfig holds the configuration for an OpenID Connect provider.
type OIDCConfig struct {
	IssuerURL    string
	ClientID     string
	ClientSecret string // never logged; redacted by String()
	Scopes       []string
	RedirectURI  string
}

// SAMLConfig holds the configuration for a SAML 2.0 provider.
type SAMLConfig struct {
	MetadataURL          string
	MetadataXML          string // alternative to MetadataURL
	EntityID             string
	AssertionConsumerURL string
	SigningCertPEM       string
}

// LDAPConfig holds the configuration for an LDAP / Active Directory bind.
type LDAPConfig struct {
	URL          string // ldaps://host:636
	BindDN       string
	BindPassword string // never logged; redacted by String()
	UserBaseDN   string
	UserFilter   string
	GroupBaseDN  string
	GroupFilter  string
	StartTLS     bool
}

// ProviderUpdate is a sparse update for ConfigureProvider.
type ProviderUpdate struct {
	DisplayName *string
	Enabled     *bool
	OIDC        *OIDCConfig
	SAML        *SAMLConfig
	LDAP        *LDAPConfig
}

// SSOBegin is the result of BeginSSOFlow. The caller redirects the user
// agent to AuthURL; State is stored server-side and validated in the
// matching CompleteSSOFlow call.
type SSOBegin struct {
	AuthURL string
	State   string
}

// TestResult is the outcome of TestProvider — the round-trip probe used by
// the "Sign-in Methods" wizard. Success=true means the provider was reachable,
// returned a valid metadata/discovery document, and (where applicable) a
// canary bind/credential exchange succeeded.
type TestResult struct {
	Success     bool
	LatencyMS   int64
	Message     string
	Diagnostics map[string]string
}

// ProviderTestResult is an alias kept for callers that prefer the longer
// name. The canonical type is TestResult.
type ProviderTestResult = TestResult

// Sentinel errors. Implementations MUST return one of these (possibly
// wrapped via fmt.Errorf("%w", ...)) for the listed conditions so that
// callers can switch on errors.Is.
var (
	// ErrInvalidCredentials is returned for any local-auth failure.
	// Implementations MUST NOT distinguish "unknown user" from "wrong
	// password" — both map to this error to prevent user enumeration.
	ErrInvalidCredentials = errors.New("auth: invalid credentials")

	// ErrUserNotFound is returned by GetUser/UpdateUser/DeleteUser when
	// the user id does not exist within the caller's tenant scope.
	ErrUserNotFound = errors.New("auth: user not found")

	// ErrUserExists is returned by CreateUser when the username or email
	// is already taken within the tenant.
	ErrUserExists = errors.New("auth: user already exists")

	// ErrGroupNotFound is returned when a GroupID does not resolve.
	ErrGroupNotFound = errors.New("auth: group not found")

	// ErrProviderNotFound is returned when a ProviderID does not resolve.
	ErrProviderNotFound = errors.New("auth: provider not found")

	// ErrSessionNotFound is returned by RefreshSession for unknown or
	// already-revoked refresh tokens. RevokeSession is idempotent and
	// does NOT return this error.
	ErrSessionNotFound = errors.New("auth: session not found")

	// ErrTokenInvalid is returned by VerifyToken for any verification
	// failure (bad signature, expired, missing tenant claim, etc.).
	// Implementations MUST NOT leak which check failed.
	ErrTokenInvalid = errors.New("auth: token invalid")

	// ErrTenantMismatch is returned when a caller attempts to operate on
	// a record that belongs to a different tenant than the caller's
	// authenticated session.
	ErrTenantMismatch = errors.New("auth: tenant mismatch")

	// ErrSSOStateInvalid is returned by CompleteSSOFlow for an unknown,
	// expired, or replayed state token.
	ErrSSOStateInvalid = errors.New("auth: sso state invalid")

	// ErrProviderTestFailed is returned by ConfigureProvider when the
	// caller skipped TestProvider or when the test probe failed. The
	// fail-closed-for-security policy forbids persisting an unverified
	// provider config.
	ErrProviderTestFailed = errors.New("auth: provider test failed")

	// ErrNotImplemented is returned by adapters that do not support a
	// particular method (e.g. an LDAP-only adapter rejecting BeginSSOFlow).
	ErrNotImplemented = errors.New("auth: not implemented")
)
