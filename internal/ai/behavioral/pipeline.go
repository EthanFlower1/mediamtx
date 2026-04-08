package behavioral

import (
	"context"
	"log/slog"
	"sync"
)

// Pipeline composes multiple Detectors for a single camera and delivers each
// incoming DetectionFrame to all detectors in parallel.
//
// The merged event stream is available via Pipeline.Events().
//
// Pipeline is safe for concurrent use: multiple goroutines may call Feed
// simultaneously.  Internal fan-out to detectors also runs in parallel.
type Pipeline struct {
	tenantID string
	cameraID string
	logger   *slog.Logger

	mu        sync.RWMutex
	detectors []Detector
	merged    chan BehavioralEvent
	closed    bool
	wg        sync.WaitGroup // waits for all merge goroutines
}

// NewPipeline creates a Pipeline for the given camera.
func NewPipeline(tenantID, cameraID string, logger *slog.Logger) *Pipeline {
	if logger == nil {
		logger = slog.Default()
	}
	return &Pipeline{
		tenantID: tenantID,
		cameraID: cameraID,
		logger:   logger.With("pipeline", "behavioral", "camera", cameraID),
		merged:   make(chan BehavioralEvent, 256),
	}
}

// AddDetector registers a Detector with the pipeline.  The pipeline starts a
// goroutine to fan events from the detector into the merged channel.
//
// AddDetector MUST be called before the first Feed call; adding a detector
// after feeding frames may cause missed events (the detector has no history).
// AddDetector is safe to call concurrently.
//
// AddDetector panics if called after Close.
func (p *Pipeline) AddDetector(d Detector) {
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.closed {
		panic("behavioral.Pipeline: AddDetector called after Close")
	}
	p.detectors = append(p.detectors, d)
	p.wg.Add(1)
	go p.merge(d)
}

// merge drains events from d into the merged channel until d.Events closes.
func (p *Pipeline) merge(d Detector) {
	defer p.wg.Done()
	for evt := range d.Events() {
		select {
		case p.merged <- evt:
		default:
			p.logger.Warn("pipeline merged channel full; dropping event",
				"kind", evt.Kind,
				"track_id", evt.TrackID)
		}
	}
}

// Feed delivers a DetectionFrame to all registered detectors in parallel.
// Feed returns when all detectors have accepted the frame (their Feed returns).
// If the pipeline is closed, Feed is a no-op.
func (p *Pipeline) Feed(ctx context.Context, frame DetectionFrame) {
	p.mu.RLock()
	dets := p.detectors
	p.mu.RUnlock()

	if len(dets) == 0 {
		return
	}

	var wg sync.WaitGroup
	wg.Add(len(dets))
	for _, d := range dets {
		go func() {
			defer wg.Done()
			d.Feed(ctx, frame)
		}()
	}
	wg.Wait()
}

// Events returns the merged read-only channel.  All behavioral events from all
// registered detectors flow through this channel.
func (p *Pipeline) Events() <-chan BehavioralEvent { return p.merged }

// Close closes all registered detectors and waits for the merged channel to
// drain.  After Close returns, the merged channel is closed.
func (p *Pipeline) Close() {
	p.mu.Lock()
	if p.closed {
		p.mu.Unlock()
		return
	}
	p.closed = true
	dets := p.detectors
	p.mu.Unlock()

	for _, d := range dets {
		d.Close()
	}
	// Wait for all merge goroutines to exit (they exit when detector channels close).
	p.wg.Wait()
	close(p.merged)
}

// ---------------------------------------------------------------------------
// Factory: build a Pipeline from a slice of DetectorConfigs
// ---------------------------------------------------------------------------

// BuildPipeline creates a Pipeline pre-loaded with Detectors built from cfgs.
// Only enabled configs are instantiated.  Configs for a different tenant or
// camera than tenantID/cameraID are silently skipped (isolation safeguard).
func BuildPipeline(tenantID, cameraID string, cfgs []DetectorConfig, logger *slog.Logger) (*Pipeline, error) {
	p := NewPipeline(tenantID, cameraID, logger)

	for _, cfg := range cfgs {
		if !cfg.Enabled {
			continue
		}
		// Isolation guard: never use a config for a different tenant.
		if cfg.TenantID != tenantID || cfg.CameraID != cameraID {
			if logger != nil {
				logger.Warn("skipping detector config: tenant/camera mismatch",
					"config_tenant", cfg.TenantID,
					"config_camera", cfg.CameraID,
					"expected_tenant", tenantID,
					"expected_camera", cameraID)
			}
			continue
		}

		switch cfg.Type {
		case DetectorTypeLoitering:
			params, err := cfg.LoiteringConfig()
			if err != nil {
				return nil, err
			}
			p.AddDetector(NewLoiteringDetector(cfg.ID, tenantID, cameraID, params, logger))

		case DetectorTypeLineCrossing:
			params, err := cfg.LineCrossingConfig()
			if err != nil {
				return nil, err
			}
			p.AddDetector(NewLineCrossingDetector(cfg.ID, tenantID, cameraID, params, logger))

		case DetectorTypeROI:
			params, err := cfg.ROIConfig()
			if err != nil {
				return nil, err
			}
			p.AddDetector(NewROIDetector(cfg.ID, tenantID, cameraID, params, logger))

		case DetectorTypeCrowdDensity:
			params, err := cfg.CrowdDensityConfig()
			if err != nil {
				return nil, err
			}
			p.AddDetector(NewCrowdDensityDetector(cfg.ID, tenantID, cameraID, params, logger))

		case DetectorTypeTailgating:
			params, err := cfg.TailgatingConfig()
			if err != nil {
				return nil, err
			}
			p.AddDetector(NewTailgatingDetector(cfg.ID, tenantID, cameraID, params, logger))

		case DetectorTypeFall:
			params, err := cfg.FallConfig()
			if err != nil {
				return nil, err
			}
			p.AddDetector(NewFallDetector(cfg.ID, tenantID, cameraID, params, logger))
		}
	}

	return p, nil
}
