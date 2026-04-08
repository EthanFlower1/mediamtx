package state

import "context"

// Cryptostore is the subset of the internal/shared/cryptostore interface
// (KAI-251) that the Recorder state cache depends on.
//
// The real implementation lives in internal/shared/cryptostore and is
// constructed once at Recorder boot and injected into the Store. Tests
// in this package use a stub implementation (see store_test.go).
//
// Encrypt takes plaintext (e.g. an RTSP password) and returns an opaque
// ciphertext blob that is safe to persist at rest. Decrypt reverses it.
// Implementations are expected to include authentication / AEAD so that
// any tampering with the stored blob surfaces as a decryption error.
type Cryptostore interface {
	Encrypt(ctx context.Context, plaintext []byte) ([]byte, error)
	Decrypt(ctx context.Context, ciphertext []byte) ([]byte, error)
}

// NoopCryptostore is a passthrough implementation used when the cache
// is opened without a real cryptostore (e.g. in local dev). It does
// NOT encrypt — it only exists so the Store has a non-nil dependency.
// Production Recorders MUST inject a real Cryptostore.
type NoopCryptostore struct{}

// Encrypt returns plaintext unchanged.
func (NoopCryptostore) Encrypt(_ context.Context, plaintext []byte) ([]byte, error) {
	if plaintext == nil {
		return nil, nil
	}
	out := make([]byte, len(plaintext))
	copy(out, plaintext)
	return out, nil
}

// Decrypt returns ciphertext unchanged.
func (NoopCryptostore) Decrypt(_ context.Context, ciphertext []byte) ([]byte, error) {
	if ciphertext == nil {
		return nil, nil
	}
	out := make([]byte, len(ciphertext))
	copy(out, ciphertext)
	return out, nil
}
