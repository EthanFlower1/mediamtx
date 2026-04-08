// Package pairing implements the single-token Recorder enrollment flow
// described in the Kaivue multi-recording-server design §13.4 (KAI-243).
//
// # Token format
//
// A PairingToken is serialized as a compact, self-describing signed blob, NOT
// a JWT. The format is:
//
//	base64url( JSON(PairingToken) ) + "." + base64url( Ed25519Signature )
//
// The signing key is the Directory's ed25519 root key (the same key used by
// the embedded step-ca in internal/directory/pki/stepca). We derive a
// dedicated pairing sub-key via HKDF-SHA256 from the root private key bytes
// using the info string "kaivue-pairing-token-v1". This scopes the signature
// to the pairing domain without needing a second key material store.
//
// # Package boundaries (depguard, KAI-236)
//
// This package MUST NOT import internal/recorder/... .
// It MAY import internal/shared/... and the sibling on-prem packages
// internal/directory/pki/... and internal/directory/mesh/... .
//
// # KAI-244 seam
//
// The Redeem function is the seam for KAI-244's /api/v1/pairing/check-in
// endpoint. That handler will call pairing.Redeem(ctx, store, tokenID) after
// verifying the inbound Recorder's identity. This package does not implement
// that check-in endpoint — only Redeem is provided.
package pairing
