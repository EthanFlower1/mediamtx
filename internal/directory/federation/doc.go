// Package federation implements the FederationPeerService — the Directory-to-
// Directory gRPC service that lets a federated integrator Directory search
// recordings and mint stream URLs on a customer Directory.
//
// Every RPC is scoped by the requesting peer's identity (extracted from mTLS +
// bearer token) and enforced via Casbin (see internal/shared/permissions).
//
// KAI-464 created the service scaffold; KAI-466 implements SearchRecordings
// and MintStreamURL.
package federation
