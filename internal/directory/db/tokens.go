package db

import (
	"database/sql"
	"errors"
	"time"

	"github.com/google/uuid"
)

// RefreshToken represents a refresh token record in the database.
type RefreshToken struct {
	ID           string     `json:"id"`
	UserID       string     `json:"user_id"`
	TokenHash    string     `json:"token_hash"`
	ExpiresAt    string     `json:"expires_at"`
	RevokedAt    *time.Time `json:"revoked_at,omitempty"`
	IPAddress    string     `json:"ip_address"`
	UserAgent    string     `json:"user_agent"`
	DeviceName   string     `json:"device_name"`
	LastActivity string     `json:"last_activity"`
	CreatedAt    string     `json:"created_at"`
}

// Session is a read-only view of an active refresh token for session listing.
type Session struct {
	ID           string `json:"id"`
	UserID       string `json:"user_id"`
	Username     string `json:"username"`
	IPAddress    string `json:"ip_address"`
	UserAgent    string `json:"user_agent"`
	DeviceName   string `json:"device_name"`
	LastActivity string `json:"last_activity"`
	CreatedAt    string `json:"created_at"`
	ExpiresAt    string `json:"expires_at"`
}

// CreateRefreshToken inserts a new refresh token into the database.
// If tok.ID is empty, a new UUID is generated.
func (d *DB) CreateRefreshToken(tok *RefreshToken) error {
	if tok.ID == "" {
		tok.ID = uuid.New().String()
	}

	now := time.Now().UTC().Format(timeFormat)
	if tok.CreatedAt == "" {
		tok.CreatedAt = now
	}
	if tok.LastActivity == "" {
		tok.LastActivity = now
	}

	_, err := d.Exec(`
		INSERT INTO refresh_tokens (id, user_id, token_hash, expires_at, ip_address, user_agent, device_name, last_activity, created_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		tok.ID, tok.UserID, tok.TokenHash, tok.ExpiresAt,
		tok.IPAddress, tok.UserAgent, tok.DeviceName, tok.LastActivity, tok.CreatedAt,
	)
	return err
}

// GetRefreshToken retrieves a refresh token by its token hash.
// Returns ErrNotFound if no match.
func (d *DB) GetRefreshToken(tokenHash string) (*RefreshToken, error) {
	tok := &RefreshToken{}
	var revokedAt sql.NullString
	err := d.QueryRow(`
		SELECT id, user_id, token_hash, expires_at, revoked_at,
		       ip_address, user_agent, device_name, last_activity, created_at
		FROM refresh_tokens WHERE token_hash = ?`, tokenHash,
	).Scan(&tok.ID, &tok.UserID, &tok.TokenHash, &tok.ExpiresAt, &revokedAt,
		&tok.IPAddress, &tok.UserAgent, &tok.DeviceName, &tok.LastActivity, &tok.CreatedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	if revokedAt.Valid {
		t, err := time.Parse(timeFormat, revokedAt.String)
		if err != nil {
			return nil, err
		}
		tok.RevokedAt = &t
	}
	return tok, nil
}

// RevokeRefreshToken marks a refresh token as revoked by setting its revoked_at
// timestamp. Returns ErrNotFound if no match.
func (d *DB) RevokeRefreshToken(id string) error {
	now := time.Now().UTC().Format(timeFormat)
	res, err := d.Exec("UPDATE refresh_tokens SET revoked_at = ? WHERE id = ?", now, id)
	if err != nil {
		return err
	}

	n, err := res.RowsAffected()
	if err != nil {
		return err
	}
	if n == 0 {
		return ErrNotFound
	}
	return nil
}

// RevokeAllUserTokens revokes all refresh tokens for a given user.
func (d *DB) RevokeAllUserTokens(userID string) error {
	now := time.Now().UTC().Format(timeFormat)
	_, err := d.Exec(
		"UPDATE refresh_tokens SET revoked_at = ? WHERE user_id = ? AND revoked_at IS NULL",
		now, userID,
	)
	return err
}

// CleanExpiredTokens deletes all refresh tokens that have expired before the
// given cutoff time.
func (d *DB) CleanExpiredTokens(before time.Time) error {
	_, err := d.Exec(
		"DELETE FROM refresh_tokens WHERE expires_at < ?",
		before.UTC().Format(timeFormat),
	)
	return err
}

// UpdateSessionActivity updates the last_activity timestamp and IP for a session.
func (d *DB) UpdateSessionActivity(tokenID, ipAddress string) error {
	now := time.Now().UTC().Format(timeFormat)
	_, err := d.Exec(
		"UPDATE refresh_tokens SET last_activity = ?, ip_address = ? WHERE id = ? AND revoked_at IS NULL",
		now, ipAddress, tokenID,
	)
	return err
}

// ListActiveSessions returns all active (non-revoked, non-expired) sessions,
// optionally filtered by userID. Results include the username from the users table.
func (d *DB) ListActiveSessions(userID string) ([]*Session, error) {
	now := time.Now().UTC().Format(timeFormat)
	query := `
		SELECT rt.id, rt.user_id, u.username, rt.ip_address, rt.user_agent,
		       rt.device_name, rt.last_activity, rt.created_at, rt.expires_at
		FROM refresh_tokens rt
		JOIN users u ON u.id = rt.user_id
		WHERE rt.revoked_at IS NULL AND rt.expires_at > ?`
	args := []any{now}

	if userID != "" {
		query += " AND rt.user_id = ?"
		args = append(args, userID)
	}
	query += " ORDER BY rt.last_activity DESC"

	rows, err := d.Query(query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var sessions []*Session
	for rows.Next() {
		s := &Session{}
		if err := rows.Scan(&s.ID, &s.UserID, &s.Username, &s.IPAddress,
			&s.UserAgent, &s.DeviceName, &s.LastActivity, &s.CreatedAt, &s.ExpiresAt); err != nil {
			return nil, err
		}
		sessions = append(sessions, s)
	}
	return sessions, rows.Err()
}

// GetRefreshTokenByID retrieves a refresh token by its ID.
// Returns ErrNotFound if no match.
func (d *DB) GetRefreshTokenByID(id string) (*RefreshToken, error) {
	tok := &RefreshToken{}
	var revokedAt sql.NullString
	err := d.QueryRow(`
		SELECT id, user_id, token_hash, expires_at, revoked_at,
		       ip_address, user_agent, device_name, last_activity, created_at
		FROM refresh_tokens WHERE id = ?`, id,
	).Scan(&tok.ID, &tok.UserID, &tok.TokenHash, &tok.ExpiresAt, &revokedAt,
		&tok.IPAddress, &tok.UserAgent, &tok.DeviceName, &tok.LastActivity, &tok.CreatedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	if revokedAt.Valid {
		t, err := time.Parse(timeFormat, revokedAt.String)
		if err != nil {
			return nil, err
		}
		tok.RevokedAt = &t
	}
	return tok, nil
}

// RevokeIdleSessions revokes all sessions with last_activity older than the
// given idle timeout duration.
func (d *DB) RevokeIdleSessions(idleTimeout time.Duration) (int64, error) {
	cutoff := time.Now().UTC().Add(-idleTimeout).Format(timeFormat)
	now := time.Now().UTC().Format(timeFormat)
	res, err := d.Exec(
		"UPDATE refresh_tokens SET revoked_at = ? WHERE revoked_at IS NULL AND last_activity < ? AND last_activity != ''",
		now, cutoff,
	)
	if err != nil {
		return 0, err
	}
	return res.RowsAffected()
}
