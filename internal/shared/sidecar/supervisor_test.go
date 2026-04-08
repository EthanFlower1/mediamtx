package sidecar_test

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"os/exec"
	"runtime"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/bluenviron/mediamtx/internal/shared/sidecar"
)

// skipIfWindows bails out on Windows, where we rely on POSIX signals
// that are not meaningful for this package.
func skipIfWindows(t *testing.T) {
	t.Helper()
	if runtime.GOOS == "windows" {
		t.Skip("sidecar supervisor tests require POSIX signals")
	}
}

// testLogger returns a slog.Logger that writes into the provided
// buffer so tests can assert on captured lines.
func testLogger(buf io.Writer) *slog.Logger {
	return slog.New(slog.NewTextHandler(buf, &slog.HandlerOptions{Level: slog.LevelDebug}))
}

// --- fake Sidecar implementations -----------------------------------------

// sleepSidecar execs `sleep N` and reports healthy after a fixed delay.
type sleepSidecar struct {
	name         string
	sleepSeconds string
	readyAfter   time.Duration

	mu          sync.Mutex
	startedAt   time.Time
	starts      int32
	onReadyHits int32

	// forceUnhealthy, when set, makes HealthCheck start returning
	// an error at the configured time.
	forceUnhealthyAfter time.Duration
	unhealthy           atomic.Bool
}

func newSleepSidecar(name, secs string) *sleepSidecar {
	return &sleepSidecar{
		name:         name,
		sleepSeconds: secs,
		readyAfter:   100 * time.Millisecond,
	}
}

func (s *sleepSidecar) Name() string { return s.name }

func (s *sleepSidecar) Command(ctx context.Context) *exec.Cmd {
	atomic.AddInt32(&s.starts, 1)
	s.mu.Lock()
	s.startedAt = time.Now()
	s.unhealthy.Store(false)
	s.mu.Unlock()
	// exec.CommandContext so ctx cancellation kills the sleep.
	return exec.CommandContext(ctx, "sleep", s.sleepSeconds)
}

func (s *sleepSidecar) HealthCheck(ctx context.Context) error {
	s.mu.Lock()
	since := time.Since(s.startedAt)
	forceAfter := s.forceUnhealthyAfter
	s.mu.Unlock()

	if since < s.readyAfter {
		return errors.New("not yet")
	}
	if forceAfter > 0 && since > forceAfter {
		s.unhealthy.Store(true)
		return errors.New("forced unhealthy")
	}
	return nil
}

func (s *sleepSidecar) OnReady()         { atomic.AddInt32(&s.onReadyHits, 1) }
func (s *sleepSidecar) Env() []string    { return nil }
func (s *sleepSidecar) WorkDir() string  { return "" }
func (s *sleepSidecar) StartCount() int  { return int(atomic.LoadInt32(&s.starts)) }
func (s *sleepSidecar) ReadyHits() int   { return int(atomic.LoadInt32(&s.onReadyHits)) }

// crashySidecar uses `false` which exits immediately with status 1.
type crashySidecar struct {
	name   string
	starts int32
}

func (c *crashySidecar) Name() string { return c.name }
func (c *crashySidecar) Command(ctx context.Context) *exec.Cmd {
	atomic.AddInt32(&c.starts, 1)
	return exec.CommandContext(ctx, "false")
}
func (c *crashySidecar) HealthCheck(ctx context.Context) error { return nil }
func (c *crashySidecar) OnReady()                              {}
func (c *crashySidecar) Env() []string                         { return nil }
func (c *crashySidecar) WorkDir() string                       { return "" }
func (c *crashySidecar) StartCount() int                       { return int(atomic.LoadInt32(&c.starts)) }

// ---------------------------------------------------------------------------

func newTestSupervisor(t *testing.T, buf *bytes.Buffer) *sidecar.Supervisor {
	t.Helper()
	return sidecar.NewSupervisor(sidecar.Config{
		HealthInterval: 100 * time.Millisecond,
		HealthTimeout:  500 * time.Millisecond,
		BackoffBase:    20 * time.Millisecond,
		BackoffCap:     200 * time.Millisecond,
		GracePeriod:    500 * time.Millisecond,
		Logger:         testLogger(buf),
	})
}

func TestSupervisor_HappyPath(t *testing.T) {
	skipIfWindows(t)
	var buf bytes.Buffer
	sup := newTestSupervisor(t, &buf)
	defer sup.Shutdown()

	sc := newSleepSidecar("happy", "30")
	if err := sup.Start(context.Background(), sc); err != nil {
		t.Fatalf("Start: %v", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	if err := sup.WaitReady(ctx, "happy"); err != nil {
		t.Fatalf("WaitReady: %v", err)
	}

	if sc.ReadyHits() != 1 {
		t.Errorf("OnReady hits = %d, want 1", sc.ReadyHits())
	}

	stats := sup.Stats()
	if len(stats) != 1 {
		t.Fatalf("Stats len = %d, want 1", len(stats))
	}
	if !stats[0].Running || !stats[0].Ready {
		t.Errorf("stats not running/ready: %+v", stats[0])
	}
	if stats[0].Uptime <= 0 {
		t.Errorf("uptime should be positive: %v", stats[0].Uptime)
	}
}

func TestSupervisor_CrashRestart(t *testing.T) {
	skipIfWindows(t)
	var buf bytes.Buffer
	sup := newTestSupervisor(t, &buf)
	defer sup.Shutdown()

	sc := &crashySidecar{name: "crasher"}
	if err := sup.Start(context.Background(), sc); err != nil {
		t.Fatalf("Start: %v", err)
	}

	// Wait for at least 3 restart attempts.
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		if sc.StartCount() >= 3 {
			break
		}
		time.Sleep(30 * time.Millisecond)
	}
	if sc.StartCount() < 3 {
		t.Fatalf("expected >=3 starts after crash-restart, got %d", sc.StartCount())
	}

	stats := sup.Stats()
	if len(stats) == 0 || stats[0].CrashCount < 2 {
		t.Errorf("expected crash count >=2, got %+v", stats)
	}
}

func TestSupervisor_GracefulShutdown(t *testing.T) {
	skipIfWindows(t)
	var buf bytes.Buffer
	sup := newTestSupervisor(t, &buf)

	sc := newSleepSidecar("shutdown", "60")
	if err := sup.Start(context.Background(), sc); err != nil {
		t.Fatalf("Start: %v", err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	if err := sup.WaitReady(ctx, "shutdown"); err != nil {
		t.Fatalf("WaitReady: %v", err)
	}

	start := time.Now()
	sup.Shutdown()
	elapsed := time.Since(start)
	if elapsed > 2*time.Second {
		t.Errorf("Shutdown took too long: %v", elapsed)
	}

	// After shutdown, Start returns an error.
	if err := sup.Start(context.Background(), newSleepSidecar("post", "5")); err == nil {
		t.Errorf("Start after Shutdown should fail")
	}
}

func TestSupervisor_HealthCheckFailureRestarts(t *testing.T) {
	skipIfWindows(t)
	var buf bytes.Buffer
	sup := newTestSupervisor(t, &buf)
	defer sup.Shutdown()

	sc := newSleepSidecar("sickly", "60")
	sc.forceUnhealthyAfter = 300 * time.Millisecond

	if err := sup.Start(context.Background(), sc); err != nil {
		t.Fatalf("Start: %v", err)
	}

	// Wait for at least one restart driven by health failure.
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		if sc.StartCount() >= 2 {
			break
		}
		time.Sleep(50 * time.Millisecond)
	}
	if sc.StartCount() < 2 {
		t.Fatalf("expected >=2 starts after health failure, got %d", sc.StartCount())
	}
}

func TestSupervisor_MultipleSidecarsConcurrent(t *testing.T) {
	skipIfWindows(t)
	var buf bytes.Buffer
	sup := newTestSupervisor(t, &buf)
	defer sup.Shutdown()

	const n = 4
	sidecars := make([]*sleepSidecar, n)
	for i := 0; i < n; i++ {
		sidecars[i] = newSleepSidecar(fmt.Sprintf("s%d", i), "30")
		if err := sup.Start(context.Background(), sidecars[i]); err != nil {
			t.Fatalf("Start[%d]: %v", i, err)
		}
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	for i := 0; i < n; i++ {
		if err := sup.WaitReady(ctx, sidecars[i].Name()); err != nil {
			t.Fatalf("WaitReady[%d]: %v", i, err)
		}
	}

	stats := sup.Stats()
	if len(stats) != n {
		t.Fatalf("Stats len = %d, want %d", len(stats), n)
	}
	ready := 0
	for _, s := range stats {
		if s.Ready {
			ready++
		}
	}
	if ready != n {
		t.Errorf("ready sidecars = %d, want %d", ready, n)
	}
}

func TestSupervisor_DuplicateName(t *testing.T) {
	skipIfWindows(t)
	var buf bytes.Buffer
	sup := newTestSupervisor(t, &buf)
	defer sup.Shutdown()

	a := newSleepSidecar("dup", "30")
	b := newSleepSidecar("dup", "30")
	if err := sup.Start(context.Background(), a); err != nil {
		t.Fatalf("Start a: %v", err)
	}
	if err := sup.Start(context.Background(), b); err == nil {
		t.Errorf("duplicate Start should fail")
	}
}

func TestSupervisor_StopAndRestart(t *testing.T) {
	skipIfWindows(t)
	var buf bytes.Buffer
	sup := newTestSupervisor(t, &buf)
	defer sup.Shutdown()

	sc := newSleepSidecar("restartable", "30")
	if err := sup.Start(context.Background(), sc); err != nil {
		t.Fatalf("Start: %v", err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	if err := sup.WaitReady(ctx, "restartable"); err != nil {
		t.Fatalf("WaitReady: %v", err)
	}

	if err := sup.Restart(context.Background(), "restartable"); err != nil {
		t.Fatalf("Restart: %v", err)
	}
	if err := sup.WaitReady(ctx, "restartable"); err != nil {
		t.Fatalf("WaitReady after restart: %v", err)
	}
	if sc.StartCount() < 2 {
		t.Errorf("start count after restart = %d, want >=2", sc.StartCount())
	}

	if err := sup.Stop("restartable"); err != nil {
		t.Fatalf("Stop: %v", err)
	}
	if err := sup.Stop("restartable"); err == nil {
		t.Errorf("second Stop should fail")
	}
}

func TestSupervisor_LogCapture(t *testing.T) {
	skipIfWindows(t)
	var buf bytes.Buffer
	sup := newTestSupervisor(t, &buf)
	defer sup.Shutdown()

	sc := &echoSidecar{name: "echoer", line: "hello-from-sidecar"}
	if err := sup.Start(context.Background(), sc); err != nil {
		t.Fatalf("Start: %v", err)
	}
	// Wait a moment for the echo line to flush through the pipe.
	time.Sleep(400 * time.Millisecond)
	sup.Shutdown()

	if !bytes.Contains(buf.Bytes(), []byte("hello-from-sidecar")) {
		t.Errorf("expected captured stdout in logs, got:\n%s", buf.String())
	}
	if !bytes.Contains(buf.Bytes(), []byte("component=echoer")) {
		t.Errorf("expected component=echoer tag in logs, got:\n%s", buf.String())
	}
}

// echoSidecar runs `sh -c 'echo LINE; sleep 30'` so we can assert log
// capture hooks stdout through slog.
type echoSidecar struct {
	name string
	line string
}

func (e *echoSidecar) Name() string { return e.name }
func (e *echoSidecar) Command(ctx context.Context) *exec.Cmd {
	return exec.CommandContext(ctx, "sh", "-c", fmt.Sprintf("echo %s; sleep 30", e.line))
}
func (e *echoSidecar) HealthCheck(ctx context.Context) error { return nil }
func (e *echoSidecar) OnReady()                              {}
func (e *echoSidecar) Env() []string                         { return nil }
func (e *echoSidecar) WorkDir() string                       { return "" }
