package headscale

import "time"

// NodeInfo is the public view of a node registered with the
// coordinator. It is intentionally small — the Directory admin UI and
// the pairing flow only need identity and lifecycle timestamps.
type NodeInfo struct {
	// ID is the stable coordinator-side identifier for the node.
	ID string

	// Hostname is the DNS-safe label the node registered with.
	Hostname string

	// Namespace is the tailnet namespace the node belongs to. Today
	// this is always DefaultNamespace (one per site) but the field is
	// plumbed through so a future multi-site deployment does not need
	// to break the API.
	Namespace string

	// IPv4 is the stable tailnet IPv4 address assigned to the node,
	// e.g. "100.64.0.7". Empty until the node completes its first
	// handshake.
	IPv4 string

	// CreatedAt is when the node first registered.
	CreatedAt time.Time

	// LastSeen is the timestamp of the most recent keep-alive.
	LastSeen time.Time
}
