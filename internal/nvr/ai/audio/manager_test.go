package audio

import (
	"testing"
)

func TestManager_EnableDisableCamera(t *testing.T) {
	pub := &mockEventPublisher{}
	mgr := NewManager("/tmp/models", pub)
	defer mgr.Stop()

	config := Config{
		CameraID:   "cam1",
		CameraName: "Test Camera",
		StreamURL:  "rtsp://test/stream",
		Enabled:    true,
	}

	if err := mgr.EnableCamera(config); err != nil {
		t.Fatalf("EnableCamera error: %v", err)
	}

	if !mgr.IsEnabled("cam1") {
		t.Error("expected cam1 to be enabled")
	}

	cameras := mgr.ActiveCameras()
	if len(cameras) != 1 || cameras[0] != "cam1" {
		t.Errorf("expected [cam1], got %v", cameras)
	}

	mgr.DisableCamera("cam1")
	if mgr.IsEnabled("cam1") {
		t.Error("expected cam1 to be disabled")
	}
}

func TestManager_Status(t *testing.T) {
	pub := &mockEventPublisher{}
	mgr := NewManager("/tmp/models", pub)
	defer mgr.Stop()

	// Load a model.
	mgr.Classifier().LoadModel(&AudioModel{
		EventType: EventGunshot,
		InferFunc: func(features []float32) (float32, error) { return 0.5, nil },
	})

	status := mgr.Status()
	if len(status.LoadedModels) != 1 || status.LoadedModels[0] != "gunshot" {
		t.Errorf("expected [gunshot] loaded, got %v", status.LoadedModels)
	}
}

func TestManager_DisableNonexistent(t *testing.T) {
	pub := &mockEventPublisher{}
	mgr := NewManager("/tmp/models", pub)
	defer mgr.Stop()

	// Should not panic.
	mgr.DisableCamera("nonexistent")
}

func TestValidateConfig(t *testing.T) {
	tests := []struct {
		name    string
		config  Config
		wantErr bool
	}{
		{
			name:    "empty camera ID",
			config:  Config{},
			wantErr: true,
		},
		{
			name:    "enabled without stream URL",
			config:  Config{CameraID: "c1", Enabled: true},
			wantErr: true,
		},
		{
			name: "valid config",
			config: Config{
				CameraID:  "c1",
				Enabled:   true,
				StreamURL: "rtsp://host/stream",
			},
			wantErr: false,
		},
		{
			name: "disabled no stream needed",
			config: Config{
				CameraID: "c1",
				Enabled:  false,
			},
			wantErr: false,
		},
		{
			name: "unknown event type",
			config: Config{
				CameraID:      "c1",
				Enabled:       true,
				StreamURL:     "rtsp://host/stream",
				EnabledEvents: []EventType{"unknown_event"},
			},
			wantErr: true,
		},
		{
			name: "invalid confidence threshold",
			config: Config{
				CameraID:  "c1",
				Enabled:   true,
				StreamURL: "rtsp://host/stream",
				ConfidenceThresholds: map[EventType]float32{
					EventGunshot: 1.5,
				},
			},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidateConfig(tt.config)
			if (err != nil) != tt.wantErr {
				t.Errorf("ValidateConfig() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestFalsePositiveBaseline(t *testing.T) {
	baseline := FalsePositiveBaseline()

	// Verify all event types have baselines.
	for _, evt := range AllEventTypes() {
		fp, ok := baseline[evt]
		if !ok {
			t.Errorf("missing FP baseline for %s", evt)
			continue
		}
		if fp.BaselineFPR < 0 || fp.BaselineFPR > 1 {
			t.Errorf("FPR for %s out of range: %f", evt, fp.BaselineFPR)
		}
		if fp.TestDataset == "" {
			t.Errorf("missing test dataset for %s", evt)
		}
		if fp.ConfThreshold <= 0 {
			t.Errorf("invalid confidence threshold for %s: %f", evt, fp.ConfThreshold)
		}
	}

	// Gunshot should have lowest FPR (most distinct sound signature).
	if baseline[EventGunshot].BaselineFPR > baseline[EventRaisedVoices].BaselineFPR {
		t.Error("gunshot FPR should be lower than raised voices FPR")
	}
}
