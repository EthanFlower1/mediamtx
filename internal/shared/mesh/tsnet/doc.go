// Package tsnet wraps tailscale.com/tsnet so that the Kaivue
// Directory, Recorder, and Gateway binaries can participate as nodes
// on a customer's private, embedded Headscale-coordinated tailnet.
//
// The package exposes a minimal Node abstraction with four operations
// used across the on-prem platform:
//
//   - Listen: accept inbound Connect-Go / HTTP traffic from other
//     mesh nodes.
//   - Dial:   connect to another mesh node by hostname
//     (e.g. "directory.tailnet.local").
//   - Addr:   report the stable 100.x.y.z tailnet address.
//   - Shutdown: gracefully stop the node.
//
// # Test mode
//
// Real tsnet needs a state directory, a reachable control server, and
// raw network access, which makes it a poor fit for hermetic unit
// tests. Passing NodeConfig.TestMode = true bypasses the real tsnet
// engine entirely and returns an in-memory loopback Node backed by a
// process-wide registry keyed on Hostname. Two test-mode Nodes created
// in the same process can Dial each other by hostname without touching
// the network.
//
// # Build tags
//
// Real tsnet depends on gvisor and the wireguard-go userspace stack,
// which compile fine on all first-tier Go platforms but noticeably
// bloat the build. The realtsnet build tag gates smoke tests that
// actually bring up a tsnet.Server against a live control URL; CI
// skips them by default. See README.md for how to run them locally.
//
// # Dependencies
//
//   - KAI-240 supplies the embedded Headscale coordinator and the
//     ControlURL value. Until it lands, callers can pass the empty
//     string to fall back to tsnet's default (login.tailscale.com),
//     which is only useful for local smoke tests.
//   - KAI-243 mints per-component pre-auth keys from pairing tokens
//     and is responsible for populating NodeConfig.AuthKey. This
//     package performs only shallow format validation of the key.
package tsnet
