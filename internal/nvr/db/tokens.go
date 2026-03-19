package db

import (
	"database/sql"
	"errors"
	"time"

	"github.com/google/uuid"
)

// RefreshToken represents a refresh token record in the database.
type RefreshToken struct {
	ID        string     `json:"id"`
	UserID    string     `json:"user_id"`
	TokenHash string     `json:"token_hash"`
	ExpiresAt string     `json:"expires_at"`
	RevokedAt *time.Time `json:"revoked_at,omitempty"`
}

// CreateRefreshToken inserts a new refresh token into the database.
// If tok.ID is empty, a new UUID is generated.
func (d *DB) CreateRefreshToken(tok *RefreshToken) error {
	if tok.ID == "" {
		tok.ID = uuid.New().String()
	}

	_, err := d.Exec(`
		INSERT INTO refresh_tokens (id, user_id, token_hash, expires_at)
		VALUES (?, ?, ?, ?)`,
		tok.ID, tok.UserID, tok.TokenHash, tok.ExpiresAt,
	)
	return err
}

// GetRefreshToken retrieves a refresh token by its token hash.
// Returns ErrNotFound if no match.
func (d *DB) GetRefreshToken(tokenHash string) (*RefreshToken, error) {
	tok := &RefreshToken{}
	var revokedAt sql.NullString
	err := d.QueryRow(`
		SELECT id, user_id, token_hash, expires_at, revoked_at
		FROM refresh_tokens WHERE token_hash = ?`, tokenHash,
	).Scan(&tok.ID, &tok.UserID, &tok.TokenHash, &tok.ExpiresAt, &revokedAt)
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
