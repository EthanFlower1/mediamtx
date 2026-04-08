package lpr

import (
	"context"
	"errors"
	"log/slog"
	"sync"

	"github.com/bluenviron/mediamtx/internal/recorder/features/lpr/watchlist"
)

// Pipeline subscribes to vehicle detection events from the object-detection
// pipeline (KAI-281) and runs the two-stage LPR pipeline on each triggered
// frame. It routes watchlist matches to the LPREventPublisher and stores all
// reads via the same publisher.
//
// Create a Pipeline with NewPipeline, subscribe its HandleVehicleEvent method
// to the object-detection event bus, and call Stop to shut down.
type Pipeline struct {
	detector  *Detector
	wlMatcher *watchlist.Matcher
	publisher LPREventPublisher
	logger    *slog.Logger

	// perCamera is keyed by camera_id and provides the per-camera LPR config.
	perCamera map[string]CameraLPRConfig

	mu     sync.RWMutex
	closed bool
	wg     sync.WaitGroup
}

// PipelineConfig holds the top-level Pipeline settings.
type PipelineConfig struct {
	// Detector is the initialised two-stage LPR detector. Required.
	Detector *Detector

	// WatchlistMatcher provides bloom-filter-backed watchlist matching.
	// May be nil; if nil watchlist matching is disabled.
	WatchlistMatcher *watchlist.Matcher

	// Publisher receives every plate read and watchlist match. Required.
	Publisher LPREventPublisher

	// Logger is used for pipeline-level structured logging. If nil,
	// slog.Default() is used.
	Logger *slog.Logger

	// CameraConfigs is the initial per-camera LPR configuration. May be
	// updated at runtime via UpdateCameraConfig.
	CameraConfigs map[string]CameraLPRConfig
}

// NewPipeline creates a new LPR Pipeline.
func NewPipeline(cfg PipelineConfig) (*Pipeline, error) {
	if cfg.Detector == nil {
		return nil, ErrInvalidConfig
	}
	if cfg.Publisher == nil {
		return nil, ErrInvalidConfig
	}
	logger := cfg.Logger
	if logger == nil {
		logger = slog.Default()
	}
	cams := cfg.CameraConfigs
	if cams == nil {
		cams = make(map[string]CameraLPRConfig)
	}
	return &Pipeline{
		detector:  cfg.Detector,
		wlMatcher: cfg.WatchlistMatcher,
		publisher: cfg.Publisher,
		logger:    logger,
		perCamera: cams,
	}, nil
}

// UpdateCameraConfig atomically replaces the per-camera config for the given
// camera. Safe to call at runtime (e.g., when an operator toggles lpr_enabled
// via the admin UI).
func (p *Pipeline) UpdateCameraConfig(cameraID string, cfg CameraLPRConfig) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.perCamera[cameraID] = cfg
}

// HandleVehicleEvent processes a single vehicle detection event. It is
// intended to be called from the object-detection pipeline's sink (either
// directly or via a goroutine). It is safe for concurrent use.
//
// The function is deliberately synchronous in its inference path so that
// callers control concurrency. Production callers should dispatch in a
// bounded goroutine pool to avoid blocking the detection pipeline.
func (p *Pipeline) HandleVehicleEvent(ctx context.Context, ev VehicleEvent) {
	p.mu.RLock()
	closed := p.closed
	camCfg, hasCam := p.perCamera[ev.CameraID]
	p.mu.RUnlock()

	if closed {
		return
	}
	if !hasCam || !camCfg.Enabled {
		return
	}

	reads, err := p.detector.ProcessFrame(ctx, ev.Frame, camCfg)
	if err != nil {
		if !errors.Is(err, ErrNoPlateFound) {
			p.logger.LogAttrs(ctx, slog.LevelError, "lpr_process_error",
				slog.String("camera_id", ev.CameraID),
				slog.String("error", err.Error()),
			)
		}
		return
	}

	for i := range reads {
		// Backfill tenant / camera / time context from the triggering event.
		reads[i].TenantID = ev.TenantID
		reads[i].CameraID = ev.CameraID
		reads[i].Timestamp = ev.CapturedAt

		read := reads[i]

		// Publish the raw read.
		if pubErr := p.publisher.PublishRead(ctx, read); pubErr != nil {
			p.logger.LogAttrs(ctx, slog.LevelError, "lpr_publish_error",
				slog.String("camera_id", ev.CameraID),
				slog.String("error", pubErr.Error()),
			)
		}

		// Watchlist matching.
		if p.wlMatcher != nil {
			match, matchErr := p.wlMatcher.Match(ctx, ev.TenantID, read.PlateText)
			if matchErr != nil {
				p.logger.LogAttrs(ctx, slog.LevelError, "lpr_watchlist_error",
					slog.String("camera_id", ev.CameraID),
					slog.String("error", matchErr.Error()),
				)
				continue
			}
			if match != nil && (match.Type == watchlist.TypeDeny || match.Type == watchlist.TypeAlert) {
				matchEv := WatchlistMatchEvent{
					PlateRead:    read,
					WatchlistID:  match.WatchlistID,
					EntryID:      match.EntryID,
					WatchlistType: string(match.Type),
				}
				if pubErr := p.publisher.PublishWatchlistMatch(ctx, matchEv); pubErr != nil {
					p.logger.LogAttrs(ctx, slog.LevelError, "lpr_watchlist_publish_error",
						slog.String("camera_id", ev.CameraID),
						slog.String("error", pubErr.Error()),
					)
				}
			}
		}
	}
}

// Stop shuts down the pipeline and waits for any in-flight handlers to
// complete. After Stop, HandleVehicleEvent is a no-op.
func (p *Pipeline) Stop() {
	p.mu.Lock()
	p.closed = true
	p.mu.Unlock()
	p.wg.Wait()
}
