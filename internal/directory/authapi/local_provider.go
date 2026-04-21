package authapi

// LocalAuthProvider is a concrete AuthProvider backed by the on-prem
// Directory SQLite database. It handles:
//   - Argon2 password verification
//   - Per-account brute-force lockout
//   - RS256 JWT access-token issuance
//   - Refresh-token rotation (stored as hashed row in refresh_tokens table)
//   - Session revocation by token-table row ID

import (
	"context"
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"github.com/matthewhartstonge/argon2"
	"golang.org/x/crypto/bcrypt"

	directorydb "github.com/bluenviron/mediamtx/internal/directory/db"
)

// LocalAuthProvider implements AuthProvider using the Directory SQLite DB.
type LocalAuthProvider struct {
	db         *directorydb.DB
	privateKey *rsa.PrivateKey
}

// NewLocalAuthProvider returns a LocalAuthProvider.
// privateKey is the RSA key used to sign JWTs; derive it from nvrJWTSecret
// using deriveRSAKey or load it from the PKI store.
func NewLocalAuthProvider(db *directorydb.DB, privateKey *rsa.PrivateKey) *LocalAuthProvider {
	return &LocalAuthProvider{db: db, privateKey: privateKey}
}

// AuthenticateLocal verifies credentials, enforces lockout, and issues a fresh
// session on success. On any failure it returns a generic error to avoid
// leaking which check failed.
func (p *LocalAuthProvider) AuthenticateLocal(
	_ context.Context,
	_ TenantRef,
	username, password string,
) (*Session, error) {
	user, err := p.db.GetUserByUsername(username)
	if err != nil {
		// User not found — return generic error.
		return nil, errors.New("invalid credentials")
	}

	// Check account lockout.
	if user.LockedUntil != nil && *user.LockedUntil != "" {
		lockedUntil, parseErr := time.Parse(timeFormat, *user.LockedUntil)
		if parseErr == nil && time.Now().Before(lockedUntil) {
			return nil, fmt.Errorf("account locked until %s", lockedUntil.Format(time.RFC3339))
		}
		// Lock expired — clear it.
		_ = p.db.UnlockUser(user.ID)
		user.FailedLoginAttempts = 0
		user.LockedUntil = nil
	}

	if !verifyPassword(password, user.PasswordHash) {
		_ = p.db.IncrementFailedLogins(user.ID)
		newCount := user.FailedLoginAttempts + 1
		if newCount >= 5 {
			lockUntil := time.Now().Add(15 * time.Minute)
			_ = p.db.LockUser(user.ID, lockUntil)
		}
		return nil, errors.New("invalid credentials")
	}

	// Successful — reset counters.
	if user.FailedLoginAttempts > 0 {
		_ = p.db.ResetFailedLogins(user.ID)
	}

	return p.issueSession(user)
}

// RefreshSession validates a raw refresh token and rotates it.
func (p *LocalAuthProvider) RefreshSession(_ context.Context, rawToken string) (*Session, error) {
	hash := sha256Hex(rawToken)
	rt, err := p.db.GetRefreshToken(hash)
	if err != nil {
		return nil, errors.New("invalid refresh token")
	}
	if rt.RevokedAt != nil {
		return nil, errors.New("refresh token revoked")
	}
	expiresAt, err := time.Parse(timeFormat, rt.ExpiresAt)
	if err != nil || time.Now().After(expiresAt) {
		return nil, errors.New("refresh token expired")
	}

	user, err := p.db.GetUser(rt.UserID)
	if err != nil {
		return nil, errors.New("user not found")
	}

	// Revoke old token and issue new one.
	_ = p.db.RevokeRefreshToken(rt.ID)
	return p.issueSession(user)
}

// RevokeSession revokes a session by its refresh-token row ID (the session ID
// used throughout the authapi package is the refresh_tokens.id value).
func (p *LocalAuthProvider) RevokeSession(_ context.Context, sessionID SessionID) error {
	err := p.db.RevokeRefreshToken(string(sessionID))
	if errors.Is(err, directorydb.ErrNotFound) {
		return nil // idempotent
	}
	return err
}

// issueSession creates a refresh-token DB row and builds a signed JWT.
func (p *LocalAuthProvider) issueSession(user *directorydb.User) (*Session, error) {
	rawToken, err := generateHexToken()
	if err != nil {
		return nil, fmt.Errorf("generate refresh token: %w", err)
	}
	tokenHash := sha256Hex(rawToken)
	now := time.Now()
	refreshExpiry := now.Add(7 * 24 * time.Hour)

	rt := &directorydb.RefreshToken{
		UserID:     user.ID,
		TokenHash:  tokenHash,
		ExpiresAt:  refreshExpiry.UTC().Format(timeFormat),
		DeviceName: "unknown",
	}
	if err := p.db.CreateRefreshToken(rt); err != nil {
		return nil, fmt.Errorf("store refresh token: %w", err)
	}

	// Build access JWT that includes the session (= rt.ID) so RevokeSession works.
	accessToken, err := p.buildJWT(user, now, rt.ID)
	if err != nil {
		return nil, fmt.Errorf("build access token: %w", err)
	}

	return &Session{
		ID:           SessionID(rt.ID),
		UserID:       user.ID,
		AccessToken:  accessToken,
		RefreshToken: rawToken,
		ExpiresAt:    now.Add(15 * time.Minute),
	}, nil
}

// buildJWT creates an RS256-signed JWT for the user.
func (p *LocalAuthProvider) buildJWT(user *directorydb.User, now time.Time, sessionID string) (string, error) {
	claims := jwt.MapClaims{
		"sub":                  user.ID,
		"username":             user.Username,
		"role":                 user.Role,
		"camera_permissions":   user.CameraPermissions,
		"mediamtx_permissions": buildMediaMTXPerms(user),
		"exp":                  now.Add(15 * time.Minute).Unix(),
		"iat":                  now.Unix(),
		"kid":                  "nvr-signing-key",
	}
	if sessionID != "" {
		claims["session_id"] = sessionID
	}

	// Attach role/camera permissions from DB.
	rolePerms, camPerms := p.lookupRolePerms(user)
	if rolePerms != "" {
		claims["role_permissions"] = rolePerms
	}
	if camPerms != "" {
		claims["camera_specific_permissions"] = camPerms
	}

	token := jwt.NewWithClaims(jwt.SigningMethodRS256, claims)
	token.Header["kid"] = "nvr-signing-key"
	return token.SignedString(p.privateKey)
}

func (p *LocalAuthProvider) lookupRolePerms(user *directorydb.User) (rolePerms, camPerms string) {
	var role *directorydb.Role
	if user.RoleID != "" {
		role, _ = p.db.GetRole(user.RoleID)
	}
	if role == nil {
		role, _ = p.db.GetRoleByName(user.Role)
	}
	if role != nil {
		if b, err := json.Marshal(role.Permissions); err == nil {
			rolePerms = string(b)
		}
	}

	cams, err := p.db.GetUserCameraPermissions(user.ID)
	if err == nil && len(cams) > 0 {
		m := make(map[string][]string, len(cams))
		for _, c := range cams {
			m[c.CameraID] = c.Permissions
		}
		if b, err := json.Marshal(m); err == nil {
			camPerms = string(b)
		}
	}
	return
}

// buildMediaMTXPerms returns the mediamtx_permissions claim array.
func buildMediaMTXPerms(user *directorydb.User) []map[string]any {
	if user.Role == "admin" {
		return []map[string]any{
			{"action": "publish", "path": "*"},
			{"action": "read", "path": "*"},
			{"action": "playback", "path": "*"},
			{"action": "api", "path": "*"},
			{"action": "pprof", "path": "*"},
			{"action": "metrics", "path": "*"},
		}
	}

	var perms []map[string]any
	if user.CameraPermissions == "*" {
		perms = append(perms, map[string]any{"action": "read", "path": "*"})
	} else if user.CameraPermissions != "" {
		var paths []string
		if err := json.Unmarshal([]byte(user.CameraPermissions), &paths); err == nil {
			for _, path := range paths {
				perms = append(perms, map[string]any{"action": "read", "path": path})
			}
		}
	}
	return perms
}

// timeFormat is the DB timestamp layout.
const timeFormat = "2006-01-02T15:04:05.000Z"

// verifyPassword checks a password against either a bcrypt or argon2 hash.
func verifyPassword(password, encoded string) bool {
	// Bcrypt hashes start with $2a$, $2b$, or $2y$
	if len(encoded) > 3 && encoded[0] == '$' && encoded[1] == '2' {
		return bcrypt.CompareHashAndPassword([]byte(encoded), []byte(password)) == nil
	}
	// Fall back to argon2
	ok, err := argon2.VerifyEncoded([]byte(password), []byte(encoded))
	return ok && err == nil
}

// generateHexToken returns a 32-byte cryptographically random hex string.
func generateHexToken() (string, error) {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return hex.EncodeToString(b), nil
}

// sha256Hex returns the hex-encoded SHA-256 digest of s.
func sha256Hex(s string) string {
	h := sha256.Sum256([]byte(s))
	return hex.EncodeToString(h[:])
}

// parseDeviceFromUA extracts a readable device label from a User-Agent string.
func parseDeviceFromUA(ua string) string {
	if ua == "" {
		return "Unknown"
	}
	switch {
	case strings.Contains(ua, "iPhone"):
		return "iPhone"
	case strings.Contains(ua, "iPad"):
		return "iPad"
	case strings.Contains(ua, "Android"):
		return "Android Device"
	case strings.Contains(ua, "Windows"):
		return "Windows PC"
	case strings.Contains(ua, "Macintosh") || strings.Contains(ua, "Mac OS"):
		return "Mac"
	case strings.Contains(ua, "Linux"):
		return "Linux PC"
	case strings.Contains(ua, "Flutter") || strings.Contains(ua, "Dart"):
		return "Mobile App"
	default:
		if len(ua) > 50 {
			return ua[:50]
		}
		return ua
	}
}
