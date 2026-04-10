package escalation

import (
	"encoding/json"
	"errors"
	"net/http"
	"strings"
	"time"
)

// Handler provides HTTP endpoints for escalation chain CRUD and alert
// acknowledgement. It is designed to be mounted on an http.ServeMux by
// the apiserver, behind the full auth middleware chain.
//
// Routes:
//
//	GET    /api/v1/escalation/chains               — list chains
//	POST   /api/v1/escalation/chains               — create chain
//	GET    /api/v1/escalation/chains/{chain_id}     — get chain + steps
//	DELETE /api/v1/escalation/chains/{chain_id}     — delete chain
//	POST   /api/v1/alerts/{alert_id}/ack            — acknowledge alert
//	GET    /api/v1/alerts/{alert_id}/escalation      — get alert escalation state
type Handler struct {
	svc            *Service
	claimsExtractor ClaimsExtractor
}

// ClaimsExtractor extracts tenant/user info from an HTTP request.
// The apiserver wires this to pull auth.Claims from context.
type ClaimsExtractor func(r *http.Request) (tenantID, userID string, ok bool)

// NewHandler creates an escalation HTTP handler.
func NewHandler(svc *Service, extractor ClaimsExtractor) *Handler {
	return &Handler{svc: svc, claimsExtractor: extractor}
}

// Dispatch routes requests to the appropriate handler method.
func (h *Handler) Dispatch(w http.ResponseWriter, r *http.Request) {
	path := r.URL.Path

	switch {
	// POST /api/v1/alerts/{alert_id}/ack
	case strings.HasPrefix(path, "/api/v1/alerts/") && strings.HasSuffix(path, "/ack") && r.Method == http.MethodPost:
		alertID := extractSegment(path, "/api/v1/alerts/", "/ack")
		if alertID == "" {
			writeErr(w, http.StatusBadRequest, "alert_id is required")
			return
		}
		h.ackAlert(w, r, alertID)

	// GET /api/v1/alerts/{alert_id}/escalation
	case strings.HasPrefix(path, "/api/v1/alerts/") && strings.HasSuffix(path, "/escalation") && r.Method == http.MethodGet:
		alertID := extractSegment(path, "/api/v1/alerts/", "/escalation")
		if alertID == "" {
			writeErr(w, http.StatusBadRequest, "alert_id is required")
			return
		}
		h.getAlertEscalation(w, r, alertID)

	// Chains CRUD
	case path == "/api/v1/escalation/chains" && r.Method == http.MethodGet:
		h.listChains(w, r)
	case path == "/api/v1/escalation/chains" && r.Method == http.MethodPost:
		h.createChain(w, r)
	case strings.HasPrefix(path, "/api/v1/escalation/chains/") && r.Method == http.MethodGet:
		chainID := strings.TrimPrefix(path, "/api/v1/escalation/chains/")
		if chainID == "" {
			writeErr(w, http.StatusBadRequest, "chain_id is required")
			return
		}
		h.getChain(w, r, chainID)
	case strings.HasPrefix(path, "/api/v1/escalation/chains/") && r.Method == http.MethodDelete:
		chainID := strings.TrimPrefix(path, "/api/v1/escalation/chains/")
		if chainID == "" {
			writeErr(w, http.StatusBadRequest, "chain_id is required")
			return
		}
		h.deleteChain(w, r, chainID)

	default:
		writeErr(w, http.StatusNotFound, "not found")
	}
}

// ---------- chain handlers ----------

type createChainRequest struct {
	Name        string            `json:"name"`
	Description string            `json:"description"`
	Enabled     bool              `json:"enabled"`
	Steps       []createStepInput `json:"steps"`
}

type createStepInput struct {
	TargetType     string `json:"target_type"`
	TargetID       string `json:"target_id"`
	ChannelType    string `json:"channel_type"`
	TimeoutSeconds int    `json:"timeout_seconds"`
}

type chainResponse struct {
	ChainID     string         `json:"chain_id"`
	TenantID    string         `json:"tenant_id"`
	Name        string         `json:"name"`
	Description string         `json:"description"`
	Enabled     bool           `json:"enabled"`
	Steps       []stepResponse `json:"steps,omitempty"`
	CreatedAt   string         `json:"created_at"`
	UpdatedAt   string         `json:"updated_at"`
}

type stepResponse struct {
	StepID         string `json:"step_id"`
	StepOrder      int    `json:"step_order"`
	TargetType     string `json:"target_type"`
	TargetID       string `json:"target_id"`
	ChannelType    string `json:"channel_type"`
	TimeoutSeconds int    `json:"timeout_seconds"`
}

type alertEscalationResponse struct {
	AlertID        string  `json:"alert_id"`
	TenantID       string  `json:"tenant_id"`
	ChainID        string  `json:"chain_id"`
	CurrentStep    int     `json:"current_step"`
	State          string  `json:"state"`
	AckedBy        string  `json:"acked_by,omitempty"`
	AckedAt        *string `json:"acked_at,omitempty"`
	NextEscalation *string `json:"next_escalation,omitempty"`
	CreatedAt      string  `json:"created_at"`
	UpdatedAt      string  `json:"updated_at"`
}

func (h *Handler) listChains(w http.ResponseWriter, r *http.Request) {
	tenantID, _, ok := h.claimsExtractor(r)
	if !ok {
		writeErr(w, http.StatusUnauthorized, "missing claims")
		return
	}

	chains, err := h.svc.ListChains(r.Context(), tenantID)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}

	resp := make([]chainResponse, len(chains))
	for i, c := range chains {
		resp[i] = chainToResponse(c, nil)
	}
	writeJSON(w, http.StatusOK, map[string]any{"chains": resp})
}

func (h *Handler) createChain(w http.ResponseWriter, r *http.Request) {
	tenantID, _, ok := h.claimsExtractor(r)
	if !ok {
		writeErr(w, http.StatusUnauthorized, "missing claims")
		return
	}

	var req createChainRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeErr(w, http.StatusBadRequest, "invalid request body")
		return
	}

	steps := make([]Step, len(req.Steps))
	for i, s := range req.Steps {
		steps[i] = Step{
			TargetType:     TargetType(s.TargetType),
			TargetID:       s.TargetID,
			ChannelType:    ChannelType(s.ChannelType),
			TimeoutSeconds: s.TimeoutSeconds,
		}
	}

	chain, created, err := h.svc.CreateChain(r.Context(), Chain{
		TenantID:    tenantID,
		Name:        req.Name,
		Description: req.Description,
		Enabled:     req.Enabled,
	}, steps)
	if err != nil {
		if errors.Is(err, ErrInvalidChain) {
			writeErr(w, http.StatusBadRequest, err.Error())
			return
		}
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}

	writeJSON(w, http.StatusCreated, chainToResponse(chain, created))
}

func (h *Handler) getChain(w http.ResponseWriter, r *http.Request, chainID string) {
	tenantID, _, ok := h.claimsExtractor(r)
	if !ok {
		writeErr(w, http.StatusUnauthorized, "missing claims")
		return
	}

	chain, err := h.svc.GetChain(r.Context(), tenantID, chainID)
	if errors.Is(err, ErrChainNotFound) {
		writeErr(w, http.StatusNotFound, "chain not found")
		return
	}
	if err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}

	steps, err := h.svc.GetSteps(r.Context(), tenantID, chainID)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}

	writeJSON(w, http.StatusOK, chainToResponse(chain, steps))
}

func (h *Handler) deleteChain(w http.ResponseWriter, r *http.Request, chainID string) {
	tenantID, _, ok := h.claimsExtractor(r)
	if !ok {
		writeErr(w, http.StatusUnauthorized, "missing claims")
		return
	}

	err := h.svc.DeleteChain(r.Context(), tenantID, chainID)
	if errors.Is(err, ErrChainNotFound) {
		writeErr(w, http.StatusNotFound, "chain not found")
		return
	}
	if err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// ---------- alert handlers ----------

func (h *Handler) ackAlert(w http.ResponseWriter, r *http.Request, alertID string) {
	tenantID, userID, ok := h.claimsExtractor(r)
	if !ok {
		writeErr(w, http.StatusUnauthorized, "missing claims")
		return
	}

	ae, err := h.svc.AcknowledgeAlert(r.Context(), tenantID, alertID, userID)
	if errors.Is(err, ErrAlertNotFound) {
		writeErr(w, http.StatusNotFound, "alert not found")
		return
	}
	if errors.Is(err, ErrAlreadyAcknowledged) {
		writeErr(w, http.StatusConflict, "alert already acknowledged")
		return
	}
	if err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}

	writeJSON(w, http.StatusOK, alertEscalationToResponse(ae))
}

func (h *Handler) getAlertEscalation(w http.ResponseWriter, r *http.Request, alertID string) {
	tenantID, _, ok := h.claimsExtractor(r)
	if !ok {
		writeErr(w, http.StatusUnauthorized, "missing claims")
		return
	}

	ae, err := h.svc.GetAlertEscalation(r.Context(), tenantID, alertID)
	if errors.Is(err, ErrAlertNotFound) {
		writeErr(w, http.StatusNotFound, "alert not found")
		return
	}
	if err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}

	writeJSON(w, http.StatusOK, alertEscalationToResponse(ae))
}

// ---------- helpers ----------

func chainToResponse(c Chain, steps []Step) chainResponse {
	resp := chainResponse{
		ChainID:     c.ChainID,
		TenantID:    c.TenantID,
		Name:        c.Name,
		Description: c.Description,
		Enabled:     c.Enabled,
		CreatedAt:   c.CreatedAt.UTC().Format(time.RFC3339),
		UpdatedAt:   c.UpdatedAt.UTC().Format(time.RFC3339),
	}
	if len(steps) > 0 {
		resp.Steps = make([]stepResponse, len(steps))
		for i, s := range steps {
			resp.Steps[i] = stepResponse{
				StepID:         s.StepID,
				StepOrder:      s.StepOrder,
				TargetType:     string(s.TargetType),
				TargetID:       s.TargetID,
				ChannelType:    string(s.ChannelType),
				TimeoutSeconds: s.TimeoutSeconds,
			}
		}
	}
	return resp
}

func alertEscalationToResponse(ae *AlertEscalation) alertEscalationResponse {
	resp := alertEscalationResponse{
		AlertID:     ae.AlertID,
		TenantID:    ae.TenantID,
		ChainID:     ae.ChainID,
		CurrentStep: ae.CurrentStep,
		State:       string(ae.State),
		AckedBy:     ae.AckedBy,
		CreatedAt:   ae.CreatedAt.UTC().Format(time.RFC3339),
		UpdatedAt:   ae.UpdatedAt.UTC().Format(time.RFC3339),
	}
	if ae.AckedAt != nil {
		s := ae.AckedAt.UTC().Format(time.RFC3339)
		resp.AckedAt = &s
	}
	if ae.NextEscalation != nil {
		s := ae.NextEscalation.UTC().Format(time.RFC3339)
		resp.NextEscalation = &s
	}
	return resp
}

func extractSegment(path, prefix, suffix string) string {
	s := strings.TrimPrefix(path, prefix)
	s = strings.TrimSuffix(s, suffix)
	return s
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}

func writeErr(w http.ResponseWriter, status int, msg string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(map[string]string{"error": msg})
}
