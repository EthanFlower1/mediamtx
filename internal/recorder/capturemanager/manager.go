// Package capturemanager translates the imperative recordercontrol.CaptureManager
// interface into MediaMTX's declarative YAML model. Each EnsureRunning call
// performs an idempotent upsert of the camera's path entry in the YAML
// configuration file (gated by ConfigVersion) and fires a supervisor reload
// callback; Stop removes the path entry and reloads. The package never
// directly manages capture goroutines — it only mutates configuration state
// and signals the MediaMTX supervisor to act on it.
package capturemanager

import (
	"encoding/json"
	"fmt"
	"log/slog"
	"sort"
	"sync"

	"github.com/bluenviron/mediamtx/internal/recorder/recordercontrol"
	"github.com/bluenviron/mediamtx/internal/recorder/yamlwriter"
)

// cameraConfigJSON is the subset of state.CameraConfig we need to extract
// the stream URL from ConfigJSON. It mirrors state.CameraConfig's JSON tags
// for the fields we care about, allowing forward-compatibility with extra keys.
type cameraConfigJSON struct {
	RTSPURL string `json:"rtsp_url"`
}

// parseStreamURL extracts the RTSP URL from a Camera.ConfigJSON blob.
// Returns an error if the JSON is malformed or the URL is empty.
func parseStreamURL(configJSON string) (string, error) {
	if configJSON == "" {
		return "", fmt.Errorf("ConfigJSON is empty")
	}
	var cfg cameraConfigJSON
	if err := json.Unmarshal([]byte(configJSON), &cfg); err != nil {
		return "", fmt.Errorf("decode ConfigJSON: %w", err)
	}
	if cfg.RTSPURL == "" {
		return "", fmt.Errorf("stream URL (rtsp_url) is empty in ConfigJSON")
	}
	return cfg.RTSPURL, nil
}

// Config holds the dependencies for Manager.
type Config struct {
	// YAML is the writer used to mutate the MediaMTX configuration file.
	// Required. New panics if this is nil.
	YAML *yamlwriter.Writer

	// Reload is called after every successful YAML mutation to signal the
	// MediaMTX supervisor to reload its configuration. Defaults to a no-op
	// if nil. The supervisor's Reload is non-blocking and coalesces calls.
	Reload func()

	// RecordingsPath is the base directory under which per-camera recording
	// subdirectories are created.
	RecordingsPath string

	// Logger is used for debug/info messages. Defaults to slog.Default() if nil.
	Logger *slog.Logger
}

// Manager implements recordercontrol.CaptureManager by translating
// EnsureRunning/Stop calls into idempotent YAML path mutations followed by
// a supervisor reload. It tracks the last-applied ConfigVersion per camera so
// that repeated calls with the same version are no-ops.
type Manager struct {
	cfg      Config
	mu       sync.Mutex
	versions map[string]int64 // camera ID → last applied ConfigVersion
}

// New creates a new Manager with the given Config.
// Panics if cfg.YAML is nil.
func New(cfg Config) *Manager {
	if cfg.YAML == nil {
		panic("capturemanager: Config.YAML is required")
	}
	if cfg.Reload == nil {
		cfg.Reload = func() {}
	}
	if cfg.Logger == nil {
		cfg.Logger = slog.Default()
	}
	return &Manager{
		cfg:      cfg,
		versions: make(map[string]int64),
	}
}

// pathName returns the MediaMTX path name for the given camera ID.
func pathName(cameraID string) string {
	return "cam-" + cameraID
}

// EnsureRunning upserts the MediaMTX path entry for the camera and signals a
// supervisor reload. If the camera's ConfigVersion matches the last-applied
// version the call is a no-op. Returns an error if the stream URL is missing
// or the YAML write fails.
func (m *Manager) EnsureRunning(c recordercontrol.Camera) error {
	streamURL, err := parseStreamURL(c.ConfigJSON)
	if err != nil {
		return fmt.Errorf("capturemanager: camera %s: %w", c.ID, err)
	}

	m.mu.Lock()

	if v, ok := m.versions[c.ID]; ok && v == c.ConfigVersion {
		// Same version already applied — no-op.
		m.mu.Unlock()
		return nil
	}

	// recordPath uses the same cam-<id> prefix as the YAML path key (see
	// pathName) so the on-disk subdirectory mirrors the path operators
	// see in MediaMTX logs — no mental mapping between log lines and disk.
	pathCfg := map[string]interface{}{
		"source":     streamURL,
		"record":     true,
		"recordPath": fmt.Sprintf("%s/cam-%s/%%Y-%%m-%%d_%%H-%%M-%%S-%%f", m.cfg.RecordingsPath, c.ID),
	}

	if err := m.cfg.YAML.AddPath(pathName(c.ID), pathCfg); err != nil {
		m.mu.Unlock()
		return fmt.Errorf("capturemanager: camera %s: write yaml: %w", c.ID, err)
	}

	m.versions[c.ID] = c.ConfigVersion
	m.mu.Unlock()

	m.cfg.Logger.Debug("capturemanager: path upserted",
		slog.String("camera_id", c.ID),
		slog.Int64("config_version", c.ConfigVersion),
	)
	m.cfg.Reload()
	return nil
}

// Stop removes the MediaMTX path entry for the camera and signals a reload.
// If the camera is not tracked, the call is a no-op.
func (m *Manager) Stop(cameraID string) error {
	m.mu.Lock()

	if _, ok := m.versions[cameraID]; !ok {
		m.mu.Unlock()
		return nil
	}

	if err := m.cfg.YAML.RemovePath(pathName(cameraID)); err != nil {
		m.mu.Unlock()
		return fmt.Errorf("capturemanager: camera %s: remove yaml path: %w", cameraID, err)
	}

	delete(m.versions, cameraID)
	m.mu.Unlock()

	m.cfg.Logger.Debug("capturemanager: path removed", slog.String("camera_id", cameraID))
	m.cfg.Reload()
	return nil
}

// RunningCameras returns a sorted list of all camera IDs whose path entries
// are currently tracked by this Manager.
func (m *Manager) RunningCameras() []string {
	m.mu.Lock()
	defer m.mu.Unlock()

	ids := make([]string, 0, len(m.versions))
	for id := range m.versions {
		ids = append(ids, id)
	}
	sort.Strings(ids)
	return ids
}
