package api

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"

	"github.com/gin-gonic/gin"

	"github.com/bluenviron/mediamtx/internal/nvr/db"
)

func setupUserTest(t *testing.T) (*UserHandler, func()) {
	t.Helper()

	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "test.db")

	database, err := db.Open(dbPath)
	if err != nil {
		t.Fatalf("open db: %v", err)
	}

	handler := &UserHandler{
		DB: database,
	}

	cleanup := func() {
		database.Close()
	}

	return handler, cleanup
}

func TestUserListRequiresAdmin(t *testing.T) {
	gin.SetMode(gin.TestMode)
	handler, cleanup := setupUserTest(t)
	defer cleanup()

	// Test without admin role — should be forbidden.
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest(http.MethodGet, "/", nil)
	c.Set("role", "viewer")

	handler.List(c)

	if w.Code != http.StatusForbidden {
		t.Fatalf("expected status %d for non-admin, got %d", http.StatusForbidden, w.Code)
	}

	// Test with admin role — should succeed.
	w = httptest.NewRecorder()
	c, _ = gin.CreateTestContext(w)
	c.Request = httptest.NewRequest(http.MethodGet, "/", nil)
	c.Set("role", "admin")

	handler.List(c)

	if w.Code != http.StatusOK {
		t.Fatalf("expected status %d for admin, got %d", http.StatusOK, w.Code)
	}

	var users []db.User
	if err := json.Unmarshal(w.Body.Bytes(), &users); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if len(users) != 0 {
		t.Fatalf("expected 0 users, got %d", len(users))
	}
}
