// Package directory is the cloud-aware Directory subsystem of the Kaivue
// Recording Server.
//
// The Directory role owns the on-prem control plane for a single tenant:
//
//   - Camera and recorder enrollment, pairing, and lifecycle management.
//   - Persistent inventory of cameras, recorders, users, and sites.
//   - Stream URL minting and access policy enforcement for clients on the
//     local network and across the federated mesh.
//   - Synchronization of identity, policy, and metadata with the Kaivue cloud
//     when connectivity is available, and graceful local-only operation when
//     it is not.
//   - Serving the management UI surfaces (admin console, REST/Connect APIs)
//     used by operators on-prem.
//
// Boundary rules (enforced by depguard, see .golangci.yml):
//
//   - Code under internal/directory MUST NOT import code under
//     internal/recorder. The two roles communicate exclusively via the
//     Connect-Go services defined in internal/shared/proto.
//   - Code under internal/directory MAY import code under internal/shared
//     for shared types, protos, and primitives.
//
// During the incremental migration from internal/nvr, this package starts
// as a skeleton; subsystems will be moved here one at a time. Existing code
// in internal/nvr continues to function unchanged until each migration step
// lands. See docs/architecture/package-layout.md for the full plan.
package directory
