package logmgr

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// mockConfigStore implements configStore for testing.
type mockConfigStore struct {
	data map[string]string
}

func newMockStore() *mockConfigStore {
	return &mockConfigStore{data: make(map[string]string)}
}

func (s *mockConfigStore) GetConfig(key string) (string, error) {
	v, ok := s.data[key]
	if !ok {
		return "", nil
	}
	return v, nil
}

func (s *mockConfigStore) SetConfig(key, value string) error {
	s.data[key] = value
	return nil
}

func TestDefaultConfig(t *testing.T) {
	cfg := DefaultConfig()
	if cfg.GlobalLevel != "info" {
		t.Fatalf("expected info, got %s", cfg.GlobalLevel)
	}
	if cfg.MaxSizeMB != 50 {
		t.Fatalf("expected 50, got %d", cfg.MaxSizeMB)
	}
	if !cfg.JSONOutput {
		t.Fatal("expected JSON output enabled")
	}
}

func TestParseLevel(t *testing.T) {
	tests := []struct {
		input string
		want  Level
	}{
		{"debug", LevelDebug},
		{"DEBUG", LevelDebug},
		{"info", LevelInfo},
		{"warn", LevelWarn},
		{"warning", LevelWarn},
		{"error", LevelError},
		{"unknown", LevelInfo},
	}
	for _, tc := range tests {
		got := ParseLevel(tc.input)
		if got != tc.want {
			t.Errorf("ParseLevel(%q) = %v, want %v", tc.input, got, tc.want)
		}
	}
}

func TestNewManager(t *testing.T) {
	dir := t.TempDir()
	cfg := DefaultConfig()
	cfg.LogDir = dir

	m, err := New(cfg, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer m.Close()

	if m.globalLevel != LevelInfo {
		t.Fatalf("expected info level, got %v", m.globalLevel)
	}
}

func TestJSONLogOutput(t *testing.T) {
	dir := t.TempDir()
	cfg := DefaultConfig()
	cfg.LogDir = dir
	cfg.GlobalLevel = "debug"

	m, err := New(cfg, nil)
	if err != nil {
		t.Fatal(err)
	}

	m.Info("api", "test message", map[string]interface{}{"key": "value"})
	m.Debug("onvif", "debug message")
	m.Close()

	data, err := os.ReadFile(filepath.Join(dir, "nvr.log"))
	if err != nil {
		t.Fatal(err)
	}

	lines := strings.Split(strings.TrimSpace(string(data)), "\n")
	if len(lines) != 2 {
		t.Fatalf("expected 2 log lines, got %d: %s", len(lines), string(data))
	}

	var entry Entry
	if err := json.Unmarshal([]byte(lines[0]), &entry); err != nil {
		t.Fatalf("failed to parse JSON log: %v", err)
	}
	if entry.Level != "info" {
		t.Errorf("expected info level, got %s", entry.Level)
	}
	if entry.Module != "api" {
		t.Errorf("expected api module, got %s", entry.Module)
	}
	if entry.Message != "test message" {
		t.Errorf("expected 'test message', got %s", entry.Message)
	}
	if entry.Fields["key"] != "value" {
		t.Errorf("expected field key=value, got %v", entry.Fields)
	}
}

func TestModuleLevelFiltering(t *testing.T) {
	dir := t.TempDir()
	cfg := DefaultConfig()
	cfg.LogDir = dir
	cfg.GlobalLevel = "info"
	cfg.ModuleLevels = map[string]string{
		"onvif": "error",
	}

	m, err := New(cfg, nil)
	if err != nil {
		t.Fatal(err)
	}

	// This should be logged (info >= info global level).
	m.Info("api", "api info")
	// This should be filtered (info < error for onvif module).
	m.Info("onvif", "onvif info")
	// This should be logged (error >= error for onvif module).
	m.Error("onvif", "onvif error")
	m.Close()

	data, err := os.ReadFile(filepath.Join(dir, "nvr.log"))
	if err != nil {
		t.Fatal(err)
	}

	lines := strings.Split(strings.TrimSpace(string(data)), "\n")
	if len(lines) != 2 {
		t.Fatalf("expected 2 log lines, got %d: %s", len(lines), string(data))
	}
}

func TestUpdateConfigPersistence(t *testing.T) {
	dir := t.TempDir()
	store := newMockStore()
	cfg := DefaultConfig()
	cfg.LogDir = dir

	m, err := New(cfg, store)
	if err != nil {
		t.Fatal(err)
	}
	defer m.Close()

	// Update config.
	newCfg := m.GetConfig()
	newCfg.GlobalLevel = "debug"
	newCfg.ModuleLevels["scheduler"] = "warn"

	if err := m.UpdateConfig(newCfg); err != nil {
		t.Fatal(err)
	}

	// Verify it was persisted.
	saved, err := store.GetConfig("logging_config")
	if err != nil {
		t.Fatal(err)
	}
	if saved == "" {
		t.Fatal("config not persisted")
	}

	var loaded Config
	if err := json.Unmarshal([]byte(saved), &loaded); err != nil {
		t.Fatal(err)
	}
	if loaded.GlobalLevel != "debug" {
		t.Errorf("expected debug, got %s", loaded.GlobalLevel)
	}
	if loaded.ModuleLevels["scheduler"] != "warn" {
		t.Errorf("expected warn for scheduler, got %s", loaded.ModuleLevels["scheduler"])
	}
}

func TestCrashDump(t *testing.T) {
	dir := t.TempDir()
	cfg := DefaultConfig()
	cfg.LogDir = dir
	cfg.CrashDumpEnabled = true

	m, err := New(cfg, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer m.Close()

	filename := m.WriteCrashDump("test panic value")
	if filename == "" {
		t.Fatal("expected crash dump filename")
	}

	// Verify file exists and contains expected content.
	data, err := os.ReadFile(filename)
	if err != nil {
		t.Fatal(err)
	}
	content := string(data)
	if !strings.Contains(content, "test panic value") {
		t.Error("crash dump missing panic value")
	}
	if !strings.Contains(content, "Stack Trace") {
		t.Error("crash dump missing stack trace")
	}

	// List crash dumps.
	dumps := m.ListCrashDumps()
	if len(dumps) != 1 {
		t.Fatalf("expected 1 crash dump, got %d", len(dumps))
	}

	// Read crash dump.
	content2, err := m.GetCrashDump(dumps[0].Filename)
	if err != nil {
		t.Fatal(err)
	}
	if content != content2 {
		t.Error("read crash dump content mismatch")
	}
}

func TestCrashDumpPathTraversal(t *testing.T) {
	dir := t.TempDir()
	cfg := DefaultConfig()
	cfg.LogDir = dir

	m, err := New(cfg, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer m.Close()

	_, err = m.GetCrashDump("../../../etc/passwd")
	if err == nil {
		t.Fatal("expected error for path traversal")
	}
}

func TestLogRotation(t *testing.T) {
	dir := t.TempDir()
	cfg := DefaultConfig()
	cfg.LogDir = dir
	cfg.MaxSizeMB = 0 // Will use raw bytes for testing.

	m, err := New(cfg, nil)
	if err != nil {
		t.Fatal(err)
	}

	// Set a very small max size to trigger rotation.
	m.writer.maxSizeBytes = 100

	// Write enough data to trigger rotation.
	for i := 0; i < 20; i++ {
		m.Info("test", "this is a test log message that should trigger rotation")
	}
	m.Close()

	// Check that rotated files exist.
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatal(err)
	}

	var logFiles int
	for _, e := range entries {
		if strings.HasSuffix(e.Name(), ".log") {
			logFiles++
		}
	}
	if logFiles < 2 {
		t.Errorf("expected at least 2 log files after rotation, got %d", logFiles)
	}
}

func TestPlainTextOutput(t *testing.T) {
	dir := t.TempDir()
	cfg := DefaultConfig()
	cfg.LogDir = dir
	cfg.JSONOutput = false

	m, err := New(cfg, nil)
	if err != nil {
		t.Fatal(err)
	}

	m.Info("api", "plain text test")
	m.Close()

	data, err := os.ReadFile(filepath.Join(dir, "nvr.log"))
	if err != nil {
		t.Fatal(err)
	}

	content := string(data)
	if !strings.Contains(content, "[INFO]") {
		t.Error("expected [INFO] in plain text output")
	}
	if !strings.Contains(content, "[api]") {
		t.Error("expected [api] in plain text output")
	}
	// Should NOT be valid JSON.
	var entry Entry
	if err := json.Unmarshal([]byte(strings.TrimSpace(content)), &entry); err == nil {
		t.Error("plain text output should not be valid JSON")
	}
}

func TestLoadPersistedConfig(t *testing.T) {
	dir := t.TempDir()
	store := newMockStore()

	// Pre-set a persisted config.
	persisted := Config{
		GlobalLevel:  "debug",
		ModuleLevels: map[string]string{"api": "error"},
		MaxSizeMB:    100,
		MaxAgeDays:   60,
		MaxBackups:   20,
		JSONOutput:   false,
	}
	data, _ := json.Marshal(persisted)
	store.SetConfig("logging_config", string(data))

	cfg := DefaultConfig()
	cfg.LogDir = dir

	m, err := New(cfg, store)
	if err != nil {
		t.Fatal(err)
	}
	defer m.Close()

	got := m.GetConfig()
	if got.GlobalLevel != "debug" {
		t.Errorf("expected debug from persisted, got %s", got.GlobalLevel)
	}
	if got.ModuleLevels["api"] != "error" {
		t.Errorf("expected error for api module, got %s", got.ModuleLevels["api"])
	}
}
