package metering

import (
	"context"
	"database/sql"
	"fmt"
	"strings"
	"time"
)

// Store is the tenant-scoped read/write surface over usage_events and
// usage_aggregates. It mirrors internal/cloud/audit.SQLRecorder in shape and
// follows the same rule: the first WHERE predicate is always tenant_id.
type Store struct {
	db      *sql.DB
	dialect Dialect
}

// NewStore wires a Store to a *sql.DB. The caller is responsible for having
// run migration 0016 (or ApplyStubSchema in tests) before calling Record.
func NewStore(db *sql.DB, dialect Dialect) (*Store, error) {
	if dialect != DialectPostgres && dialect != DialectSQLite {
		return nil, ErrUnknownDialect
	}
	return &Store{db: db, dialect: dialect}, nil
}

// ApplyStubSchema creates unpartitioned usage_events + usage_aggregates
// tables matching migration 0016's SQLite variant. It exists for tests so
// they do not need to run the whole migration runner.
func (s *Store) ApplyStubSchema(ctx context.Context) error {
	var stmts []string
	switch s.dialect {
	case DialectSQLite:
		stmts = []string{sqliteUsageEventsDDL, sqliteUsageAggregatesDDL}
	case DialectPostgres:
		stmts = []string{postgresUsageEventsStubDDL, postgresUsageAggregatesDDL}
	default:
		return ErrUnknownDialect
	}
	for _, stmt := range stmts {
		if _, err := s.db.ExecContext(ctx, stmt); err != nil {
			return fmt.Errorf("metering: apply stub schema: %w", err)
		}
	}
	return nil
}

const sqliteUsageEventsDDL = `
CREATE TABLE IF NOT EXISTS usage_events (
    id          INTEGER PRIMARY KEY AUTOINCREMENT,
    tenant_id   TEXT NOT NULL,
    ts          TIMESTAMP NOT NULL,
    metric      TEXT NOT NULL,
    value       DOUBLE PRECISION NOT NULL
);
CREATE INDEX IF NOT EXISTS idx_usage_events_tenant_metric_ts ON usage_events(tenant_id, metric, ts);
`

const sqliteUsageAggregatesDDL = `
CREATE TABLE IF NOT EXISTS usage_aggregates (
    tenant_id      TEXT NOT NULL,
    period_start   TIMESTAMP NOT NULL,
    period_end     TIMESTAMP NOT NULL,
    metric         TEXT NOT NULL,
    sum            DOUBLE PRECISION NOT NULL,
    max            DOUBLE PRECISION NOT NULL,
    snapshot_count INTEGER NOT NULL,
    PRIMARY KEY (tenant_id, period_start, metric)
);
CREATE INDEX IF NOT EXISTS idx_usage_aggregates_tenant_period ON usage_aggregates(tenant_id, period_start, period_end);
`

// postgresUsageEventsStubDDL is a non-partitioned version of usage_events
// suitable for tests. The production migration 0016 creates a pg_partman
// parent; this stub is flat.
const postgresUsageEventsStubDDL = `
CREATE TABLE IF NOT EXISTS usage_events (
    id          BIGSERIAL PRIMARY KEY,
    tenant_id   TEXT NOT NULL,
    ts          TIMESTAMPTZ NOT NULL,
    metric      TEXT NOT NULL,
    value       DOUBLE PRECISION NOT NULL
);
CREATE INDEX IF NOT EXISTS idx_usage_events_tenant_metric_ts ON usage_events(tenant_id, metric, ts);
`

const postgresUsageAggregatesDDL = `
CREATE TABLE IF NOT EXISTS usage_aggregates (
    tenant_id      TEXT NOT NULL,
    period_start   TIMESTAMPTZ NOT NULL,
    period_end     TIMESTAMPTZ NOT NULL,
    metric         TEXT NOT NULL,
    sum            DOUBLE PRECISION NOT NULL,
    max            DOUBLE PRECISION NOT NULL,
    snapshot_count INTEGER NOT NULL,
    PRIMARY KEY (tenant_id, period_start, metric)
);
CREATE INDEX IF NOT EXISTS idx_usage_aggregates_tenant_period ON usage_aggregates(tenant_id, period_start, period_end);
`

func (s *Store) placeholder(n int) string {
	if s.dialect == DialectPostgres {
		return fmt.Sprintf("$%d", n)
	}
	return "?"
}

// Record inserts a single usage event. All validation is up-front and
// fail-closed: missing tenant, unknown metric, or negative value are all
// refused before touching the database.
func (s *Store) Record(ctx context.Context, e Event) error {
	if e.TenantID == "" {
		return ErrMissingTenant
	}
	if !e.Metric.Valid() {
		return fmt.Errorf("%w: %q", ErrUnknownMetric, e.Metric)
	}
	if e.Value < 0 {
		return ErrNegativeValue
	}
	if e.Timestamp.IsZero() {
		e.Timestamp = time.Now().UTC()
	}
	query := fmt.Sprintf(
		`INSERT INTO usage_events (tenant_id, ts, metric, value) VALUES (%s, %s, %s, %s)`,
		s.placeholder(1), s.placeholder(2), s.placeholder(3), s.placeholder(4),
	)
	if _, err := s.db.ExecContext(ctx, query,
		e.TenantID, e.Timestamp.UTC(), string(e.Metric), e.Value,
	); err != nil {
		return fmt.Errorf("metering: insert event: %w", err)
	}
	return nil
}

// ListEvents returns raw events for the filter's tenant. TenantID is
// mandatory. The first WHERE predicate is always tenant_id.
func (s *Store) ListEvents(ctx context.Context, f QueryFilter) ([]Event, error) {
	if f.TenantID == "" {
		return nil, ErrMissingTenant
	}

	var (
		conds = []string{fmt.Sprintf("tenant_id = %s", s.placeholder(1))}
		args  = []interface{}{f.TenantID}
		n     = 2
	)
	if f.Metric != "" {
		if !f.Metric.Valid() {
			return nil, fmt.Errorf("%w: %q", ErrUnknownMetric, f.Metric)
		}
		conds = append(conds, fmt.Sprintf("metric = %s", s.placeholder(n)))
		args = append(args, string(f.Metric))
		n++
	}
	if !f.Since.IsZero() {
		conds = append(conds, fmt.Sprintf("ts >= %s", s.placeholder(n)))
		args = append(args, f.Since.UTC())
		n++
	}
	if !f.Until.IsZero() {
		conds = append(conds, fmt.Sprintf("ts < %s", s.placeholder(n)))
		args = append(args, f.Until.UTC())
		n++
	}

	q := "SELECT tenant_id, ts, metric, value FROM usage_events WHERE " +
		strings.Join(conds, " AND ") + " ORDER BY ts ASC"
	if f.Limit > 0 {
		q += fmt.Sprintf(" LIMIT %d", f.Limit)
	}

	rows, err := s.db.QueryContext(ctx, q, args...)
	if err != nil {
		return nil, fmt.Errorf("metering: list events: %w", err)
	}
	defer rows.Close()

	var out []Event
	for rows.Next() {
		var (
			e      Event
			metric string
			ts     time.Time
		)
		if err := rows.Scan(&e.TenantID, &ts, &metric, &e.Value); err != nil {
			return nil, fmt.Errorf("metering: scan event: %w", err)
		}
		e.Timestamp = ts.UTC()
		e.Metric = Metric(metric)
		out = append(out, e)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("metering: iterate events: %w", err)
	}
	return out, nil
}

// ListAggregates returns rollup rows for the filter's tenant. TenantID is
// mandatory. The first WHERE predicate is always tenant_id.
func (s *Store) ListAggregates(ctx context.Context, f QueryFilter) ([]Aggregate, error) {
	if f.TenantID == "" {
		return nil, ErrMissingTenant
	}

	var (
		conds = []string{fmt.Sprintf("tenant_id = %s", s.placeholder(1))}
		args  = []interface{}{f.TenantID}
		n     = 2
	)
	if f.Metric != "" {
		if !f.Metric.Valid() {
			return nil, fmt.Errorf("%w: %q", ErrUnknownMetric, f.Metric)
		}
		conds = append(conds, fmt.Sprintf("metric = %s", s.placeholder(n)))
		args = append(args, string(f.Metric))
		n++
	}
	if !f.Since.IsZero() {
		conds = append(conds, fmt.Sprintf("period_start >= %s", s.placeholder(n)))
		args = append(args, f.Since.UTC())
		n++
	}
	if !f.Until.IsZero() {
		conds = append(conds, fmt.Sprintf("period_end <= %s", s.placeholder(n)))
		args = append(args, f.Until.UTC())
		n++
	}

	q := "SELECT tenant_id, period_start, period_end, metric, sum, max, snapshot_count FROM usage_aggregates WHERE " +
		strings.Join(conds, " AND ") + " ORDER BY period_start ASC, metric ASC"
	if f.Limit > 0 {
		q += fmt.Sprintf(" LIMIT %d", f.Limit)
	}

	rows, err := s.db.QueryContext(ctx, q, args...)
	if err != nil {
		return nil, fmt.Errorf("metering: list aggregates: %w", err)
	}
	defer rows.Close()

	var out []Aggregate
	for rows.Next() {
		var (
			a      Aggregate
			metric string
			ps, pe time.Time
		)
		if err := rows.Scan(&a.TenantID, &ps, &pe, &metric, &a.Sum, &a.Max, &a.SnapshotCount); err != nil {
			return nil, fmt.Errorf("metering: scan aggregate: %w", err)
		}
		a.PeriodStart = ps.UTC()
		a.PeriodEnd = pe.UTC()
		a.Metric = Metric(metric)
		out = append(out, a)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("metering: iterate aggregates: %w", err)
	}
	return out, nil
}

// upsertAggregate is called by Aggregator.Run. It is idempotent for a given
// (tenant_id, period_start, metric) triple.
func (s *Store) upsertAggregate(ctx context.Context, a Aggregate) error {
	var query string
	switch s.dialect {
	case DialectPostgres:
		query = `INSERT INTO usage_aggregates (tenant_id, period_start, period_end, metric, sum, max, snapshot_count)
VALUES ($1, $2, $3, $4, $5, $6, $7)
ON CONFLICT (tenant_id, period_start, metric)
DO UPDATE SET period_end = EXCLUDED.period_end, sum = EXCLUDED.sum, max = EXCLUDED.max, snapshot_count = EXCLUDED.snapshot_count`
	case DialectSQLite:
		query = `INSERT INTO usage_aggregates (tenant_id, period_start, period_end, metric, sum, max, snapshot_count)
VALUES (?, ?, ?, ?, ?, ?, ?)
ON CONFLICT (tenant_id, period_start, metric)
DO UPDATE SET period_end = excluded.period_end, sum = excluded.sum, max = excluded.max, snapshot_count = excluded.snapshot_count`
	default:
		return ErrUnknownDialect
	}
	if _, err := s.db.ExecContext(ctx, query,
		a.TenantID, a.PeriodStart.UTC(), a.PeriodEnd.UTC(), string(a.Metric),
		a.Sum, a.Max, a.SnapshotCount,
	); err != nil {
		return fmt.Errorf("metering: upsert aggregate: %w", err)
	}
	return nil
}
