package audit

import (
	"context"
	"database/sql"
	"fmt"
	"strings"
	"time"
)

// PartitionManager owns the monthly partitions of the `audit_log` parent
// table. It is a thin wrapper around *sql.DB that emits DDL; the SQL it
// runs is idempotent so a crash mid-operation is always safe to retry.
//
// Under DialectSQLite every method is a no-op that returns nil. This lets
// production code call the manager unconditionally and unit tests run
// against SQLite without drift.
type PartitionManager struct {
	db      *sql.DB
	dialect Dialect
	parent  string
}

// NewPartitionManager wires a manager to a DB. parent is the partitioned
// parent table name (default "audit_log").
func NewPartitionManager(db *sql.DB, dialect Dialect) *PartitionManager {
	return &PartitionManager{db: db, dialect: dialect, parent: "audit_log"}
}

// PartitionName returns the canonical child table name for a month,
// e.g. audit_log_2026_04. It is stable and idempotent: CreateNextMonthPartition
// can be called from multiple processes without collision.
func PartitionName(parent string, month time.Time) string {
	m := month.UTC()
	return fmt.Sprintf("%s_%04d_%02d", parent, m.Year(), int(m.Month()))
}

// CreateNextMonthPartition creates the partition covering [first-of-next-month,
// first-of-month-after-that). pg_partman-style pre-creation keeps us a month
// ahead of the insert edge.
func (p *PartitionManager) CreateNextMonthPartition(ctx context.Context, now time.Time) error {
	if p.dialect == DialectSQLite {
		return nil
	}
	first := firstOfMonth(now.UTC()).AddDate(0, 1, 0)
	next := first.AddDate(0, 1, 0)
	return p.createPartition(ctx, first, next)
}

// CreatePartitionForMonth creates (or ensures) the partition covering the
// whole calendar month that contains ts. Useful for backfill.
func (p *PartitionManager) CreatePartitionForMonth(ctx context.Context, ts time.Time) error {
	if p.dialect == DialectSQLite {
		return nil
	}
	first := firstOfMonth(ts.UTC())
	next := first.AddDate(0, 1, 0)
	return p.createPartition(ctx, first, next)
}

func (p *PartitionManager) createPartition(ctx context.Context, from, to time.Time) error {
	name := PartitionName(p.parent, from)
	stmt := fmt.Sprintf(
		"CREATE TABLE IF NOT EXISTS %s PARTITION OF %s FOR VALUES FROM ('%s') TO ('%s')",
		name, p.parent,
		from.Format("2006-01-02"), to.Format("2006-01-02"),
	)
	if _, err := p.db.ExecContext(ctx, stmt); err != nil {
		return fmt.Errorf("audit: create partition %s: %w", name, err)
	}
	return nil
}

// DropExpiredPartitions removes every partition whose upper bound is older
// than (now - retention). The default retention is DefaultRetention
// (7 years). It returns the names of the partitions it dropped so the
// caller can log them for compliance evidence.
func (p *PartitionManager) DropExpiredPartitions(ctx context.Context, now time.Time, retention time.Duration) ([]string, error) {
	if p.dialect == DialectSQLite {
		return nil, nil
	}
	if retention <= 0 {
		retention = DefaultRetention
	}
	cutoff := now.UTC().Add(-retention)

	rows, err := p.db.QueryContext(ctx, `
        SELECT c.relname
        FROM pg_inherits i
        JOIN pg_class c ON c.oid = i.inhrelid
        JOIN pg_class p ON p.oid = i.inhparent
        WHERE p.relname = $1
    `, p.parent)
	if err != nil {
		return nil, fmt.Errorf("audit: list partitions: %w", err)
	}
	defer rows.Close()

	var toDrop []string
	for rows.Next() {
		var name string
		if err := rows.Scan(&name); err != nil {
			return nil, err
		}
		boundary, ok := partitionUpperBound(name, p.parent)
		if !ok {
			// Non-standard child name — skip rather than drop something
			// we don't recognize.
			continue
		}
		if !boundary.After(cutoff) {
			toDrop = append(toDrop, name)
		}
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}

	var dropped []string
	for _, name := range toDrop {
		if _, err := p.db.ExecContext(ctx, fmt.Sprintf("DROP TABLE IF EXISTS %s", name)); err != nil {
			return dropped, fmt.Errorf("audit: drop partition %s: %w", name, err)
		}
		dropped = append(dropped, name)
	}
	return dropped, nil
}

// partitionUpperBound decodes the "_YYYY_MM" suffix of a partition name
// back into its upper bound (the first of the *following* month). Only
// partitions older than (now - retention) get dropped.
func partitionUpperBound(name, parent string) (time.Time, bool) {
	rest := strings.TrimPrefix(name, parent+"_")
	if rest == name {
		return time.Time{}, false
	}
	if len(rest) != len("YYYY_MM") || rest[4] != '_' {
		return time.Time{}, false
	}
	var year, month int
	if _, err := fmt.Sscanf(rest, "%04d_%02d", &year, &month); err != nil {
		return time.Time{}, false
	}
	if month < 1 || month > 12 {
		return time.Time{}, false
	}
	lower := time.Date(year, time.Month(month), 1, 0, 0, 0, 0, time.UTC)
	return lower.AddDate(0, 1, 0), true
}

func firstOfMonth(t time.Time) time.Time {
	return time.Date(t.Year(), t.Month(), 1, 0, 0, 0, 0, time.UTC)
}
