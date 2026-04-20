package api

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"

	"github.com/bluenviron/mediamtx/internal/shared/logmgr"
)

func setupLogConfigTest(t *testing.T) (*gin.Engine, *logmgr.Manager) {
	t.Helper()
	gin.SetMode(gin.TestMode)

	dir := t.TempDir()
	cfg := logmgr.DefaultConfig()
	cfg.LogDir = dir

	mgr, err := logmgr.New(cfg, nil)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { mgr.Close() })

	handler := &LogConfigHandler{LogManager: mgr}

	engine := gin.New()
	// Inject admin role for tests.
	engine.Use(func(c *gin.Context) {
		c.Set("role", "admin")
		c.Next()
	})
	engine.GET("/system/logging/config", handler.GetLoggingConfig)
	engine.PUT("/system/logging/config", handler.UpdateLoggingConfig)
	engine.GET("/system/logging/crashes/:filename", handler.GetCrashDump)

	return engine, mgr
}

func TestGetLoggingConfig(t *testing.T) {
	engine, _ := setupLogConfigTest(t)

	w := httptest.NewRecorder()
	req, _ := http.NewRequest("GET", "/system/logging/config", nil)
	engine.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}

	var resp map[string]interface{}
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatal(err)
	}
	if resp["global_level"] != "info" {
		t.Errorf("expected info, got %v", resp["global_level"])
	}
	if resp["json_output"] != true {
		t.Errorf("expected json_output true, got %v", resp["json_output"])
	}
}

func TestUpdateLoggingConfig(t *testing.T) {
	engine, mgr := setupLogConfigTest(t)

	body := map[string]interface{}{
		"global_level": "debug",
		"module_levels": map[string]string{
			"onvif": "error",
			"api":   "warn",
		},
		"max_size_mb": 100,
	}
	data, _ := json.Marshal(body)

	w := httptest.NewRecorder()
	req, _ := http.NewRequest("PUT", "/system/logging/config", bytes.NewReader(data))
	req.Header.Set("Content-Type", "application/json")
	engine.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}

	// Verify the config was updated.
	cfg := mgr.GetConfig()
	if cfg.GlobalLevel != "debug" {
		t.Errorf("expected debug, got %s", cfg.GlobalLevel)
	}
	if cfg.ModuleLevels["onvif"] != "error" {
		t.Errorf("expected error for onvif, got %s", cfg.ModuleLevels["onvif"])
	}
	if cfg.MaxSizeMB != 100 {
		t.Errorf("expected 100, got %d", cfg.MaxSizeMB)
	}
}

func TestUpdateLoggingConfigInvalidLevel(t *testing.T) {
	engine, _ := setupLogConfigTest(t)

	body := map[string]interface{}{
		"global_level": "trace",
	}
	data, _ := json.Marshal(body)

	w := httptest.NewRecorder()
	req, _ := http.NewRequest("PUT", "/system/logging/config", bytes.NewReader(data))
	req.Header.Set("Content-Type", "application/json")
	engine.ServeHTTP(w, req)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d: %s", w.Code, w.Body.String())
	}
}

func TestUpdateLoggingConfigInvalidModuleLevel(t *testing.T) {
	engine, _ := setupLogConfigTest(t)

	body := map[string]interface{}{
		"module_levels": map[string]string{
			"api": "verbose",
		},
	}
	data, _ := json.Marshal(body)

	w := httptest.NewRecorder()
	req, _ := http.NewRequest("PUT", "/system/logging/config", bytes.NewReader(data))
	req.Header.Set("Content-Type", "application/json")
	engine.ServeHTTP(w, req)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d: %s", w.Code, w.Body.String())
	}
}

func TestGetCrashDump(t *testing.T) {
	engine, mgr := setupLogConfigTest(t)

	// Write a crash dump first.
	mgr.WriteCrashDump("test panic")

	dumps := mgr.ListCrashDumps()
	if len(dumps) == 0 {
		t.Fatal("expected at least one crash dump")
	}

	w := httptest.NewRecorder()
	req, _ := http.NewRequest("GET", "/system/logging/crashes/"+dumps[0].Filename, nil)
	engine.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}

	var resp map[string]interface{}
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatal(err)
	}
	content, ok := resp["content"].(string)
	if !ok || content == "" {
		t.Fatal("expected non-empty content")
	}
}

func TestGetCrashDumpNotFound(t *testing.T) {
	engine, _ := setupLogConfigTest(t)

	w := httptest.NewRecorder()
	req, _ := http.NewRequest("GET", "/system/logging/crashes/nonexistent.log", nil)
	engine.ServeHTTP(w, req)

	if w.Code != http.StatusNotFound {
		t.Fatalf("expected 404, got %d", w.Code)
	}
}

func TestNonAdminDenied(t *testing.T) {
	gin.SetMode(gin.TestMode)

	dir := t.TempDir()
	cfg := logmgr.DefaultConfig()
	cfg.LogDir = dir

	mgr, err := logmgr.New(cfg, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer mgr.Close()

	handler := &LogConfigHandler{LogManager: mgr}

	engine := gin.New()
	// Inject non-admin role.
	engine.Use(func(c *gin.Context) {
		c.Set("role", "viewer")
		c.Next()
	})
	engine.GET("/system/logging/config", handler.GetLoggingConfig)

	w := httptest.NewRecorder()
	req, _ := http.NewRequest("GET", "/system/logging/config", nil)
	engine.ServeHTTP(w, req)

	if w.Code != http.StatusForbidden {
		t.Fatalf("expected 403, got %d: %s", w.Code, w.Body.String())
	}
}
