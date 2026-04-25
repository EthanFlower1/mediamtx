package adminapi

import (
	"encoding/json"
	"net/http"

	directorydb "github.com/bluenviron/mediamtx/internal/directory/db"
)

// WebhookHandlers groups webhook HTTP handlers for the admin API.
// It directly uses the directory DB (rather than the Store wrapper) because
// webhook operations were added after the Store interface was defined.
type WebhookHandlers struct {
	DB     *directorydb.DB
	Logger interface{ Printf(string, ...any) }
}

// WebhooksHandler handles GET (list) and POST (create) on /api/v1/admin/webhooks.
func (h *WebhookHandlers) WebhooksHandler() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		switch r.Method {
		case http.MethodGet:
			hooks, err := h.DB.ListWebhookConfigs()
			if err != nil {
				jsonErr(w, http.StatusInternalServerError, err.Error())
				return
			}
			if hooks == nil {
				hooks = []*directorydb.WebhookConfig{}
			}
			jsonOK(w, map[string]any{"items": hooks})

		case http.MethodPost:
			var cfg directorydb.WebhookConfig
			if err := json.NewDecoder(r.Body).Decode(&cfg); err != nil {
				jsonErr(w, http.StatusBadRequest, "invalid JSON")
				return
			}
			if cfg.URL == "" {
				jsonErr(w, http.StatusBadRequest, "url is required")
				return
			}
			cfg.ID = genID()
			if err := h.DB.InsertWebhookConfig(&cfg); err != nil {
				jsonErr(w, http.StatusInternalServerError, err.Error())
				return
			}
			w.WriteHeader(http.StatusCreated)
			jsonOK(w, map[string]string{"id": cfg.ID})

		default:
			jsonErr(w, http.StatusMethodNotAllowed, "method not allowed")
		}
	}
}

// WebhookByIDHandler handles GET, PUT, DELETE on /api/v1/admin/webhooks/by-id?id=<id>.
func (h *WebhookHandlers) WebhookByIDHandler() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		id := r.URL.Query().Get("id")
		if id == "" {
			jsonErr(w, http.StatusBadRequest, "id required")
			return
		}
		switch r.Method {
		case http.MethodGet:
			cfg, err := h.DB.GetWebhookConfig(id)
			if err != nil {
				jsonErr(w, http.StatusNotFound, "webhook not found")
				return
			}
			jsonOK(w, cfg)

		case http.MethodPut:
			var cfg directorydb.WebhookConfig
			if err := json.NewDecoder(r.Body).Decode(&cfg); err != nil {
				jsonErr(w, http.StatusBadRequest, "invalid JSON")
				return
			}
			cfg.ID = id
			if err := h.DB.UpdateWebhookConfig(&cfg); err != nil {
				jsonErr(w, http.StatusInternalServerError, err.Error())
				return
			}
			jsonOK(w, map[string]string{"status": "updated"})

		case http.MethodDelete:
			if err := h.DB.DeleteWebhookConfig(id); err != nil {
				jsonErr(w, http.StatusInternalServerError, err.Error())
				return
			}
			jsonOK(w, map[string]string{"status": "deleted"})

		default:
			jsonErr(w, http.StatusMethodNotAllowed, "method not allowed")
		}
	}
}
