package cryptostore

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/hkdf"
	"crypto/rand"
	"crypto/sha256"
	"errors"
	"fmt"
)

// Format constants for the on-disk ciphertext envelope.
const (
	// FormatVersionReserved (0x00) is intentionally unused so that a
	// zero-filled blob never decodes as a valid ciphertext.
	FormatVersionReserved byte = 0x00
	// FormatVersionV1 is the current AES-256-GCM + HKDF-SHA256 format.
	FormatVersionV1 byte = 0x01

	// KeySize is the length in bytes of an AES-256 subkey.
	KeySize = 32
	// NonceSize is the length in bytes of the GCM nonce (standard 96-bit).
	NonceSize = 12
	// TagSize is the length in bytes of the GCM auth tag.
	TagSize = 16
	// HeaderSize is the length of the version+nonce prefix.
	HeaderSize = 1 + NonceSize
)

// Well-known HKDF info strings used throughout the NVR.
const (
	InfoRTSPCredentials = "rtsp-credentials"
	InfoFaceVault       = "face-vault"
	InfoPairingTokens   = "pairing-tokens"
	InfoFederationRoot  = "federation-root"
	InfoZitadelBootstrap = "zitadel-bootstrap"
)

// Sentinel errors. Tests and callers should use errors.Is to check them.
var (
	ErrInvalidKey        = errors.New("cryptostore: invalid key length")
	ErrInvalidCiphertext = errors.New("cryptostore: ciphertext too short or malformed")
	ErrUnsupportedVersion = errors.New("cryptostore: unsupported ciphertext version")
	ErrAuthFailed        = errors.New("cryptostore: authentication failed")
	ErrEmptyInfo         = errors.New("cryptostore: info string must not be empty")
	ErrEmptyMaster       = errors.New("cryptostore: master key must not be empty")
)

// Cryptostore is the minimal column-level encryption interface. It is stable
// and other packages (e.g. internal/recorder/state for KAI-250) may depend on
// it without coupling to the concrete AES-GCM implementation.
type Cryptostore interface {
	// Encrypt returns a version-tagged AES-256-GCM envelope over plaintext.
	Encrypt(plaintext []byte) ([]byte, error)
	// Decrypt parses the envelope, authenticates it, and returns the plaintext.
	Decrypt(ciphertext []byte) ([]byte, error)
	// RotateKey decrypts ciphertext with oldKey and re-encrypts it with newKey.
	// oldKey and newKey are raw 32-byte AES-256 subkeys (already derived).
	RotateKey(oldKey, newKey []byte) error
}

// aesGCM is the default AES-256-GCM implementation of Cryptostore.
// The struct stores a single derived subkey, not the master key.
type aesGCM struct {
	key []byte // 32 bytes
	aead cipher.AEAD
}

// New constructs an AES-256-GCM Cryptostore from an already-derived 32-byte subkey.
// Callers that hold a master key should prefer NewFromMaster.
func New(subkey []byte) (Cryptostore, error) {
	if len(subkey) != KeySize {
		return nil, fmt.Errorf("%w: got %d bytes, want %d", ErrInvalidKey, len(subkey), KeySize)
	}
	block, err := aes.NewCipher(subkey)
	if err != nil {
		return nil, fmt.Errorf("cryptostore: aes.NewCipher: %w", err)
	}
	aead, err := cipher.NewGCM(block)
	if err != nil {
		return nil, fmt.Errorf("cryptostore: cipher.NewGCM: %w", err)
	}
	// Copy key so callers can zero their buffer without affecting us.
	keyCopy := make([]byte, KeySize)
	copy(keyCopy, subkey)
	return &aesGCM{key: keyCopy, aead: aead}, nil
}

// NewFromMaster derives a purpose-specific subkey from master via HKDF-SHA256
// and returns a Cryptostore bound to that subkey. info must be non-empty and
// should be a stable well-known string (see Info* constants) because changing
// it makes previously encrypted data undecryptable.
//
// salt may be nil, in which case HKDF uses a zero-filled salt. The master key
// itself is not retained.
func NewFromMaster(master, salt []byte, info string) (Cryptostore, error) {
	if len(master) == 0 {
		return nil, ErrEmptyMaster
	}
	if info == "" {
		return nil, ErrEmptyInfo
	}
	subkey, err := DeriveSubkey(master, salt, info)
	if err != nil {
		return nil, err
	}
	return New(subkey)
}

// DeriveSubkey derives a deterministic 32-byte AES-256 subkey from master with
// HKDF-SHA256. Exposed for callers that need to rotate keys or manage multiple
// Cryptostore instances from the same master.
func DeriveSubkey(master, salt []byte, info string) ([]byte, error) {
	if len(master) == 0 {
		return nil, ErrEmptyMaster
	}
	if info == "" {
		return nil, ErrEmptyInfo
	}
	return hkdf.Key(sha256.New, master, salt, info, KeySize)
}

// Encrypt implements Cryptostore.
func (a *aesGCM) Encrypt(plaintext []byte) ([]byte, error) {
	nonce := make([]byte, NonceSize)
	if _, err := rand.Read(nonce); err != nil {
		return nil, fmt.Errorf("cryptostore: rand: %w", err)
	}
	// Envelope: version | nonce | ciphertext||tag
	out := make([]byte, 0, HeaderSize+len(plaintext)+TagSize)
	out = append(out, FormatVersionV1)
	out = append(out, nonce...)
	out = a.aead.Seal(out, nonce, plaintext, versionAAD(FormatVersionV1))
	return out, nil
}

// Decrypt implements Cryptostore.
func (a *aesGCM) Decrypt(ciphertext []byte) ([]byte, error) {
	if len(ciphertext) < HeaderSize+TagSize {
		return nil, ErrInvalidCiphertext
	}
	version := ciphertext[0]
	switch version {
	case FormatVersionV1:
		// ok
	case FormatVersionReserved:
		return nil, fmt.Errorf("%w: 0x00 is reserved", ErrUnsupportedVersion)
	default:
		return nil, fmt.Errorf("%w: 0x%02x", ErrUnsupportedVersion, version)
	}
	nonce := ciphertext[1 : 1+NonceSize]
	sealed := ciphertext[1+NonceSize:]
	plaintext, err := a.aead.Open(nil, nonce, sealed, versionAAD(version))
	if err != nil {
		return nil, fmt.Errorf("%w: %v", ErrAuthFailed, err)
	}
	return plaintext, nil
}

// RotateKey is a no-op on the aesGCM struct itself — key rotation is a
// per-value or per-column operation. This method exists so callers can rotate
// a single in-memory cryptostore instance, but the practical API for batch
// rotation is RotateValue / RotateColumn.
//
// The contract: callers pass raw 32-byte subkeys (already derived from the
// old and new master keys via DeriveSubkey). This method rebuilds the
// underlying AEAD so future Encrypt/Decrypt calls use newKey. The data stored
// in databases must be rotated separately with RotateValue or RotateColumn.
func (a *aesGCM) RotateKey(oldKey, newKey []byte) error {
	if len(oldKey) != KeySize || len(newKey) != KeySize {
		return fmt.Errorf("%w: rotate requires 32-byte keys", ErrInvalidKey)
	}
	// Verify oldKey matches the currently installed key so callers catch
	// mismatches early.
	if !constantTimeEqual(a.key, oldKey) {
		return fmt.Errorf("%w: old key does not match current key", ErrInvalidKey)
	}
	block, err := aes.NewCipher(newKey)
	if err != nil {
		return fmt.Errorf("cryptostore: aes.NewCipher: %w", err)
	}
	aead, err := cipher.NewGCM(block)
	if err != nil {
		return fmt.Errorf("cryptostore: cipher.NewGCM: %w", err)
	}
	a.aead = aead
	keyCopy := make([]byte, KeySize)
	copy(keyCopy, newKey)
	a.key = keyCopy
	return nil
}

// RotateValue decrypts a single envelope with oldStore and re-encrypts it
// with newStore. This is the primitive used by RotateColumn.
func RotateValue(oldStore, newStore Cryptostore, ciphertext []byte) ([]byte, error) {
	pt, err := oldStore.Decrypt(ciphertext)
	if err != nil {
		return nil, fmt.Errorf("rotate: decrypt: %w", err)
	}
	out, err := newStore.Encrypt(pt)
	if err != nil {
		return nil, fmt.Errorf("rotate: encrypt: %w", err)
	}
	// Best-effort scrub of plaintext.
	for i := range pt {
		pt[i] = 0
	}
	return out, nil
}

// versionAAD binds the version byte into the AEAD as additional authenticated
// data, so swapping the version prefix on a forged envelope causes auth to
// fail even if the attacker controls nothing else.
func versionAAD(v byte) []byte {
	return []byte{v}
}

// constantTimeEqual compares two byte slices of equal length without early
// return. Used to check key equality in RotateKey.
func constantTimeEqual(a, b []byte) bool {
	if len(a) != len(b) {
		return false
	}
	var v byte
	for i := range a {
		v |= a[i] ^ b[i]
	}
	return v == 0
}
