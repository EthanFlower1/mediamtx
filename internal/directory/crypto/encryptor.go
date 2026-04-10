// Package crypto provides column-level encryption for sensitive fields in the
// Directory's SQLite database. It uses AES-256-GCM with HKDF-SHA256 key
// derivation to produce per-column keys from a single master secret.
package crypto

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"fmt"
	"io"

	"golang.org/x/crypto/hkdf"
)

const (
	keyLen   = 32 // AES-256
	nonceLen = 12 // GCM standard nonce
)

// ColumnEncryptor encrypts and decrypts individual column values using
// AES-256-GCM. Each column gets a distinct derived key via HKDF-SHA256
// so that compromising one column's ciphertext reveals nothing about another.
type ColumnEncryptor struct {
	gcm  cipher.AEAD
	info string // column identifier used in HKDF derivation
}

// NewColumnEncryptor derives a per-column AES-256-GCM key from masterSecret
// using HKDF-SHA256 with the given columnInfo string (e.g. "rtsp_username").
func NewColumnEncryptor(masterSecret []byte, columnInfo string) (*ColumnEncryptor, error) {
	if len(masterSecret) == 0 {
		return nil, fmt.Errorf("crypto: master secret is empty")
	}
	if columnInfo == "" {
		return nil, fmt.Errorf("crypto: column info is empty")
	}

	// HKDF: extract-then-expand with SHA-256.
	// Salt is nil (HKDF uses a zero-salt internally).
	reader := hkdf.New(sha256.New, masterSecret, nil, []byte(columnInfo))
	key := make([]byte, keyLen)
	if _, err := io.ReadFull(reader, key); err != nil {
		return nil, fmt.Errorf("crypto: hkdf derive: %w", err)
	}

	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, fmt.Errorf("crypto: new cipher: %w", err)
	}

	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return nil, fmt.Errorf("crypto: new gcm: %w", err)
	}

	return &ColumnEncryptor{gcm: gcm, info: columnInfo}, nil
}

// Encrypt encrypts plaintext and returns a base64-encoded string suitable for
// storage in a TEXT column. Format: base64(nonce || ciphertext || tag).
func (e *ColumnEncryptor) Encrypt(plaintext string) (string, error) {
	nonce := make([]byte, nonceLen)
	if _, err := io.ReadFull(rand.Reader, nonce); err != nil {
		return "", fmt.Errorf("crypto: generate nonce: %w", err)
	}

	// Seal appends ciphertext+tag to nonce.
	sealed := e.gcm.Seal(nonce, nonce, []byte(plaintext), nil)
	return base64.StdEncoding.EncodeToString(sealed), nil
}

// Decrypt decodes a base64-encoded ciphertext (as produced by Encrypt) and
// returns the original plaintext.
func (e *ColumnEncryptor) Decrypt(encoded string) (string, error) {
	data, err := base64.StdEncoding.DecodeString(encoded)
	if err != nil {
		return "", fmt.Errorf("crypto: base64 decode: %w", err)
	}

	if len(data) < nonceLen+e.gcm.Overhead() {
		return "", fmt.Errorf("crypto: ciphertext too short")
	}

	nonce := data[:nonceLen]
	ciphertext := data[nonceLen:]

	plaintext, err := e.gcm.Open(nil, nonce, ciphertext, nil)
	if err != nil {
		return "", fmt.Errorf("crypto: decrypt: %w", err)
	}

	return string(plaintext), nil
}

// CredentialEncryptor holds per-column encryptors for RTSP username and password.
type CredentialEncryptor struct {
	Username *ColumnEncryptor
	Password *ColumnEncryptor
}

// NewCredentialEncryptor creates encryptors for both RTSP credential columns.
func NewCredentialEncryptor(masterSecret []byte) (*CredentialEncryptor, error) {
	usernameEnc, err := NewColumnEncryptor(masterSecret, "rtsp_username")
	if err != nil {
		return nil, fmt.Errorf("crypto: username encryptor: %w", err)
	}

	passwordEnc, err := NewColumnEncryptor(masterSecret, "rtsp_password")
	if err != nil {
		return nil, fmt.Errorf("crypto: password encryptor: %w", err)
	}

	return &CredentialEncryptor{
		Username: usernameEnc,
		Password: passwordEnc,
	}, nil
}

// EncryptCredentials encrypts a username/password pair, returning base64 ciphertexts.
func (ce *CredentialEncryptor) EncryptCredentials(username, password string) (encUsername, encPassword string, err error) {
	encUsername, err = ce.Username.Encrypt(username)
	if err != nil {
		return "", "", err
	}
	encPassword, err = ce.Password.Encrypt(password)
	if err != nil {
		return "", "", err
	}
	return encUsername, encPassword, nil
}

// DecryptCredentials decrypts a username/password pair from base64 ciphertexts.
func (ce *CredentialEncryptor) DecryptCredentials(encUsername, encPassword string) (username, password string, err error) {
	username, err = ce.Username.Decrypt(encUsername)
	if err != nil {
		return "", "", err
	}
	password, err = ce.Password.Decrypt(encPassword)
	if err != nil {
		return "", "", err
	}
	return username, password, nil
}
