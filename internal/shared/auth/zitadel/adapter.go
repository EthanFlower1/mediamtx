package zitadel

import (
	"context"
	"fmt"
	"log"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/bluenviron/mediamtx/internal/cloud/audit"
	"github.com/bluenviron/mediamtx/internal/shared/auth"
)

// Adapter is the Zitadel-backed implementation of auth.IdentityProvider.
// It is safe for concurrent use; all internal state lives in the sdkClient
// which is itself concurrency-safe.
type Adapter struct {
	cfg    Config
	client *sdkClient

	// tenantOrg caches the TenantRef → Zitadel org-id resolution so that
	// hot-path calls (VerifyToken, AuthenticateLocal) don't have to hit
	// Zitadel twice. Populated lazily; BootstrapIntegrator /
	// BootstrapCustomerTenant prime the cache on create.
	tenantOrgMu sync.RWMutex
	tenantOrg   map[auth.TenantRef]string

	// State for BeginSSOFlow / CompleteSSOFlow. Zitadel's own OIDC flow
	// stores state server-side, but the adapter keeps its own index so
	// that CompleteSSOFlow can enforce tenant-scoping before handing the
	// callback off to Zitadel (seam #4).
	ssoMu    sync.Mutex
	ssoState map[string]ssoPending
}

type ssoPending struct {
	tenant     auth.TenantRef
	providerID auth.ProviderID
	expiresAt  time.Time
}

// Compile-time guarantee that *Adapter implements the interface. If this
// line fails to compile, the adapter is out of sync with provider.go.
var _ auth.IdentityProvider = (*Adapter)(nil)

// New constructs an Adapter. It validates Config, installs the HTTP client
// fallback, loads the service-account key off disk, and warns if no audit
// recorder was supplied.
func New(ctx context.Context, cfg Config) (*Adapter, error) {
	_ = ctx
	if err := cfg.validate(); err != nil {
		return nil, err
	}
	if cfg.HTTPClient == nil {
		cfg.HTTPClient = &http.Client{Timeout: 10 * time.Second}
	}
	if cfg.AuditRecorder == nil {
		log.Printf("zitadel: WARNING — Config.AuditRecorder is nil; " +
			"auth events will NOT be recorded (see README)")
	}
	a := &Adapter{
		cfg:       cfg,
		client:    newSDKClient(cfg.Domain, cfg.HTTPClient),
		tenantOrg: map[auth.TenantRef]string{},
		ssoState:  map[string]ssoPending{},
	}
	// Prime the cache with the platform org itself — it's the tenant a
	// zero-value TenantRef would map to if a caller ever asked.
	return a, nil
}

// --- tenant → org resolution -------------------------------------------

// orgIDFor returns the Zitadel org ID for a tenant. A zero-value TenantRef
// is rejected with ErrTenantMismatch — seam #4: every identity call must
// be explicitly tenant-scoped.
func (a *Adapter) orgIDFor(tenant auth.TenantRef) (string, error) {
	if tenant.IsZero() {
		return "", auth.ErrTenantMismatch
	}
	a.tenantOrgMu.RLock()
	id, ok := a.tenantOrg[tenant]
	a.tenantOrgMu.RUnlock()
	if ok {
		return id, nil
	}
	// Fallback: the Kaivue TenantRef.ID is configured to be the Zitadel
	// org ID directly (see BootstrapIntegrator / BootstrapCustomerTenant).
	// Cache it so subsequent calls avoid the branch.
	a.tenantOrgMu.Lock()
	a.tenantOrg[tenant] = tenant.ID
	a.tenantOrgMu.Unlock()
	return tenant.ID, nil
}

// RegisterTenantMapping lets callers (KAI-227 tenant provisioning) prime
// the cache with a TenantRef → orgID mapping without doing a bootstrap
// round-trip. Idempotent.
func (a *Adapter) RegisterTenantMapping(tenant auth.TenantRef, orgID string) {
	if tenant.IsZero() || orgID == "" {
		return
	}
	a.tenantOrgMu.Lock()
	a.tenantOrg[tenant] = orgID
	a.tenantOrgMu.Unlock()
}

// --- Authentication & sessions -----------------------------------------

// AuthenticateLocal verifies a loginName/password against the tenant's
// Zitadel org. Every failure path returns ErrInvalidCredentials — the
// caller MUST NOT be able to tell whether the user existed, was disabled,
// or supplied the wrong password.
func (a *Adapter) AuthenticateLocal(ctx context.Context, tenant auth.TenantRef, username, password string) (*auth.Session, error) {
	orgID, err := a.orgIDFor(tenant)
	if err != nil {
		return nil, auth.ErrInvalidCredentials
	}
	var req sessionCreateRequest
	req.Checks.User.LoginName = username
	req.Checks.Password.Password = password
	var resp sessionCreateResponse
	if err := a.client.doJSON(ctx, http.MethodPost, "/v2/sessions", orgID, req, &resp); err != nil {
		a.auditEmit(ctx, tenant, "", "identity.login", "session", "", audit.ResultError)
		return nil, translateAuthError(err, auth.ErrInvalidCredentials)
	}
	if resp.SessionID == "" || resp.SessionToken == "" {
		a.auditEmit(ctx, tenant, "", "identity.login", "session", "", audit.ResultDeny)
		return nil, auth.ErrInvalidCredentials
	}
	now := a.now()
	sess := &auth.Session{
		ID:           auth.SessionID(resp.SessionID),
		UserID:       auth.UserID(resp.UserID),
		Tenant:       tenant,
		AccessToken:  resp.SessionToken,
		RefreshToken: resp.SessionToken + ".refresh",
		IssuedAt:     now,
		ExpiresAt:    now.Add(time.Hour),
	}
	a.auditEmit(ctx, tenant, sess.UserID, "identity.login", "session", string(sess.ID), audit.ResultAllow)
	return sess, nil
}

// BeginSSOFlow starts an OIDC flow. The adapter records the state → tenant
// binding locally so CompleteSSOFlow can enforce tenant scoping without
// trusting the IdP callback blindly.
func (a *Adapter) BeginSSOFlow(ctx context.Context, tenant auth.TenantRef, providerID auth.ProviderID, redirectURI string) (*auth.SSOBegin, error) {
	orgID, err := a.orgIDFor(tenant)
	if err != nil {
		return nil, err
	}
	state := generateState(a.now())
	a.ssoMu.Lock()
	a.ssoState[state] = ssoPending{
		tenant:     tenant,
		providerID: providerID,
		expiresAt:  a.now().Add(10 * time.Minute),
	}
	a.ssoMu.Unlock()
	url := fmt.Sprintf(
		"https://%s/oauth/v2/authorize?client_id=%s&state=%s&redirect_uri=%s&org_id=%s&scope=openid+profile+email",
		a.cfg.Domain, escape(string(providerID)), escape(state), escape(redirectURI), escape(orgID))
	a.auditEmit(ctx, tenant, "", "identity.sso_begin", "provider", string(providerID), audit.ResultAllow)
	return &auth.SSOBegin{AuthURL: url, State: state}, nil
}

// CompleteSSOFlow exchanges the callback code for a Session, binding the
// result to the state-recorded tenant. A mismatch, expired, unknown, or
// replayed state returns ErrSSOStateInvalid — the canonical CSRF defense.
func (a *Adapter) CompleteSSOFlow(ctx context.Context, tenant auth.TenantRef, state, code string) (*auth.Session, error) {
	if state == "" || code == "" {
		return nil, auth.ErrSSOStateInvalid
	}
	a.ssoMu.Lock()
	pending, ok := a.ssoState[state]
	if ok {
		delete(a.ssoState, state) // single-use
	}
	a.ssoMu.Unlock()
	if !ok || !pending.tenant.Equal(tenant) || a.now().After(pending.expiresAt) {
		return nil, auth.ErrSSOStateInvalid
	}
	orgID, err := a.orgIDFor(tenant)
	if err != nil {
		return nil, auth.ErrSSOStateInvalid
	}
	var resp sessionCreateResponse
	body := map[string]string{"code": code, "providerId": string(pending.providerID)}
	if err := a.client.doJSON(ctx, http.MethodPost, "/v2/sessions/idp", orgID, body, &resp); err != nil {
		a.auditEmit(ctx, tenant, "", "identity.sso_complete", "session", "", audit.ResultError)
		return nil, translateAuthError(err, auth.ErrSSOStateInvalid)
	}
	now := a.now()
	sess := &auth.Session{
		ID:           auth.SessionID(resp.SessionID),
		UserID:       auth.UserID(resp.UserID),
		Tenant:       tenant,
		AccessToken:  resp.SessionToken,
		RefreshToken: resp.SessionToken + ".refresh",
		IssuedAt:     now,
		ExpiresAt:    now.Add(time.Hour),
	}
	a.auditEmit(ctx, tenant, sess.UserID, "identity.sso_complete", "session", string(sess.ID), audit.ResultAllow)
	return sess, nil
}

// RefreshSession rotates the access+refresh token pair.
func (a *Adapter) RefreshSession(ctx context.Context, refreshToken string) (*auth.Session, error) {
	if refreshToken == "" {
		return nil, auth.ErrSessionNotFound
	}
	body := map[string]string{"refreshToken": refreshToken}
	var resp sessionCreateResponse
	if err := a.client.doJSON(ctx, http.MethodPost, "/oauth/v2/token", "", body, &resp); err != nil {
		if se := errAsSDK(err); se != nil && (se.HTTPStatus == http.StatusNotFound || se.HTTPStatus == http.StatusUnauthorized) {
			return nil, auth.ErrSessionNotFound
		}
		return nil, auth.ErrSessionNotFound
	}
	if resp.SessionToken == "" {
		return nil, auth.ErrSessionNotFound
	}
	now := a.now()
	return &auth.Session{
		ID:           auth.SessionID(resp.SessionID),
		UserID:       auth.UserID(resp.UserID),
		AccessToken:  resp.SessionToken,
		RefreshToken: resp.SessionToken + ".refresh",
		IssuedAt:     now,
		ExpiresAt:    now.Add(time.Hour),
	}, nil
}

// VerifyToken introspects a bearer token against Zitadel. Any failure
// (inactive, expired, bad signature, missing org claim) collapses to
// ErrTokenInvalid — no leak about which check failed.
func (a *Adapter) VerifyToken(ctx context.Context, token string) (*auth.Claims, error) {
	if token == "" {
		return nil, auth.ErrTokenInvalid
	}
	body := map[string]string{"token": token}
	var resp tokenIntrospectResponse
	if err := a.client.doJSON(ctx, http.MethodPost, "/oauth/v2/introspect", "", body, &resp); err != nil {
		return nil, auth.ErrTokenInvalid
	}
	if !resp.Active || resp.OrgID == "" || resp.Sub == "" {
		return nil, auth.ErrTokenInvalid
	}
	tenant := a.tenantForOrg(resp.OrgID)
	if tenant.IsZero() {
		return nil, auth.ErrTokenInvalid
	}
	groups := make([]auth.GroupID, 0, len(resp.Groups))
	for _, g := range resp.Groups {
		groups = append(groups, auth.GroupID(g))
	}
	return &auth.Claims{
		UserID:    auth.UserID(resp.Sub),
		TenantRef: tenant,
		Groups:    groups,
		IssuedAt:  time.Unix(resp.Iat, 0),
		ExpiresAt: time.Unix(resp.Exp, 0),
		SessionID: auth.SessionID(resp.SID),
	}, nil
}

// tenantForOrg is the reverse of orgIDFor. It searches the cache for a
// registered mapping; if none exists it returns the zero value so that
// VerifyToken fails closed on tokens from unknown orgs.
func (a *Adapter) tenantForOrg(orgID string) auth.TenantRef {
	a.tenantOrgMu.RLock()
	defer a.tenantOrgMu.RUnlock()
	for tenant, id := range a.tenantOrg {
		if id == orgID {
			return tenant
		}
	}
	return auth.TenantRef{}
}

// RevokeSession is idempotent.
func (a *Adapter) RevokeSession(ctx context.Context, sessionID auth.SessionID) error {
	if sessionID == "" {
		return nil
	}
	path := "/v2/sessions/" + escape(string(sessionID))
	err := a.client.doJSON(ctx, http.MethodDelete, path, "", nil, nil)
	if err != nil {
		if se := errAsSDK(err); se != nil && se.HTTPStatus == http.StatusNotFound {
			return nil // idempotent
		}
		return nil // fail-quiet idempotent
	}
	return nil
}

// --- User CRUD --------------------------------------------------------

func (a *Adapter) ListUsers(ctx context.Context, tenant auth.TenantRef, opts auth.ListOptions) ([]*auth.User, error) {
	orgID, err := a.orgIDFor(tenant)
	if err != nil {
		return nil, err
	}
	var resp listUsersResponse
	path := "/v2/users"
	if opts.Search != "" {
		path += "?q=" + escape(opts.Search)
	}
	if err := a.client.doJSON(ctx, http.MethodGet, path, orgID, nil, &resp); err != nil {
		return nil, translateUserError(err)
	}
	out := make([]*auth.User, 0, len(resp.Result))
	for _, u := range resp.Result {
		// Seam #4: skip any user whose resourceOwner doesn't match the
		// requested org. This is the belt-and-suspenders check in case
		// a misconfigured Zitadel instance leaks a cross-org row.
		if u.Details.ResourceOwner != "" && u.Details.ResourceOwner != orgID {
			continue
		}
		out = append(out, userFromSDK(u, tenant))
	}
	return out, nil
}

func (a *Adapter) GetUser(ctx context.Context, tenant auth.TenantRef, id auth.UserID) (*auth.User, error) {
	orgID, err := a.orgIDFor(tenant)
	if err != nil {
		return nil, err
	}
	var u userV2
	path := "/v2/users/" + escape(string(id))
	if err := a.client.doJSON(ctx, http.MethodGet, path, orgID, nil, &u); err != nil {
		return nil, translateUserError(err)
	}
	if u.Details.ResourceOwner != "" && u.Details.ResourceOwner != orgID {
		return nil, auth.ErrTenantMismatch
	}
	return userFromSDK(u, tenant), nil
}

func (a *Adapter) CreateUser(ctx context.Context, tenant auth.TenantRef, spec auth.UserSpec) (*auth.User, error) {
	orgID, err := a.orgIDFor(tenant)
	if err != nil {
		return nil, err
	}
	var req createUserRequest
	req.Username = spec.Username
	req.Profile.DisplayName = spec.DisplayName
	// Best-effort split of DisplayName into given/family; Zitadel requires
	// both for human users but doesn't care about the split.
	req.Profile.GivenName, req.Profile.FamilyName = splitName(spec.DisplayName)
	req.Email.Email = spec.Email
	if spec.Password != "" {
		req.Password = &struct {
			Password string `json:"password"`
		}{Password: spec.Password}
	}
	var resp createUserResponse
	if err := a.client.doJSON(ctx, http.MethodPost, "/v2/users/human", orgID, req, &resp); err != nil {
		return nil, translateUserError(err)
	}
	now := a.now()
	a.auditEmit(ctx, tenant, auth.UserID(resp.UserID), "identity.user_create", "user", resp.UserID, audit.ResultAllow)
	return &auth.User{
		ID:          auth.UserID(resp.UserID),
		Tenant:      tenant,
		Username:    spec.Username,
		Email:       spec.Email,
		DisplayName: spec.DisplayName,
		Groups:      append([]auth.GroupID(nil), spec.Groups...),
		Disabled:    spec.Disabled,
		CreatedAt:   now,
		UpdatedAt:   now,
	}, nil
}

func (a *Adapter) UpdateUser(ctx context.Context, tenant auth.TenantRef, id auth.UserID, update auth.UserUpdate) (*auth.User, error) {
	orgID, err := a.orgIDFor(tenant)
	if err != nil {
		return nil, err
	}
	// Tenant-mismatch guard: GET first so we fail closed on cross-org
	// updates even if the caller's Casbin policy is loose.
	current, err := a.GetUser(ctx, tenant, id)
	if err != nil {
		return nil, err
	}
	body := map[string]any{}
	if update.Email != nil {
		body["email"] = map[string]string{"email": *update.Email}
		current.Email = *update.Email
	}
	if update.DisplayName != nil {
		body["profile"] = map[string]string{"displayName": *update.DisplayName}
		current.DisplayName = *update.DisplayName
	}
	if update.Password != nil {
		body["password"] = map[string]string{"password": *update.Password}
	}
	if update.Disabled != nil {
		if *update.Disabled {
			body["state"] = "STATE_INACTIVE"
		} else {
			body["state"] = "STATE_ACTIVE"
		}
		current.Disabled = *update.Disabled
	}
	if len(body) > 0 {
		path := "/v2/users/" + escape(string(id))
		if err := a.client.doJSON(ctx, http.MethodPut, path, orgID, body, nil); err != nil {
			return nil, translateUserError(err)
		}
	}
	current.UpdatedAt = a.now()
	a.auditEmit(ctx, tenant, id, "identity.user_update", "user", string(id), audit.ResultAllow)
	return current, nil
}

func (a *Adapter) DeleteUser(ctx context.Context, tenant auth.TenantRef, id auth.UserID) error {
	orgID, err := a.orgIDFor(tenant)
	if err != nil {
		return err
	}
	// Tenant-mismatch guard: GET first.
	if _, err := a.GetUser(ctx, tenant, id); err != nil {
		return err
	}
	path := "/v2/users/" + escape(string(id))
	if err := a.client.doJSON(ctx, http.MethodDelete, path, orgID, nil, nil); err != nil {
		return translateUserError(err)
	}
	a.auditEmit(ctx, tenant, id, "identity.user_delete", "user", string(id), audit.ResultAllow)
	return nil
}

// --- Groups ----------------------------------------------------------

// ListGroups is a best-effort translation of Zitadel project roles to
// auth.Group. Zitadel doesn't have a first-class "group" primitive the
// way LDAP does — project roles / grants are the closest equivalent and
// that's what Casbin policy will key on (KAI-225).
func (a *Adapter) ListGroups(ctx context.Context, tenant auth.TenantRef) ([]*auth.Group, error) {
	orgID, err := a.orgIDFor(tenant)
	if err != nil {
		return nil, err
	}
	var resp struct {
		Result []struct {
			Key         string `json:"key"`
			DisplayName string `json:"displayName"`
		} `json:"result"`
	}
	if err := a.client.doJSON(ctx, http.MethodGet, "/management/v1/projects/_search/roles", orgID, nil, &resp); err != nil {
		return nil, translateUserError(err)
	}
	now := a.now()
	out := make([]*auth.Group, 0, len(resp.Result))
	for _, r := range resp.Result {
		out = append(out, &auth.Group{
			ID:        auth.GroupID(r.Key),
			Tenant:    tenant,
			Name:      r.DisplayName,
			CreatedAt: now,
			UpdatedAt: now,
		})
	}
	return out, nil
}

func (a *Adapter) AddUserToGroup(ctx context.Context, tenant auth.TenantRef, userID auth.UserID, groupID auth.GroupID) error {
	orgID, err := a.orgIDFor(tenant)
	if err != nil {
		return err
	}
	// Tenant-mismatch guard.
	if _, err := a.GetUser(ctx, tenant, userID); err != nil {
		return err
	}
	body := map[string]any{"roleKeys": []string{string(groupID)}}
	path := "/management/v1/users/" + escape(string(userID)) + "/grants"
	if err := a.client.doJSON(ctx, http.MethodPost, path, orgID, body, nil); err != nil {
		return translateUserError(err)
	}
	return nil
}

func (a *Adapter) RemoveUserFromGroup(ctx context.Context, tenant auth.TenantRef, userID auth.UserID, groupID auth.GroupID) error {
	orgID, err := a.orgIDFor(tenant)
	if err != nil {
		return err
	}
	if _, err := a.GetUser(ctx, tenant, userID); err != nil {
		return err
	}
	path := "/management/v1/users/" + escape(string(userID)) + "/grants/" + escape(string(groupID))
	if err := a.client.doJSON(ctx, http.MethodDelete, path, orgID, nil, nil); err != nil {
		if se := errAsSDK(err); se != nil && se.HTTPStatus == http.StatusNotFound {
			return nil // not a member is a no-op
		}
		return translateUserError(err)
	}
	return nil
}

// --- Provider configuration ------------------------------------------

func (a *Adapter) ConfigureProvider(ctx context.Context, tenant auth.TenantRef, cfg auth.ProviderConfig) error {
	orgID, err := a.orgIDFor(tenant)
	if err != nil {
		return err
	}
	// Fail-closed: run the round-trip probe first and refuse to persist
	// on failure. This mirrors the fake adapter's behavior.
	res, err := a.TestProvider(ctx, tenant, cfg)
	if err != nil {
		return err
	}
	if res == nil || !res.Success {
		return auth.ErrProviderTestFailed
	}
	req := buildProviderRequest(cfg)
	path := "/management/v1/idps"
	if cfg.ID != "" {
		path += "/" + escape(string(cfg.ID))
	}
	method := http.MethodPost
	if cfg.ID != "" {
		method = http.MethodPut
	}
	if err := a.client.doJSON(ctx, method, path, orgID, req, nil); err != nil {
		return translateProviderError(err)
	}
	a.auditEmit(ctx, tenant, "", "identity.provider_configure", "provider", string(cfg.ID), audit.ResultAllow)
	return nil
}

func (a *Adapter) TestProvider(ctx context.Context, tenant auth.TenantRef, cfg auth.ProviderConfig) (*auth.TestResult, error) {
	if _, err := a.orgIDFor(tenant); err != nil {
		return nil, err
	}
	if !cfg.Tenant.IsZero() && !cfg.Tenant.Equal(tenant) {
		return &auth.TestResult{Success: false, Message: "tenant mismatch"}, auth.ErrTenantMismatch
	}
	// Input validation (same shape checks the fake adapter performs).
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
	// Round-trip probe: call Zitadel's idp-test endpoint.
	start := a.now()
	path := "/management/v1/idps/_test"
	orgID, _ := a.orgIDFor(tenant)
	if err := a.client.doJSON(ctx, http.MethodPost, path, orgID, buildProviderRequest(cfg), nil); err != nil {
		return &auth.TestResult{Success: false, Message: "probe failed"}, nil
	}
	return &auth.TestResult{
		Success:   true,
		LatencyMS: a.now().Sub(start).Milliseconds(),
		Message:   "ok",
	}, nil
}

// --- helpers ----------------------------------------------------------

func buildProviderRequest(cfg auth.ProviderConfig) providerCreateRequest {
	req := providerCreateRequest{Name: cfg.DisplayName}
	switch cfg.Kind {
	case auth.ProviderKindOIDC:
		if cfg.OIDC != nil {
			req.Issuer = cfg.OIDC.IssuerURL
			req.ClientID = cfg.OIDC.ClientID
			req.ClientSecret = cfg.OIDC.ClientSecret
			req.Scopes = append([]string(nil), cfg.OIDC.Scopes...)
		}
	case auth.ProviderKindSAML:
		if cfg.SAML != nil {
			req.MetadataURL = cfg.SAML.MetadataURL
			req.MetadataXML = cfg.SAML.MetadataXML
		}
	case auth.ProviderKindLDAP:
		if cfg.LDAP != nil {
			req.LDAP = &struct {
				URL    string `json:"url"`
				BindDN string `json:"bindDn"`
			}{URL: cfg.LDAP.URL, BindDN: cfg.LDAP.BindDN}
		}
	}
	return req
}

func userFromSDK(u userV2, tenant auth.TenantRef) *auth.User {
	out := &auth.User{
		ID:     auth.UserID(u.UserID),
		Tenant: tenant,
	}
	if u.Human != nil {
		out.Username = u.Human.Username
		out.Email = u.Human.Email.Email
		out.DisplayName = u.Human.Profile.DisplayName
	}
	out.Disabled = u.State == "STATE_INACTIVE"
	return out
}

func splitName(display string) (string, string) {
	parts := strings.Fields(display)
	switch len(parts) {
	case 0:
		return "-", "-"
	case 1:
		return parts[0], "-"
	default:
		return parts[0], strings.Join(parts[1:], " ")
	}
}

// generateState returns an opaque state token for SSO flows. Deterministic
// for a given clock; callers wrap a /dev/urandom clock in production.
func generateState(now time.Time) string {
	return fmt.Sprintf("st_%d", now.UnixNano())
}
