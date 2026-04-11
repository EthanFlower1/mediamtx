package api

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"

	"github.com/bluenviron/mediamtx/internal/nvr/db"
)

func setupAPIKeyRouter(t *testing.T) (*gin.Engine, *db.DB) {
	t.Helper()
	gin.SetMode(gin.TestMode)

	d, err := db.Open(t.TempDir() + "/test.db")
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	t.Cleanup(func() { d.Close() })

	handler := &APIKeyHandler{
		DB:    d,
		Audit: &AuditLogger{DB: d},
	}

	r := gin.New()
	// Simulate auth middleware by injecting context values.
	r.Use(func(c *gin.Context) {
		c.Set("user_id", "test-user-id")
		c.Set("username", "admin")
		c.Set("role", "admin")
		c.Next()
	})
	g := r.Group("/api/nvr")
	g.POST("/api-keys", handler.Generate)
	g.GET("/api-keys", handler.List)
	g.POST("/api-keys/:id/rotate", handler.Rotate)
	g.DELETE("/api-keys/:id", handler.Revoke)
	g.GET("/api-keys/:id/audit", handler.AuditLog)

	return r, d
}

func TestGenerateAPIKey(t *testing.T) {
	r, _ := setupAPIKeyRouter(t)

	body := `{"name":"my-key","scope":"read-only","customer_scope":"cust-1"}`
	req := httptest.NewRequest(http.MethodPost, "/api/nvr/api-keys", bytes.NewBufferString(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusCreated {
		t.Fatalf("expected 201, got %d: %s", w.Code, w.Body.String())
	}

	var resp generateResponse
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if resp.Key == "" {
		t.Error("expected raw key in response")
	}
	if len(resp.Key) != 64 {
		t.Errorf("expected 64-char hex key, got %d chars", len(resp.Key))
	}
	if resp.APIKey.Name != "my-key" {
		t.Errorf("expected name 'my-key', got %q", resp.APIKey.Name)
	}
	if resp.APIKey.Scope != "read-only" {
		t.Errorf("expected scope 'read-only', got %q", resp.APIKey.Scope)
	}
}

func TestListAPIKeys(t *testing.T) {
	r, _ := setupAPIKeyRouter(t)

	// Create two keys.
	for _, name := range []string{"key-a", "key-b"} {
		body := `{"name":"` + name + `","scope":"read-write"}`
		req := httptest.NewRequest(http.MethodPost, "/api/nvr/api-keys", bytes.NewBufferString(body))
		req.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()
		r.ServeHTTP(w, req)
		if w.Code != http.StatusCreated {
			t.Fatalf("create %s: %d", name, w.Code)
		}
	}

	req := httptest.NewRequest(http.MethodGet, "/api/nvr/api-keys", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("list: %d", w.Code)
	}

	var resp struct {
		APIKeys []*db.APIKey `json:"api_keys"`
	}
	_ = json.Unmarshal(w.Body.Bytes(), &resp)
	if len(resp.APIKeys) != 2 {
		t.Errorf("expected 2 keys, got %d", len(resp.APIKeys))
	}
	// key_hash must NOT leak in JSON.
	raw := w.Body.String()
	if bytes.Contains([]byte(raw), []byte("key_hash")) {
		t.Error("key_hash must not appear in list response")
	}
}

func TestRevokeAPIKey(t *testing.T) {
	r, _ := setupAPIKeyRouter(t)

	// Create.
	body := `{"name":"revoke-me","scope":"read-only"}`
	req := httptest.NewRequest(http.MethodPost, "/api/nvr/api-keys", bytes.NewBufferString(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	var created generateResponse
	_ = json.Unmarshal(w.Body.Bytes(), &created)

	// Revoke.
	req = httptest.NewRequest(http.MethodDelete, "/api/nvr/api-keys/"+created.APIKey.ID, nil)
	w = httptest.NewRecorder()
	r.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("revoke: %d %s", w.Code, w.Body.String())
	}

	// Double revoke → 404.
	req = httptest.NewRequest(http.MethodDelete, "/api/nvr/api-keys/"+created.APIKey.ID, nil)
	w = httptest.NewRecorder()
	r.ServeHTTP(w, req)
	if w.Code != http.StatusNotFound {
		t.Errorf("expected 404 on double revoke, got %d", w.Code)
	}
}

func TestRotateAPIKey(t *testing.T) {
	r, _ := setupAPIKeyRouter(t)

	// Create original.
	body := `{"name":"rotate-me","scope":"read-write"}`
	req := httptest.NewRequest(http.MethodPost, "/api/nvr/api-keys", bytes.NewBufferString(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	var original generateResponse
	_ = json.Unmarshal(w.Body.Bytes(), &original)

	// Rotate with custom grace period.
	rotateBody := `{"grace_period_hours":48}`
	req = httptest.NewRequest(http.MethodPost, "/api/nvr/api-keys/"+original.APIKey.ID+"/rotate",
		bytes.NewBufferString(rotateBody))
	req.Header.Set("Content-Type", "application/json")
	w = httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusCreated {
		t.Fatalf("rotate: %d %s", w.Code, w.Body.String())
	}

	var rotated rotateResponse
	_ = json.Unmarshal(w.Body.Bytes(), &rotated)
	if rotated.Key == "" {
		t.Error("expected new raw key")
	}
	if rotated.APIKey.RotatedFrom != original.APIKey.ID {
		t.Errorf("expected rotated_from=%s, got %s", original.APIKey.ID, rotated.APIKey.RotatedFrom)
	}
	if rotated.Key == original.Key {
		t.Error("new key must differ from original")
	}
}

func TestAPIKeyAuditLog(t *testing.T) {
	r, _ := setupAPIKeyRouter(t)

	// Create a key (generates an audit entry).
	body := `{"name":"audit-test","scope":"read-only"}`
	req := httptest.NewRequest(http.MethodPost, "/api/nvr/api-keys", bytes.NewBufferString(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	var created generateResponse
	_ = json.Unmarshal(w.Body.Bytes(), &created)

	// Fetch audit log.
	req = httptest.NewRequest(http.MethodGet, "/api/nvr/api-keys/"+created.APIKey.ID+"/audit", nil)
	w = httptest.NewRecorder()
	r.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("audit: %d", w.Code)
	}

	var resp struct {
		Entries []*db.APIKeyAuditEntry `json:"entries"`
	}
	_ = json.Unmarshal(w.Body.Bytes(), &resp)
	if len(resp.Entries) < 1 {
		t.Error("expected at least 1 audit entry")
	}
}

func TestScopeValidation(t *testing.T) {
	r, _ := setupAPIKeyRouter(t)

	body := `{"name":"bad","scope":"admin"}` // invalid scope
	req := httptest.NewRequest(http.MethodPost, "/api/nvr/api-keys", bytes.NewBufferString(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("expected 400 for invalid scope, got %d", w.Code)
	}
}
