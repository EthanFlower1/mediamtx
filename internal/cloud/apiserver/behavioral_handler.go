package apiserver

// KAI-429: Behavioral config CRUD handler.
//
// Routes (plain JSON, not Connect-Go, until KAI-310 lands):
//
//	POST   /api/v1/cameras/{camera_id}/behavioral/config
//	GET    /api/v1/cameras/{camera_id}/behavioral/config
//	PATCH  /api/v1/cameras/{camera_id}/behavioral/config/{detector_type}
//	DELETE /api/v1/cameras/{camera_id}/behavioral/config/{detector_type}
//
// All routes are behind the full middleware chain (auth + Casbin + audit).
// Casbin enforcement uses:
//   - read  → ActionBehavioralConfigRead  ("behavioral.config.read")
//   - write → ActionBehavioralConfigWrite ("behavioral.config.write")
//
// Tenant isolation: every handler reads tenantID from the verified auth
// claims (never from a URL param) so it is impossible for a caller to
// access another tenant's config by guessing a camera_id.

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"strings"
	"time"

	"github.com/bluenviron/mediamtx/internal/cloud/behavioral"
)


// BehavioralStore is the minimal Store seam the handler needs.
// Using a local interface keeps the handler testable without importing the
// entire behavioral package as a concrete dependency.
type BehavioralStore interface {
	Get(ctx context.Context, tenantID, cameraID string, dt behavioral.DetectorType) (behavioral.Config, error)
	List(ctx context.Context, tenantID, cameraID string) ([]behavioral.Config, error)
	Upsert(ctx context.Context, cfg behavioral.Config) error
	Delete(ctx context.Context, tenantID, cameraID string, dt behavioral.DetectorType) error
}

// behavioralHandler holds the store and is mounted by RegisterBehavioralRoutes.
type behavioralHandler struct {
	store BehavioralStore
}

// behavioralConfigRequest is the JSON body for POST and PATCH.
type behavioralConfigRequest struct {
	DetectorType string `json:"detector_type"`
	Params       string `json:"params"`
	Enabled      *bool  `json:"enabled"`
}

// behavioralConfigResponse is the JSON representation of a single config row.
type behavioralConfigResponse struct {
	TenantID     string `json:"tenant_id"`
	CameraID     string `json:"camera_id"`
	DetectorType string `json:"detector_type"`
	Params       string `json:"params"`
	Enabled      bool   `json:"enabled"`
	CreatedAt    string `json:"created_at"`
	UpdatedAt    string `json:"updated_at"`
}

func configToResponse(c behavioral.Config) behavioralConfigResponse {
	return behavioralConfigResponse{
		TenantID:     c.TenantID,
		CameraID:     c.CameraID,
		DetectorType: string(c.DetectorType),
		Params:       c.Params,
		Enabled:      c.Enabled,
		CreatedAt:    c.CreatedAt.UTC().Format(time.RFC3339),
		UpdatedAt:    c.UpdatedAt.UTC().Format(time.RFC3339),
	}
}

// RegisterBehavioralRoutes mounts the four behavioral config endpoints on mux
// using the provided store and connects them through the full middleware chain.
// It also registers the route-authorization entries needed by Casbin enforcement.
//
// Called from server.go when BehavioralStore is non-nil in Config.
func (s *Server) RegisterBehavioralRoutes(store BehavioralStore) {
	h := &behavioralHandler{store: store}
	chain := s.buildConnectChain()

	// POST /api/v1/cameras/{camera_id}/behavioral/config
	// Using a pattern-prefix so ServeMux routes the single-segment remainder.
	// Go 1.22+ supports {camera_id} wildcards; for earlier compatibility we
	// parse the path in the handler.
	s.mux.Handle("/api/v1/cameras/", chain(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		h.dispatch(w, r)
	})))

	// Register route authorizations for Casbin. Actions match the constants in
	// internal/cloud/permissions/actions.go (ActionBehavioralConfigRead/Write).
	s.routes["/api/v1/cameras/*/behavioral/config"]   = RouteAuthorization{ResourceType: "behavioral_config", Action: "behavioral.config.read"}
	s.routes["/api/v1/cameras/*/behavioral/config/*"] = RouteAuthorization{ResourceType: "behavioral_config", Action: "behavioral.config.write"}
}

// dispatch routes by method and path suffix. All behavioral paths share the
// /api/v1/cameras/ prefix; we strip it and parse the remainder.
func (h *behavioralHandler) dispatch(w http.ResponseWriter, r *http.Request) {
	// Path: /api/v1/cameras/{camera_id}/behavioral/config[/{detector_type}]
	rest := strings.TrimPrefix(r.URL.Path, "/api/v1/cameras/")
	parts := strings.Split(rest, "/")
	// parts[0] = camera_id
	// parts[1] = "behavioral"
	// parts[2] = "config"
	// parts[3] = detector_type (optional)
	if len(parts) < 3 || parts[1] != "behavioral" || parts[2] != "config" {
		writeError(w, NewError(CodeNotFound, errors.New("not found")))
		return
	}
	cameraID := parts[0]
	if cameraID == "" {
		writeError(w, NewError(CodeInvalidArgument, errors.New("camera_id is required")))
		return
	}

	hasDetector := len(parts) >= 4 && parts[3] != ""
	switch {
	case r.Method == http.MethodGet && !hasDetector:
		h.list(w, r, cameraID)
	case r.Method == http.MethodPost && !hasDetector:
		h.upsert(w, r, cameraID, "")
	case r.Method == http.MethodPatch && hasDetector:
		h.upsert(w, r, cameraID, parts[3])
	case r.Method == http.MethodDelete && hasDetector:
		h.delete(w, r, cameraID, parts[3])
	default:
		writeError(w, NewError(CodeNotFound, errors.New("not found")))
	}
}

// list handles GET /api/v1/cameras/{camera_id}/behavioral/config
func (h *behavioralHandler) list(w http.ResponseWriter, r *http.Request, cameraID string) {
	claims, ok := ClaimsFromContext(r.Context())
	if !ok {
		writeError(w, NewError(CodeUnauthenticated, errors.New("missing claims")))
		return
	}
	tenantID := claims.TenantRef.ID

	configs, err := h.store.List(r.Context(), tenantID, cameraID)
	if err != nil {
		writeError(w, NewError(CodeInternal, err))
		return
	}

	resp := make([]behavioralConfigResponse, len(configs))
	for i, c := range configs {
		resp[i] = configToResponse(c)
	}
	writeJSON(w, http.StatusOK, map[string]any{"configs": resp})
}

// upsert handles POST (body carries detector_type) and PATCH (detector_type in URL).
func (h *behavioralHandler) upsert(w http.ResponseWriter, r *http.Request, cameraID, urlDetector string) {
	claims, ok := ClaimsFromContext(r.Context())
	if !ok {
		writeError(w, NewError(CodeUnauthenticated, errors.New("missing claims")))
		return
	}
	tenantID := claims.TenantRef.ID

	var req behavioralConfigRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, NewError(CodeInvalidArgument, errors.New("invalid request body")))
		return
	}

	// On PATCH the detector_type comes from the URL; on POST from the body.
	dtStr := urlDetector
	if dtStr == "" {
		dtStr = req.DetectorType
	}
	dt := behavioral.DetectorType(dtStr)
	if !dt.IsValid() {
		writeError(w, NewError(CodeInvalidArgument, errors.New("invalid detector_type")))
		return
	}

	params := req.Params
	if params == "" {
		params = "{}"
	}
	if err := behavioral.ValidateParams(dt, params); err != nil {
		writeError(w, NewError(CodeInvalidArgument, err))
		return
	}

	enabled := false
	if req.Enabled != nil {
		enabled = *req.Enabled
	}

	cfg := behavioral.Config{
		TenantID:     tenantID,
		CameraID:     cameraID,
		DetectorType: dt,
		Params:       params,
		Enabled:      enabled,
	}
	if err := h.store.Upsert(r.Context(), cfg); err != nil {
		writeError(w, NewError(CodeInternal, err))
		return
	}

	// Re-read to get server-assigned timestamps.
	saved, err := h.store.Get(r.Context(), tenantID, cameraID, dt)
	if err != nil {
		writeError(w, NewError(CodeInternal, err))
		return
	}
	writeJSON(w, http.StatusOK, configToResponse(saved))
}

// delete handles DELETE /api/v1/cameras/{camera_id}/behavioral/config/{detector_type}
func (h *behavioralHandler) delete(w http.ResponseWriter, r *http.Request, cameraID, dtStr string) {
	claims, ok := ClaimsFromContext(r.Context())
	if !ok {
		writeError(w, NewError(CodeUnauthenticated, errors.New("missing claims")))
		return
	}
	tenantID := claims.TenantRef.ID

	dt := behavioral.DetectorType(dtStr)
	if !dt.IsValid() {
		writeError(w, NewError(CodeInvalidArgument, errors.New("invalid detector_type")))
		return
	}

	err := h.store.Delete(r.Context(), tenantID, cameraID, dt)
	if errors.Is(err, behavioral.ErrNotFound) {
		writeError(w, NewError(CodeNotFound, errors.New("config not found")))
		return
	}
	if err != nil {
		writeError(w, NewError(CodeInternal, err))
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// writeJSON encodes v as JSON and writes it with the given status.
func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}
