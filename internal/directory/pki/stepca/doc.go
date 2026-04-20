// Package stepca implements the per-site cluster Certificate Authority used
// by the Kaivue Directory to issue mTLS leaf certificates for internal
// Directory <-> Recorder <-> Gateway communication.
//
// Design summary
//
// On first start, the Directory bootstraps a self-signed Ed25519 root
// certificate ("Kaivue Site Root CA"). The root private key is sealed at
// rest with the cryptostore seam (KAI-251) using the HKDF label
// cryptostore.InfoFederationRoot derived from the installation's
// nvrJWTSecret master key. The root public certificate is stored next to it
// in PEM form so it can be served to paired clients without unsealing the
// private key.
//
// Leaves are issued with a 24h validity window (rotation handled by KAI-242)
// and stored to disk for audit only — the CA itself does not maintain a
// long-term database. The SHA-256 fingerprint of the root is surfaced via
// Fingerprint() so it can be embedded in pairing tokens (KAI-243), letting
// clients pin the site root the first time they enroll.
//
// FIPS boundary
//
// All private-key sealing flows through internal/shared/cryptostore, which
// is the single place the project will swap to a FIPS-validated provider in
// KAI-388. No raw AEAD or KDF primitives live in this package.
//
// Smallstep vs. stripped-down fallback
//
// The ticket (KAI-241) permits either embedding github.com/smallstep/certificates
// directly or using crypto/x509 for a stripped-down implementation. This
// package ships the stripped-down variant: it avoids the multi-hundred-megabyte
// dependency closure that smallstep/certificates pulls in (badger, cloudkms,
// pkcs11, step-cli, etc.), which is incompatible with the Raikada build
// footprint target. See README.md for details.
package stepca
