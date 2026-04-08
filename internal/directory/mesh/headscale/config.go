package headscale

import (
	"errors"
	"fmt"
	"regexp"
)

// DefaultNamespace is the single namespace per site. See doc.go for
// the rationale behind namespace-per-site.
const DefaultNamespace = "kaivue-site"

// DefaultStateDir is the canonical on-disk location for the encrypted
// coordinator state in production deployments. Override via Config.
const DefaultStateDir = "/var/lib/mediamtx-directory/mesh"

// DefaultListenAddr is the loopback bind the coordinator listens on
// when Config.ListenAddr is empty. The coordinator never binds a
// public interface — tsnet clients either dial it via the mesh or via
// the Directory ingress that fronts it.
const DefaultListenAddr = "127.0.0.1:0"

// Sentinel errors. Callers should check with errors.Is.
var (
	// ErrMissingMasterKey is returned when Config.MasterKey is empty
	// but TestMode is false. The master key comes from mediamtx.yml
	// nvrJWTSecret and is mandatory in production.
	ErrMissingMasterKey = errors.New("headscale: master key required")

	// ErrInvalidNamespace is returned when the namespace is not a
	// DNS-safe label.
	ErrInvalidNamespace = errors.New("headscale: invalid namespace")

	// ErrNotStarted is returned by operations that require a running
	// coordinator before Start has been called.
	ErrNotStarted = errors.New("headscale: coordinator not started")

	// ErrAlreadyStarted is returned by a second Start call.
	ErrAlreadyStarted = errors.New("headscale: coordinator already started")

	// ErrStopped is returned after Shutdown has been called.
	ErrStopped = errors.New("headscale: coordinator stopped")

	// ErrUnknownNode is returned by RevokeNode when no node with the
	// supplied ID exists.
	ErrUnknownNode = errors.New("headscale: unknown node")

	// ErrEmptyNamespaceArg is returned by MintPreAuthKey when its
	// namespace argument is empty.
	ErrEmptyNamespaceArg = errors.New("headscale: namespace argument must not be empty")
)

// namespaceRE constrains namespaces to DNS-safe labels.
var namespaceRE = regexp.MustCompile(`^[a-z0-9]([a-z0-9-]{0,61}[a-z0-9])?$`)

// Config configures a Coordinator.
type Config struct {
	// ListenAddr is the local bind address of the coordinator control
	// interface. It must be a loopback address or a unix socket path
	// — the coordinator never listens on a public interface. Empty
	// defaults to DefaultListenAddr.
	ListenAddr string

	// StateDir is the directory where the coordinator persists its
	// encrypted database (nodes, pre-auth keys, namespace metadata).
	// Empty defaults to DefaultStateDir. Ignored in TestMode.
	StateDir string

	// Namespace is the single namespace created on first start.
	// Empty defaults to DefaultNamespace.
	Namespace string

	// ServerURL is the customer-facing URL tsnet clients dial. It is
	// typically derived from the Directory public endpoint. Empty
	// defaults to "http://" + effective ListenAddr, which is only
	// sensible in tests.
	ServerURL string

	// MasterKey is the raw bytes of the site master key, normally
	// the value of mediamtx.yml's nvrJWTSecret. Do NOT add a new
	// field in mediamtx.yml for the coordinator — per CLAUDE.md the
	// existing master key is reused.
	//
	// Mandatory unless TestMode is true.
	MasterKey []byte

	// Logf is a structured-log sink. Production callers should wire
	// this to internal/shared/logging. Nil is tolerated and silently
	// discards coordinator diagnostics.
	Logf func(format string, args ...any)

	// TestMode selects the hermetic in-memory backend. No disk is
	// touched and MasterKey is not required. Intended for unit tests
	// across the codebase.
	TestMode bool
}

// withDefaults returns a copy of c with empty fields filled in.
func (c Config) withDefaults() Config {
	out := c
	if out.ListenAddr == "" {
		out.ListenAddr = DefaultListenAddr
	}
	if out.StateDir == "" {
		out.StateDir = DefaultStateDir
	}
	if out.Namespace == "" {
		out.Namespace = DefaultNamespace
	}
	return out
}

// Validate checks the invariants that must hold before Start is called.
// It does not touch the filesystem or network.
func (c Config) Validate() error {
	cfg := c.withDefaults()
	if !namespaceRE.MatchString(cfg.Namespace) {
		return fmt.Errorf("%w: %q", ErrInvalidNamespace, cfg.Namespace)
	}
	if !cfg.TestMode && len(cfg.MasterKey) == 0 {
		return ErrMissingMasterKey
	}
	return nil
}
