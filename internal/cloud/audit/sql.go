package audit

import (
	"context"
	"database/sql"
	"fmt"
	"io"
	"strings"
	"time"
)

// Dialect selects the SQL flavour emitted by SQLRecorder. The cloud runs on
// Postgres; the SQLite flavour exists purely so unit tests can exercise the
// SQL code path without requiring a live Postgres instance.
type Dialect int

const (
	// DialectPostgres emits Postgres-flavoured SQL with `$1` placeholders
	// and references the partitioned `audit_log` parent declared by
	// KAI-218. This is the production dialect.
	DialectPostgres Dialect = iota
	// DialectSQLite emits SQLite-compatible SQL with `?` placeholders and
	// references an unpartitioned `audit_log` table. SQLite has no
	// declarative partitioning so the partition helpers are no-ops under
	// this dialect.
	DialectSQLite
)

// SQLRecorder is the production Recorder. It writes to the `audit_log`
// parent table declared in KAI-218 migrations and enforces tenant scoping
// by always binding TenantID as the first WHERE predicate.
//
// NOTE: as of the KAI-233 ship-date the KAI-218 migration set does **not**
// yet include the `audit_log` partitioned parent nor the
// `tenant_audit_retention` override table. Until KAI-218 lands those, call
// SQLRecorder.ApplyStubSchema to create a minimal in-schema equivalent for
// tests. When KAI-218 ships, delete ApplyStubSchema and point the SQL
// impl at the real parent table.
type SQLRecorder struct {
	db      *sql.DB
	dialect Dialect
}

// NewSQLRecorder wires a Recorder to a *sql.DB. The caller is responsible
// for running migrations (or ApplyStubSchema in tests) so the `audit_log`
// table exists before Record is called.
func NewSQLRecorder(db *sql.DB, dialect Dialect) *SQLRecorder {
	return &SQLRecorder{db: db, dialect: dialect}
}

// ApplyStubSchema creates a minimal `audit_log` table matching the Entry
// struct plus the `tenant_audit_retention` override table. It exists until
// KAI-218's migrations land the real partitioned parent; see README.md.
//
// Under DialectSQLite this is the only way to get a working table. Under
// DialectPostgres prefer the KAI-218 migrations.
func (s *SQLRecorder) ApplyStubSchema(ctx context.Context) error {
	var stmts []string
	switch s.dialect {
	case DialectSQLite:
		stmts = []string{sqliteAuditLogDDL, sqliteTenantRetentionDDL}
	case DialectPostgres:
		stmts = []string{postgresAuditLogStubDDL, postgresTenantRetentionDDL}
	default:
		return fmt.Errorf("audit: unknown dialect %d", s.dialect)
	}
	for _, stmt := range stmts {
		if _, err := s.db.ExecContext(ctx, stmt); err != nil {
			return fmt.Errorf("audit: apply stub schema: %w", err)
		}
	}
	return nil
}

const sqliteAuditLogDDL = `
CREATE TABLE IF NOT EXISTS audit_log (
    id                     TEXT PRIMARY KEY,
    tenant_id              TEXT NOT NULL,
    actor_user_id          TEXT NOT NULL,
    actor_agent            TEXT NOT NULL,
    impersonating_user_id  TEXT,
    impersonated_tenant_id TEXT,
    action                 TEXT NOT NULL,
    resource_type          TEXT NOT NULL,
    resource_id            TEXT NOT NULL DEFAULT '',
    result                 TEXT NOT NULL,
    error_code             TEXT,
    ip_address             TEXT NOT NULL DEFAULT '',
    user_agent             TEXT NOT NULL DEFAULT '',
    request_id             TEXT NOT NULL DEFAULT '',
    timestamp              TIMESTAMP NOT NULL
);
CREATE INDEX IF NOT EXISTS idx_audit_log_tenant_ts ON audit_log(tenant_id, timestamp DESC);
CREATE INDEX IF NOT EXISTS idx_audit_log_impersonated ON audit_log(impersonated_tenant_id, timestamp DESC);
CREATE INDEX IF NOT EXISTS idx_audit_log_actor ON audit_log(actor_user_id, timestamp DESC);
`

const sqliteTenantRetentionDDL = `
CREATE TABLE IF NOT EXISTS tenant_audit_retention (
    tenant_id TEXT PRIMARY KEY,
    retention_seconds INTEGER NOT NULL
);
`

// postgresAuditLogStubDDL is a non-partitioned version of the parent table
// sufficient for tests. KAI-218 replaces this with the real partitioned
// parent and monthly child partitions.
const postgresAuditLogStubDDL = `
CREATE TABLE IF NOT EXISTS audit_log (
    id                     TEXT PRIMARY KEY,
    tenant_id              TEXT NOT NULL,
    actor_user_id          TEXT NOT NULL,
    actor_agent            TEXT NOT NULL,
    impersonating_user_id  TEXT,
    impersonated_tenant_id TEXT,
    action                 TEXT NOT NULL,
    resource_type          TEXT NOT NULL,
    resource_id            TEXT NOT NULL DEFAULT '',
    result                 TEXT NOT NULL,
    error_code             TEXT,
    ip_address             TEXT NOT NULL DEFAULT '',
    user_agent             TEXT NOT NULL DEFAULT '',
    request_id             TEXT NOT NULL DEFAULT '',
    timestamp              TIMESTAMPTZ NOT NULL
);
CREATE INDEX IF NOT EXISTS idx_audit_log_tenant_ts ON audit_log(tenant_id, timestamp DESC);
CREATE INDEX IF NOT EXISTS idx_audit_log_impersonated ON audit_log(impersonated_tenant_id, timestamp DESC);
CREATE INDEX IF NOT EXISTS idx_audit_log_actor ON audit_log(actor_user_id, timestamp DESC);
`

const postgresTenantRetentionDDL = `
CREATE TABLE IF NOT EXISTS tenant_audit_retention (
    tenant_id         TEXT PRIMARY KEY,
    retention_seconds BIGINT NOT NULL
);
`

func (s *SQLRecorder) placeholder(n int) string {
	if s.dialect == DialectPostgres {
		return fmt.Sprintf("$%d", n)
	}
	return "?"
}

// Record inserts the entry into `audit_log`. It does not fail-open: if the
// insert fails the caller must propagate the error so the handler can
// refuse to commit the underlying action (SOC 2 CC4.1).
func (s *SQLRecorder) Record(ctx context.Context, entry Entry) error {
	if err := entry.Validate(); err != nil {
		return err
	}
	if entry.ID == "" {
		entry.ID = newID()
	}
	query := fmt.Sprintf(`INSERT INTO audit_log (
        id, tenant_id, actor_user_id, actor_agent,
        impersonating_user_id, impersonated_tenant_id,
        action, resource_type, resource_id,
        result, error_code, ip_address, user_agent, request_id, timestamp
    ) VALUES (%s,%s,%s,%s,%s,%s,%s,%s,%s,%s,%s,%s,%s,%s,%s)`,
		s.placeholder(1), s.placeholder(2), s.placeholder(3), s.placeholder(4),
		s.placeholder(5), s.placeholder(6), s.placeholder(7), s.placeholder(8),
		s.placeholder(9), s.placeholder(10), s.placeholder(11), s.placeholder(12),
		s.placeholder(13), s.placeholder(14), s.placeholder(15),
	)
	_, err := s.db.ExecContext(ctx, query,
		entry.ID, entry.TenantID, entry.ActorUserID, string(entry.ActorAgent),
		nullString(entry.ImpersonatingUserID), nullString(entry.ImpersonatedTenantID),
		entry.Action, entry.ResourceType, entry.ResourceID,
		string(entry.Result), nullString(entry.ErrorCode),
		entry.IPAddress, entry.UserAgent, entry.RequestID,
		entry.Timestamp.UTC(),
	)
	if err != nil {
		return fmt.Errorf("audit: insert: %w", err)
	}
	return nil
}

func nullString(p *string) interface{} {
	if p == nil {
		return nil
	}
	return *p
}

// Query runs a tenant-scoped SELECT. The first predicate is always
// `tenant_id = ?` so the query planner, EXPLAIN, and any reviewer can see
// the scope at a glance (Seam #4).
func (s *SQLRecorder) Query(ctx context.Context, filter QueryFilter) ([]Entry, error) {
	if err := filter.Validate(); err != nil {
		return nil, err
	}

	var (
		conds []string
		args  []interface{}
		n     = 1
	)

	if filter.IncludeImpersonatedTenant {
		conds = append(conds, fmt.Sprintf("(tenant_id = %s OR impersonated_tenant_id = %s)", s.placeholder(n), s.placeholder(n+1)))
		args = append(args, filter.TenantID, filter.TenantID)
		n += 2
	} else {
		conds = append(conds, fmt.Sprintf("tenant_id = %s", s.placeholder(n)))
		args = append(args, filter.TenantID)
		n++
	}

	if filter.ActorUserID != "" {
		conds = append(conds, fmt.Sprintf("actor_user_id = %s", s.placeholder(n)))
		args = append(args, filter.ActorUserID)
		n++
	}
	if filter.ActionPattern != "" {
		// Translate glob-like '*' to SQL LIKE '%'.
		like := strings.ReplaceAll(filter.ActionPattern, "*", "%")
		conds = append(conds, fmt.Sprintf("action LIKE %s", s.placeholder(n)))
		args = append(args, like)
		n++
	}
	if filter.ResourceType != "" {
		conds = append(conds, fmt.Sprintf("resource_type = %s", s.placeholder(n)))
		args = append(args, filter.ResourceType)
		n++
	}
	if filter.Result != "" {
		conds = append(conds, fmt.Sprintf("result = %s", s.placeholder(n)))
		args = append(args, string(filter.Result))
		n++
	}
	if !filter.Since.IsZero() {
		conds = append(conds, fmt.Sprintf("timestamp >= %s", s.placeholder(n)))
		args = append(args, filter.Since.UTC())
		n++
	}
	if !filter.Until.IsZero() {
		conds = append(conds, fmt.Sprintf("timestamp <= %s", s.placeholder(n)))
		args = append(args, filter.Until.UTC())
		n++
	}
	if filter.Cursor != "" {
		// Cursor paginates by (timestamp, id) tuple. We fetch the cursor's
		// timestamp in a sub-select so callers only pass the ID.
		conds = append(conds, fmt.Sprintf(
			"(timestamp, id) < (SELECT timestamp, id FROM audit_log WHERE id = %s)",
			s.placeholder(n),
		))
		args = append(args, filter.Cursor)
		n++
	}

	q := "SELECT id, tenant_id, actor_user_id, actor_agent, impersonating_user_id, impersonated_tenant_id, action, resource_type, resource_id, result, error_code, ip_address, user_agent, request_id, timestamp FROM audit_log WHERE " +
		strings.Join(conds, " AND ") + " ORDER BY timestamp DESC, id DESC"
	if filter.Limit > 0 {
		q += fmt.Sprintf(" LIMIT %d", filter.Limit)
	}

	rows, err := s.db.QueryContext(ctx, q, args...)
	if err != nil {
		return nil, fmt.Errorf("audit: query: %w", err)
	}
	defer rows.Close()

	var out []Entry
	for rows.Next() {
		var (
			e           Entry
			agent       string
			res         string
			imperUser   sql.NullString
			imperTenant sql.NullString
			errCode     sql.NullString
			ts          time.Time
		)
		if err := rows.Scan(
			&e.ID, &e.TenantID, &e.ActorUserID, &agent,
			&imperUser, &imperTenant,
			&e.Action, &e.ResourceType, &e.ResourceID,
			&res, &errCode,
			&e.IPAddress, &e.UserAgent, &e.RequestID,
			&ts,
		); err != nil {
			return nil, fmt.Errorf("audit: scan: %w", err)
		}
		e.ActorAgent = ActorAgent(agent)
		e.Result = Result(res)
		if imperUser.Valid {
			s := imperUser.String
			e.ImpersonatingUserID = &s
		}
		if imperTenant.Valid {
			s := imperTenant.String
			e.ImpersonatedTenantID = &s
		}
		if errCode.Valid {
			s := errCode.String
			e.ErrorCode = &s
		}
		e.Timestamp = ts.UTC()
		// Defense in depth: the WHERE clause already scoped the query, but
		// if a future refactor relaxes the predicate by accident this check
		// catches the bug in tests.
		if !matchesTenant(e, filter) {
			return nil, ErrTenantMismatch
		}
		out = append(out, e)
	}
	return out, rows.Err()
}

// Export streams Query results in the requested format.
func (s *SQLRecorder) Export(ctx context.Context, filter QueryFilter, format ExportFormat, w io.Writer) error {
	return exportEntries(ctx, s, filter, format, w)
}
