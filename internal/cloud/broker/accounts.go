package broker

import (
	"net/http"
	"strings"
)

// authenticateRequest extracts an API key from the X-API-Key header or the
// Authorization: Bearer header, validates it against the store, and returns
// the associated tenant ID. If authentication fails it writes a 401 response
// and returns an empty string.
func authenticateRequest(store *Store, w http.ResponseWriter, r *http.Request) string {
	key := r.Header.Get("X-API-Key")
	if key == "" {
		if auth := r.Header.Get("Authorization"); strings.HasPrefix(auth, "Bearer ") {
			key = strings.TrimPrefix(auth, "Bearer ")
		}
	}

	if key == "" {
		writeJSON(w, http.StatusUnauthorized, map[string]string{
			"error": "missing API key",
		})
		return ""
	}

	tenantID, err := store.ValidateAPIKey(key)
	if err != nil {
		writeJSON(w, http.StatusUnauthorized, map[string]string{
			"error": "invalid API key",
		})
		return ""
	}

	return tenantID
}

// AccountHandler returns an http.Handler for GET /api/v1/account.
// It authenticates via API key and returns tenant info.
func AccountHandler(store *Store) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}

		tenantID := authenticateRequest(store, w, r)
		if tenantID == "" {
			return
		}

		tenant, err := store.GetTenant(tenantID)
		if err != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]string{
				"error": "failed to retrieve account",
			})
			return
		}

		writeJSON(w, http.StatusOK, tenant)
	})
}

// ListKeysHandler returns an http.Handler for GET /api/v1/account/keys.
// It authenticates via API key and returns all API key metadata for the tenant.
func ListKeysHandler(store *Store) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}

		tenantID := authenticateRequest(store, w, r)
		if tenantID == "" {
			return
		}

		keys, err := store.ListAPIKeys(tenantID)
		if err != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]string{
				"error": "failed to list API keys",
			})
			return
		}

		writeJSON(w, http.StatusOK, map[string]interface{}{
			"keys": keys,
		})
	})
}
