// Package cryptostore provides column-level encryption for sensitive data at
// rest (RTSP credentials, face vault entries, pairing tokens, federation root
// keys, etc.).
//
// # Overview
//
// A Cryptostore wraps an AES-256-GCM cipher whose key is derived from a master
// key via HKDF-SHA256 with a purpose-specific info string. Distinct columns
// ("rtsp-credentials", "face-vault", "pairing-tokens", ...) derive different
// subkeys from the same master, so a compromise of a single subkey does not
// cascade and callers never handle raw master key material.
//
// # Ciphertext format
//
//	version_byte | nonce (12 bytes) | ciphertext | gcm_tag (16 bytes)
//
// The leading version byte enables future format migrations without
// ambiguity. Version 0x01 is the current format. Version 0x00 is reserved
// and intentionally rejected so accidentally zeroed blobs surface as errors.
//
// # Master key
//
// The master key is sourced from the existing `nvrJWTSecret` field in
// mediamtx.yml — callers pass that byte slice into NewFromMaster. Cryptostore
// never reads the config file itself and never stores the master key beyond
// the lifetime of the derivation call.
//
// # Key rotation
//
// RotateKey re-encrypts a single value from an old master/key to a new one.
// RotateColumn performs batch re-encryption of an entire SQL column, suitable
// for background migration jobs driven by the NVR operator.
//
// # FIPS boundary (see KAI-388)
//
// This package deliberately uses only Go standard library cryptography
// (crypto/aes, crypto/cipher, crypto/hkdf, crypto/rand, crypto/sha256). No
// third-party crypto primitives are imported. When the Go toolchain is built
// in FIPS-140 mode (GOEXPERIMENT=boringcrypto or the Go 1.24+ native FIPS
// module), all primitives used here route through the validated module
// without code changes.
package cryptostore
