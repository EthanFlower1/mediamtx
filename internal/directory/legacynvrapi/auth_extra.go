package legacynvrapi

// auth_extra.go — Additional auth endpoints for the legacy NVR API.
//
// PUT /api/nvr/auth/password — Change password for the authenticated user.
//
// The caller must supply:
//   Authorization: Bearer <access_token>   (JWT issued at login)
//   Body: { "current_password": "...", "new_password": "..." }
//
// Steps:
//   1. Extract user ID from JWT sub claim (falls back to username lookup via
//      the "preferred_username" claim or a "user_id" query param for clients
//      that cannot set headers).
//   2. Fetch user record from directory DB.
//   3. Verify current password against stored bcrypt or argon2 hash.
//   4. Hash new password with bcrypt.
//   5. Update user record.

import (
	"errors"
	"net/http"
	"strings"

	"github.com/golang-jwt/jwt/v5"
	"golang.org/x/crypto/bcrypt"
	"github.com/matthewhartstonge/argon2"

	dirdb "github.com/bluenviron/mediamtx/internal/directory/db"
)

// passwordChangeRequest is the body for PUT /api/nvr/auth/password.
type passwordChangeRequest struct {
	CurrentPassword string `json:"current_password"`
	NewPassword     string `json:"new_password"`
}

func (h *Handlers) authPasswordChange(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPut {
		http.Error(w, `{"error":"method not allowed"}`, http.StatusMethodNotAllowed)
		return
	}

	if h.DB == nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{
			"code":    "DB_UNAVAILABLE",
			"message": "database not available",
		})
		return
	}

	var req passwordChangeRequest
	if err := readJSON(r, &req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid JSON: " + err.Error()})
		return
	}
	if req.CurrentPassword == "" || req.NewPassword == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "current_password and new_password are required"})
		return
	}
	if len(req.NewPassword) < 8 {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "new_password must be at least 8 characters"})
		return
	}

	// Extract user identity from the JWT bearer token.
	userID, username := extractIdentityFromJWT(r)

	var user *dirdb.User
	var err error

	if userID != "" {
		user, err = h.DB.GetUser(userID)
	} else if username != "" {
		user, err = h.DB.GetUserByUsername(username)
	} else {
		writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "authentication required"})
		return
	}

	if err != nil {
		if errors.Is(err, dirdb.ErrNotFound) {
			writeJSON(w, http.StatusNotFound, map[string]string{"error": "user not found"})
			return
		}
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}

	// Verify the current password.
	if !verifyPasswordHash(req.CurrentPassword, user.PasswordHash) {
		writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "current password is incorrect"})
		return
	}

	// Hash the new password with bcrypt.
	hash, err := bcrypt.GenerateFromPassword([]byte(req.NewPassword), bcrypt.DefaultCost)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to hash password"})
		return
	}

	user.PasswordHash = string(hash)
	if err := h.DB.UpdateUser(user); err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}

	writeJSON(w, http.StatusOK, map[string]string{"status": "password updated"})
}

// -----------------------------------------------------------------------
// Auth sub-router
// -----------------------------------------------------------------------

func (h *Handlers) authSubrouter(w http.ResponseWriter, r *http.Request) {
	sub := strings.TrimPrefix(r.URL.Path, "/api/nvr/auth/")
	sub = strings.TrimSuffix(sub, "/")

	switch sub {
	case "password":
		h.authPasswordChange(w, r)
	default:
		// login, refresh, revoke are registered in boot.go — should not arrive here.
		h.notImplemented(w, r)
	}
}

// -----------------------------------------------------------------------
// Helpers
// -----------------------------------------------------------------------

// extractIdentityFromJWT parses the Authorization header bearer token and
// returns the (user_id, username) from the JWT claims without verifying the
// signature (the auth middleware upstream already validated it).
// On any parse failure it returns empty strings.
func extractIdentityFromJWT(r *http.Request) (userID, username string) {
	auth := r.Header.Get("Authorization")
	if !strings.HasPrefix(auth, "Bearer ") {
		return "", ""
	}
	tokenStr := strings.TrimPrefix(auth, "Bearer ")

	// Parse without validation — we just need the claims.
	p := jwt.NewParser()
	token, _, err := p.ParseUnverified(tokenStr, jwt.MapClaims{})
	if err != nil {
		return "", ""
	}

	claims, ok := token.Claims.(jwt.MapClaims)
	if !ok {
		return "", ""
	}

	if sub, ok := claims["sub"].(string); ok && sub != "" {
		userID = sub
	}
	if un, ok := claims["preferred_username"].(string); ok && un != "" {
		username = un
	}

	return userID, username
}

// verifyPasswordHash verifies a plain-text password against a stored hash.
// Supports both bcrypt ($2a$/$2b$/$2y$ prefix) and argon2 encoded strings.
func verifyPasswordHash(password, encoded string) bool {
	if encoded == "" {
		return false
	}
	// Bcrypt hashes start with $2.
	if len(encoded) > 3 && encoded[0] == '$' && encoded[1] == '2' {
		return bcrypt.CompareHashAndPassword([]byte(encoded), []byte(password)) == nil
	}
	// Argon2 encoded format.
	ok, err := argon2.VerifyEncoded([]byte(password), []byte(encoded))
	return ok && err == nil
}
