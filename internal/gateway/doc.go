// Package gateway implements the Gateway role: an authenticated, MediaMTX-fronted
// streaming proxy that bridges off-LAN clients to upstream Recorders over the
// tsnet mesh.
//
// # Role overview
//
// In the Kaivue topology there are three peer roles, each living in its own
// internal/* package and sharing only via internal/shared:
//
//   - Directory  (internal/directory)  — control plane / inventory
//   - Recorder   (internal/recorder)   — captures, stores, and serves video
//   - Gateway    (internal/gateway)    — auth-gated streaming proxy (this pkg)
//
// The Gateway is co-resident with the Directory in v1 (KAI-261). It is built
// to also run as a sibling process; the only coupling to the Directory is
// through the [RecorderResolver] interface, which a future task wires up to
// the Directory's mesh + camera registry.
//
// # Request flow
//
//  1. Off-LAN client requests a stream URL from the Directory.
//  2. Directory consults the routing decision logic (KAI-258) and, when the
//     LAN path is unavailable, mints a stream JWT scoped to a Gateway.
//  3. Client connects to the Gateway with the stream JWT.
//  4. The Gateway's [Service] verifies the JWT (via internal/shared/streamclaims),
//     resolves the camera-id -> Recorder mesh address (via [RecorderResolver]),
//     and rewrites the request to point at the upstream MediaMTX path on the
//     resolved Recorder.
//  5. A co-resident MediaMTX sidecar (managed via internal/shared/sidecar)
//     serves the actual media bytes; the sidecar's path config is rendered
//     dynamically with `source:` URLs that point at the upstream Recorders
//     over the mesh.
//
// # Boundaries
//
// This package depends only on standard library + internal/shared/*. It does
// NOT import internal/directory or internal/recorder. Cross-role data must
// arrive via the interfaces defined in interfaces.go (RecorderResolver,
// StreamVerifier) which the lead-onprem composition layer wires up.
//
// KAI-258 (LAN-vs-Gateway routing) and KAI-260 (MediaMTX auth webhook
// handler) are owned by sibling tickets. This package depends on them only
// through interfaces and ships with thin in-memory fakes for tests.
package gateway
