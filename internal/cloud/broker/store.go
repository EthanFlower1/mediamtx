// Package broker implements the cloud broker's tenant and API key management.
package broker

import (
	"crypto/rand"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"errors"
	"fmt"
	"strings"
	"time"

	"golang.org/x/crypto/bcrypt"
)

// bcryptCost is intentionally hardcoded; tune via a single constant if needed.
const bcryptCost = 12

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
);
CREATE TABLE IF NOT EXISTS users (
    id TEXT PRIMARY KEY,
    tenant_id TEXT NOT NULL REFERENCES tenants(id),
    email TEXT NOT NULL UNIQUE,
    name TEXT NOT NULL DEFAULT '',
    password_hash TEXT NOT NULL,
    created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP
);
CREATE TABLE IF NOT EXISTS sessions (
    token_hash TEXT PRIMARY KEY,
    user_id TEXT NOT NULL REFERENCES users(id),
    tenant_id TEXT NOT NULL REFERENCES tenants(id),
    expires_at DATETIME NOT NULL,
    created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
    last_used_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP
);
CREATE INDEX IF NOT EXISTS sessions_user_idx ON sessions(user_id);
CREATE INDEX IF NOT EXISTS sessions_expires_idx ON sessions(expires_at);`
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

// User represents an end-user (an admin or operator) belonging to a tenant.
type User struct {
	ID        string    `json:"id"`
	TenantID  string    `json:"tenant_id"`
	Email     string    `json:"email"`
	Name      string    `json:"name"`
	CreatedAt time.Time `json:"created_at"`
}

// Session represents an authenticated browser session.
type Session struct {
	UserID     string
	TenantID   string
	ExpiresAt  time.Time
	CreatedAt  time.Time
	LastUsedAt time.Time
}

// ErrInvalidCredentials is returned when login fails.
var ErrInvalidCredentials = errors.New("invalid credentials")

// ErrSessionInvalid is returned when a session token is unknown or expired.
var ErrSessionInvalid = errors.New("session invalid or expired")

// CreateUser inserts a new user with a bcrypt-hashed password and returns the user ID.
func (s *Store) CreateUser(tenantID, email, name, password string) (string, error) {
	if password == "" {
		return "", errors.New("password required")
	}
	hash, err := bcrypt.GenerateFromPassword([]byte(password), bcryptCost)
	if err != nil {
		return "", fmt.Errorf("hash password: %w", err)
	}
	id, err := randomHex(16)
	if err != nil {
		return "", fmt.Errorf("generate user id: %w", err)
	}
	_, err = s.db.Exec(
		`INSERT INTO users (id, tenant_id, email, name, password_hash) VALUES (?, ?, ?, ?, ?)`,
		id, tenantID, strings.ToLower(email), name, string(hash),
	)
	if err != nil {
		return "", fmt.Errorf("insert user: %w", err)
	}
	return id, nil
}

// GetUserByEmail looks up a user by email. Returns ErrInvalidCredentials if not found
// (deliberately conflated with bad-password to prevent user-enumeration).
func (s *Store) GetUserByEmail(email string) (*User, error) {
	var u User
	err := s.db.QueryRow(
		`SELECT id, tenant_id, email, name, created_at FROM users WHERE email = ?`,
		strings.ToLower(email),
	).Scan(&u.ID, &u.TenantID, &u.Email, &u.Name, &u.CreatedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrInvalidCredentials
	}
	if err != nil {
		return nil, err
	}
	return &u, nil
}

// VerifyPassword compares a plaintext password against the stored hash for the
// given email. Returns the user on success, ErrInvalidCredentials otherwise.
func (s *Store) VerifyPassword(email, password string) (*User, error) {
	var u User
	var hash string
	err := s.db.QueryRow(
		`SELECT id, tenant_id, email, name, password_hash, created_at FROM users WHERE email = ?`,
		strings.ToLower(email),
	).Scan(&u.ID, &u.TenantID, &u.Email, &u.Name, &hash, &u.CreatedAt)
	if errors.Is(err, sql.ErrNoRows) {
		// Run bcrypt against a constant to keep timing comparable.
		_ = bcrypt.CompareHashAndPassword([]byte("$2a$12$abcdefghijklmnopqrstuv"), []byte(password))
		return nil, ErrInvalidCredentials
	}
	if err != nil {
		return nil, err
	}
	if err := bcrypt.CompareHashAndPassword([]byte(hash), []byte(password)); err != nil {
		return nil, ErrInvalidCredentials
	}
	return &u, nil
}

// hashSessionToken returns the SHA-256 hex of a session token.
func hashSessionToken(token string) string {
	h := sha256.Sum256([]byte(token))
	return hex.EncodeToString(h[:])
}

// CreateSession generates a random opaque token, persists its hash, and returns
// the plaintext token (the only time it's revealed). ttl is the session lifetime.
func (s *Store) CreateSession(userID, tenantID string, ttl time.Duration) (string, error) {
	token, err := randomHex(32) // 256 bits
	if err != nil {
		return "", fmt.Errorf("generate session token: %w", err)
	}
	expires := time.Now().Add(ttl).UTC()
	_, err = s.db.Exec(
		`INSERT INTO sessions (token_hash, user_id, tenant_id, expires_at) VALUES (?, ?, ?, ?)`,
		hashSessionToken(token), userID, tenantID, expires,
	)
	if err != nil {
		return "", fmt.Errorf("insert session: %w", err)
	}
	return token, nil
}

// ValidateSession looks up a session by token, returns its user, or
// ErrSessionInvalid if missing/expired. Updates last_used_at as a side effect.
func (s *Store) ValidateSession(token string) (*User, error) {
	var sess Session
	err := s.db.QueryRow(
		`SELECT user_id, tenant_id, expires_at, created_at, last_used_at
		 FROM sessions WHERE token_hash = ?`,
		hashSessionToken(token),
	).Scan(&sess.UserID, &sess.TenantID, &sess.ExpiresAt, &sess.CreatedAt, &sess.LastUsedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrSessionInvalid
	}
	if err != nil {
		return nil, err
	}
	if time.Now().After(sess.ExpiresAt) {
		return nil, ErrSessionInvalid
	}
	if _, err := s.db.Exec(
		`UPDATE sessions SET last_used_at = CURRENT_TIMESTAMP WHERE token_hash = ?`,
		hashSessionToken(token),
	); err != nil {
		return nil, fmt.Errorf("update session: %w", err)
	}
	var u User
	err = s.db.QueryRow(
		`SELECT id, tenant_id, email, name, created_at FROM users WHERE id = ?`,
		sess.UserID,
	).Scan(&u.ID, &u.TenantID, &u.Email, &u.Name, &u.CreatedAt)
	if err != nil {
		return nil, err
	}
	return &u, nil
}

// RotateSession revokes the old session and issues a new one with a fresh
// expiry. Returns the new token.
func (s *Store) RotateSession(oldToken string, ttl time.Duration) (string, error) {
	user, err := s.ValidateSession(oldToken)
	if err != nil {
		return "", err
	}
	if err := s.RevokeSession(oldToken); err != nil {
		return "", err
	}
	return s.CreateSession(user.ID, user.TenantID, ttl)
}

// RevokeSession deletes a session. No error if it doesn't exist.
func (s *Store) RevokeSession(token string) error {
	_, err := s.db.Exec(
		`DELETE FROM sessions WHERE token_hash = ?`,
		hashSessionToken(token),
	)
	return err
}

// CleanupExpiredSessions removes all expired session rows. Call periodically.
func (s *Store) CleanupExpiredSessions() error {
	_, err := s.db.Exec(`DELETE FROM sessions WHERE expires_at < CURRENT_TIMESTAMP`)
	return err
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
