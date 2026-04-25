// Package db — API key persistence for the integrator portal (KAI-319).
package db

import (
	"database/sql"
	"errors"
	"time"

	"github.com/google/uuid"
)

// APIKey represents an integrator API key stored in the database.
type APIKey struct {
	ID             string `json:"id"`
	Name           string `json:"name"`
	KeyPrefix      string `json:"key_prefix"`
	KeyHash        string `json:"-"` // never exposed in JSON
	Scope          string `json:"scope"`           // "read-only" or "read-write"
	CustomerScope  string `json:"customer_scope"`  // optional customer limitation
	CreatedBy      string `json:"created_by"`
	ExpiresAt      string `json:"expires_at,omitempty"`
	RevokedAt      string `json:"revoked_at,omitempty"`
	RotatedFrom    string `json:"rotated_from,omitempty"`    // ID of previous key (rotation chain)
	GraceExpiresAt string `json:"grace_expires_at,omitempty"` // old key still valid until this time
	LastUsedAt     string `json:"last_used_at,omitempty"`
	CreatedAt      string `json:"created_at"`
	UpdatedAt      string `json:"updated_at"`
}

// APIKeyAuditEntry is a per-key audit log row.
type APIKeyAuditEntry struct {
	ID            int64  `json:"id"`
	APIKeyID      string `json:"api_key_id"`
	Action        string `json:"action"` // "created", "rotated", "revoked", "used"
	ActorID       string `json:"actor_id"`
	ActorUsername string `json:"actor_username"`
	IPAddress     string `json:"ip_address"`
	Details       string `json:"details"`
	CreatedAt     string `json:"created_at"`
}

// CreateAPIKey inserts a new API key record. If k.ID is empty a UUID is
// generated. The caller is responsible for hashing the raw key before passing
// it as k.KeyHash.
func (d *DB) CreateAPIKey(k *APIKey) error {
	if k.ID == "" {
		k.ID = uuid.New().String()
	}
	now := time.Now().UTC().Format(timeFormat)
	if k.CreatedAt == "" {
		k.CreatedAt = now
	}
	k.UpdatedAt = now

	_, err := d.Exec(`
		INSERT INTO api_keys
			(id, name, key_prefix, key_hash, scope, customer_scope,
			 created_by, expires_at, rotated_from, grace_expires_at,
			 created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		k.ID, k.Name, k.KeyPrefix, k.KeyHash, k.Scope, k.CustomerScope,
		k.CreatedBy, k.ExpiresAt, k.RotatedFrom, k.GraceExpiresAt,
		k.CreatedAt, k.UpdatedAt,
	)
	return err
}

// GetAPIKey retrieves a single API key by its ID. Returns ErrNotFound when
// the key does not exist.
func (d *DB) GetAPIKey(id string) (*APIKey, error) {
	k := &APIKey{}
	err := d.QueryRow(`
		SELECT id, name, key_prefix, key_hash, scope, customer_scope,
		       created_by, expires_at, revoked_at, rotated_from,
		       grace_expires_at, last_used_at, created_at, updated_at
		FROM api_keys WHERE id = ?`, id,
	).Scan(&k.ID, &k.Name, &k.KeyPrefix, &k.KeyHash, &k.Scope, &k.CustomerScope,
		&k.CreatedBy, &k.ExpiresAt, &k.RevokedAt, &k.RotatedFrom,
		&k.GraceExpiresAt, &k.LastUsedAt, &k.CreatedAt, &k.UpdatedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrNotFound
	}
	return k, err
}

// GetAPIKeyByHash looks up an API key by its SHA-256 hash. This is the hot
// path for authenticating incoming API requests. Returns ErrNotFound when the
// hash does not match any key.
func (d *DB) GetAPIKeyByHash(hash string) (*APIKey, error) {
	k := &APIKey{}
	err := d.QueryRow(`
		SELECT id, name, key_prefix, key_hash, scope, customer_scope,
		       created_by, expires_at, revoked_at, rotated_from,
		       grace_expires_at, last_used_at, created_at, updated_at
		FROM api_keys WHERE key_hash = ?`, hash,
	).Scan(&k.ID, &k.Name, &k.KeyPrefix, &k.KeyHash, &k.Scope, &k.CustomerScope,
		&k.CreatedBy, &k.ExpiresAt, &k.RevokedAt, &k.RotatedFrom,
		&k.GraceExpiresAt, &k.LastUsedAt, &k.CreatedAt, &k.UpdatedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrNotFound
	}
	return k, err
}

// ListAPIKeys returns all API keys created by the given user (or all keys
// if createdBy is empty). Revoked keys are included so the UI can show
// history.
func (d *DB) ListAPIKeys(createdBy string) ([]*APIKey, error) {
	query := `
		SELECT id, name, key_prefix, scope, customer_scope,
		       created_by, expires_at, revoked_at, rotated_from,
		       grace_expires_at, last_used_at, created_at, updated_at
		FROM api_keys`
	args := []any{}
	if createdBy != "" {
		query += " WHERE created_by = ?"
		args = append(args, createdBy)
	}
	query += " ORDER BY created_at DESC"

	rows, err := d.Query(query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var keys []*APIKey
	for rows.Next() {
		k := &APIKey{}
		if err := rows.Scan(&k.ID, &k.Name, &k.KeyPrefix, &k.Scope, &k.CustomerScope,
			&k.CreatedBy, &k.ExpiresAt, &k.RevokedAt, &k.RotatedFrom,
			&k.GraceExpiresAt, &k.LastUsedAt, &k.CreatedAt, &k.UpdatedAt); err != nil {
			return nil, err
		}
		keys = append(keys, k)
	}
	return keys, rows.Err()
}

// RevokeAPIKey marks a key as revoked. Returns ErrNotFound if the key does
// not exist.
func (d *DB) RevokeAPIKey(id string) error {
	now := time.Now().UTC().Format(timeFormat)
	res, err := d.Exec(
		"UPDATE api_keys SET revoked_at = ?, updated_at = ? WHERE id = ? AND revoked_at = ''",
		now, now, id,
	)
	if err != nil {
		return err
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return ErrNotFound
	}
	return nil
}

// UpdateAPIKeyLastUsed bumps the last_used_at timestamp for the given key.
func (d *DB) UpdateAPIKeyLastUsed(id string) error {
	now := time.Now().UTC().Format(timeFormat)
	_, err := d.Exec("UPDATE api_keys SET last_used_at = ? WHERE id = ?", now, id)
	return err
}

// SetAPIKeyGraceExpiry sets the grace_expires_at on a key (used during
// rotation so the old key remains valid for a grace period).
func (d *DB) SetAPIKeyGraceExpiry(id string, grace time.Time) error {
	now := time.Now().UTC().Format(timeFormat)
	_, err := d.Exec(
		"UPDATE api_keys SET grace_expires_at = ?, updated_at = ? WHERE id = ?",
		grace.UTC().Format(timeFormat), now, id,
	)
	return err
}

// InsertAPIKeyAudit records an audit entry for a specific API key.
func (d *DB) InsertAPIKeyAudit(e *APIKeyAuditEntry) error {
	_, err := d.Exec(`
		INSERT INTO api_key_audit_log
			(api_key_id, action, actor_id, actor_username, ip_address, details)
		VALUES (?, ?, ?, ?, ?, ?)`,
		e.APIKeyID, e.Action, e.ActorID, e.ActorUsername, e.IPAddress, e.Details,
	)
	return err
}

// ListAPIKeyAudit returns audit entries for the given key, most recent first.
func (d *DB) ListAPIKeyAudit(apiKeyID string) ([]*APIKeyAuditEntry, error) {
	rows, err := d.Query(`
		SELECT id, api_key_id, action, actor_id, actor_username,
		       ip_address, details, created_at
		FROM api_key_audit_log WHERE api_key_id = ?
		ORDER BY id DESC`, apiKeyID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var entries []*APIKeyAuditEntry
	for rows.Next() {
		e := &APIKeyAuditEntry{}
		if err := rows.Scan(&e.ID, &e.APIKeyID, &e.Action, &e.ActorID,
			&e.ActorUsername, &e.IPAddress, &e.Details, &e.CreatedAt); err != nil {
			return nil, err
		}
		entries = append(entries, e)
	}
	return entries, rows.Err()
}
