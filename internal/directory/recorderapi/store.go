// Package recorderapi implements the Directory's API for managing recording
// servers in managed mode. It handles recorder registration, heartbeats,
// service token auth, camera CRUD, and fan-out queries to recorders.
package recorderapi

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"fmt"
	"time"
)

// RecorderRow is the database representation of a registered recorder.
type RecorderRow struct {
	ID              string
	Name            string
	Hostname        string
	InternalAPIAddr string
	HealthStatus    string
	LastCheckinAt   time.Time
}

// Store handles recorder persistence in the Directory's SQLite database.
type Store struct {
	db *sql.DB
}

// NewStore creates a Store backed by the given database connection.
func NewStore(db *sql.DB) *Store {
	return &Store{db: db}
}

// GetRecorder returns a recorder by ID.
func (s *Store) GetRecorder(ctx context.Context, id string) (RecorderRow, error) {
	var r RecorderRow
	var lastCheckin string
	err := s.db.QueryRowContext(ctx, `
		SELECT id, COALESCE(name,''), COALESCE(hostname,''), COALESCE(internal_api_addr,''),
		       COALESCE(health_status,'unknown'), COALESCE(last_checkin_at,'')
		FROM recorders WHERE id = ?
	`, id).Scan(&r.ID, &r.Name, &r.Hostname, &r.InternalAPIAddr, &r.HealthStatus, &lastCheckin)
	if err != nil {
		return RecorderRow{}, fmt.Errorf("recorderapi/store: get %s: %w", id, err)
	}
	r.LastCheckinAt, _ = time.Parse(time.RFC3339, lastCheckin)
	return r, nil
}

// ListRecorders returns all registered recorders.
func (s *Store) ListRecorders(ctx context.Context) ([]RecorderRow, error) {
	rows, err := s.db.QueryContext(ctx, `
		SELECT id, COALESCE(name,''), COALESCE(hostname,''), COALESCE(internal_api_addr,''),
		       COALESCE(health_status,'unknown'), COALESCE(last_checkin_at,'')
		FROM recorders ORDER BY id
	`)
	if err != nil {
		return nil, fmt.Errorf("recorderapi/store: list: %w", err)
	}
	defer rows.Close()

	var out []RecorderRow
	for rows.Next() {
		var r RecorderRow
		var lastCheckin string
		if err := rows.Scan(&r.ID, &r.Name, &r.Hostname, &r.InternalAPIAddr, &r.HealthStatus, &lastCheckin); err != nil {
			return nil, fmt.Errorf("recorderapi/store: scan: %w", err)
		}
		r.LastCheckinAt, _ = time.Parse(time.RFC3339, lastCheckin)
		out = append(out, r)
	}
	return out, rows.Err()
}

// DefaultTenantID is used for on-prem single-tenant deployments.
const DefaultTenantID = "local"

// UpsertRecorder inserts or updates a recorder entry from a registration.
func (s *Store) UpsertRecorder(ctx context.Context, r RecorderRow) error {
	_, err := s.db.ExecContext(ctx, `
		INSERT INTO recorders (id, tenant_id, name, hostname, internal_api_addr, health_status, last_checkin_at, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, CURRENT_TIMESTAMP, CURRENT_TIMESTAMP)
		ON CONFLICT(id) DO UPDATE SET
			name = excluded.name,
			hostname = excluded.hostname,
			internal_api_addr = excluded.internal_api_addr,
			health_status = excluded.health_status,
			last_checkin_at = excluded.last_checkin_at,
			updated_at = CURRENT_TIMESTAMP
	`, r.ID, DefaultTenantID, r.Name, r.Hostname, r.InternalAPIAddr, r.HealthStatus, r.LastCheckinAt.UTC().Format(time.RFC3339))
	if err != nil {
		return fmt.Errorf("recorderapi/store: upsert %s: %w", r.ID, err)
	}
	return nil
}

// UpdateHeartbeat updates the last checkin time and health status.
func (s *Store) UpdateHeartbeat(ctx context.Context, recorderID, healthStatus string) error {
	res, err := s.db.ExecContext(ctx, `
		UPDATE recorders SET last_checkin_at = ?, health_status = ?, updated_at = CURRENT_TIMESTAMP
		WHERE id = ?
	`, time.Now().UTC().Format(time.RFC3339), healthStatus, recorderID)
	if err != nil {
		return fmt.Errorf("recorderapi/store: heartbeat %s: %w", recorderID, err)
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return fmt.Errorf("recorderapi/store: recorder %s not found", recorderID)
	}
	return nil
}

// SetServiceToken generates a random service token, hashes it, and stores
// the hash. Returns the plaintext token (shown once to the operator).
func (s *Store) SetServiceToken(ctx context.Context, recorderID string) (string, error) {
	tokenBytes := make([]byte, 32)
	if _, err := rand.Read(tokenBytes); err != nil {
		return "", fmt.Errorf("recorderapi/store: generate token: %w", err)
	}
	token := hex.EncodeToString(tokenBytes)

	salt := make([]byte, 16)
	if _, err := rand.Read(salt); err != nil {
		return "", fmt.Errorf("recorderapi/store: generate salt: %w", err)
	}

	hash := hashToken(token, salt)

	res, err := s.db.ExecContext(ctx, `
		UPDATE recorders SET service_token_hash = ?, service_token_salt = ?, updated_at = CURRENT_TIMESTAMP
		WHERE id = ?
	`, hash, salt, recorderID)
	if err != nil {
		return "", fmt.Errorf("recorderapi/store: set token %s: %w", recorderID, err)
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return "", fmt.Errorf("recorderapi/store: recorder %s not found", recorderID)
	}
	return token, nil
}

// ValidateToken checks a bearer token against stored hashes and returns
// the recorder ID if valid.
func (s *Store) ValidateToken(ctx context.Context, token string) (string, error) {
	rows, err := s.db.QueryContext(ctx, `
		SELECT id, service_token_hash, service_token_salt
		FROM recorders
		WHERE service_token_hash IS NOT NULL
	`)
	if err != nil {
		return "", fmt.Errorf("recorderapi/store: validate token query: %w", err)
	}
	defer rows.Close()

	for rows.Next() {
		var id string
		var storedHash, salt []byte
		if err := rows.Scan(&id, &storedHash, &salt); err != nil {
			continue
		}
		candidate := hashToken(token, salt)
		if sha256Equal(candidate, storedHash) {
			return id, nil
		}
	}
	return "", fmt.Errorf("recorderapi/store: invalid token")
}

// hashToken produces a SHA-256 hash of token+salt. For v1 this is sufficient;
// upgrade to Argon2id when this moves to production.
func hashToken(token string, salt []byte) []byte {
	h := sha256.New()
	h.Write(salt)
	h.Write([]byte(token))
	return h.Sum(nil)
}

func sha256Equal(a, b []byte) bool {
	if len(a) != len(b) {
		return false
	}
	var result byte
	for i := range a {
		result |= a[i] ^ b[i]
	}
	return result == 0
}
