// Package recordinghealth provides a runtime drift watchdog that reconciles
// the cameras the Recorder believes should be recording (from state.Store)
// against the cameras mediamtx is actually publishing (from its runtime
// /v3/paths/list endpoint).
//
// This complements the supervisor's config-level drift correction:
//   - Supervisor reconciles state.Store ↔ mediamtx CONFIG paths every ~5 s.
//   - Watchdog reconciles state.Store ↔ mediamtx RUNTIME publishing state.
//
// A path can be configured in mediamtx but not actually publishing if the
// RTSP source is unreachable, codec mismatched, etc.  The watchdog catches
// that layer by querying GET /v3/paths/list and checking the ready field.
// When a camera is drifted for DriftAcknowledgmentCycles consecutive cycles
// it emits a single Warn log and calls Reload() — once per drift episode.
package recordinghealth

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/bluenviron/mediamtx/internal/recorder/state"
)

const (
	defaultInterval              = 30 * time.Second
	defaultDriftAckCycles        = 2
	httpClientTimeout            = 5 * time.Second
	pathsPerPage                 = 200
)

// StateReader is the subset of state.Store consumed by the watchdog.
type StateReader interface {
	ListAssigned(ctx context.Context) ([]state.AssignedCamera, error)
}

// RuntimeClient queries mediamtx's runtime path state.
//
// ListPublishingCameras returns the camera IDs (path names with the configured
// prefix stripped) that mediamtx reports as ready/publishing.  The
// implementation is responsible for prefix filtering and stripping; the
// watchdog receives bare camera IDs and compares them directly against the
// IDs returned by state.Store.
type RuntimeClient interface {
	ListPublishingCameras(ctx context.Context) ([]string, error)
}

// Config holds all parameters for a Watchdog.
type Config struct {
	// Store reads the authoritative camera assignments.
	Store StateReader

	// MediaMTX queries mediamtx's runtime path state.
	MediaMTX RuntimeClient

	// Reload nudges the supervisor when persistent drift is detected.
	Reload func()

	// Interval between checks.  Defaults to 30 s if zero.
	Interval time.Duration

	// DriftAcknowledgmentCycles is how many consecutive drifted cycles a
	// camera must accumulate before we trigger a Reload + Warn log.
	// Defaults to 2.  A value of 1 triggers on the first drifted cycle.
	DriftAcknowledgmentCycles int

	// Logger receives structured ops logs.  Defaults to slog.Default().
	Logger *slog.Logger
}

func (c *Config) interval() time.Duration {
	if c.Interval > 0 {
		return c.Interval
	}
	return defaultInterval
}

func (c *Config) driftAckCycles() int {
	if c.DriftAcknowledgmentCycles > 0 {
		return c.DriftAcknowledgmentCycles
	}
	return defaultDriftAckCycles
}

func (c *Config) logger() *slog.Logger {
	if c.Logger != nil {
		return c.Logger
	}
	return slog.Default()
}

// Watchdog detects runtime drift between the expected set of recording cameras
// and mediamtx's actual publishing state.
type Watchdog struct {
	cfg      Config
	cycle    <-chan time.Time // production: ticker; tests: injected channel
	driftCnt map[string]int  // camera_id → consecutive drift count
	notified map[string]bool // camera_id → already alerted in current episode
}

// New constructs a Watchdog with a production time.Ticker.
// Panics if Store, MediaMTX, or Reload are nil.
func New(cfg Config) *Watchdog {
	if cfg.Store == nil {
		panic("recordinghealth: Config.Store must not be nil")
	}
	if cfg.MediaMTX == nil {
		panic("recordinghealth: Config.MediaMTX must not be nil")
	}
	if cfg.Reload == nil {
		panic("recordinghealth: Config.Reload must not be nil")
	}
	ticker := time.NewTicker(cfg.interval())
	return &Watchdog{
		cfg:      cfg,
		cycle:    ticker.C,
		driftCnt: make(map[string]int),
		notified: make(map[string]bool),
	}
}

// Run blocks until ctx is done.  It performs one immediate check then ticks at
// cfg.Interval.
//
// Each tick:
//  1. Read expected cameras from state.Store and actual from mediamtx runtime.
//  2. Compute drift = expected camera IDs - actual camera IDs.
//  3. Increment per-camera consecutive-drift counter; reset counters for
//     cameras that are no longer drifted.
//  4. For any camera whose counter reaches DriftAcknowledgmentCycles, emit a
//     single Warn log and call Reload().  Counter is reset so the next episode
//     starts fresh; the notified flag prevents duplicate alerts in the same
//     episode.
//
// Errors from Store or MediaMTX are logged at Error level and the cycle is
// skipped (no counter mutations, no Reload).
func (w *Watchdog) Run(ctx context.Context) {
	log := w.cfg.logger()

	// Perform an immediate first check before waiting for the first tick.
	w.check(ctx, log)

	for {
		select {
		case <-ctx.Done():
			return
		case <-w.cycle:
			w.check(ctx, log)
		}
	}
}

// check executes a single drift-detection cycle.
func (w *Watchdog) check(ctx context.Context, log *slog.Logger) {
	assigned, err := w.cfg.Store.ListAssigned(ctx)
	if err != nil {
		log.Error("recording-health: failed to list assigned cameras",
			slog.String("error", err.Error()))
		return
	}

	publishing, err := w.cfg.MediaMTX.ListPublishingCameras(ctx)
	if err != nil {
		log.Error("recording-health: failed to list publishing cameras from mediamtx",
			slog.String("error", err.Error()))
		return
	}

	// Build a set of expected IDs.
	expected := make(map[string]struct{}, len(assigned))
	for _, cam := range assigned {
		id := cam.CameraID
		if id == "" {
			id = cam.Config.ID
		}
		if id != "" {
			expected[id] = struct{}{}
		}
	}

	// Build a set of actually-publishing IDs.
	actual := make(map[string]struct{}, len(publishing))
	for _, id := range publishing {
		actual[id] = struct{}{}
	}

	// Update per-camera drift counters.
	ackCycles := w.cfg.driftAckCycles()

	// First, reset counters for cameras that are no longer drifted.
	for id := range w.driftCnt {
		if _, stillDrifted := expected[id]; !stillDrifted {
			// Camera was removed from assignments; clean up.
			delete(w.driftCnt, id)
			delete(w.notified, id)
			continue
		}
		if _, publishing := actual[id]; publishing {
			// Camera is now publishing; reset drift state.
			delete(w.driftCnt, id)
			delete(w.notified, id)
		}
	}

	// Accumulate drift for cameras still missing.
	for id := range expected {
		if _, ok := actual[id]; ok {
			continue // publishing — fine
		}
		w.driftCnt[id]++

		cnt := w.driftCnt[id]
		log.Debug("recording-health: camera drifted",
			slog.String("camera_id", id),
			slog.Int("consecutive_cycles", cnt),
			slog.Int("threshold", ackCycles))

		if cnt >= ackCycles && !w.notified[id] {
			log.Warn("recording-health: camera not publishing after persistent drift; triggering reload",
				slog.String("camera_id", id),
				slog.Int("consecutive_cycles", cnt))
			w.notified[id] = true
			// Reset counter so the next episode starts fresh after recovery.
			w.driftCnt[id] = 0
			w.cfg.Reload()
		}
	}
}

// ---------------------------------------------------------------------------
// HTTP RuntimeClient
// ---------------------------------------------------------------------------

// httpRuntimeClient implements RuntimeClient via mediamtx's paginated
// GET /v3/paths/list endpoint.
type httpRuntimeClient struct {
	baseURL    string
	pathPrefix string
	client     *http.Client
}

// NewHTTPRuntimeClient returns a RuntimeClient that queries mediamtx's
// runtime paths endpoint.
//
// baseURL is the mediamtx admin URL, e.g. "http://127.0.0.1:9997".
// pathPrefix is the prefix used by the supervisor, e.g. "cam_".
// client may be nil; a default client with a 5 s timeout is used.
func NewHTTPRuntimeClient(baseURL, pathPrefix string, client *http.Client) RuntimeClient {
	if client == nil {
		client = &http.Client{Timeout: httpClientTimeout}
	}
	return &httpRuntimeClient{
		baseURL:    strings.TrimRight(baseURL, "/"),
		pathPrefix: pathPrefix,
		client:     client,
	}
}

// pathsListResponse mirrors the mediamtx /v3/paths/list JSON shape.
type pathsListResponse struct {
	ItemCount int         `json:"itemCount"`
	PageCount int         `json:"pageCount"`
	Items     []pathsItem `json:"items"`
}

type pathsItem struct {
	Name  string `json:"name"`
	Ready bool   `json:"ready"`
}

// ListPublishingCameras fetches all runtime paths from mediamtx, filters by
// pathPrefix and ready==true, then returns the camera IDs (prefix stripped).
func (c *httpRuntimeClient) ListPublishingCameras(ctx context.Context) ([]string, error) {
	var out []string
	page := 1

	for {
		url := fmt.Sprintf("%s/v3/paths/list?itemsPerPage=%d&page=%d",
			c.baseURL, pathsPerPage, page)

		reqCtx, cancel := context.WithTimeout(ctx, httpClientTimeout)
		req, err := http.NewRequestWithContext(reqCtx, http.MethodGet, url, nil)
		if err != nil {
			cancel()
			return nil, fmt.Errorf("recordinghealth: build request: %w", err)
		}

		resp, err := c.client.Do(req)
		cancel()
		if err != nil {
			return nil, fmt.Errorf("recordinghealth: GET /v3/paths/list page %d: %w", page, err)
		}

		if resp.StatusCode != http.StatusOK {
			_ = resp.Body.Close()
			return nil, fmt.Errorf("recordinghealth: GET /v3/paths/list page %d: status %d",
				page, resp.StatusCode)
		}

		var body pathsListResponse
		if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
			_ = resp.Body.Close()
			return nil, fmt.Errorf("recordinghealth: decode /v3/paths/list page %d: %w", page, err)
		}
		_ = resp.Body.Close()

		for _, item := range body.Items {
			if !item.Ready {
				continue
			}
			if !strings.HasPrefix(item.Name, c.pathPrefix) {
				continue
			}
			cameraID := strings.TrimPrefix(item.Name, c.pathPrefix)
			out = append(out, cameraID)
		}

		if page >= body.PageCount {
			break
		}
		page++
	}

	return out, nil
}

// pageStr is only used internally in tests; exported for clarity.
func pageStr(n int) string {
	return strconv.Itoa(n)
}

// ensure pageStr is used (avoids "declared and not used" error).
var _ = pageStr
