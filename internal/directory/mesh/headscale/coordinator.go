package headscale

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"sync"
	"time"
)

// Coordinator is the per-site tailnet coordinator. The type is a
// concrete struct rather than an interface so callers across the
// codebase couple to one name; the pluggable behavior lives behind
// the unexported backend field.
//
// TODO(KAI-240/follow-up): replace newStubBackend with a real embedded
// Headscale backend. The plan is:
//
//  1. Vendor github.com/juanfont/headscale at a pinned tag.
//  2. Construct an hscontrol.Headscale directly from an in-memory
//     config struct (mirroring what headscale's own main.go does), so
//     no YAML config file or command-line flags are required.
//  3. Drive its SQLite state file through internal/shared/cryptostore
//     (open-encrypt-write wrapper) so state is encrypted at rest.
//  4. Wire Coordinator.MintPreAuthKey / ListNodes / RevokeNode onto
//     the embedded admin gRPC client.
//
// Headscale is notoriously tricky to embed because hscontrol is under
// `internal/` and its constructors move between releases. The spec
// budgets ~1 day per upgrade for adapter fixes; for the first pass we
// ship the stable interface and the stub so the rest of wave 2 can
// proceed unblocked. See README.md §TODO.
type Coordinator struct {
	cfg     Config
	backend backend

	mu      sync.Mutex
	started bool
	stopped bool
	addr    string
}

// backend is the minimal contract every coordinator implementation
// (real-embedded, stub, future sidecar shim) must satisfy. It is kept
// unexported so callers always hold a *Coordinator.
type backend interface {
	start(ctx context.Context) (addr string, err error)
	shutdown(ctx context.Context) error
	mintPreAuthKey(ctx context.Context, namespace string, ttl time.Duration) (string, error)
	listNodes(ctx context.Context) ([]NodeInfo, error)
	revokeNode(ctx context.Context, nodeID string) error
	healthy() bool
}

// New constructs a Coordinator from cfg. It does not start the
// coordinator; call Start for that. The returned Coordinator is safe
// for concurrent use.
func New(cfg Config) (*Coordinator, error) {
	if err := cfg.Validate(); err != nil {
		return nil, err
	}
	cfg = cfg.withDefaults()

	var be backend
	if cfg.TestMode {
		be = newStubBackend(cfg, nil)
	} else {
		// Real mode today: persistent stub backed by cryptostore.
		// When the real embedded Headscale lands, swap this branch
		// for the real backend constructor.
		store, err := openEncryptedStateStore(cfg)
		if err != nil {
			return nil, fmt.Errorf("headscale: open state store: %w", err)
		}
		be = newStubBackend(cfg, store)
	}
	return &Coordinator{cfg: cfg, backend: be}, nil
}

// Start brings the coordinator online. It is idempotent-unsafe: a
// second call returns ErrAlreadyStarted. Start blocks until the
// coordinator is accepting connections or ctx is cancelled.
func (c *Coordinator) Start(ctx context.Context) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.stopped {
		return ErrStopped
	}
	if c.started {
		return ErrAlreadyStarted
	}
	addr, err := c.backend.start(ctx)
	if err != nil {
		return fmt.Errorf("headscale: start backend: %w", err)
	}
	c.addr = addr
	c.started = true
	c.logf("coordinator started addr=%s namespace=%s", addr, c.cfg.Namespace)
	return nil
}

// Shutdown stops the coordinator and releases resources. It is safe
// to call multiple times; subsequent calls are no-ops.
func (c *Coordinator) Shutdown(ctx context.Context) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	if !c.started || c.stopped {
		c.stopped = true
		return nil
	}
	err := c.backend.shutdown(ctx)
	c.stopped = true
	c.logf("coordinator shutdown err=%v", err)
	return err
}

// MintPreAuthKey creates a pre-auth key the pairing flow (KAI-243)
// hands to a tsnet client. ttl controls how long the key is valid
// before the coordinator rejects it. An empty namespace is invalid.
func (c *Coordinator) MintPreAuthKey(ctx context.Context, namespace string, ttl time.Duration) (string, error) {
	if namespace == "" {
		return "", ErrEmptyNamespaceArg
	}
	if err := c.requireStarted(); err != nil {
		return "", err
	}
	return c.backend.mintPreAuthKey(ctx, namespace, ttl)
}

// ListNodes returns the nodes currently registered with the
// coordinator. The order is implementation-defined; callers should
// sort by ID if they need determinism.
func (c *Coordinator) ListNodes(ctx context.Context) ([]NodeInfo, error) {
	if err := c.requireStarted(); err != nil {
		return nil, err
	}
	return c.backend.listNodes(ctx)
}

// RevokeNode removes a node from the coordinator, ending its
// membership in the tailnet. Returns ErrUnknownNode if no node with
// that ID exists.
func (c *Coordinator) RevokeNode(ctx context.Context, nodeID string) error {
	if err := c.requireStarted(); err != nil {
		return err
	}
	return c.backend.revokeNode(ctx, nodeID)
}

// Addr returns the control URL tsnet clients should dial. In test
// mode it is a synthetic loopback URL; in real mode it is the
// ServerURL configured on the Config (or the loopback bind if
// ServerURL was left empty). Addr is only meaningful after Start.
func (c *Coordinator) Addr() string {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.cfg.ServerURL != "" {
		return c.cfg.ServerURL
	}
	if c.addr == "" {
		return ""
	}
	return "http://" + c.addr
}

// Healthy reports whether the coordinator is up and accepting work.
// Directory admin surfaces use this as their readiness probe.
func (c *Coordinator) Healthy() bool {
	c.mu.Lock()
	defer c.mu.Unlock()
	if !c.started || c.stopped {
		return false
	}
	return c.backend.healthy()
}

func (c *Coordinator) requireStarted() error {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.stopped {
		return ErrStopped
	}
	if !c.started {
		return ErrNotStarted
	}
	return nil
}

func (c *Coordinator) logf(format string, args ...any) {
	if c.cfg.Logf != nil {
		c.cfg.Logf(format, args...)
	}
}

// generatePreAuthKey returns a realistic-looking pre-auth key. The
// prefix "hskey-auth-" mirrors the headscale convention so the rest
// of the codebase sees identical shapes in stub and real modes.
func generatePreAuthKey() (string, error) {
	var b [24]byte
	if _, err := rand.Read(b[:]); err != nil {
		return "", fmt.Errorf("headscale: rand: %w", err)
	}
	return "hskey-auth-" + hex.EncodeToString(b[:]), nil
}
