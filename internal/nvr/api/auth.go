package api

import (
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/golang-jwt/jwt/v5"
	"github.com/matthewhartstonge/argon2"

	"github.com/bluenviron/mediamtx/internal/nvr/db"
)

// AuthHandler implements authentication endpoints.
type AuthHandler struct {
	DB         *db.DB
	PrivateKey *rsa.PrivateKey
}

// setupRequest is the JSON body for the initial admin setup.
type setupRequest struct {
	Username string `json:"username" binding:"required"`
	Password string `json:"password" binding:"required"`
}

// loginRequest is the JSON body for login.
type loginRequest struct {
	Username string `json:"username" binding:"required"`
	Password string `json:"password" binding:"required"`
}

// Setup creates the initial admin user. It only works when no users exist.
func (h *AuthHandler) Setup(c *gin.Context) {
	count, err := h.DB.CountUsers()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "database error"})
		return
	}
	if count > 0 {
		c.JSON(http.StatusConflict, gin.H{"error": "setup already completed"})
		return
	}

	var req setupRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid request"})
		return
	}

	user := &db.User{
		Username:          req.Username,
		PasswordHash:      hashPassword(req.Password),
		Role:              "admin",
		CameraPermissions: "*",
	}
	if err := h.DB.CreateUser(user); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to create user"})
		return
	}

	c.JSON(http.StatusCreated, gin.H{"id": user.ID, "username": user.Username, "role": user.Role})
}

// Login validates credentials and issues a JWT access token and refresh token.
func (h *AuthHandler) Login(c *gin.Context) {
	var req loginRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid request"})
		return
	}

	user, err := h.DB.GetUserByUsername(req.Username)
	if err != nil {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "invalid credentials"})
		return
	}

	if !verifyPassword(req.Password, user.PasswordHash) {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "invalid credentials"})
		return
	}

	// Build access token.
	now := time.Now()
	accessToken, err := h.buildAccessToken(user, now)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to generate token"})
		return
	}

	// Build refresh token.
	rawToken := generateRandomToken()
	tokenHash := sha256Hash(rawToken)
	refreshExpiry := now.Add(7 * 24 * time.Hour) // 7 days

	rt := &db.RefreshToken{
		UserID:    user.ID,
		TokenHash: tokenHash,
		ExpiresAt: refreshExpiry.UTC().Format("2006-01-02T15:04:05.000Z"),
	}
	if err := h.DB.CreateRefreshToken(rt); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to store refresh token"})
		return
	}

	c.SetSameSite(http.SameSiteStrictMode)
	c.SetCookie("refresh_token", rawToken, 7*24*3600, "/", "", false, true)

	c.JSON(http.StatusOK, gin.H{
		"access_token": accessToken,
		"token_type":   "Bearer",
		"expires_in":   900,
	})
}

// Refresh validates the refresh token cookie and issues a new access JWT.
func (h *AuthHandler) Refresh(c *gin.Context) {
	rawToken, err := c.Cookie("refresh_token")
	if err != nil || rawToken == "" {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "missing refresh token"})
		return
	}

	tokenHash := sha256Hash(rawToken)
	rt, err := h.DB.GetRefreshToken(tokenHash)
	if err != nil {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "invalid refresh token"})
		return
	}

	if rt.RevokedAt != nil {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "refresh token revoked"})
		return
	}

	expiresAt, err := time.Parse("2006-01-02T15:04:05.000Z", rt.ExpiresAt)
	if err != nil || time.Now().After(expiresAt) {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "refresh token expired"})
		return
	}

	user, err := h.DB.GetUser(rt.UserID)
	if err != nil {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "user not found"})
		return
	}

	now := time.Now()
	accessToken, err := h.buildAccessToken(user, now)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to generate token"})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"access_token": accessToken,
		"token_type":   "Bearer",
		"expires_in":   900,
	})
}

// Revoke revokes the current refresh token and clears the cookie.
func (h *AuthHandler) Revoke(c *gin.Context) {
	rawToken, err := c.Cookie("refresh_token")
	if err != nil || rawToken == "" {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "missing refresh token"})
		return
	}

	tokenHash := sha256Hash(rawToken)
	rt, err := h.DB.GetRefreshToken(tokenHash)
	if err != nil {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "invalid refresh token"})
		return
	}

	if err := h.DB.RevokeRefreshToken(rt.ID); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to revoke token"})
		return
	}

	// Clear the cookie.
	c.SetSameSite(http.SameSiteStrictMode)
	c.SetCookie("refresh_token", "", -1, "/", "", false, true)

	c.JSON(http.StatusOK, gin.H{"message": "token revoked"})
}

// buildAccessToken creates a signed RS256 JWT for the given user.
func (h *AuthHandler) buildAccessToken(user *db.User, now time.Time) (string, error) {
	claims := jwt.MapClaims{
		"sub":                  user.ID,
		"username":             user.Username,
		"role":                 user.Role,
		"camera_permissions":   user.CameraPermissions,
		"mediamtx_permissions": buildMediaMTXPermissions(user),
		"exp":                  now.Add(15 * time.Minute).Unix(),
		"iat":                  now.Unix(),
		"kid":                  "nvr-signing-key",
	}

	token := jwt.NewWithClaims(jwt.SigningMethodRS256, claims)
	token.Header["kid"] = "nvr-signing-key"

	return token.SignedString(h.PrivateKey)
}

// buildMediaMTXPermissions returns a permissions array for JWT claims
// based on the user's role and camera permissions.
func buildMediaMTXPermissions(user *db.User) []map[string]any {
	if user.Role == "admin" {
		return []map[string]any{
			{
				"action": "publish",
				"path":   "*",
			},
			{
				"action": "read",
				"path":   "*",
			},
			{
				"action": "playback",
				"path":   "*",
			},
			{
				"action": "api",
				"path":   "*",
			},
			{
				"action": "pprof",
				"path":   "*",
			},
			{
				"action": "metrics",
				"path":   "*",
			},
		}
	}

	// For non-admin users, build permissions based on camera_permissions.
	var perms []map[string]any

	// Parse camera_permissions — could be "*" or a JSON array of camera paths.
	if user.CameraPermissions == "*" {
		perms = append(perms, map[string]any{
			"action": "read",
			"path":   "*",
		})
	} else if user.CameraPermissions != "" {
		var paths []string
		if err := json.Unmarshal([]byte(user.CameraPermissions), &paths); err == nil {
			for _, p := range paths {
				perms = append(perms, map[string]any{
					"action": "read",
					"path":   p,
				})
			}
		}
	}

	return perms
}

// hashPassword hashes a password using argon2.
func hashPassword(password string) string {
	cfg := argon2.DefaultConfig()
	encoded, err := cfg.HashEncoded([]byte(password))
	if err != nil {
		panic("argon2 hash failed: " + err.Error())
	}
	return string(encoded)
}

// verifyPassword checks a password against an argon2 encoded hash.
func verifyPassword(password, encoded string) bool {
	ok, err := argon2.VerifyEncoded([]byte(password), []byte(encoded))
	return ok && err == nil
}

// generateRandomToken returns a 32-byte hex-encoded random string.
func generateRandomToken() string {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		panic("crypto/rand failed: " + err.Error())
	}
	return hex.EncodeToString(b)
}

// sha256Hash returns the hex-encoded SHA-256 hash of s.
func sha256Hash(s string) string {
	h := sha256.Sum256([]byte(s))
	return hex.EncodeToString(h[:])
}
