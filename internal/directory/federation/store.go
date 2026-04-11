package federation

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"

	directorydb "github.com/bluenviron/mediamtx/internal/directory/db"
)

// TokenStatus is the lifecycle state of a stored federation token.
type TokenStatus string

const (
	StatusPending  TokenStatus = "pending"
	StatusRedeemed TokenStatus = "redeemed"
	StatusExpired  TokenStatus = "expired"
)

// StoredToken is the DB row shape for a federation token.
type StoredToken struct {
	TokenID     string
	EncodedBlob string
	Status      TokenStatus
	PeerSiteID  string
	IssuedBy    string
	ExpiresAt   time.Time
	CreatedAt   time.Time
	RedeemedAt  *time.Time
}

// MemberRow is the DB row shape for a federation member.
type MemberRow struct {
	SiteID         string
	Name           string
	Endpoint       string
	JWKSJson       string
	CAFingerprint  string
	JoinedAt       time.Time
	LastSeenAt     *time.Time
	Status         string
}

// Store is the SQLite-backed federation token and member repository.
// It is safe for concurrent use and enforces single-use semantics at the
// database level, following the same pattern as internal/directory/pairing.
type Store struct {
	db *directorydb.DB
}

// NewStore constructs a Store backed by the given directory DB handle.
func NewStore(db *directorydb.DB) *Store {
	return &Store{db: db}
}

// InsertToken persists a newly minted federation token.
func (s *Store) InsertToken(ctx context.Context, tokenID, encodedBlob, peerSiteID, issuedBy string, expiresAt time.Time) error {
	_, err := s.db.ExecContext(ctx,
		`INSERT INTO federation_tokens
            (token_id, encoded_blob, status, peer_site_id, issued_by, expires_at)
         VALUES (?, ?, 'pending', ?, ?, ?)`,
		tokenID,
		encodedBlob,
		peerSiteID,
		issuedBy,
		expiresAt.UTC().Format(time.RFC3339),
	)
	if err != nil {
		return fmt.Errorf("federation/store: insert token: %w", err)
	}
	return nil
}

// GetToken returns the stored token row. Returns ErrNotFound if absent.
func (s *Store) GetToken(ctx context.Context, tokenID string) (*StoredToken, error) {
	row := s.db.QueryRowContext(ctx,
		`SELECT token_id, encoded_blob, status, peer_site_id, issued_by,
                expires_at, created_at, redeemed_at
           FROM federation_tokens WHERE token_id = ?`, tokenID)
	return scanToken(row)
}

// RedeemToken atomically transitions a token from 'pending' to 'redeemed'.
// Only one caller will observe rowsAffected==1; all others see an error.
//
// Returns ErrAlreadyRedeemed if the token was already consumed.
// Returns ErrNotFound if no such token exists.
// Returns ErrTokenExpired if the token is past its ExpiresAt.
func (s *Store) RedeemToken(ctx context.Context, tokenID string) error {
	now := time.Now().UTC()
	res, err := s.db.ExecContext(ctx,
		`UPDATE federation_tokens
            SET status = 'redeemed', redeemed_at = ?
          WHERE token_id = ? AND status = 'pending' AND expires_at > ?`,
		now.Format(time.RFC3339),
		tokenID,
		now.Format(time.RFC3339),
	)
	if err != nil {
		return fmt.Errorf("federation/store: redeem: %w", err)
	}
	n, err := res.RowsAffected()
	if err != nil {
		return fmt.Errorf("federation/store: redeem rows affected: %w", err)
	}
	if n == 1 {
		return nil
	}

	// Zero rows updated — determine why.
	st, err := s.GetToken(ctx, tokenID)
	if err != nil {
		if errors.Is(err, ErrNotFound) {
			return ErrNotFound
		}
		return fmt.Errorf("federation/store: redeem lookup: %w", err)
	}
	if st.Status == StatusRedeemed {
		return ErrAlreadyRedeemed
	}
	if st.ExpiresAt.Before(now) || st.Status == StatusExpired {
		return ErrTokenExpired
	}
	return fmt.Errorf("federation/store: redeem: unexpected state %q", st.Status)
}

// MarkExpired bulk-updates all pending tokens past their ExpiresAt.
func (s *Store) MarkExpired(ctx context.Context) (int64, error) {
	res, err := s.db.ExecContext(ctx,
		`UPDATE federation_tokens
            SET status = 'expired'
          WHERE status = 'pending' AND expires_at <= ?`,
		time.Now().UTC().Format(time.RFC3339),
	)
	if err != nil {
		return 0, fmt.Errorf("federation/store: mark expired: %w", err)
	}
	return res.RowsAffected()
}

// InsertMember writes a new federation member row after a successful handshake.
func (s *Store) InsertMember(ctx context.Context, m MemberRow) error {
	_, err := s.db.ExecContext(ctx,
		`INSERT INTO federation_members
            (site_id, name, endpoint, jwks_json, ca_fingerprint, status)
         VALUES (?, ?, ?, ?, ?, 'active')`,
		m.SiteID,
		m.Name,
		m.Endpoint,
		m.JWKSJson,
		m.CAFingerprint,
	)
	if err != nil {
		return fmt.Errorf("federation/store: insert member: %w", err)
	}
	return nil
}

// GetMember returns a federation member by site ID.
func (s *Store) GetMember(ctx context.Context, siteID string) (*MemberRow, error) {
	row := s.db.QueryRowContext(ctx,
		`SELECT site_id, name, endpoint, jwks_json, ca_fingerprint,
                joined_at, last_seen_at, status
           FROM federation_members WHERE site_id = ?`, siteID)
	return scanMember(row)
}

// ListActiveMembers returns all members with status='active'.
func (s *Store) ListActiveMembers(ctx context.Context) ([]MemberRow, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT site_id, name, endpoint, jwks_json, ca_fingerprint,
                joined_at, last_seen_at, status
           FROM federation_members WHERE status = 'active' ORDER BY joined_at ASC`)
	if err != nil {
		return nil, fmt.Errorf("federation/store: list active members: %w", err)
	}
	defer rows.Close()

	var out []MemberRow
	for rows.Next() {
		var (
			m           MemberRow
			joinedRaw   string
			lastSeenRaw sql.NullString
		)
		if err := rows.Scan(
			&m.SiteID, &m.Name, &m.Endpoint, &m.JWKSJson,
			&m.CAFingerprint, &joinedRaw, &lastSeenRaw, &m.Status,
		); err != nil {
			return nil, fmt.Errorf("federation/store: scan member: %w", err)
		}
		if t, err := time.Parse(time.RFC3339, joinedRaw); err == nil {
			m.JoinedAt = t.UTC()
		}
		if lastSeenRaw.Valid && lastSeenRaw.String != "" {
			if t, err := time.Parse(time.RFC3339, lastSeenRaw.String); err == nil {
				t = t.UTC()
				m.LastSeenAt = &t
			}
		}
		out = append(out, m)
	}
	return out, rows.Err()
}

// UpdateMemberLastSeen bumps the last_seen_at timestamp for a member.
func (s *Store) UpdateMemberLastSeen(ctx context.Context, siteID string) error {
	now := time.Now().UTC().Format(time.RFC3339)
	_, err := s.db.ExecContext(ctx,
		`UPDATE federation_members SET last_seen_at = ? WHERE site_id = ?`,
		now, siteID,
	)
	return err
}

// --- sentinel errors --------------------------------------------------------

var (
	// ErrNotFound is returned when no token or member with the requested ID exists.
	ErrNotFound = errors.New("federation: not found")
	// ErrAlreadyRedeemed is returned when RedeemToken is called on a consumed token.
	ErrAlreadyRedeemed = errors.New("federation: token already redeemed")
	// ErrTokenExpired is returned when RedeemToken is called on an expired token.
	ErrTokenExpired = errors.New("federation: token expired")
)

// --- internal helpers -------------------------------------------------------

func scanToken(row *sql.Row) (*StoredToken, error) {
	var (
		st          StoredToken
		expiresRaw  string
		createdRaw  string
		redeemedRaw sql.NullString
	)
	if err := row.Scan(
		&st.TokenID,
		&st.EncodedBlob,
		(*string)(&st.Status),
		&st.PeerSiteID,
		&st.IssuedBy,
		&expiresRaw,
		&createdRaw,
		&redeemedRaw,
	); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, ErrNotFound
		}
		return nil, fmt.Errorf("federation/store: scan token: %w", err)
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

func scanMember(row *sql.Row) (*MemberRow, error) {
	var (
		m           MemberRow
		joinedRaw   string
		lastSeenRaw sql.NullString
	)
	if err := row.Scan(
		&m.SiteID, &m.Name, &m.Endpoint, &m.JWKSJson,
		&m.CAFingerprint, &joinedRaw, &lastSeenRaw, &m.Status,
	); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, ErrNotFound
		}
		return nil, fmt.Errorf("federation/store: scan member: %w", err)
	}
	if t, err := time.Parse(time.RFC3339, joinedRaw); err == nil {
		m.JoinedAt = t.UTC()
	}
	if lastSeenRaw.Valid && lastSeenRaw.String != "" {
		if t, err := time.Parse(time.RFC3339, lastSeenRaw.String); err == nil {
			t = t.UTC()
			m.LastSeenAt = &t
		}
	}
	return &m, nil
}
