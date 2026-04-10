package apikeys

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/bluenviron/mediamtx/internal/cloud/publicapi"
	"github.com/google/uuid"
)

// Dialect selects the SQL placeholder style. Production uses Postgres;
// unit tests use SQLite.
type Dialect int

const (
	DialectPostgres Dialect = iota
	DialectSQLite
)

// Store is the SQL-backed implementation of publicapi.APIKeyStore.
// It also provides Create, Get, List, Rotate, Revoke, and ListExpiring
// which go beyond the minimal Validate/TouchLastUsed contract that the
// middleware needs.
type Store struct {
	db      *sql.DB
	dialect Dialect
}

// compile-time interface check
var _ publicapi.APIKeyStore = (*Store)(nil)

// New creates a Store. The caller must ensure the api_keys and
// api_key_audit_log tables exist (via migrations or ApplyStubSchema).
func New(db *sql.DB, dialect Dialect) *Store {
	return &Store{db: db, dialect: dialect}
}

// ApplyStubSchema creates the api_keys and api_key_audit_log tables for
// unit tests running against SQLite. Production uses the migration files.
func (s *Store) ApplyStubSchema(ctx context.Context) error {
	stmts := []string{
		`CREATE TABLE IF NOT EXISTS api_keys (
			id               TEXT PRIMARY KEY,
			tenant_id        TEXT NOT NULL,
			name             TEXT NOT NULL,
			key_prefix       TEXT NOT NULL,
			key_hash         TEXT NOT NULL,
			scopes           TEXT NOT NULL DEFAULT '[]',
			tier             TEXT NOT NULL DEFAULT 'free',
			created_by       TEXT NOT NULL,
			created_at       DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
			expires_at       DATETIME,
			revoked_at       DATETIME,
			last_used_at     DATETIME,
			rotated_from_id  TEXT,
			grace_expires_at DATETIME
		)`,
		`CREATE TABLE IF NOT EXISTS api_key_audit_log (
			id         TEXT PRIMARY KEY,
			key_id     TEXT NOT NULL,
			tenant_id  TEXT NOT NULL,
			action     TEXT NOT NULL,
			actor_id   TEXT NOT NULL,
			ip_address TEXT,
			user_agent TEXT,
			metadata   TEXT NOT NULL DEFAULT '{}',
			created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP
		)`,
	}
	for _, stmt := range stmts {
		if _, err := s.db.ExecContext(ctx, stmt); err != nil {
			return fmt.Errorf("apikeys: stub schema: %w", err)
		}
	}
	return nil
}

// generateRawKey produces a cryptographically random key with the kvue_ prefix.
// Format: "kvue_" + 40 random hex chars (20 bytes of entropy).
func generateRawKey() (string, error) {
	b := make([]byte, 20)
	if _, err := rand.Read(b); err != nil {
		return "", fmt.Errorf("apikeys: generate key: %w", err)
	}
	return publicapi.APIKeyPrefix + hex.EncodeToString(b), nil
}

// hashKey returns the SHA-256 hex digest of a raw key.
func hashKey(rawKey string) string {
	h := sha256.Sum256([]byte(rawKey))
	return hex.EncodeToString(h[:])
}

// Create generates a new API key and stores its hash. The plaintext key is
// returned in CreateAPIKeyResult.RawKey and is never stored or retrievable.
func (s *Store) Create(ctx context.Context, req publicapi.CreateAPIKeyRequest) (*publicapi.CreateAPIKeyResult, error) {
	if req.TenantID == "" {
		return nil, errors.New("apikeys: tenant_id is required")
	}
	if req.Name == "" {
		return nil, errors.New("apikeys: name is required")
	}
	if req.CreatedBy == "" {
		return nil, errors.New("apikeys: created_by is required")
	}

	rawKey, err := generateRawKey()
	if err != nil {
		return nil, err
	}

	keyID := uuid.New().String()
	keyHash := hashKey(rawKey)
	now := time.Now().UTC()

	// Key prefix for display: first 8 chars (e.g. "kvue_a1b")
	prefix := rawKey
	if len(prefix) > 8 {
		prefix = prefix[:8]
	}

	tier := req.Tier
	if tier == "" {
		tier = publicapi.TierFree
	}

	scopesJSON, err := json.Marshal(req.Scopes)
	if err != nil {
		return nil, fmt.Errorf("apikeys: marshal scopes: %w", err)
	}
	if req.Scopes == nil {
		scopesJSON = []byte("[]")
	}

	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return nil, fmt.Errorf("apikeys: begin tx: %w", err)
	}
	defer tx.Rollback() //nolint:errcheck

	var insertQuery string
	switch s.dialect {
	case DialectPostgres:
		insertQuery = `INSERT INTO api_keys (id, tenant_id, name, key_prefix, key_hash, scopes, tier, created_by, created_at, expires_at)
			VALUES ($1, $2, $3, $4, $5, $6::jsonb, $7, $8, $9, $10)`
	default:
		insertQuery = `INSERT INTO api_keys (id, tenant_id, name, key_prefix, key_hash, scopes, tier, created_by, created_at, expires_at)
			VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`
	}

	var expiresAt any
	if !req.ExpiresAt.IsZero() {
		expiresAt = req.ExpiresAt.UTC()
	}

	if _, err := tx.ExecContext(ctx, insertQuery,
		keyID, req.TenantID, req.Name, prefix, keyHash,
		string(scopesJSON), string(tier), req.CreatedBy, now, expiresAt,
	); err != nil {
		return nil, fmt.Errorf("apikeys: insert key: %w", err)
	}

	// Record audit entry.
	if err := s.recordAudit(ctx, tx, AuditEntry{
		KeyID:    keyID,
		TenantID: req.TenantID,
		Action:   AuditCreate,
		ActorID:  req.CreatedBy,
	}); err != nil {
		return nil, err
	}

	if err := tx.Commit(); err != nil {
		return nil, fmt.Errorf("apikeys: commit: %w", err)
	}

	key := &publicapi.APIKey{
		ID:        keyID,
		KeyHash:   keyHash,
		TenantID:  req.TenantID,
		Tier:      tier,
		Name:      req.Name,
		Scopes:    req.Scopes,
		CreatedAt: now,
		ExpiresAt: req.ExpiresAt,
		CreatedBy: req.CreatedBy,
		KeyPrefix: prefix,
	}

	return &publicapi.CreateAPIKeyResult{
		RawKey: rawKey,
		Key:    key,
	}, nil
}

// Get returns key metadata by ID.
func (s *Store) Get(ctx context.Context, keyID string) (*publicapi.APIKey, error) {
	var query string
	switch s.dialect {
	case DialectPostgres:
		query = `SELECT id, tenant_id, name, key_prefix, key_hash, scopes, tier, created_by, created_at,
			expires_at, revoked_at, last_used_at, rotated_from_id, grace_expires_at
			FROM api_keys WHERE id = $1`
	default:
		query = `SELECT id, tenant_id, name, key_prefix, key_hash, scopes, tier, created_by, created_at,
			expires_at, revoked_at, last_used_at, rotated_from_id, grace_expires_at
			FROM api_keys WHERE id = ?`
	}

	return s.scanKey(s.db.QueryRowContext(ctx, query, keyID))
}

// List returns keys for a tenant, ordered by created_at desc.
func (s *Store) List(ctx context.Context, filter publicapi.ListAPIKeysFilter) ([]*publicapi.APIKey, error) {
	if filter.TenantID == "" {
		return nil, errors.New("apikeys: tenant_id is required")
	}
	if filter.Limit <= 0 || filter.Limit > 1000 {
		filter.Limit = 100
	}

	var (
		query string
		args  []any
	)

	switch s.dialect {
	case DialectPostgres:
		query = `SELECT id, tenant_id, name, key_prefix, key_hash, scopes, tier, created_by, created_at,
			expires_at, revoked_at, last_used_at, rotated_from_id, grace_expires_at
			FROM api_keys WHERE tenant_id = $1`
		args = append(args, filter.TenantID)
		idx := 2
		if !filter.IncludeRevoked {
			query += fmt.Sprintf(" AND revoked_at IS NULL")
		}
		if filter.Cursor != "" {
			query += fmt.Sprintf(" AND id < $%d", idx)
			args = append(args, filter.Cursor)
			idx++
		}
		query += fmt.Sprintf(" ORDER BY created_at DESC LIMIT $%d", idx)
		args = append(args, filter.Limit)
	default:
		query = `SELECT id, tenant_id, name, key_prefix, key_hash, scopes, tier, created_by, created_at,
			expires_at, revoked_at, last_used_at, rotated_from_id, grace_expires_at
			FROM api_keys WHERE tenant_id = ?`
		args = append(args, filter.TenantID)
		if !filter.IncludeRevoked {
			query += " AND revoked_at IS NULL"
		}
		if filter.Cursor != "" {
			query += " AND id < ?"
			args = append(args, filter.Cursor)
		}
		query += " ORDER BY created_at DESC LIMIT ?"
		args = append(args, filter.Limit)
	}

	rows, err := s.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("apikeys: list: %w", err)
	}
	defer rows.Close()

	var keys []*publicapi.APIKey
	for rows.Next() {
		k, err := s.scanKeyFromRows(rows)
		if err != nil {
			return nil, err
		}
		keys = append(keys, k)
	}
	return keys, rows.Err()
}

// Rotate creates a new key to replace an existing one. The old key enters
// a grace period during which both are valid.
func (s *Store) Rotate(ctx context.Context, req publicapi.RotateAPIKeyRequest) (*publicapi.RotateAPIKeyResult, error) {
	if req.KeyID == "" {
		return nil, errors.New("apikeys: key_id is required")
	}
	if req.RotatedBy == "" {
		return nil, errors.New("apikeys: rotated_by is required")
	}

	gracePeriod := req.GracePeriod
	if gracePeriod == 0 {
		gracePeriod = publicapi.DefaultGracePeriod
	}

	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return nil, fmt.Errorf("apikeys: begin tx: %w", err)
	}
	defer tx.Rollback() //nolint:errcheck

	// Load the existing key.
	var oldQuery string
	switch s.dialect {
	case DialectPostgres:
		oldQuery = `SELECT id, tenant_id, name, key_prefix, key_hash, scopes, tier, created_by, created_at,
			expires_at, revoked_at, last_used_at, rotated_from_id, grace_expires_at
			FROM api_keys WHERE id = $1 FOR UPDATE`
	default:
		oldQuery = `SELECT id, tenant_id, name, key_prefix, key_hash, scopes, tier, created_by, created_at,
			expires_at, revoked_at, last_used_at, rotated_from_id, grace_expires_at
			FROM api_keys WHERE id = ?`
	}

	oldKey, err := s.scanKey(tx.QueryRowContext(ctx, oldQuery, req.KeyID))
	if err != nil {
		if errors.Is(err, publicapi.ErrAPIKeyNotFound) {
			return nil, err
		}
		return nil, fmt.Errorf("apikeys: load old key: %w", err)
	}

	if oldKey.IsRevoked() {
		return nil, publicapi.ErrAPIKeyRevoked
	}

	// Generate the new key.
	rawKey, err := generateRawKey()
	if err != nil {
		return nil, err
	}

	now := time.Now().UTC()
	graceEnd := now.Add(gracePeriod)
	newKeyID := uuid.New().String()
	newHash := hashKey(rawKey)
	prefix := rawKey[:8]

	scopesJSON, _ := json.Marshal(oldKey.Scopes)
	if oldKey.Scopes == nil {
		scopesJSON = []byte("[]")
	}

	// Insert the new key, carrying forward name, scopes, tier, and expiry.
	var insertQuery string
	switch s.dialect {
	case DialectPostgres:
		insertQuery = `INSERT INTO api_keys (id, tenant_id, name, key_prefix, key_hash, scopes, tier, created_by, created_at, expires_at, rotated_from_id)
			VALUES ($1, $2, $3, $4, $5, $6::jsonb, $7, $8, $9, $10, $11)`
	default:
		insertQuery = `INSERT INTO api_keys (id, tenant_id, name, key_prefix, key_hash, scopes, tier, created_by, created_at, expires_at, rotated_from_id)
			VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`
	}

	var expiresAt any
	if !oldKey.ExpiresAt.IsZero() {
		expiresAt = oldKey.ExpiresAt
	}

	if _, err := tx.ExecContext(ctx, insertQuery,
		newKeyID, oldKey.TenantID, oldKey.Name, prefix, newHash,
		string(scopesJSON), string(oldKey.Tier), req.RotatedBy, now,
		expiresAt, oldKey.ID,
	); err != nil {
		return nil, fmt.Errorf("apikeys: insert rotated key: %w", err)
	}

	// Set grace period on the old key.
	var updateQuery string
	switch s.dialect {
	case DialectPostgres:
		updateQuery = `UPDATE api_keys SET grace_expires_at = $1 WHERE id = $2`
	default:
		updateQuery = `UPDATE api_keys SET grace_expires_at = ? WHERE id = ?`
	}

	if _, err := tx.ExecContext(ctx, updateQuery, graceEnd, oldKey.ID); err != nil {
		return nil, fmt.Errorf("apikeys: set grace period: %w", err)
	}

	// Audit entries for both old and new keys.
	meta := fmt.Sprintf(`{"old_key_id":"%s","new_key_id":"%s","grace_period_seconds":%d}`,
		oldKey.ID, newKeyID, int(gracePeriod.Seconds()))

	if err := s.recordAudit(ctx, tx, AuditEntry{
		KeyID:    oldKey.ID,
		TenantID: oldKey.TenantID,
		Action:   AuditRotate,
		ActorID:  req.RotatedBy,
		Metadata: meta,
	}); err != nil {
		return nil, err
	}
	if err := s.recordAudit(ctx, tx, AuditEntry{
		KeyID:    newKeyID,
		TenantID: oldKey.TenantID,
		Action:   AuditCreate,
		ActorID:  req.RotatedBy,
		Metadata: meta,
	}); err != nil {
		return nil, err
	}

	if err := tx.Commit(); err != nil {
		return nil, fmt.Errorf("apikeys: commit: %w", err)
	}

	newKey := &publicapi.APIKey{
		ID:            newKeyID,
		KeyHash:       newHash,
		TenantID:      oldKey.TenantID,
		Tier:          oldKey.Tier,
		Name:          oldKey.Name,
		Scopes:        oldKey.Scopes,
		CreatedAt:     now,
		ExpiresAt:     oldKey.ExpiresAt,
		CreatedBy:     req.RotatedBy,
		KeyPrefix:     prefix,
		RotatedFromID: oldKey.ID,
	}

	return &publicapi.RotateAPIKeyResult{
		RawKey:         rawKey,
		NewKey:         newKey,
		OldKeyGraceEnd: graceEnd,
	}, nil
}

// Revoke immediately marks a key as revoked.
func (s *Store) Revoke(ctx context.Context, keyID string, revokedBy string) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("apikeys: begin tx: %w", err)
	}
	defer tx.Rollback() //nolint:errcheck

	// Load the key to verify it exists and get tenant ID.
	var loadQuery string
	switch s.dialect {
	case DialectPostgres:
		loadQuery = `SELECT id, tenant_id, revoked_at FROM api_keys WHERE id = $1`
	default:
		loadQuery = `SELECT id, tenant_id, revoked_at FROM api_keys WHERE id = ?`
	}

	var id, tenantID string
	var revokedAt sql.NullTime
	if err := tx.QueryRowContext(ctx, loadQuery, keyID).Scan(&id, &tenantID, &revokedAt); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return publicapi.ErrAPIKeyNotFound
		}
		return fmt.Errorf("apikeys: load for revoke: %w", err)
	}

	if revokedAt.Valid {
		return publicapi.ErrAPIKeyAlreadyRevoked
	}

	now := time.Now().UTC()

	var updateQuery string
	switch s.dialect {
	case DialectPostgres:
		updateQuery = `UPDATE api_keys SET revoked_at = $1 WHERE id = $2`
	default:
		updateQuery = `UPDATE api_keys SET revoked_at = ? WHERE id = ?`
	}

	if _, err := tx.ExecContext(ctx, updateQuery, now, keyID); err != nil {
		return fmt.Errorf("apikeys: revoke: %w", err)
	}

	if err := s.recordAudit(ctx, tx, AuditEntry{
		KeyID:    keyID,
		TenantID: tenantID,
		Action:   AuditRevoke,
		ActorID:  revokedBy,
	}); err != nil {
		return err
	}

	return tx.Commit()
}

// Validate looks up a key by its raw value, verifies the hash, checks
// expiry and revocation status, and returns the key record.
func (s *Store) Validate(ctx context.Context, rawKey string) (*publicapi.APIKey, error) {
	if !strings.HasPrefix(rawKey, publicapi.APIKeyPrefix) {
		return nil, publicapi.ErrInvalidAPIKey
	}

	keyHash := hashKey(rawKey)

	var query string
	switch s.dialect {
	case DialectPostgres:
		query = `SELECT id, tenant_id, name, key_prefix, key_hash, scopes, tier, created_by, created_at,
			expires_at, revoked_at, last_used_at, rotated_from_id, grace_expires_at
			FROM api_keys WHERE key_hash = $1`
	default:
		query = `SELECT id, tenant_id, name, key_prefix, key_hash, scopes, tier, created_by, created_at,
			expires_at, revoked_at, last_used_at, rotated_from_id, grace_expires_at
			FROM api_keys WHERE key_hash = ?`
	}

	key, err := s.scanKey(s.db.QueryRowContext(ctx, query, keyHash))
	if err != nil {
		if errors.Is(err, publicapi.ErrAPIKeyNotFound) {
			return nil, publicapi.ErrInvalidAPIKey
		}
		return nil, fmt.Errorf("apikeys: validate: %w", err)
	}

	// Check revocation first (takes priority over expiry).
	if key.IsRevoked() {
		return nil, publicapi.ErrAPIKeyRevoked
	}

	// Check expiry. For rotated keys in their grace period, use grace_expires_at.
	if !key.GraceExpiresAt.IsZero() {
		if time.Now().After(key.GraceExpiresAt) {
			return nil, publicapi.ErrAPIKeyExpired
		}
		// Key is in grace period; treat as active even if normal expiry passed.
	} else if key.IsExpired() {
		return nil, publicapi.ErrAPIKeyExpired
	}

	return key, nil
}

// TouchLastUsed updates the last_used_at timestamp. Best-effort.
func (s *Store) TouchLastUsed(ctx context.Context, keyID string) error {
	var query string
	switch s.dialect {
	case DialectPostgres:
		query = `UPDATE api_keys SET last_used_at = $1 WHERE id = $2`
	default:
		query = `UPDATE api_keys SET last_used_at = ? WHERE id = ?`
	}
	_, err := s.db.ExecContext(ctx, query, time.Now().UTC(), keyID)
	return err
}

// ListExpiring returns active, non-revoked keys for a tenant whose expiry
// falls within the given duration from now.
func (s *Store) ListExpiring(ctx context.Context, tenantID string, within time.Duration) ([]*publicapi.APIKey, error) {
	if tenantID == "" {
		return nil, errors.New("apikeys: tenant_id is required")
	}

	deadline := time.Now().UTC().Add(within)

	var query string
	switch s.dialect {
	case DialectPostgres:
		query = `SELECT id, tenant_id, name, key_prefix, key_hash, scopes, tier, created_by, created_at,
			expires_at, revoked_at, last_used_at, rotated_from_id, grace_expires_at
			FROM api_keys
			WHERE tenant_id = $1 AND revoked_at IS NULL AND expires_at IS NOT NULL AND expires_at <= $2 AND expires_at > $3
			ORDER BY expires_at ASC`
	default:
		query = `SELECT id, tenant_id, name, key_prefix, key_hash, scopes, tier, created_by, created_at,
			expires_at, revoked_at, last_used_at, rotated_from_id, grace_expires_at
			FROM api_keys
			WHERE tenant_id = ? AND revoked_at IS NULL AND expires_at IS NOT NULL AND expires_at <= ? AND expires_at > ?
			ORDER BY expires_at ASC`
	}

	now := time.Now().UTC()
	rows, err := s.db.QueryContext(ctx, query, tenantID, deadline, now)
	if err != nil {
		return nil, fmt.Errorf("apikeys: list expiring: %w", err)
	}
	defer rows.Close()

	var keys []*publicapi.APIKey
	for rows.Next() {
		k, err := s.scanKeyFromRows(rows)
		if err != nil {
			return nil, err
		}
		keys = append(keys, k)
	}
	return keys, rows.Err()
}

// scanKey scans a single row from a *sql.Row into an APIKey.
func (s *Store) scanKey(row *sql.Row) (*publicapi.APIKey, error) {
	var (
		k              publicapi.APIKey
		scopesJSON     string
		tier           string
		expiresAt      sql.NullTime
		revokedAt      sql.NullTime
		lastUsedAt     sql.NullTime
		rotatedFromID  sql.NullString
		graceExpiresAt sql.NullTime
	)

	err := row.Scan(&k.ID, &k.TenantID, &k.Name, &k.KeyPrefix, &k.KeyHash,
		&scopesJSON, &tier, &k.CreatedBy, &k.CreatedAt,
		&expiresAt, &revokedAt, &lastUsedAt, &rotatedFromID, &graceExpiresAt)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, publicapi.ErrAPIKeyNotFound
		}
		return nil, fmt.Errorf("apikeys: scan key: %w", err)
	}

	k.Tier = publicapi.TenantTier(tier)
	if expiresAt.Valid {
		k.ExpiresAt = expiresAt.Time
	}
	if revokedAt.Valid {
		k.RevokedAt = revokedAt.Time
	}
	if lastUsedAt.Valid {
		k.LastUsedAt = lastUsedAt.Time
	}
	if rotatedFromID.Valid {
		k.RotatedFromID = rotatedFromID.String
	}
	if graceExpiresAt.Valid {
		k.GraceExpiresAt = graceExpiresAt.Time
	}
	if err := json.Unmarshal([]byte(scopesJSON), &k.Scopes); err != nil {
		return nil, fmt.Errorf("apikeys: unmarshal scopes: %w", err)
	}

	return &k, nil
}

// scanKeyFromRows scans a single row from *sql.Rows (used in List methods).
func (s *Store) scanKeyFromRows(rows *sql.Rows) (*publicapi.APIKey, error) {
	var (
		k              publicapi.APIKey
		scopesJSON     string
		tier           string
		expiresAt      sql.NullTime
		revokedAt      sql.NullTime
		lastUsedAt     sql.NullTime
		rotatedFromID  sql.NullString
		graceExpiresAt sql.NullTime
	)

	err := rows.Scan(&k.ID, &k.TenantID, &k.Name, &k.KeyPrefix, &k.KeyHash,
		&scopesJSON, &tier, &k.CreatedBy, &k.CreatedAt,
		&expiresAt, &revokedAt, &lastUsedAt, &rotatedFromID, &graceExpiresAt)
	if err != nil {
		return nil, fmt.Errorf("apikeys: scan key: %w", err)
	}

	k.Tier = publicapi.TenantTier(tier)
	if expiresAt.Valid {
		k.ExpiresAt = expiresAt.Time
	}
	if revokedAt.Valid {
		k.RevokedAt = revokedAt.Time
	}
	if lastUsedAt.Valid {
		k.LastUsedAt = lastUsedAt.Time
	}
	if rotatedFromID.Valid {
		k.RotatedFromID = rotatedFromID.String
	}
	if graceExpiresAt.Valid {
		k.GraceExpiresAt = graceExpiresAt.Time
	}
	if err := json.Unmarshal([]byte(scopesJSON), &k.Scopes); err != nil {
		return nil, fmt.Errorf("apikeys: unmarshal scopes: %w", err)
	}

	return &k, nil
}
