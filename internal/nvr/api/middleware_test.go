package api

import (
	"crypto/rand"
	"crypto/rsa"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/golang-jwt/jwt/v5"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func generateTestKey(t *testing.T) *rsa.PrivateKey {
	t.Helper()
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	require.NoError(t, err)
	return key
}

func signTestToken(t *testing.T, key *rsa.PrivateKey, claims jwt.MapClaims) string {
	t.Helper()
	token := jwt.NewWithClaims(jwt.SigningMethodRS256, claims)
	token.Header["kid"] = "nvr-signing-key"
	signed, err := token.SignedString(key)
	require.NoError(t, err)
	return signed
}

func TestMiddlewareValidToken(t *testing.T) {
	key := generateTestKey(t)
	m := &Middleware{PrivateKey: key}

	now := time.Now()
	tokenStr := signTestToken(t, key, jwt.MapClaims{
		"sub":                "user-123",
		"role":               "admin",
		"camera_permissions": "*",
		"exp":                now.Add(15 * time.Minute).Unix(),
		"iat":                now.Unix(),
	})

	router := gin.New()
	router.Use(m.Handler())
	router.GET("/test", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{
			"user_id":            c.GetString("user_id"),
			"role":               c.GetString("role"),
			"camera_permissions": c.GetString("camera_permissions"),
		})
	})

	req := httptest.NewRequest(http.MethodGet, "/test", nil)
	req.Header.Set("Authorization", "Bearer "+tokenStr)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)
	assert.Contains(t, w.Body.String(), "user-123")
	assert.Contains(t, w.Body.String(), "admin")
}

func TestMiddlewareQueryToken(t *testing.T) {
	key := generateTestKey(t)
	m := &Middleware{PrivateKey: key}

	now := time.Now()
	tokenStr := signTestToken(t, key, jwt.MapClaims{
		"sub":  "user-456",
		"role": "viewer",
		"exp":  now.Add(15 * time.Minute).Unix(),
		"iat":  now.Unix(),
	})

	router := gin.New()
	router.Use(m.Handler())
	router.GET("/test", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{"user_id": c.GetString("user_id")})
	})

	req := httptest.NewRequest(http.MethodGet, "/test?token="+tokenStr, nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)
	assert.Contains(t, w.Body.String(), "user-456")
}

func TestMiddlewareNoToken(t *testing.T) {
	key := generateTestKey(t)
	m := &Middleware{PrivateKey: key}

	router := gin.New()
	router.Use(m.Handler())
	router.GET("/test", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{"ok": true})
	})

	req := httptest.NewRequest(http.MethodGet, "/test", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusUnauthorized, w.Code)
	assert.Contains(t, w.Body.String(), "missing token")
}

func TestMiddlewareExpiredToken(t *testing.T) {
	key := generateTestKey(t)
	m := &Middleware{PrivateKey: key}

	past := time.Now().Add(-1 * time.Hour)
	tokenStr := signTestToken(t, key, jwt.MapClaims{
		"sub":  "user-789",
		"role": "admin",
		"exp":  past.Unix(),
		"iat":  past.Add(-15 * time.Minute).Unix(),
	})

	router := gin.New()
	router.Use(m.Handler())
	router.GET("/test", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{"ok": true})
	})

	req := httptest.NewRequest(http.MethodGet, "/test", nil)
	req.Header.Set("Authorization", "Bearer "+tokenStr)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusUnauthorized, w.Code)
	assert.Contains(t, w.Body.String(), "invalid or expired token")
}
