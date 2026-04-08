package tsnet

import (
	"errors"
	"fmt"
	"regexp"
	"strings"
)

// NodeConfig configures a mesh Node.
type NodeConfig struct {
	// Hostname is the component's mesh hostname. It must be a
	// DNS-safe label (lowercase letters, digits, hyphens; no leading
	// or trailing hyphen) and no longer than 63 characters. A typical
	// value is "recorder-abc123" or "directory-xyz789"; the tailnet
	// suffix (".tailnet.local") is supplied by the coordinator and is
	// not part of this field.
	Hostname string

	// AuthKey is a Headscale pre-auth key minted for this component
	// by the pairing flow (KAI-243). This package performs only
	// shallow validation; the control server is the authority.
	AuthKey string

	// StateDir is the on-disk directory where tsnet persists its
	// long-lived node identity (the WireGuard key, the control
	// credentials, DERP maps, etc). It is ignored in TestMode.
	StateDir string

	// ControlURL is the Headscale coordinator URL. Leave empty to
	// fall back to tsnet's default (login.tailscale.com) — useful
	// only for local smoke tests. Production deployments will set
	// this to the URL of the embedded Headscale from KAI-240.
	ControlURL string

	// Logf is an optional structured-log sink. When nil the Node
	// discards tsnet's internal logging. Production callers should
	// wire this to internal/shared/logging.
	Logf func(format string, args ...any)

	// TestMode selects the hermetic in-memory backend. It is
	// intended for unit tests; see the package doc.
	TestMode bool
}

// ErrInvalidHostname is returned when NodeConfig.Hostname is not a
// DNS-safe label of at most 63 characters.
var ErrInvalidHostname = errors.New("tsnet: invalid hostname")

// ErrInvalidAuthKey is returned when NodeConfig.AuthKey fails shallow
// format validation.
var ErrInvalidAuthKey = errors.New("tsnet: invalid auth key")

// ErrUnknownHost is returned by test-mode Dial when the requested
// hostname is not registered in the in-memory address registry.
var ErrUnknownHost = errors.New("tsnet: unknown host")

// ErrClosed is returned after Shutdown has been called.
var ErrClosed = errors.New("tsnet: node closed")

// hostnameRE matches a single DNS label: RFC 1123 with the additional
// constraint that we require lowercase (tailnet hostnames are
// normalized lowercase by the control plane).
var hostnameRE = regexp.MustCompile(`^[a-z0-9]([a-z0-9-]{0,61}[a-z0-9])?$`)

// Validate checks the hostname and auth key. It does not touch the
// state directory or network; those are validated when the Node is
// actually started.
func (c NodeConfig) Validate() error {
	if err := validateHostname(c.Hostname); err != nil {
		return err
	}
	if !c.TestMode {
		// Test-mode nodes do not authenticate against a real
		// coordinator, so AuthKey is optional.
		if err := validateAuthKey(c.AuthKey); err != nil {
			return err
		}
		if c.StateDir == "" {
			return errors.New("tsnet: StateDir required in real mode")
		}
	}
	return nil
}

func validateHostname(h string) error {
	if h == "" {
		return fmt.Errorf("%w: empty", ErrInvalidHostname)
	}
	if len(h) > 63 {
		return fmt.Errorf("%w: %q exceeds 63 characters", ErrInvalidHostname, h)
	}
	if !hostnameRE.MatchString(h) {
		return fmt.Errorf("%w: %q is not a DNS-safe label", ErrInvalidHostname, h)
	}
	return nil
}

// validateAuthKey performs shallow format checks only. Real pre-auth
// keys look like "tskey-auth-..." but Headscale may use a different
// prefix; we therefore accept any non-empty key that begins with
// "tskey-", "hskey-", or is at least 16 characters of opaque data.
func validateAuthKey(k string) error {
	if k == "" {
		return fmt.Errorf("%w: empty", ErrInvalidAuthKey)
	}
	if strings.HasPrefix(k, "tskey-") || strings.HasPrefix(k, "hskey-") {
		return nil
	}
	if len(k) >= 16 {
		return nil
	}
	return fmt.Errorf("%w: %q does not look like a pre-auth key", ErrInvalidAuthKey, k)
}
