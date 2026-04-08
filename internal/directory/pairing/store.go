package pairing

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	directorydb "github.com/bluenviron/mediamtx/internal/directory/db"
)

// TokenStatus is the lifecycle state of a stored PairingToken.
type TokenStatus string

const (
	StatusPending  TokenStatus = "pending"
	StatusRedeemed TokenStatus = "redeemed"
	StatusExpired  TokenStatus = "expired"
)

// StoredToken is the DB row shape returned by Store.Get.
type StoredToken struct {
	TokenID        string
	EncodedBlob    string
	Status         TokenStatus
	SuggestedRoles []string
	SignedBy       UserID
	CloudTenant    string
	ExpiresAt      time.Time
	CreatedAt      time.Time
	RedeemedAt     *time.Time
}

// Store is the SQLite-backed pairing token repository. It is safe for
// concurrent use and enforces single-use semantics at the database level.
type Store struct {
	db *directorydb.DB
}

// NewStore constructs a Store backed by the given directory DB handle.
func NewStore(db *directorydb.DB) *Store {
	return &Store{db: db}
}

// Insert persists a newly generated token. The encoded blob is the full
// base64url-encoded signed string produced by PairingToken.Encode.
func (s *Store) Insert(ctx context.Context, pt *PairingToken, encodedBlob string) error {
	rolesJSON, err := json.Marshal(pt.SuggestedRoles)
	if err != nil {
		return fmt.Errorf("pairing/store: marshal roles: %w", err)
	}
	_, err = s.db.ExecContext(ctx,
		`INSERT INTO pairing_tokens
            (token_id, encoded_blob, status, suggested_roles, signed_by, cloud_tenant, expires_at)
         VALUES (?, ?, 'pending', ?, ?, ?, ?)`,
		pt.TokenID,
		encodedBlob,
		string(rolesJSON),
		string(pt.SignedBy),
		pt.CloudTenantBinding,
		pt.ExpiresAt.UTC().Format(time.RFC3339),
	)
	if err != nil {
		return fmt.Errorf("pairing/store: insert: %w", err)
	}
	return nil
}

// Get returns the stored token row for tokenID. Returns ErrNotFound if absent.
func (s *Store) Get(ctx context.Context, tokenID string) (*StoredToken, error) {
	row := s.db.QueryRowContext(ctx,
		`SELECT token_id, encoded_blob, status, suggested_roles, signed_by,
                cloud_tenant, expires_at, created_at, redeemed_at
           FROM pairing_tokens WHERE token_id = ?`, tokenID)
	return scanToken(row)
}

// Redeem atomically transitions a token from 'pending' to 'redeemed' using a
// single UPDATE WHERE status='pending'. It is safe under concurrent load: only
// one goroutine or process will observe rows_affected == 1.
//
// Returns ErrAlreadyRedeemed if the token was already consumed.
// Returns ErrNotFound if no such token exists.
// Returns ErrTokenExpired if the token exists but is past its ExpiresAt.
func (s *Store) Redeem(ctx context.Context, tokenID string) error {
	now := time.Now().UTC()
	res, err := s.db.ExecContext(ctx,
		`UPDATE pairing_tokens
            SET status = 'redeemed', redeemed_at = ?
          WHERE token_id = ? AND status = 'pending' AND expires_at > ?`,
		now.Format(time.RFC3339),
		tokenID,
		now.Format(time.RFC3339),
	)
	if err != nil {
		return fmt.Errorf("pairing/store: redeem: %w", err)
	}
	n, err := res.RowsAffected()
	if err != nil {
		return fmt.Errorf("pairing/store: redeem rows affected: %w", err)
	}
	if n == 1 {
		return nil
	}

	// Zero rows updated — determine why.
	st, err := s.Get(ctx, tokenID)
	if err != nil {
		if errors.Is(err, ErrNotFound) {
			return ErrNotFound
		}
		return fmt.Errorf("pairing/store: redeem lookup: %w", err)
	}
	if st.Status == StatusRedeemed {
		return ErrAlreadyRedeemed
	}
	if st.ExpiresAt.Before(now) || st.Status == StatusExpired {
		return ErrTokenExpired
	}
	return fmt.Errorf("pairing/store: redeem: unexpected state %q", st.Status)
}

// MarkExpired bulk-updates all pending tokens past their ExpiresAt to
// 'expired'. Returns the number of rows affected. Called by the sweeper.
func (s *Store) MarkExpired(ctx context.Context) (int64, error) {
	res, err := s.db.ExecContext(ctx,
		`UPDATE pairing_tokens
            SET status = 'expired'
          WHERE status = 'pending' AND expires_at <= ?`,
		time.Now().UTC().Format(time.RFC3339),
	)
	if err != nil {
		return 0, fmt.Errorf("pairing/store: mark expired: %w", err)
	}
	return res.RowsAffected()
}

// --- sentinel errors --------------------------------------------------------

var (
	// ErrNotFound is returned when no token with the requested ID exists.
	ErrNotFound = errors.New("pairing: token not found")
	// ErrAlreadyRedeemed is returned when Redeem is called on a consumed token.
	ErrAlreadyRedeemed = errors.New("pairing: token already redeemed")
	// ErrTokenExpired is returned when Redeem is called on an expired token.
	ErrTokenExpired = errors.New("pairing: token expired")
)

// --- internal helpers -------------------------------------------------------

func scanToken(row *sql.Row) (*StoredToken, error) {
	var (
		st          StoredToken
		rolesJSON   string
		expiresRaw  string
		createdRaw  string
		redeemedRaw sql.NullString
	)
	if err := row.Scan(
		&st.TokenID,
		&st.EncodedBlob,
		(*string)(&st.Status),
		&rolesJSON,
		(*string)(&st.SignedBy),
		&st.CloudTenant,
		&expiresRaw,
		&createdRaw,
		&redeemedRaw,
	); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, ErrNotFound
		}
		return nil, fmt.Errorf("pairing/store: scan: %w", err)
	}
	if err := json.Unmarshal([]byte(rolesJSON), &st.SuggestedRoles); err != nil {
		return nil, fmt.Errorf("pairing/store: unmarshal roles: %w", err)
	}
	if t, err := time.Parse(time.RFC3339, expiresRaw); err == nil {
		st.ExpiresAt = t.UTC()
	}
	if t, err := time.Parse(time.RFC3339, createdRaw); err == nil {
		st.CreatedAt = t.UTC()
	}
	if redeemedRaw.Valid && redeemedRaw.String != "" {
		if t, err := time.Parse(time.RFC3339, redeemedRaw.String); err == nil {
			t = t.UTC()
			st.RedeemedAt = &t
		}
	}
	return &st, nil
}
