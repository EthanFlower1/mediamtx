package apiserver

// KAI-371: Per-user notification preferences CRUD handler.
//
// Routes (plain JSON, not Connect-Go, until KAI-310 lands):
//
//	GET    /api/v1/notification-prefs           — list user's prefs
//	POST   /api/v1/notification-prefs           — upsert a preference
//	DELETE /api/v1/notification-prefs/{pref_id} — delete a preference
//	POST   /api/v1/notification-prefs/resolve   — resolve best pref for camera+event
//
// Tenant isolation: tenantID is always taken from verified auth claims.

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"strings"
	"time"

	"github.com/bluenviron/mediamtx/internal/cloud/notifications"
	"github.com/bluenviron/mediamtx/internal/cloud/notifications/preferences"
)

// NotificationPrefsStore is the minimal seam the handler needs.
type NotificationPrefsStore interface {
	Upsert(ctx context.Context, p preferences.Pref) (preferences.Pref, error)
	List(ctx context.Context, tenantID, userID string) ([]preferences.Pref, error)
	Delete(ctx context.Context, prefID string) error
	ResolveDelivery(ctx context.Context, tenantID, userID, cameraID, eventType string, severity preferences.Severity) (preferences.ResolvedDelivery, error)
}

type notifPrefsHandler struct {
	store NotificationPrefsStore
}

// notifPrefRequest is the JSON body for POST (upsert).
type notifPrefRequest struct {
	CameraID      string                      `json:"camera_id"`
	EventType     string                      `json:"event_type"`
	Channels      []notifications.ChannelType `json:"channels"`
	SeverityMin   string                      `json:"severity_min"`
	QuietStart    string                      `json:"quiet_start"`
	QuietEnd      string                      `json:"quiet_end"`
	QuietTimezone string                      `json:"quiet_timezone"`
	QuietDays     []int                       `json:"quiet_days"`
	Enabled       *bool                       `json:"enabled"`
}

// notifPrefResponse is the JSON representation of a single preference.
type notifPrefResponse struct {
	PrefID        string                      `json:"pref_id"`
	TenantID      string                      `json:"tenant_id"`
	UserID        string                      `json:"user_id"`
	CameraID      string                      `json:"camera_id"`
	EventType     string                      `json:"event_type"`
	Channels      []notifications.ChannelType `json:"channels"`
	SeverityMin   string                      `json:"severity_min"`
	QuietStart    string                      `json:"quiet_start"`
	QuietEnd      string                      `json:"quiet_end"`
	QuietTimezone string                      `json:"quiet_timezone"`
	QuietDays     []int                       `json:"quiet_days"`
	Enabled       bool                        `json:"enabled"`
	CreatedAt     string                      `json:"created_at"`
	UpdatedAt     string                      `json:"updated_at"`
}

func prefToResponse(p preferences.Pref) notifPrefResponse {
	return notifPrefResponse{
		PrefID:        p.PrefID,
		TenantID:      p.TenantID,
		UserID:        p.UserID,
		CameraID:      p.CameraID,
		EventType:     p.EventType,
		Channels:      p.Channels,
		SeverityMin:   string(p.SeverityMin),
		QuietStart:    p.QuietStart,
		QuietEnd:      p.QuietEnd,
		QuietTimezone: p.QuietTimezone,
		QuietDays:     p.QuietDays,
		Enabled:       p.Enabled,
		CreatedAt:     p.CreatedAt.UTC().Format(time.RFC3339),
		UpdatedAt:     p.UpdatedAt.UTC().Format(time.RFC3339),
	}
}

// notifResolveRequest is the JSON body for POST /resolve.
type notifResolveRequest struct {
	CameraID  string `json:"camera_id"`
	EventType string `json:"event_type"`
	Severity  string `json:"severity"`
}

// notifResolveResponse is the JSON response for /resolve.
type notifResolveResponse struct {
	Pref       *notifPrefResponse `json:"pref,omitempty"`
	Suppressed bool               `json:"suppressed"`
	Reason     string             `json:"reason,omitempty"`
}

// RegisterNotificationPrefsRoutes mounts the notification preference
// endpoints on the server mux behind the full middleware chain.
func (s *Server) RegisterNotificationPrefsRoutes(store NotificationPrefsStore) {
	h := &notifPrefsHandler{store: store}
	chain := s.buildConnectChain()

	s.mux.Handle("/api/v1/notification-prefs/", chain(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		h.dispatch(w, r)
	})))
	s.mux.Handle("/api/v1/notification-prefs", chain(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		h.dispatch(w, r)
	})))

	s.routes["/api/v1/notification-prefs"]   = RouteAuthorization{ResourceType: "notification_prefs", Action: "notification_prefs.read"}
	s.routes["/api/v1/notification-prefs/*"] = RouteAuthorization{ResourceType: "notification_prefs", Action: "notification_prefs.write"}
}

func (h *notifPrefsHandler) dispatch(w http.ResponseWriter, r *http.Request) {
	path := strings.TrimPrefix(r.URL.Path, "/api/v1/notification-prefs")
	path = strings.TrimPrefix(path, "/")

	switch {
	case r.Method == http.MethodGet && path == "":
		h.list(w, r)
	case r.Method == http.MethodPost && path == "":
		h.upsert(w, r)
	case r.Method == http.MethodPost && path == "resolve":
		h.resolve(w, r)
	case r.Method == http.MethodDelete && path != "":
		h.delete(w, r, path)
	default:
		writeError(w, NewError(CodeNotFound, errors.New("not found")))
	}
}

func (h *notifPrefsHandler) list(w http.ResponseWriter, r *http.Request) {
	claims, ok := ClaimsFromContext(r.Context())
	if !ok {
		writeError(w, NewError(CodeUnauthenticated, errors.New("missing claims")))
		return
	}

	prefs, err := h.store.List(r.Context(), claims.TenantRef.ID, string(claims.UserID))
	if err != nil {
		writeError(w, NewError(CodeInternal, err))
		return
	}

	resp := make([]notifPrefResponse, len(prefs))
	for i, p := range prefs {
		resp[i] = prefToResponse(p)
	}
	writeJSON(w, http.StatusOK, map[string]any{"preferences": resp})
}

func (h *notifPrefsHandler) upsert(w http.ResponseWriter, r *http.Request) {
	claims, ok := ClaimsFromContext(r.Context())
	if !ok {
		writeError(w, NewError(CodeUnauthenticated, errors.New("missing claims")))
		return
	}

	var req notifPrefRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, NewError(CodeInvalidArgument, errors.New("invalid request body")))
		return
	}

	enabled := true
	if req.Enabled != nil {
		enabled = *req.Enabled
	}

	sev := preferences.Severity(req.SeverityMin)
	if sev == "" {
		sev = preferences.SeverityInfo
	}

	p := preferences.Pref{
		TenantID:      claims.TenantRef.ID,
		UserID:        string(claims.UserID),
		CameraID:      req.CameraID,
		EventType:     req.EventType,
		Channels:      req.Channels,
		SeverityMin:   sev,
		QuietStart:    req.QuietStart,
		QuietEnd:      req.QuietEnd,
		QuietTimezone: req.QuietTimezone,
		QuietDays:     req.QuietDays,
		Enabled:       enabled,
	}

	saved, err := h.store.Upsert(r.Context(), p)
	if err != nil {
		writeError(w, NewError(CodeInternal, err))
		return
	}
	writeJSON(w, http.StatusOK, prefToResponse(saved))
}

func (h *notifPrefsHandler) delete(w http.ResponseWriter, r *http.Request, prefID string) {
	_, ok := ClaimsFromContext(r.Context())
	if !ok {
		writeError(w, NewError(CodeUnauthenticated, errors.New("missing claims")))
		return
	}

	err := h.store.Delete(r.Context(), prefID)
	if errors.Is(err, preferences.ErrPrefNotFound) {
		writeError(w, NewError(CodeNotFound, errors.New("preference not found")))
		return
	}
	if err != nil {
		writeError(w, NewError(CodeInternal, err))
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (h *notifPrefsHandler) resolve(w http.ResponseWriter, r *http.Request) {
	claims, ok := ClaimsFromContext(r.Context())
	if !ok {
		writeError(w, NewError(CodeUnauthenticated, errors.New("missing claims")))
		return
	}

	var req notifResolveRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, NewError(CodeInvalidArgument, errors.New("invalid request body")))
		return
	}

	sev := preferences.Severity(req.Severity)
	if sev == "" {
		sev = preferences.SeverityInfo
	}

	rd, err := h.store.ResolveDelivery(r.Context(), claims.TenantRef.ID, string(claims.UserID),
		req.CameraID, req.EventType, sev)
	if errors.Is(err, preferences.ErrPrefNotFound) {
		writeJSON(w, http.StatusOK, notifResolveResponse{Suppressed: true, Reason: "no matching preference"})
		return
	}
	if err != nil {
		writeError(w, NewError(CodeInternal, err))
		return
	}

	resp := notifResolveResponse{
		Suppressed: rd.Suppressed,
		Reason:     rd.Reason,
	}
	pr := prefToResponse(rd.Pref)
	resp.Pref = &pr
	writeJSON(w, http.StatusOK, resp)
}
