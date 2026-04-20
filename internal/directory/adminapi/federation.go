package adminapi

import (
	"encoding/json"
	"net/http"

	directorydb "github.com/bluenviron/mediamtx/internal/directory/db"
)

// FederationHandlers groups federation HTTP handlers for the admin API.
type FederationHandlers struct {
	DB *directorydb.DB
}

// FederationHandler handles federation management on /api/v1/admin/federation.
func (h *FederationHandlers) FederationHandler() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		switch r.Method {
		case http.MethodGet:
			fed, err := h.DB.GetFederation()
			if err != nil || fed == nil {
				jsonOK(w, map[string]any{"federation": nil})
				return
			}
			jsonOK(w, fed)

		case http.MethodPost:
			var req struct {
				Name string `json:"name"`
			}
			if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
				jsonErr(w, http.StatusBadRequest, "invalid JSON")
				return
			}
			if req.Name == "" {
				jsonErr(w, http.StatusBadRequest, "name is required")
				return
			}
			fed, err := h.DB.CreateFederation(req.Name)
			if err != nil {
				jsonErr(w, http.StatusInternalServerError, err.Error())
				return
			}
			w.WriteHeader(http.StatusCreated)
			jsonOK(w, fed)

		case http.MethodDelete:
			fed, err := h.DB.GetFederation()
			if err != nil || fed == nil {
				jsonErr(w, http.StatusNotFound, "no federation configured")
				return
			}
			if err := h.DB.DeleteFederation(fed.ID); err != nil {
				jsonErr(w, http.StatusInternalServerError, err.Error())
				return
			}
			jsonOK(w, map[string]string{"status": "deleted"})

		default:
			jsonErr(w, http.StatusMethodNotAllowed, "method not allowed")
		}
	}
}

// FederationPeersHandler handles listing and adding federation peers.
func (h *FederationHandlers) FederationPeersHandler() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		fed, err := h.DB.GetFederation()
		if err != nil || fed == nil {
			jsonErr(w, http.StatusNotFound, "no federation configured")
			return
		}

		switch r.Method {
		case http.MethodGet:
			peers, err := h.DB.ListFederationPeers(fed.ID)
			if err != nil {
				jsonErr(w, http.StatusInternalServerError, err.Error())
				return
			}
			if peers == nil {
				peers = []directorydb.FederationPeer{}
			}
			jsonOK(w, map[string]any{"items": peers})

		case http.MethodPost:
			var req struct {
				Token string `json:"token"`
			}
			if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
				jsonErr(w, http.StatusBadRequest, "invalid JSON")
				return
			}
			if req.Token == "" {
				jsonErr(w, http.StatusBadRequest, "token is required")
				return
			}
			peer, err := h.DB.AddFederationPeer(fed.ID, req.Token)
			if err != nil {
				jsonErr(w, http.StatusInternalServerError, err.Error())
				return
			}
			w.WriteHeader(http.StatusCreated)
			jsonOK(w, peer)

		default:
			jsonErr(w, http.StatusMethodNotAllowed, "method not allowed")
		}
	}
}
