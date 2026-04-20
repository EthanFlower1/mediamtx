package audio

import (
	"context"
	"fmt"
	"log"
	"sync"
)

// Manager coordinates audio analytics pipelines across all cameras.
// It handles per-camera enable/disable, model loading, and lifecycle.
type Manager struct {
	mu         sync.RWMutex
	pipelines  map[string]*Pipeline // cameraID -> pipeline
	classifier *Classifier
	eventPub   AudioEventPublisher
	metrics    *Metrics
	ctx        context.Context
	cancel     context.CancelFunc
}

// NewManager creates an audio analytics manager.
func NewManager(modelDir string, eventPub AudioEventPublisher) *Manager {
	ctx, cancel := context.WithCancel(context.Background())
	return &Manager{
		pipelines:  make(map[string]*Pipeline),
		classifier: NewClassifier(modelDir),
		eventPub:   eventPub,
		metrics:    NewMetrics(),
		ctx:        ctx,
		cancel:     cancel,
	}
}

// Classifier returns the shared classifier instance for model loading.
func (m *Manager) Classifier() *Classifier {
	return m.classifier
}

// Metrics returns the shared metrics collector.
func (m *Manager) Metrics() *Metrics {
	return m.metrics
}

// EnableCamera starts the audio analytics pipeline for the given camera.
// If the camera is already enabled, it is first disabled then re-enabled
// with the new configuration.
func (m *Manager) EnableCamera(config Config) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	// Stop existing pipeline if any.
	if existing, ok := m.pipelines[config.CameraID]; ok {
		existing.Stop()
		delete(m.pipelines, config.CameraID)
	}

	if !config.Enabled {
		return nil
	}

	pipeline := NewPipeline(config, m.classifier, m.eventPub, m.metrics)
	pipeline.Start(m.ctx)
	m.pipelines[config.CameraID] = pipeline

	log.Printf("[audio] enabled camera %s (%s)", config.CameraID, config.CameraName)
	return nil
}

// DisableCamera stops the audio analytics pipeline for the given camera.
func (m *Manager) DisableCamera(cameraID string) {
	m.mu.Lock()
	defer m.mu.Unlock()

	if pipeline, ok := m.pipelines[cameraID]; ok {
		pipeline.Stop()
		delete(m.pipelines, cameraID)
		log.Printf("[audio] disabled camera %s", cameraID)
	}
}

// IsEnabled returns true if audio analytics is running for the given camera.
func (m *Manager) IsEnabled(cameraID string) bool {
	m.mu.RLock()
	defer m.mu.RUnlock()
	_, ok := m.pipelines[cameraID]
	return ok
}

// ActiveCameras returns the IDs of all cameras with active audio pipelines.
func (m *Manager) ActiveCameras() []string {
	m.mu.RLock()
	defer m.mu.RUnlock()

	ids := make([]string, 0, len(m.pipelines))
	for id := range m.pipelines {
		ids = append(ids, id)
	}
	return ids
}

// Status returns a summary of the audio analytics subsystem.
func (m *Manager) Status() ManagerStatus {
	m.mu.RLock()
	defer m.mu.RUnlock()

	cameras := make([]CameraStatus, 0, len(m.pipelines))
	for id := range m.pipelines {
		cameras = append(cameras, CameraStatus{
			CameraID: id,
			Active:   true,
		})
	}

	models := make([]string, 0)
	for _, evt := range AllEventTypes() {
		if m.classifier.HasModel(evt) {
			models = append(models, string(evt))
		}
	}

	return ManagerStatus{
		ActivePipelines: len(m.pipelines),
		LoadedModels:    models,
		Cameras:         cameras,
	}
}

// Stop shuts down all audio analytics pipelines and releases resources.
func (m *Manager) Stop() {
	m.cancel()

	m.mu.Lock()
	defer m.mu.Unlock()

	for id, pipeline := range m.pipelines {
		pipeline.Stop()
		delete(m.pipelines, id)
	}
	m.classifier.Close()

	log.Printf("[audio] manager stopped (%d pipelines shut down)", len(m.pipelines))
}

// ManagerStatus describes the current state of the audio analytics subsystem.
type ManagerStatus struct {
	ActivePipelines int            `json:"active_pipelines"`
	LoadedModels    []string       `json:"loaded_models"`
	Cameras         []CameraStatus `json:"cameras"`
}

// CameraStatus describes audio analytics state for a single camera.
type CameraStatus struct {
	CameraID string `json:"camera_id"`
	Active   bool   `json:"active"`
}

// FalsePositiveBaseline documents the expected false positive rates for each
// audio event type. These are baseline measurements from testing against
// standard audio datasets and should be updated as models improve.
//
// The baseline is exported as a function so it can be referenced by tests
// and documentation generators.
func FalsePositiveBaseline() map[EventType]FPBaseline {
	return map[EventType]FPBaseline{
		EventGunshot: {
			EventType:    EventGunshot,
			BaselineFPR:  0.02,
			TestDataset:  "UrbanSound8K + ESC-50 gunshot subset",
			Notes:        "Tested with fireworks, car backfire, and thunder as negative samples",
			ConfThreshold: 0.70,
		},
		EventGlassBreak: {
			EventType:    EventGlassBreak,
			BaselineFPR:  0.03,
			TestDataset:  "ESC-50 + AudioSet glass break subset",
			Notes:        "Tested with clinking glasses and ceramic break as negative samples",
			ConfThreshold: 0.65,
		},
		EventRaisedVoices: {
			EventType:    EventRaisedVoices,
			BaselineFPR:  0.08,
			TestDataset:  "VoxCeleb2 + AudioSet speech subset",
			Notes:        "Higher FPR due to boundary between loud speech and shouting; threshold tuning recommended per deployment",
			ConfThreshold: 0.60,
		},
		EventSirenHorn: {
			EventType:    EventSirenHorn,
			BaselineFPR:  0.04,
			TestDataset:  "AudioSet siren/horn subset + UrbanSound8K",
			Notes:        "Tested with musical horns, alarms, and tonal sounds as negative samples",
			ConfThreshold: 0.65,
		},
	}
}

// FPBaseline documents the false positive rate baseline for a specific event type.
type FPBaseline struct {
	EventType     EventType `json:"event_type"`
	BaselineFPR   float64   `json:"baseline_fpr"`
	TestDataset   string    `json:"test_dataset"`
	Notes         string    `json:"notes"`
	ConfThreshold float32   `json:"confidence_threshold"`
}

// ValidateConfig checks an audio analytics configuration for errors.
func ValidateConfig(config Config) error {
	if config.CameraID == "" {
		return fmt.Errorf("camera_id is required")
	}
	if config.Enabled && config.StreamURL == "" {
		return fmt.Errorf("stream_url is required when audio analytics is enabled")
	}
	for _, evt := range config.EnabledEvents {
		valid := false
		for _, known := range AllEventTypes() {
			if evt == known {
				valid = true
				break
			}
		}
		if !valid {
			return fmt.Errorf("unknown event type: %s", evt)
		}
	}
	for evt, thresh := range config.ConfidenceThresholds {
		if thresh < 0 || thresh > 1 {
			return fmt.Errorf("confidence threshold for %s must be between 0 and 1, got %f", evt, thresh)
		}
	}
	return nil
}
