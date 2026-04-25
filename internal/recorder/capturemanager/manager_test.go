package capturemanager_test

import (
	"io"
	"log/slog"
	"sort"
	"sync"
	"testing"

	"github.com/bluenviron/mediamtx/internal/recorder/capturemanager"
	"github.com/bluenviron/mediamtx/internal/recorder/recordercontrol"
)

// reloadCounter is a thread-safe reload counter for use in tests.
func newReloadCounter() (func(), func() int) {
	var mu sync.Mutex
	var count int
	reload := func() {
		mu.Lock()
		count++
		mu.Unlock()
	}
	get := func() int {
		mu.Lock()
		defer mu.Unlock()
		return count
	}
	return reload, get
}

func discardLogger() *slog.Logger {
	return slog.New(slog.NewTextHandler(io.Discard, nil))
}

func camera(id string, version int64) recordercontrol.Camera {
	return recordercontrol.Camera{ID: id, ConfigVersion: version}
}

// TestNew_PanicsOnNilReload verifies that New panics when Reload is nil.
func TestNew_PanicsOnNilReload(t *testing.T) {
	defer func() {
		r := recover()
		if r == nil {
			t.Fatal("expected panic when Reload is nil, got none")
		}
	}()
	capturemanager.New(capturemanager.Config{})
}

// TestRunningCameras_EmptyByDefault verifies that a fresh manager has no cameras.
func TestRunningCameras_EmptyByDefault(t *testing.T) {
	reload, _ := newReloadCounter()
	m := capturemanager.New(capturemanager.Config{Reload: reload, Logger: discardLogger()})
	got := m.RunningCameras()
	if len(got) != 0 {
		t.Fatalf("expected empty RunningCameras, got %v", got)
	}
}

// TestEnsureRunning_TracksAndReloads verifies the happy path: one call
// results in the camera being tracked and Reload called once.
func TestEnsureRunning_TracksAndReloads(t *testing.T) {
	reload, reloads := newReloadCounter()
	m := capturemanager.New(capturemanager.Config{Reload: reload, Logger: discardLogger()})

	if err := m.EnsureRunning(camera("cam1", 1)); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if reloads() != 1 {
		t.Fatalf("expected 1 reload, got %d", reloads())
	}
	running := m.RunningCameras()
	if len(running) != 1 || running[0] != "cam1" {
		t.Fatalf("expected [cam1], got %v", running)
	}
}

// TestEnsureRunning_Idempotent verifies that calling EnsureRunning twice with
// the same ConfigVersion does not trigger a second reload.
func TestEnsureRunning_Idempotent(t *testing.T) {
	reload, reloads := newReloadCounter()
	m := capturemanager.New(capturemanager.Config{Reload: reload, Logger: discardLogger()})

	_ = m.EnsureRunning(camera("cam1", 42))
	_ = m.EnsureRunning(camera("cam1", 42))

	if reloads() != 1 {
		t.Fatalf("expected 1 reload after two identical calls, got %d", reloads())
	}
	running := m.RunningCameras()
	if len(running) != 1 {
		t.Fatalf("expected 1 running camera, got %v", running)
	}
}

// TestEnsureRunning_VersionChange_Reloads verifies that a version bump triggers
// a second reload.
func TestEnsureRunning_VersionChange_Reloads(t *testing.T) {
	reload, reloads := newReloadCounter()
	m := capturemanager.New(capturemanager.Config{Reload: reload, Logger: discardLogger()})

	_ = m.EnsureRunning(camera("cam1", 1))
	_ = m.EnsureRunning(camera("cam1", 2))

	if reloads() != 2 {
		t.Fatalf("expected 2 reloads after version change, got %d", reloads())
	}
}

// TestStop_OnUnknownCamera_NoReload verifies that Stop on a never-seen ID is a
// no-op.
func TestStop_OnUnknownCamera_NoReload(t *testing.T) {
	reload, reloads := newReloadCounter()
	m := capturemanager.New(capturemanager.Config{Reload: reload, Logger: discardLogger()})

	if err := m.Stop("ghost"); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if reloads() != 0 {
		t.Fatalf("expected 0 reloads for unknown camera, got %d", reloads())
	}
}

// TestStop_RemovesAndReloads verifies that EnsureRunning then Stop yields
// 2 total reloads and leaves RunningCameras empty.
func TestStop_RemovesAndReloads(t *testing.T) {
	reload, reloads := newReloadCounter()
	m := capturemanager.New(capturemanager.Config{Reload: reload, Logger: discardLogger()})

	_ = m.EnsureRunning(camera("cam1", 1))
	_ = m.Stop("cam1")

	if reloads() != 2 {
		t.Fatalf("expected 2 reloads (ensure+stop), got %d", reloads())
	}
	running := m.RunningCameras()
	if len(running) != 0 {
		t.Fatalf("expected empty RunningCameras after Stop, got %v", running)
	}
}

// TestRunningCameras_Sorted verifies that RunningCameras returns IDs in sorted
// order regardless of EnsureRunning call order.
func TestRunningCameras_Sorted(t *testing.T) {
	reload, _ := newReloadCounter()
	m := capturemanager.New(capturemanager.Config{Reload: reload, Logger: discardLogger()})

	ids := []string{"cam3", "cam1", "cam5", "cam2", "cam4"}
	for _, id := range ids {
		_ = m.EnsureRunning(camera(id, 1))
	}

	got := m.RunningCameras()
	if len(got) != len(ids) {
		t.Fatalf("expected %d cameras, got %d", len(ids), len(got))
	}
	if !sort.StringsAreSorted(got) {
		t.Fatalf("RunningCameras not sorted: %v", got)
	}
}

// TestConcurrent_SafeUnderRace fires 100 concurrent EnsureRunning calls with
// distinct IDs. Run with -race to verify no data races.
func TestConcurrent_SafeUnderRace(t *testing.T) {
	reload, _ := newReloadCounter()
	m := capturemanager.New(capturemanager.Config{Reload: reload, Logger: discardLogger()})

	const n = 100
	var wg sync.WaitGroup
	wg.Add(n)
	for i := range n {
		go func(i int) {
			defer wg.Done()
			id := "cam" + string(rune('A'+i%26)) + string(rune('0'+i/26))
			_ = m.EnsureRunning(camera(id, 1))
		}(i)
	}
	wg.Wait()

	running := m.RunningCameras()
	if len(running) != n {
		t.Fatalf("expected %d running cameras, got %d", n, len(running))
	}
}
