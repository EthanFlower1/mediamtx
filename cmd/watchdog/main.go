// cmd/watchdog/main.go — watchdog process for Raikada.
// Monitors the main server process, auto-restarts on crash, detects deadlocks
// via health-check timeouts, and applies exponential backoff between restarts.
package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"sync"
	"syscall"
	"time"
)

// restartRecord captures metadata about a single restart event.
type restartRecord struct {
	Timestamp time.Time `json:"timestamp"`
	Reason    string    `json:"reason"`
	ExitCode  int       `json:"exitCode"`
	Attempt   int       `json:"attempt"`
}

// restartHistory is the on-disk log of all restart events.
type restartHistory struct {
	Restarts []restartRecord `json:"restarts"`
}

// config holds all watchdog tunables.
type config struct {
	// Path to the raikada binary.
	binary string
	// Extra args forwarded to raikada.
	args []string
	// Health-check endpoint (full URL).
	healthURL string
	// How often to poll the health endpoint.
	healthInterval time.Duration
	// How long a health request may take before we consider the server hung.
	healthTimeout time.Duration
	// Number of consecutive health-check failures before we force-restart.
	healthFailThreshold int
	// Backoff parameters.
	initialBackoff time.Duration
	maxBackoff     time.Duration
	backoffFactor  float64
	// Where to persist the restart-history JSON log.
	historyPath string
	// Maximum number of restarts before the watchdog gives up (0 = unlimited).
	maxRestarts int
}

func main() {
	cfg := parseFlags()

	log.SetFlags(log.Ldate | log.Ltime | log.Lmicroseconds)
	log.Printf("[watchdog] starting — binary=%s healthURL=%s interval=%s timeout=%s",
		cfg.binary, cfg.healthURL, cfg.healthInterval, cfg.healthTimeout)

	// Trap signals so the watchdog can shut down gracefully.
	ctx, cancel := context.WithCancel(context.Background())
	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)
	go func() {
		sig := <-sigCh
		log.Printf("[watchdog] received %s, shutting down", sig)
		cancel()
	}()

	history := loadHistory(cfg.historyPath)
	backoff := cfg.initialBackoff
	attempt := 0

	for {
		if cfg.maxRestarts > 0 && attempt >= cfg.maxRestarts {
			log.Printf("[watchdog] reached maximum restart limit (%d), exiting", cfg.maxRestarts)
			os.Exit(1)
		}

		if attempt > 0 {
			log.Printf("[watchdog] backing off %s before restart (attempt %d)", backoff, attempt)
			select {
			case <-time.After(backoff):
			case <-ctx.Done():
				log.Println("[watchdog] shutdown during backoff")
				return
			}
			// Exponential backoff with cap.
			backoff = time.Duration(float64(backoff) * cfg.backoffFactor)
			if backoff > cfg.maxBackoff {
				backoff = cfg.maxBackoff
			}
		}

		exitCode, reason := runServer(ctx, cfg)
		if ctx.Err() != nil {
			// Watchdog itself was asked to stop; don't restart.
			log.Println("[watchdog] context cancelled, not restarting server")
			return
		}

		attempt++
		rec := restartRecord{
			Timestamp: time.Now().UTC(),
			Reason:    reason,
			ExitCode:  exitCode,
			Attempt:   attempt,
		}
		history.Restarts = append(history.Restarts, rec)
		saveHistory(cfg.historyPath, history)

		log.Printf("[watchdog] server exited: reason=%q exit_code=%d attempt=%d", reason, exitCode, attempt)

		// Reset backoff after a long healthy run (server lived > 5 min).
		if len(history.Restarts) >= 2 {
			prev := history.Restarts[len(history.Restarts)-2].Timestamp
			if rec.Timestamp.Sub(prev) > 5*time.Minute {
				backoff = cfg.initialBackoff
				attempt = 1
			}
		}
	}
}

// runServer starts the raikada process and monitors it.
// It returns the exit code and a human-readable reason when the server stops.
func runServer(ctx context.Context, cfg *config) (exitCode int, reason string) {
	cmd := exec.CommandContext(ctx, cfg.binary, cfg.args...)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr

	if err := cmd.Start(); err != nil {
		return -1, fmt.Sprintf("failed to start: %v", err)
	}
	log.Printf("[watchdog] server started (pid %d)", cmd.Process.Pid)

	// Channel receives when the process exits on its own.
	procDone := make(chan error, 1)
	go func() {
		procDone <- cmd.Wait()
	}()

	// Health-check loop.
	healthFails := 0
	ticker := time.NewTicker(cfg.healthInterval)
	defer ticker.Stop()

	// Wait a grace period before starting health checks so the server can boot.
	gracePeriod := 10 * time.Second
	select {
	case <-time.After(gracePeriod):
	case err := <-procDone:
		return exitCodeFrom(err), fmt.Sprintf("crashed during startup: %v", err)
	case <-ctx.Done():
		killServer(cmd)
		return -1, "watchdog shutdown"
	}

	for {
		select {
		case err := <-procDone:
			return exitCodeFrom(err), fmt.Sprintf("process exited: %v", err)

		case <-ticker.C:
			if checkHealth(cfg.healthURL, cfg.healthTimeout) {
				healthFails = 0
				continue
			}
			healthFails++
			log.Printf("[watchdog] health check failed (%d/%d)", healthFails, cfg.healthFailThreshold)
			if healthFails >= cfg.healthFailThreshold {
				log.Printf("[watchdog] deadlock detected — killing server (pid %d)", cmd.Process.Pid)
				killServer(cmd)
				// Drain the procDone channel.
				select {
				case <-procDone:
				case <-time.After(10 * time.Second):
				}
				return -1, fmt.Sprintf("deadlock: %d consecutive health-check failures", healthFails)
			}

		case <-ctx.Done():
			killServer(cmd)
			select {
			case <-procDone:
			case <-time.After(10 * time.Second):
			}
			return -1, "watchdog shutdown"
		}
	}
}

// checkHealth performs a single HTTP GET against the health endpoint.
func checkHealth(url string, timeout time.Duration) bool {
	client := &http.Client{Timeout: timeout}
	resp, err := client.Get(url) //nolint:noctx
	if err != nil {
		return false
	}
	resp.Body.Close()
	return resp.StatusCode >= 200 && resp.StatusCode < 400
}

// killServer sends SIGTERM, then SIGKILL if the process doesn't exit quickly.
func killServer(cmd *exec.Cmd) {
	if cmd.Process == nil {
		return
	}
	_ = cmd.Process.Signal(syscall.SIGTERM)

	done := make(chan struct{})
	go func() {
		cmd.Wait() //nolint:errcheck
		close(done)
	}()

	select {
	case <-done:
		log.Println("[watchdog] server stopped gracefully")
	case <-time.After(5 * time.Second):
		log.Println("[watchdog] server did not stop in time, sending SIGKILL")
		_ = cmd.Process.Kill()
	}
}

// exitCodeFrom extracts the exit code from a Wait() error.
func exitCodeFrom(err error) int {
	if err == nil {
		return 0
	}
	if exitErr, ok := err.(*exec.ExitError); ok {
		return exitErr.ExitCode()
	}
	return -1
}

// --- history persistence ---

var historyMu sync.Mutex

func loadHistory(path string) *restartHistory {
	historyMu.Lock()
	defer historyMu.Unlock()

	h := &restartHistory{}
	data, err := os.ReadFile(path)
	if err != nil {
		return h
	}
	_ = json.Unmarshal(data, h)
	return h
}

func saveHistory(path string, h *restartHistory) {
	historyMu.Lock()
	defer historyMu.Unlock()

	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		log.Printf("[watchdog] failed to create history dir: %v", err)
		return
	}
	data, err := json.MarshalIndent(h, "", "  ")
	if err != nil {
		log.Printf("[watchdog] failed to marshal history: %v", err)
		return
	}
	if err := os.WriteFile(path, data, 0o644); err != nil {
		log.Printf("[watchdog] failed to write history: %v", err)
	}
}

// --- flag parsing ---

func parseFlags() *config {
	cfg := &config{}

	home, _ := os.UserHomeDir()
	defaultHistory := filepath.Join(home, ".mediamtx", "watchdog-history.json")

	flag.StringVar(&cfg.binary, "binary", "./raikada", "path to the raikada binary")
	flag.StringVar(&cfg.healthURL, "health-url", "http://127.0.0.1:9997/v3/paths/list", "URL to poll for health checks")
	flag.DurationVar(&cfg.healthInterval, "health-interval", 15*time.Second, "interval between health checks")
	flag.DurationVar(&cfg.healthTimeout, "health-timeout", 5*time.Second, "timeout for each health-check request")
	flag.IntVar(&cfg.healthFailThreshold, "health-fail-threshold", 3, "consecutive failures before forced restart")
	flag.DurationVar(&cfg.initialBackoff, "initial-backoff", 2*time.Second, "initial restart backoff duration")
	flag.DurationVar(&cfg.maxBackoff, "max-backoff", 5*time.Minute, "maximum restart backoff duration")
	flag.Float64Var(&cfg.backoffFactor, "backoff-factor", 2.0, "backoff multiplier per restart")
	flag.StringVar(&cfg.historyPath, "history-path", defaultHistory, "path to the restart-history JSON log")
	flag.IntVar(&cfg.maxRestarts, "max-restarts", 0, "maximum restarts before giving up (0 = unlimited)")

	flag.Parse()
	cfg.args = flag.Args()

	return cfg
}
