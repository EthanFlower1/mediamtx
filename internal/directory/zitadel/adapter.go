// Package zitadel provides the IdentityProvider adapter that delegates all
// authentication, user/group CRUD, and SSO operations to a Zitadel instance
// (local sidecar or remote). This is architectural seam #3 — the Directory
// imports only the shared/auth interface; the Zitadel specifics stay here.
package zitadel

import (
	"context"
	"fmt"
	"time"

	"github.com/bluenviron/mediamtx/internal/shared/auth"
)

// Client abstracts the Zitadel gRPC/HTTP API. In production this wraps
// the generated gRPC stubs from zitadel/zitadel-go. In tests it's a fake.
type Client interface {
	// --- Auth ---
	VerifyPassword(ctx context.Context, orgID, username, password string) (userID, sessionToken string, err error)
	CreateSession(ctx context.Context, userID string) (session *auth.Session, err error)
	RefreshSession(ctx context.Context, refreshToken string) (*auth.Session, error)
	IntrospectToken(ctx context.Context, token string) (*auth.Claims, error)
	RevokeSession(ctx context.Context, sessionID string) error

	// --- SSO ---
	BeginAuthFlow(ctx context.Context, orgID string, providerID, redirectURI string) (authURL, state string, err error)
	ExchangeCode(ctx context.Context, orgID, state, code string) (*auth.Session, error)

	// --- User CRUD ---
	ListUsers(ctx context.Context, orgID string, search string, pageSize int, cursor string) ([]*auth.User, error)
	GetUser(ctx context.Context, orgID, userID string) (*auth.User, error)
	CreateUser(ctx context.Context, orgID string, spec auth.UserSpec) (*auth.User, error)
	UpdateUser(ctx context.Context, orgID, userID string, update auth.UserUpdate) (*auth.User, error)
	DeleteUser(ctx context.Context, orgID, userID string) error

	// --- Groups ---
	ListGroups(ctx context.Context, orgID string) ([]*auth.Group, error)
	AddUserToGroup(ctx context.Context, orgID, userID, groupID string) error
	RemoveUserFromGroup(ctx context.Context, orgID, userID, groupID string) error

	// --- Provider config ---
	ConfigureProvider(ctx context.Context, orgID string, cfg auth.ProviderConfig) error
	TestProvider(ctx context.Context, orgID string, cfg auth.ProviderConfig) (*auth.TestResult, error)
}

// OrgResolver maps a TenantRef to a Zitadel organization ID. On-prem
// installations typically have a single org; multi-tenant cloud deployments
// may have one org per customer tenant.
type OrgResolver interface {
	ResolveOrg(ctx context.Context, tenant auth.TenantRef) (orgID string, err error)
}

// Adapter implements auth.IdentityProvider by delegating to a Zitadel Client.
type Adapter struct {
	client Client
	orgs   OrgResolver
}

// NewAdapter creates a Zitadel IdentityProvider adapter.
func NewAdapter(client Client, orgs OrgResolver) (*Adapter, error) {
	if client == nil {
		return nil, fmt.Errorf("zitadel/adapter: Client is required")
	}
	if orgs == nil {
		return nil, fmt.Errorf("zitadel/adapter: OrgResolver is required")
	}
	return &Adapter{client: client, orgs: orgs}, nil
}

// --- Authentication & sessions ---

func (a *Adapter) AuthenticateLocal(ctx context.Context, tenant auth.TenantRef, username, password string) (*auth.Session, error) {
	orgID, err := a.orgs.ResolveOrg(ctx, tenant)
	if err != nil {
		return nil, auth.ErrInvalidCredentials
	}

	_, sessionToken, err := a.client.VerifyPassword(ctx, orgID, username, password)
	if err != nil || sessionToken == "" {
		return nil, auth.ErrInvalidCredentials
	}

	sess, err := a.client.CreateSession(ctx, sessionToken)
	if err != nil {
		return nil, auth.ErrInvalidCredentials
	}
	return sess, nil
}

func (a *Adapter) BeginSSOFlow(ctx context.Context, tenant auth.TenantRef, providerID auth.ProviderID, redirectURI string) (*auth.SSOBegin, error) {
	orgID, err := a.orgs.ResolveOrg(ctx, tenant)
	if err != nil {
		return nil, fmt.Errorf("zitadel: resolve org: %w", err)
	}

	authURL, state, err := a.client.BeginAuthFlow(ctx, orgID, string(providerID), redirectURI)
	if err != nil {
		return nil, fmt.Errorf("zitadel: begin SSO: %w", err)
	}

	return &auth.SSOBegin{AuthURL: authURL, State: state}, nil
}

func (a *Adapter) CompleteSSOFlow(ctx context.Context, tenant auth.TenantRef, state, code string) (*auth.Session, error) {
	orgID, err := a.orgs.ResolveOrg(ctx, tenant)
	if err != nil {
		return nil, auth.ErrSSOStateInvalid
	}

	sess, err := a.client.ExchangeCode(ctx, orgID, state, code)
	if err != nil {
		return nil, auth.ErrSSOStateInvalid
	}
	return sess, nil
}

func (a *Adapter) RefreshSession(ctx context.Context, refreshToken string) (*auth.Session, error) {
	sess, err := a.client.RefreshSession(ctx, refreshToken)
	if err != nil {
		return nil, auth.ErrSessionNotFound
	}
	return sess, nil
}

func (a *Adapter) VerifyToken(ctx context.Context, token string) (*auth.Claims, error) {
	claims, err := a.client.IntrospectToken(ctx, token)
	if err != nil || claims == nil {
		return nil, auth.ErrTokenInvalid
	}
	return claims, nil
}

func (a *Adapter) RevokeSession(ctx context.Context, sessionID auth.SessionID) error {
	return a.client.RevokeSession(ctx, string(sessionID))
}

// --- User CRUD ---

func (a *Adapter) ListUsers(ctx context.Context, tenant auth.TenantRef, opts auth.ListOptions) ([]*auth.User, error) {
	orgID, err := a.orgs.ResolveOrg(ctx, tenant)
	if err != nil {
		return nil, fmt.Errorf("zitadel: resolve org: %w", err)
	}
	return a.client.ListUsers(ctx, orgID, opts.Search, opts.PageSize, opts.Cursor)
}

func (a *Adapter) GetUser(ctx context.Context, tenant auth.TenantRef, id auth.UserID) (*auth.User, error) {
	orgID, err := a.orgs.ResolveOrg(ctx, tenant)
	if err != nil {
		return nil, auth.ErrUserNotFound
	}
	user, err := a.client.GetUser(ctx, orgID, string(id))
	if err != nil {
		return nil, auth.ErrUserNotFound
	}
	if !user.Tenant.Equal(tenant) {
		return nil, auth.ErrTenantMismatch
	}
	return user, nil
}

func (a *Adapter) CreateUser(ctx context.Context, tenant auth.TenantRef, spec auth.UserSpec) (*auth.User, error) {
	orgID, err := a.orgs.ResolveOrg(ctx, tenant)
	if err != nil {
		return nil, fmt.Errorf("zitadel: resolve org: %w", err)
	}
	user, err := a.client.CreateUser(ctx, orgID, spec)
	if err != nil {
		return nil, auth.ErrUserExists
	}
	return user, nil
}

func (a *Adapter) UpdateUser(ctx context.Context, tenant auth.TenantRef, id auth.UserID, update auth.UserUpdate) (*auth.User, error) {
	orgID, err := a.orgs.ResolveOrg(ctx, tenant)
	if err != nil {
		return nil, auth.ErrUserNotFound
	}
	user, err := a.client.UpdateUser(ctx, orgID, string(id), update)
	if err != nil {
		return nil, auth.ErrUserNotFound
	}
	return user, nil
}

func (a *Adapter) DeleteUser(ctx context.Context, tenant auth.TenantRef, id auth.UserID) error {
	orgID, err := a.orgs.ResolveOrg(ctx, tenant)
	if err != nil {
		return auth.ErrUserNotFound
	}
	return a.client.DeleteUser(ctx, orgID, string(id))
}

// --- Group CRUD ---

func (a *Adapter) ListGroups(ctx context.Context, tenant auth.TenantRef) ([]*auth.Group, error) {
	orgID, err := a.orgs.ResolveOrg(ctx, tenant)
	if err != nil {
		return nil, fmt.Errorf("zitadel: resolve org: %w", err)
	}
	return a.client.ListGroups(ctx, orgID)
}

func (a *Adapter) AddUserToGroup(ctx context.Context, tenant auth.TenantRef, userID auth.UserID, groupID auth.GroupID) error {
	orgID, err := a.orgs.ResolveOrg(ctx, tenant)
	if err != nil {
		return auth.ErrTenantMismatch
	}
	return a.client.AddUserToGroup(ctx, orgID, string(userID), string(groupID))
}

func (a *Adapter) RemoveUserFromGroup(ctx context.Context, tenant auth.TenantRef, userID auth.UserID, groupID auth.GroupID) error {
	orgID, err := a.orgs.ResolveOrg(ctx, tenant)
	if err != nil {
		return auth.ErrTenantMismatch
	}
	return a.client.RemoveUserFromGroup(ctx, orgID, string(userID), string(groupID))
}

// --- Provider configuration ---

func (a *Adapter) ConfigureProvider(ctx context.Context, tenant auth.TenantRef, cfg auth.ProviderConfig) error {
	orgID, err := a.orgs.ResolveOrg(ctx, tenant)
	if err != nil {
		return fmt.Errorf("zitadel: resolve org: %w", err)
	}
	return a.client.ConfigureProvider(ctx, orgID, cfg)
}

func (a *Adapter) TestProvider(ctx context.Context, tenant auth.TenantRef, cfg auth.ProviderConfig) (*auth.TestResult, error) {
	orgID, err := a.orgs.ResolveOrg(ctx, tenant)
	if err != nil {
		return nil, fmt.Errorf("zitadel: resolve org: %w", err)
	}
	return a.client.TestProvider(ctx, orgID, cfg)
}

// compile-time assertion
var _ auth.IdentityProvider = (*Adapter)(nil)

// StaticOrgResolver maps all tenants to a single org ID. Suitable for
// single-tenant on-prem deployments.
type StaticOrgResolver struct {
	OrgID string
}

// ResolveOrg returns the static org ID.
func (s *StaticOrgResolver) ResolveOrg(_ context.Context, _ auth.TenantRef) (string, error) {
	if s.OrgID == "" {
		return "", fmt.Errorf("zitadel: no org configured")
	}
	return s.OrgID, nil
}

// Now returns a time.Now helper (used for testing seams).
func Now() time.Time { return time.Now() }
