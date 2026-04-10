// Package authapi provides HTTP handlers for the Directory's authentication
// endpoints: login, refresh, and logout. These handlers delegate to the
// IdentityProvider interface (internal/shared/auth) and are pure on-prem —
// no Zitadel or cloud dependency.
package authapi

import (
	"context"
	"encoding/json"
	"log/slog"
	"net/http"
	"time"
)

// AuthProvider is the subset of auth.IdentityProvider that this package needs.
// Using a narrow interface avoids importing the full provider in tests.
type AuthProvider interface {
	AuthenticateLocal(ctx context.Context, tenant TenantRef, username, password string) (*Session, error)
	RefreshSession(ctx context.Context, refreshToken string) (*Session, error)
	RevokeSession(ctx context.Context, sessionID SessionID) error
}

// TenantRef identifies a tenant. Mirrors auth.TenantRef.
type TenantRef struct {
	Type string
	ID   string
}

// Session is the result of a successful login or refresh.
type Session struct {
	ID           SessionID
	UserID       string
	AccessToken  string
	RefreshToken string
	IDToken      string
	ExpiresAt    time.Time
}

// SessionID is a unique session identifier.
type SessionID string

// TenantResolver extracts the tenant from the request. For on-prem single-tenant
// deployments this returns the default tenant; for multi-tenant it reads a header
// or subdomain.
type TenantResolver func(r *http.Request) TenantRef

// SessionFromCtx extracts the session ID from an authenticated request context.
// Used by the logout handler.
type SessionFromCtx func(ctx context.Context) (SessionID, bool)

// Handlers holds the HTTP handlers for the auth API.
type Handlers struct {
	provider  AuthProvider
	tenant    TenantResolver
	sessionFn SessionFromCtx
	log       *slog.Logger
}

// NewHandlers creates auth API handlers backed by the given provider.
func NewHandlers(provider AuthProvider, tenant TenantResolver, sessionFn SessionFromCtx, log *slog.Logger) *Handlers {
	if log == nil {
		log = slog.Default()
	}
	return &Handlers{
		provider:  provider,
		tenant:    tenant,
		sessionFn: sessionFn,
		log:       log,
	}
}

// --- Request/Response types ---

// LoginRequest is the JSON body for POST /api/v1/auth/login.
type LoginRequest struct {
	Username string `json:"username"`
	Password string `json:"password"`
}

// LoginResponse is the JSON body returned by successful login or refresh.
type LoginResponse struct {
	AccessToken  string `json:"access_token"`
	RefreshToken string `json:"refresh_token"`
	IDToken      string `json:"id_token,omitempty"`
	ExpiresAt    string `json:"expires_at"`
	SessionID    string `json:"session_id"`
}

// RefreshRequest is the JSON body for POST /api/v1/auth/refresh.
type RefreshRequest struct {
	RefreshToken string `json:"refresh_token"`
}

// --- Handlers ---

// Login returns an http.HandlerFunc for POST /api/v1/auth/login.
func (h *Handlers) Login() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			writeError(w, http.StatusMethodNotAllowed, "METHOD_NOT_ALLOWED", "use POST")
			return
		}

		var req LoginRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			writeError(w, http.StatusBadRequest, "BAD_REQUEST", "invalid JSON body")
			return
		}
		if req.Username == "" || req.Password == "" {
			writeError(w, http.StatusBadRequest, "BAD_REQUEST", "username and password are required")
			return
		}

		tenant := h.tenant(r)
		session, err := h.provider.AuthenticateLocal(r.Context(), tenant, req.Username, req.Password)
		if err != nil {
			// Do not distinguish "user not found" from "wrong password".
			h.log.Debug("auth: login failed", "username", req.Username, "error", err)
			writeError(w, http.StatusUnauthorized, "UNAUTHORIZED", "invalid credentials")
			return
		}

		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_ = json.NewEncoder(w).Encode(sessionToResponse(session))
	}
}

// Refresh returns an http.HandlerFunc for POST /api/v1/auth/refresh.
func (h *Handlers) Refresh() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			writeError(w, http.StatusMethodNotAllowed, "METHOD_NOT_ALLOWED", "use POST")
			return
		}

		var req RefreshRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			writeError(w, http.StatusBadRequest, "BAD_REQUEST", "invalid JSON body")
			return
		}
		if req.RefreshToken == "" {
			writeError(w, http.StatusBadRequest, "BAD_REQUEST", "refresh_token is required")
			return
		}

		session, err := h.provider.RefreshSession(r.Context(), req.RefreshToken)
		if err != nil {
			h.log.Debug("auth: refresh failed", "error", err)
			writeError(w, http.StatusUnauthorized, "UNAUTHORIZED", "invalid or expired refresh token")
			return
		}

		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_ = json.NewEncoder(w).Encode(sessionToResponse(session))
	}
}

// Logout returns an http.HandlerFunc for POST /api/v1/auth/logout.
// Requires an authenticated request (session ID in context).
func (h *Handlers) Logout() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			writeError(w, http.StatusMethodNotAllowed, "METHOD_NOT_ALLOWED", "use POST")
			return
		}

		sid, ok := h.sessionFn(r.Context())
		if !ok || sid == "" {
			writeError(w, http.StatusUnauthorized, "UNAUTHENTICATED", "no session in context")
			return
		}

		if err := h.provider.RevokeSession(r.Context(), sid); err != nil {
			h.log.Error("auth: logout revoke failed", "session_id", string(sid), "error", err)
			writeError(w, http.StatusInternalServerError, "INTERNAL", "failed to revoke session")
			return
		}

		w.WriteHeader(http.StatusNoContent)
	}
}

func sessionToResponse(s *Session) LoginResponse {
	return LoginResponse{
		AccessToken:  s.AccessToken,
		RefreshToken: s.RefreshToken,
		IDToken:      s.IDToken,
		ExpiresAt:    s.ExpiresAt.Format(time.RFC3339),
		SessionID:    string(s.ID),
	}
}

func writeError(w http.ResponseWriter, status int, code, message string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(map[string]string{"code": code, "message": message})
}
