package apiserver

// KAI-319: API Key management CRUD handler.
//
// Routes (plain JSON, not Connect-Go, until KAI-310 lands):
//
//	GET    /api/v1/api-keys            — list keys for the caller's tenant
//	POST   /api/v1/api-keys            — create a new key (returns one-time raw key)
//	POST   /api/v1/api-keys/{id}/rotate — rotate an existing key
//	POST   /api/v1/api-keys/{id}/revoke — revoke an existing key
//	GET    /api/v1/api-keys/{id}/audit  — per-key audit log
//
// All routes are behind the full middleware chain (auth + Casbin + audit).
// Casbin enforcement uses:
//   - read  → "apikeys.read"
//   - write → "apikeys.write"
//
// Tenant isolation: every handler reads tenantID from the verified auth
// claims (never from a URL param) so it is impossible for a caller to
// access another tenant's keys.

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"
)

// APIKeyStore is the minimal store seam the handler needs.
// Production wires this to internal/cloud/publicapi/apikeys.Store (KAI-400).
type APIKeyStore interface {
	Create(ctx context.Context, req CreateAPIKeyReq) (*CreateAPIKeyRes, error)
	List(ctx context.Context, tenantID string, includeRevoked bool) ([]*APIKeyInfo, error)
	Rotate(ctx context.Context, keyID string, rotatedBy string, graceHours int) (*RotateAPIKeyRes, error)
	Revoke(ctx context.Context, keyID string, revokedBy string) error
	ListAuditLog(ctx context.Context, tenantID, keyID string, limit int) ([]APIKeyAuditEntry, error)
}

// CreateAPIKeyReq is the request body for key creation.
type CreateAPIKeyReq struct {
	TenantID     string   `json:"tenant_id"`
	Name         string   `json:"name"`
	Scopes       []string `json:"scopes"`
	ExpiresInDays *int    `json:"expires_in_days"`
	CreatedBy    string   `json:"created_by"`
}

// CreateAPIKeyRes is the response for key creation.
type CreateAPIKeyRes struct {
	RawKey string      `json:"raw_key"`
	Key    *APIKeyInfo `json:"key"`
}

// RotateAPIKeyRes is the response for key rotation.
type RotateAPIKeyRes struct {
	RawKey          string      `json:"raw_key"`
	NewKey          *APIKeyInfo `json:"new_key"`
	OldKeyGraceEnd  string      `json:"old_key_grace_end"`
}

// APIKeyInfo is the JSON representation of a key (never includes the raw key).
type APIKeyInfo struct {
	ID             string   `json:"id"`
	TenantID       string   `json:"tenant_id"`
	Name           string   `json:"name"`
	KeyPrefix      string   `json:"key_prefix"`
	Scopes         []string `json:"scopes"`
	Status         string   `json:"status"`
	CreatedBy      string   `json:"created_by"`
	CreatedAt      string   `json:"created_at"`
	ExpiresAt      *string  `json:"expires_at"`
	RevokedAt      *string  `json:"revoked_at"`
	LastUsedAt     *string  `json:"last_used_at"`
	RotatedFromID  *string  `json:"rotated_from_id"`
	GraceExpiresAt *string  `json:"grace_expires_at"`
}

// APIKeyAuditEntry is a single audit log row.
type APIKeyAuditEntry struct {
	ID        string `json:"id"`
	KeyID     string `json:"key_id"`
	Action    string `json:"action"`
	ActorID   string `json:"actor_id"`
	IPAddress string `json:"ip_address"`
	UserAgent string `json:"user_agent"`
	Metadata  string `json:"metadata"`
	CreatedAt string `json:"created_at"`
}

// apiKeysHandler holds the store reference.
type apiKeysHandler struct {
	store APIKeyStore
}

// RegisterAPIKeysRoutes mounts the API key management endpoints on mux
// using the provided store and connects them through the full middleware chain.
func (s *Server) RegisterAPIKeysRoutes(store APIKeyStore) {
	h := &apiKeysHandler{store: store}
	chain := s.buildConnectChain()

	// List + Create share the same path but differ by method.
	s.mux.Handle("/api/v1/api-keys", chain(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.Method {
		case http.MethodGet:
			h.handleList(w, r)
		case http.MethodPost:
			h.handleCreate(w, r)
		default:
			w.WriteHeader(http.StatusMethodNotAllowed)
		}
	})))

	// Rotate, Revoke, Audit are sub-resources under a key ID.
	s.mux.Handle("/api/v1/api-keys/", chain(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		path := strings.TrimPrefix(r.URL.Path, "/api/v1/api-keys/")
		parts := strings.SplitN(path, "/", 2)
		if len(parts) < 2 {
			w.WriteHeader(http.StatusNotFound)
			return
		}
		keyID := parts[0]
		action := parts[1]

		switch {
		case action == "rotate" && r.Method == http.MethodPost:
			h.handleRotate(w, r, keyID)
		case action == "revoke" && r.Method == http.MethodPost:
			h.handleRevoke(w, r, keyID)
		case action == "audit" && r.Method == http.MethodGet:
			h.handleAudit(w, r, keyID)
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	})))

	// Route authorizations for Casbin enforcement.
	s.routes["POST /api/v1/api-keys"] = RouteAuthorization{Action: "apikeys.write"}
	s.routes["GET /api/v1/api-keys"] = RouteAuthorization{Action: "apikeys.read"}
	s.routes["POST /api/v1/api-keys/*/rotate"] = RouteAuthorization{Action: "apikeys.write"}
	s.routes["POST /api/v1/api-keys/*/revoke"] = RouteAuthorization{Action: "apikeys.write"}
	s.routes["GET /api/v1/api-keys/*/audit"] = RouteAuthorization{Action: "apikeys.read"}
}

// handleList returns all keys for the authenticated tenant.
func (h *apiKeysHandler) handleList(w http.ResponseWriter, r *http.Request) {
	claims, ok := ClaimsFromContext(r.Context())
	if !ok {
		writeJSON(w, http.StatusUnauthorized, map[string]string{"message": "unauthorized"})
		return
	}

	includeRevoked := r.URL.Query().Get("include_revoked") == "true"

	keys, err := h.store.List(r.Context(), claims.TenantRef.ID, includeRevoked)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"message": "failed to list keys"})
		return
	}

	writeJSON(w, http.StatusOK, map[string]any{"keys": keys})
}

// handleCreate creates a new API key.
func (h *apiKeysHandler) handleCreate(w http.ResponseWriter, r *http.Request) {
	claims, ok := ClaimsFromContext(r.Context())
	if !ok {
		writeJSON(w, http.StatusUnauthorized, map[string]string{"message": "unauthorized"})
		return
	}

	var body struct {
		Name          string   `json:"name"`
		Scopes        []string `json:"scopes"`
		ExpiresInDays *int     `json:"expires_in_days"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"message": "invalid request body"})
		return
	}

	if body.Name == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"message": "name is required"})
		return
	}

	req := CreateAPIKeyReq{
		TenantID:      claims.TenantRef.ID,
		Name:          body.Name,
		Scopes:        body.Scopes,
		ExpiresInDays: body.ExpiresInDays,
		CreatedBy:     string(claims.UserID),
	}

	result, err := h.store.Create(r.Context(), req)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"message": "failed to create key"})
		return
	}

	writeJSON(w, http.StatusCreated, result)
}

// handleRotate rotates an existing key.
func (h *apiKeysHandler) handleRotate(w http.ResponseWriter, r *http.Request, keyID string) {
	claims, ok := ClaimsFromContext(r.Context())
	if !ok {
		writeJSON(w, http.StatusUnauthorized, map[string]string{"message": "unauthorized"})
		return
	}

	var body struct {
		GracePeriodHours int `json:"grace_period_hours"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"message": "invalid request body"})
		return
	}

	if body.GracePeriodHours <= 0 {
		body.GracePeriodHours = 24
	}

	result, err := h.store.Rotate(r.Context(), keyID, string(claims.UserID), body.GracePeriodHours)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"message": "failed to rotate key"})
		return
	}

	writeJSON(w, http.StatusOK, result)
}

// handleRevoke revokes a key immediately.
func (h *apiKeysHandler) handleRevoke(w http.ResponseWriter, r *http.Request, keyID string) {
	claims, ok := ClaimsFromContext(r.Context())
	if !ok {
		writeJSON(w, http.StatusUnauthorized, map[string]string{"message": "unauthorized"})
		return
	}

	if err := h.store.Revoke(r.Context(), keyID, string(claims.UserID)); err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"message": "failed to revoke key"})
		return
	}

	w.WriteHeader(http.StatusNoContent)
}

// handleAudit returns the audit log for a specific key.
func (h *apiKeysHandler) handleAudit(w http.ResponseWriter, r *http.Request, keyID string) {
	claims, ok := ClaimsFromContext(r.Context())
	if !ok {
		writeJSON(w, http.StatusUnauthorized, map[string]string{"message": "unauthorized"})
		return
	}

	entries, err := h.store.ListAuditLog(r.Context(), claims.TenantRef.ID, keyID, 100)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"message": "failed to load audit log"})
		return
	}

	writeJSON(w, http.StatusOK, map[string]any{"entries": entries})
}

// writeJSON encodes v as JSON and writes it to w with the given status code.
func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}

