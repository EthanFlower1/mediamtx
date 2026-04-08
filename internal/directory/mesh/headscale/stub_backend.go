package headscale

import (
	"context"
	"encoding/json"
	"fmt"
	"net"
	"sort"
	"sync"
	"time"
)

// stubBackend is an in-memory coordinator that behaves like a real
// Headscale coordinator from the caller's perspective: it mints keys,
// tracks nodes, and — when a stateStore is wired — persists its state
// encrypted across restarts via internal/shared/cryptostore.
//
// It exists to let the rest of the codebase (KAI-239 tsnet, KAI-243
// pairing, KAI-246 sidecar) build against a stable surface while we
// sort out the real embedded Headscale. See coordinator.go's TODO.
type stubBackend struct {
	cfg   Config
	store stateStore // may be nil in TestMode

	mu        sync.Mutex
	running   bool
	listener  net.Listener
	addr      string
	state     persistedState
	preAuthKS map[string]preAuthKey // live, un-consumed keys
}

// persistedState is the on-disk shape of the stub backend's state.
// Keep the fields stable; bumping the schema requires a migration.
type persistedState struct {
	Version   int                 `json:"version"`
	Namespace string              `json:"namespace"`
	CreatedAt time.Time           `json:"created_at"`
	Nodes     map[string]NodeInfo `json:"nodes"`
}

type preAuthKey struct {
	Namespace string
	ExpiresAt time.Time
}

const stubStateVersion = 1

func newStubBackend(cfg Config, store stateStore) *stubBackend {
	return &stubBackend{
		cfg:   cfg,
		store: store,
		state: persistedState{
			Version:   stubStateVersion,
			Namespace: cfg.Namespace,
			Nodes:     make(map[string]NodeInfo),
		},
		preAuthKS: make(map[string]preAuthKey),
	}
}

func (s *stubBackend) start(_ context.Context) (string, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	// Load persisted state if we have a store wired.
	if s.store != nil {
		loaded, ok, err := s.store.load()
		if err != nil {
			return "", fmt.Errorf("load persisted state: %w", err)
		}
		if ok {
			if loaded.Nodes == nil {
				loaded.Nodes = make(map[string]NodeInfo)
			}
			// Namespace in persisted state wins — it was chosen on
			// first bootstrap and changing it mid-life is a data
			// hazard.
			s.state = loaded
		} else {
			// First boot: persist the namespace decision.
			s.state.CreatedAt = time.Now().UTC()
			if err := s.store.save(s.state); err != nil {
				return "", fmt.Errorf("bootstrap persisted state: %w", err)
			}
		}
	}

	// Bind a loopback listener so Addr() returns something a tsnet
	// test harness could plausibly dial. We do not serve anything on
	// it — this backend is a stand-in — but holding the socket is
	// honest about resource ownership.
	ln, err := net.Listen("tcp", s.cfg.ListenAddr)
	if err != nil {
		return "", fmt.Errorf("listen %s: %w", s.cfg.ListenAddr, err)
	}
	s.listener = ln
	s.addr = ln.Addr().String()
	s.running = true
	return s.addr, nil
}

func (s *stubBackend) shutdown(_ context.Context) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if !s.running {
		return nil
	}
	s.running = false
	if s.listener != nil {
		_ = s.listener.Close()
		s.listener = nil
	}
	return nil
}

func (s *stubBackend) mintPreAuthKey(_ context.Context, namespace string, ttl time.Duration) (string, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if !s.running {
		return "", ErrNotStarted
	}
	key, err := generatePreAuthKey()
	if err != nil {
		return "", err
	}
	expires := time.Time{}
	if ttl > 0 {
		expires = time.Now().UTC().Add(ttl)
	}
	s.preAuthKS[key] = preAuthKey{Namespace: namespace, ExpiresAt: expires}
	return key, nil
}

func (s *stubBackend) listNodes(_ context.Context) ([]NodeInfo, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if !s.running {
		return nil, ErrNotStarted
	}
	out := make([]NodeInfo, 0, len(s.state.Nodes))
	for _, n := range s.state.Nodes {
		out = append(out, n)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].ID < out[j].ID })
	return out, nil
}

func (s *stubBackend) revokeNode(_ context.Context, nodeID string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if !s.running {
		return ErrNotStarted
	}
	if _, ok := s.state.Nodes[nodeID]; !ok {
		return ErrUnknownNode
	}
	delete(s.state.Nodes, nodeID)
	if s.store != nil {
		if err := s.store.save(s.state); err != nil {
			return fmt.Errorf("persist after revoke: %w", err)
		}
	}
	return nil
}

func (s *stubBackend) healthy() bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.running
}

// registerTestNode is a helper for tests and for the Directory admin
// flow to inject a pretend node into the stub. It is package-private:
// callers reach it via Coordinator.registerTestNode below.
func (s *stubBackend) registerTestNode(n NodeInfo) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if !s.running {
		return ErrNotStarted
	}
	if n.ID == "" {
		return fmt.Errorf("register: empty node id")
	}
	if n.Namespace == "" {
		n.Namespace = s.state.Namespace
	}
	if n.CreatedAt.IsZero() {
		n.CreatedAt = time.Now().UTC()
	}
	if n.LastSeen.IsZero() {
		n.LastSeen = n.CreatedAt
	}
	s.state.Nodes[n.ID] = n
	if s.store != nil {
		if err := s.store.save(s.state); err != nil {
			return fmt.Errorf("persist after register: %w", err)
		}
	}
	return nil
}

// RegisterTestNode is a package-public escape hatch used by sibling
// packages' tests (notably KAI-239 tsnet integration tests) to pretend
// a node has paired without running the full pairing flow. It is a
// no-op in any mode other than the stub-backend configuration that
// this package currently always uses.
func (c *Coordinator) RegisterTestNode(n NodeInfo) error {
	c.mu.Lock()
	be := c.backend
	c.mu.Unlock()
	if stub, ok := be.(*stubBackend); ok {
		return stub.registerTestNode(n)
	}
	return fmt.Errorf("headscale: RegisterTestNode not supported by active backend")
}

// encodeState marshals persistedState to JSON. Split out so the
// cryptostore-backed store can reuse the same encoding without
// learning the struct layout.
func encodeState(ps persistedState) ([]byte, error) {
	return json.Marshal(ps)
}

func decodeState(b []byte) (persistedState, error) {
	var ps persistedState
	if err := json.Unmarshal(b, &ps); err != nil {
		return persistedState{}, err
	}
	if ps.Nodes == nil {
		ps.Nodes = make(map[string]NodeInfo)
	}
	return ps, nil
}
