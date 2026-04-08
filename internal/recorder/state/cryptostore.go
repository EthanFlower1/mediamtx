package state

import (
	"context"

	sharedcrypto "github.com/bluenviron/mediamtx/internal/shared/cryptostore"
)

// Cryptostore is the ctx-aware interface the Recorder state cache depends on.
//
// It is deliberately a superset of internal/shared/cryptostore.Cryptostore
// (which is pure — crypto operations don't block on I/O). The context
// parameter is reserved for future implementations that may need it
// (e.g. KMS-backed keys, audit-log sinks). Tests in this package use a
// stub implementation (see store_test.go) and production Recorders
// inject the real shared cryptostore via FromShared().
//
// Encrypt takes plaintext (e.g. an RTSP password) and returns an opaque
// ciphertext blob that is safe to persist at rest. Decrypt reverses it.
// Implementations are expected to include authentication / AEAD so that
// any tampering with the stored blob surfaces as a decryption error.
type Cryptostore interface {
	Encrypt(ctx context.Context, plaintext []byte) ([]byte, error)
	Decrypt(ctx context.Context, ciphertext []byte) ([]byte, error)
}

// FromShared wraps a shared cryptostore (which has no ctx) so it
// satisfies the state package's ctx-aware Cryptostore interface. This
// is the bridge between the KAI-251 production cryptostore and the
// KAI-250 Recorder state cache; the ctx parameter is ignored by the
// underlying AES-256-GCM primitive but kept in the signature so
// future KMS- or audit-backed implementations can use it.
func FromShared(s sharedcrypto.Cryptostore) Cryptostore {
	return &sharedAdapter{inner: s}
}

type sharedAdapter struct {
	inner sharedcrypto.Cryptostore
}

func (a *sharedAdapter) Encrypt(_ context.Context, plaintext []byte) ([]byte, error) {
	return a.inner.Encrypt(plaintext)
}

func (a *sharedAdapter) Decrypt(_ context.Context, ciphertext []byte) ([]byte, error) {
	return a.inner.Decrypt(ciphertext)
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
