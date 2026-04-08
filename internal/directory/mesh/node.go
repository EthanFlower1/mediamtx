// Package mesh provides the Directory-role convenience wrapper around
// internal/shared/mesh/tsnet. It exists so that Directory callers
// never import the Recorder's mesh package (and vice versa), in
// keeping with seam #1 of the package-boundary linter (KAI-236).
package mesh

import (
	"context"

	sharedtsnet "github.com/bluenviron/mediamtx/internal/shared/mesh/tsnet"
)

// RoleHostnamePrefix is prepended to the caller-supplied component
// identifier to produce the mesh hostname for a Directory node.
const RoleHostnamePrefix = "directory-"

// Config is the Directory-specific subset of tsnet.NodeConfig. The
// caller provides the component identifier (typically the Directory
// UUID); this package prefixes it with "directory-" to produce a
// mesh hostname and forwards the rest of the fields unchanged.
type Config struct {
	ComponentID string
	AuthKey     string
	StateDir    string
	ControlURL  string
	Logf        func(format string, args ...any)
	TestMode    bool
}

// New constructs a Directory mesh node. The returned value is a
// shared *tsnet.Node; callers Listen/Dial/Shutdown it directly.
//
// The ctx parameter is reserved for future use (e.g. cancelling a
// slow initial coordinator handshake) and is currently unused.
func New(_ context.Context, cfg Config) (*sharedtsnet.Node, error) {
	return sharedtsnet.New(sharedtsnet.NodeConfig{
		Hostname:   RoleHostnamePrefix + cfg.ComponentID,
		AuthKey:    cfg.AuthKey,
		StateDir:   cfg.StateDir,
		ControlURL: cfg.ControlURL,
		Logf:       cfg.Logf,
		TestMode:   cfg.TestMode,
	})
}
