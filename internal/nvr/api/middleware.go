// Package api provides HTTP handlers and middleware for the NVR API.
package api

import (
	"crypto/rsa"
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/golang-jwt/jwt/v5"
)

// Middleware validates JWT tokens on incoming requests.
type Middleware struct {
	PrivateKey *rsa.PrivateKey
}

// Handler returns a gin.HandlerFunc that validates JWT tokens.
// It extracts the token from the Authorization header ("Bearer ...")
// or the "token" query parameter (for SSE connections).
// On success it sets "user_id", "role", and "camera_permissions" on the context.
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
		if role, ok := claims["role"].(string); ok {
			c.Set("role", role)
		}
		if perms, ok := claims["camera_permissions"].(string); ok {
			c.Set("camera_permissions", perms)
		}

		c.Next()
	}
}
