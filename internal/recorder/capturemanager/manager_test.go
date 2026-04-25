package capturemanager_test

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/bluenviron/mediamtx/internal/recorder/capturemanager"
	"github.com/bluenviron/mediamtx/internal/recorder/recordercontrol"
	"github.com/bluenviron/mediamtx/internal/recorder/yamlwriter"
	"github.com/bluenviron/mediamtx/internal/recorder/state"
)

// newTestWriter creates a temp YAML file seeded with "paths:\n" and returns a
// Writer pointing at it plus the file path for content inspection.
func newTestWriter(t *testing.T) (*yamlwriter.Writer, string) {
	t.Helper()
	dir := t.TempDir()
	path := filepath.Join(dir, "mediamtx.yml")
	if err := os.WriteFile(path, []byte("paths:\n"), 0o644); err != nil {
		t.Fatalf("seed file: %v", err)
	}
	return yamlwriter.New(path), path
}

// makeCamera returns a Camera whose ConfigJSON encodes a state.CameraConfig
// with the given RTSP URL.
func makeCamera(id, rtspURL string, version int64) recordercontrol.Camera {
	cfg := state.CameraConfig{
		ID:      id,
		Name:    "Test Camera " + id,
		RTSPURL: rtspURL,
	}
	b, _ := json.Marshal(cfg)
	return recordercontrol.Camera{
		ID:            id,
		ConfigJSON:    string(b),
		ConfigVersion: version,
	}
}

func TestRunningCameras_EmptyByDefault(t *testing.T) {
	w, _ := newTestWriter(t)
	mgr := capturemanager.New(capturemanager.Config{
		YAML:           w,
		RecordingsPath: "/tmp/recordings",
	})

	got := mgr.RunningCameras()
	if len(got) != 0 {
		t.Fatalf("expected no running cameras, got %v", got)
	}
}

func TestStop_OnUnknownCamera_NoError(t *testing.T) {
	w, _ := newTestWriter(t)
	mgr := capturemanager.New(capturemanager.Config{
		YAML:           w,
		RecordingsPath: "/tmp/recordings",
	})

	if err := mgr.Stop("does-not-exist"); err != nil {
		t.Fatalf("Stop on unknown camera returned error: %v", err)
	}
}

func TestEnsureRunning_AddsPathAndTracks(t *testing.T) {
	w, yamlPath := newTestWriter(t)
	reloadCount := 0
	mgr := capturemanager.New(capturemanager.Config{
		YAML:           w,
		RecordingsPath: "/recordings",
		Reload:         func() { reloadCount++ },
	})

	cam := makeCamera("cam-abc", "rtsp://192.168.1.10/stream1", 1)
	if err := mgr.EnsureRunning(cam); err != nil {
		t.Fatalf("EnsureRunning returned error: %v", err)
	}

	// RunningCameras must contain the ID.
	running := mgr.RunningCameras()
	if len(running) != 1 || running[0] != "cam-abc" {
		t.Fatalf("RunningCameras = %v, want [cam-abc]", running)
	}

	// Reload must have been called once.
	if reloadCount != 1 {
		t.Fatalf("reload count = %d, want 1", reloadCount)
	}

	// YAML file must contain the camera ID, the stream URL, and "record: true".
	data, err := os.ReadFile(yamlPath)
	if err != nil {
		t.Fatalf("read yaml: %v", err)
	}
	content := string(data)
	if !strings.Contains(content, "cam-abc") {
		t.Errorf("yaml does not contain camera ID 'cam-abc':\n%s", content)
	}
	if !strings.Contains(content, "rtsp://192.168.1.10/stream1") {
		t.Errorf("yaml does not contain stream URL:\n%s", content)
	}
	if !strings.Contains(content, "record: true") {
		t.Errorf("yaml does not contain 'record: true':\n%s", content)
	}
}

func TestEnsureRunning_Idempotent(t *testing.T) {
	w, _ := newTestWriter(t)
	reloadCount := 0
	mgr := capturemanager.New(capturemanager.Config{
		YAML:           w,
		RecordingsPath: "/recordings",
		Reload:         func() { reloadCount++ },
	})

	cam := makeCamera("cam-idem", "rtsp://192.168.1.20/stream", 5)

	if err := mgr.EnsureRunning(cam); err != nil {
		t.Fatalf("first EnsureRunning: %v", err)
	}
	if err := mgr.EnsureRunning(cam); err != nil {
		t.Fatalf("second EnsureRunning: %v", err)
	}

	// Reload should only be called once (same ConfigVersion).
	if reloadCount != 1 {
		t.Fatalf("reload count = %d, want 1 (idempotent)", reloadCount)
	}

	// Only one entry in RunningCameras.
	running := mgr.RunningCameras()
	if len(running) != 1 {
		t.Fatalf("RunningCameras = %v, want exactly 1 entry", running)
	}
}

func TestEnsureRunning_VersionChangeRestarts(t *testing.T) {
	w, _ := newTestWriter(t)
	reloadCount := 0
	mgr := capturemanager.New(capturemanager.Config{
		YAML:           w,
		RecordingsPath: "/recordings",
		Reload:         func() { reloadCount++ },
	})

	cam1 := makeCamera("cam-ver", "rtsp://10.0.0.1/live", 1)
	cam2 := makeCamera("cam-ver", "rtsp://10.0.0.1/live", 2)

	if err := mgr.EnsureRunning(cam1); err != nil {
		t.Fatalf("first EnsureRunning: %v", err)
	}
	if err := mgr.EnsureRunning(cam2); err != nil {
		t.Fatalf("second EnsureRunning: %v", err)
	}

	// Different ConfigVersion must trigger two reloads.
	if reloadCount != 2 {
		t.Fatalf("reload count = %d, want 2 (version change)", reloadCount)
	}
}

func TestStop_RemovesPath(t *testing.T) {
	w, yamlPath := newTestWriter(t)
	mgr := capturemanager.New(capturemanager.Config{
		YAML:           w,
		RecordingsPath: "/recordings",
	})

	cam := makeCamera("cam-stop", "rtsp://10.0.0.2/ch0", 1)
	if err := mgr.EnsureRunning(cam); err != nil {
		t.Fatalf("EnsureRunning: %v", err)
	}

	if err := mgr.Stop("cam-stop"); err != nil {
		t.Fatalf("Stop: %v", err)
	}

	// YAML file must no longer contain the camera ID.
	data, err := os.ReadFile(yamlPath)
	if err != nil {
		t.Fatalf("read yaml: %v", err)
	}
	if strings.Contains(string(data), "cam-stop") {
		t.Errorf("yaml still contains 'cam-stop' after Stop:\n%s", string(data))
	}

	// RunningCameras must be empty.
	running := mgr.RunningCameras()
	if len(running) != 0 {
		t.Fatalf("RunningCameras = %v, want empty after Stop", running)
	}
}
