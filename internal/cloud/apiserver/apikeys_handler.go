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
	"time"

	"github.com/bluenviron/mediamtx/internal/cloud/publicapi"
	"github.com/bluenviron/mediamtx/internal/cloud/publicapi/apikeys"
)

// APIKeyStore extends publicapi.APIKeyStore with audit log queries.
// The concrete apikeys.Store satisfies this interface.
type APIKeyStore interface {
	publicapi.APIKeyStore
	ListAuditLog(ctx context.Context, tenantID, keyID string, limit int) ([]apikeys.AuditEntry, error)
}

// apiKeyInfoJSON is the JSON representation of a key (never includes the raw key).
type apiKeyInfoJSON struct {
	ID             string   `json:"id"`
	TenantID       string   `json:"tenant_id"`
	Name           string   `json:"name"`
	KeyPrefix      string   `json:"key_prefix"`
	Scopes         []string `json:"scopes"`
	Status         string   `json:"status"`
	CreatedBy      string   `json:"created_by"`
	CreatedAt      string   `json:"created_at"`
	ExpiresAt      *string  `json:"expires_at,omitempty"`
	RevokedAt      *string  `json:"revoked_at,omitempty"`
	LastUsedAt     *string  `json:"last_used_at,omitempty"`
	RotatedFromID  *string  `json:"rotated_from_id,omitempty"`
	GraceExpiresAt *string  `json:"grace_expires_at,omitempty"`
}

// apiKeyToJSON converts a publicapi.APIKey to the JSON-friendly representation.
func apiKeyToJSON(k *publicapi.APIKey) *apiKeyInfoJSON {
	info := &apiKeyInfoJSON{
		ID:        k.ID,
		TenantID:  k.TenantID,
		Name:      k.Name,
		KeyPrefix: k.KeyPrefix,
		Scopes:    k.Scopes,
		CreatedBy: k.CreatedBy,
		CreatedAt: k.CreatedAt.Format(time.RFC3339),
	}
	if k.IsRevoked() {
		info.Status = "revoked"
	} else if k.IsExpired() {
		info.Status = "expired"
	} else {
		info.Status = "active"
	}
	if !k.ExpiresAt.IsZero() {
		s := k.ExpiresAt.Format(time.RFC3339)
		info.ExpiresAt = &s
	}
	if !k.RevokedAt.IsZero() {
		s := k.RevokedAt.Format(time.RFC3339)
		info.RevokedAt = &s
	}
	if !k.LastUsedAt.IsZero() {
		s := k.LastUsedAt.Format(time.RFC3339)
		info.LastUsedAt = &s
	}
	if k.RotatedFromID != "" {
		s := k.RotatedFromID
		info.RotatedFromID = &s
	}
	if !k.GraceExpiresAt.IsZero() {
		s := k.GraceExpiresAt.Format(time.RFC3339)
		info.GraceExpiresAt = &s
	}
	return info
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

	filter := publicapi.ListAPIKeysFilter{
		TenantID:       claims.TenantRef.ID,
		IncludeRevoked: includeRevoked,
	}
	keys, err := h.store.List(r.Context(), filter)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"message": "failed to list keys"})
		return
	}

	jsonKeys := make([]*apiKeyInfoJSON, len(keys))
	for i, k := range keys {
		jsonKeys[i] = apiKeyToJSON(k)
	}
	writeJSON(w, http.StatusOK, map[string]any{"keys": jsonKeys})
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

	var expiresAt time.Time
	if body.ExpiresInDays != nil && *body.ExpiresInDays > 0 {
		expiresAt = time.Now().AddDate(0, 0, *body.ExpiresInDays)
	}

	req := publicapi.CreateAPIKeyRequest{
		TenantID:  claims.TenantRef.ID,
		Name:      body.Name,
		Scopes:    body.Scopes,
		ExpiresAt: expiresAt,
		CreatedBy: string(claims.UserID),
	}

	result, err := h.store.Create(r.Context(), req)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"message": "failed to create key"})
		return
	}

	writeJSON(w, http.StatusCreated, map[string]any{
		"raw_key": result.RawKey,
		"key":     apiKeyToJSON(result.Key),
	})
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

	gracePeriod := time.Duration(body.GracePeriodHours) * time.Hour
	if gracePeriod <= 0 {
		gracePeriod = publicapi.DefaultGracePeriod
	}

	req := publicapi.RotateAPIKeyRequest{
		KeyID:       keyID,
		RotatedBy:   string(claims.UserID),
		GracePeriod: gracePeriod,
	}

	result, err := h.store.Rotate(r.Context(), req)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"message": "failed to rotate key"})
		return
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"raw_key":            result.RawKey,
		"new_key":            apiKeyToJSON(result.NewKey),
		"old_key_grace_end":  result.OldKeyGraceEnd.Format(time.RFC3339),
	})
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


