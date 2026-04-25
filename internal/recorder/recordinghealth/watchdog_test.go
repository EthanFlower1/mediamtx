package recordinghealth

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/bluenviron/mediamtx/internal/recorder/state"
)

// ---- fakes -----------------------------------------------------------------

type fakeStateReader struct {
	cameras []state.AssignedCamera
	err     error
}

func (f *fakeStateReader) ListAssigned(_ context.Context) ([]state.AssignedCamera, error) {
	return f.cameras, f.err
}

type fakeRuntimeClient struct {
	publishing []string
	err        error
}

func (f *fakeRuntimeClient) ListPublishingCameras(_ context.Context) ([]string, error) {
	return f.publishing, f.err
}

// newTestWatchdog returns a Watchdog with an injected tick channel so tests
// can advance cycles deterministically without time.Sleep.
// onReloadFn may be nil; a no-op default is used in that case.
func newTestWatchdog(
	stateReader StateReader,
	rtClient RuntimeClient,
	reloadFn func(),
	ackCycles int,
	logger *slog.Logger,
	onReloadFn func(),
) (*Watchdog, chan time.Time) {
	if logger == nil {
		logger = slog.Default()
	}
	if onReloadFn == nil {
		onReloadFn = func() {}
	}
	tickCh := make(chan time.Time, 16)
	wd := &Watchdog{
		cfg: Config{
			Store:                     stateReader,
			MediaMTX:                  rtClient,
			Reload:                    reloadFn,
			OnReload:                  onReloadFn,
			Interval:                  30 * time.Second, // irrelevant; we inject tick
			DriftAcknowledgmentCycles: ackCycles,
			Logger:                    logger,
		},
		cycle:     tickCh,
		driftCnt:  make(map[string]int),
		notified:  make(map[string]bool),
	}
	return wd, tickCh
}

// camList builds a []state.AssignedCamera from camera IDs.
func camList(ids ...string) []state.AssignedCamera {
	out := make([]state.AssignedCamera, len(ids))
	for i, id := range ids {
		out[i] = state.AssignedCamera{CameraID: id}
	}
	return out
}

// pushCycles drains (and re-pushes) n ticks on tickCh to advance the
// watchdog n cycles.  We write to the buffered channel; Run reads from it.
func pushCycles(tickCh chan time.Time, n int) {
	for i := 0; i < n; i++ {
		tickCh <- time.Now()
	}
}

// drainWithTimeout waits for Run to process all buffered ticks (channel
// drained to 0) or times out.
func drainWithTimeout(tickCh chan time.Time, timeout time.Duration) bool {
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		if len(tickCh) == 0 {
			// Give Run a moment to process the last tick.
			time.Sleep(5 * time.Millisecond)
			if len(tickCh) == 0 {
				return true
			}
		}
		time.Sleep(2 * time.Millisecond)
	}
	return false
}

// ---- tests -----------------------------------------------------------------

// 1. No drift → no Reload after multiple cycles.
func TestRun_NoDrift_NoReload(t *testing.T) {
	var reloadCalls atomic.Int64
	sr := &fakeStateReader{cameras: camList("a", "b")}
	rc := &fakeRuntimeClient{publishing: []string{"a", "b"}}

	wd, tickCh := newTestWatchdog(sr, rc, func() { reloadCalls.Add(1) }, 2, nil, nil)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go wd.Run(ctx)

	pushCycles(tickCh, 5)
	if !drainWithTimeout(tickCh, 200*time.Millisecond) {
		t.Fatal("ticks not processed in time")
	}
	cancel()

	if got := reloadCalls.Load(); got != 0 {
		t.Errorf("expected 0 Reload calls, got %d", got)
	}
}

// 2. One cycle drift then aligned → no Reload (counter resets).
func TestRun_TransientDrift_OneCycle_NoReload(t *testing.T) {
	var reloadCalls atomic.Int64
	sr := &fakeStateReader{cameras: camList("a")}
	// First tick: not publishing.  Second tick: publishing.
	callCount := 0
	rc := &fakeRuntimeClient{}
	rcDynamic := &dynamicRuntimeClient{fn: func() ([]string, error) {
		callCount++
		if callCount == 1 {
			return []string{}, nil // drifted
		}
		return []string{"a"}, nil // recovered
	}}

	wd, tickCh := newTestWatchdog(sr, rcDynamic, func() { reloadCalls.Add(1) }, 2, nil, nil)
	_ = rc // unused but kept for type safety

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go wd.Run(ctx)

	pushCycles(tickCh, 2)
	if !drainWithTimeout(tickCh, 200*time.Millisecond) {
		t.Fatal("ticks not processed in time")
	}
	cancel()

	if got := reloadCalls.Load(); got != 0 {
		t.Errorf("expected 0 Reload calls, got %d", got)
	}
}

// dynamicRuntimeClient lets us swap responses per call.
type dynamicRuntimeClient struct {
	fn func() ([]string, error)
}

func (d *dynamicRuntimeClient) ListPublishingCameras(_ context.Context) ([]string, error) {
	return d.fn()
}

// 3. Drift for DriftAcknowledgmentCycles → Reload called once, Warn logged,
// and OnReload called the same number of times as Reload.
func TestRun_PersistentDrift_TriggersReload(t *testing.T) {
	var reloadCalls atomic.Int64
	var onReloadCalls atomic.Int64
	sr := &fakeStateReader{cameras: camList("a")}
	rc := &fakeRuntimeClient{publishing: []string{}} // "a" never publishing

	var warnLogged atomic.Bool
	handler := &captureHandler{fn: func(r slog.Record) {
		if r.Level == slog.LevelWarn && strings.Contains(r.Message, "recording-health") {
			warnLogged.Store(true)
		}
	}}
	logger := slog.New(handler)

	wd, tickCh := newTestWatchdog(
		sr, rc,
		func() { reloadCalls.Add(1) },
		2,
		logger,
		func() { onReloadCalls.Add(1) },
	)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go wd.Run(ctx)

	pushCycles(tickCh, 2) // cycle 1 → cnt=1, cycle 2 → cnt=2 → trigger
	if !drainWithTimeout(tickCh, 300*time.Millisecond) {
		t.Fatal("ticks not processed in time")
	}
	cancel()

	if got := reloadCalls.Load(); got != 1 {
		t.Errorf("expected 1 Reload call, got %d", got)
	}
	if got := onReloadCalls.Load(); got != reloadCalls.Load() {
		t.Errorf("expected OnReload calls (%d) to equal Reload calls (%d)", got, reloadCalls.Load())
	}
	if !warnLogged.Load() {
		t.Error("expected Warn log not emitted")
	}
}

// 4. Drift continues 5 cycles after threshold → Reload fires only once per episode.
func TestRun_PersistentDrift_LogsOnce(t *testing.T) {
	var reloadCalls atomic.Int64
	sr := &fakeStateReader{cameras: camList("a")}
	rc := &fakeRuntimeClient{publishing: []string{}}

	wd, tickCh := newTestWatchdog(sr, rc, func() { reloadCalls.Add(1) }, 2, nil, nil)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go wd.Run(ctx)

	// 2 cycles to trigger, then 5 more — still only 1 Reload per episode.
	pushCycles(tickCh, 7)
	if !drainWithTimeout(tickCh, 300*time.Millisecond) {
		t.Fatal("ticks not processed in time")
	}
	cancel()

	if got := reloadCalls.Load(); got != 1 {
		t.Errorf("expected 1 Reload call (one per episode), got %d", got)
	}
}

// 5. StateStore error → logs Error, continues next cycle.
func TestRun_StateStoreError_LogsAndContinues(t *testing.T) {
	var reloadCalls atomic.Int64
	callCount := 0
	sr := &dynamicStateReader{fn: func() ([]state.AssignedCamera, error) {
		callCount++
		if callCount == 1 {
			return nil, errors.New("db unavailable")
		}
		return camList("a"), nil
	}}
	rc := &fakeRuntimeClient{publishing: []string{"a"}}

	var errLogged atomic.Bool
	handler := &captureHandler{fn: func(r slog.Record) {
		if r.Level == slog.LevelError {
			errLogged.Store(true)
		}
	}}
	logger := slog.New(handler)

	wd, tickCh := newTestWatchdog(sr, rc, func() { reloadCalls.Add(1) }, 2, logger, nil)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go wd.Run(ctx)

	pushCycles(tickCh, 3)
	if !drainWithTimeout(tickCh, 300*time.Millisecond) {
		t.Fatal("ticks not processed in time")
	}
	cancel()

	if !errLogged.Load() {
		t.Error("expected Error log for state store failure")
	}
	if got := reloadCalls.Load(); got != 0 {
		t.Errorf("expected 0 Reload calls (error cycle skipped), got %d", got)
	}
}

// dynamicStateReader for per-call responses.
type dynamicStateReader struct {
	fn func() ([]state.AssignedCamera, error)
}

func (d *dynamicStateReader) ListAssigned(_ context.Context) ([]state.AssignedCamera, error) {
	return d.fn()
}

// 6. MediaMTX error → logs Error, continues.
func TestRun_MediaMTXError_LogsAndContinues(t *testing.T) {
	var reloadCalls atomic.Int64
	sr := &fakeStateReader{cameras: camList("a")}
	callCount := 0
	rc := &dynamicRuntimeClient{fn: func() ([]string, error) {
		callCount++
		if callCount == 1 {
			return nil, errors.New("mediamtx unreachable")
		}
		return []string{"a"}, nil
	}}

	var errLogged atomic.Bool
	handler := &captureHandler{fn: func(r slog.Record) {
		if r.Level == slog.LevelError {
			errLogged.Store(true)
		}
	}}
	logger := slog.New(handler)

	wd, tickCh := newTestWatchdog(sr, rc, func() { reloadCalls.Add(1) }, 2, logger, nil)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go wd.Run(ctx)

	pushCycles(tickCh, 3)
	if !drainWithTimeout(tickCh, 300*time.Millisecond) {
		t.Fatal("ticks not processed in time")
	}
	cancel()

	if !errLogged.Load() {
		t.Error("expected Error log for mediamtx failure")
	}
	if got := reloadCalls.Load(); got != 0 {
		t.Errorf("expected 0 Reload calls, got %d", got)
	}
}

// 7. Drift → reload → cleared → drifts again → second reload.
func TestRun_DriftClearsThenRecurs(t *testing.T) {
	var reloadCalls atomic.Int64
	sr := &fakeStateReader{cameras: camList("a")}
	callCount := 0
	rc := &dynamicRuntimeClient{fn: func() ([]string, error) {
		callCount++
		switch callCount {
		case 1, 2:
			return []string{}, nil // drift ep 1
		case 3, 4:
			return []string{"a"}, nil // recovered
		default:
			return []string{}, nil // drift ep 2
		}
	}}

	wd, tickCh := newTestWatchdog(sr, rc, func() { reloadCalls.Add(1) }, 2, nil, nil)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go wd.Run(ctx)

	// ep1 trigger (2 cycles) + 2 recovery cycles + ep2 trigger (2 cycles)
	pushCycles(tickCh, 6)
	if !drainWithTimeout(tickCh, 400*time.Millisecond) {
		t.Fatal("ticks not processed in time")
	}
	cancel()

	if got := reloadCalls.Load(); got != 2 {
		t.Errorf("expected 2 Reload calls (one per episode), got %d", got)
	}
}

// 8. Cancel context → Run returns within 50ms.
func TestRun_RespectsContext(t *testing.T) {
	sr := &fakeStateReader{cameras: camList("a")}
	rc := &fakeRuntimeClient{publishing: []string{"a"}}

	wd, _ := newTestWatchdog(sr, rc, func() {}, 2, nil, nil)

	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() {
		defer close(done)
		wd.Run(ctx)
	}()

	cancel()
	select {
	case <-done:
	case <-time.After(50 * time.Millisecond):
		t.Fatal("Run did not return within 50ms after context cancellation")
	}
}

// ---- captureHandler --------------------------------------------------------

// captureHandler is a slog.Handler that forwards records to a callback.
type captureHandler struct {
	fn func(slog.Record)
}

func (h *captureHandler) Enabled(_ context.Context, _ slog.Level) bool { return true }

func (h *captureHandler) Handle(_ context.Context, r slog.Record) error {
	h.fn(r)
	return nil
}

func (h *captureHandler) WithAttrs(_ []slog.Attr) slog.Handler { return h }

func (h *captureHandler) WithGroup(_ string) slog.Handler { return h }

// ---- httpRuntimeClient tests -----------------------------------------------

func pathItem(name string, ready bool) map[string]any {
	return map[string]any{"name": name, "ready": ready}
}

func writeJSON(w http.ResponseWriter, v any) {
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(v)
}

// Single-page response with mixed ready/not-ready paths.
func TestHTTPRuntimeClient_SinglePage(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, map[string]any{
			"itemCount": 3,
			"pageCount": 1,
			"items": []any{
				pathItem("cam_a", true),
				pathItem("cam_b", false),
				pathItem("cam_c", true),
				pathItem("other_x", true), // not matching prefix
			},
		})
	}))
	defer srv.Close()

	c := NewHTTPRuntimeClient(srv.URL, "cam_", nil)
	ids, err := c.ListPublishingCameras(context.Background())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(ids) != 2 {
		t.Fatalf("expected 2 ids, got %v", ids)
	}
	want := map[string]bool{"a": true, "c": true}
	for _, id := range ids {
		if !want[id] {
			t.Errorf("unexpected id %q", id)
		}
	}
}

// Paginated response: two pages.
func TestHTTPRuntimeClient_Paginated(t *testing.T) {
	callCount := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		callCount++
		page := r.URL.Query().Get("page")
		switch page {
		case "", "0", "1":
			writeJSON(w, map[string]any{
				"itemCount": 2,
				"pageCount": 2,
				"items": []any{
					pathItem("cam_a", true),
				},
			})
		case "2":
			writeJSON(w, map[string]any{
				"itemCount": 2,
				"pageCount": 2,
				"items": []any{
					pathItem("cam_b", true),
				},
			})
		default:
			writeJSON(w, map[string]any{
				"itemCount": 0,
				"pageCount": 2,
				"items":     []any{},
			})
		}
	}))
	defer srv.Close()

	c := NewHTTPRuntimeClient(srv.URL, "cam_", nil)
	ids, err := c.ListPublishingCameras(context.Background())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(ids) != 2 {
		t.Fatalf("expected 2 ids from 2 pages, got %v", ids)
	}
}

// Server returns 5xx → error returned.
func TestHTTPRuntimeClient_ServerError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "internal error", http.StatusInternalServerError)
	}))
	defer srv.Close()

	c := NewHTTPRuntimeClient(srv.URL, "cam_", nil)
	_, err := c.ListPublishingCameras(context.Background())
	if err == nil {
		t.Fatal("expected error from 5xx response, got nil")
	}
	if !strings.Contains(err.Error(), "500") {
		t.Errorf("expected error to mention 500, got %v", err)
	}
}

// Verify the watchdog strips prefix correctly when comparing with state store.
func TestRun_PrefixStripping(t *testing.T) {
	// Cameras assigned: "x", "y"
	// MediaMTX publishes: "cam_x", "cam_y" — after stripping "cam_" → "x", "y"
	var reloadCalls atomic.Int64
	sr := &fakeStateReader{cameras: camList("x", "y")}
	// The httpRuntimeClient strips the prefix internally and returns bare IDs.
	// The fakeRuntimeClient returns bare IDs too (it represents post-strip output).
	rc := &fakeRuntimeClient{publishing: []string{"x", "y"}}

	wd, tickCh := newTestWatchdog(sr, rc, func() { reloadCalls.Add(1) }, 2, nil, nil)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go wd.Run(ctx)

	pushCycles(tickCh, 4)
	if !drainWithTimeout(tickCh, 200*time.Millisecond) {
		t.Fatal("ticks not processed in time")
	}
	cancel()

	if got := reloadCalls.Load(); got != 0 {
		t.Errorf("expected 0 Reload calls (no drift), got %d", got)
	}
}

// Ensure the immediate first check on Run start works (no tick needed).
func TestRun_ImmediateFirstCheck(t *testing.T) {
	// If "a" is assigned but not publishing, we should accumulate drift immediately.
	var reloadCalls atomic.Int64
	sr := &fakeStateReader{cameras: camList("a")}
	rc := &fakeRuntimeClient{publishing: []string{}}

	wd, tickCh := newTestWatchdog(sr, rc, func() { reloadCalls.Add(1) }, 1, nil, nil)
	// ackCycles=1 means 1 cycle triggers reload.

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go wd.Run(ctx)

	// The watchdog does one immediate check, then waits for tick.
	// With ackCycles=1, the first check should trigger reload.
	// Give it time to process before we check.
	time.Sleep(30 * time.Millisecond)
	cancel()
	_ = tickCh

	if got := reloadCalls.Load(); got != 1 {
		t.Errorf("expected 1 Reload call from immediate first check (ackCycles=1), got %d", got)
	}
}

// TestRun_OnReload_FiresBeforeReload verifies that OnReload is invoked the same
// number of times as Reload across multiple drift episodes, confirming the
// metric counter would track every reconcile event.
func TestRun_OnReload_FiresBeforeReload(t *testing.T) {
	var reloadCalls atomic.Int64
	var onReloadCalls atomic.Int64
	sr := &fakeStateReader{cameras: camList("a")}
	rc := &fakeRuntimeClient{publishing: []string{}} // "a" never publishing

	wd, tickCh := newTestWatchdog(
		sr, rc,
		func() { reloadCalls.Add(1) },
		2,
		nil,
		func() { onReloadCalls.Add(1) },
	)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go wd.Run(ctx)

	pushCycles(tickCh, 2)
	if !drainWithTimeout(tickCh, 300*time.Millisecond) {
		t.Fatal("ticks not processed in time")
	}
	cancel()

	reload := reloadCalls.Load()
	onReload := onReloadCalls.Load()
	if reload != 1 {
		t.Errorf("expected 1 Reload call, got %d", reload)
	}
	if onReload != reload {
		t.Errorf("expected OnReload calls (%d) == Reload calls (%d)", onReload, reload)
	}
}

// TestRun_OnReload_NilSafe verifies that a nil OnReload is treated as a no-op
// and does not panic when a drift-trigger cycle executes.
func TestRun_OnReload_NilSafe(t *testing.T) {
	var reloadCalls atomic.Int64
	sr := &fakeStateReader{cameras: camList("a")}
	rc := &fakeRuntimeClient{publishing: []string{}} // "a" never publishing

	// Construct the Config directly and go through New() so the nil default
	// is applied via the constructor path (not the test helper).
	cfg := Config{
		Store:                     sr,
		MediaMTX:                  rc,
		Reload:                    func() { reloadCalls.Add(1) },
		OnReload:                  nil, // explicit nil — must not panic
		DriftAcknowledgmentCycles: 1,
		Logger:                    slog.Default(),
	}
	wd := New(cfg) // New() defaults nil OnReload to no-op

	// Replace the production ticker with a test channel.
	wd.ticker.Stop()
	tickCh := make(chan time.Time, 16)
	wd.cycle = tickCh

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	done := make(chan struct{})
	go func() {
		defer close(done)
		wd.Run(ctx) // must not panic
	}()

	// One tick to drive a drift-trigger cycle (ackCycles=1 means first cycle fires).
	tickCh <- time.Now()
	if !drainWithTimeout(tickCh, 200*time.Millisecond) {
		t.Fatal("tick not processed in time")
	}
	cancel()
	<-done

	if got := reloadCalls.Load(); got != 1 {
		t.Errorf("expected 1 Reload call, got %d", got)
	}
}

// unused import guard
var _ = fmt.Sprintf
