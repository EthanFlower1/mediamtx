package revocation

import (
	"context"
	"database/sql"
	"fmt"
	"time"
)

// RevokedToken is a single entry in the revocation blocklist.
type RevokedToken struct {
	JTI        string
	RecorderID string
	TenantID   string
	RevokedBy  string
	Reason     string
	RevokedAt  time.Time
	ExpiresAt  time.Time
}

// Store is the SQLite-backed revocation blocklist. It is safe for
// concurrent use (inherits *sql.DB's pool safety).
type Store struct {
	db *sql.DB
}

// NewStore wraps the given database handle. The caller must ensure
// migration 0003 has been applied before calling any methods.
func NewStore(db *sql.DB) *Store {
	return &Store{db: db}
}

// IsRevoked reports whether the given JTI is in the blocklist.
// This is the hot-path check called from auth middleware on every
// authenticated request. It must be fast.
func (s *Store) IsRevoked(ctx context.Context, jti string) (bool, error) {
	var exists int
	err := s.db.QueryRowContext(ctx,
		`SELECT 1 FROM revoked_tokens WHERE jti = ? LIMIT 1`, jti,
	).Scan(&exists)
	if err == sql.ErrNoRows {
		return false, nil
	}
	if err != nil {
		return false, fmt.Errorf("revocation: is_revoked: %w", err)
	}
	return true, nil
}

// Revoke inserts a single token into the blocklist. It is idempotent:
// inserting a JTI that already exists is a no-op (INSERT OR IGNORE).
func (s *Store) Revoke(ctx context.Context, tok RevokedToken) error {
	_, err := s.db.ExecContext(ctx,
		`INSERT OR IGNORE INTO revoked_tokens
		    (jti, recorder_id, tenant_id, revoked_by, reason, revoked_at, expires_at)
		 VALUES (?, ?, ?, ?, ?, ?, ?)`,
		tok.JTI, tok.RecorderID, tok.TenantID, tok.RevokedBy,
		tok.Reason, tok.RevokedAt, tok.ExpiresAt,
	)
	if err != nil {
		return fmt.Errorf("revocation: revoke: %w", err)
	}
	return nil
}

// RevokeBatch inserts multiple tokens in a single transaction.
// Idempotent per-JTI (INSERT OR IGNORE).
func (s *Store) RevokeBatch(ctx context.Context, tokens []RevokedToken) (int64, error) {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return 0, fmt.Errorf("revocation: begin tx: %w", err)
	}
	defer tx.Rollback() //nolint:errcheck

	stmt, err := tx.PrepareContext(ctx,
		`INSERT OR IGNORE INTO revoked_tokens
		    (jti, recorder_id, tenant_id, revoked_by, reason, revoked_at, expires_at)
		 VALUES (?, ?, ?, ?, ?, ?, ?)`,
	)
	if err != nil {
		return 0, fmt.Errorf("revocation: prepare: %w", err)
	}
	defer stmt.Close()

	var total int64
	for _, tok := range tokens {
		res, err := stmt.ExecContext(ctx,
			tok.JTI, tok.RecorderID, tok.TenantID, tok.RevokedBy,
			tok.Reason, tok.RevokedAt, tok.ExpiresAt,
		)
		if err != nil {
			return total, fmt.Errorf("revocation: insert jti=%s: %w", tok.JTI, err)
		}
		n, _ := res.RowsAffected()
		total += n
	}

	if err := tx.Commit(); err != nil {
		return 0, fmt.Errorf("revocation: commit: %w", err)
	}
	return total, nil
}

// ListByRecorder returns all revoked tokens for a given recorder, ordered
// by revoked_at descending. For admin UI / audit display.
func (s *Store) ListByRecorder(ctx context.Context, recorderID string) ([]RevokedToken, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT jti, recorder_id, tenant_id, revoked_by, reason, revoked_at, expires_at
		 FROM revoked_tokens
		 WHERE recorder_id = ?
		 ORDER BY revoked_at DESC`,
		recorderID,
	)
	if err != nil {
		return nil, fmt.Errorf("revocation: list_by_recorder: %w", err)
	}
	defer rows.Close()

	return scanTokens(rows)
}

// PurgeExpired deletes rows whose original token expiry has passed. This
// is safe because the token verifier already rejects expired tokens, so
// the blocklist entry is redundant after that point. Returns the number
// of rows deleted.
func (s *Store) PurgeExpired(ctx context.Context, now time.Time) (int64, error) {
	res, err := s.db.ExecContext(ctx,
		`DELETE FROM revoked_tokens WHERE expires_at < ?`, now,
	)
	if err != nil {
		return 0, fmt.Errorf("revocation: purge_expired: %w", err)
	}
	n, _ := res.RowsAffected()
	return n, nil
}

// Count returns the total number of revoked tokens in the blocklist.
func (s *Store) Count(ctx context.Context) (int64, error) {
	var n int64
	err := s.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM revoked_tokens`).Scan(&n)
	if err != nil {
		return 0, fmt.Errorf("revocation: count: %w", err)
	}
	return n, nil
}

func scanTokens(rows *sql.Rows) ([]RevokedToken, error) {
	var out []RevokedToken
	for rows.Next() {
		var t RevokedToken
		if err := rows.Scan(
			&t.JTI, &t.RecorderID, &t.TenantID, &t.RevokedBy,
			&t.Reason, &t.RevokedAt, &t.ExpiresAt,
		); err != nil {
			return nil, fmt.Errorf("revocation: scan: %w", err)
		}
		out = append(out, t)
	}
	return out, rows.Err()
}
