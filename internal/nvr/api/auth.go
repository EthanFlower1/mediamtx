package api

import (
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/golang-jwt/jwt/v5"
	"github.com/matthewhartstonge/argon2"

	"github.com/bluenviron/mediamtx/internal/nvr/db"
)

// loginAttempts tracks per-IP login attempt counts for rate limiting.
var loginAttempts = struct {
	sync.Mutex
	counts map[string]int
	resets map[string]time.Time
}{
	counts: make(map[string]int),
	resets: make(map[string]time.Time),
}

// checkLoginRateLimit returns true if the IP is allowed to attempt login.
func checkLoginRateLimit(ip string) bool {
	loginAttempts.Lock()
	defer loginAttempts.Unlock()

	if reset, ok := loginAttempts.resets[ip]; ok && time.Now().After(reset) {
		delete(loginAttempts.counts, ip)
		delete(loginAttempts.resets, ip)
	}

	count := loginAttempts.counts[ip]
	if count >= 10 {
		return false
	}

	loginAttempts.counts[ip]++
	if _, ok := loginAttempts.resets[ip]; !ok {
		loginAttempts.resets[ip] = time.Now().Add(15 * time.Minute)
	}
	return true
}

// sanitizeURL redacts credentials from a URL for safe logging.
func sanitizeURL(rawURL string) string {
	u, err := url.Parse(rawURL)
	if err != nil {
		return rawURL
	}
	if u.User != nil {
		u.User = url.UserPassword("***", "***")
	}
	return u.String()
}

// AuthHandler implements authentication endpoints.
type AuthHandler struct {
	DB         *db.DB
	PrivateKey *rsa.PrivateKey
	Audit      *AuditLogger
}

// buildRolePermissions retrieves the permissions string for the user's assigned role.
func (h *AuthHandler) buildRolePermissions(user *db.User) string {
	if user.Role == "admin" {
		return `["view_live","view_playback","export","ptz_control","admin"]`
	}

	// Try role_id first (new granular RBAC).
	if user.RoleID != "" {
		role, err := h.DB.GetRole(user.RoleID)
		if err == nil {
			return role.Permissions
		}
	}

	// Fall back to legacy role name lookup.
	role, err := h.DB.GetRoleByName(user.Role)
	if err == nil {
		return role.Permissions
	}

	return "[]"
}

// buildPerCameraPermissions returns a JSON-encoded map of camera_id -> permissions
// for embedding in the JWT.
func (h *AuthHandler) buildPerCameraPermissions(userID string) string {
	perms, err := h.DB.ListCameraPermissions(userID)
	if err != nil || len(perms) == 0 {
		return ""
	}

	result := make(map[string]json.RawMessage, len(perms))
	for _, p := range perms {
		result[p.CameraID] = json.RawMessage(p.Permissions)
	}

	data, err := json.Marshal(result)
	if err != nil {
		return ""
	}
	return string(data)
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
		apiError(c, http.StatusInternalServerError, "failed to check user count", err)
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

	hashed, err := hashPassword(req.Password)
	if err != nil {
		apiError(c, http.StatusInternalServerError, "failed to hash password", err)
		return
	}

	user := &db.User{
		Username:          req.Username,
		PasswordHash:      hashed,
		Role:              "admin",
		CameraPermissions: "*",
	}
	if err := h.DB.CreateUser(user); err != nil {
		apiError(c, http.StatusInternalServerError, "failed to create initial admin user", err)
		return
	}

	nvrLogInfo("auth", "Initial admin setup completed for user "+user.Username)
	c.JSON(http.StatusCreated, gin.H{"id": user.ID, "username": user.Username, "role": user.Role})
}

// Login validates credentials and issues a JWT access token and refresh token.
func (h *AuthHandler) Login(c *gin.Context) {
	if !checkLoginRateLimit(c.ClientIP()) {
		c.JSON(http.StatusTooManyRequests, gin.H{"error": "too many login attempts, try again later"})
		return
	}

	var req loginRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid request"})
		return
	}

	user, err := h.DB.GetUserByUsername(req.Username)
	if err != nil {
		if h.Audit != nil {
			h.Audit.logLoginAttempt(c, "", req.Username, false)
		}
		c.JSON(http.StatusUnauthorized, gin.H{"error": "invalid credentials"})
		return
	}

	if !verifyPassword(req.Password, user.PasswordHash) {
		if h.Audit != nil {
			h.Audit.logLoginAttempt(c, user.ID, req.Username, false)
		}
		c.JSON(http.StatusUnauthorized, gin.H{"error": "invalid credentials"})
		return
	}

	// Build access token.
	now := time.Now()
	accessToken, err := h.buildAccessToken(user, now)
	if err != nil {
		apiError(c, http.StatusInternalServerError, "failed to generate access token", err)
		return
	}

	// Build refresh token.
	rawToken, err := generateRandomToken()
	if err != nil {
		apiError(c, http.StatusInternalServerError, "failed to generate refresh token", err)
		return
	}
	tokenHash := sha256Hash(rawToken)
	refreshExpiry := now.Add(7 * 24 * time.Hour) // 7 days

	rt := &db.RefreshToken{
		UserID:    user.ID,
		TokenHash: tokenHash,
		ExpiresAt: refreshExpiry.UTC().Format("2006-01-02T15:04:05.000Z"),
		IPAddress: c.ClientIP(),
		UserAgent: c.GetHeader("User-Agent"),
		DeviceName: parseDeviceName(c.GetHeader("User-Agent")),
	}
	if err := h.DB.CreateRefreshToken(rt); err != nil {
		apiError(c, http.StatusInternalServerError, "failed to store refresh token", err)
		return
	}

	// Re-build access token with session ID included.
	accessToken, err = h.buildAccessTokenWithSession(user, now, rt.ID)
	if err != nil {
		apiError(c, http.StatusInternalServerError, "failed to generate access token", err)
		return
	}

	c.SetSameSite(http.SameSiteStrictMode)
	c.SetCookie("refresh_token", rawToken, 7*24*3600, "/", "", false, true)

	if h.Audit != nil {
		h.Audit.logLoginAttempt(c, user.ID, user.Username, true)
	}

	c.JSON(http.StatusOK, gin.H{
		"access_token": accessToken,
		"token_type":   "Bearer",
		"expires_in":   900,
		"user": gin.H{
			"id":       user.ID,
			"username": user.Username,
			"role":     user.Role,
		},
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

	// Update session activity on refresh.
	_ = h.DB.UpdateSessionActivity(rt.ID, c.ClientIP())

	now := time.Now()
	accessToken, err := h.buildAccessTokenWithSession(user, now, rt.ID)
	if err != nil {
		apiError(c, http.StatusInternalServerError, "failed to generate access token on refresh", err)
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"access_token": accessToken,
		"token_type":   "Bearer",
		"expires_in":   900,
		"user": gin.H{
			"id":       user.ID,
			"username": user.Username,
			"role":     user.Role,
		},
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
		apiError(c, http.StatusInternalServerError, "failed to revoke token", err)
		return
	}

	// Clear the cookie.
	c.SetSameSite(http.SameSiteStrictMode)
	c.SetCookie("refresh_token", "", -1, "/", "", false, true)

	c.JSON(http.StatusOK, gin.H{"message": "token revoked"})
}

// buildAccessToken creates a signed RS256 JWT for the given user.
func (h *AuthHandler) buildAccessToken(user *db.User, now time.Time) (string, error) {
	return h.buildAccessTokenWithSession(user, now, "")
}

// buildAccessTokenWithSession creates a signed RS256 JWT that includes the session ID.
func (h *AuthHandler) buildAccessTokenWithSession(user *db.User, now time.Time, sessionID string) (string, error) {
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
	if sessionID != "" {
		claims["session_id"] = sessionID
	}

	// Add granular RBAC claims.
	rolePerms := h.buildRolePermissions(user)
	if rolePerms != "" {
		claims["role_permissions"] = rolePerms
	}
	pcPerms := h.buildPerCameraPermissions(user.ID)
	if pcPerms != "" {
		claims["per_camera_permissions"] = pcPerms
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
func hashPassword(password string) (string, error) {
	cfg := argon2.DefaultConfig()
	encoded, err := cfg.HashEncoded([]byte(password))
	if err != nil {
		return "", fmt.Errorf("hash password: %w", err)
	}
	return string(encoded), nil
}

// verifyPassword checks a password against an argon2 encoded hash.
func verifyPassword(password, encoded string) bool {
	ok, err := argon2.VerifyEncoded([]byte(password), []byte(encoded))
	return ok && err == nil
}

// generateRandomToken returns a 32-byte hex-encoded random string.
func generateRandomToken() (string, error) {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		return "", fmt.Errorf("generate token: %w", err)
	}
	return hex.EncodeToString(b), nil
}

// sha256Hash returns the hex-encoded SHA-256 hash of s.
func sha256Hash(s string) string {
	h := sha256.Sum256([]byte(s))
	return hex.EncodeToString(h[:])
}

// parseDeviceName extracts a human-readable device name from the User-Agent header.
func parseDeviceName(ua string) string {
	if ua == "" {
		return "Unknown"
	}

	// Check for common patterns.
	switch {
	case contains(ua, "iPhone"):
		return "iPhone"
	case contains(ua, "iPad"):
		return "iPad"
	case contains(ua, "Android"):
		return "Android Device"
	case contains(ua, "Windows"):
		return "Windows PC"
	case contains(ua, "Macintosh") || contains(ua, "Mac OS"):
		return "Mac"
	case contains(ua, "Linux"):
		return "Linux PC"
	case contains(ua, "Flutter") || contains(ua, "Dart"):
		return "Mobile App"
	default:
		// Truncate to something reasonable.
		if len(ua) > 50 {
			return ua[:50]
		}
		return ua
	}
}

// contains checks if s contains substr (case-insensitive would be better,
// but User-Agent strings are reliably cased).
func contains(s, substr string) bool {
	return len(s) >= len(substr) && strings.Contains(s, substr)
}
