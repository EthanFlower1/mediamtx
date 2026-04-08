// Package fake provides an in-memory IdentityProvider implementation for
// tests. It is intentionally simple, deterministic, and contains NO Zitadel
// or other external IdP code — that is the entire point of the auth seam.
//
// It is NOT safe to use in production: passwords are stored in plaintext
// and tokens are opaque random strings rather than signed JWTs.
package fake

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/bluenviron/mediamtx/internal/shared/auth"
)

// Provider is an in-memory implementation of auth.IdentityProvider.
type Provider struct {
	mu sync.Mutex

	now func() time.Time

	users     map[string]*storedUser // key: tenantKey + "|" + userID
	groups    map[string]*auth.Group // key: tenantKey + "|" + groupID
	providers map[auth.ProviderID]*auth.ProviderConfig
	sessions  map[auth.SessionID]*storedSession
	byRefresh map[string]auth.SessionID
	byAccess  map[string]auth.SessionID
	ssoStates map[string]ssoState

	nextID uint64
}

type storedUser struct {
	user     auth.User
	password string
}

type storedSession struct {
	session auth.Session
	revoked bool
}

type ssoState struct {
	tenant     auth.TenantRef
	providerID auth.ProviderID
	expiresAt  time.Time
}

// New constructs an empty in-memory Provider.
func New() *Provider {
	return &Provider{
		now:       time.Now,
		users:     map[string]*storedUser{},
		groups:    map[string]*auth.Group{},
		providers: map[auth.ProviderID]*auth.ProviderConfig{},
		sessions:  map[auth.SessionID]*storedSession{},
		byRefresh: map[string]auth.SessionID{},
		byAccess:  map[string]auth.SessionID{},
		ssoStates: map[string]ssoState{},
	}
}

// Compile-time check that *Provider satisfies the interface.
var _ auth.IdentityProvider = (*Provider)(nil)

// --- helpers --------------------------------------------------------------

func tenantKey(t auth.TenantRef) string {
	return string(t.Type) + ":" + t.ID
}

func (p *Provider) genID(prefix string) string {
	p.nextID++
	return fmt.Sprintf("%s_%d", prefix, p.nextID)
}

func randToken() string {
	var b [24]byte
	_, _ = rand.Read(b[:])
	return hex.EncodeToString(b[:])
}

// --- Authentication & sessions -------------------------------------------

// AuthenticateLocal implements auth.IdentityProvider.
func (p *Provider) AuthenticateLocal(_ context.Context, tenant auth.TenantRef, username, password string) (*auth.Session, error) {
	p.mu.Lock()
	defer p.mu.Unlock()

	tk := tenantKey(tenant)
	for _, u := range p.users {
		if tenantKey(u.user.Tenant) != tk {
			continue
		}
		if u.user.Username != username {
			continue
		}
		if u.user.Disabled || u.password == "" || u.password != password {
			return nil, auth.ErrInvalidCredentials
		}
		return p.issueSessionLocked(u.user), nil
	}
	return nil, auth.ErrInvalidCredentials
}

func (p *Provider) issueSessionLocked(u auth.User) *auth.Session {
	now := p.now()
	s := auth.Session{
		ID:           auth.SessionID(p.genID("sess")),
		UserID:       u.ID,
		Tenant:       u.Tenant,
		AccessToken:  randToken(),
		RefreshToken: randToken(),
		IssuedAt:     now,
		ExpiresAt:    now.Add(time.Hour),
	}
	p.sessions[s.ID] = &storedSession{session: s}
	p.byRefresh[s.RefreshToken] = s.ID
	p.byAccess[s.AccessToken] = s.ID
	// update last login
	if su, ok := p.users[tenantKey(u.Tenant)+"|"+string(u.ID)]; ok {
		su.user.LastLoginAt = now
	}
	out := s
	return &out
}

// BeginSSOFlow implements auth.IdentityProvider.
func (p *Provider) BeginSSOFlow(_ context.Context, tenant auth.TenantRef, providerID auth.ProviderID, redirectURI string) (*auth.SSOBegin, error) {
	p.mu.Lock()
	defer p.mu.Unlock()

	cfg, ok := p.providers[providerID]
	if !ok || !cfg.Tenant.Equal(tenant) {
		return nil, auth.ErrProviderNotFound
	}
	if !cfg.Enabled {
		return nil, auth.ErrProviderTestFailed
	}
	state := randToken()
	p.ssoStates[state] = ssoState{
		tenant:     tenant,
		providerID: providerID,
		expiresAt:  p.now().Add(10 * time.Minute),
	}
	return &auth.SSOBegin{
		AuthURL: fmt.Sprintf("https://fake-idp.local/authorize?provider=%s&redirect=%s&state=%s", providerID, redirectURI, state),
		State:   state,
	}, nil
}

// CompleteSSOFlow implements auth.IdentityProvider.
func (p *Provider) CompleteSSOFlow(_ context.Context, tenant auth.TenantRef, state, code string) (*auth.Session, error) {
	p.mu.Lock()
	defer p.mu.Unlock()

	st, ok := p.ssoStates[state]
	if !ok || !st.tenant.Equal(tenant) || p.now().After(st.expiresAt) {
		return nil, auth.ErrSSOStateInvalid
	}
	delete(p.ssoStates, state) // single use
	if code == "" {
		return nil, auth.ErrSSOStateInvalid
	}
	// In the fake, the SSO `code` is interpreted as the username of an
	// already-provisioned user in the tenant — good enough for tests.
	tk := tenantKey(tenant)
	for _, u := range p.users {
		if tenantKey(u.user.Tenant) == tk && u.user.Username == code {
			return p.issueSessionLocked(u.user), nil
		}
	}
	return nil, auth.ErrSSOStateInvalid
}

// RefreshSession implements auth.IdentityProvider.
func (p *Provider) RefreshSession(_ context.Context, refreshToken string) (*auth.Session, error) {
	p.mu.Lock()
	defer p.mu.Unlock()

	sid, ok := p.byRefresh[refreshToken]
	if !ok {
		return nil, auth.ErrSessionNotFound
	}
	stored, ok := p.sessions[sid]
	if !ok || stored.revoked {
		return nil, auth.ErrSessionNotFound
	}
	// rotate
	delete(p.byRefresh, stored.session.RefreshToken)
	delete(p.byAccess, stored.session.AccessToken)
	now := p.now()
	stored.session.AccessToken = randToken()
	stored.session.RefreshToken = randToken()
	stored.session.IssuedAt = now
	stored.session.ExpiresAt = now.Add(time.Hour)
	p.byRefresh[stored.session.RefreshToken] = sid
	p.byAccess[stored.session.AccessToken] = sid
	out := stored.session
	return &out, nil
}

// VerifyToken implements auth.IdentityProvider.
func (p *Provider) VerifyToken(_ context.Context, token string) (*auth.Claims, error) {
	p.mu.Lock()
	defer p.mu.Unlock()

	sid, ok := p.byAccess[token]
	if !ok {
		return nil, auth.ErrTokenInvalid
	}
	stored, ok := p.sessions[sid]
	if !ok || stored.revoked || p.now().After(stored.session.ExpiresAt) {
		return nil, auth.ErrTokenInvalid
	}
	// Resolve groups for the user.
	var groups []auth.GroupID
	if su, ok := p.users[tenantKey(stored.session.Tenant)+"|"+string(stored.session.UserID)]; ok {
		groups = append(groups, su.user.Groups...)
	}
	return &auth.Claims{
		UserID:    stored.session.UserID,
		TenantRef: stored.session.Tenant,
		Groups:    groups,
		IssuedAt:  stored.session.IssuedAt,
		ExpiresAt: stored.session.ExpiresAt,
		SessionID: stored.session.ID,
	}, nil
}

// RevokeSession implements auth.IdentityProvider.
func (p *Provider) RevokeSession(_ context.Context, sessionID auth.SessionID) error {
	p.mu.Lock()
	defer p.mu.Unlock()

	stored, ok := p.sessions[sessionID]
	if !ok {
		return nil // idempotent
	}
	stored.revoked = true
	delete(p.byAccess, stored.session.AccessToken)
	delete(p.byRefresh, stored.session.RefreshToken)
	return nil
}

// --- User CRUD -----------------------------------------------------------

// ListUsers implements auth.IdentityProvider.
func (p *Provider) ListUsers(_ context.Context, tenant auth.TenantRef, opts auth.ListOptions) ([]*auth.User, error) {
	p.mu.Lock()
	defer p.mu.Unlock()

	tk := tenantKey(tenant)
	out := make([]*auth.User, 0)
	for _, u := range p.users {
		if tenantKey(u.user.Tenant) != tk {
			continue
		}
		if opts.Search != "" && !strings.Contains(u.user.Username, opts.Search) && !strings.Contains(u.user.Email, opts.Search) {
			continue
		}
		clone := u.user
		out = append(out, &clone)
	}
	return out, nil
}

// GetUser implements auth.IdentityProvider.
func (p *Provider) GetUser(_ context.Context, tenant auth.TenantRef, id auth.UserID) (*auth.User, error) {
	p.mu.Lock()
	defer p.mu.Unlock()

	if u, ok := p.users[tenantKey(tenant)+"|"+string(id)]; ok {
		clone := u.user
		return &clone, nil
	}
	// Look for cross-tenant existence to surface ErrTenantMismatch.
	for _, u := range p.users {
		if u.user.ID == id {
			return nil, auth.ErrTenantMismatch
		}
	}
	return nil, auth.ErrUserNotFound
}

// CreateUser implements auth.IdentityProvider.
func (p *Provider) CreateUser(_ context.Context, tenant auth.TenantRef, spec auth.UserSpec) (*auth.User, error) {
	p.mu.Lock()
	defer p.mu.Unlock()

	tk := tenantKey(tenant)
	for _, u := range p.users {
		if tenantKey(u.user.Tenant) != tk {
			continue
		}
		if u.user.Username == spec.Username || (spec.Email != "" && u.user.Email == spec.Email) {
			return nil, auth.ErrUserExists
		}
	}
	now := p.now()
	id := auth.UserID(p.genID("user"))
	u := auth.User{
		ID:          id,
		Tenant:      tenant,
		Username:    spec.Username,
		Email:       spec.Email,
		DisplayName: spec.DisplayName,
		Groups:      append([]auth.GroupID(nil), spec.Groups...),
		Disabled:    spec.Disabled,
		CreatedAt:   now,
		UpdatedAt:   now,
	}
	p.users[tk+"|"+string(id)] = &storedUser{user: u, password: spec.Password}
	clone := u
	return &clone, nil
}

// UpdateUser implements auth.IdentityProvider.
func (p *Provider) UpdateUser(_ context.Context, tenant auth.TenantRef, id auth.UserID, update auth.UserUpdate) (*auth.User, error) {
	p.mu.Lock()
	defer p.mu.Unlock()

	key := tenantKey(tenant) + "|" + string(id)
	su, ok := p.users[key]
	if !ok {
		for _, u := range p.users {
			if u.user.ID == id {
				return nil, auth.ErrTenantMismatch
			}
		}
		return nil, auth.ErrUserNotFound
	}
	if update.Email != nil {
		su.user.Email = *update.Email
	}
	if update.DisplayName != nil {
		su.user.DisplayName = *update.DisplayName
	}
	if update.Password != nil {
		su.password = *update.Password
	}
	if update.Disabled != nil {
		su.user.Disabled = *update.Disabled
	}
	su.user.UpdatedAt = p.now()
	clone := su.user
	return &clone, nil
}

// DeleteUser implements auth.IdentityProvider.
func (p *Provider) DeleteUser(_ context.Context, tenant auth.TenantRef, id auth.UserID) error {
	p.mu.Lock()
	defer p.mu.Unlock()

	key := tenantKey(tenant) + "|" + string(id)
	if _, ok := p.users[key]; !ok {
		for _, u := range p.users {
			if u.user.ID == id {
				return auth.ErrTenantMismatch
			}
		}
		return auth.ErrUserNotFound
	}
	delete(p.users, key)
	// Revoke any sessions belonging to the user.
	for sid, st := range p.sessions {
		if st.session.UserID == id && st.session.Tenant.Equal(tenant) {
			st.revoked = true
			delete(p.byAccess, st.session.AccessToken)
			delete(p.byRefresh, st.session.RefreshToken)
			_ = sid
		}
	}
	return nil
}

// --- Group CRUD & membership ---------------------------------------------

// ListGroups implements auth.IdentityProvider.
func (p *Provider) ListGroups(_ context.Context, tenant auth.TenantRef) ([]*auth.Group, error) {
	p.mu.Lock()
	defer p.mu.Unlock()

	tk := tenantKey(tenant)
	out := make([]*auth.Group, 0)
	for _, g := range p.groups {
		if tenantKey(g.Tenant) != tk {
			continue
		}
		clone := *g
		out = append(out, &clone)
	}
	return out, nil
}

// CreateGroup is a fake-only convenience for tests; it is not part of the
// IdentityProvider interface but matches the shape Wave 2's Zitadel adapter
// will expose internally.
func (p *Provider) CreateGroup(tenant auth.TenantRef, name, description string) (*auth.Group, error) {
	p.mu.Lock()
	defer p.mu.Unlock()

	now := p.now()
	id := auth.GroupID(p.genID("group"))
	g := &auth.Group{
		ID:          id,
		Tenant:      tenant,
		Name:        name,
		Description: description,
		CreatedAt:   now,
		UpdatedAt:   now,
	}
	p.groups[tenantKey(tenant)+"|"+string(id)] = g
	clone := *g
	return &clone, nil
}

// AddUserToGroup implements auth.IdentityProvider.
func (p *Provider) AddUserToGroup(_ context.Context, tenant auth.TenantRef, userID auth.UserID, groupID auth.GroupID) error {
	p.mu.Lock()
	defer p.mu.Unlock()

	tk := tenantKey(tenant)
	su, ok := p.users[tk+"|"+string(userID)]
	if !ok {
		return auth.ErrUserNotFound
	}
	g, ok := p.groups[tk+"|"+string(groupID)]
	if !ok {
		return auth.ErrGroupNotFound
	}
	if !g.Tenant.Equal(tenant) || !su.user.Tenant.Equal(tenant) {
		return auth.ErrTenantMismatch
	}
	for _, existing := range su.user.Groups {
		if existing == groupID {
			return nil
		}
	}
	su.user.Groups = append(su.user.Groups, groupID)
	su.user.UpdatedAt = p.now()
	return nil
}

// RemoveUserFromGroup implements auth.IdentityProvider.
func (p *Provider) RemoveUserFromGroup(_ context.Context, tenant auth.TenantRef, userID auth.UserID, groupID auth.GroupID) error {
	p.mu.Lock()
	defer p.mu.Unlock()

	tk := tenantKey(tenant)
	su, ok := p.users[tk+"|"+string(userID)]
	if !ok {
		return auth.ErrUserNotFound
	}
	out := su.user.Groups[:0]
	for _, g := range su.user.Groups {
		if g != groupID {
			out = append(out, g)
		}
	}
	su.user.Groups = out
	su.user.UpdatedAt = p.now()
	return nil
}

// --- Provider configuration ----------------------------------------------

// ConfigureProvider implements auth.IdentityProvider.
func (p *Provider) ConfigureProvider(ctx context.Context, tenant auth.TenantRef, cfg auth.ProviderConfig) error {
	// Per the fail-closed policy, run TestProvider first and refuse on
	// failure. The fake always passes for non-empty configs.
	res, err := p.TestProvider(ctx, tenant, cfg)
	if err != nil {
		return err
	}
	if res == nil || !res.Success {
		return auth.ErrProviderTestFailed
	}

	p.mu.Lock()
	defer p.mu.Unlock()

	if cfg.ID == "" {
		cfg.ID = auth.ProviderID(p.genID("idp"))
		cfg.CreatedAt = p.now()
	}
	cfg.Tenant = tenant
	cfg.UpdatedAt = p.now()
	stored := cfg
	p.providers[cfg.ID] = &stored
	return nil
}

// TestProvider implements auth.IdentityProvider.
func (p *Provider) TestProvider(_ context.Context, tenant auth.TenantRef, cfg auth.ProviderConfig) (*auth.TestResult, error) {
	if !cfg.Tenant.IsZero() && !cfg.Tenant.Equal(tenant) {
		return &auth.TestResult{Success: false, Message: "tenant mismatch"}, auth.ErrTenantMismatch
	}
	switch cfg.Kind {
	case auth.ProviderKindOIDC:
		if cfg.OIDC == nil || cfg.OIDC.IssuerURL == "" || cfg.OIDC.ClientID == "" {
			return &auth.TestResult{Success: false, Message: "missing oidc fields"}, nil
		}
	case auth.ProviderKindSAML:
		if cfg.SAML == nil || (cfg.SAML.MetadataURL == "" && cfg.SAML.MetadataXML == "") {
			return &auth.TestResult{Success: false, Message: "missing saml metadata"}, nil
		}
	case auth.ProviderKindLDAP:
		if cfg.LDAP == nil || cfg.LDAP.URL == "" || cfg.LDAP.BindDN == "" {
			return &auth.TestResult{Success: false, Message: "missing ldap fields"}, nil
		}
	default:
		return &auth.TestResult{Success: false, Message: "unknown provider kind"}, nil
	}
	return &auth.TestResult{Success: true, LatencyMS: 1, Message: "ok"}, nil
}

// SetClock allows tests to control the fake's notion of "now".
func (p *Provider) SetClock(now func() time.Time) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.now = now
}
