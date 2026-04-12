package directory_test

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"os"
	"testing"
	"time"

	"github.com/bluenviron/mediamtx/internal/directory"
)

func TestBoot_StartsAndServesHealthz(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "directory-boot-test-*")
	if err != nil {
		t.Fatalf("create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	// Use a random port and disable mDNS (requires multicast which may
	// not be available in CI).
	mdnsOff := false
	srv, err := directory.Boot(ctx, directory.BootConfig{
		DataDir:     tmpDir,
		ListenAddr:  "127.0.0.1:0",
		MasterKey:   []byte("test-master-key-at-least-16-bytes"),
		Logger:      slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelDebug})),
		MDNSEnabled: &mdnsOff,
	})
	if err != nil {
		t.Fatalf("Boot() failed: %v", err)
	}
	defer srv.Shutdown(ctx) //nolint:errcheck

	// Verify we got a bound address.
	addr := srv.Addr()
	if addr == "" {
		t.Fatal("server address is empty after Boot")
	}

	// GET /healthz should return 200 with status=ok.
	healthURL := fmt.Sprintf("http://%s/healthz", addr)
	resp, err := http.Get(healthURL)
	if err != nil {
		t.Fatalf("GET /healthz failed: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("GET /healthz status = %d, want 200", resp.StatusCode)
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatalf("read body: %v", err)
	}

	var result map[string]string
	if err := json.Unmarshal(body, &result); err != nil {
		t.Fatalf("unmarshal healthz response: %v", err)
	}

	if result["status"] != "ok" {
		t.Errorf("healthz status = %q, want %q", result["status"], "ok")
	}
	if result["mode"] != "directory" {
		t.Errorf("healthz mode = %q, want %q", result["mode"], "directory")
	}
}

func TestBoot_ShutdownIsIdempotent(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "directory-shutdown-test-*")
	if err != nil {
		t.Fatalf("create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	ctx := context.Background()
	mdnsOff := false
	srv, err := directory.Boot(ctx, directory.BootConfig{
		DataDir:     tmpDir,
		ListenAddr:  "127.0.0.1:0",
		MasterKey:   []byte("test-master-key-at-least-16-bytes"),
		MDNSEnabled: &mdnsOff,
	})
	if err != nil {
		t.Fatalf("Boot() failed: %v", err)
	}

	// Shutdown twice should not panic.
	if err := srv.Shutdown(ctx); err != nil {
		t.Fatalf("first Shutdown: %v", err)
	}
	if err := srv.Shutdown(ctx); err != nil {
		t.Fatalf("second Shutdown: %v", err)
	}
}
