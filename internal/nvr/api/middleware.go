// Package api provides HTTP handlers and middleware for the NVR API.
package api

import (
	"crypto/rsa"
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/golang-jwt/jwt/v5"

	"github.com/bluenviron/mediamtx/internal/nvr/db"
)

// Middleware validates JWT tokens on incoming requests.
type Middleware struct {
	PrivateKey *rsa.PrivateKey
	DB         *db.DB // optional; when set, updates session activity on each request
}

// Handler returns a gin.HandlerFunc that validates JWT tokens.
// It extracts the token from the Authorization header ("Bearer ...")
// or the "token" query parameter (for SSE connections).
// On success it sets "user_id", "role", "camera_permissions", and "session_id" on the context.
func (m *Middleware) Handler() gin.HandlerFunc {
	return func(c *gin.Context) {
		tokenStr := ""

		// Try Authorization header first.
		if auth := c.GetHeader("Authorization"); strings.HasPrefix(auth, "Bearer ") {
			tokenStr = strings.TrimPrefix(auth, "Bearer ")
		}

		// Fall back to query parameter (for SSE).
		if tokenStr == "" {
			tokenStr = c.Query("token")
		}

		if tokenStr == "" {
			c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{"error": "missing token"})
			return
		}

		token, err := jwt.Parse(tokenStr, func(t *jwt.Token) (any, error) {
			if _, ok := t.Method.(*jwt.SigningMethodRSA); !ok {
				return nil, jwt.ErrSignatureInvalid
			}
			return &m.PrivateKey.PublicKey, nil
		})
		if err != nil || !token.Valid {
			c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{"error": "invalid or expired token"})
			return
		}

		claims, ok := token.Claims.(jwt.MapClaims)
		if !ok {
			c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{"error": "invalid claims"})
			return
		}

		if sub, ok := claims["sub"].(string); ok {
			c.Set("user_id", sub)
		}
		if username, ok := claims["username"].(string); ok {
			c.Set("username", username)
		}
		if role, ok := claims["role"].(string); ok {
			c.Set("role", role)
		}
		if perms, ok := claims["camera_permissions"].(string); ok {
			c.Set("camera_permissions", perms)
		}
		if sessionID, ok := claims["session_id"].(string); ok {
			c.Set("session_id", sessionID)

			// Update session activity in the background.
			if m.DB != nil && sessionID != "" {
				go func(sid, ip string) {
					_ = m.DB.UpdateSessionActivity(sid, ip)
				}(sessionID, c.ClientIP())
			}
		}

		c.Next()
	}
}
