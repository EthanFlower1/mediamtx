package tsnet

import (
	"context"
	"errors"
	"io"
	"net"
	"strings"
	"testing"
	"time"
)

func TestValidateHostname(t *testing.T) {
	cases := []struct {
		name    string
		host    string
		wantErr bool
	}{
		{"ok simple", "recorder-abc123", false},
		{"ok single letter", "a", false},
		{"ok max length", strings.Repeat("a", 63), false},
		{"empty", "", true},
		{"too long", strings.Repeat("a", 64), true},
		{"uppercase rejected", "Recorder", true},
		{"leading hyphen", "-foo", true},
		{"trailing hyphen", "foo-", true},
		{"underscore", "foo_bar", true},
		{"dot", "foo.bar", true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			err := validateHostname(tc.host)
			if tc.wantErr && err == nil {
				t.Fatalf("expected error for %q", tc.host)
			}
			if !tc.wantErr && err != nil {
				t.Fatalf("unexpected error for %q: %v", tc.host, err)
			}
		})
	}
}

func TestValidateAuthKey(t *testing.T) {
	cases := []struct {
		key     string
		wantErr bool
	}{
		{"", true},
		{"short", true},
		{"tskey-auth-abcd", false},
		{"hskey-abcd1234", false},
		{"opaque-but-long-enough-1234", false},
	}
	for _, tc := range cases {
		if err := validateAuthKey(tc.key); (err != nil) != tc.wantErr {
			t.Fatalf("validateAuthKey(%q): wantErr=%v, err=%v", tc.key, tc.wantErr, err)
		}
	}
}

func TestNewTestModeSkipsAuthKeyAndStateDir(t *testing.T) {
	n, err := New(NodeConfig{Hostname: uniqueHost(t), TestMode: true})
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	t.Cleanup(func() { _ = n.Shutdown(context.Background()) })
	if !n.Addr().Is4() {
		t.Fatalf("expected IPv4 synthetic addr, got %v", n.Addr())
	}
	if got, want := n.Addr().As4()[0], byte(127); got != want {
		t.Fatalf("synthetic addr must be loopback, got %v", n.Addr())
	}
}

func TestNewRealModeRequiresAuthKeyAndStateDir(t *testing.T) {
	_, err := New(NodeConfig{Hostname: "valid-host"})
	if err == nil {
		t.Fatal("expected error for real-mode node with no auth key")
	}
	if !errors.Is(err, ErrInvalidAuthKey) {
		t.Fatalf("expected ErrInvalidAuthKey, got %v", err)
	}

	_, err = New(NodeConfig{Hostname: "valid-host", AuthKey: "tskey-auth-abcd1234"})
	if err == nil || !strings.Contains(err.Error(), "StateDir") {
		t.Fatalf("expected StateDir required error, got %v", err)
	}
}

func TestTestModeListenAcceptsConnection(t *testing.T) {
	n, err := New(NodeConfig{Hostname: uniqueHost(t), TestMode: true})
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	t.Cleanup(func() { _ = n.Shutdown(context.Background()) })

	ln, err := n.Listen("tcp", ":0")
	if err != nil {
		t.Fatalf("Listen: %v", err)
	}

	errCh := make(chan error, 1)
	go func() {
		conn, err := ln.Accept()
		if err != nil {
			errCh <- err
			return
		}
		defer conn.Close()
		_, _ = conn.Write([]byte("hello"))
		errCh <- nil
	}()

	c, err := net.Dial("tcp", ln.Addr().String())
	if err != nil {
		t.Fatalf("dial listener: %v", err)
	}
	defer c.Close()
	buf, err := io.ReadAll(c)
	if err != nil {
		t.Fatalf("read: %v", err)
	}
	if string(buf) != "hello" {
		t.Fatalf("got %q, want %q", buf, "hello")
	}
	if err := <-errCh; err != nil {
		t.Fatalf("accept: %v", err)
	}
}

func TestTestModeDialUnknownHost(t *testing.T) {
	n, err := New(NodeConfig{Hostname: uniqueHost(t), TestMode: true})
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	t.Cleanup(func() { _ = n.Shutdown(context.Background()) })

	_, err = n.Dial(context.Background(), "tcp", "does-not-exist:80")
	if !errors.Is(err, ErrUnknownHost) {
		t.Fatalf("expected ErrUnknownHost, got %v", err)
	}
}

func TestTestModeDialBetweenTwoNodes(t *testing.T) {
	serverHost := uniqueHost(t) + "-a"
	clientHost := uniqueHost(t) + "-b"

	server, err := New(NodeConfig{Hostname: serverHost, TestMode: true})
	if err != nil {
		t.Fatalf("New server: %v", err)
	}
	t.Cleanup(func() { _ = server.Shutdown(context.Background()) })

	client, err := New(NodeConfig{Hostname: clientHost, TestMode: true})
	if err != nil {
		t.Fatalf("New client: %v", err)
	}
	t.Cleanup(func() { _ = client.Shutdown(context.Background()) })

	ln, err := server.Listen("tcp", ":0")
	if err != nil {
		t.Fatalf("Listen: %v", err)
	}

	done := make(chan struct{})
	go func() {
		defer close(done)
		c, err := ln.Accept()
		if err != nil {
			return
		}
		defer c.Close()
		_, _ = c.Write([]byte("ping"))
	}()

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	conn, err := client.Dial(ctx, "tcp", serverHost+":443")
	if err != nil {
		t.Fatalf("Dial: %v", err)
	}
	defer conn.Close()
	buf := make([]byte, 4)
	if _, err := io.ReadFull(conn, buf); err != nil {
		t.Fatalf("read: %v", err)
	}
	if string(buf) != "ping" {
		t.Fatalf("got %q", buf)
	}
	<-done
}

func TestShutdownClosesListener(t *testing.T) {
	n, err := New(NodeConfig{Hostname: uniqueHost(t), TestMode: true})
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	ln, err := n.Listen("tcp", ":0")
	if err != nil {
		t.Fatalf("Listen: %v", err)
	}
	if err := n.Shutdown(context.Background()); err != nil {
		t.Fatalf("Shutdown: %v", err)
	}
	// After shutdown the listener must reject Accept.
	if _, err := ln.Accept(); err == nil {
		t.Fatal("expected listener to be closed after Shutdown")
	}
	// And the node should be gone from the registry.
	if _, err := n.Dial(context.Background(), "tcp", "anything:80"); !errors.Is(err, ErrClosed) {
		t.Fatalf("expected ErrClosed after Shutdown, got %v", err)
	}
}

func TestDuplicateHostnameRejected(t *testing.T) {
	host := uniqueHost(t)
	n1, err := New(NodeConfig{Hostname: host, TestMode: true})
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	t.Cleanup(func() { _ = n1.Shutdown(context.Background()) })

	if _, err := New(NodeConfig{Hostname: host, TestMode: true}); err == nil {
		t.Fatal("expected duplicate hostname to be rejected")
	}
}

// uniqueHost returns a DNS-safe hostname unique to the current test.
// It lowercases the test name and replaces unsupported characters so
// the result passes validateHostname.
func uniqueHost(t *testing.T) string {
	t.Helper()
	name := strings.ToLower(t.Name())
	var b strings.Builder
	for _, r := range name {
		switch {
		case r >= 'a' && r <= 'z', r >= '0' && r <= '9':
			b.WriteRune(r)
		default:
			b.WriteRune('-')
		}
	}
	s := strings.Trim(b.String(), "-")
	if len(s) > 50 {
		s = s[len(s)-50:]
		s = strings.TrimLeft(s, "-")
	}
	return s
}
