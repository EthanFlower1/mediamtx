package diagnostics

import (
	"context"
	"crypto/rand"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"strings"
	"testing"
	"time"
)

// ----- Mock providers -----

type mockLogProvider struct {
	entries []CollectorLogEntry
	err     error
}

func (m *mockLogProvider) ReadLogs(_ int) ([]CollectorLogEntry, error) {
	return m.entries, m.err
}

type mockMetricsProvider struct {
	current MetricPoint
	history []MetricPoint
}

func (m *mockMetricsProvider) CurrentMetrics() MetricPoint   { return m.current }
func (m *mockMetricsProvider) HistoryMetrics() []MetricPoint { return m.history }

type mockCameraProvider struct {
	states []CameraState
	err    error
}

func (m *mockCameraProvider) ListCameraStates(_ context.Context) ([]CameraState, error) {
	return m.states, m.err
}

type mockHardwareProvider struct {
	health *HardwareHealth
	err    error
}

func (m *mockHardwareProvider) GetHardwareHealth() (*HardwareHealth, error) {
	return m.health, m.err
}

type mockSidecarProvider struct {
	statuses []SidecarStatus
	err      error
}

func (m *mockSidecarProvider) ListSidecarStatus(_ context.Context) ([]SidecarStatus, error) {
	return m.statuses, m.err
}

type mockUploader struct {
	uploaded   map[string][]byte
	uploadErr  error
	deleteErr  error
	deletedKeys []string
}

func newMockUploader() *mockUploader {
	return &mockUploader{uploaded: make(map[string][]byte)}
}

func (m *mockUploader) Upload(_ context.Context, key string, data io.Reader, _ int64, _ time.Duration) (string, error) {
	if m.uploadErr != nil {
		return "", m.uploadErr
	}
	b, _ := io.ReadAll(data)
	m.uploaded[key] = b
	return fmt.Sprintf("https://example.com/%s", key), nil
}

func (m *mockUploader) Delete(_ context.Context, key string) error {
	if m.deleteErr != nil {
		return m.deleteErr
	}
	m.deletedKeys = append(m.deletedKeys, key)
	delete(m.uploaded, key)
	return nil
}

// ----- Tests -----

func TestGenerate_AllSections(t *testing.T) {
	logs := &mockLogProvider{
		entries: []CollectorLogEntry{
			{Timestamp: time.Now().UTC().Format(time.RFC3339Nano), Level: "info", Module: "nvr", Message: "test log"},
		},
	}
	metrics := &mockMetricsProvider{
		current: MetricPoint{Timestamp: time.Now().Unix(), CPUPercent: 42.5, MemPercent: 65.0, Goroutines: 12},
		history: []MetricPoint{
			{Timestamp: time.Now().Add(-5 * time.Minute).Unix(), CPUPercent: 40.0},
			{Timestamp: time.Now().Unix(), CPUPercent: 42.5},
		},
	}
	cameras := &mockCameraProvider{
		states: []CameraState{
			{ID: "cam1", Name: "Front Door", Status: "recording"},
			{ID: "cam2", Name: "Back Yard", Status: "offline"},
		},
	}
	hw := &mockHardwareProvider{
		health: &HardwareHealth{
			CPUCores: 8, CPUArch: "arm64", GOOS: "linux",
			TotalRAMGB: 16.0, FreeDiskGB: 500.0, Tier: "enterprise",
			NetworkIFs: []string{"eth0"},
		},
	}
	sidecars := &mockSidecarProvider{
		statuses: []SidecarStatus{
			{Name: "zitadel", Status: "running"},
			{Name: "mediamtx", Status: "running"},
		},
	}

	key := make([]byte, 32)
	rand.Read(key)

	c := NewCollector(CollectorConfig{
		Logs:          logs,
		Metrics:       metrics,
		Cameras:       cameras,
		Hardware:      hw,
		Sidecars:      sidecars,
		EncryptionKey: key,
		Version:       "1.0.0-test",
		IDGen:         func() string { return "test-bundle-001" },
	})

	bundle, encrypted, err := c.Generate(context.Background(), GenerateRequest{HoursBack: 24})
	if err != nil {
		t.Fatalf("Generate failed: %v", err)
	}

	// Verify bundle metadata.
	if bundle.BundleID != "test-bundle-001" {
		t.Errorf("expected bundle ID test-bundle-001, got %s", bundle.BundleID)
	}
	if bundle.Status != StatusReady {
		t.Errorf("expected status ready, got %s", bundle.Status)
	}
	if !bundle.Encrypted {
		t.Error("expected bundle to be encrypted")
	}
	if bundle.SizeBytes <= 0 {
		t.Error("expected positive size")
	}
	if len(bundle.Sections) != len(AllSections) {
		t.Errorf("expected %d sections, got %d", len(AllSections), len(bundle.Sections))
	}
	if bundle.ExpiresAt.Before(time.Now()) {
		t.Error("expected expiry in the future")
	}

	// Decrypt and verify archive contents.
	if encrypted == nil {
		t.Fatal("expected encrypted payload when no uploader configured")
	}

	plain, err := Decrypt(key, encrypted)
	if err != nil {
		t.Fatalf("Decrypt failed: %v", err)
	}

	files, err := ReadBundleArchive(plain)
	if err != nil {
		t.Fatalf("ReadBundleArchive failed: %v", err)
	}

	expected := []string{
		"manifest.json",
		"logs/structured.json",
		"metrics/snapshot.json",
		"metrics/current.json",
		"cameras/states.json",
		"hardware/report.json",
		"sidecars/status.json",
	}
	fileSet := make(map[string]bool)
	for _, f := range files {
		fileSet[f] = true
	}
	for _, e := range expected {
		if !fileSet[e] {
			t.Errorf("expected file %s in archive, got: %v", e, files)
		}
	}

	// Verify manifest contents.
	manifestData, err := ReadArchiveFile(plain, "manifest.json")
	if err != nil {
		t.Fatalf("read manifest: %v", err)
	}
	var manifest BundleManifest
	if err := json.Unmarshal(manifestData, &manifest); err != nil {
		t.Fatalf("unmarshal manifest: %v", err)
	}
	if manifest.BundleID != "test-bundle-001" {
		t.Errorf("manifest bundle ID = %s, want test-bundle-001", manifest.BundleID)
	}
	if manifest.Version != "1.0.0-test" {
		t.Errorf("manifest version = %s, want 1.0.0-test", manifest.Version)
	}

	// Verify camera states.
	cameraData, err := ReadArchiveFile(plain, "cameras/states.json")
	if err != nil {
		t.Fatalf("read cameras: %v", err)
	}
	var cameraStates []CameraState
	if err := json.Unmarshal(cameraData, &cameraStates); err != nil {
		t.Fatalf("unmarshal cameras: %v", err)
	}
	if len(cameraStates) != 2 {
		t.Errorf("expected 2 cameras, got %d", len(cameraStates))
	}
}

func TestGenerate_SelectiveSections(t *testing.T) {
	logs := &mockLogProvider{
		entries: []CollectorLogEntry{
			{Timestamp: time.Now().UTC().Format(time.RFC3339Nano), Level: "error", Module: "db", Message: "disk full"},
		},
	}
	hw := &mockHardwareProvider{
		health: &HardwareHealth{CPUCores: 4, Tier: "mid"},
	}

	c := NewCollector(CollectorConfig{
		Logs:     logs,
		Hardware: hw,
		Version:  "1.0.0",
		IDGen:    func() string { return "selective-001" },
	})

	bundle, payload, err := c.Generate(context.Background(), GenerateRequest{
		HoursBack: 1,
		Sections:  []string{"logs", "hardware"},
	})
	if err != nil {
		t.Fatalf("Generate failed: %v", err)
	}

	if len(bundle.Sections) != 2 {
		t.Errorf("expected 2 sections, got %d", len(bundle.Sections))
	}

	files, err := ReadBundleArchive(payload)
	if err != nil {
		t.Fatalf("ReadBundleArchive failed: %v", err)
	}

	fileSet := make(map[string]bool)
	for _, f := range files {
		fileSet[f] = true
	}

	if !fileSet["logs/structured.json"] {
		t.Error("expected logs/structured.json")
	}
	if !fileSet["hardware/report.json"] {
		t.Error("expected hardware/report.json")
	}
	// Should NOT contain metrics or cameras since they weren't requested.
	if fileSet["metrics/snapshot.json"] {
		t.Error("did not expect metrics/snapshot.json")
	}
	if fileSet["cameras/states.json"] {
		t.Error("did not expect cameras/states.json")
	}
}

func TestGenerate_WithUploader(t *testing.T) {
	uploader := newMockUploader()

	c := NewCollector(CollectorConfig{
		Logs: &mockLogProvider{
			entries: []CollectorLogEntry{{Level: "info", Message: "hello"}},
		},
		Uploader:      uploader,
		EncryptionKey: make([]byte, 32), // zero key for test
		Version:       "1.0.0",
		IDGen:         func() string { return "upload-001" },
	})

	bundle, payload, err := c.Generate(context.Background(), GenerateRequest{
		Sections: []string{"logs"},
	})
	if err != nil {
		t.Fatalf("Generate failed: %v", err)
	}

	if bundle.Status != StatusReady {
		t.Errorf("expected status ready, got %s", bundle.Status)
	}
	if bundle.DownloadURL == "" {
		t.Error("expected download URL")
	}
	if !strings.Contains(bundle.StorageKey, "upload-001") {
		t.Errorf("expected storage key to contain bundle ID, got %s", bundle.StorageKey)
	}
	if payload != nil {
		t.Error("expected nil payload when uploader is used")
	}
	if len(uploader.uploaded) != 1 {
		t.Errorf("expected 1 upload, got %d", len(uploader.uploaded))
	}
}

func TestGenerate_UploadError(t *testing.T) {
	uploader := newMockUploader()
	uploader.uploadErr = fmt.Errorf("network timeout")

	c := NewCollector(CollectorConfig{
		Logs:     &mockLogProvider{entries: []CollectorLogEntry{{Level: "info"}}},
		Uploader: uploader,
		Version:  "1.0.0",
		IDGen:    func() string { return "fail-001" },
	})

	bundle, _, err := c.Generate(context.Background(), GenerateRequest{
		Sections: []string{"logs"},
	})
	if err == nil {
		t.Fatal("expected error")
	}
	if bundle.Status != StatusFailed {
		t.Errorf("expected status failed, got %s", bundle.Status)
	}
	if !strings.Contains(bundle.Error, "network timeout") {
		t.Errorf("expected error to contain 'network timeout', got %s", bundle.Error)
	}
}

func TestEncryptDecrypt(t *testing.T) {
	key := make([]byte, 32)
	rand.Read(key)

	plaintext := []byte("this is sensitive support bundle data")

	ciphertext, err := encrypt(key, plaintext)
	if err != nil {
		t.Fatalf("encrypt: %v", err)
	}

	if string(ciphertext) == string(plaintext) {
		t.Error("ciphertext should differ from plaintext")
	}

	decrypted, err := Decrypt(key, ciphertext)
	if err != nil {
		t.Fatalf("decrypt: %v", err)
	}

	if string(decrypted) != string(plaintext) {
		t.Errorf("decrypted = %q, want %q", decrypted, plaintext)
	}
}

func TestEncrypt_WrongKeyFails(t *testing.T) {
	key1 := make([]byte, 32)
	key2 := make([]byte, 32)
	rand.Read(key1)
	rand.Read(key2)

	ciphertext, err := encrypt(key1, []byte("secret data"))
	if err != nil {
		t.Fatalf("encrypt: %v", err)
	}

	_, err = Decrypt(key2, ciphertext)
	if err == nil {
		t.Error("expected decryption to fail with wrong key")
	}
}

func TestEncrypt_InvalidKeyLength(t *testing.T) {
	_, err := encrypt([]byte("short"), []byte("data"))
	if err == nil {
		t.Error("expected error for invalid key length")
	}
}

func TestCleanExpired(t *testing.T) {
	uploader := newMockUploader()
	uploader.uploaded["key1"] = []byte("data1")
	uploader.uploaded["key2"] = []byte("data2")

	c := NewCollector(CollectorConfig{
		Uploader: uploader,
	})

	bundles := []CollectorBundle{
		{BundleID: "old", StorageKey: "key1", ExpiresAt: time.Now().Add(-1 * time.Hour)},
		{BundleID: "fresh", StorageKey: "key2", ExpiresAt: time.Now().Add(24 * time.Hour)},
	}

	deleted, err := c.CleanExpired(context.Background(), bundles)
	if err != nil {
		t.Fatalf("CleanExpired: %v", err)
	}
	if deleted != 1 {
		t.Errorf("expected 1 deleted, got %d", deleted)
	}
	if len(uploader.deletedKeys) != 1 || uploader.deletedKeys[0] != "key1" {
		t.Errorf("expected key1 deleted, got %v", uploader.deletedKeys)
	}
}

func TestCleanExpired_NoUploader(t *testing.T) {
	c := NewCollector(CollectorConfig{})
	deleted, err := c.CleanExpired(context.Background(), []CollectorBundle{
		{BundleID: "x", StorageKey: "k", ExpiresAt: time.Now().Add(-1 * time.Hour)},
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if deleted != 0 {
		t.Errorf("expected 0 deleted, got %d", deleted)
	}
}

func TestDefaultHoursBack(t *testing.T) {
	var capturedHours int
	logs := &mockLogProvider{}
	// We'll verify through the manifest that defaults to 24.
	c := NewCollector(CollectorConfig{
		Logs:    logs,
		Version: "1.0.0",
		IDGen:   func() string { return "default-hours" },
	})

	bundle, payload, err := c.Generate(context.Background(), GenerateRequest{
		Sections: []string{"logs"},
	})
	if err != nil {
		t.Fatalf("Generate failed: %v", err)
	}
	_ = capturedHours
	_ = bundle

	// Check manifest for hours_back = 24.
	manifestData, err := ReadArchiveFile(payload, "manifest.json")
	if err != nil {
		t.Fatalf("read manifest: %v", err)
	}
	var manifest BundleManifest
	json.Unmarshal(manifestData, &manifest)
	if manifest.HoursBack != 24 {
		t.Errorf("expected hours_back=24, got %d", manifest.HoursBack)
	}
}

func TestGenerate_ProviderErrors_NonFatal(t *testing.T) {
	// When providers return errors, the bundle should still succeed with
	// error marker files.
	logs := &mockLogProvider{err: fmt.Errorf("log read failed")}
	cameras := &mockCameraProvider{err: fmt.Errorf("db connection lost")}
	hw := &mockHardwareProvider{err: fmt.Errorf("disk stat failed")}
	sidecars := &mockSidecarProvider{err: fmt.Errorf("network unreachable")}

	c := NewCollector(CollectorConfig{
		Logs:     logs,
		Cameras:  cameras,
		Hardware: hw,
		Sidecars: sidecars,
		Version:  "1.0.0",
		IDGen:    func() string { return "errors-001" },
	})

	bundle, payload, err := c.Generate(context.Background(), GenerateRequest{})
	if err != nil {
		t.Fatalf("Generate should not fail on provider errors: %v", err)
	}
	if bundle.Status != StatusReady {
		t.Errorf("expected ready, got %s", bundle.Status)
	}

	files, _ := ReadBundleArchive(payload)
	fileSet := make(map[string]bool)
	for _, f := range files {
		fileSet[f] = true
	}

	// Should have error marker files.
	if !fileSet["logs/error.json"] {
		t.Error("expected logs/error.json")
	}
	if !fileSet["cameras/error.json"] {
		t.Error("expected cameras/error.json")
	}
	if !fileSet["hardware/error.json"] {
		t.Error("expected hardware/error.json")
	}
	if !fileSet["sidecars/error.json"] {
		t.Error("expected sidecars/error.json")
	}
}

func TestReadSanitizedConfig(t *testing.T) {
	// Create a temp config file.
	tmpDir := t.TempDir()
	configPath := tmpDir + "/mediamtx.yml"

	config := `logLevel: debug
api: true
nvr: true
nvrJWTSecret: super-secret-key-123
paths:
  cam1:
    source: rtsp://admin:password123@192.168.1.100/stream
    password: mysecretpass
`
	if err := writeTestFile(configPath, config); err != nil {
		t.Fatal(err)
	}

	sanitized, err := readSanitizedConfig(configPath)
	if err != nil {
		t.Fatalf("readSanitizedConfig: %v", err)
	}

	s := string(sanitized)
	if strings.Contains(s, "super-secret-key-123") {
		t.Error("nvrJWTSecret was not redacted")
	}
	if strings.Contains(s, "mysecretpass") {
		t.Error("password was not redacted")
	}
	if !strings.Contains(s, "nvrJWTSecret: [REDACTED]") {
		t.Error("expected nvrJWTSecret to be redacted")
	}
	if !strings.Contains(s, "logLevel: debug") {
		t.Error("expected non-sensitive values to be preserved")
	}
}

func writeTestFile(path, content string) error {
	return os.WriteFile(path, []byte(content), 0o644)
}
