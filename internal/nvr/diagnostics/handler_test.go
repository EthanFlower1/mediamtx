package diagnostics

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
)

func init() {
	gin.SetMode(gin.TestMode)
}

func setupRouter(h *Handler) *gin.Engine {
	r := gin.New()
	r.POST("/api/nvr/diagnostics/bundle", h.GenerateBundle)
	r.GET("/api/nvr/diagnostics/bundle/:id", h.GetBundle)
	r.GET("/api/nvr/diagnostics/bundles", h.ListBundles)
	r.POST("/api/nvr/diagnostics/cleanup", h.CleanExpired)
	return r
}

func TestHandler_GenerateBundle(t *testing.T) {
	c := NewCollector(CollectorConfig{
		Logs: &mockLogProvider{
			entries: []LogEntry{{Level: "info", Message: "test"}},
		},
		Hardware: &mockHardwareProvider{
			health: &HardwareHealth{CPUCores: 4, Tier: "mid"},
		},
		Version: "1.0.0",
		IDGen:   func() string { return "handler-test-001" },
	})

	h := NewHandler(c)
	router := setupRouter(h)

	req := httptest.NewRequest(http.MethodPost, "/api/nvr/diagnostics/bundle",
		strings.NewReader(`{"hours_back": 2, "sections": ["logs", "hardware"]}`))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()

	router.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}

	var bundle Bundle
	if err := json.Unmarshal(w.Body.Bytes(), &bundle); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}

	if bundle.BundleID != "handler-test-001" {
		t.Errorf("bundle ID = %s, want handler-test-001", bundle.BundleID)
	}
	if bundle.Status != StatusReady {
		t.Errorf("status = %s, want ready", bundle.Status)
	}
	if len(bundle.Sections) != 2 {
		t.Errorf("sections = %d, want 2", len(bundle.Sections))
	}
}

func TestHandler_GenerateBundle_EmptyBody(t *testing.T) {
	c := NewCollector(CollectorConfig{
		Version: "1.0.0",
		IDGen:   func() string { return "empty-body-001" },
	})

	h := NewHandler(c)
	router := setupRouter(h)

	req := httptest.NewRequest(http.MethodPost, "/api/nvr/diagnostics/bundle", nil)
	w := httptest.NewRecorder()

	router.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}
}

func TestHandler_GetBundle_NotFound(t *testing.T) {
	c := NewCollector(CollectorConfig{Version: "1.0.0"})
	h := NewHandler(c)
	router := setupRouter(h)

	req := httptest.NewRequest(http.MethodGet, "/api/nvr/diagnostics/bundle/nonexistent", nil)
	w := httptest.NewRecorder()

	router.ServeHTTP(w, req)

	if w.Code != http.StatusNotFound {
		t.Errorf("expected 404, got %d", w.Code)
	}
}

func TestHandler_GetBundle_Found(t *testing.T) {
	c := NewCollector(CollectorConfig{
		Logs:    &mockLogProvider{entries: []LogEntry{{Level: "info"}}},
		Version: "1.0.0",
		IDGen:   func() string { return "found-001" },
	})

	h := NewHandler(c)
	router := setupRouter(h)

	// Generate first.
	genReq := httptest.NewRequest(http.MethodPost, "/api/nvr/diagnostics/bundle",
		strings.NewReader(`{"sections": ["logs"]}`))
	genReq.Header.Set("Content-Type", "application/json")
	genW := httptest.NewRecorder()
	router.ServeHTTP(genW, genReq)

	if genW.Code != http.StatusOK {
		t.Fatalf("generate failed: %d", genW.Code)
	}

	// Now get it.
	getReq := httptest.NewRequest(http.MethodGet, "/api/nvr/diagnostics/bundle/found-001", nil)
	getW := httptest.NewRecorder()
	router.ServeHTTP(getW, getReq)

	if getW.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", getW.Code)
	}

	var bundle Bundle
	json.Unmarshal(getW.Body.Bytes(), &bundle)
	if bundle.BundleID != "found-001" {
		t.Errorf("bundle ID = %s, want found-001", bundle.BundleID)
	}
}

func TestHandler_ListBundles(t *testing.T) {
	c := NewCollector(CollectorConfig{
		Logs:    &mockLogProvider{entries: []LogEntry{{Level: "info"}}},
		Version: "1.0.0",
	})

	counter := 0
	c.cfg.IDGen = func() string {
		counter++
		return "list-" + string(rune('0'+counter))
	}

	h := NewHandler(c)
	router := setupRouter(h)

	// Generate two bundles.
	for i := 0; i < 2; i++ {
		req := httptest.NewRequest(http.MethodPost, "/api/nvr/diagnostics/bundle",
			strings.NewReader(`{"sections": ["logs"]}`))
		req.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()
		router.ServeHTTP(w, req)
	}

	// List.
	listReq := httptest.NewRequest(http.MethodGet, "/api/nvr/diagnostics/bundles", nil)
	listW := httptest.NewRecorder()
	router.ServeHTTP(listW, listReq)

	if listW.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", listW.Code)
	}

	var bundles []*Bundle
	json.Unmarshal(listW.Body.Bytes(), &bundles)
	if len(bundles) != 2 {
		t.Errorf("expected 2 bundles, got %d", len(bundles))
	}
}
