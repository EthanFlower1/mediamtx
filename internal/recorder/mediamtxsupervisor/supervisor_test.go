package mediamtxsupervisor

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/bluenviron/mediamtx/internal/recorder/state"
)

// ---- fakes -------------------------------------------------------------

type fakeSource struct {
	mu   sync.Mutex
	cams []state.AssignedCamera
	err  error
}

func (f *fakeSource) ListAssigned(ctx context.Context) ([]state.AssignedCamera, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.err != nil {
		return nil, f.err
	}
	out := make([]state.AssignedCamera, len(f.cams))
	copy(out, f.cams)
	return out, nil
}

func (f *fakeSource) set(cams []state.AssignedCamera) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.cams = cams
}

func (f *fakeSource) setErr(err error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.err = err
}

type fakeController struct {
	mu      sync.Mutex
	calls   int
	last    PathConfigSet
	failNxt error
}

func (c *fakeController) ApplyPaths(ctx context.Context, set PathConfigSet) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.calls++
	if c.failNxt != nil {
		err := c.failNxt
		c.failNxt = nil
		return err
	}
	c.last = set
	return nil
}

func (c *fakeController) Healthy(ctx context.Context) error { return nil }

func (c *fakeController) snapshot() (int, PathConfigSet) {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.calls, c.last
}

// ---- helpers -----------------------------------------------------------

func mkCam(id, rtsp string) state.AssignedCamera {
	return state.AssignedCamera{
		CameraID: id,
		Config: state.CameraConfig{
			ID:           id,
			Name:         id,
			RTSPURL:      rtsp,
			RTSPUsername: "user",
		},
		RTSPPassword: "pw",
	}
}

func waitFor(t *testing.T, cond func() bool) {
	t.Helper()
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		if cond() {
			return
		}
		time.Sleep(5 * time.Millisecond)
	}
	t.Fatalf("condition not met within deadline")
}

// ---- RenderPaths --------------------------------------------------------

func TestRenderPathsBasic(t *testing.T) {
	cams := []state.AssignedCamera{
		mkCam("cam-b", "rtsp://10.0.0.2/stream"),
		mkCam("cam-a", "rtsp://10.0.0.1/stream"),
	}
	set, err := RenderPaths(cams, RenderOptions{})
	if err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	if got := len(set.Paths); got != 2 {
		t.Fatalf("paths=%d want 2", got)
	}
	if set.Paths[0].Name != "cam_cam-a" || set.Paths[1].Name != "cam_cam-b" {
		t.Fatalf("paths not sorted: %+v", set.Names())
	}
	for _, p := range set.Paths {
		if !p.SourceOnDemand {
			t.Errorf("%s: sourceOnDemand should be true", p.Name)
		}
		if !p.Record {
			t.Errorf("%s: record should be true", p.Name)
		}
		if p.RecordFormat != "fmp4" {
			t.Errorf("%s: format=%q want fmp4", p.Name, p.RecordFormat)
		}
		if !strings.HasPrefix(p.Source, "rtsp://user:pw@") {
			t.Errorf("%s: source=%q missing creds", p.Name, p.Source)
		}
	}
}

func TestRenderPathsSkipsBadRows(t *testing.T) {
	cams := []state.AssignedCamera{
		mkCam("good", "rtsp://10.0.0.1/s"),
		{CameraID: "no-url", Config: state.CameraConfig{ID: "no-url"}},
		{Config: state.CameraConfig{}}, // empty id
	}
	set, err := RenderPaths(cams, RenderOptions{})
	if err == nil {
		t.Fatalf("expected RenderError")
	}
	var re *RenderError
	if !errors.As(err, &re) {
		t.Fatalf("wrong err type: %T", err)
	}
	if len(re.Skipped) != 2 {
		t.Fatalf("skipped=%v want 2", re.Skipped)
	}
	if len(set.Paths) != 1 || set.Paths[0].Name != "cam_good" {
		t.Fatalf("expected only cam_good in set: %+v", set.Names())
	}
}

func TestRenderPathsCustomOptions(t *testing.T) {
	set, err := RenderPaths(
		[]state.AssignedCamera{mkCam("x", "rtsp://10.0.0.1/s")},
		RenderOptions{
			PathPrefix:        "rec_",
			RecordFormat:      "mpegts",
			SegmentDuration:   "30m",
			RecordDeleteAfter: "168h",
		},
	)
	if err != nil {
		t.Fatalf("err=%v", err)
	}
	p := set.Paths[0]
	if p.Name != "rec_x" {
		t.Errorf("name=%q", p.Name)
	}
	if p.RecordFormat != "mpegts" {
		t.Errorf("format=%q", p.RecordFormat)
	}
	if p.RecordSegmentDuration != "30m" {
		t.Errorf("segdur=%q", p.RecordSegmentDuration)
	}
	if p.RecordDeleteAfter != "168h" {
		t.Errorf("delete=%q", p.RecordDeleteAfter)
	}
}

// ---- MediaMTXSupervisor -------------------------------------------------

func TestSupervisorInitialReload(t *testing.T) {
	src := &fakeSource{cams: []state.AssignedCamera{mkCam("cam1", "rtsp://10.0.0.1/s")}}
	ctrl := &fakeController{}
	sup, err := New(Config{Source: src, Controller: ctrl, PollInterval: -1})
	if err != nil {
		t.Fatalf("new: %v", err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	if err := sup.Start(ctx); err != nil {
		t.Fatalf("start: %v", err)
	}
	defer sup.Close()

	calls, set := ctrl.snapshot()
	if calls != 1 {
		t.Fatalf("calls=%d want 1", calls)
	}
	if len(set.Paths) != 1 || set.Paths[0].Name != "cam_cam1" {
		t.Fatalf("unexpected set: %+v", set.Names())
	}
	if got := sup.Stats().PathsApplied; got != 1 {
		t.Errorf("Stats.PathsApplied=%d want 1", got)
	}
}

func TestSupervisorReloadCoalescesAndApplies(t *testing.T) {
	src := &fakeSource{cams: []state.AssignedCamera{mkCam("a", "rtsp://10.0.0.1/s")}}
	ctrl := &fakeController{}
	sup, err := New(Config{Source: src, Controller: ctrl, PollInterval: time.Hour})
	if err != nil {
		t.Fatalf("new: %v", err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	if err := sup.Start(ctx); err != nil {
		t.Fatalf("start: %v", err)
	}
	defer sup.Close()

	// initial apply already happened
	src.set([]state.AssignedCamera{
		mkCam("a", "rtsp://10.0.0.1/s"),
		mkCam("b", "rtsp://10.0.0.2/s"),
	})
	for i := 0; i < 5; i++ {
		sup.Reload()
	}

	waitFor(t, func() bool {
		_, set := ctrl.snapshot()
		return len(set.Paths) == 2
	})

	calls, set := ctrl.snapshot()
	if len(set.Paths) != 2 {
		t.Fatalf("paths=%d want 2", len(set.Paths))
	}
	if calls > 3 {
		t.Errorf("calls=%d, expected coalescing to keep it small", calls)
	}
	if got := sup.Stats().ReloadCount; got < 2 {
		t.Errorf("ReloadCount=%d want >= 2", got)
	}
}

func TestSupervisorSkipsWhenUnchanged(t *testing.T) {
	src := &fakeSource{cams: []state.AssignedCamera{mkCam("a", "rtsp://10.0.0.1/s")}}
	ctrl := &fakeController{}
	sup, _ := New(Config{Source: src, Controller: ctrl, PollInterval: time.Hour})
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	_ = sup.Start(ctx)
	defer sup.Close()

	for i := 0; i < 3; i++ {
		sup.Reload()
	}
	// Give the loop a moment to drain.
	time.Sleep(50 * time.Millisecond)

	calls, _ := ctrl.snapshot()
	if calls != 1 {
		t.Errorf("calls=%d want 1 (initial only; rest should be no-ops)", calls)
	}
	if got := sup.Stats().SkipCount; got < 1 {
		t.Errorf("SkipCount=%d want >= 1", got)
	}
}

func TestSupervisorFailOpenOnSourceError(t *testing.T) {
	src := &fakeSource{cams: []state.AssignedCamera{mkCam("a", "rtsp://10.0.0.1/s")}}
	ctrl := &fakeController{}
	sup, _ := New(Config{Source: src, Controller: ctrl, PollInterval: time.Hour})
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	_ = sup.Start(ctx)
	defer sup.Close()

	// Initial apply succeeded; now break the source.
	src.setErr(errors.New("directory unreachable"))
	sup.Reload()
	time.Sleep(50 * time.Millisecond)

	if got := sup.Stats().PathsApplied; got != 1 {
		t.Errorf("PathsApplied=%d, expected supervisor to keep last set", got)
	}
	if sup.Stats().LastError == nil {
		t.Errorf("LastError should be set after source failure")
	}

	// Recover.
	src.setErr(nil)
	src.set([]state.AssignedCamera{
		mkCam("a", "rtsp://10.0.0.1/s"),
		mkCam("c", "rtsp://10.0.0.3/s"),
	})
	sup.Reload()
	waitFor(t, func() bool { return sup.Stats().PathsApplied == 2 })
	if sup.Stats().LastError != nil {
		t.Errorf("LastError should clear after success: %v", sup.Stats().LastError)
	}
}

func TestSupervisorControllerErrorRecorded(t *testing.T) {
	src := &fakeSource{cams: []state.AssignedCamera{mkCam("a", "rtsp://10.0.0.1/s")}}
	ctrl := &fakeController{failNxt: errors.New("boom")}
	sup, _ := New(Config{Source: src, Controller: ctrl, PollInterval: time.Hour})
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	// Initial apply will fail; Start logs and returns the error.
	_ = sup.Start(ctx)
	defer sup.Close()

	if sup.Stats().LastError == nil {
		t.Fatalf("LastError should be set after controller failure")
	}
	if sup.Stats().PathsApplied != 0 {
		t.Errorf("PathsApplied=%d want 0", sup.Stats().PathsApplied)
	}

	// Next reload should succeed and clear the error.
	sup.Reload()
	waitFor(t, func() bool { return sup.Stats().PathsApplied == 1 })
}

func TestSupervisorPollPicksUpChanges(t *testing.T) {
	src := &fakeSource{cams: []state.AssignedCamera{mkCam("a", "rtsp://10.0.0.1/s")}}
	ctrl := &fakeController{}
	sup, _ := New(Config{Source: src, Controller: ctrl, PollInterval: 20 * time.Millisecond})
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	_ = sup.Start(ctx)
	defer sup.Close()

	// Mutate the source without calling Reload().
	src.set([]state.AssignedCamera{
		mkCam("a", "rtsp://10.0.0.1/s"),
		mkCam("z", "rtsp://10.0.0.9/s"),
	})

	waitFor(t, func() bool {
		_, set := ctrl.snapshot()
		return len(set.Paths) == 2
	})
}

func TestSupervisorRequiresSourceAndController(t *testing.T) {
	if _, err := New(Config{}); err == nil {
		t.Fatalf("expected error for missing source")
	}
	if _, err := New(Config{Source: &fakeSource{}}); err == nil {
		t.Fatalf("expected error for missing controller")
	}
}

func TestSupervisorCloseIdempotent(t *testing.T) {
	src := &fakeSource{}
	ctrl := &fakeController{}
	sup, _ := New(Config{Source: src, Controller: ctrl, PollInterval: time.Hour})
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	_ = sup.Start(ctx)
	sup.Close()
	sup.Close() // must not panic
}

// ---- HTTPController against httptest -----------------------------------

func TestHTTPControllerApplyPathsAddPatchDelete(t *testing.T) {
	var (
		mu      sync.Mutex
		gets    int32
		adds    []string
		patches []string
		deletes []string
	)

	// Server-side simulated state: starts with cam_old + live, then tracks
	// adds/deletes so the /list endpoint reflects reality across multiple
	// ApplyPaths calls.
	state := map[string]struct{}{"cam_old": {}, "live": {}}

	mux := http.NewServeMux()
	mux.HandleFunc("/v3/config/paths/list", func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&gets, 1)
		mu.Lock()
		names := make([]string, 0, len(state))
		for n := range state {
			names = append(names, n)
		}
		mu.Unlock()
		w.Header().Set("Content-Type", "application/json")
		var sb strings.Builder
		sb.WriteString(`{"itemCount":`)
		fmt.Fprintf(&sb, "%d", len(names))
		sb.WriteString(`,"pageCount":1,"items":[`)
		for i, n := range names {
			if i > 0 {
				sb.WriteString(",")
			}
			fmt.Fprintf(&sb, `{"name":%q}`, n)
		}
		sb.WriteString(`]}`)
		_, _ = w.Write([]byte(sb.String()))
	})
	mux.HandleFunc("/v3/config/paths/add/", func(w http.ResponseWriter, r *http.Request) {
		name := strings.TrimPrefix(r.URL.Path, "/v3/config/paths/add/")
		mu.Lock()
		adds = append(adds, name)
		state[name] = struct{}{}
		mu.Unlock()
		w.WriteHeader(http.StatusOK)
	})
	mux.HandleFunc("/v3/config/paths/patch/", func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		patches = append(patches, strings.TrimPrefix(r.URL.Path, "/v3/config/paths/patch/"))
		mu.Unlock()
		w.WriteHeader(http.StatusOK)
	})
	mux.HandleFunc("/v3/config/paths/delete/", func(w http.ResponseWriter, r *http.Request) {
		name := strings.TrimPrefix(r.URL.Path, "/v3/config/paths/delete/")
		mu.Lock()
		deletes = append(deletes, name)
		delete(state, name)
		mu.Unlock()
		w.WriteHeader(http.StatusOK)
	})
	mux.HandleFunc("/v3/config/global/get", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{}`))
	})

	srv := httptest.NewServer(mux)
	defer srv.Close()

	ctrl := &HTTPController{BaseURL: srv.URL, PathPrefix: "cam_"}
	if err := ctrl.Healthy(context.Background()); err != nil {
		t.Fatalf("healthy: %v", err)
	}

	set := PathConfigSet{Paths: []PathConfig{
		{Name: "cam_new", Source: "rtsp://10.0.0.1/s", SourceOnDemand: true, Record: true},
		{Name: "cam_old", Source: "rtsp://10.0.0.2/s", SourceOnDemand: true, Record: true},
	}}
	if err := ctrl.ApplyPaths(context.Background(), set); err != nil {
		t.Fatalf("apply: %v", err)
	}

	// Assert first-apply observations, then release the mutex before the
	// next ApplyPaths call — the mux handlers grab mu themselves, so holding
	// it across a second HTTP round-trip would deadlock.
	func() {
		mu.Lock()
		defer mu.Unlock()
		if len(adds) != 1 || adds[0] != "cam_new" {
			t.Errorf("adds=%v want [cam_new]", adds)
		}
		if len(patches) != 1 || patches[0] != "cam_old" {
			t.Errorf("patches=%v want [cam_old]", patches)
		}
		if len(deletes) != 0 {
			t.Errorf("deletes=%v want none (cam_old kept, live not owned)", deletes)
		}
	}()

	// Now apply a smaller set: cam_new is dropped.
	if err := ctrl.ApplyPaths(context.Background(), PathConfigSet{Paths: []PathConfig{
		{Name: "cam_old", Source: "rtsp://10.0.0.2/s"},
	}}); err != nil {
		t.Fatalf("apply2: %v", err)
	}

	mu.Lock()
	defer mu.Unlock()
	if len(deletes) != 1 || deletes[0] != "cam_new" {
		t.Errorf("deletes after shrink=%v want [cam_new]", deletes)
	}
}
