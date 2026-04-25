// Package managed implements the Directory-managed mode for the recording server.
// When enabled, the recorder registers with a Directory server, sends periodic
// heartbeats, accepts config pushes, and exposes an internal query API for
// Directory fan-out queries.
package managed

import (
	"fmt"
	"net/url"
)

// Config holds the settings for managed (Directory-controlled) mode.
type Config struct {
	// DirectoryURL is the base URL of the Directory server
	// (e.g. "https://directory.local:9000").
	DirectoryURL string

	// ServiceToken is a shared secret used to authenticate
	// recorder ↔ Directory communication. In production this
	// would be replaced by mTLS, but a bearer token is fine for v1.
	ServiceToken string

	// InternalListenAddr is the address the internal API binds to.
	// Defaults to ":8880". Only the Directory should reach this.
	InternalListenAddr string

	// RecorderID is a stable identifier for this recorder instance.
	// If empty, one is generated on first boot and persisted to the DB.
	RecorderID string

	// Hostname is a human-readable name for this recorder.
	// Defaults to os.Hostname().
	Hostname string
}

// Validate checks that required fields are set.
func (c *Config) Validate() error {
	if c.DirectoryURL == "" {
		return fmt.Errorf("managed: DirectoryURL is required")
	}
	if _, err := url.Parse(c.DirectoryURL); err != nil {
		return fmt.Errorf("managed: invalid DirectoryURL: %w", err)
	}
	if c.ServiceToken == "" {
		return fmt.Errorf("managed: ServiceToken is required")
	}
	return nil
}

// ListenAddr returns the internal API listen address, defaulting to ":8880".
func (c *Config) ListenAddr() string {
	if c.InternalListenAddr != "" {
		return c.InternalListenAddr
	}
	return ":8880"
}
