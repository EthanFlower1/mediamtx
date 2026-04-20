package mediamtxsupervisor

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"reflect"
	"sync"
	"time"

	"github.com/bluenviron/mediamtx/internal/recorder/state"
)

// CameraSource is anything that can return the current set of
// assigned cameras for this Recorder. The production implementation is
// a thin wrapper around *state.Store; tests inject a fake.
//
// ListAssigned must be safe for concurrent use.
type CameraSource interface {
	ListAssigned(ctx context.Context) ([]state.AssignedCamera, error)
}

// Controller is the surface the supervisor talks to in order to drive
// the underlying Raikada instance. The production implementation
// speaks HTTP to Raikada's `/v3/config/paths/replace` endpoints; tests
// substitute a recorder/fake.
//
// All methods must be safe for concurrent use.
type Controller interface {
	// ApplyPaths replaces the full set of Raikada paths the
	// Recorder owns with the given config. Implementations should
	// perform a hot reload — i.e. PATCH/POST to the path-config
	// API rather than restarting the Raikada process. Returning
	// an error signals "couldn't hot reload"; the supervisor will
	// log it and try again on the next change.
	ApplyPaths(ctx context.Context, set PathConfigSet) error

	// Healthy returns nil iff the Raikada HTTP API is reachable
	// and the controller can talk to it. The supervisor uses this
	// for its sidecar health probe.
	Healthy(ctx context.Context) error
}

// Config tunes the supervisor.
type Config struct {
	// Source is the assigned-cameras cache. Required.
	Source CameraSource

	// Controller is the Raikada-facing handle. Required.
	Controller Controller

	// Render holds the path-config rendering options. The zero
	// value uses Recorder defaults (sourceOnDemand: true, fmp4,
	// 1h segments, ./recordings/...).
	Render RenderOptions

	// PollInterval is the fallback poll cadence for picking up
	// cache changes when no explicit Reload signal is delivered.
	// Default: 5s. Setting it to 0 disables polling and forces
	// callers to drive the supervisor exclusively via Reload.
	PollInterval time.Duration

	// Logger is the base slog logger. nil means slog.Default().
	Logger *slog.Logger
}

func (c *Config) applyDefaults() {
	if c.PollInterval == 0 {
		c.PollInterval = 5 * time.Second
	}
	if c.Logger == nil {
		c.Logger = slog.Default()
	}
}

// Stats is a point-in-time snapshot of the supervisor's reload state.
type Stats struct {
	// PathsApplied is the number of paths in the most recent
	// successful ApplyPaths call.
	PathsApplied int

	// LastApplyAt is the wall-clock time of the most recent
	// successful ApplyPaths call.
	LastApplyAt time.Time

	// LastError is the most recent ApplyPaths or Source error
	// since the last successful reload. nil means everything is
	// healthy.
	LastError error

	// ReloadCount is the lifetime number of successful reloads.
	ReloadCount int

	// SkipCount is the lifetime number of cache changes we
	// processed but where the rendered config was byte-equal to
	// the previously-applied set.
	SkipCount int
}

// MediaMTXSupervisor is the orchestrator. Construct via New, call
// Start exactly once with a long-lived context, drive cache changes
// via Reload, and shut down by cancelling the context (or calling
// Close).
type MediaMTXSupervisor struct {
	cfg    Config
	logger *slog.Logger

	mu      sync.Mutex
	last    PathConfigSet
	stats   Stats
	started bool
	closed  bool

	// reloadCh fans Reload() requests into the background loop.
	// Buffered so callers don't block; if a reload is already
	// pending we coalesce.
	reloadCh chan struct{}

	doneCh chan struct{}
}

// New constructs a supervisor. It does NOT start the background loop;
// call Start.
func New(cfg Config) (*MediaMTXSupervisor, error) {
	if cfg.Source == nil {
		return nil, errors.New("mediamtxsupervisor: Source is required")
	}
	if cfg.Controller == nil {
		return nil, errors.New("mediamtxsupervisor: Controller is required")
	}
	cfg.applyDefaults()
	return &MediaMTXSupervisor{
		cfg:      cfg,
		logger:   cfg.Logger.With(slog.String("component", "mediamtx-supervisor")),
		reloadCh: make(chan struct{}, 1),
		doneCh:   make(chan struct{}),
	}, nil
}

// Start kicks off the background reconcile loop. It returns
// immediately. The loop exits when ctx is cancelled or Close is
// called, whichever comes first.
//
// Start performs an initial synchronous reload before returning so
// that the first call to Stats reflects steady state. The initial
// reload's error (if any) is returned to the caller.
func (s *MediaMTXSupervisor) Start(ctx context.Context) error {
	s.mu.Lock()
	if s.started {
		s.mu.Unlock()
		return errors.New("mediamtxsupervisor: already started")
	}
	if s.closed {
		s.mu.Unlock()
		return errors.New("mediamtxsupervisor: closed")
	}
	s.started = true
	s.mu.Unlock()

	initialErr := s.reloadOnce(ctx)
	if initialErr != nil {
		// Fail-open: log but keep going. The background loop
		// will retry on the next tick / Reload.
		s.logger.Warn("initial path reload failed (continuing fail-open)",
			slog.Any("error", initialErr))
	}

	go s.loop(ctx)
	return initialErr
}

// Close stops the background loop and waits for it to exit. It is
// safe to call multiple times.
func (s *MediaMTXSupervisor) Close() {
	s.mu.Lock()
	if s.closed {
		s.mu.Unlock()
		return
	}
	s.closed = true
	close(s.reloadCh)
	s.mu.Unlock()
	<-s.doneCh
}

// Reload requests an asynchronous path reload. Multiple concurrent
// callers are coalesced into one reload. Reload never blocks.
//
// Cache writers (e.g. directoryingest applying a ReconcileDiff) should
// call Reload after committing changes to the state.Store.
func (s *MediaMTXSupervisor) Reload() {
	s.mu.Lock()
	if s.closed || !s.started {
		s.mu.Unlock()
		return
	}
	s.mu.Unlock()
	select {
	case s.reloadCh <- struct{}{}:
	default:
		// A reload is already pending; coalesce.
	}
}

// Stats returns a snapshot of the supervisor's reload state.
func (s *MediaMTXSupervisor) Stats() Stats {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.stats
}

func (s *MediaMTXSupervisor) loop(ctx context.Context) {
	defer close(s.doneCh)

	var tick <-chan time.Time
	if s.cfg.PollInterval > 0 {
		t := time.NewTicker(s.cfg.PollInterval)
		defer t.Stop()
		tick = t.C
	}

	for {
		select {
		case <-ctx.Done():
			return
		case _, ok := <-s.reloadCh:
			if !ok {
				return
			}
			if err := s.reloadOnce(ctx); err != nil {
				s.logger.Warn("reload failed",
					slog.Any("error", err))
			}
		case <-tick:
			if err := s.reloadOnce(ctx); err != nil {
				s.logger.Warn("poll reload failed",
					slog.Any("error", err))
			}
		}
	}
}

// reloadOnce performs one rendering + ApplyPaths cycle. It is the
// single point where Stats is mutated.
func (s *MediaMTXSupervisor) reloadOnce(ctx context.Context) error {
	cams, err := s.cfg.Source.ListAssigned(ctx)
	if err != nil {
		// Fail-open: do not clear the previously-applied set.
		s.recordError(fmt.Errorf("list assigned cameras: %w", err))
		return err
	}

	set, renderErr := RenderPaths(cams, s.cfg.Render)
	if renderErr != nil {
		// RenderPaths returns a non-fatal *RenderError listing
		// skipped cameras; we still apply the partial set.
		s.logger.Warn("path rendering produced skips",
			slog.Any("error", renderErr))
	}

	s.mu.Lock()
	if reflect.DeepEqual(s.last, set) {
		s.stats.SkipCount++
		s.mu.Unlock()
		return nil
	}
	s.mu.Unlock()

	if err := s.cfg.Controller.ApplyPaths(ctx, set); err != nil {
		s.recordError(fmt.Errorf("apply paths: %w", err))
		return err
	}

	s.mu.Lock()
	s.last = set
	s.stats.LastApplyAt = time.Now().UTC()
	s.stats.PathsApplied = len(set.Paths)
	s.stats.LastError = nil
	s.stats.ReloadCount++
	s.mu.Unlock()

	s.logger.Info("applied raikada path config",
		slog.Int("paths", len(set.Paths)),
		slog.Int("reload_count", s.stats.ReloadCount))
	return nil
}

func (s *MediaMTXSupervisor) recordError(err error) {
	s.mu.Lock()
	s.stats.LastError = err
	s.mu.Unlock()
}

// ---- CameraSource adapter for *state.Store -----------------------------

// StoreSource adapts *state.Store to the CameraSource interface.
//
// We don't put this on *state.Store directly to keep the state package
// free of mediamtxsupervisor-specific concerns; the adapter lives here.
type StoreSource struct {
	Store *state.Store
}

// ListAssigned implements CameraSource.
func (s StoreSource) ListAssigned(ctx context.Context) ([]state.AssignedCamera, error) {
	if s.Store == nil {
		return nil, errors.New("mediamtxsupervisor: nil state.Store")
	}
	return s.Store.ListAssigned(ctx)
}
