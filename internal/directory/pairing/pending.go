package pairing

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"

	directorydb "github.com/bluenviron/mediamtx/internal/directory/db"
)

// PendingRequestTTL is the time a pending pairing request stays open for admin
// decision before it is automatically expired by the sweeper.
const PendingRequestTTL = 5 * time.Minute

// PendingStatus is the lifecycle state of a pending pairing request.
type PendingStatus string

const (
	PendingStatusPending  PendingStatus = "pending"
	PendingStatusApproved PendingStatus = "approved"
	PendingStatusDenied   PendingStatus = "denied"
	PendingStatusExpired  PendingStatus = "expired"
)

// PendingRequest represents an inbound request from a Recorder that discovered
// the Directory via mDNS and wants to pair (admin-approval flow, KAI-245).
type PendingRequest struct {
	ID               string
	RecorderHostname string
	RecorderIP       string
	RequestedRoles   []string
	Status           PendingStatus
	// TokenID is populated on approval — foreign key to pairing_tokens.
	TokenID     string
	RequestNote string
	ExpiresAt   time.Time
	CreatedAt   time.Time
	DecidedAt   *time.Time
	DecidedBy   UserID
}

// PendingStore is the SQLite-backed repository for pending pairing requests.
// It is safe for concurrent use.
type PendingStore struct {
	db *directorydb.DB
}

// NewPendingStore constructs a PendingStore backed by the given directory DB.
func NewPendingStore(db *directorydb.DB) *PendingStore {
	return &PendingStore{db: db}
}

// Create inserts a new pending pairing request and returns it with generated
// ID and timestamps set.
func (s *PendingStore) Create(ctx context.Context, hostname, ip, note string, roles []string) (*PendingRequest, error) {
	if hostname == "" {
		return nil, fmt.Errorf("pending/store: recorder_hostname is required")
	}
	if len(roles) == 0 {
		roles = []string{"recorder"}
	}
	now := time.Now().UTC()
	req := &PendingRequest{
		ID:               uuid.NewString(),
		RecorderHostname: hostname,
		RecorderIP:       ip,
		RequestedRoles:   roles,
		Status:           PendingStatusPending,
		RequestNote:      note,
		ExpiresAt:        now.Add(PendingRequestTTL),
		CreatedAt:        now,
	}

	rolesJSON := marshalRoles(roles)
	_, err := s.db.ExecContext(ctx, `
		INSERT INTO pending_pairing_requests
			(id, recorder_hostname, recorder_ip, requested_roles, status, request_note, expires_at)
		VALUES (?, ?, ?, ?, 'pending', ?, ?)`,
		req.ID,
		req.RecorderHostname,
		req.RecorderIP,
		rolesJSON,
		req.RequestNote,
		req.ExpiresAt.Format(time.RFC3339),
	)
	if err != nil {
		return nil, fmt.Errorf("pending/store: insert: %w", err)
	}
	return req, nil
}

// Get returns the pending request with the given id.
// Returns ErrPendingNotFound if absent.
func (s *PendingStore) Get(ctx context.Context, id string) (*PendingRequest, error) {
	row := s.db.QueryRowContext(ctx, `
		SELECT id, recorder_hostname, recorder_ip, requested_roles, status,
		       token_id, request_note, expires_at, created_at, decided_at, decided_by
		  FROM pending_pairing_requests WHERE id = ?`, id)
	return scanPending(row)
}

// ListPending returns all requests in 'pending' status that have not yet
// expired, ordered by created_at ascending.
func (s *PendingStore) ListPending(ctx context.Context) ([]*PendingRequest, error) {
	rows, err := s.db.QueryContext(ctx, `
		SELECT id, recorder_hostname, recorder_ip, requested_roles, status,
		       token_id, request_note, expires_at, created_at, decided_at, decided_by
		  FROM pending_pairing_requests
		 WHERE status = 'pending' AND expires_at > ?
		 ORDER BY created_at ASC`,
		time.Now().UTC().Format(time.RFC3339),
	)
	if err != nil {
		return nil, fmt.Errorf("pending/store: list: %w", err)
	}
	defer rows.Close()

	var out []*PendingRequest
	for rows.Next() {
		req, err := scanPendingRow(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, req)
	}
	return out, rows.Err()
}

// Approve transitions a pending request to 'approved', records the deciding
// admin, and stores the resulting token_id.
//
// Returns ErrPendingNotFound, ErrPendingAlreadyDecided, or
// ErrPendingExpired on failure.
func (s *PendingStore) Approve(ctx context.Context, id, tokenID string, decidedBy UserID) error {
	return s.decide(ctx, id, tokenID, PendingStatusApproved, decidedBy)
}

// Deny transitions a pending request to 'denied'.
// Returns ErrPendingNotFound, ErrPendingAlreadyDecided, or ErrPendingExpired.
func (s *PendingStore) Deny(ctx context.Context, id string, decidedBy UserID) error {
	return s.decide(ctx, id, "", PendingStatusDenied, decidedBy)
}

// MarkExpired bulk-expires all pending requests past their ExpiresAt.
// Returns the number of rows updated.
func (s *PendingStore) MarkExpired(ctx context.Context) (int64, error) {
	res, err := s.db.ExecContext(ctx, `
		UPDATE pending_pairing_requests
		   SET status = 'expired'
		 WHERE status = 'pending' AND expires_at <= ?`,
		time.Now().UTC().Format(time.RFC3339),
	)
	if err != nil {
		return 0, fmt.Errorf("pending/store: mark expired: %w", err)
	}
	return res.RowsAffected()
}

// --- internal ---------------------------------------------------------------

func (s *PendingStore) decide(ctx context.Context, id, tokenID string, status PendingStatus, by UserID) error {
	now := time.Now().UTC()
	res, err := s.db.ExecContext(ctx, `
		UPDATE pending_pairing_requests
		   SET status = ?, token_id = ?, decided_at = ?, decided_by = ?
		 WHERE id = ? AND status = 'pending' AND expires_at > ?`,
		string(status), tokenID, now.Format(time.RFC3339), string(by),
		id, now.Format(time.RFC3339),
	)
	if err != nil {
		return fmt.Errorf("pending/store: decide: %w", err)
	}
	n, err := res.RowsAffected()
	if err != nil {
		return fmt.Errorf("pending/store: decide rows: %w", err)
	}
	if n == 1 {
		return nil
	}

	// Determine why.
	req, err := s.Get(ctx, id)
	if err != nil {
		if errors.Is(err, ErrPendingNotFound) {
			return ErrPendingNotFound
		}
		return fmt.Errorf("pending/store: decide lookup: %w", err)
	}
	if req.Status != PendingStatusPending {
		return ErrPendingAlreadyDecided
	}
	if req.ExpiresAt.Before(now) {
		return ErrPendingExpired
	}
	return fmt.Errorf("pending/store: decide: unexpected state %q", req.Status)
}

func scanPending(row *sql.Row) (*PendingRequest, error) {
	var (
		pr          PendingRequest
		rolesJSON   string
		tokenID     sql.NullString
		decidedAt   sql.NullString
		decidedBy   string
		expiresRaw  string
		createdRaw  string
	)
	if err := row.Scan(
		&pr.ID,
		&pr.RecorderHostname,
		&pr.RecorderIP,
		&rolesJSON,
		(*string)(&pr.Status),
		&tokenID,
		&pr.RequestNote,
		&expiresRaw,
		&createdRaw,
		&decidedAt,
		&decidedBy,
	); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, ErrPendingNotFound
		}
		return nil, fmt.Errorf("pending/store: scan: %w", err)
	}
	return finalizePending(&pr, rolesJSON, tokenID, decidedAt, decidedBy, expiresRaw, createdRaw)
}

func scanPendingRow(rows *sql.Rows) (*PendingRequest, error) {
	var (
		pr          PendingRequest
		rolesJSON   string
		tokenID     sql.NullString
		decidedAt   sql.NullString
		decidedBy   string
		expiresRaw  string
		createdRaw  string
	)
	if err := rows.Scan(
		&pr.ID,
		&pr.RecorderHostname,
		&pr.RecorderIP,
		&rolesJSON,
		(*string)(&pr.Status),
		&tokenID,
		&pr.RequestNote,
		&expiresRaw,
		&createdRaw,
		&decidedAt,
		&decidedBy,
	); err != nil {
		return nil, fmt.Errorf("pending/store: scan row: %w", err)
	}
	return finalizePending(&pr, rolesJSON, tokenID, decidedAt, decidedBy, expiresRaw, createdRaw)
}

func finalizePending(pr *PendingRequest, rolesJSON string, tokenID, decidedAt sql.NullString, decidedBy, expiresRaw, createdRaw string) (*PendingRequest, error) {
	pr.RequestedRoles = unmarshalRoles(rolesJSON)
	if tokenID.Valid {
		pr.TokenID = tokenID.String
	}
	pr.DecidedBy = UserID(decidedBy)
	if t, err := time.Parse(time.RFC3339, expiresRaw); err == nil {
		pr.ExpiresAt = t.UTC()
	}
	if t, err := time.Parse(time.RFC3339, createdRaw); err == nil {
		pr.CreatedAt = t.UTC()
	}
	if decidedAt.Valid && decidedAt.String != "" {
		if t, err := time.Parse(time.RFC3339, decidedAt.String); err == nil {
			t = t.UTC()
			pr.DecidedAt = &t
		}
	}
	return pr, nil
}

// --- sentinel errors --------------------------------------------------------

var (
	// ErrPendingNotFound is returned when no request with the given ID exists.
	ErrPendingNotFound = errors.New("pending/pairing: request not found")
	// ErrPendingAlreadyDecided is returned when Approve or Deny is called on a
	// request that has already been approved, denied, or expired.
	ErrPendingAlreadyDecided = errors.New("pending/pairing: request already decided")
	// ErrPendingExpired is returned when a decision is attempted on an expired request.
	ErrPendingExpired = errors.New("pending/pairing: request expired")
)

// --- JSON role encoding helpers ---------------------------------------------

func marshalRoles(roles []string) string {
	if len(roles) == 0 {
		return `["recorder"]`
	}
	b, _ := marshalJSON(roles)
	return string(b)
}

func unmarshalRoles(s string) []string {
	var roles []string
	if err := unmarshalJSON([]byte(s), &roles); err != nil || len(roles) == 0 {
		return []string{"recorder"}
	}
	return roles
}

// marshalJSON and unmarshalJSON are thin wrappers used by marshalRoles /
// unmarshalRoles so the helpers are easy to test-stub if needed.
func marshalJSON(v any) ([]byte, error) {
	return json.Marshal(v)
}

func unmarshalJSON(data []byte, v any) error {
	return json.Unmarshal(data, v)
}
