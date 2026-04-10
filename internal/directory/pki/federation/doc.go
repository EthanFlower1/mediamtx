// Package federation implements the Federation Certificate Authority used by
// the Kaivue Directory for cross-site Directory <-> Directory mTLS.
//
// This is a separate PKI domain from the per-site cluster CA
// (internal/directory/pki/stepca). The cluster CA secures communication
// within a single site (Directory <-> Recorder <-> Gateway). The federation
// CA secures communication between peer Directory instances across sites.
//
// # Modes
//
// Air-gapped mode: the founding Directory bootstraps its own self-signed
// Ed25519 federation root CA. Each peer Directory that joins the federation
// receives a leaf certificate signed by this root (via a peer enrollment
// token that embeds the federation CA fingerprint).
//
// Cloud-connected mode: a cloud identity service provisions federation
// credentials. The CloudCAProvider interface abstracts this; the Directory
// delegates to the cloud provider instead of self-signing.
//
// # Security model
//
// The federation root private key is sealed at rest using the cryptostore
// seam (internal/shared/cryptostore) with HKDF label "federation-ca-root".
// This is distinct from the cluster CA's label ("federation-root") to ensure
// complete key separation between the two PKI domains.
//
// Peer enrollment tokens embed the federation CA fingerprint (SHA-256 of
// root DER) so joining peers can trust-on-first-use the federation root.
//
// # Downstream consumers
//
// KAI-269 (Federation pairing token) — uses MintPeerEnrollmentToken
// KAI-464 (FederationPeer proto) — uses IssuePeerCert for mTLS identity
package federation
