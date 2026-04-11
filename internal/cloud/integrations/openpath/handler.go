package openpath

import (
	"encoding/json"
	"net/http"
	"time"
)

// Handler exposes the OpenPath / Avigilon Alta integration as HTTP endpoints
// for the config UI and inbound webhooks.
//
// Routes:
//
//	POST   /integrations/openpath/connect          -> register tenant config
//	GET    /integrations/openpath/connection        -> get connection status
//	DELETE /integrations/openpath/connection        -> disconnect
//	POST   /integrations/openpath/test              -> test connectivity
//	GET    /integrations/openpath/doors             -> list doors from Alta
//	GET    /integrations/openpath/mappings          -> list door-camera mappings
//	POST   /integrations/openpath/mappings          -> create/update mapping
//	DELETE /integrations/openpath/mappings/{id}     -> delete mapping
//	POST   /integrations/openpath/webhook/{tenant_id} -> inbound Alta event
//	POST   /integrations/openpath/lockdown          -> trigger door lockdown
//	GET    /integrations/openpath/events            -> list recent events
type Handler struct {
	svc *Service
	mux *http.ServeMux
}

// TenantExtractor pulls the authenticated tenant ID from the request.
// Typically wired to the session middleware.
type TenantExtractor func(r *http.Request) string

// HandlerConfig wires the Handler to the integration service and tenant
// extraction middleware.
type HandlerConfig struct {
	Service *Service
	Tenant  TenantExtractor
}

// NewHandler constructs an HTTP handler for the OpenPath integration.
func NewHandler(cfg HandlerConfig) *Handler {
	h := &Handler{svc: cfg.Service}
	mux := http.NewServeMux()

	tenant := cfg.Tenant
	if tenant == nil {
		tenant = func(r *http.Request) string {
			return r.Header.Get("X-Tenant-ID")
		}
	}

	// Connect: register tenant credentials (client_credentials flow).
	mux.HandleFunc("POST /integrations/openpath/connect", func(w http.ResponseWriter, r *http.Request) {
		tenantID := tenant(r)
		if tenantID == "" {
			writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "missing tenant"})
			return
		}

		var req struct {
			OrgID         string `json:"org_id"`
			ClientID      string `json:"client_id"`
			ClientSecret  string `json:"client_secret"`
			WebhookSecret string `json:"webhook_secret"`
			BaseURL       string `json:"base_url,omitempty"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid json"})
			return
		}

		newCfg := Config{
			TenantID:      tenantID,
			OrgID:         req.OrgID,
			ClientID:      req.ClientID,
			ClientSecret:  req.ClientSecret,
			BaseURL:       req.BaseURL,
			WebhookSecret: req.WebhookSecret,
			Enabled:       true,
		}

		if err := h.svc.Register(r.Context(), newCfg); err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
			return
		}
		writeJSON(w, http.StatusOK, map[string]string{"status": "connected"})
	})

	// Connection status.
	mux.HandleFunc("GET /integrations/openpath/connection", func(w http.ResponseWriter, r *http.Request) {
		tenantID := tenant(r)
		ts, err := h.svc.tenant(tenantID)
		if err != nil {
			writeJSON(w, http.StatusNotFound, map[string]string{
				"status": "disconnected",
				"error":  err.Error(),
			})
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{
			"status":  "connected",
			"org_id":  ts.cfg.OrgID,
			"enabled": ts.cfg.Enabled,
			"doors":   len(ts.cfg.DoorCameraMappings),
		})
	})

	// Disconnect.
	mux.HandleFunc("DELETE /integrations/openpath/connection", func(w http.ResponseWriter, r *http.Request) {
		tenantID := tenant(r)
		h.svc.Unregister(tenantID)
		writeJSON(w, http.StatusOK, map[string]string{"status": "disconnected"})
	})

	// Test connectivity.
	mux.HandleFunc("POST /integrations/openpath/test", func(w http.ResponseWriter, r *http.Request) {
		tenantID := tenant(r)
		ts, err := h.svc.tenant(tenantID)
		if err != nil {
			writeJSON(w, http.StatusNotFound, map[string]string{"error": err.Error()})
			return
		}
		if _, err := ts.client.Authenticate(r.Context()); err != nil {
			writeJSON(w, http.StatusServiceUnavailable, map[string]string{
				"status": "failed",
				"error":  err.Error(),
			})
			return
		}
		writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
	})

	// List doors from Alta.
	mux.HandleFunc("GET /integrations/openpath/doors", func(w http.ResponseWriter, r *http.Request) {
		tenantID := tenant(r)
		doors, err := h.svc.ListDoors(r.Context(), tenantID)
		if err != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
			return
		}
		writeJSON(w, http.StatusOK, doors)
	})

	// Door-camera mappings: list.
	mux.HandleFunc("GET /integrations/openpath/mappings", func(w http.ResponseWriter, r *http.Request) {
		tenantID := tenant(r)
		ts, err := h.svc.tenant(tenantID)
		if err != nil {
			writeJSON(w, http.StatusNotFound, map[string]string{"error": err.Error()})
			return
		}
		mappings := ts.cfg.DoorCameraMappings
		if mappings == nil {
			mappings = []DoorCameraMapping{}
		}
		writeJSON(w, http.StatusOK, mappings)
	})

	// Door-camera mappings: create/update.
	mux.HandleFunc("POST /integrations/openpath/mappings", func(w http.ResponseWriter, r *http.Request) {
		tenantID := tenant(r)
		var m DoorCameraMapping
		if err := json.NewDecoder(r.Body).Decode(&m); err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid json"})
			return
		}
		if m.DoorID == "" || len(m.CameraPaths) == 0 {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "door_id and camera_paths required"})
			return
		}

		if err := h.svc.SetDoorCameraMapping(tenantID, m); err != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
			return
		}
		writeJSON(w, http.StatusOK, m)
	})

	// Door-camera mappings: delete.
	mux.HandleFunc("DELETE /integrations/openpath/mappings/{door_id}", func(w http.ResponseWriter, r *http.Request) {
		tenantID := tenant(r)
		doorID := r.PathValue("door_id")
		if err := h.svc.DeleteDoorCameraMapping(tenantID, doorID); err != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
			return
		}
		writeJSON(w, http.StatusOK, map[string]string{"status": "deleted"})
	})

	// Inbound Alta webhook.
	mux.HandleFunc("POST /integrations/openpath/webhook/{tenant_id}", h.svc.HandleWebhook)

	// Trigger lockdown (NVR -> Alta).
	mux.HandleFunc("POST /integrations/openpath/lockdown", func(w http.ResponseWriter, r *http.Request) {
		tenantID := tenant(r)
		var req LockdownRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid json"})
			return
		}
		req.TenantID = tenantID

		if err := h.svc.TriggerLockdown(r.Context(), req); err != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
			return
		}
		writeJSON(w, http.StatusOK, map[string]string{"status": "lockdown_triggered"})
	})

	// List recent events.
	mux.HandleFunc("GET /integrations/openpath/events", func(w http.ResponseWriter, r *http.Request) {
		tenantID := tenant(r)
		to := time.Now().UTC()
		from := to.Add(-24 * time.Hour) // default: last 24h
		events, err := h.svc.ListDoorEvents(r.Context(), tenantID, from, to)
		if err != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
			return
		}
		if events == nil {
			events = []DoorEvent{}
		}
		writeJSON(w, http.StatusOK, events)
	})

	h.mux = mux
	return h
}

// ServeHTTP implements http.Handler.
func (h *Handler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	h.mux.ServeHTTP(w, r)
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}
