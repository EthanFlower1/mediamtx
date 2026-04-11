// Package federation implements the FederationPeerService, which exposes
// Directory-to-Directory RPCs over Connect-Go with mTLS authentication,
// and the federation pairing lifecycle (KAI-269).
//
// # FederationPeerService (KAI-464 + KAI-465 + KAI-466)
//
// KAI-464 laid the scaffold with Ping and GetJWKS. KAI-465 adds the three
// catalog RPCs (ListUsers, ListGroups, ListCameras), each filtered by Casbin
// grants for the requesting peer using the "federation:<peer_directory_id>"
// subject prefix. KAI-466 implements SearchRecordings and MintStreamURL,
// scoped by the requesting peer's identity and enforced via Casbin.
//
// # Token format (KAI-269)
//
// A federation pairing token is a "FED-" prefixed string wrapping a signed
// PeerEnrollmentToken from the federation CA. The format is:
//
//	FED-v1.<base64url(JSON payload)>.<base64url(Ed25519 signature)>
//
// The FED- prefix gives UX recognizability in admin dashboards and clipboard
// workflows. The v1 version tag allows future format evolution.
//
// # Lifecycle
//
//  1. Admin on the founding Directory clicks "+ Invite another Directory"
//  2. The service mints a FED-v1 token with a 60-minute TTL
//  3. The token is stored in federation_tokens (status='pending')
//  4. Admin copies the token and sends it out-of-band to the peer site admin
//  5. Peer admin pastes the token into their Directory's "Join Federation" form
//  6. POST /api/v1/federation/join triggers the handshake:
//     a. Decode + verify the enrollment token
//     b. Atomically redeem (single-use enforcement)
//     c. Exchange JWKS public key sets
//     d. Write the peer into federation_members
//  7. Both Directories can now verify each other's JWTs and establish mTLS
//
// # Single-use and TTL enforcement
//
// Single-use is enforced at the database level via an atomic
// UPDATE ... WHERE status='pending' AND expires_at > now() pattern,
// identical to the recorder pairing store (internal/directory/pairing).
package federation
