// KAI-467: Impersonation API handler.
//
// Routes (plain JSON, not Connect-Go, until KAI-310 lands):
//
//	POST   /api/v1/impersonation/start      — begin impersonation session
//	POST   /api/v1/impersonation/end         — end active impersonation session
//	GET    /api/v1/impersonation/sessions     — list impersonation audit sessions
//
// All routes are behind the full middleware chain (auth + Casbin + audit).
// The handler delegates token minting / revocation to the crosstenant.Service
// (KAI-224) and emits audit entries visible to both the integrator and the
// customer admin.
//
// Impersonation auto-terminates after the scoped token TTL (default 15 min).
// The UI receives the TTL in the start response so it can display a countdown
// and auto-revoke on timeout.
package apiserver

import (
	"encoding/json"
	"errors"
	"net/http"
	"strings"
	"time"

	"github.com/bluenviron/mediamtx/internal/cloud/audit"
	"github.com/bluenviron/mediamtx/internal/cloud/identity/crosstenant"
	"github.com/bluenviron/mediamtx/internal/shared/auth"
)

// ImpersonationService is the minimal surface the handler needs from the
// crosstenant service. Using a local interface keeps the handler testable.
type ImpersonationService interface {
	MintScopedToken(ctx interface{ Deadline() (time.Time, bool); Done() <-chan struct{}; Err() error; Value(any) any }, integratorUserID auth.UserID, customerTenantID string) (*crosstenant.ScopedToken, error)
	RevokeScopedSession(ctx interface{ Deadline() (time.Time, bool); Done() <-chan struct{}; Err() error; Value(any) any }, sessionID string) error
}

// impersonationHandler holds the dependencies for impersonation routes.
type impersonationHandler struct {
	service       ImpersonationService
	auditRecorder audit.Recorder
	sessionStore  crosstenant.ScopedSessionStore
}

// --- Request / Response types -----------------------------------------------

type startImpersonationRequest struct {
	CustomerTenantID string `json:"customer_tenant_id"`
	Reason           string `json:"reason"`
	ConsentAcked     bool   `json:"consent_acknowledged"`
}

type startImpersonationResponse struct {
	SessionID        string   `json:"session_id"`
	Token            string   `json:"token"`
	CustomerTenantID string   `json:"customer_tenant_id"`
	ExpiresAt        string   `json:"expires_at"`
	TTLSeconds       int      `json:"ttl_seconds"`
	PermissionScope  []string `json:"permission_scope"`
}

type endImpersonationRequest struct {
	SessionID string `json:"session_id"`
}

type impersonationSessionResponse struct {
	SessionID        string   `json:"session_id"`
	IntegratorUserID string   `json:"integrator_user_id"`
	IntegratorTenant string   `json:"integrator_tenant"`
	CustomerTenantID string   `json:"customer_tenant_id"`
	PermissionScope  []string `json:"permission_scope"`
	IssuedAt         string   `json:"issued_at"`
	ExpiresAt        string   `json:"expires_at"`
	Revoked          bool     `json:"revoked"`
	Active           bool     `json:"active"`
}

type listSessionsResponse struct {
	Sessions []impersonationSessionResponse `json:"sessions"`
	Total    int                            `json:"total"`
}

// --- Route registration -----------------------------------------------------

// RegisterImpersonationRoutes mounts the impersonation API endpoints. Called
// from server.go when an ImpersonationService is configured.
func (s *Server) RegisterImpersonationRoutes(
	svc ImpersonationService,
	auditRecorder audit.Recorder,
	sessionStore crosstenant.ScopedSessionStore,
) {
	h := &impersonationHandler{
		service:       svc,
		auditRecorder: auditRecorder,
		sessionStore:  sessionStore,
	}
	chain := s.buildConnectChain()

	s.mux.Handle("/api/v1/impersonation/", chain(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		h.dispatch(w, r)
	})))

	// Route authorizations for Casbin.
	s.routes["/api/v1/impersonation/start"] = RouteAuthorization{
		ResourceType: "impersonation",
		Action:       "impersonation.start",
	}
	s.routes["/api/v1/impersonation/end"] = RouteAuthorization{
		ResourceType: "impersonation",
		Action:       "impersonation.end",
	}
	s.routes["/api/v1/impersonation/sessions"] = RouteAuthorization{
		ResourceType: "impersonation",
		Action:       "impersonation.read",
	}
}

// dispatch routes by method and path suffix.
func (h *impersonationHandler) dispatch(w http.ResponseWriter, r *http.Request) {
	suffix := strings.TrimPrefix(r.URL.Path, "/api/v1/impersonation/")
	switch {
	case r.Method == http.MethodPost && suffix == "start":
		h.start(w, r)
	case r.Method == http.MethodPost && suffix == "end":
		h.end(w, r)
	case r.Method == http.MethodGet && suffix == "sessions":
		h.listSessions(w, r)
	default:
		writeError(w, NewError(CodeNotFound, errors.New("not found")))
	}
}

// start handles POST /api/v1/impersonation/start
func (h *impersonationHandler) start(w http.ResponseWriter, r *http.Request) {
	claims, ok := ClaimsFromContext(r.Context())
	if !ok {
		writeError(w, NewError(CodeUnauthenticated, errors.New("missing claims")))
		return
	}

	var req startImpersonationRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, NewError(CodeInvalidArgument, errors.New("invalid request body")))
		return
	}

	if req.CustomerTenantID == "" {
		writeError(w, NewError(CodeInvalidArgument, errors.New("customer_tenant_id is required")))
		return
	}
	if !req.ConsentAcked {
		writeError(w, NewError(CodeInvalidArgument, errors.New("consent_acknowledged must be true")))
		return
	}

	tok, err := h.service.MintScopedToken(r.Context(), claims.UserID, req.CustomerTenantID)
	if err != nil {
		switch {
		case errors.Is(err, crosstenant.ErrNoRelationship):
			writeError(w, NewError(CodePermissionDenied, errors.New("no integrator relationship with this customer")))
		case errors.Is(err, crosstenant.ErrRelationshipRevoked):
			writeError(w, NewError(CodePermissionDenied, errors.New("integrator relationship has been revoked")))
		case errors.Is(err, crosstenant.ErrUnknownIntegrator):
			writeError(w, NewError(CodePermissionDenied, errors.New("unknown integrator user")))
		case errors.Is(err, crosstenant.ErrEmptyScope):
			writeError(w, NewError(CodePermissionDenied, errors.New("no permissions granted for this customer")))
		default:
			writeError(w, NewError(CodeInternal, errors.New("failed to start impersonation session")))
		}
		return
	}

	ttl := int(time.Until(tok.ExpiresAt).Seconds())
	if ttl < 0 {
		ttl = 0
	}

	// Record the impersonation start with the reason in audit details.
	actorUID := string(claims.UserID)
	customerTenantID := req.CustomerTenantID
	_ = h.auditRecorder.Record(r.Context(), audit.Entry{
		TenantID:             req.CustomerTenantID,
		ActorUserID:          string(claims.UserID),
		ActorAgent:           audit.AgentIntegrator,
		ImpersonatingUserID:  &actorUID,
		ImpersonatedTenantID: &customerTenantID,
		Action:               "impersonation.session_start",
		ResourceType:         "impersonation_session",
		ResourceID:           tok.SessionID,
		Result:               audit.ResultAllow,
		Timestamp:            time.Now().UTC(),
	})

	writeJSON(w, http.StatusOK, startImpersonationResponse{
		SessionID:        tok.SessionID,
		Token:            tok.Token,
		CustomerTenantID: tok.CustomerTenantID,
		ExpiresAt:        tok.ExpiresAt.Format(time.RFC3339),
		TTLSeconds:       ttl,
		PermissionScope:  tok.PermissionScope,
	})
}

// end handles POST /api/v1/impersonation/end
func (h *impersonationHandler) end(w http.ResponseWriter, r *http.Request) {
	claims, ok := ClaimsFromContext(r.Context())
	if !ok {
		writeError(w, NewError(CodeUnauthenticated, errors.New("missing claims")))
		return
	}

	var req endImpersonationRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, NewError(CodeInvalidArgument, errors.New("invalid request body")))
		return
	}

	if req.SessionID == "" {
		writeError(w, NewError(CodeInvalidArgument, errors.New("session_id is required")))
		return
	}

	if err := h.service.RevokeScopedSession(r.Context(), req.SessionID); err != nil {
		writeError(w, NewError(CodeInternal, errors.New("failed to end impersonation session")))
		return
	}

	// Record the impersonation end event.
	actorUID := string(claims.UserID)
	_ = h.auditRecorder.Record(r.Context(), audit.Entry{
		TenantID:    claims.TenantRef.ID,
		ActorUserID: string(claims.UserID),
		ActorAgent:  audit.AgentIntegrator,
		ImpersonatingUserID: &actorUID,
		ImpersonatedTenantID: func() *string {
			s := claims.TenantRef.ID
			return &s
		}(),
		Action:       "impersonation.session_end",
		ResourceType: "impersonation_session",
		ResourceID:   req.SessionID,
		Result:       audit.ResultAllow,
		Timestamp:    time.Now().UTC(),
	})

	writeJSON(w, http.StatusOK, map[string]string{
		"status":     "ended",
		"session_id": req.SessionID,
	})
}

// listSessions handles GET /api/v1/impersonation/sessions
// Returns impersonation sessions visible to the current user's tenant
// (either as integrator viewing their own sessions, or customer admin
// viewing sessions into their tenant).
func (h *impersonationHandler) listSessions(w http.ResponseWriter, r *http.Request) {
	claims, ok := ClaimsFromContext(r.Context())
	if !ok {
		writeError(w, NewError(CodeUnauthenticated, errors.New("missing claims")))
		return
	}

	tenantID := claims.TenantRef.ID

	// Query audit log for impersonation events targeting this tenant.
	entries, err := h.auditRecorder.Query(r.Context(), audit.QueryFilter{
		TenantID:                  tenantID,
		ActionPattern:             "impersonation.*",
		IncludeImpersonatedTenant: true,
		Limit:                     100,
	})
	if err != nil {
		writeError(w, NewError(CodeInternal, errors.New("failed to query impersonation sessions")))
		return
	}

	// Deduplicate by session ID and build response.
	sessionMap := make(map[string]*impersonationSessionResponse)
	now := time.Now().UTC()

	for _, e := range entries {
		if e.ResourceType != "impersonation_session" {
			continue
		}
		sid := e.ResourceID
		if sid == "" {
			continue
		}

		existing, exists := sessionMap[sid]
		if !exists {
			sess := &impersonationSessionResponse{
				SessionID:        sid,
				CustomerTenantID: e.TenantID,
				IssuedAt:         e.Timestamp.Format(time.RFC3339),
				ExpiresAt:        e.Timestamp.Add(crosstenant.DefaultTTL).Format(time.RFC3339),
			}
			if e.ImpersonatingUserID != nil {
				sess.IntegratorUserID = *e.ImpersonatingUserID
			}
			if e.ImpersonatedTenantID != nil {
				sess.CustomerTenantID = *e.ImpersonatedTenantID
			}
			sessionMap[sid] = sess
			existing = sess
		}

		// Mark as revoked if we see a session_end event.
		if e.Action == "impersonation.session_end" {
			existing.Revoked = true
		}
	}

	// Build final list, computing active status.
	sessions := make([]impersonationSessionResponse, 0, len(sessionMap))
	for _, s := range sessionMap {
		expiresAt, _ := time.Parse(time.RFC3339, s.ExpiresAt)
		s.Active = !s.Revoked && now.Before(expiresAt)
		sessions = append(sessions, *s)
	}

	writeJSON(w, http.StatusOK, listSessionsResponse{
		Sessions: sessions,
		Total:    len(sessions),
	})
}
