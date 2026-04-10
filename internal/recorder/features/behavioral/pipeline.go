package behavioral

import (
	"context"
	"log/slog"
	"sync"
)

// ConfigSource is the seam the Pipeline uses to fetch behavioral configs at
// startup or on demand. In production this is backed by a local cache
// populated from StreamAssignments events (KAI-253). In unit tests a simple
// map-backed stub satisfies the interface.
type ConfigSource interface {
	// GetCameraConfig returns the full behavioral configuration for a camera.
	// Returns an empty CameraConfig (not an error) if no config has been
	// pushed for the camera yet.
	GetCameraConfig(ctx context.Context, tenantID, cameraID string) (CameraConfig, error)
}

// Pipeline is the recorder-side orchestrator for all six behavioral detectors.
// It holds a per-camera config cache, loads configs on startup, and re-applies
// configs when the reconciler delivers a camera update.
//
// NOTE (KAI-429): This is the persistence-plumbing release. Full frame-level
// inference wiring (KAI-284) is a separate commit on the KAI-284 branch that
// will merge after this lands on main. The public API surface below is stable
// for KAI-284 to build on.
type Pipeline struct {
	source ConfigSource
	logger *slog.Logger

	mu      sync.RWMutex
	cameras map[string]CameraConfig // keyed by camera_id
}

// NewPipeline constructs a Pipeline with the provided config source.
// Logger may be nil; defaults to slog.Default().
func NewPipeline(source ConfigSource, logger *slog.Logger) *Pipeline {
	if logger == nil {
		logger = slog.Default()
	}
	return &Pipeline{
		source:  source,
		logger:  logger,
		cameras: make(map[string]CameraConfig),
	}
}

// LoadConfig fetches and caches the behavioral config for a single camera.
// Called at startup by the recorder for each assigned camera, and again
// whenever the reconciler applies a CameraUpdated event.
//
// Thread-safe: multiple goroutines may call LoadConfig concurrently for
// different cameras.
func (p *Pipeline) LoadConfig(ctx context.Context, tenantID, cameraID string) error {
	cfg, err := p.source.GetCameraConfig(ctx, tenantID, cameraID)
	if err != nil {
		return err
	}
	p.mu.Lock()
	p.cameras[cameraID] = cfg
	p.mu.Unlock()
	p.logger.Debug("behavioral: loaded config",
		"tenant_id", tenantID,
		"camera_id", cameraID,
		"detector_count", len(cfg.Detectors),
	)
	return nil
}

// GetConfig returns the cached config for a camera. Returns an empty
// CameraConfig if LoadConfig has not yet been called for this camera.
// Safe for concurrent reads.
func (p *Pipeline) GetConfig(cameraID string) CameraConfig {
	p.mu.RLock()
	defer p.mu.RUnlock()
	return p.cameras[cameraID]
}

// IsEnabled reports whether a given detector is enabled for a camera.
// Returns false if no config is loaded for the camera or the detector type.
func (p *Pipeline) IsEnabled(cameraID string, dt DetectorType) bool {
	cfg := p.GetConfig(cameraID)
	for _, d := range cfg.Detectors {
		if d.DetectorType == dt {
			return d.Enabled
		}
	}
	return false
}

// DetectorParams returns the raw JSON params for a detector on a camera.
// Returns "{}" if no config is loaded or the detector is not configured.
func (p *Pipeline) DetectorParams(cameraID string, dt DetectorType) string {
	cfg := p.GetConfig(cameraID)
	for _, d := range cfg.Detectors {
		if d.DetectorType == dt {
			if d.Params == "" {
				return "{}"
			}
			return d.Params
		}
	}
	return "{}"
}

// RemoveCamera evicts the config for a camera that has been unassigned.
// Called by the reconciler on CameraRemoved events.
func (p *Pipeline) RemoveCamera(cameraID string) {
	p.mu.Lock()
	delete(p.cameras, cameraID)
	p.mu.Unlock()
}
