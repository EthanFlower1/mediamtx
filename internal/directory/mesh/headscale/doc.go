// Package headscale runs the per-site tailnet coordinator that every
// other component on a customer's network registers against.
//
// # Role
//
// Exactly one instance of this package runs inside the Directory binary.
// It owns:
//
//   - Bootstrapping a single-namespace tailnet on first start.
//   - Persisting the coordinator's state (nodes, pre-auth keys) on disk,
//     encrypted at rest via internal/shared/cryptostore using the master
//     key already supplied by mediamtx.yml's nvrJWTSecret (see CLAUDE.md
//     — we do not add a new secret field).
//   - Minting short-lived pre-auth keys for pairing flows (KAI-243).
//   - Listing and revoking nodes so Directory admin surfaces can manage
//     mesh membership.
//   - Exposing a control URL that tsnet clients (KAI-239) dial to join.
//
// # Embedding strategy (important)
//
// The upstream Headscale project (github.com/juanfont/headscale) does
// not publish a stable Go API for in-process embedding: most of its
// control-plane machinery lives under internal/hscontrol and its public
// surface assumes a long-running process driven by configuration files
// and command-line flags. Historical attempts to import it as a library
// have been fragile — every minor release tends to break package paths
// and require adapter rewrites.
//
// To unblock the rest of wave 2 (KAI-239 tsnet client, KAI-241 step-ca,
// KAI-243 pairing token, KAI-246 sidecar supervisor) without burning
// the full week budgeted in the spec for "Headscale complications",
// this package ships:
//
//  1. A stable Coordinator interface that represents the coordinator
//     contract the rest of the codebase is allowed to depend on.
//  2. A stub/fake in-memory implementation that satisfies the interface
//     end-to-end. It generates realistic-looking pre-auth keys, tracks
//     nodes, and persists its state (AES-GCM encrypted via cryptostore)
//     so restarts preserve the namespace and any registered nodes.
//  3. A TestMode flag that keeps the fake purely in-memory for unit
//     tests (no disk, no cryptostore, no filesystem side effects).
//
// The hook for wiring a real embedded Headscale is a single constructor
// call in New(); see the TODO near the top of coordinator.go and the
// README.
//
// # Namespace-per-site
//
// Kaivue runs one Headscale namespace per customer site, not per
// tenant. A single customer may have multiple tenants but they share
// the on-prem mesh — tenancy separation happens above the mesh via
// mTLS identity (KAI-241) and RBAC, not via separate tailnets. The
// default namespace name is "kaivue-site".
//
// # Test-mode escape hatch
//
// Config.TestMode = true short-circuits every disk and network side
// effect: the coordinator runs fully in-memory, MintPreAuthKey returns
// deterministic-looking keys, and no cryptostore is required. This is
// what unit tests across the codebase should use — see
// coordinator_test.go for the baseline.
package headscale
