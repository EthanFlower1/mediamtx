package pairing

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/ed25519"
	"crypto/rand"
	"crypto/sha256"
	"crypto/x509"
	"encoding/pem"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"

	"golang.org/x/crypto/hkdf"
)

const (
	// KeystoreDefaultDir is where the device keypair lives on a production Recorder.
	KeystoreDefaultDir = "/var/lib/mediamtx-recorder"

	// deviceKeyFile is the filename for the encrypted device private key.
	deviceKeyFile = "device.key.enc"

	// pemTypeEncKey is the PEM block type written to disk.
	pemTypeEncKey = "KAIVUE ENCRYPTED PRIVATE KEY"

	// hkdfInfoDevice scopes the encryption sub-key to the device-key domain.
	hkdfInfoDevice = "kaivue-recorder-device-key-v1"

	// gcmNonceSize is the standard GCM nonce length.
	gcmNonceSize = 12
)

// Keystore persists an encrypted Ed25519 device keypair to disk. The keypair
// is stable across reboots; the encryption key is derived from the
// Headscale-issued machine key material so the sealed blob is tied to this
// machine's tailnet identity.
//
// Encryption: AES-256-GCM. Nonce is prepended to ciphertext. Key is derived
// via HKDF-SHA256 from the caller-supplied master material.
type Keystore struct {
	dir string
}

// NewKeystore constructs a Keystore rooted at dir. dir is created with
// 0700 permissions if it does not exist.
func NewKeystore(dir string) (*Keystore, error) {
	if dir == "" {
		dir = KeystoreDefaultDir
	}
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return nil, fmt.Errorf("keystore: mkdir %q: %w", dir, err)
	}
	return &Keystore{dir: dir}, nil
}

// LoadOrGenerate loads the device keypair from disk, or generates and persists
// a fresh one if none exists. masterMaterial is used to derive the
// encryption key; it should be stable and machine-specific (e.g. the raw
// bytes of the Headscale machine key or a site-specific seed).
func (ks *Keystore) LoadOrGenerate(masterMaterial []byte) (ed25519.PrivateKey, error) {
	path := filepath.Join(ks.dir, deviceKeyFile)

	// Try to load an existing key first.
	if _, err := os.Stat(path); err == nil {
		priv, err := ks.load(path, masterMaterial)
		if err != nil {
			return nil, fmt.Errorf("keystore: load: %w", err)
		}
		return priv, nil
	}

	// Generate a new keypair.
	_, priv, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		return nil, fmt.Errorf("keystore: generate: %w", err)
	}
	if err := ks.save(path, priv, masterMaterial); err != nil {
		return nil, fmt.Errorf("keystore: save: %w", err)
	}
	return priv, nil
}

// save serializes and seals priv to path atomically.
func (ks *Keystore) save(path string, priv ed25519.PrivateKey, masterMaterial []byte) error {
	der, err := x509.MarshalPKCS8PrivateKey(priv)
	if err != nil {
		return fmt.Errorf("marshal: %w", err)
	}
	defer func() {
		for i := range der {
			der[i] = 0
		}
	}()

	encKey, err := deriveEncKey(masterMaterial)
	if err != nil {
		return err
	}
	defer func() {
		for i := range encKey {
			encKey[i] = 0
		}
	}()

	sealed, err := sealGCM(encKey, der)
	if err != nil {
		return fmt.Errorf("seal: %w", err)
	}

	block := pem.EncodeToMemory(&pem.Block{Type: pemTypeEncKey, Bytes: sealed})

	// Atomic write: temp file + rename.
	tmp, err := os.CreateTemp(ks.dir, ".device.key.*")
	if err != nil {
		return fmt.Errorf("tempfile: %w", err)
	}
	tmpName := tmp.Name()
	if err := tmp.Chmod(0o600); err != nil {
		tmp.Close()
		os.Remove(tmpName)
		return err
	}
	if _, err := tmp.Write(block); err != nil {
		tmp.Close()
		os.Remove(tmpName)
		return err
	}
	if err := tmp.Sync(); err != nil {
		tmp.Close()
		os.Remove(tmpName)
		return err
	}
	if err := tmp.Close(); err != nil {
		os.Remove(tmpName)
		return err
	}
	return os.Rename(tmpName, path)
}

// load reads and decrypts the device keypair from path.
func (ks *Keystore) load(path string, masterMaterial []byte) (ed25519.PrivateKey, error) {
	pemData, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read: %w", err)
	}
	block, _ := pem.Decode(pemData)
	if block == nil || block.Type != pemTypeEncKey {
		return nil, errors.New("malformed PEM block")
	}

	encKey, err := deriveEncKey(masterMaterial)
	if err != nil {
		return nil, err
	}
	defer func() {
		for i := range encKey {
			encKey[i] = 0
		}
	}()

	der, err := openGCM(encKey, block.Bytes)
	if err != nil {
		return nil, fmt.Errorf("open: %w", err)
	}
	defer func() {
		for i := range der {
			der[i] = 0
		}
	}()

	raw, err := x509.ParsePKCS8PrivateKey(der)
	if err != nil {
		return nil, fmt.Errorf("parse pkcs8: %w", err)
	}
	priv, ok := raw.(ed25519.PrivateKey)
	if !ok {
		return nil, fmt.Errorf("unexpected key type %T", raw)
	}
	return priv, nil
}

// deriveEncKey derives a 32-byte AES-256 encryption key from masterMaterial
// using HKDF-SHA256 with domain label hkdfInfoDevice.
func deriveEncKey(masterMaterial []byte) ([]byte, error) {
	if len(masterMaterial) == 0 {
		return nil, errors.New("keystore: masterMaterial must be non-empty")
	}
	h := hkdf.New(sha256.New, masterMaterial, nil, []byte(hkdfInfoDevice))
	key := make([]byte, 32)
	if _, err := io.ReadFull(h, key); err != nil {
		return nil, fmt.Errorf("hkdf: %w", err)
	}
	return key, nil
}

// sealGCM encrypts plaintext with AES-256-GCM. Returns nonce || ciphertext.
func sealGCM(key, plaintext []byte) ([]byte, error) {
	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, err
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return nil, err
	}
	nonce := make([]byte, gcmNonceSize)
	if _, err := rand.Read(nonce); err != nil {
		return nil, err
	}
	ct := gcm.Seal(nonce, nonce, plaintext, nil)
	return ct, nil
}

// openGCM decrypts AES-256-GCM ciphertext produced by sealGCM.
func openGCM(key, data []byte) ([]byte, error) {
	if len(data) < gcmNonceSize {
		return nil, errors.New("ciphertext too short")
	}
	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, err
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return nil, err
	}
	nonce := data[:gcmNonceSize]
	ct := data[gcmNonceSize:]
	return gcm.Open(nil, nonce, ct, nil)
}
