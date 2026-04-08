package mesh

import (
	"context"
	"strings"
	"testing"
)

func TestNewDirectoryNodePrefixesHostname(t *testing.T) {
	n, err := New(context.Background(), Config{
		ComponentID: "abc123",
		TestMode:    true,
	})
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	defer n.Shutdown(context.Background())

	if !strings.HasPrefix(n.Hostname(), RoleHostnamePrefix) {
		t.Fatalf("hostname %q missing role prefix", n.Hostname())
	}
	if !strings.HasSuffix(n.Hostname(), "abc123") {
		t.Fatalf("hostname %q missing component id", n.Hostname())
	}
}
