// Package nvr implements the NVR subsystem for MediaMTX.
package nvr

import (
	"crypto/rand"
	"crypto/rsa"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"fmt"
	"os"
	"path/filepath"

	"github.com/bluenviron/mediamtx/internal/nvr/crypto"
	"github.com/bluenviron/mediamtx/internal/nvr/db"
	"github.com/bluenviron/mediamtx/internal/nvr/yamlwriter"
)

// NVR is the main NVR subsystem struct.
type NVR struct {
	DatabasePath string
	JWTSecret    string
	ConfigPath   string

	database   *db.DB
	yamlWriter *yamlwriter.Writer
	privateKey *rsa.PrivateKey
	jwksJSON   []byte
}

// Initialize sets up the NVR subsystem: auto-generates JWTSecret if empty,
// creates the DB directory, opens the database, creates the YAML writer,
// and loads or generates RSA keys.
func (n *NVR) Initialize() error {
	if n.JWTSecret == "" {
		secret := make([]byte, 32)
		if _, err := rand.Read(secret); err != nil {
			return fmt.Errorf("generate JWT secret: %w", err)
		}
		n.JWTSecret = hex.EncodeToString(secret)
	}

	dbDir := filepath.Dir(n.DatabasePath)
	if err := os.MkdirAll(dbDir, 0o755); err != nil {
		return fmt.Errorf("create database directory: %w", err)
	}

	var err error
	n.database, err = db.Open(n.DatabasePath)
	if err != nil {
		return fmt.Errorf("open database: %w", err)
	}

	n.yamlWriter = yamlwriter.New(n.ConfigPath)

	if err := n.loadOrGenerateKeys(); err != nil {
		n.database.Close()
		return fmt.Errorf("load or generate keys: %w", err)
	}

	return nil
}

// Close closes the NVR subsystem.
func (n *NVR) Close() {
	if n.database != nil {
		n.database.Close()
	}
}

// IsSetupRequired returns true if no users exist in the database.
func (n *NVR) IsSetupRequired() bool {
	count, err := n.database.CountUsers()
	if err != nil {
		return true
	}
	return count == 0
}

// DB returns the database handle.
func (n *NVR) DB() *db.DB {
	return n.database
}

// JWKSJSON returns the JWKS JSON document.
func (n *NVR) JWKSJSON() []byte {
	return n.jwksJSON
}

// PrivateKey returns the RSA private key.
func (n *NVR) PrivateKey() *rsa.PrivateKey {
	return n.privateKey
}

// loadOrGenerateKeys derives an encryption key, then loads or generates
// RSA keys from the database config table.
func (n *NVR) loadOrGenerateKeys() error {
	encKey := crypto.DeriveKey(n.JWTSecret, "nvr-rsa-key-encryption")

	encPrivB64, err := n.database.GetConfig("rsa_private_key")
	if errors.Is(err, db.ErrNotFound) {
		// Generate new RSA key pair.
		privPEM, pubPEM, err := crypto.GenerateRSAKeyPair()
		if err != nil {
			return fmt.Errorf("generate RSA key pair: %w", err)
		}

		// Encrypt private key and store as base64.
		encPriv, err := crypto.Encrypt(encKey, privPEM)
		if err != nil {
			return fmt.Errorf("encrypt private key: %w", err)
		}
		encPrivB64 = base64.StdEncoding.EncodeToString(encPriv)

		if err := n.database.SetConfig("rsa_private_key", encPrivB64); err != nil {
			return fmt.Errorf("store private key: %w", err)
		}
		if err := n.database.SetConfig("rsa_public_key", base64.StdEncoding.EncodeToString(pubPEM)); err != nil {
			return fmt.Errorf("store public key: %w", err)
		}
	} else if err != nil {
		return fmt.Errorf("load private key: %w", err)
	}

	// Decrypt private key.
	encPriv, err := base64.StdEncoding.DecodeString(encPrivB64)
	if err != nil {
		return fmt.Errorf("decode private key: %w", err)
	}
	privPEM, err := crypto.Decrypt(encKey, encPriv)
	if err != nil {
		return fmt.Errorf("decrypt private key: %w", err)
	}
	n.privateKey, err = crypto.ParseRSAPrivateKey(privPEM)
	if err != nil {
		return fmt.Errorf("parse private key: %w", err)
	}

	// Load public key and generate JWKS.
	pubB64, err := n.database.GetConfig("rsa_public_key")
	if err != nil {
		return fmt.Errorf("load public key: %w", err)
	}
	pubPEM, err := base64.StdEncoding.DecodeString(pubB64)
	if err != nil {
		return fmt.Errorf("decode public key: %w", err)
	}
	n.jwksJSON, err = crypto.JWKSFromPublicKey(pubPEM)
	if err != nil {
		return fmt.Errorf("generate JWKS: %w", err)
	}

	return nil
}
