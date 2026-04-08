package behavioral_test

import (
	"context"
	"testing"

	"github.com/bluenviron/mediamtx/internal/recorder/features/behavioral"
)

// -----------------------------------------------------------------------
// Mock ConfigSource
// -----------------------------------------------------------------------

type mockSource struct {
	configs map[string]behavioral.CameraConfig
}

func (m *mockSource) GetCameraConfig(_ context.Context, _, cameraID string) (behavioral.CameraConfig, error) {
	if cfg, ok := m.configs[cameraID]; ok {
		return cfg, nil
	}
	return behavioral.CameraConfig{}, nil
}

// -----------------------------------------------------------------------
// Tests
// -----------------------------------------------------------------------

func TestPipeline_LoadConfig_SetsCache(t *testing.T) {
	loiteringParams := `{"roi_polygon":[[0,0],[1,0],[1,1]],"threshold_seconds":30}`
	src := &mockSource{
		configs: map[string]behavioral.CameraConfig{
			"cam-1": {
				CameraID: "cam-1",
				Detectors: []behavioral.CameraDetectorConfig{
					{DetectorType: behavioral.DetectorLoitering, Params: loiteringParams, Enabled: true},
				},
			},
		},
	}

	p := behavioral.NewPipeline(src, nil)
	if err := p.LoadConfig(context.Background(), "tenant-a", "cam-1"); err != nil {
		t.Fatalf("LoadConfig: %v", err)
	}

	cfg := p.GetConfig("cam-1")
	if cfg.CameraID != "cam-1" {
		t.Errorf("CameraID = %q, want %q", cfg.CameraID, "cam-1")
	}
	if len(cfg.Detectors) != 1 {
		t.Errorf("Detectors count = %d, want 1", len(cfg.Detectors))
	}
}

func TestPipeline_IsEnabled(t *testing.T) {
	src := &mockSource{
		configs: map[string]behavioral.CameraConfig{
			"cam-1": {
				CameraID: "cam-1",
				Detectors: []behavioral.CameraDetectorConfig{
					{DetectorType: behavioral.DetectorLoitering, Enabled: true},
					{DetectorType: behavioral.DetectorROI, Enabled: false},
				},
			},
		},
	}

	p := behavioral.NewPipeline(src, nil)
	_ = p.LoadConfig(context.Background(), "tenant-a", "cam-1")

	if !p.IsEnabled("cam-1", behavioral.DetectorLoitering) {
		t.Error("Loitering should be enabled")
	}
	if p.IsEnabled("cam-1", behavioral.DetectorROI) {
		t.Error("ROI should not be enabled")
	}
	// Detector not in config → false.
	if p.IsEnabled("cam-1", behavioral.DetectorFallDetection) {
		t.Error("FallDetection not configured should return false")
	}
}

func TestPipeline_IsEnabled_UnknownCamera(t *testing.T) {
	p := behavioral.NewPipeline(&mockSource{configs: map[string]behavioral.CameraConfig{}}, nil)
	if p.IsEnabled("nonexistent", behavioral.DetectorLoitering) {
		t.Error("IsEnabled on unknown camera should return false")
	}
}

func TestPipeline_DetectorParams(t *testing.T) {
	params := `{"roi_polygon":[[0,0],[1,0],[1,1]],"threshold_seconds":30}`
	src := &mockSource{
		configs: map[string]behavioral.CameraConfig{
			"cam-1": {
				CameraID: "cam-1",
				Detectors: []behavioral.CameraDetectorConfig{
					{DetectorType: behavioral.DetectorLoitering, Params: params, Enabled: true},
				},
			},
		},
	}

	p := behavioral.NewPipeline(src, nil)
	_ = p.LoadConfig(context.Background(), "tenant-a", "cam-1")

	got := p.DetectorParams("cam-1", behavioral.DetectorLoitering)
	if got != params {
		t.Errorf("params = %q, want %q", got, params)
	}
	// Unknown detector returns "{}"
	if d := p.DetectorParams("cam-1", behavioral.DetectorROI); d != "{}" {
		t.Errorf("unknown detector params = %q, want {}", d)
	}
}

func TestPipeline_RemoveCamera(t *testing.T) {
	src := &mockSource{
		configs: map[string]behavioral.CameraConfig{
			"cam-1": {CameraID: "cam-1", Detectors: []behavioral.CameraDetectorConfig{
				{DetectorType: behavioral.DetectorTailgating, Enabled: true},
			}},
		},
	}

	p := behavioral.NewPipeline(src, nil)
	_ = p.LoadConfig(context.Background(), "tenant-a", "cam-1")

	if !p.IsEnabled("cam-1", behavioral.DetectorTailgating) {
		t.Fatal("should be enabled before removal")
	}
	p.RemoveCamera("cam-1")
	if p.IsEnabled("cam-1", behavioral.DetectorTailgating) {
		t.Error("should not be enabled after RemoveCamera")
	}
}

func TestPipeline_LoadConfig_Empty(t *testing.T) {
	// Source returns empty config — should not error.
	src := &mockSource{configs: map[string]behavioral.CameraConfig{}}
	p := behavioral.NewPipeline(src, nil)
	if err := p.LoadConfig(context.Background(), "tenant-a", "cam-missing"); err != nil {
		t.Fatalf("LoadConfig on missing camera: %v", err)
	}
	cfg := p.GetConfig("cam-missing")
	if len(cfg.Detectors) != 0 {
		t.Errorf("expected 0 detectors, got %d", len(cfg.Detectors))
	}
}

func TestParseCameraConfig_RoundTrip(t *testing.T) {
	raw := `{"camera_id":"cam-x","detectors":[` +
		`{"detector_type":"roi","params":"{\"roi_polygon\":[[0,0],[1,0],[1,1]]}","enabled":true}` +
		`]}`
	cfg, err := behavioral.ParseCameraConfig(raw)
	if err != nil {
		t.Fatalf("ParseCameraConfig: %v", err)
	}
	if cfg.CameraID != "cam-x" {
		t.Errorf("CameraID = %q, want cam-x", cfg.CameraID)
	}
	if len(cfg.Detectors) != 1 {
		t.Errorf("Detectors = %d, want 1", len(cfg.Detectors))
	}
	if cfg.Detectors[0].DetectorType != behavioral.DetectorROI {
		t.Errorf("DetectorType = %q, want roi", cfg.Detectors[0].DetectorType)
	}
}

func TestParseCameraConfig_Empty(t *testing.T) {
	cfg, err := behavioral.ParseCameraConfig("")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(cfg.Detectors) != 0 {
		t.Errorf("expected empty detectors, got %d", len(cfg.Detectors))
	}
}
