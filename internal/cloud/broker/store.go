// Package broker implements the cloud broker's tenant and API key management.
package broker

import (
	"crypto/rand"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"fmt"
	"time"
)

// Tenant represents a registered cloud tenant.
type Tenant struct {
	ID        string    `json:"id"`
	Name      string    `json:"name"`
	Email     string    `json:"email"`
	CreatedAt time.Time `json:"created_at"`
}

// APIKeyInfo holds non-secret metadata about an API key.
type APIKeyInfo struct {
	ID        string    `json:"id"`
	TenantID  string    `json:"tenant_id"`
	Name      string    `json:"name"`
	Prefix    string    `json:"prefix"`
	CreatedAt time.Time `json:"created_at"`
}

// Store provides CRUD operations for tenants and API keys backed by SQLite.
type Store struct {
	db *sql.DB
}

// NewStore creates a Store and ensures the schema exists.
func NewStore(db *sql.DB) (*Store, error) {
	schema := `
CREATE TABLE IF NOT EXISTS tenants (
    id TEXT PRIMARY KEY,
    name TEXT NOT NULL,
    email TEXT NOT NULL UNIQUE,
    created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP
);
CREATE TABLE IF NOT EXISTS api_keys (
    id TEXT PRIMARY KEY,
    tenant_id TEXT NOT NULL REFERENCES tenants(id),
    name TEXT NOT NULL DEFAULT 'default',
    key_hash TEXT NOT NULL UNIQUE,
    key_prefix TEXT NOT NULL,
    created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP
);`
	if _, err := db.Exec(schema); err != nil {
		return nil, fmt.Errorf("broker store: create tables: %w", err)
	}
	return &Store{db: db}, nil
}

// randomHex generates n random bytes and returns their hex encoding.
func randomHex(n int) (string, error) {
	b := make([]byte, n)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return hex.EncodeToString(b), nil
}

// CreateTenant registers a new tenant and returns its ID.
func (s *Store) CreateTenant(name, email string) (string, error) {
	id, err := randomHex(16)
	if err != nil {
		return "", fmt.Errorf("generate tenant id: %w", err)
	}
	_, err = s.db.Exec(
		`INSERT INTO tenants (id, name, email) VALUES (?, ?, ?)`,
		id, name, email,
	)
	if err != nil {
		return "", fmt.Errorf("insert tenant: %w", err)
	}
	return id, nil
}

// GetTenant retrieves a tenant by ID.
func (s *Store) GetTenant(id string) (*Tenant, error) {
	var t Tenant
	err := s.db.QueryRow(
		`SELECT id, name, email, created_at FROM tenants WHERE id = ?`, id,
	).Scan(&t.ID, &t.Name, &t.Email, &t.CreatedAt)
	if err != nil {
		return nil, err
	}
	return &t, nil
}

// GetTenantByEmail retrieves a tenant by email address.
func (s *Store) GetTenantByEmail(email string) (*Tenant, error) {
	var t Tenant
	err := s.db.QueryRow(
		`SELECT id, name, email, created_at FROM tenants WHERE email = ?`, email,
	).Scan(&t.ID, &t.Name, &t.Email, &t.CreatedAt)
	if err != nil {
		return nil, err
	}
	return &t, nil
}

// CreateAPIKey generates a new API key for a tenant and returns the plain-text
// key. The key format is "kvue_" followed by 40 hex characters. Only the
// SHA-256 hash is persisted.
func (s *Store) CreateAPIKey(tenantID, name string) (string, error) {
	id, err := randomHex(16)
	if err != nil {
		return "", fmt.Errorf("generate key id: %w", err)
	}

	secret, err := randomHex(20) // 20 bytes = 40 hex chars
	if err != nil {
		return "", fmt.Errorf("generate key secret: %w", err)
	}
	plainKey := "kvue_" + secret

	hash := sha256.Sum256([]byte(plainKey))
	keyHash := hex.EncodeToString(hash[:])
	prefix := plainKey[:10] // "kvue_" + first 5 hex chars

	_, err = s.db.Exec(
		`INSERT INTO api_keys (id, tenant_id, name, key_hash, key_prefix)
		 VALUES (?, ?, ?, ?, ?)`,
		id, tenantID, name, keyHash, prefix,
	)
	if err != nil {
		return "", fmt.Errorf("insert api key: %w", err)
	}
	return plainKey, nil
}

// ValidateAPIKey checks a plain-text API key and returns the associated
// tenant ID if valid.
func (s *Store) ValidateAPIKey(plainKey string) (string, error) {
	hash := sha256.Sum256([]byte(plainKey))
	keyHash := hex.EncodeToString(hash[:])

	var tenantID string
	err := s.db.QueryRow(
		`SELECT tenant_id FROM api_keys WHERE key_hash = ?`, keyHash,
	).Scan(&tenantID)
	if err != nil {
		return "", err
	}
	return tenantID, nil
}

// ListAPIKeys returns all API key metadata for a tenant (no secrets).
func (s *Store) ListAPIKeys(tenantID string) ([]APIKeyInfo, error) {
	rows, err := s.db.Query(
		`SELECT id, tenant_id, name, key_prefix, created_at
		 FROM api_keys WHERE tenant_id = ? ORDER BY created_at`,
		tenantID,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var keys []APIKeyInfo
	for rows.Next() {
		var k APIKeyInfo
		if err := rows.Scan(&k.ID, &k.TenantID, &k.Name, &k.Prefix, &k.CreatedAt); err != nil {
			return nil, err
		}
		keys = append(keys, k)
	}
	return keys, rows.Err()
}
