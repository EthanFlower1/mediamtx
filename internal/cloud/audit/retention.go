package audit

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"
)

// RetentionStore manages per-tenant overrides of the default 7-year
// retention window. Most tenants inherit DefaultRetention; regulated
// tenants (eg. finance, medical) may contractually require longer windows.
//
// The backing table, `tenant_audit_retention`, is declared by
// SQLRecorder.ApplyStubSchema today and will migrate to KAI-218 when that
// work lands.
type RetentionStore struct {
	db      *sql.DB
	dialect Dialect
}

// NewRetentionStore wires a store to a *sql.DB.
func NewRetentionStore(db *sql.DB, dialect Dialect) *RetentionStore {
	return &RetentionStore{db: db, dialect: dialect}
}

func (r *RetentionStore) placeholder(n int) string {
	if r.dialect == DialectPostgres {
		return fmt.Sprintf("$%d", n)
	}
	return "?"
}

// Set stores (or updates) the retention override for a tenant. retention
// must be strictly positive.
func (r *RetentionStore) Set(ctx context.Context, tenantID string, retention time.Duration) error {
	if tenantID == "" {
		return errors.New("audit: tenant_id is required")
	}
	if retention <= 0 {
		return errors.New("audit: retention must be positive")
	}

	var q string
	switch r.dialect {
	case DialectPostgres:
		q = "INSERT INTO tenant_audit_retention (tenant_id, retention_seconds) VALUES ($1, $2) ON CONFLICT (tenant_id) DO UPDATE SET retention_seconds = EXCLUDED.retention_seconds"
	case DialectSQLite:
		q = "INSERT INTO tenant_audit_retention (tenant_id, retention_seconds) VALUES (?, ?) ON CONFLICT(tenant_id) DO UPDATE SET retention_seconds = excluded.retention_seconds"
	default:
		return fmt.Errorf("audit: unknown dialect %d", r.dialect)
	}
	_, err := r.db.ExecContext(ctx, q, tenantID, int64(retention.Seconds()))
	return err
}

// Get returns the effective retention for a tenant. If no override exists
// DefaultRetention is returned.
func (r *RetentionStore) Get(ctx context.Context, tenantID string) (time.Duration, error) {
	if tenantID == "" {
		return 0, errors.New("audit: tenant_id is required")
	}
	q := fmt.Sprintf("SELECT retention_seconds FROM tenant_audit_retention WHERE tenant_id = %s", r.placeholder(1))
	var seconds int64
	err := r.db.QueryRowContext(ctx, q, tenantID).Scan(&seconds)
	if errors.Is(err, sql.ErrNoRows) {
		return DefaultRetention, nil
	}
	if err != nil {
		return 0, err
	}
	return time.Duration(seconds) * time.Second, nil
}
