package impersonation

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"sort"
	"time"

	"github.com/bluenviron/mediamtx/internal/cloud/audit"
	"github.com/bluenviron/mediamtx/internal/cloud/identity/crosstenant"
	"github.com/bluenviron/mediamtx/internal/shared/auth"
	"github.com/bluenviron/mediamtx/internal/shared/permissions"
)

// Audit actions emitted by this package.
const (
	AuditActionImpersonationStart     = "impersonation.session.start"
	AuditActionImpersonationEnd       = "impersonation.session.end"
	AuditActionImpersonationAction    = "impersonation.session.action"
	AuditActionImpersonationGrantCreate = "impersonation.grant.create"
	AuditActionImpersonationGrantRevoke = "impersonation.grant.revoke"
)

// DefaultTimeout is the default impersonation session duration.
const DefaultTimeout = 30 * time.Minute

// DefaultGrantTTL is how long an authorization grant is valid before
// it must be consumed.
const DefaultGrantTTL = 1 * time.Hour

// AdminActions are actions that are NEVER permitted during impersonation.
// This is the security boundary: impersonators cannot escalate privileges
// or make destructive identity/billing changes.
var AdminActions = map[string]bool{
	permissions.ActionUsersCreate:      true,
	permissions.ActionUsersDelete:      true,
	permissions.ActionUsersImpersonate: true,
	permissions.ActionPermissionsGrant:  true,
	permissions.ActionPermissionsRevoke: true,
	permissions.ActionSettingsEdit:      true,
	permissions.ActionBillingChange:     true,
}

// Config configures a Service.
type Config struct {
	// SigningKey is used for grant token signing (HS256).
	SigningKey []byte

	// DefaultTimeout is the default session timeout. Zero means DefaultTimeout.
	DefaultSessionTimeout time.Duration

	// DefaultGrantTTL is the default grant validity. Zero means DefaultGrantTTL.
	DefaultGrantTTL time.Duration

	// Now is injectable for tests. Defaults to time.Now.
	Now func() time.Time

	// NewID generates unique identifiers. Injectable for tests.
	NewID func() string
}

// Service implements the impersonation workflow.
type Service struct {
	cfg Config

	identity          auth.IdentityProvider
	relationshipStore crosstenant.RelationshipStore
	permissionStore   permissions.IntegratorRelationshipStore
	sessionStore      SessionStore
	grantStore        GrantStore
	auditRecorder     audit.Recorder
	notifier          NotificationSender
}

// NewService constructs an impersonation service. All dependencies are required.
func NewService(
	cfg Config,
	identity auth.IdentityProvider,
	relationshipStore crosstenant.RelationshipStore,
	permissionStore permissions.IntegratorRelationshipStore,
	sessionStore SessionStore,
	grantStore GrantStore,
	auditRecorder audit.Recorder,
	notifier NotificationSender,
) (*Service, error) {
	if len(cfg.SigningKey) == 0 {
		return nil, errors.New("impersonation: signing key is required")
	}
	if identity == nil {
		return nil, errors.New("impersonation: identity provider is required")
	}
	if relationshipStore == nil {
		return nil, errors.New("impersonation: relationship store is required")
	}
	if permissionStore == nil {
		return nil, errors.New("impersonation: permission store is required")
	}
	if sessionStore == nil {
		return nil, errors.New("impersonation: session store is required")
	}
	if grantStore == nil {
		return nil, errors.New("impersonation: grant store is required")
	}
	if auditRecorder == nil {
		return nil, errors.New("impersonation: audit recorder is required")
	}
	if notifier == nil {
		return nil, errors.New("impersonation: notification sender is required")
	}
	if cfg.DefaultSessionTimeout <= 0 {
		cfg.DefaultSessionTimeout = DefaultTimeout
	}
	if cfg.DefaultGrantTTL <= 0 {
		cfg.DefaultGrantTTL = DefaultGrantTTL
	}
	if cfg.Now == nil {
		cfg.Now = time.Now
	}
	if cfg.NewID == nil {
		cfg.NewID = defaultID
	}
	return &Service{
		cfg:               cfg,
		identity:          identity,
		relationshipStore: relationshipStore,
		permissionStore:   permissionStore,
		sessionStore:      sessionStore,
		grantStore:        grantStore,
		auditRecorder:     auditRecorder,
		notifier:          notifier,
	}, nil
}

// CreateAuthorizationGrant creates a time-limited grant that a platform support
// agent can use to begin an impersonation session. Only customer admins can
// create grants.
func (s *Service) CreateAuthorizationGrant(ctx context.Context, req CreateGrantRequest) (*AuthorizationGrant, error) {
	if req.TenantID == "" {
		return nil, errors.New("impersonation: tenant_id is required")
	}
	if req.GrantedByUserID == "" {
		return nil, errors.New("impersonation: granted_by_user_id is required")
	}

	now := s.cfg.Now().UTC()

	maxDuration := req.MaxDuration
	if maxDuration <= 0 {
		maxDuration = s.cfg.DefaultSessionTimeout
	}

	grantTTL := req.GrantTTL
	if grantTTL <= 0 {
		grantTTL = s.cfg.DefaultGrantTTL
	}

	grant := AuthorizationGrant{
		GrantID:         s.cfg.NewID(),
		TenantID:        req.TenantID,
		GrantedByUserID: req.GrantedByUserID,
		Reason:          req.Reason,
		MaxDuration:     maxDuration,
		CreatedAt:       now,
		ExpiresAt:       now.Add(grantTTL),
	}

	if err := s.grantStore.PutGrant(ctx, grant); err != nil {
		return nil, fmt.Errorf("impersonation: persist grant: %w", err)
	}

	// Audit the grant creation.
	if err := s.auditRecorder.Record(ctx, audit.Entry{
		TenantID:    req.TenantID,
		ActorUserID: req.GrantedByUserID,
		ActorAgent:  audit.AgentCloud,
		Action:      AuditActionImpersonationGrantCreate,
		ResourceType: "impersonation_grant",
		ResourceID:  grant.GrantID,
		Result:      audit.ResultAllow,
		Timestamp:   now,
	}); err != nil {
		return nil, fmt.Errorf("impersonation: record audit: %w", err)
	}

	return &grant, nil
}

// RevokeAuthorizationGrant revokes an authorization grant so it cannot be used
// to start an impersonation session. Idempotent for already-revoked grants.
func (s *Service) RevokeAuthorizationGrant(ctx context.Context, grantID, revokedByUserID string) error {
	grant, ok, err := s.grantStore.GetGrant(ctx, grantID)
	if err != nil {
		return fmt.Errorf("impersonation: get grant: %w", err)
	}
	if !ok {
		return ErrGrantNotFound
	}

	if grant.Revoked {
		return nil // idempotent
	}

	grant.Revoked = true
	if err := s.grantStore.UpdateGrant(ctx, grant); err != nil {
		return fmt.Errorf("impersonation: update grant: %w", err)
	}

	now := s.cfg.Now().UTC()
	if err := s.auditRecorder.Record(ctx, audit.Entry{
		TenantID:    grant.TenantID,
		ActorUserID: revokedByUserID,
		ActorAgent:  audit.AgentCloud,
		Action:      AuditActionImpersonationGrantRevoke,
		ResourceType: "impersonation_grant",
		ResourceID:  grant.GrantID,
		Result:      audit.ResultAllow,
		Timestamp:   now,
	}); err != nil {
		return fmt.Errorf("impersonation: record audit: %w", err)
	}

	return nil
}

// CreateSession starts a new impersonation session. For integrator mode, it
// validates the integrator-customer relationship and derives permissions.
// For platform support mode, it validates and consumes the authorization grant.
func (s *Service) CreateSession(ctx context.Context, req CreateSessionRequest) (*Session, error) {
	if !req.Mode.Valid() {
		return nil, ErrInvalidMode
	}
	if req.ImpersonatingUserID == "" {
		return nil, errors.New("impersonation: impersonating_user_id is required")
	}
	if req.ImpersonatingTenantID == "" {
		return nil, errors.New("impersonation: impersonating_tenant_id is required")
	}
	if req.ImpersonatedTenantID == "" {
		return nil, errors.New("impersonation: impersonated_tenant_id is required")
	}

	now := s.cfg.Now().UTC()
	var scopedPermissions []string

	switch req.Mode {
	case ModeIntegrator:
		perms, err := s.resolveIntegratorPermissions(ctx, req, now)
		if err != nil {
			return nil, err
		}
		scopedPermissions = perms

	case ModePlatformSupport:
		if req.AuthorizationGrantID == "" {
			s.recordSessionFailure(ctx, req, now, "missing_grant")
			return nil, ErrMissingGrant
		}
		perms, err := s.resolvePlatformSupportPermissions(ctx, req, now)
		if err != nil {
			return nil, err
		}
		scopedPermissions = perms
	}

	// Strip admin actions from the scope.
	scopedPermissions = stripAdminActions(scopedPermissions)

	if len(scopedPermissions) == 0 {
		s.recordSessionFailure(ctx, req, now, "empty_scope")
		return nil, errors.New("impersonation: resolved permission scope is empty after stripping admin actions")
	}

	sort.Strings(scopedPermissions)

	timeout := req.Timeout
	if timeout <= 0 {
		timeout = s.cfg.DefaultSessionTimeout
	}

	session := Session{
		SessionID:             s.cfg.NewID(),
		Mode:                  req.Mode,
		ImpersonatingUserID:   req.ImpersonatingUserID,
		ImpersonatingTenantID: req.ImpersonatingTenantID,
		ImpersonatedTenantID:  req.ImpersonatedTenantID,
		ScopedPermissions:     scopedPermissions,
		AuthorizationToken:    req.AuthorizationGrantID,
		Status:                StatusActive,
		CreatedAt:             now,
		ExpiresAt:             now.Add(timeout),
	}

	if err := s.sessionStore.PutSession(ctx, session); err != nil {
		return nil, fmt.Errorf("impersonation: persist session: %w", err)
	}

	// Audit the session start.
	impersonatingUID := req.ImpersonatingUserID
	impersonatedTenantID := req.ImpersonatedTenantID
	if err := s.auditRecorder.Record(ctx, audit.Entry{
		TenantID:             req.ImpersonatedTenantID,
		ActorUserID:          req.ImpersonatingUserID,
		ActorAgent:           agentForMode(req.Mode),
		ImpersonatingUserID:  &impersonatingUID,
		ImpersonatedTenantID: &impersonatedTenantID,
		Action:               AuditActionImpersonationStart,
		ResourceType:         "impersonation_session",
		ResourceID:           session.SessionID,
		Result:               audit.ResultAllow,
		Timestamp:            now,
	}); err != nil {
		return nil, fmt.Errorf("impersonation: record audit: %w", err)
	}

	// Notify customer admin.
	if err := s.notifier.NotifyImpersonationStart(ctx, session); err != nil {
		// Log but don't fail the session creation; the audit entry is the
		// authoritative record.
		_ = err
	}

	return &session, nil
}

// ValidateSession checks that a session is still valid and returns it.
// Returns ErrSessionNotFound if the session does not exist, and
// ErrSessionExpired if the session has passed its expiry.
func (s *Service) ValidateSession(ctx context.Context, sessionID string) (*Session, error) {
	sess, ok, err := s.sessionStore.GetSession(ctx, sessionID)
	if err != nil {
		return nil, fmt.Errorf("impersonation: get session: %w", err)
	}
	if !ok {
		return nil, ErrSessionNotFound
	}

	now := s.cfg.Now().UTC()

	if sess.Status != StatusActive {
		return nil, ErrSessionAlreadyTerminated
	}

	if sess.IsExpired(now) {
		// Auto-terminate expired sessions.
		sess.Status = StatusExpired
		terminatedAt := now
		sess.TerminatedAt = &terminatedAt
		_ = s.sessionStore.UpdateSession(ctx, sess)

		// Audit the auto-termination.
		impersonatingUID := sess.ImpersonatingUserID
		impersonatedTenantID := sess.ImpersonatedTenantID
		_ = s.auditRecorder.Record(ctx, audit.Entry{
			TenantID:             sess.ImpersonatedTenantID,
			ActorUserID:          sess.ImpersonatingUserID,
			ActorAgent:           agentForMode(sess.Mode),
			ImpersonatingUserID:  &impersonatingUID,
			ImpersonatedTenantID: &impersonatedTenantID,
			Action:               AuditActionImpersonationEnd,
			ResourceType:         "impersonation_session",
			ResourceID:           sess.SessionID,
			Result:               audit.ResultAllow,
			Timestamp:            now,
		})

		// Notify customer.
		_ = s.notifier.NotifyImpersonationEnd(ctx, sess)

		return nil, ErrSessionExpired
	}

	return &sess, nil
}

// RecordAction records an audited action performed during an impersonation
// session. It validates that the action is in the session's scoped permissions
// and is not an admin action.
func (s *Service) RecordAction(ctx context.Context, sessionID, action, resourceType, resourceID string) error {
	sess, err := s.ValidateSession(ctx, sessionID)
	if err != nil {
		return err
	}

	// Check the action is in scope.
	if AdminActions[action] {
		now := s.cfg.Now().UTC()
		impersonatingUID := sess.ImpersonatingUserID
		impersonatedTenantID := sess.ImpersonatedTenantID
		errCode := "admin_action_blocked"
		_ = s.auditRecorder.Record(ctx, audit.Entry{
			TenantID:             sess.ImpersonatedTenantID,
			ActorUserID:          sess.ImpersonatingUserID,
			ActorAgent:           agentForMode(sess.Mode),
			ImpersonatingUserID:  &impersonatingUID,
			ImpersonatedTenantID: &impersonatedTenantID,
			Action:               action,
			ResourceType:         resourceType,
			ResourceID:           resourceID,
			Result:               audit.ResultDeny,
			ErrorCode:            &errCode,
			Timestamp:            now,
		})
		return ErrAdminActionBlocked
	}

	if !isActionInScope(action, sess.ScopedPermissions) {
		now := s.cfg.Now().UTC()
		impersonatingUID := sess.ImpersonatingUserID
		impersonatedTenantID := sess.ImpersonatedTenantID
		errCode := "action_not_in_scope"
		_ = s.auditRecorder.Record(ctx, audit.Entry{
			TenantID:             sess.ImpersonatedTenantID,
			ActorUserID:          sess.ImpersonatingUserID,
			ActorAgent:           agentForMode(sess.Mode),
			ImpersonatingUserID:  &impersonatingUID,
			ImpersonatedTenantID: &impersonatedTenantID,
			Action:               action,
			ResourceType:         resourceType,
			ResourceID:           resourceID,
			Result:               audit.ResultDeny,
			ErrorCode:            &errCode,
			Timestamp:            now,
		})
		return ErrUnauthorized
	}

	// Record the action.
	now := s.cfg.Now().UTC()
	impersonatingUID := sess.ImpersonatingUserID
	impersonatedTenantID := sess.ImpersonatedTenantID
	if err := s.auditRecorder.Record(ctx, audit.Entry{
		TenantID:             sess.ImpersonatedTenantID,
		ActorUserID:          sess.ImpersonatingUserID,
		ActorAgent:           agentForMode(sess.Mode),
		ImpersonatingUserID:  &impersonatingUID,
		ImpersonatedTenantID: &impersonatedTenantID,
		Action:               action,
		ResourceType:         resourceType,
		ResourceID:           resourceID,
		Result:               audit.ResultAllow,
		Timestamp:            now,
	}); err != nil {
		return fmt.Errorf("impersonation: record audit: %w", err)
	}

	return nil
}

// TerminateSession explicitly terminates an active impersonation session.
func (s *Service) TerminateSession(ctx context.Context, sessionID, terminatedByUserID string) error {
	sess, ok, err := s.sessionStore.GetSession(ctx, sessionID)
	if err != nil {
		return fmt.Errorf("impersonation: get session: %w", err)
	}
	if !ok {
		return ErrSessionNotFound
	}

	if sess.Status != StatusActive {
		return ErrSessionAlreadyTerminated
	}

	now := s.cfg.Now().UTC()
	sess.Status = StatusTerminated
	sess.TerminatedAt = &now
	sess.TerminatedBy = terminatedByUserID

	if err := s.sessionStore.UpdateSession(ctx, sess); err != nil {
		return fmt.Errorf("impersonation: update session: %w", err)
	}

	// Audit the termination.
	impersonatingUID := sess.ImpersonatingUserID
	impersonatedTenantID := sess.ImpersonatedTenantID
	if err := s.auditRecorder.Record(ctx, audit.Entry{
		TenantID:             sess.ImpersonatedTenantID,
		ActorUserID:          terminatedByUserID,
		ActorAgent:           agentForMode(sess.Mode),
		ImpersonatingUserID:  &impersonatingUID,
		ImpersonatedTenantID: &impersonatedTenantID,
		Action:               AuditActionImpersonationEnd,
		ResourceType:         "impersonation_session",
		ResourceID:           sess.SessionID,
		Result:               audit.ResultAllow,
		Timestamp:            now,
	}); err != nil {
		return fmt.Errorf("impersonation: record audit: %w", err)
	}

	// Notify customer admin.
	if err := s.notifier.NotifyImpersonationEnd(ctx, sess); err != nil {
		_ = err
	}

	return nil
}

// ListActiveSessions returns all active impersonation sessions for a tenant.
// Customer admins use this to see who is impersonating their tenant.
func (s *Service) ListActiveSessions(ctx context.Context, tenantID string) ([]Session, error) {
	sessions, err := s.sessionStore.ListActiveSessions(ctx, tenantID)
	if err != nil {
		return nil, fmt.Errorf("impersonation: list sessions: %w", err)
	}

	// Filter out expired sessions.
	now := s.cfg.Now().UTC()
	var active []Session
	for _, sess := range sessions {
		if sess.IsActive(now) {
			active = append(active, sess)
		}
	}
	return active, nil
}

// --- internal helpers -------------------------------------------------------

func (s *Service) resolveIntegratorPermissions(ctx context.Context, req CreateSessionRequest, now time.Time) ([]string, error) {
	// Verify the relationship exists and is not revoked.
	rel, ok, err := s.relationshipStore.Lookup(ctx, req.ImpersonatingUserID, req.ImpersonatedTenantID)
	if err != nil {
		s.recordSessionFailure(ctx, req, now, "relationship_lookup_error")
		return nil, fmt.Errorf("impersonation: relationship lookup: %w", err)
	}
	if !ok {
		s.recordSessionFailure(ctx, req, now, "no_relationship")
		return nil, ErrNoRelationship
	}
	if rel.Revoked {
		s.recordSessionFailure(ctx, req, now, "relationship_revoked")
		return nil, ErrRelationshipRevoked
	}

	// Resolve the intersected scope.
	customerTenant := auth.TenantRef{Type: auth.TenantTypeCustomer, ID: req.ImpersonatedTenantID}
	allowed, found, err := permissions.ResolveIntegratorScope(
		ctx, s.permissionStore, auth.UserID(req.ImpersonatingUserID), customerTenant,
	)
	if err != nil {
		s.recordSessionFailure(ctx, req, now, "scope_resolve_error")
		return nil, fmt.Errorf("impersonation: resolve scope: %w", err)
	}
	if !found {
		s.recordSessionFailure(ctx, req, now, "permissions_no_relationship")
		return nil, ErrNoRelationship
	}

	return allowed, nil
}

func (s *Service) resolvePlatformSupportPermissions(ctx context.Context, req CreateSessionRequest, now time.Time) ([]string, error) {
	// Validate and consume the authorization grant.
	grant, ok, err := s.grantStore.GetGrant(ctx, req.AuthorizationGrantID)
	if err != nil {
		s.recordSessionFailure(ctx, req, now, "grant_lookup_error")
		return nil, fmt.Errorf("impersonation: get grant: %w", err)
	}
	if !ok {
		s.recordSessionFailure(ctx, req, now, "grant_not_found")
		return nil, ErrGrantNotFound
	}

	// The grant must be for the correct tenant.
	if grant.TenantID != req.ImpersonatedTenantID {
		s.recordSessionFailure(ctx, req, now, "grant_tenant_mismatch")
		return nil, ErrUnauthorized
	}

	if grant.Revoked {
		s.recordSessionFailure(ctx, req, now, "grant_revoked")
		return nil, ErrGrantRevoked
	}
	if grant.Consumed {
		s.recordSessionFailure(ctx, req, now, "grant_consumed")
		return nil, ErrGrantConsumed
	}
	if now.After(grant.ExpiresAt) {
		s.recordSessionFailure(ctx, req, now, "grant_expired")
		return nil, ErrGrantExpired
	}

	// Consume the grant.
	grant.Consumed = true
	consumedAt := now
	grant.ConsumedAt = &consumedAt
	if err := s.grantStore.UpdateGrant(ctx, grant); err != nil {
		return nil, fmt.Errorf("impersonation: consume grant: %w", err)
	}

	// Cap session timeout to grant's MaxDuration.
	if req.Timeout <= 0 || req.Timeout > grant.MaxDuration {
		req.Timeout = grant.MaxDuration
	}

	// Platform support gets a broad read-only scope (no admin actions).
	// The full set minus admin actions will be applied by the caller.
	return defaultPlatformSupportScope(), nil
}

// defaultPlatformSupportScope returns the default permission set for platform
// support impersonation. This is a read-heavy, non-destructive set.
func defaultPlatformSupportScope() []string {
	return []string{
		permissions.ActionViewThumbnails,
		permissions.ActionViewLive,
		permissions.ActionViewPlayback,
		permissions.ActionViewSnapshot,
		permissions.ActionCamerasEdit,
		permissions.ActionUsersView,
		permissions.ActionAuditRead,
		permissions.ActionSystemHealth,
		permissions.ActionBillingView,
		permissions.ActionRelationshipsRead,
		permissions.ActionBehavioralConfigRead,
	}
}

func (s *Service) recordSessionFailure(ctx context.Context, req CreateSessionRequest, now time.Time, code string) {
	errCode := code
	impersonatingUID := req.ImpersonatingUserID
	impersonatedTenantID := req.ImpersonatedTenantID

	tenantID := req.ImpersonatedTenantID
	if tenantID == "" {
		tenantID = req.ImpersonatingTenantID
	}

	actorUID := req.ImpersonatingUserID
	if actorUID == "" {
		actorUID = "unknown"
	}

	// ImpersonatingUserID and ImpersonatedTenantID must be set together.
	var impUID *string
	var impTID *string
	if impersonatingUID != "" && impersonatedTenantID != "" {
		impUID = &impersonatingUID
		impTID = &impersonatedTenantID
	}

	_ = s.auditRecorder.Record(ctx, audit.Entry{
		TenantID:             tenantID,
		ActorUserID:          actorUID,
		ActorAgent:           agentForMode(req.Mode),
		ImpersonatingUserID:  impUID,
		ImpersonatedTenantID: impTID,
		Action:               AuditActionImpersonationStart,
		ResourceType:         "impersonation_session",
		ResourceID:           "",
		Result:               audit.ResultDeny,
		ErrorCode:            &errCode,
		Timestamp:            now,
	})
}

func stripAdminActions(actions []string) []string {
	var out []string
	for _, a := range actions {
		if !AdminActions[a] {
			out = append(out, a)
		}
	}
	return out
}

func isActionInScope(action string, scope []string) bool {
	for _, a := range scope {
		if a == action {
			return true
		}
	}
	return false
}

func agentForMode(mode ImpersonationMode) audit.ActorAgent {
	switch mode {
	case ModeIntegrator:
		return audit.AgentIntegrator
	case ModePlatformSupport:
		return audit.AgentCloud
	default:
		return audit.AgentCloud
	}
}

func defaultID() string {
	var buf [16]byte
	if _, err := rand.Read(buf[:]); err != nil {
		return "ERROR"
	}
	return hex.EncodeToString(buf[:])
}
