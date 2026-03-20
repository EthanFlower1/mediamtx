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
	"strings"
	"time"

	"github.com/gin-gonic/gin"

	"github.com/bluenviron/mediamtx/internal/nvr/api"
	"github.com/bluenviron/mediamtx/internal/nvr/crypto"
	"github.com/bluenviron/mediamtx/internal/nvr/db"
	"github.com/bluenviron/mediamtx/internal/nvr/onvif"
	"github.com/bluenviron/mediamtx/internal/nvr/yamlwriter"
)

// NVR is the main NVR subsystem struct.
type NVR struct {
	DatabasePath string
	JWTSecret    string
	ConfigPath   string
	APIAddress   string

	database   *db.DB
	yamlWriter *yamlwriter.Writer
	privateKey *rsa.PrivateKey
	jwksJSON   []byte
	discovery  *onvif.Discovery
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

		// Persist the generated secret to the config file so it survives restarts.
		if n.ConfigPath != "" {
			w := yamlwriter.New(n.ConfigPath)
			if err := w.SetTopLevelValue("nvrJWTSecret", n.JWTSecret); err != nil {
				return fmt.Errorf("persist JWT secret: %w", err)
			}
		}
	}

	// Expand ~ to the user's home directory.
	if strings.HasPrefix(n.DatabasePath, "~/") {
		if home, err := os.UserHomeDir(); err == nil {
			n.DatabasePath = filepath.Join(home, n.DatabasePath[2:])
		}
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
	n.discovery = onvif.NewDiscovery()

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

// RegisterRoutes registers NVR API routes on the given gin engine.
func (n *NVR) RegisterRoutes(engine *gin.Engine, version string) {
	api.RegisterRoutes(engine, &api.RouterConfig{
		DB:           n.database,
		PrivateKey:   n.privateKey,
		JWKSJSON:     n.jwksJSON,
		YAMLWriter:   n.yamlWriter,
		Version:      version,
		Discovery:    n.discovery,
		APIAddress:   n.APIAddress,
		SetupChecker: n,
	})
}

// OnSegmentComplete is called when a recording segment finishes writing.
// It matches recorder.OnSegmentCompleteFunc: func(path string, duration time.Duration).
func (n *NVR) OnSegmentComplete(filePath string, duration time.Duration) {
	// Find camera by checking if any camera's mediamtx_path is in the file path.
	cameras, err := n.database.ListCameras()
	if err != nil {
		return
	}

	var cam *db.Camera
	for _, c := range cameras {
		if c.MediaMTXPath != "" && strings.Contains(filePath, c.MediaMTXPath) {
			cam = c
			break
		}
	}
	if cam == nil {
		return
	}

	var fileSize int64
	if info, err := os.Stat(filePath); err == nil {
		fileSize = info.Size()
	}

	format := "fmp4"
	if strings.HasSuffix(filePath, ".ts") {
		format = "mpegts"
	}

	now := time.Now().UTC()
	start := now.Add(-duration)

	n.database.InsertRecording(&db.Recording{
		CameraID:   cam.ID,
		StartTime:  start.Format("2006-01-02T15:04:05.000Z"),
		EndTime:    now.Format("2006-01-02T15:04:05.000Z"),
		DurationMs: duration.Milliseconds(),
		FilePath:   filePath,
		FileSize:   fileSize,
		Format:     format,
	})
}

// OnSegmentDelete is called when a recording segment is deleted by the cleaner.
func (n *NVR) OnSegmentDelete(filePath string) {
	n.database.DeleteRecordingByPath(filePath)
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
