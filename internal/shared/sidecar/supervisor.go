package sidecar

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"math/rand"
	"os"
	"os/exec"
	"sync"
	"syscall"
	"time"

	"github.com/bluenviron/mediamtx/internal/shared/logging"
)

// Default tuning knobs. Callers can override via Config.
const (
	defaultHealthInterval = 5 * time.Second
	defaultHealthTimeout  = 3 * time.Second
	defaultBackoffBase    = 500 * time.Millisecond
	defaultBackoffCap     = 60 * time.Second
	defaultGracePeriod    = 10 * time.Second
)

// Config tunes Supervisor behaviour. All fields are optional; zero
// values fall back to package-level defaults.
type Config struct {
	// HealthInterval is the period between HealthCheck polls once
	// a sidecar has started. Default: 5s.
	HealthInterval time.Duration

	// HealthTimeout bounds each individual HealthCheck call.
	// Default: 3s.
	HealthTimeout time.Duration

	// BackoffBase is the starting delay after the first crash.
	// Default: 500ms.
	BackoffBase time.Duration

	// BackoffCap is the ceiling for exponential backoff. Default: 60s.
	BackoffCap time.Duration

	// GracePeriod is how long Shutdown waits after SIGTERM before
	// sending SIGKILL. Default: 10s.
	GracePeriod time.Duration

	// Logger is the base slog logger. Per-sidecar loggers are
	// derived via logging.WithComponent. If nil, slog.Default()
	// is used.
	Logger *slog.Logger

	// Rand lets tests inject a deterministic jitter source. Nil
	// means use a package-local rand seeded from time.
	Rand *rand.Rand
}

func (c *Config) applyDefaults() {
	if c.HealthInterval <= 0 {
		c.HealthInterval = defaultHealthInterval
	}
	if c.HealthTimeout <= 0 {
		c.HealthTimeout = defaultHealthTimeout
	}
	if c.BackoffBase <= 0 {
		c.BackoffBase = defaultBackoffBase
	}
	if c.BackoffCap <= 0 {
		c.BackoffCap = defaultBackoffCap
	}
	if c.GracePeriod <= 0 {
		c.GracePeriod = defaultGracePeriod
	}
	if c.Logger == nil {
		c.Logger = slog.Default()
	}
}

// Stats is a point-in-time snapshot of one sidecar's health.
type Stats struct {
	Name        string
	Running     bool
	Ready       bool
	CrashCount  int
	LastRestart time.Time
	StartedAt   time.Time
	Uptime      time.Duration
}

// Supervisor manages a fleet of Sidecars. The zero value is not
// usable; construct via NewSupervisor.
type Supervisor struct {
	cfg  Config
	rng  *rand.Rand
	rngM sync.Mutex

	mu      sync.Mutex
	workers map[string]*worker
	closed  bool
}

// NewSupervisor constructs a Supervisor with the given config.
func NewSupervisor(cfg Config) *Supervisor {
	cfg.applyDefaults()
	rng := cfg.Rand
	if rng == nil {
		rng = rand.New(rand.NewSource(time.Now().UnixNano()))
	}
	return &Supervisor{
		cfg:     cfg,
		rng:     rng,
		workers: make(map[string]*worker),
	}
}

// Start registers a sidecar and begins its supervision loop. It
// returns immediately; the sidecar is not guaranteed to be ready yet.
// Use WaitReady to block until HealthCheck first passes.
//
// Starting a name that is already registered returns an error.
func (s *Supervisor) Start(ctx context.Context, sc Sidecar) error {
	if sc == nil {
		return errors.New("sidecar: nil sidecar")
	}
	name := sc.Name()
	if name == "" {
		return errors.New("sidecar: empty name")
	}

	s.mu.Lock()
	if s.closed {
		s.mu.Unlock()
		return errors.New("sidecar: supervisor is shut down")
	}
	if _, exists := s.workers[name]; exists {
		s.mu.Unlock()
		return fmt.Errorf("sidecar: %q already registered", name)
	}

	logger := logging.WithComponent(s.cfg.Logger, name)
	wctx, cancel := context.WithCancel(ctx)
	w := &worker{
		sup:    s,
		sc:     sc,
		name:   name,
		logger: logger,
		ctx:    wctx,
		cancel: cancel,
		ready:  make(chan struct{}),
		done:   make(chan struct{}),
	}
	s.workers[name] = w
	s.mu.Unlock()

	go w.run()
	return nil
}

// Stop tears down a single named sidecar. It blocks until the
// supervision loop has exited and the process is no longer running.
func (s *Supervisor) Stop(name string) error {
	s.mu.Lock()
	w, ok := s.workers[name]
	if ok {
		delete(s.workers, name)
	}
	s.mu.Unlock()
	if !ok {
		return fmt.Errorf("sidecar: %q not found", name)
	}
	w.stop(s.cfg.GracePeriod)
	return nil
}

// Restart stops and starts a single sidecar by name, preserving its
// Sidecar interface value. It blocks until the replacement worker has
// been scheduled (but not necessarily ready).
func (s *Supervisor) Restart(ctx context.Context, name string) error {
	s.mu.Lock()
	w, ok := s.workers[name]
	s.mu.Unlock()
	if !ok {
		return fmt.Errorf("sidecar: %q not found", name)
	}
	sc := w.sc
	if err := s.Stop(name); err != nil {
		return err
	}
	return s.Start(ctx, sc)
}

// WaitReady blocks until the named sidecar has passed its first
// HealthCheck, or until ctx is cancelled.
func (s *Supervisor) WaitReady(ctx context.Context, name string) error {
	s.mu.Lock()
	w, ok := s.workers[name]
	s.mu.Unlock()
	if !ok {
		return fmt.Errorf("sidecar: %q not found", name)
	}
	select {
	case <-w.ready:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	case <-w.done:
		return fmt.Errorf("sidecar: %q exited before becoming ready", name)
	}
}

// Shutdown sends SIGTERM to every registered sidecar and waits up to
// GracePeriod for each to exit before escalating to SIGKILL. After
// Shutdown returns, the Supervisor refuses further Start calls.
func (s *Supervisor) Shutdown() {
	s.mu.Lock()
	if s.closed {
		s.mu.Unlock()
		return
	}
	s.closed = true
	workers := make([]*worker, 0, len(s.workers))
	for _, w := range s.workers {
		workers = append(workers, w)
	}
	s.workers = map[string]*worker{}
	s.mu.Unlock()

	var wg sync.WaitGroup
	for _, w := range workers {
		wg.Add(1)
		go func(w *worker) {
			defer wg.Done()
			w.stop(s.cfg.GracePeriod)
		}(w)
	}
	wg.Wait()
}

// Stats returns a snapshot of every registered sidecar's metrics.
func (s *Supervisor) Stats() []Stats {
	s.mu.Lock()
	defer s.mu.Unlock()
	out := make([]Stats, 0, len(s.workers))
	for _, w := range s.workers {
		out = append(out, w.stats())
	}
	return out
}

// nextBackoff returns an exponentially-increasing delay for the
// given attempt count (1-based) with a ±25% jitter, capped at
// BackoffCap.
func (s *Supervisor) nextBackoff(attempt int) time.Duration {
	if attempt < 1 {
		attempt = 1
	}
	// base * 2^(attempt-1)
	d := s.cfg.BackoffBase
	for i := 1; i < attempt; i++ {
		d *= 2
		if d >= s.cfg.BackoffCap {
			d = s.cfg.BackoffCap
			break
		}
	}
	if d > s.cfg.BackoffCap {
		d = s.cfg.BackoffCap
	}
	// jitter ±25%
	s.rngM.Lock()
	j := s.rng.Float64()*0.5 - 0.25
	s.rngM.Unlock()
	d += time.Duration(float64(d) * j)
	if d < 0 {
		d = 0
	}
	return d
}

// ---------------------------------------------------------------------
// worker — per-sidecar state + control loop
// ---------------------------------------------------------------------

type worker struct {
	sup    *Supervisor
	sc     Sidecar
	name   string
	logger *slog.Logger

	ctx    context.Context
	cancel context.CancelFunc

	ready chan struct{} // closed on first healthy
	done  chan struct{} // closed when run() returns

	mu           sync.Mutex
	cmd          *exec.Cmd
	running      bool
	isReady      bool
	crashCount   int
	lastRestart  time.Time
	startedAt    time.Time
	stopRequested bool
}

func (w *worker) stats() Stats {
	w.mu.Lock()
	defer w.mu.Unlock()
	var uptime time.Duration
	if w.running && !w.startedAt.IsZero() {
		uptime = time.Since(w.startedAt)
	}
	return Stats{
		Name:        w.name,
		Running:     w.running,
		Ready:       w.isReady,
		CrashCount:  w.crashCount,
		LastRestart: w.lastRestart,
		StartedAt:   w.startedAt,
		Uptime:      uptime,
	}
}

// run is the main supervision loop. It returns only when ctx is
// cancelled (via Stop / Shutdown).
func (w *worker) run() {
	defer close(w.done)
	attempt := 0
	for {
		if err := w.ctx.Err(); err != nil {
			return
		}
		attempt++
		err := w.runOnce()
		if w.ctx.Err() != nil {
			return
		}

		w.mu.Lock()
		w.crashCount++
		w.lastRestart = time.Now()
		w.mu.Unlock()

		delay := w.sup.nextBackoff(attempt)
		w.logger.Warn("sidecar exited, scheduling restart",
			slog.Duration("backoff", delay),
			slog.Int("attempt", attempt),
			slog.Any("error", err),
		)

		select {
		case <-time.After(delay):
		case <-w.ctx.Done():
			return
		}
	}
}

// runOnce starts the subprocess, streams its logs, polls health, and
// blocks until either the process exits or health fails. It returns
// the exit error (or the health error, or ctx error).
func (w *worker) runOnce() error {
	cmd := w.sc.Command(w.ctx)
	if cmd == nil {
		return errors.New("sidecar: Command returned nil")
	}
	// Merge env: inherit process env then append custom entries.
	if env := w.sc.Env(); len(env) > 0 {
		cmd.Env = append(os.Environ(), env...)
	}
	if wd := w.sc.WorkDir(); wd != "" {
		cmd.Dir = wd
	}

	stdoutR, stdoutW := io.Pipe()
	stderrR, stderrW := io.Pipe()
	cmd.Stdout = stdoutW
	cmd.Stderr = stderrW

	// Put the child in its own process group so we can signal the
	// whole tree on Stop().
	setProcessGroup(cmd)

	if err := cmd.Start(); err != nil {
		stdoutW.Close()
		stderrW.Close()
		return fmt.Errorf("start: %w", err)
	}

	w.mu.Lock()
	w.cmd = cmd
	w.running = true
	w.isReady = false
	w.startedAt = time.Now()
	w.mu.Unlock()

	w.logger.Info("sidecar started", slog.Int("pid", cmd.Process.Pid))

	// Pipe readers — closed automatically when the child closes
	// its end and cmd.Wait reaps the pipes.
	var pipeWG sync.WaitGroup
	pipeWG.Add(2)
	go func() { defer pipeWG.Done(); w.pumpLogs(stdoutR, slog.LevelInfo) }()
	go func() { defer pipeWG.Done(); w.pumpLogs(stderrR, slog.LevelWarn) }()

	// Health check loop + readiness signaling.
	healthErrCh := make(chan error, 1)
	healthCtx, healthCancel := context.WithCancel(w.ctx)
	defer healthCancel()
	go w.healthLoop(healthCtx, healthErrCh)

	// Wait for the process to exit.
	waitCh := make(chan error, 1)
	go func() { waitCh <- cmd.Wait() }()

	var runErr error
	select {
	case err := <-waitCh:
		runErr = err
	case err := <-healthErrCh:
		runErr = fmt.Errorf("health: %w", err)
		// Kill the subprocess so Wait unblocks.
		w.killProcess(cmd)
		<-waitCh
	case <-w.ctx.Done():
		// Stop requested; terminate gracefully.
		w.terminateProcess(cmd, w.sup.cfg.GracePeriod)
		<-waitCh
		runErr = w.ctx.Err()
	}

	// Close the pipe writers so the pumpLogs goroutines unblock.
	stdoutW.Close()
	stderrW.Close()
	pipeWG.Wait()

	w.mu.Lock()
	w.running = false
	w.isReady = false
	w.cmd = nil
	w.mu.Unlock()

	return runErr
}

// healthLoop polls sc.HealthCheck at cfg.HealthInterval. On the first
// nil return it closes w.ready and calls OnReady. On any subsequent
// failure it sends the error to out and exits.
func (w *worker) healthLoop(ctx context.Context, out chan<- error) {
	interval := w.sup.cfg.HealthInterval
	timeout := w.sup.cfg.HealthTimeout

	// Small initial delay so the process has a chance to open
	// its listening sockets before the first probe.
	select {
	case <-time.After(50 * time.Millisecond):
	case <-ctx.Done():
		return
	}

	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	// Use a tighter poll until ready.
	firstTick := time.NewTicker(interval / 5)
	defer firstTick.Stop()

	ready := false
	check := func() error {
		hctx, cancel := context.WithTimeout(ctx, timeout)
		defer cancel()
		return w.sc.HealthCheck(hctx)
	}

	for {
		if !ready {
			if err := check(); err == nil {
				ready = true
				w.mu.Lock()
				w.isReady = true
				w.mu.Unlock()
				w.logger.Info("sidecar ready")
				// Fire OnReady; swallow panics so a
				// buggy callback can't crash the
				// supervisor.
				func() {
					defer func() {
						if r := recover(); r != nil {
							w.logger.Error("OnReady panic",
								slog.Any("panic", r))
						}
					}()
					w.sc.OnReady()
				}()
				// Close ready exactly once.
				select {
				case <-w.ready:
				default:
					close(w.ready)
				}
			}
		} else {
			if err := check(); err != nil {
				select {
				case out <- err:
				case <-ctx.Done():
				}
				return
			}
		}

		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
		case <-firstTick.C:
			if ready {
				// Disable fast ticker once ready by
				// draining it between slow ticks.
				continue
			}
		}
	}
}

// pumpLogs reads lines from r and re-emits them via the worker's
// logger at the requested level with a "stream" attribute.
func (w *worker) pumpLogs(r io.Reader, level slog.Level) {
	scanner := bufio.NewScanner(r)
	// Allow long lines from verbose sidecars (MediaMTX can emit
	// multi-KB RTSP DESCRIBE responses at debug level).
	scanner.Buffer(make([]byte, 0, 4096), 1024*1024)
	streamName := "stdout"
	if level >= slog.LevelWarn {
		streamName = "stderr"
	}
	for scanner.Scan() {
		line := scanner.Text()
		w.logger.Log(context.Background(), level, line,
			slog.String("stream", streamName))
	}
}

// stop signals the worker to exit, terminates any running process,
// and blocks until the run loop returns.
func (w *worker) stop(grace time.Duration) {
	w.mu.Lock()
	w.stopRequested = true
	cmd := w.cmd
	w.mu.Unlock()

	w.cancel()
	if cmd != nil && cmd.Process != nil {
		w.terminateProcess(cmd, grace)
	}
	<-w.done
}

// terminateProcess sends SIGTERM to the process group, waits up to
// grace, then escalates to SIGKILL.
func (w *worker) terminateProcess(cmd *exec.Cmd, grace time.Duration) {
	if cmd == nil || cmd.Process == nil {
		return
	}
	_ = signalProcessGroup(cmd, syscall.SIGTERM)

	done := make(chan struct{})
	go func() {
		// We can't Wait here (the run loop already does). Poll
		// via signal 0 which returns an error once the process
		// is gone.
		for {
			if err := cmd.Process.Signal(syscall.Signal(0)); err != nil {
				close(done)
				return
			}
			time.Sleep(50 * time.Millisecond)
		}
	}()

	select {
	case <-done:
		return
	case <-time.After(grace):
		w.logger.Warn("sidecar did not exit on SIGTERM, sending SIGKILL",
			slog.Duration("grace", grace))
		_ = signalProcessGroup(cmd, syscall.SIGKILL)
	}
}

// killProcess hard-kills a subprocess without waiting. Used when a
// health check fails and we want a fast restart.
func (w *worker) killProcess(cmd *exec.Cmd) {
	if cmd == nil || cmd.Process == nil {
		return
	}
	_ = signalProcessGroup(cmd, syscall.SIGKILL)
}
