//go:build integration

package idptest

import (
	"context"
	"fmt"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"time"
)

// ComposeUp starts the mock-IdP Docker Compose stack and blocks until every
// service passes its healthcheck (or ctx is cancelled). It returns a teardown
// function that calls docker compose down -v.
//
// The caller is expected to invoke this from TestMain or a test helper:
//
//	teardown, err := idptest.ComposeUp(ctx)
//	if err != nil { t.Fatal(err) }
//	defer teardown()
func ComposeUp(ctx context.Context) (teardown func(), err error) {
	file := composeFilePath()
	if _, err := os.Stat(file); err != nil {
		return nil, fmt.Errorf("idptest: docker-compose.yml not found at %s: %w", file, err)
	}

	// Start all services.
	cmd := exec.CommandContext(ctx, "docker", "compose", "-f", file, "up", "-d", "--wait")
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	if err := cmd.Run(); err != nil {
		return nil, fmt.Errorf("idptest: docker compose up failed: %w", err)
	}

	// Extra belt-and-suspenders: wait for TCP ports to accept connections
	// so the first test call does not race the healthcheck poll interval.
	ports := []string{
		"localhost:9090", // OIDC
		"localhost:8080", // SAML
		"localhost:389",  // LDAP
	}
	for _, addr := range ports {
		if err := waitForPort(ctx, addr, 30*time.Second); err != nil {
			// Tear down on partial start so we don't leave orphans.
			_ = composeDown(file)
			return nil, fmt.Errorf("idptest: port %s not ready: %w", addr, err)
		}
	}

	return func() { _ = composeDown(file) }, nil
}

// composeDown runs docker compose down -v synchronously.
func composeDown(file string) error {
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, "docker", "compose", "-f", file, "down", "-v")
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	return cmd.Run()
}

// composeFilePath resolves test/idp/docker-compose.yml relative to the repo
// root. It walks up from the current source file's directory.
func composeFilePath() string {
	_, thisFile, _, _ := runtime.Caller(0)
	// thisFile = .../internal/shared/auth/zitadel/idptest/compose.go
	// repo root = 6 levels up
	root := filepath.Join(filepath.Dir(thisFile), "..", "..", "..", "..", "..", "..")
	return filepath.Join(root, "test", "idp", "docker-compose.yml")
}

// waitForPort polls a TCP address until it accepts a connection or ctx expires.
func waitForPort(ctx context.Context, addr string, timeout time.Duration) error {
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}
		conn, err := net.DialTimeout("tcp", addr, 500*time.Millisecond)
		if err == nil {
			_ = conn.Close()
			return nil
		}
		time.Sleep(500 * time.Millisecond)
	}
	return fmt.Errorf("timed out after %s", timeout)
}
