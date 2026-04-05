package api

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"

	"github.com/bluenviron/mediamtx/internal/nvr/ai"
)

func setupModelTestRouter(mgr *ai.ModelManager) *gin.Engine {
	gin.SetMode(gin.TestMode)
	r := gin.New()

	handler := &ModelHandler{Manager: mgr}
	g := r.Group("/api/nvr")
	g.GET("/ai/models", handler.List)
	g.POST("/ai/models/activate", handler.Activate)
	g.POST("/ai/models/rollback", handler.Rollback)
	g.POST("/ai/models/verify", handler.Verify)

	return r
}

func TestModelHandlerList(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "yolov8n.onnx"), []byte("fake"), 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "yolov8s.onnx"), []byte("fake-bigger"), 0644); err != nil {
		t.Fatal(err)
	}

	mgr := ai.NewModelManager(dir, nil, filepath.Join(dir, "yolov8n.onnx"))
	r := setupModelTestRouter(mgr)

	w := httptest.NewRecorder()
	req, _ := http.NewRequest("GET", "/api/nvr/ai/models", nil)
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}

	var resp struct {
		Models      []ai.ModelInfo `json:"models"`
		ActiveModel string         `json:"active_model"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}

	if len(resp.Models) != 2 {
		t.Errorf("expected 2 models, got %d", len(resp.Models))
	}
	if resp.ActiveModel != filepath.Join(dir, "yolov8n.onnx") {
		t.Errorf("unexpected active model: %s", resp.ActiveModel)
	}
}

func TestModelHandlerList_NilManager(t *testing.T) {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	handler := &ModelHandler{Manager: nil}
	r.GET("/api/nvr/ai/models", handler.List)

	w := httptest.NewRecorder()
	req, _ := http.NewRequest("GET", "/api/nvr/ai/models", nil)
	r.ServeHTTP(w, req)

	if w.Code != http.StatusServiceUnavailable {
		t.Fatalf("expected 503, got %d", w.Code)
	}
}

func TestModelHandlerVerify(t *testing.T) {
	dir := t.TempDir()
	modelPath := filepath.Join(dir, "test.onnx")
	if err := os.WriteFile(modelPath, []byte("test content"), 0644); err != nil {
		t.Fatal(err)
	}

	mgr := ai.NewModelManager(dir, nil, "")
	r := setupModelTestRouter(mgr)

	body := `{"model_path":"test.onnx"}`
	w := httptest.NewRecorder()
	req, _ := http.NewRequest("POST", "/api/nvr/ai/models/verify", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}

	var resp struct {
		SHA256 string `json:"sha256"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if resp.SHA256 == "" {
		t.Error("expected non-empty sha256")
	}
}

func TestModelHandlerActivate_NotFound(t *testing.T) {
	mgr := ai.NewModelManager(t.TempDir(), nil, "")
	r := setupModelTestRouter(mgr)

	body := `{"model_path":"nonexistent.onnx"}`
	w := httptest.NewRecorder()
	req, _ := http.NewRequest("POST", "/api/nvr/ai/models/activate", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	r.ServeHTTP(w, req)

	if w.Code != http.StatusInternalServerError {
		t.Fatalf("expected 500, got %d: %s", w.Code, w.Body.String())
	}
}

func TestModelHandlerRollback_NoPrevious(t *testing.T) {
	mgr := ai.NewModelManager(t.TempDir(), nil, "")
	r := setupModelTestRouter(mgr)

	w := httptest.NewRecorder()
	req, _ := http.NewRequest("POST", "/api/nvr/ai/models/rollback", nil)
	r.ServeHTTP(w, req)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d: %s", w.Code, w.Body.String())
	}
}

func TestModelHandlerActivate_MissingBody(t *testing.T) {
	mgr := ai.NewModelManager(t.TempDir(), nil, "")
	r := setupModelTestRouter(mgr)

	w := httptest.NewRecorder()
	req, _ := http.NewRequest("POST", "/api/nvr/ai/models/activate", strings.NewReader("{}"))
	req.Header.Set("Content-Type", "application/json")
	r.ServeHTTP(w, req)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d: %s", w.Code, w.Body.String())
	}
}
