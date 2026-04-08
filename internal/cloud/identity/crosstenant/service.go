package crosstenant

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"sort"
	"time"

	"github.com/golang-jwt/jwt/v5"

	"github.com/bluenviron/mediamtx/internal/cloud/audit"
	"github.com/bluenviron/mediamtx/internal/cloud/permissions"
	"github.com/bluenviron/mediamtx/internal/shared/auth"
)

// Audit actions emitted by this package. Sibling packages (KAI-226 API server)
// reference these constants when asserting on the audit log.
const (
	AuditActionCrossTenantGrant  = "permissions.cross_tenant_grant"
	AuditActionCrossTenantVerify = "permissions.cross_tenant_verify"

	// JWTIssuer is the issuer claim baked into every scoped token. Callers
	// verifying tokens from outside the package (the API server middleware)
	// should assert this value.
	JWTIssuer = "mediamtx-cloud/crosstenant"

	// DefaultTTL is the lifetime of a newly minted scoped token.
	DefaultTTL = 15 * time.Minute
)

// Config configures a Service. All fields except SigningKey and TTL have
// sensible defaults.
type Config struct {
	// SigningKey is the HS256 secret used to sign scoped tokens. In tests
	// this is the sha256 of "test-jwt-key-REPLACE_ME" or similar. In
	// production it comes from the cloud's KMS — NEVER a real key in this
	// package or any test fixture.
	SigningKey []byte

	// TTL is the scoped token lifetime. Defaults to DefaultTTL.
	TTL time.Duration

	// Now is injectable for tests. Defaults to time.Now.
	Now func() time.Time

	// NewSessionID is injectable for tests. Defaults to 32 hex chars from
	// crypto/rand.
	NewSessionID func() string

	// IntegratorTenant is the integrator's own tenant. KAI-224 services are
	// scoped to a single integrator tenant; the API layer will construct one
	// Service per integrator tenant (or inject it through a lookup). Kept as
	// Config for test simplicity.
	IntegratorTenant auth.TenantRef
}

// Service implements the cross-tenant access workflow.
type Service struct {
	cfg Config

	identity           auth.IdentityProvider
	relationshipStore  RelationshipStore
	permissionStore    permissions.IntegratorRelationshipStore
	sessionStore       ScopedSessionStore
	auditRecorder      audit.Recorder
}

// NewService constructs a cross-tenant service. All dependencies are required.
func NewService(
	cfg Config,
	identity auth.IdentityProvider,
	relationshipStore RelationshipStore,
	permissionStore permissions.IntegratorRelationshipStore,
	sessionStore ScopedSessionStore,
	auditRecorder audit.Recorder,
) (*Service, error) {
	if len(cfg.SigningKey) == 0 {
		return nil, errors.New("crosstenant: signing key is required")
	}
	if identity == nil {
		return nil, errors.New("crosstenant: identity provider is required")
	}
	if relationshipStore == nil {
		return nil, errors.New("crosstenant: relationship store is required")
	}
	if permissionStore == nil {
		return nil, errors.New("crosstenant: permission store is required")
	}
	if sessionStore == nil {
		return nil, errors.New("crosstenant: session store is required")
	}
	if auditRecorder == nil {
		return nil, errors.New("crosstenant: audit recorder is required")
	}
	if cfg.IntegratorTenant.ID == "" || cfg.IntegratorTenant.Type == "" {
		return nil, errors.New("crosstenant: integrator tenant is required")
	}
	if cfg.TTL <= 0 {
		cfg.TTL = DefaultTTL
	}
	if cfg.Now == nil {
		cfg.Now = time.Now
	}
	if cfg.NewSessionID == nil {
		cfg.NewSessionID = defaultSessionID
	}
	return &Service{
		cfg:               cfg,
		identity:          identity,
		relationshipStore: relationshipStore,
		permissionStore:   permissionStore,
		sessionStore:      sessionStore,
		auditRecorder:     auditRecorder,
	}, nil
}

// MintScopedToken issues a new cross-tenant scoped JWT for
// (integratorUserID, customerTenantID). It runs the flow documented in the
// package doc and emits an audit entry with action
// "permissions.cross_tenant_grant" on every code path (allow, deny, error).
func (s *Service) MintScopedToken(
	ctx context.Context,
	integratorUserID auth.UserID,
	customerTenantID string,
) (*ScopedToken, error) {
	customerTenant := auth.TenantRef{Type: auth.TenantTypeCustomer, ID: customerTenantID}

	// (a) Verify integrator user exists in the integrator tenant.
	if _, err := s.identity.GetUser(ctx, s.cfg.IntegratorTenant, integratorUserID); err != nil {
		s.recordMintFailure(ctx, integratorUserID, customerTenantID, "unknown_integrator")
		if errors.Is(err, auth.ErrUserNotFound) || errors.Is(err, auth.ErrTenantMismatch) {
			return nil, ErrUnknownIntegrator
		}
		return nil, fmt.Errorf("crosstenant: identity lookup: %w", err)
	}

	// (b) Confirm a non-revoked relationship exists.
	rel, ok, err := s.relationshipStore.Lookup(ctx, string(integratorUserID), customerTenantID)
	if err != nil {
		s.recordMintFailure(ctx, integratorUserID, customerTenantID, "relationship_lookup_error")
		return nil, fmt.Errorf("crosstenant: relationship lookup: %w", err)
	}
	if !ok {
		s.recordMintFailure(ctx, integratorUserID, customerTenantID, "no_relationship")
		return nil, ErrNoRelationship
	}
	if rel.Revoked {
		s.recordMintFailure(ctx, integratorUserID, customerTenantID, "relationship_revoked")
		return nil, ErrRelationshipRevoked
	}

	// (c) Walk the parent chain and compute the intersected scope. If the
	// permission store errors with "hierarchy too deep" we translate it to
	// our sentinel.
	allowed, found, err := permissions.ResolveIntegratorScope(
		ctx, s.permissionStore, integratorUserID, customerTenant,
	)
	if err != nil {
		s.recordMintFailure(ctx, integratorUserID, customerTenantID, "scope_resolve_error")
		if isDepthExceeded(err) {
			return nil, ErrHierarchyTooDeep
		}
		return nil, fmt.Errorf("crosstenant: resolve scope: %w", err)
	}
	// The permissions package walks silently up to maxDepth=32. Our
	// fail-closed contract says the caller must be told — so we also check
	// the chain length ourselves via a depth probe.
	if depth, dErr := probeHierarchyDepth(ctx, s.permissionStore, integratorUserID, customerTenant); dErr != nil {
		s.recordMintFailure(ctx, integratorUserID, customerTenantID, "depth_probe_error")
		return nil, fmt.Errorf("crosstenant: probe depth: %w", dErr)
	} else if depth > maxHierarchyDepth {
		s.recordMintFailure(ctx, integratorUserID, customerTenantID, "hierarchy_too_deep")
		return nil, ErrHierarchyTooDeep
	}
	if !found {
		// Store says no direct relationship — this shouldn't happen because
		// (b) already confirmed one, but fail-closed just in case.
		s.recordMintFailure(ctx, integratorUserID, customerTenantID, "permissions_no_relationship")
		return nil, ErrNoRelationship
	}
	if len(allowed) == 0 {
		s.recordMintFailure(ctx, integratorUserID, customerTenantID, "empty_scope")
		return nil, ErrEmptyScope
	}
	sort.Strings(allowed)

	// (d) Assemble claims and sign.
	now := s.cfg.Now().UTC()
	expiresAt := now.Add(s.cfg.TTL)
	sessionID := s.cfg.NewSessionID()
	subject := permissions.NewIntegratorSubject(integratorUserID, customerTenant).String()

	claims := jwt.MapClaims{
		"iss":                  JWTIssuer,
		"sub":                  subject,
		"iat":                  now.Unix(),
		"nbf":                  now.Unix(),
		"exp":                  expiresAt.Unix(),
		"sid":                  sessionID,
		"integrator_user_id":   string(integratorUserID),
		"integrator_tenant_id": s.cfg.IntegratorTenant.ID,
		"customer_tenant_id":   customerTenantID,
		"scope":                allowed,
	}
	tok := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	signed, err := tok.SignedString(s.cfg.SigningKey)
	if err != nil {
		s.recordMintFailure(ctx, integratorUserID, customerTenantID, "sign_error")
		return nil, fmt.Errorf("crosstenant: sign token: %w", err)
	}

	// (f) Store session.
	if err := s.sessionStore.Put(ctx, ScopedSessionRecord{
		SessionID:        sessionID,
		IntegratorUserID: string(integratorUserID),
		IntegratorTenant: s.cfg.IntegratorTenant.ID,
		CustomerTenantID: customerTenantID,
		PermissionScope:  allowed,
		IssuedAt:         now,
		ExpiresAt:        expiresAt,
	}); err != nil {
		s.recordMintFailure(ctx, integratorUserID, customerTenantID, "session_store_error")
		return nil, fmt.Errorf("crosstenant: persist session: %w", err)
	}

	// (e) Emit audit entry on success.
	impersonatingUID := string(integratorUserID)
	impersonatedTenantID := customerTenantID
	if err := s.auditRecorder.Record(ctx, audit.Entry{
		TenantID:             customerTenantID,
		ActorUserID:          string(integratorUserID),
		ActorAgent:           audit.AgentIntegrator,
		ImpersonatingUserID:  &impersonatingUID,
		ImpersonatedTenantID: &impersonatedTenantID,
		Action:               AuditActionCrossTenantGrant,
		ResourceType:         "scoped_token",
		ResourceID:           sessionID,
		Result:               audit.ResultAllow,
		Timestamp:            now,
	}); err != nil {
		return nil, fmt.Errorf("crosstenant: record audit: %w", err)
	}

	return &ScopedToken{
		Token:            signed,
		SessionID:        sessionID,
		ExpiresAt:        expiresAt,
		CustomerTenantID: customerTenantID,
		PermissionScope:  allowed,
	}, nil
}

// VerifyScopedToken parses + verifies a scoped token. It also checks the
// revocation store. Every verification — success or failure — emits an audit
// entry with action "permissions.cross_tenant_verify".
func (s *Service) VerifyScopedToken(ctx context.Context, token string) (*ScopedClaims, error) {
	parsed, err := jwt.Parse(token, func(t *jwt.Token) (interface{}, error) {
		if _, ok := t.Method.(*jwt.SigningMethodHMAC); !ok {
			return nil, fmt.Errorf("unexpected signing method: %v", t.Header["alg"])
		}
		return s.cfg.SigningKey, nil
	}, jwt.WithIssuer(JWTIssuer), jwt.WithTimeFunc(func() time.Time { return s.cfg.Now() }))
	if err != nil {
		s.recordVerifyFailure(ctx, "", "", "", "parse_error")
		if errors.Is(err, jwt.ErrTokenExpired) {
			return nil, ErrScopedTokenExpired
		}
		return nil, ErrScopedTokenInvalid
	}
	if !parsed.Valid {
		s.recordVerifyFailure(ctx, "", "", "", "invalid_token")
		return nil, ErrScopedTokenInvalid
	}

	mc, ok := parsed.Claims.(jwt.MapClaims)
	if !ok {
		s.recordVerifyFailure(ctx, "", "", "", "claims_type")
		return nil, ErrScopedTokenInvalid
	}

	sub, _ := mc["sub"].(string)
	sid, _ := mc["sid"].(string)
	integratorUserID, _ := mc["integrator_user_id"].(string)
	integratorTenantID, _ := mc["integrator_tenant_id"].(string)
	customerTenantID, _ := mc["customer_tenant_id"].(string)
	if sub == "" || sid == "" || integratorUserID == "" || customerTenantID == "" {
		s.recordVerifyFailure(ctx, integratorUserID, integratorTenantID, customerTenantID, "missing_claim")
		return nil, ErrScopedTokenInvalid
	}

	var scope []string
	if raw, ok := mc["scope"].([]interface{}); ok {
		for _, v := range raw {
			if s, ok := v.(string); ok {
				scope = append(scope, s)
			}
		}
	}

	iat := timeFromClaim(mc, "iat")
	exp := timeFromClaim(mc, "exp")

	// Session revocation check.
	rec, ok, err := s.sessionStore.Get(ctx, sid)
	if err != nil {
		s.recordVerifyFailure(ctx, integratorUserID, integratorTenantID, customerTenantID, "session_lookup_error")
		return nil, fmt.Errorf("crosstenant: session lookup: %w", err)
	}
	if !ok || rec.Revoked {
		s.recordVerifyFailure(ctx, integratorUserID, integratorTenantID, customerTenantID, "session_revoked")
		return nil, ErrSessionRevoked
	}

	claims := &ScopedClaims{
		Subject:          sub,
		IntegratorUserID: auth.UserID(integratorUserID),
		IntegratorTenant: auth.TenantRef{Type: auth.TenantTypeIntegrator, ID: integratorTenantID},
		CustomerTenant:   auth.TenantRef{Type: auth.TenantTypeCustomer, ID: customerTenantID},
		PermissionScope:  scope,
		SessionID:        sid,
		IssuedAt:         iat,
		ExpiresAt:        exp,
	}

	// Success audit entry.
	impersonatingUID := integratorUserID
	impersonatedTenantID := customerTenantID
	if err := s.auditRecorder.Record(ctx, audit.Entry{
		TenantID:             customerTenantID,
		ActorUserID:          integratorUserID,
		ActorAgent:           audit.AgentIntegrator,
		ImpersonatingUserID:  &impersonatingUID,
		ImpersonatedTenantID: &impersonatedTenantID,
		Action:               AuditActionCrossTenantVerify,
		ResourceType:         "scoped_token",
		ResourceID:           sid,
		Result:               audit.ResultAllow,
		Timestamp:            s.cfg.Now().UTC(),
	}); err != nil {
		return nil, fmt.Errorf("crosstenant: record audit: %w", err)
	}
	return claims, nil
}

// RevokeScopedSession marks a session revoked. Idempotent — an unknown id is
// a no-op, matching auth.IdentityProvider.RevokeSession semantics.
func (s *Service) RevokeScopedSession(ctx context.Context, sessionID string) error {
	return s.sessionStore.Revoke(ctx, sessionID)
}

// --- helpers --------------------------------------------------------------

func (s *Service) recordMintFailure(ctx context.Context, uid auth.UserID, customerTenantID, code string) {
	errCode := code
	impersonatingUID := string(uid)
	var impersonatedTenantID *string
	if customerTenantID != "" {
		t := customerTenantID
		impersonatedTenantID = &t
	}
	tenantID := customerTenantID
	if tenantID == "" {
		tenantID = s.cfg.IntegratorTenant.ID
	}
	_ = s.auditRecorder.Record(ctx, audit.Entry{
		TenantID:             tenantID,
		ActorUserID:          string(uid),
		ActorAgent:           audit.AgentIntegrator,
		ImpersonatingUserID:  &impersonatingUID,
		ImpersonatedTenantID: impersonatedTenantID,
		Action:               AuditActionCrossTenantGrant,
		ResourceType:         "scoped_token",
		ResourceID:           "",
		Result:               audit.ResultDeny,
		ErrorCode:            &errCode,
		Timestamp:            s.cfg.Now().UTC(),
	})
}

func (s *Service) recordVerifyFailure(ctx context.Context, uid, integratorTenantID, customerTenantID, code string) {
	errCode := code
	var impersonatingUID *string
	if uid != "" {
		u := uid
		impersonatingUID = &u
	}
	var impersonatedTenantID *string
	if customerTenantID != "" {
		t := customerTenantID
		impersonatedTenantID = &t
	}
	// tenant_id must be non-empty; fall back to integrator tenant or service
	// config integrator tenant.
	tenantID := customerTenantID
	if tenantID == "" {
		tenantID = integratorTenantID
	}
	if tenantID == "" {
		tenantID = s.cfg.IntegratorTenant.ID
	}
	// ImpersonatingUserID and ImpersonatedTenantID must be set together.
	if (impersonatingUID == nil) != (impersonatedTenantID == nil) {
		impersonatingUID = nil
		impersonatedTenantID = nil
	}
	actorUID := uid
	if actorUID == "" {
		actorUID = "unknown"
	}
	_ = s.auditRecorder.Record(ctx, audit.Entry{
		TenantID:             tenantID,
		ActorUserID:          actorUID,
		ActorAgent:           audit.AgentIntegrator,
		ImpersonatingUserID:  impersonatingUID,
		ImpersonatedTenantID: impersonatedTenantID,
		Action:               AuditActionCrossTenantVerify,
		ResourceType:         "scoped_token",
		ResourceID:           "",
		Result:               audit.ResultDeny,
		ErrorCode:            &errCode,
		Timestamp:            s.cfg.Now().UTC(),
	})
}

func defaultSessionID() string {
	var buf [16]byte
	if _, err := rand.Read(buf[:]); err != nil {
		// crypto/rand should never fail on a real host; but fail-closed
		// here means we return an obviously-garbage id that will fail the
		// sessionStore.Put uniqueness check.
		return "ERROR"
	}
	return hex.EncodeToString(buf[:])
}

// probeHierarchyDepth walks the parent chain manually to detect chains that
// would exceed maxHierarchyDepth. The permissions package silently caps at 32
// to prevent cycles — we want to surface the overflow as a hard error.
func probeHierarchyDepth(
	ctx context.Context,
	store permissions.IntegratorRelationshipStore,
	uid auth.UserID,
	tenant auth.TenantRef,
) (int, error) {
	rel, ok, err := store.LookupRelationship(ctx, uid, tenant)
	if err != nil || !ok {
		return 0, err
	}
	depth := 1
	cursor := rel.ParentIntegrator
	// Allow walking ONE past maxHierarchyDepth so the caller can detect
	// overflow (> maxHierarchyDepth).
	for cursor != "" && depth <= maxHierarchyDepth+1 {
		parent, ok, err := store.LookupParent(ctx, cursor)
		if err != nil {
			return depth, err
		}
		if !ok {
			break
		}
		depth++
		cursor = parent.ParentIntegrator
	}
	return depth, nil
}

func isDepthExceeded(err error) bool {
	// permissions.ResolveIntegratorScope currently does not return an error
	// for depth overflow (it silently stops at 32). Kept as a seam for
	// future work.
	return false
}

func timeFromClaim(mc jwt.MapClaims, key string) time.Time {
	switch v := mc[key].(type) {
	case float64:
		return time.Unix(int64(v), 0).UTC()
	case int64:
		return time.Unix(v, 0).UTC()
	}
	return time.Time{}
}
