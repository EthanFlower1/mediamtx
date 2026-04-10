// Package apikeys implements the APIKeyStore interface defined in
// package publicapi (KAI-399 contract). It persists keys in the cloud
// database and emits per-key audit log entries for every lifecycle event.
package apikeys

import (
	"context"
	"database/sql"
	"fmt"
	"time"

	"github.com/google/uuid"
)

// AuditAction identifies the lifecycle event recorded in the audit log.
type AuditAction string

const (
	// AuditCreate is logged when a new key is created.
	AuditCreate AuditAction = "create"
	// AuditRotate is logged when a key is rotated (both old and new entries).
	AuditRotate AuditAction = "rotate"
	// AuditRevoke is logged when a key is revoked.
	AuditRevoke AuditAction = "revoke"
	// AuditAuthenticate is logged on successful key validation.
	AuditAuthenticate AuditAction = "authenticate"
	// AuditAuthFail is logged on failed key validation.
	AuditAuthFail AuditAction = "auth_fail"
)

// AuditEntry is a single per-key audit record.
type AuditEntry struct {
	ID        string
	KeyID     string
	TenantID  string
	Action    AuditAction
	ActorID   string
	IPAddress string
	UserAgent string
	Metadata  string // JSON
	CreatedAt time.Time
}

// recordAudit writes a single audit log entry. It uses the store's dialect
// to emit the correct placeholder syntax.
func (s *Store) recordAudit(ctx context.Context, tx *sql.Tx, entry AuditEntry) error {
	if entry.ID == "" {
		entry.ID = uuid.New().String()
	}
	if entry.CreatedAt.IsZero() {
		entry.CreatedAt = time.Now().UTC()
	}
	if entry.Metadata == "" {
		entry.Metadata = "{}"
	}

	var query string
	switch s.dialect {
	case DialectPostgres:
		query = `INSERT INTO api_key_audit_log (id, key_id, tenant_id, action, actor_id, ip_address, user_agent, metadata, created_at)
			VALUES ($1, $2, $3, $4, $5, $6, $7, $8::jsonb, $9)`
	default:
		query = `INSERT INTO api_key_audit_log (id, key_id, tenant_id, action, actor_id, ip_address, user_agent, metadata, created_at)
			VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)`
	}

	var execer interface {
		ExecContext(ctx context.Context, query string, args ...any) (sql.Result, error)
	}
	if tx != nil {
		execer = tx
	} else {
		execer = s.db
	}

	_, err := execer.ExecContext(ctx, query,
		entry.ID,
		entry.KeyID,
		entry.TenantID,
		string(entry.Action),
		entry.ActorID,
		entry.IPAddress,
		entry.UserAgent,
		entry.Metadata,
		entry.CreatedAt,
	)
	if err != nil {
		return fmt.Errorf("apikeys: record audit: %w", err)
	}
	return nil
}

// ListAuditLog returns audit entries for a specific key, ordered by
// created_at desc. Requires tenant_id for scoping.
func (s *Store) ListAuditLog(ctx context.Context, tenantID, keyID string, limit int) ([]AuditEntry, error) {
	if limit <= 0 || limit > 1000 {
		limit = 100
	}

	var query string
	switch s.dialect {
	case DialectPostgres:
		query = `SELECT id, key_id, tenant_id, action, actor_id, ip_address, user_agent, metadata, created_at
			FROM api_key_audit_log WHERE tenant_id = $1 AND key_id = $2 ORDER BY created_at DESC LIMIT $3`
	default:
		query = `SELECT id, key_id, tenant_id, action, actor_id, ip_address, user_agent, metadata, created_at
			FROM api_key_audit_log WHERE tenant_id = ? AND key_id = ? ORDER BY created_at DESC LIMIT ?`
	}

	rows, err := s.db.QueryContext(ctx, query, tenantID, keyID, limit)
	if err != nil {
		return nil, fmt.Errorf("apikeys: list audit: %w", err)
	}
	defer rows.Close()

	var entries []AuditEntry
	for rows.Next() {
		var e AuditEntry
		var action string
		if err := rows.Scan(&e.ID, &e.KeyID, &e.TenantID, &action, &e.ActorID,
			&e.IPAddress, &e.UserAgent, &e.Metadata, &e.CreatedAt); err != nil {
			return nil, fmt.Errorf("apikeys: scan audit entry: %w", err)
		}
		e.Action = AuditAction(action)
		entries = append(entries, e)
	}
	return entries, rows.Err()
}
