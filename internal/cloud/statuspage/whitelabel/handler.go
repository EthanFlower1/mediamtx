package whitelabel

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strings"
)

// Handler provides the HTTP surface for white-label status pages.
// It supports both admin endpoints (CRUD for config and subscribers) and
// public endpoints (subdomain-routed status page rendering).
type Handler struct {
	svc *Service
}

// NewHandler creates a handler backed by the given service.
func NewHandler(svc *Service) *Handler {
	return &Handler{svc: svc}
}

// Route describes a single method+path+handler tuple.
type Route struct {
	Method  string
	Pattern string
	Handler http.HandlerFunc
}

// AdminRoutes returns the admin API endpoints for managing integrator status
// page configs and subscribers.
//
//	PUT    /api/v1/integrators/{id}/status-page
//	GET    /api/v1/integrators/{id}/status-page
//	DELETE /api/v1/integrators/{id}/status-page
//	POST   /api/v1/integrators/{id}/status-page/subscribers
//	POST   /api/v1/integrators/{id}/status-page/subscribers/confirm
//	DELETE /api/v1/integrators/{id}/status-page/subscribers/{email}
//	GET    /api/v1/integrators/{id}/status-page/subscribers
//	GET    /api/v1/integrators/{id}/status-page/render
func (h *Handler) AdminRoutes() []Route {
	return []Route{
		{http.MethodPut, "/api/v1/integrators/{id}/status-page", h.handlePutConfig},
		{http.MethodGet, "/api/v1/integrators/{id}/status-page", h.handleGetConfig},
		{http.MethodDelete, "/api/v1/integrators/{id}/status-page", h.handleDeleteConfig},
		{http.MethodPost, "/api/v1/integrators/{id}/status-page/subscribers", h.handleSubscribe},
		{http.MethodPost, "/api/v1/integrators/{id}/status-page/subscribers/confirm", h.handleConfirm},
		{http.MethodDelete, "/api/v1/integrators/{id}/status-page/subscribers/{email}", h.handleUnsubscribe},
		{http.MethodGet, "/api/v1/integrators/{id}/status-page/subscribers", h.handleListSubscribers},
		{http.MethodGet, "/api/v1/integrators/{id}/status-page/render", h.handleRenderPage},
	}
}

// PublicRoutes returns the public-facing endpoints for subdomain-routed status
// pages. The SubdomainMiddleware (or CustomDomainMiddleware) should extract the
// subdomain and set it in the request context before these routes are reached.
//
//	GET /status       — JSON API
//	GET /status.html  — branded HTML page
func (h *Handler) PublicRoutes() []Route {
	return []Route{
		{http.MethodGet, "/status", h.handlePublicStatus},
		{http.MethodGet, "/status.html", h.handlePublicStatusHTML},
	}
}

// ServeHTTP dispatches requests using simple pattern matching. Useful for
// testing; production should mount routes via gin or chi.
func (h *Handler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	allRoutes := append(h.AdminRoutes(), h.PublicRoutes()...)
	for _, rt := range allRoutes {
		if rt.Method != r.Method {
			continue
		}
		params, ok := matchPattern(rt.Pattern, r.URL.Path)
		if !ok {
			continue
		}
		ctx := context.WithValue(r.Context(), routeParamsKey{}, params)
		rt.Handler.ServeHTTP(w, r.WithContext(ctx))
		return
	}
	http.NotFound(w, r)
}

// --- Admin Handlers ---

func (h *Handler) handlePutConfig(w http.ResponseWriter, r *http.Request) {
	id := paramsFrom(r)["id"]
	var cfg StatusPageConfig
	if err := json.NewDecoder(r.Body).Decode(&cfg); err != nil {
		writeError(w, http.StatusBadRequest, fmt.Sprintf("decode: %v", err))
		return
	}
	cfg.IntegratorID = id
	if cfg.ComponentIDs == nil {
		cfg.ComponentIDs = []string{}
	}
	saved, err := h.svc.UpsertConfig(r.Context(), cfg)
	if err != nil {
		if errors.Is(err, ErrSubdomainTaken) {
			writeError(w, http.StatusConflict, err.Error())
			return
		}
		if errors.Is(err, ErrInvalidSubdomain) {
			writeError(w, http.StatusBadRequest, err.Error())
			return
		}
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, saved)
}

func (h *Handler) handleGetConfig(w http.ResponseWriter, r *http.Request) {
	id := paramsFrom(r)["id"]
	cfg, err := h.svc.GetConfig(r.Context(), id)
	if err != nil {
		if errors.Is(err, ErrConfigNotFound) {
			writeError(w, http.StatusNotFound, err.Error())
			return
		}
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, cfg)
}

func (h *Handler) handleDeleteConfig(w http.ResponseWriter, r *http.Request) {
	id := paramsFrom(r)["id"]
	if err := h.svc.DeleteConfig(r.Context(), id); err != nil {
		if errors.Is(err, ErrConfigNotFound) {
			writeError(w, http.StatusNotFound, err.Error())
			return
		}
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (h *Handler) handleSubscribe(w http.ResponseWriter, r *http.Request) {
	id := paramsFrom(r)["id"]
	var body struct {
		Email string `json:"email"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeError(w, http.StatusBadRequest, fmt.Sprintf("decode: %v", err))
		return
	}
	sub, err := h.svc.Subscribe(r.Context(), id, body.Email)
	if err != nil {
		if errors.Is(err, ErrSubscriberExists) {
			writeError(w, http.StatusConflict, err.Error())
			return
		}
		if errors.Is(err, ErrInvalidEmail) {
			writeError(w, http.StatusBadRequest, err.Error())
			return
		}
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusCreated, sub)
}

func (h *Handler) handleConfirm(w http.ResponseWriter, r *http.Request) {
	id := paramsFrom(r)["id"]
	var body struct {
		Token string `json:"token"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeError(w, http.StatusBadRequest, fmt.Sprintf("decode: %v", err))
		return
	}
	if err := h.svc.ConfirmSubscriber(r.Context(), id, body.Token); err != nil {
		if errors.Is(err, ErrInvalidToken) {
			writeError(w, http.StatusBadRequest, err.Error())
			return
		}
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "confirmed"})
}

func (h *Handler) handleUnsubscribe(w http.ResponseWriter, r *http.Request) {
	id := paramsFrom(r)["id"]
	email := paramsFrom(r)["email"]
	if err := h.svc.Unsubscribe(r.Context(), id, email); err != nil {
		if errors.Is(err, ErrSubscriberNotFound) {
			writeError(w, http.StatusNotFound, err.Error())
			return
		}
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (h *Handler) handleListSubscribers(w http.ResponseWriter, r *http.Request) {
	id := paramsFrom(r)["id"]
	subs, err := h.svc.ListConfirmedSubscribers(r.Context(), id)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	if subs == nil {
		subs = []Subscriber{}
	}
	writeJSON(w, http.StatusOK, subs)
}

func (h *Handler) handleRenderPage(w http.ResponseWriter, r *http.Request) {
	id := paramsFrom(r)["id"]
	page, err := h.svc.RenderPublicPage(r.Context(), id)
	if err != nil {
		if errors.Is(err, ErrConfigNotFound) {
			writeError(w, http.StatusNotFound, err.Error())
			return
		}
		if errors.Is(err, ErrPageDisabled) {
			writeError(w, http.StatusForbidden, err.Error())
			return
		}
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, page)
}

// --- Public Handlers (subdomain-routed) ---

type subdomainContextKey struct{}

// SubdomainMiddleware extracts the subdomain from the Host header and stores
// it in the request context. It expects hosts in the form
// "{subdomain}.status.example.com" where baseDomain is "status.example.com".
func SubdomainMiddleware(baseDomain string) func(http.Handler) http.Handler {
	suffix := "." + strings.TrimPrefix(baseDomain, ".")
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			host := r.Host
			// Strip port if present.
			if idx := strings.LastIndex(host, ":"); idx != -1 {
				host = host[:idx]
			}
			if !strings.HasSuffix(host, suffix) {
				writeError(w, http.StatusNotFound, "unknown host")
				return
			}
			subdomain := strings.TrimSuffix(host, suffix)
			if subdomain == "" {
				writeError(w, http.StatusNotFound, "no subdomain")
				return
			}
			ctx := context.WithValue(r.Context(), subdomainContextKey{}, subdomain)
			next.ServeHTTP(w, r.WithContext(ctx))
		})
	}
}

// SubdomainFrom extracts the subdomain stored by SubdomainMiddleware.
func SubdomainFrom(ctx context.Context) string {
	if v, ok := ctx.Value(subdomainContextKey{}).(string); ok {
		return v
	}
	return ""
}

type customDomainContextKey struct{}

// CustomDomainMiddleware stores the full Host header (minus port) in the
// request context as a custom domain. This is used as a fallback when the
// host does not match the base subdomain pattern, allowing integrators to
// use their own domains (e.g. status.acmealarm.com).
func CustomDomainMiddleware() func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			host := r.Host
			// Strip port if present.
			if idx := strings.LastIndex(host, ":"); idx != -1 {
				host = host[:idx]
			}
			if host == "" {
				writeError(w, http.StatusNotFound, "no host")
				return
			}
			ctx := context.WithValue(r.Context(), customDomainContextKey{}, host)
			next.ServeHTTP(w, r.WithContext(ctx))
		})
	}
}

// CustomDomainFrom extracts the custom domain stored by CustomDomainMiddleware.
func CustomDomainFrom(ctx context.Context) string {
	if v, ok := ctx.Value(customDomainContextKey{}).(string); ok {
		return v
	}
	return ""
}

// CombinedDomainMiddleware tries SubdomainMiddleware first. If the host does
// not match the base subdomain pattern, it falls back to storing the full host
// as a custom domain for lookup in the integrator_status_configs table.
func CombinedDomainMiddleware(baseDomain string) func(http.Handler) http.Handler {
	suffix := "." + strings.TrimPrefix(baseDomain, ".")
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			host := r.Host
			if idx := strings.LastIndex(host, ":"); idx != -1 {
				host = host[:idx]
			}
			if host == "" {
				writeError(w, http.StatusNotFound, "no host")
				return
			}

			ctx := r.Context()
			if strings.HasSuffix(host, suffix) {
				subdomain := strings.TrimSuffix(host, suffix)
				if subdomain != "" {
					ctx = context.WithValue(ctx, subdomainContextKey{}, subdomain)
				}
			} else {
				// Not a subdomain of our base domain -- treat the full
				// host as a custom domain (e.g. status.acmealarm.com).
				ctx = context.WithValue(ctx, customDomainContextKey{}, host)
			}
			next.ServeHTTP(w, r.WithContext(ctx))
		})
	}
}

func (h *Handler) handlePublicStatus(w http.ResponseWriter, r *http.Request) {
	page, err := h.resolvePublicPage(r)
	if err != nil {
		h.writePublicError(w, err)
		return
	}
	// Strip sensitive fields from JSON response.
	page.Config.CustomCSS = ""
	writeJSON(w, http.StatusOK, page)
}

func (h *Handler) handlePublicStatusHTML(w http.ResponseWriter, r *http.Request) {
	page, err := h.resolvePublicPage(r)
	if err != nil {
		h.writePublicError(w, err)
		return
	}
	html, renderErr := RenderHTML(page)
	if renderErr != nil {
		writeError(w, http.StatusInternalServerError, renderErr.Error())
		return
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write(html)
}

// resolvePublicPage attempts to find the integrator's status page using
// the subdomain stored in context (from SubdomainMiddleware) or falling
// back to the custom domain (from CustomDomainMiddleware).
func (h *Handler) resolvePublicPage(r *http.Request) (PublicStatusPage, error) {
	subdomain := SubdomainFrom(r.Context())
	if subdomain != "" {
		return h.svc.RenderPublicPageBySubdomain(r.Context(), subdomain)
	}
	domain := CustomDomainFrom(r.Context())
	if domain != "" {
		return h.svc.RenderPublicPageByCustomDomain(r.Context(), domain)
	}
	return PublicStatusPage{}, ErrConfigNotFound
}

func (h *Handler) writePublicError(w http.ResponseWriter, err error) {
	if errors.Is(err, ErrConfigNotFound) {
		writeError(w, http.StatusNotFound, err.Error())
		return
	}
	if errors.Is(err, ErrPageDisabled) {
		writeError(w, http.StatusForbidden, err.Error())
		return
	}
	writeError(w, http.StatusInternalServerError, err.Error())
}

// --- Helpers ---

type routeParamsKey struct{}

func paramsFrom(r *http.Request) map[string]string {
	if v, ok := r.Context().Value(routeParamsKey{}).(map[string]string); ok {
		return v
	}
	return map[string]string{}
}

func matchPattern(pattern, path string) (map[string]string, bool) {
	pp := strings.Split(strings.Trim(pattern, "/"), "/")
	xp := strings.Split(strings.Trim(path, "/"), "/")
	if len(pp) != len(xp) {
		return nil, false
	}
	out := map[string]string{}
	for i, seg := range pp {
		if strings.HasPrefix(seg, "{") && strings.HasSuffix(seg, "}") {
			out[strings.Trim(seg, "{}")] = xp[i]
			continue
		}
		if seg != xp[i] {
			return nil, false
		}
	}
	return out, true
}

type errorBody struct {
	Error string `json:"error"`
}

func writeJSON(w http.ResponseWriter, status int, body interface{}) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(body)
}

func writeError(w http.ResponseWriter, status int, msg string) {
	writeJSON(w, status, errorBody{Error: msg})
}
