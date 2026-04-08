package mesh

import (
	"context"
	"strings"
	"testing"
)

func TestNewRecorderNodePrefixesHostname(t *testing.T) {
	n, err := New(context.Background(), Config{
		ComponentID: "xyz789",
		TestMode:    true,
	})
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	defer n.Shutdown(context.Background())

	if !strings.HasPrefix(n.Hostname(), RoleHostnamePrefix) {
		t.Fatalf("hostname %q missing role prefix", n.Hostname())
	}
	if !strings.HasSuffix(n.Hostname(), "xyz789") {
		t.Fatalf("hostname %q missing component id", n.Hostname())
	}
}
