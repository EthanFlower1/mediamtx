package api

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

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

// Ensure the test binary doesn't set GIN_MODE to debug.
func TestMain(m *testing.M) {
	gin.SetMode(gin.TestMode)
	os.Exit(m.Run())
}
