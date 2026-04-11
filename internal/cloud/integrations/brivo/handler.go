package brivo

import (
	"encoding/json"
	"io"
	"net/http"
	"time"
)

// Handler exposes the Brivo integration as HTTP endpoints for the config UI
// and inbound webhooks. It is registered under the apiserver's router.
//
// Routes:
//
//	GET    /integrations/brivo/auth          -> begin OAuth flow
//	GET    /integrations/brivo/callback      -> OAuth callback
//	GET    /integrations/brivo/connection     -> get connection status
//	DELETE /integrations/brivo/connection     -> disconnect
//	POST   /integrations/brivo/test          -> test connectivity
//	GET    /integrations/brivo/sites         -> list Brivo sites
//	GET    /integrations/brivo/doors?site_id= -> list doors for site
//	GET    /integrations/brivo/mappings      -> list door-camera mappings
//	POST   /integrations/brivo/mappings      -> create/update mapping
//	DELETE /integrations/brivo/mappings/:id  -> delete mapping
//	POST   /integrations/brivo/webhook       -> inbound Brivo event
//	GET    /integrations/brivo/events        -> list recent events
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

// NewHandler constructs an HTTP handler for the Brivo integration.
func NewHandler(cfg HandlerConfig) *Handler {
	h := &Handler{svc: cfg.Service}
	mux := http.NewServeMux()

	tenant := cfg.Tenant
	if tenant == nil {
		tenant = func(r *http.Request) string {
			return r.Header.Get("X-Tenant-ID")
		}
	}

	// OAuth flow.
	mux.HandleFunc("GET /integrations/brivo/auth", func(w http.ResponseWriter, r *http.Request) {
		tenantID := tenant(r)
		if tenantID == "" {
			writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "missing tenant"})
			return
		}
		authURL, err := h.svc.OAuth().BeginAuthorize(r.Context(), tenantID)
		if err != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
			return
		}
		writeJSON(w, http.StatusOK, map[string]string{"authorize_url": authURL})
	})

	mux.HandleFunc("GET /integrations/brivo/callback", func(w http.ResponseWriter, r *http.Request) {
		state := r.URL.Query().Get("state")
		code := r.URL.Query().Get("code")
		siteID := r.URL.Query().Get("site_id")
		siteName := r.URL.Query().Get("site_name")
		tenantID := tenant(r)

		if err := h.svc.Connect(r.Context(), tenantID, state, code, siteID, siteName); err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
			return
		}
		writeJSON(w, http.StatusOK, map[string]string{"status": "connected"})
	})

	// Connection management.
	mux.HandleFunc("GET /integrations/brivo/connection", func(w http.ResponseWriter, r *http.Request) {
		tenantID := tenant(r)
		conn, err := h.svc.GetConnection(r.Context(), tenantID)
		if err != nil {
			writeJSON(w, http.StatusNotFound, map[string]string{"error": err.Error()})
			return
		}
		writeJSON(w, http.StatusOK, conn)
	})

	mux.HandleFunc("DELETE /integrations/brivo/connection", func(w http.ResponseWriter, r *http.Request) {
		tenantID := tenant(r)
		if err := h.svc.Disconnect(r.Context(), tenantID); err != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
			return
		}
		writeJSON(w, http.StatusOK, map[string]string{"status": "disconnected"})
	})

	// Test connectivity.
	mux.HandleFunc("POST /integrations/brivo/test", func(w http.ResponseWriter, r *http.Request) {
		tenantID := tenant(r)
		if err := h.svc.TestConnection(r.Context(), tenantID); err != nil {
			writeJSON(w, http.StatusServiceUnavailable, map[string]string{
				"status": "failed",
				"error":  err.Error(),
			})
			return
		}
		writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
	})

	// Config UI: sites + doors.
	mux.HandleFunc("GET /integrations/brivo/sites", func(w http.ResponseWriter, r *http.Request) {
		tenantID := tenant(r)
		sites, err := h.svc.ListSites(r.Context(), tenantID)
		if err != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
			return
		}
		writeJSON(w, http.StatusOK, sites)
	})

	mux.HandleFunc("GET /integrations/brivo/doors", func(w http.ResponseWriter, r *http.Request) {
		tenantID := tenant(r)
		siteID := r.URL.Query().Get("site_id")
		doors, err := h.svc.ListDoors(r.Context(), tenantID, siteID)
		if err != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
			return
		}
		writeJSON(w, http.StatusOK, doors)
	})

	// Door-camera mappings.
	mux.HandleFunc("GET /integrations/brivo/mappings", func(w http.ResponseWriter, r *http.Request) {
		tenantID := tenant(r)
		mappings, err := h.svc.ListDoorCameraMappings(r.Context(), tenantID)
		if err != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
			return
		}
		if mappings == nil {
			mappings = []DoorCameraMapping{}
		}
		writeJSON(w, http.StatusOK, mappings)
	})

	mux.HandleFunc("POST /integrations/brivo/mappings", func(w http.ResponseWriter, r *http.Request) {
		tenantID := tenant(r)
		var m DoorCameraMapping
		if err := json.NewDecoder(r.Body).Decode(&m); err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid json"})
			return
		}
		m.TenantID = tenantID
		if err := h.svc.SetDoorCameraMapping(r.Context(), m); err != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
			return
		}
		writeJSON(w, http.StatusOK, m)
	})

	mux.HandleFunc("DELETE /integrations/brivo/mappings/{id}", func(w http.ResponseWriter, r *http.Request) {
		tenantID := tenant(r)
		mappingID := r.PathValue("id")
		if err := h.svc.DeleteDoorCameraMapping(r.Context(), tenantID, mappingID); err != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
			return
		}
		writeJSON(w, http.StatusOK, map[string]string{"status": "deleted"})
	})

	// Inbound webhook from Brivo.
	mux.HandleFunc("POST /integrations/brivo/webhook", func(w http.ResponseWriter, r *http.Request) {
		body, err := io.ReadAll(r.Body)
		if err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "read body"})
			return
		}

		sig := r.Header.Get("X-Brivo-Signature")
		if err := h.svc.VerifyWebhookSignature(body, sig); err != nil {
			writeJSON(w, http.StatusUnauthorized, map[string]string{"error": err.Error()})
			return
		}

		var event DoorEvent
		if err := json.Unmarshal(body, &event); err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid event json"})
			return
		}

		result, err := h.svc.HandleDoorEvent(r.Context(), event)
		if err != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
			return
		}
		writeJSON(w, http.StatusOK, result)
	})

	// List recent events.
	mux.HandleFunc("GET /integrations/brivo/events", func(w http.ResponseWriter, r *http.Request) {
		tenantID := tenant(r)
		to := time.Now().UTC()
		from := to.Add(-24 * time.Hour) // default: last 24h
		events, err := h.svc.ListEvents(r.Context(), tenantID, from, to, 100)
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
