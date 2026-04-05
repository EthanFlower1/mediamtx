package api

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/bluenviron/mediamtx/internal/nvr/db"
)

func setupTestDB(t *testing.T) *db.DB {
	t.Helper()
	dir := t.TempDir()
	d, err := db.Open(filepath.Join(dir, "test.db"))
	require.NoError(t, err)
	t.Cleanup(func() { d.Close() })
	return d
}

func TestSetupEndpoint(t *testing.T) {
	d := setupTestDB(t)
	key := generateTestKey(t)

	h := &AuthHandler{DB: d, PrivateKey: key}

	router := gin.New()
	router.POST("/api/nvr/auth/setup", h.Setup)

	// First setup should succeed.
	body, _ := json.Marshal(map[string]string{
		"username": "admin",
		"password": "secret123",
	})
	req := httptest.NewRequest(http.MethodPost, "/api/nvr/auth/setup", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusCreated, w.Code)

	var resp map[string]any
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
	assert.Equal(t, "admin", resp["username"])
	assert.Equal(t, "admin", resp["role"])
	assert.NotEmpty(t, resp["id"])

	// Second setup should fail.
	req2 := httptest.NewRequest(http.MethodPost, "/api/nvr/auth/setup", bytes.NewReader(body))
	req2.Header.Set("Content-Type", "application/json")
	w2 := httptest.NewRecorder()
	router.ServeHTTP(w2, req2)

	assert.Equal(t, http.StatusConflict, w2.Code)
}

func TestLoginEndpoint(t *testing.T) {
	d := setupTestDB(t)
	key := generateTestKey(t)

	// Create a user first via setup.
	h := &AuthHandler{DB: d, PrivateKey: key}

	router := gin.New()
	router.POST("/api/nvr/auth/setup", h.Setup)
	router.POST("/api/nvr/auth/login", h.Login)

	setupBody, _ := json.Marshal(map[string]string{
		"username": "admin",
		"password": "mypassword",
	})
	req := httptest.NewRequest(http.MethodPost, "/api/nvr/auth/setup", bytes.NewReader(setupBody))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)
	require.Equal(t, http.StatusCreated, w.Code)

	// Login with correct credentials.
	loginBody, _ := json.Marshal(map[string]string{
		"username": "admin",
		"password": "mypassword",
	})
	req2 := httptest.NewRequest(http.MethodPost, "/api/nvr/auth/login", bytes.NewReader(loginBody))
	req2.Header.Set("Content-Type", "application/json")
	w2 := httptest.NewRecorder()
	router.ServeHTTP(w2, req2)

	assert.Equal(t, http.StatusOK, w2.Code)

	var loginResp map[string]any
	require.NoError(t, json.Unmarshal(w2.Body.Bytes(), &loginResp))
	assert.NotEmpty(t, loginResp["access_token"])
	assert.Equal(t, "Bearer", loginResp["token_type"])
	assert.Equal(t, float64(900), loginResp["expires_in"])

	// Check that refresh_token cookie is set.
	cookies := w2.Result().Cookies()
	found := false
	for _, c := range cookies {
		if c.Name == "refresh_token" {
			found = true
			assert.True(t, c.HttpOnly)
			break
		}
	}
	assert.True(t, found, "refresh_token cookie should be set")

	// Login with wrong password.
	badBody, _ := json.Marshal(map[string]string{
		"username": "admin",
		"password": "wrongpassword",
	})
	req3 := httptest.NewRequest(http.MethodPost, "/api/nvr/auth/login", bytes.NewReader(badBody))
	req3.Header.Set("Content-Type", "application/json")
	w3 := httptest.NewRecorder()
	router.ServeHTTP(w3, req3)

	assert.Equal(t, http.StatusUnauthorized, w3.Code)
}

func TestAccountLockoutAfterFailedAttempts(t *testing.T) {
	d := setupTestDB(t)
	key := generateTestKey(t)

	audit := &AuditLogger{DB: d}
	h := &AuthHandler{
		DB:         d,
		PrivateKey: key,
		Audit:      audit,
		BruteForceConfig: BruteForceConfig{
			MaxFailedAttempts: 3,
			LockoutDuration:  1 * time.Minute,
		},
		rateLimiter: newLoginRateLimiter(BruteForceConfig{
			IPRateLimit:  100,
			IPRateWindow: 15 * time.Minute,
		}),
	}

	router := gin.New()
	router.POST("/api/nvr/auth/setup", h.Setup)
	router.POST("/api/nvr/auth/login", h.Login)

	// Create a user.
	setupBody, _ := json.Marshal(map[string]string{
		"username": "admin",
		"password": "correctpassword",
	})
	req := httptest.NewRequest(http.MethodPost, "/api/nvr/auth/setup", bytes.NewReader(setupBody))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)
	require.Equal(t, http.StatusCreated, w.Code)

	// Fail 3 times — should lock the account.
	badBody, _ := json.Marshal(map[string]string{
		"username": "admin",
		"password": "wrongpassword",
	})
	for i := 0; i < 3; i++ {
		req := httptest.NewRequest(http.MethodPost, "/api/nvr/auth/login", bytes.NewReader(badBody))
		req.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()
		router.ServeHTTP(w, req)
		if i < 2 {
			assert.Equal(t, http.StatusUnauthorized, w.Code, "attempt %d should be unauthorized", i+1)
		} else {
			// Third attempt triggers the lock.
			assert.Equal(t, http.StatusTooManyRequests, w.Code, "attempt %d should trigger lockout", i+1)
		}
	}

	// Now even correct password should be rejected (account locked).
	goodBody, _ := json.Marshal(map[string]string{
		"username": "admin",
		"password": "correctpassword",
	})
	req = httptest.NewRequest(http.MethodPost, "/api/nvr/auth/login", bytes.NewReader(goodBody))
	req.Header.Set("Content-Type", "application/json")
	w = httptest.NewRecorder()
	router.ServeHTTP(w, req)
	assert.Equal(t, http.StatusTooManyRequests, w.Code)

	var resp map[string]any
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
	assert.Contains(t, resp["error"], "account is locked")
	assert.NotEmpty(t, resp["locked_until"])
}

func TestSuccessfulLoginResetsFailedAttempts(t *testing.T) {
	d := setupTestDB(t)
	key := generateTestKey(t)

	h := &AuthHandler{
		DB:         d,
		PrivateKey: key,
		BruteForceConfig: BruteForceConfig{
			MaxFailedAttempts: 5,
			LockoutDuration:  1 * time.Minute,
		},
		rateLimiter: newLoginRateLimiter(BruteForceConfig{
			IPRateLimit:  100,
			IPRateWindow: 15 * time.Minute,
		}),
	}

	router := gin.New()
	router.POST("/api/nvr/auth/setup", h.Setup)
	router.POST("/api/nvr/auth/login", h.Login)

	// Create a user.
	setupBody, _ := json.Marshal(map[string]string{
		"username": "admin",
		"password": "correctpassword",
	})
	req := httptest.NewRequest(http.MethodPost, "/api/nvr/auth/setup", bytes.NewReader(setupBody))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)
	require.Equal(t, http.StatusCreated, w.Code)

	// Fail 3 times (below the threshold of 5).
	badBody, _ := json.Marshal(map[string]string{
		"username": "admin",
		"password": "wrongpassword",
	})
	for i := 0; i < 3; i++ {
		req := httptest.NewRequest(http.MethodPost, "/api/nvr/auth/login", bytes.NewReader(badBody))
		req.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()
		router.ServeHTTP(w, req)
		assert.Equal(t, http.StatusUnauthorized, w.Code)
	}

	// Successful login should reset the counter.
	goodBody, _ := json.Marshal(map[string]string{
		"username": "admin",
		"password": "correctpassword",
	})
	req = httptest.NewRequest(http.MethodPost, "/api/nvr/auth/login", bytes.NewReader(goodBody))
	req.Header.Set("Content-Type", "application/json")
	w = httptest.NewRecorder()
	router.ServeHTTP(w, req)
	assert.Equal(t, http.StatusOK, w.Code)

	// Verify failed_login_attempts was reset to 0.
	user, err := d.GetUserByUsername("admin")
	require.NoError(t, err)
	assert.Equal(t, 0, user.FailedLoginAttempts)
}

func TestIPRateLimiting(t *testing.T) {
	d := setupTestDB(t)
	key := generateTestKey(t)

	rl := newLoginRateLimiter(BruteForceConfig{
		IPRateLimit:  3,
		IPRateWindow: 15 * time.Minute,
	})

	h := &AuthHandler{
		DB:         d,
		PrivateKey: key,
		BruteForceConfig: BruteForceConfig{
			MaxFailedAttempts: 100, // high so lockout doesn't interfere
			LockoutDuration:  1 * time.Minute,
		},
		rateLimiter: rl,
	}

	router := gin.New()
	router.POST("/api/nvr/auth/setup", h.Setup)
	router.POST("/api/nvr/auth/login", h.Login)

	// Create a user.
	setupBody, _ := json.Marshal(map[string]string{
		"username": "admin",
		"password": "mypassword",
	})
	req := httptest.NewRequest(http.MethodPost, "/api/nvr/auth/setup", bytes.NewReader(setupBody))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)
	require.Equal(t, http.StatusCreated, w.Code)

	// Make 3 login attempts (IP rate limit is 3).
	loginBody, _ := json.Marshal(map[string]string{
		"username": "admin",
		"password": "wrongpassword",
	})
	for i := 0; i < 3; i++ {
		req := httptest.NewRequest(http.MethodPost, "/api/nvr/auth/login", bytes.NewReader(loginBody))
		req.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()
		router.ServeHTTP(w, req)
		assert.Equal(t, http.StatusUnauthorized, w.Code, "attempt %d should be unauthorized", i+1)
	}

	// 4th attempt should be rate limited.
	req = httptest.NewRequest(http.MethodPost, "/api/nvr/auth/login", bytes.NewReader(loginBody))
	req.Header.Set("Content-Type", "application/json")
	w = httptest.NewRecorder()
	router.ServeHTTP(w, req)
	assert.Equal(t, http.StatusTooManyRequests, w.Code)
}

func TestAdminUnlockEndpoint(t *testing.T) {
	d := setupTestDB(t)

	// Create an admin user and lock them.
	hashed, err := hashPassword("testpass")
	require.NoError(t, err)
	lockedUntil := time.Now().Add(15 * time.Minute).UTC().Format("2006-01-02T15:04:05.000Z")
	user := &db.User{
		Username:            "lockeduser",
		PasswordHash:        hashed,
		Role:                "viewer",
		CameraPermissions:   "*",
		LockedUntil:         &lockedUntil,
		FailedLoginAttempts: 5,
	}
	require.NoError(t, d.CreateUser(user))

	userHandler := &UserHandler{DB: d, Audit: &AuditLogger{DB: d}}

	router := gin.New()
	router.POST("/api/nvr/users/:id/unlock", func(c *gin.Context) {
		// Simulate admin context.
		c.Set("role", "admin")
		c.Set("user_id", "admin-id")
		c.Set("username", "admin")
		userHandler.Unlock(c)
	})

	req := httptest.NewRequest(http.MethodPost, "/api/nvr/users/"+user.ID+"/unlock", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)

	var resp map[string]any
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
	assert.Equal(t, "account unlocked", resp["message"])
	assert.Equal(t, "lockeduser", resp["username"])

	// Verify user is actually unlocked.
	updated, err := d.GetUser(user.ID)
	require.NoError(t, err)
	assert.Nil(t, updated.LockedUntil)
	assert.Equal(t, 0, updated.FailedLoginAttempts)
}

// Ensure the test binary doesn't set GIN_MODE to debug.
func TestMain(m *testing.M) {
	gin.SetMode(gin.TestMode)
	os.Exit(m.Run())
}
