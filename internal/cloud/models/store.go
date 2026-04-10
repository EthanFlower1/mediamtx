package models

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"

	clouddb "github.com/bluenviron/mediamtx/internal/cloud/db"
)

// Store is the model registry data access interface.
type Store interface {
	Create(ctx context.Context, input CreateModelInput) (Model, error)
	Get(ctx context.Context, tenantID, id string) (Model, error)
	List(ctx context.Context, filter ListFilter) ([]Model, error)
	UpdateApproval(ctx context.Context, input UpdateApprovalInput) (Model, error)
	UpdateMetrics(ctx context.Context, tenantID, id string, metrics json.RawMessage) error
	Delete(ctx context.Context, tenantID, id string) error
	ResolveApproved(ctx context.Context, tenantID, name string) (Model, error)
}

// -----------------------------------------------------------------------
// modelStore — SQL-backed Store implementation
// -----------------------------------------------------------------------

type modelStore struct {
	db *clouddb.DB
}

// NewStore constructs a SQL-backed model Store.
func NewStore(db *clouddb.DB) Store {
	return &modelStore{db: db}
}

func (s *modelStore) ph(i int) string {
	return s.db.Placeholder(i)
}

// -----------------------------------------------------------------------
// Create
// -----------------------------------------------------------------------

func (s *modelStore) Create(ctx context.Context, input CreateModelInput) (Model, error) {
	if input.TenantID == "" {
		return Model{}, ErrInvalidTenantID
	}

	id := uuid.New().String()
	now := time.Now().UTC()
	metrics := json.RawMessage("{}")

	q := fmt.Sprintf(
		`INSERT INTO models
		    (id, tenant_id, name, version, framework, file_ref, file_sha256,
		     size_bytes, metrics, approval_state, owner_user_id, created_at, updated_at)
		 VALUES (%s,%s,%s,%s,%s,%s,%s,%s,%s,%s,%s,%s,%s)`,
		s.ph(1), s.ph(2), s.ph(3), s.ph(4), s.ph(5), s.ph(6), s.ph(7),
		s.ph(8), s.ph(9), s.ph(10), s.ph(11), s.ph(12), s.ph(13),
	)

	_, err := s.db.ExecContext(ctx, q,
		id, input.TenantID, input.Name, input.Version, string(input.Framework),
		input.FileRef, input.FileSHA256, input.SizeBytes,
		string(metrics), string(StateDraft), input.OwnerUserID, now, now,
	)
	if err != nil {
		if isDuplicateErr(err) {
			return Model{}, ErrDuplicateVersion
		}
		return Model{}, fmt.Errorf("models.Create: %w", err)
	}

	return Model{
		ID:            id,
		TenantID:      input.TenantID,
		Name:          input.Name,
		Version:       input.Version,
		Framework:     input.Framework,
		FileRef:       input.FileRef,
		FileSHA256:    input.FileSHA256,
		SizeBytes:     input.SizeBytes,
		Metrics:       metrics,
		ApprovalState: StateDraft,
		OwnerUserID:   input.OwnerUserID,
		CreatedAt:     now,
		UpdatedAt:     now,
	}, nil
}

// -----------------------------------------------------------------------
// Get
// -----------------------------------------------------------------------

func (s *modelStore) Get(ctx context.Context, tenantID, id string) (Model, error) {
	if tenantID == "" {
		return Model{}, ErrInvalidTenantID
	}
	if id == "" {
		return Model{}, ErrInvalidID
	}
	q := fmt.Sprintf(
		`SELECT id, tenant_id, name, version, framework, file_ref, file_sha256,
		        size_bytes, metrics, approval_state, approved_by, approved_at,
		        owner_user_id, created_at, updated_at
		 FROM models
		 WHERE tenant_id = %s AND id = %s`,
		s.ph(1), s.ph(2),
	)
	row := s.db.QueryRowContext(ctx, q, tenantID, id)
	m, err := scanModel(row)
	if errors.Is(err, sql.ErrNoRows) {
		return Model{}, ErrNotFound
	}
	if err != nil {
		return Model{}, fmt.Errorf("models.Get: %w", err)
	}
	return m, nil
}

// -----------------------------------------------------------------------
// List
// -----------------------------------------------------------------------

func (s *modelStore) List(ctx context.Context, filter ListFilter) ([]Model, error) {
	if filter.TenantID == "" {
		return nil, ErrInvalidTenantID
	}

	args := []any{filter.TenantID}
	idx := 2

	where := fmt.Sprintf("tenant_id = %s", s.ph(1))

	if filter.ApprovalState != nil {
		where += fmt.Sprintf(" AND approval_state = %s", s.ph(idx))
		args = append(args, string(*filter.ApprovalState))
		idx++
	}
	if filter.OwnerUserID != nil {
		where += fmt.Sprintf(" AND owner_user_id = %s", s.ph(idx))
		args = append(args, *filter.OwnerUserID)
		idx++
	}

	q := fmt.Sprintf(
		`SELECT id, tenant_id, name, version, framework, file_ref, file_sha256,
		        size_bytes, metrics, approval_state, approved_by, approved_at,
		        owner_user_id, created_at, updated_at
		 FROM models
		 WHERE %s
		 ORDER BY created_at ASC`,
		where,
	)

	rows, err := s.db.QueryContext(ctx, q, args...)
	if err != nil {
		return nil, fmt.Errorf("models.List: %w", err)
	}
	defer rows.Close()
	return scanModels(rows)
}

// -----------------------------------------------------------------------
// UpdateApproval
// -----------------------------------------------------------------------

func (s *modelStore) UpdateApproval(ctx context.Context, input UpdateApprovalInput) (Model, error) {
	if input.TenantID == "" {
		return Model{}, ErrInvalidTenantID
	}
	if input.ModelID == "" {
		return Model{}, ErrInvalidID
	}

	// Fetch current state to validate the transition.
	current, err := s.Get(ctx, input.TenantID, input.ModelID)
	if err != nil {
		return Model{}, err
	}

	// Validate state transition.
	allowed, ok := ValidTransitions[current.ApprovalState]
	if !ok {
		return Model{}, ErrInvalidTransition
	}
	valid := false
	for _, s := range allowed {
		if s == input.NewState {
			valid = true
			break
		}
	}
	if !valid {
		return Model{}, ErrInvalidTransition
	}

	now := time.Now().UTC()
	var approvedBy *string
	var approvedAt *time.Time
	if input.NewState == StateApproved {
		approvedBy = &input.ApprovedBy
		approvedAt = &now
	}

	q := fmt.Sprintf(
		`UPDATE models
		 SET approval_state = %s, approved_by = %s, approved_at = %s, updated_at = %s
		 WHERE tenant_id = %s AND id = %s`,
		s.ph(1), s.ph(2), s.ph(3), s.ph(4), s.ph(5), s.ph(6),
	)

	res, err := s.db.ExecContext(ctx, q,
		string(input.NewState), nullStringPtr(approvedBy), nullTimePtr(approvedAt),
		now, input.TenantID, input.ModelID,
	)
	if err != nil {
		return Model{}, fmt.Errorf("models.UpdateApproval: %w", err)
	}
	n, err := res.RowsAffected()
	if err == nil && n == 0 {
		return Model{}, ErrNotFound
	}

	// Return the updated model.
	return s.Get(ctx, input.TenantID, input.ModelID)
}

// -----------------------------------------------------------------------
// UpdateMetrics
// -----------------------------------------------------------------------

func (s *modelStore) UpdateMetrics(ctx context.Context, tenantID, id string, metrics json.RawMessage) error {
	if tenantID == "" {
		return ErrInvalidTenantID
	}
	if id == "" {
		return ErrInvalidID
	}
	now := time.Now().UTC()
	q := fmt.Sprintf(
		`UPDATE models SET metrics = %s, updated_at = %s WHERE tenant_id = %s AND id = %s`,
		s.ph(1), s.ph(2), s.ph(3), s.ph(4),
	)
	res, err := s.db.ExecContext(ctx, q, string(metrics), now, tenantID, id)
	if err != nil {
		return fmt.Errorf("models.UpdateMetrics: %w", err)
	}
	n, err := res.RowsAffected()
	if err == nil && n == 0 {
		return ErrNotFound
	}
	return err
}

// -----------------------------------------------------------------------
// Delete
// -----------------------------------------------------------------------

func (s *modelStore) Delete(ctx context.Context, tenantID, id string) error {
	if tenantID == "" {
		return ErrInvalidTenantID
	}
	if id == "" {
		return ErrInvalidID
	}
	q := fmt.Sprintf(
		`DELETE FROM models WHERE tenant_id = %s AND id = %s`,
		s.ph(1), s.ph(2),
	)
	res, err := s.db.ExecContext(ctx, q, tenantID, id)
	if err != nil {
		return fmt.Errorf("models.Delete: %w", err)
	}
	n, err := res.RowsAffected()
	if err == nil && n == 0 {
		return ErrNotFound
	}
	return err
}

// -----------------------------------------------------------------------
// ResolveApproved
// -----------------------------------------------------------------------

func (s *modelStore) ResolveApproved(ctx context.Context, tenantID, name string) (Model, error) {
	if tenantID == "" {
		return Model{}, ErrInvalidTenantID
	}
	q := fmt.Sprintf(
		`SELECT id, tenant_id, name, version, framework, file_ref, file_sha256,
		        size_bytes, metrics, approval_state, approved_by, approved_at,
		        owner_user_id, created_at, updated_at
		 FROM models
		 WHERE tenant_id = %s AND name = %s AND approval_state = %s
		 ORDER BY created_at DESC
		 LIMIT 1`,
		s.ph(1), s.ph(2), s.ph(3),
	)
	row := s.db.QueryRowContext(ctx, q, tenantID, name, string(StateApproved))
	m, err := scanModel(row)
	if errors.Is(err, sql.ErrNoRows) {
		return Model{}, ErrNotFound
	}
	if err != nil {
		return Model{}, fmt.Errorf("models.ResolveApproved: %w", err)
	}
	return m, nil
}

// -----------------------------------------------------------------------
// scan helpers
// -----------------------------------------------------------------------

type modelScanner interface {
	Scan(dest ...any) error
}

func scanModel(row modelScanner) (Model, error) {
	var m Model
	var metrics string
	var approvedBy sql.NullString
	var approvedAt sql.NullTime

	err := row.Scan(
		&m.ID, &m.TenantID, &m.Name, &m.Version, &m.Framework,
		&m.FileRef, &m.FileSHA256, &m.SizeBytes,
		&metrics, &m.ApprovalState, &approvedBy, &approvedAt,
		&m.OwnerUserID, &m.CreatedAt, &m.UpdatedAt,
	)
	if err != nil {
		return Model{}, err
	}
	m.Metrics = json.RawMessage(metrics)
	if approvedBy.Valid {
		m.ApprovedBy = &approvedBy.String
	}
	if approvedAt.Valid {
		t := approvedAt.Time
		m.ApprovedAt = &t
	}
	return m, nil
}

func scanModels(rows *sql.Rows) ([]Model, error) {
	var out []Model
	for rows.Next() {
		m, err := scanModel(rows)
		if err != nil {
			return nil, fmt.Errorf("models scan: %w", err)
		}
		out = append(out, m)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("models rows: %w", err)
	}
	if out == nil {
		out = []Model{}
	}
	return out, nil
}

// -----------------------------------------------------------------------
// null-coercion helpers
// -----------------------------------------------------------------------

func nullStringPtr(s *string) sql.NullString {
	if s == nil {
		return sql.NullString{}
	}
	return sql.NullString{String: *s, Valid: true}
}

func nullTimePtr(t *time.Time) sql.NullTime {
	if t == nil {
		return sql.NullTime{}
	}
	return sql.NullTime{Time: *t, Valid: true}
}

// isDuplicateErr detects UNIQUE constraint violations for both Postgres and
// SQLite. This is a simple string-match approach consistent with the rest of
// the codebase.
func isDuplicateErr(err error) bool {
	if err == nil {
		return false
	}
	msg := err.Error()
	return strings.Contains(msg, "UNIQUE constraint failed") ||
		strings.Contains(msg, "duplicate key value violates unique constraint")
}
