package broker

import (
	"encoding/json"
	"errors"
	"net/http"
	"strings"
	"time"
)

// SessionTTL is the lifetime of a browser session cookie.
const SessionTTL = 14 * 24 * time.Hour

// SessionCookieName is the name of the cookie that carries the opaque session token.
const SessionCookieName = "raikada_session"

// SessionConfig controls how cookies are issued. CookieDomain should be ".raikada.com"
// in production so subdomains share the session; empty in dev means default
// to the request host (localhost).
type SessionConfig struct {
	CookieDomain string
	// SecureCookies should be true in production. False allows dev over HTTP.
	SecureCookies bool
}

// LoginRequest is the JSON body for POST /api/v1/login.
type LoginRequest struct {
	Email    string `json:"email"`
	Password string `json:"password"`
}

// SessionResponse is returned by /login and /session: the current user + tenant.
type SessionResponse struct {
	User   *User   `json:"user"`
	Tenant *Tenant `json:"tenant"`
}

// LoginHandler verifies email+password, creates a session, sets the cookie,
// and returns the user+tenant.
func LoginHandler(store *Store, cfg SessionConfig) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		var req LoginRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid JSON body"})
			return
		}
		req.Email = strings.TrimSpace(req.Email)
		if req.Email == "" || req.Password == "" {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "email and password are required"})
			return
		}
		user, err := store.VerifyPassword(req.Email, req.Password)
		if err != nil {
			// Always return the same error for unknown user vs wrong password.
			writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "invalid credentials"})
			return
		}
		token, err := store.CreateSession(user.ID, user.TenantID, SessionTTL)
		if err != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to create session"})
			return
		}
		setSessionCookie(w, cfg, token, SessionTTL)
		tenant, err := store.GetTenant(user.TenantID)
		if err != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to load tenant"})
			return
		}
		writeJSON(w, http.StatusOK, SessionResponse{User: user, Tenant: tenant})
	})
}

// LogoutHandler revokes the current session and clears the cookie.
// Idempotent: returns 204 even if there was no session.
func LogoutHandler(store *Store, cfg SessionConfig) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		if c, err := r.Cookie(SessionCookieName); err == nil {
			_ = store.RevokeSession(c.Value)
		}
		clearSessionCookie(w, cfg)
		w.WriteHeader(http.StatusNoContent)
	})
}

// SessionHandler returns the current user+tenant or 401.
func SessionHandler(store *Store) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		user, err := authenticateSession(store, r)
		if err != nil {
			writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "unauthenticated"})
			return
		}
		tenant, err := store.GetTenant(user.TenantID)
		if err != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to load tenant"})
			return
		}
		writeJSON(w, http.StatusOK, SessionResponse{User: user, Tenant: tenant})
	})
}

// RefreshHandler rotates the session: revokes the old token and issues a new one.
func RefreshHandler(store *Store, cfg SessionConfig) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		c, err := r.Cookie(SessionCookieName)
		if err != nil {
			writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "no session"})
			return
		}
		newToken, err := store.RotateSession(c.Value, SessionTTL)
		if err != nil {
			clearSessionCookie(w, cfg)
			writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "session invalid"})
			return
		}
		setSessionCookie(w, cfg, newToken, SessionTTL)
		w.WriteHeader(http.StatusNoContent)
	})
}

// authenticateSession extracts the session cookie, validates it, and returns the user.
func authenticateSession(store *Store, r *http.Request) (*User, error) {
	c, err := r.Cookie(SessionCookieName)
	if err != nil {
		return nil, err
	}
	user, err := store.ValidateSession(c.Value)
	if err != nil {
		return nil, err
	}
	if user == nil {
		return nil, errors.New("nil user")
	}
	return user, nil
}

// setSessionCookie writes the session cookie with the right attrs.
func setSessionCookie(w http.ResponseWriter, cfg SessionConfig, token string, ttl time.Duration) {
	http.SetCookie(w, &http.Cookie{
		Name:     SessionCookieName,
		Value:    token,
		Path:     "/",
		Domain:   cfg.CookieDomain,
		Expires:  time.Now().Add(ttl),
		MaxAge:   int(ttl.Seconds()),
		HttpOnly: true,
		Secure:   cfg.SecureCookies,
		SameSite: http.SameSiteLaxMode,
	})
}

func clearSessionCookie(w http.ResponseWriter, cfg SessionConfig) {
	http.SetCookie(w, &http.Cookie{
		Name:     SessionCookieName,
		Value:    "",
		Path:     "/",
		Domain:   cfg.CookieDomain,
		Expires:  time.Unix(0, 0),
		MaxAge:   -1,
		HttpOnly: true,
		Secure:   cfg.SecureCookies,
		SameSite: http.SameSiteLaxMode,
	})
}
