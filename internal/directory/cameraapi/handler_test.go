package cameraapi_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"

	"github.com/bluenviron/mediamtx/internal/directory/cameraapi"
	dirdb "github.com/bluenviron/mediamtx/internal/directory/db"
)

func newTestDB(t *testing.T) *dirdb.DB {
	t.Helper()
	db, err := dirdb.Open(context.Background(), ":memory:")
	if err != nil {
		t.Fatalf("open test db: %v", err)
	}
	t.Cleanup(func() { db.Close() })
	return db
}

func newRouter(db *dirdb.DB) *gin.Engine {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	h := cameraapi.NewHandler(db)
	h.Register(r)
	return r
}

// TestRegister_RoutesExist verifies that the five CRUD routes are registered
// (returns something other than 404 for each method/path combination).
func TestRegister_RoutesExist(t *testing.T) {
	db := newTestDB(t)
	r := newRouter(db)

	cases := []struct {
		method string
		path   string
	}{
		{http.MethodGet, "/api/v1/cameras"},
		{http.MethodPost, "/api/v1/cameras"},
		{http.MethodGet, "/api/v1/cameras/nonexistent-id"},
		{http.MethodPut, "/api/v1/cameras/nonexistent-id"},
		{http.MethodDelete, "/api/v1/cameras/nonexistent-id"},
	}

	for _, tc := range cases {
		req := httptest.NewRequest(tc.method, tc.path, nil)
		w := httptest.NewRecorder()
		r.ServeHTTP(w, req)
		if w.Code == http.StatusNotFound {
			// A 404 from gin means the route was not registered at all.
			// (Our handlers also return 404 for missing resources, but gin's
			// router returns a body of "404 page not found" without JSON.)
			body := w.Body.String()
			if body == "404 page not found\n" {
				t.Errorf("%s %s: route not registered (got gin 404)", tc.method, tc.path)
			}
		}
	}
}

// TestList_Empty verifies List returns an empty items array on a fresh DB.
func TestList_Empty(t *testing.T) {
	db := newTestDB(t)
	r := newRouter(db)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/cameras", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}

	var resp struct {
		Items []any `json:"items"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if len(resp.Items) != 0 {
		t.Errorf("expected 0 items, got %d", len(resp.Items))
	}
}

// TestCreateAndGet verifies creating a camera and retrieving it by ID.
func TestCreateAndGet(t *testing.T) {
	db := newTestDB(t)
	r := newRouter(db)

	// Create.
	body := `{"name":"Front Door","rtsp_url":"rtsp://192.0.2.1/stream"}`
	req := httptest.NewRequest(http.MethodPost, "/api/v1/cameras", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusCreated {
		t.Fatalf("create: expected 201, got %d: %s", w.Code, w.Body.String())
	}

	var created struct {
		ID   string `json:"id"`
		Name string `json:"name"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &created); err != nil {
		t.Fatalf("decode create response: %v", err)
	}
	if created.ID == "" {
		t.Fatal("expected non-empty ID in create response")
	}
	if created.Name != "Front Door" {
		t.Errorf("expected name 'Front Door', got %q", created.Name)
	}

	// Get.
	req2 := httptest.NewRequest(http.MethodGet, "/api/v1/cameras/"+created.ID, nil)
	w2 := httptest.NewRecorder()
	r.ServeHTTP(w2, req2)

	if w2.Code != http.StatusOK {
		t.Fatalf("get: expected 200, got %d: %s", w2.Code, w2.Body.String())
	}
}

// TestCreate_MissingName verifies 400 is returned when name is omitted.
func TestCreate_MissingName(t *testing.T) {
	db := newTestDB(t)
	r := newRouter(db)

	req := httptest.NewRequest(http.MethodPost, "/api/v1/cameras", strings.NewReader(`{}`))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("expected 400, got %d", w.Code)
	}
}

// TestDelete_NotFound verifies 404 is returned for a non-existent camera.
func TestDelete_NotFound(t *testing.T) {
	db := newTestDB(t)
	r := newRouter(db)

	req := httptest.NewRequest(http.MethodDelete, "/api/v1/cameras/does-not-exist", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusNotFound {
		t.Errorf("expected 404, got %d: %s", w.Code, w.Body.String())
	}
}
