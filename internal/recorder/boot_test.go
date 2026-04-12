package recorder

import (
	"context"
	"log/slog"
	"os"
	"path/filepath"
	"testing"

	"github.com/bluenviron/mediamtx/internal/recorder/pairing"
	"github.com/bluenviron/mediamtx/internal/recorder/state"
)

// TestBootAlreadyPaired verifies that Boot succeeds when the local
// state store already contains a valid PairedState, skipping the
// Joiner entirely and bringing up the supervisor + streaming clients.
func TestBootAlreadyPaired(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	dbPath := filepath.Join(dir, "state.db")

	// Pre-seed the state store with a paired entry.
	store, err := state.Open(dbPath, state.Options{})
	if err != nil {
		t.Fatalf("open state store: %v", err)
	}
	ps := pairing.PairedState{
		RecorderUUID: "test-recorder-uuid",
		DirectoryURL: "https://directory.test:8443",
		MeshHostname: "recorder-test-recorder-uuid",
		CACertFP:     "deadbeef",
	}
	if err := store.SetState(context.Background(), "pairing.paired", ps); err != nil {
		t.Fatalf("seed paired state: %v", err)
	}
	store.Close()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	srv, err := Boot(ctx, BootConfig{
		StateDir:     dir,
		Logger:       slog.Default(),
		MeshTestMode: true,
		// MediaMTX API is unreachable in tests; supervisor starts fail-open.
		MediaMTXAPIURL: "http://127.0.0.1:19997",
	})
	if err != nil {
		t.Fatalf("Boot() error: %v", err)
	}
	defer srv.Shutdown()

	if srv.RecorderID != "test-recorder-uuid" {
		t.Errorf("RecorderID = %q, want %q", srv.RecorderID, "test-recorder-uuid")
	}
	if srv.DirectoryURL != "https://directory.test:8443" {
		t.Errorf("DirectoryURL = %q, want %q", srv.DirectoryURL, "https://directory.test:8443")
	}
	if srv.MeshHostname != "recorder-test-recorder-uuid" {
		t.Errorf("MeshHostname = %q, want %q", srv.MeshHostname, "recorder-test-recorder-uuid")
	}
}

// TestBootNotPairedNoToken verifies that Boot returns a clear error
// when the Recorder is not yet paired and no pairing token is provided.
func TestBootNotPairedNoToken(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()

	// Ensure MTX_PAIRING_TOKEN is not set.
	os.Unsetenv("MTX_PAIRING_TOKEN")

	ctx := context.Background()
	_, err := Boot(ctx, BootConfig{
		StateDir: dir,
		Logger:   slog.Default(),
	})
	if err == nil {
		t.Fatal("expected error for unpaired recorder with no token")
	}
	if got := err.Error(); got == "" {
		t.Fatal("expected non-empty error message")
	}
	t.Logf("got expected error: %v", err)
}

// TestBooterInterface verifies the Booter satisfies RecorderBooter.
func TestBooterInterface(t *testing.T) {
	t.Parallel()

	b := &Booter{}
	// PairingRedeemer should return a non-nil value.
	if b.PairingRedeemer() == nil {
		t.Fatal("PairingRedeemer() returned nil")
	}
	// Shutdown on un-booted Booter should be safe.
	if err := b.Shutdown(context.Background()); err != nil {
		t.Fatalf("Shutdown() on un-booted Booter: %v", err)
	}
}

// TestBootShutdownIdempotent verifies that calling Shutdown multiple
// times on a RecorderServer does not panic.
func TestBootShutdownIdempotent(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	dbPath := filepath.Join(dir, "state.db")

	// Pre-seed.
	store, err := state.Open(dbPath, state.Options{})
	if err != nil {
		t.Fatalf("open state store: %v", err)
	}
	ps := pairing.PairedState{
		RecorderUUID: "idempotent-test",
		DirectoryURL: "https://dir.test:443",
		MeshHostname: "recorder-idempotent-test",
	}
	if err := store.SetState(context.Background(), "pairing.paired", ps); err != nil {
		t.Fatalf("seed: %v", err)
	}
	store.Close()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	srv, err := Boot(ctx, BootConfig{
		StateDir:       dir,
		Logger:         slog.Default(),
		MeshTestMode:   true,
		MediaMTXAPIURL: "http://127.0.0.1:19997",
	})
	if err != nil {
		t.Fatalf("Boot() error: %v", err)
	}

	// Double shutdown should not panic.
	srv.Shutdown()
	srv.Shutdown()
}
