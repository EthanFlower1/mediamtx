package diagnostics

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"
)

type mockHealthProvider struct {
	health []RecordingHealth
}

func (m *mockHealthProvider) GetAllRecordingHealth() []RecordingHealth {
	return m.health
}

func TestNewService(t *testing.T) {
	dir := t.TempDir()
	svc, err := NewService(ServiceConfig{
		BundleDir: filepath.Join(dir, "bundles"),
		LogDir:    filepath.Join(dir, "logs"),
		Version:   "test-1.0",
	})
	if err != nil {
		t.Fatalf("NewService: %v", err)
	}
	defer svc.Close()

	if svc.version != "test-1.0" {
		t.Errorf("expected version test-1.0, got %s", svc.version)
	}
}

func TestGetRecorderStatuses(t *testing.T) {
	dir := t.TempDir()
	hp := &mockHealthProvider{
		health: []RecordingHealth{
			{CameraID: "cam1", CameraName: "Front Door", Status: "recording"},
			{CameraID: "cam2", CameraName: "Backyard", Status: "stalled", ErrorMessage: "no data"},
		},
	}

	svc, err := NewService(ServiceConfig{
		BundleDir:      filepath.Join(dir, "bundles"),
		LogDir:         filepath.Join(dir, "logs"),
		HealthProvider: hp,
	})
	if err != nil {
		t.Fatalf("NewService: %v", err)
	}
	defer svc.Close()

	statuses := svc.GetRecorderStatuses()
	if len(statuses) != 2 {
		t.Fatalf("expected 2 statuses, got %d", len(statuses))
	}
	if statuses[0].CameraName != "Front Door" {
		t.Errorf("expected Front Door, got %s", statuses[0].CameraName)
	}
	if statuses[1].Status != "stalled" {
		t.Errorf("expected stalled, got %s", statuses[1].Status)
	}
}

func TestGetRecorderStatusesNilProvider(t *testing.T) {
	dir := t.TempDir()
	svc, err := NewService(ServiceConfig{
		BundleDir: filepath.Join(dir, "bundles"),
		LogDir:    filepath.Join(dir, "logs"),
	})
	if err != nil {
		t.Fatalf("NewService: %v", err)
	}
	defer svc.Close()

	statuses := svc.GetRecorderStatuses()
	if len(statuses) != 0 {
		t.Errorf("expected empty statuses, got %d", len(statuses))
	}
}

func TestQueryLogs(t *testing.T) {
	dir := t.TempDir()
	logDir := filepath.Join(dir, "logs")
	os.MkdirAll(logDir, 0o755)

	// Write some JSON log lines.
	entries := []LogEntry{
		{Timestamp: "2026-04-10T10:00:00Z", Level: "info", Module: "camera", Message: "camera connected"},
		{Timestamp: "2026-04-10T10:01:00Z", Level: "warn", Module: "storage", Message: "disk space low"},
		{Timestamp: "2026-04-10T10:02:00Z", Level: "error", Module: "camera", Message: "connection lost to camera"},
	}
	var lines []byte
	for _, e := range entries {
		data, _ := json.Marshal(e)
		lines = append(lines, data...)
		lines = append(lines, '\n')
	}
	os.WriteFile(filepath.Join(logDir, "nvr.log"), lines, 0o644)

	svc, err := NewService(ServiceConfig{
		BundleDir: filepath.Join(dir, "bundles"),
		LogDir:    logDir,
	})
	if err != nil {
		t.Fatalf("NewService: %v", err)
	}
	defer svc.Close()

	// Query all.
	results, total, err := svc.QueryLogs(LogQuery{Limit: 10})
	if err != nil {
		t.Fatalf("QueryLogs: %v", err)
	}
	if total != 3 {
		t.Errorf("expected 3 total entries, got %d", total)
	}
	if len(results) != 3 {
		t.Errorf("expected 3 results, got %d", len(results))
	}

	// Filter by level.
	results, total, err = svc.QueryLogs(LogQuery{Level: "error", Limit: 10})
	if err != nil {
		t.Fatalf("QueryLogs level filter: %v", err)
	}
	if total != 1 {
		t.Errorf("expected 1 error entry, got %d", total)
	}

	// Search.
	results, total, err = svc.QueryLogs(LogQuery{Search: "disk", Limit: 10})
	if err != nil {
		t.Fatalf("QueryLogs search: %v", err)
	}
	if total != 1 {
		t.Errorf("expected 1 entry matching 'disk', got %d", total)
	}

	// Time filter.
	results, total, err = svc.QueryLogs(LogQuery{
		After: "2026-04-10T10:00:30Z",
		Limit: 10,
	})
	if err != nil {
		t.Fatalf("QueryLogs time filter: %v", err)
	}
	if total != 2 {
		t.Errorf("expected 2 entries after 10:00:30, got %d", total)
	}
}

func TestGenerateAndDownloadBundle(t *testing.T) {
	dir := t.TempDir()
	logDir := filepath.Join(dir, "logs")
	os.MkdirAll(logDir, 0o755)
	os.WriteFile(filepath.Join(logDir, "test.log"), []byte("test log data\n"), 0o644)

	svc, err := NewService(ServiceConfig{
		BundleDir:    filepath.Join(dir, "bundles"),
		LogDir:       logDir,
		Version:      "test-1.0",
		BundleExpiry: 1 * time.Hour,
	})
	if err != nil {
		t.Fatalf("NewService: %v", err)
	}
	defer svc.Close()

	id := svc.GenerateBundle()
	if id == "" {
		t.Fatal("GenerateBundle returned empty ID")
	}

	// Wait for bundle to build.
	var bundle *Bundle
	for i := 0; i < 50; i++ {
		time.Sleep(100 * time.Millisecond)
		b, ok := svc.GetBundle(id)
		if ok && b.Status == BundleStatusReady {
			bundle = b
			break
		}
	}

	if bundle == nil {
		b, _ := svc.GetBundle(id)
		t.Fatalf("bundle not ready after 5s, status: %v", b)
	}

	if bundle.SizeBytes <= 0 {
		t.Errorf("expected positive bundle size, got %d", bundle.SizeBytes)
	}

	path := svc.BundlePath(id)
	if path == "" {
		t.Error("BundlePath returned empty for ready bundle")
	}

	// Verify file exists.
	info, err := os.Stat(path)
	if err != nil {
		t.Fatalf("bundle file not found: %v", err)
	}
	if info.Size() != bundle.SizeBytes {
		t.Errorf("file size mismatch: file=%d, metadata=%d", info.Size(), bundle.SizeBytes)
	}
}

func TestListBundles(t *testing.T) {
	dir := t.TempDir()
	svc, err := NewService(ServiceConfig{
		BundleDir: filepath.Join(dir, "bundles"),
		LogDir:    filepath.Join(dir, "logs"),
	})
	if err != nil {
		t.Fatalf("NewService: %v", err)
	}
	defer svc.Close()

	// Initially empty.
	bundles := svc.ListBundles()
	if len(bundles) != 0 {
		t.Errorf("expected 0 bundles, got %d", len(bundles))
	}

	// Generate one.
	svc.GenerateBundle()
	bundles = svc.ListBundles()
	if len(bundles) != 1 {
		t.Errorf("expected 1 bundle, got %d", len(bundles))
	}
}

func TestParsePlainLogLine(t *testing.T) {
	line := "[2026-04-10T10:00:00Z] [WARN] [storage] disk space low"
	entry := parsePlainLogLine(line)

	if entry.Timestamp != "2026-04-10T10:00:00Z" {
		t.Errorf("expected timestamp 2026-04-10T10:00:00Z, got %s", entry.Timestamp)
	}
	if entry.Level != "warn" {
		t.Errorf("expected level warn, got %s", entry.Level)
	}
	if entry.Module != "storage" {
		t.Errorf("expected module storage, got %s", entry.Module)
	}
	if entry.Message != "disk space low" {
		t.Errorf("expected message 'disk space low', got '%s'", entry.Message)
	}
}

func TestRunDefaultProbes(t *testing.T) {
	dir := t.TempDir()
	svc, err := NewService(ServiceConfig{
		BundleDir: filepath.Join(dir, "bundles"),
		LogDir:    filepath.Join(dir, "logs"),
	})
	if err != nil {
		t.Fatalf("NewService: %v", err)
	}
	defer svc.Close()

	results := svc.RunDefaultProbes()
	if len(results) == 0 {
		t.Error("expected at least one probe result")
	}
	// Each result should have a target and port.
	for _, r := range results {
		if r.Target == "" {
			t.Error("probe result has empty target")
		}
		if r.Port == 0 {
			t.Error("probe result has zero port")
		}
	}
}
