// Package capturemanager implements recordercontrol.CaptureManager as a
// thin shim around mediamtxsupervisor.Reload(). The supervisor is the
// canonical mechanism for telling mediamtx what to record; this package
// just nudges it whenever the recordercontrol reconciler updates the
// authoritative camera set.
//
// RunningCameras returns the in-memory tracking state — it does NOT imply
// that the most recent Reload has been processed by the supervisor or
// applied by mediamtx. Callers needing acknowledged state should query
// the supervisor's stats or mediamtx's runtime /v3/paths/list directly.
package capturemanager

import (
	"log/slog"
	"sort"
	"sync"

	"github.com/bluenviron/mediamtx/internal/recorder/recordercontrol"
)

// Config holds the dependencies for Manager.
type Config struct {
	// Reload nudges the supervisor to re-render its config from state.Store
	// and push the result to mediamtx via HTTP. Required.
	Reload func()
	// Logger receives structured ops logs. Defaults to slog.Default if nil.
	Logger *slog.Logger
}

// Manager is a thin shim that tracks which cameras are "running" (i.e. have
// been pushed to mediamtx via the supervisor) and calls cfg.Reload whenever
// the camera set changes.
type Manager struct {
	cfg  Config
	mu   sync.Mutex
	seen map[string]int64 // cameraID → ConfigVersion
}

// New creates a Manager. Panics if cfg.Reload is nil.
func New(cfg Config) *Manager {
	if cfg.Reload == nil {
		panic("capturemanager: Config.Reload must not be nil")
	}
	if cfg.Logger == nil {
		cfg.Logger = slog.Default()
	}
	return &Manager{
		cfg:  cfg,
		seen: make(map[string]int64),
	}
}

// EnsureRunning records the camera in the in-memory map and nudges the
// supervisor via Reload if this is the first time we've seen this camera or
// its ConfigVersion has changed. If the same version was already recorded,
// this is a no-op (idempotent).
func (m *Manager) EnsureRunning(c recordercontrol.Camera) error {
	m.mu.Lock()
	if v, ok := m.seen[c.ID]; ok && v == c.ConfigVersion {
		m.mu.Unlock()
		return nil
	}
	m.seen[c.ID] = c.ConfigVersion
	m.mu.Unlock()

	m.cfg.Logger.Info("capturemanager: ensure running",
		slog.String("camera_id", c.ID),
		slog.Int64("config_version", c.ConfigVersion))
	m.cfg.Reload()
	return nil
}

// Stop removes the camera from the in-memory map and calls Reload so the
// supervisor can stop recording it. If the camera is not known, this is a
// no-op.
func (m *Manager) Stop(cameraID string) error {
	m.mu.Lock()
	if _, ok := m.seen[cameraID]; !ok {
		m.mu.Unlock()
		return nil
	}
	delete(m.seen, cameraID)
	m.mu.Unlock()

	m.cfg.Logger.Info("capturemanager: stop",
		slog.String("camera_id", cameraID))
	m.cfg.Reload()
	return nil
}

// RunningCameras returns a sorted slice of currently-tracked camera IDs.
func (m *Manager) RunningCameras() []string {
	m.mu.Lock()
	ids := make([]string, 0, len(m.seen))
	for id := range m.seen {
		ids = append(ids, id)
	}
	m.mu.Unlock()

	sort.Strings(ids)
	return ids
}
