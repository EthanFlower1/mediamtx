//go:build realtsnet

// This file is compiled only with -tags realtsnet. It brings up a
// real tsnet.Server against an actual control URL and is intended to
// be run by a human operator, not by CI. See README.md for
// instructions on how to run it against a local Headscale.

package tsnet

import (
	"context"
	"os"
	"testing"
	"time"
)

func TestRealNodeSmoke(t *testing.T) {
	authKey := os.Getenv("KAIVUE_TSNET_AUTHKEY")
	if authKey == "" {
		t.Skip("KAIVUE_TSNET_AUTHKEY not set")
	}
	controlURL := os.Getenv("KAIVUE_TSNET_CONTROL_URL")
	stateDir := t.TempDir()

	n, err := New(NodeConfig{
		Hostname:   "kaivue-smoke-test",
		AuthKey:    authKey,
		StateDir:   stateDir,
		ControlURL: controlURL,
		Logf:       t.Logf,
	})
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	if err := n.Shutdown(ctx); err != nil {
		t.Fatalf("Shutdown: %v", err)
	}
}
