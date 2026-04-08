package headscale

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func testCtx(t *testing.T) context.Context {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	t.Cleanup(cancel)
	return ctx
}

func newTestCoordinator(t *testing.T) *Coordinator {
	t.Helper()
	c, err := New(Config{TestMode: true})
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	t.Cleanup(func() { _ = c.Shutdown(context.Background()) })
	return c
}

func TestNewRequiresMasterKeyInRealMode(t *testing.T) {
	_, err := New(Config{StateDir: t.TempDir()})
	if !errors.Is(err, ErrMissingMasterKey) {
		t.Fatalf("want ErrMissingMasterKey, got %v", err)
	}
}

func TestNewTestModeSkipsMasterKey(t *testing.T) {
	if _, err := New(Config{TestMode: true}); err != nil {
		t.Fatalf("New test-mode: %v", err)
	}
}

func TestConfigValidateNamespace(t *testing.T) {
	cases := map[string]bool{
		"kaivue-site": true,
		"site1":       true,
		"":            true, // empty gets defaulted
		"Bad Name":    false,
		"UPPER":       false,
		"-leading":    false,
	}
	for ns, ok := range cases {
		err := Config{TestMode: true, Namespace: ns}.Validate()
		if ok && err != nil {
			t.Errorf("namespace %q: want ok, got %v", ns, err)
		}
		if !ok && !errors.Is(err, ErrInvalidNamespace) {
			t.Errorf("namespace %q: want ErrInvalidNamespace, got %v", ns, err)
		}
	}
}

func TestBootstrapHappyPath(t *testing.T) {
	c := newTestCoordinator(t)
	ctx := testCtx(t)

	if err := c.Start(ctx); err != nil {
		t.Fatalf("Start: %v", err)
	}
	if !c.Healthy() {
		t.Fatal("expected Healthy() after Start")
	}
	if c.Addr() == "" {
		t.Fatal("expected non-empty Addr()")
	}

	key, err := c.MintPreAuthKey(ctx, DefaultNamespace, time.Hour)
	if err != nil {
		t.Fatalf("MintPreAuthKey: %v", err)
	}
	if key == "" {
		t.Fatal("expected non-empty pre-auth key")
	}
	if !strings.HasPrefix(key, "hskey-auth-") {
		t.Errorf("unexpected key prefix: %q", key)
	}

	if err := c.Shutdown(ctx); err != nil {
		t.Fatalf("Shutdown: %v", err)
	}
	if c.Healthy() {
		t.Fatal("expected Healthy()==false after Shutdown")
	}
}

func TestMintPreAuthKeyRejectsEmptyNamespace(t *testing.T) {
	c := newTestCoordinator(t)
	ctx := testCtx(t)
	if err := c.Start(ctx); err != nil {
		t.Fatalf("Start: %v", err)
	}
	_, err := c.MintPreAuthKey(ctx, "", time.Hour)
	if !errors.Is(err, ErrEmptyNamespaceArg) {
		t.Fatalf("want ErrEmptyNamespaceArg, got %v", err)
	}
}

func TestMintPreAuthKeyBeforeStart(t *testing.T) {
	c := newTestCoordinator(t)
	_, err := c.MintPreAuthKey(context.Background(), DefaultNamespace, time.Hour)
	if !errors.Is(err, ErrNotStarted) {
		t.Fatalf("want ErrNotStarted, got %v", err)
	}
}

func TestListNodesReturnsRegisteredNodes(t *testing.T) {
	c := newTestCoordinator(t)
	ctx := testCtx(t)
	if err := c.Start(ctx); err != nil {
		t.Fatalf("Start: %v", err)
	}
	nodes := []NodeInfo{
		{ID: "n1", Hostname: "recorder-1", IPv4: "100.64.0.2"},
		{ID: "n2", Hostname: "recorder-2", IPv4: "100.64.0.3"},
	}
	for _, n := range nodes {
		if err := c.RegisterTestNode(n); err != nil {
			t.Fatalf("RegisterTestNode %s: %v", n.ID, err)
		}
	}
	got, err := c.ListNodes(ctx)
	if err != nil {
		t.Fatalf("ListNodes: %v", err)
	}
	if len(got) != 2 {
		t.Fatalf("want 2 nodes, got %d", len(got))
	}
	if got[0].ID != "n1" || got[1].ID != "n2" {
		t.Errorf("unexpected node order: %+v", got)
	}
	// Namespace is defaulted from config when empty on input.
	for _, n := range got {
		if n.Namespace != DefaultNamespace {
			t.Errorf("node %s namespace = %q, want %q", n.ID, n.Namespace, DefaultNamespace)
		}
	}
}

func TestRevokeNodeRemovesNode(t *testing.T) {
	c := newTestCoordinator(t)
	ctx := testCtx(t)
	if err := c.Start(ctx); err != nil {
		t.Fatalf("Start: %v", err)
	}
	if err := c.RegisterTestNode(NodeInfo{ID: "n1", Hostname: "r1"}); err != nil {
		t.Fatalf("register: %v", err)
	}
	if err := c.RevokeNode(ctx, "n1"); err != nil {
		t.Fatalf("RevokeNode: %v", err)
	}
	nodes, err := c.ListNodes(ctx)
	if err != nil {
		t.Fatalf("ListNodes: %v", err)
	}
	if len(nodes) != 0 {
		t.Fatalf("want 0 nodes after revoke, got %d", len(nodes))
	}
	// Revoking again returns ErrUnknownNode.
	if err := c.RevokeNode(ctx, "n1"); !errors.Is(err, ErrUnknownNode) {
		t.Fatalf("want ErrUnknownNode, got %v", err)
	}
}

func TestDoubleStartRejected(t *testing.T) {
	c := newTestCoordinator(t)
	ctx := testCtx(t)
	if err := c.Start(ctx); err != nil {
		t.Fatalf("Start 1: %v", err)
	}
	if err := c.Start(ctx); !errors.Is(err, ErrAlreadyStarted) {
		t.Fatalf("want ErrAlreadyStarted, got %v", err)
	}
}

func TestShutdownIdempotent(t *testing.T) {
	c := newTestCoordinator(t)
	ctx := testCtx(t)
	if err := c.Start(ctx); err != nil {
		t.Fatalf("Start: %v", err)
	}
	if err := c.Shutdown(ctx); err != nil {
		t.Fatalf("Shutdown 1: %v", err)
	}
	if err := c.Shutdown(ctx); err != nil {
		t.Fatalf("Shutdown 2: %v", err)
	}
}

// TestStatePersistsAcrossRestart exercises the real-mode code path
// with a master key but in a temp dir. It proves:
//   - bootstrap writes an encrypted state file
//   - a fresh Coordinator loads the namespace and nodes back
//   - the file on disk is NOT plaintext JSON (encryption works)
func TestStatePersistsAcrossRestart(t *testing.T) {
	dir := t.TempDir()
	master := []byte("test-master-key-do-not-use-in-prod-32b")
	base := Config{
		StateDir:  dir,
		MasterKey: master,
		// Explicit non-default namespace so we can assert load.
		Namespace: "site-alpha",
	}

	// Boot 1 — bootstrap + register a node.
	c1, err := New(base)
	if err != nil {
		t.Fatalf("New 1: %v", err)
	}
	if err := c1.Start(testCtx(t)); err != nil {
		t.Fatalf("Start 1: %v", err)
	}
	if err := c1.RegisterTestNode(NodeInfo{ID: "persist-1", Hostname: "r"}); err != nil {
		t.Fatalf("register: %v", err)
	}
	if err := c1.Shutdown(testCtx(t)); err != nil {
		t.Fatalf("Shutdown 1: %v", err)
	}

	// File should exist and NOT be plaintext JSON.
	raw := mustReadFile(t, filepath.Join(dir, stateFileName))
	if len(raw) == 0 {
		t.Fatal("state file is empty")
	}
	if raw[0] == '{' {
		t.Fatal("state file appears to be plaintext JSON — encryption failed")
	}

	// Boot 2 — new Coordinator over the same dir.
	c2, err := New(base)
	if err != nil {
		t.Fatalf("New 2: %v", err)
	}
	if err := c2.Start(testCtx(t)); err != nil {
		t.Fatalf("Start 2: %v", err)
	}
	t.Cleanup(func() { _ = c2.Shutdown(context.Background()) })

	nodes, err := c2.ListNodes(testCtx(t))
	if err != nil {
		t.Fatalf("ListNodes: %v", err)
	}
	if len(nodes) != 1 || nodes[0].ID != "persist-1" {
		t.Fatalf("unexpected nodes after restart: %+v", nodes)
	}
}

func TestStateDecryptFailsWithWrongMasterKey(t *testing.T) {
	dir := t.TempDir()
	good := []byte("master-key-one-xxxxxxxxxxxxxxxxxxxxxx")
	bad := []byte("master-key-two-xxxxxxxxxxxxxxxxxxxxxx")

	c1, err := New(Config{StateDir: dir, MasterKey: good})
	if err != nil {
		t.Fatalf("New 1: %v", err)
	}
	if err := c1.Start(testCtx(t)); err != nil {
		t.Fatalf("Start 1: %v", err)
	}
	if err := c1.Shutdown(testCtx(t)); err != nil {
		t.Fatalf("Shutdown 1: %v", err)
	}

	c2, err := New(Config{StateDir: dir, MasterKey: bad})
	if err != nil {
		t.Fatalf("New 2: %v", err)
	}
	if err := c2.Start(testCtx(t)); err == nil {
		t.Fatal("expected Start to fail with wrong master key")
	}
}

func mustReadFile(t *testing.T, path string) []byte {
	t.Helper()
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read %s: %v", path, err)
	}
	return b
}
