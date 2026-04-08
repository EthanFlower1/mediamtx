package db

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"net/url"
	"strings"
	"time"

	_ "modernc.org/sqlite" // pure-Go SQLite driver, compatibility mode for unit tests
)

// DefaultRegion is the only region v1 actually runs in. Seam #9: the column
// exists everywhere so adding region #2 is a data migration, not a schema one.
const DefaultRegion = "us-east-2"

// DB wraps *sql.DB with a dialect marker so tenant-scoped helpers can craft
// portable queries. Production uses DialectPostgres (RDS via KAI-216); unit
// tests use DialectSQLite.
type DB struct {
	*sql.DB
	dialect Dialect
}

// Dialect returns the SQL dialect this handle was opened in.
func (d *DB) Dialect() Dialect { return d.dialect }

// Open opens a cloud database connection, runs pending migrations, and returns
// a ready-to-use handle.
//
// The dsn format is driver-specific:
//
//   - Postgres:  postgres://user:pass@host:5432/dbname?sslmode=require
//   - SQLite:    sqlite://file:test.db  (or "sqlite://:memory:")
//
// The scheme selects the driver and dialect; no schema-inference magic.
func Open(ctx context.Context, dsn string) (*DB, error) {
	driver, rest, dialect, err := parseDSN(dsn)
	if err != nil {
		return nil, err
	}

	sqlDB, err := sql.Open(driver, rest)
	if err != nil {
		return nil, fmt.Errorf("open %s: %w", driver, err)
	}

	// Conservative defaults. Production (KAI-215 / KAI-216) can tune via
	// wrapper options once the real RDS instance exists.
	sqlDB.SetMaxOpenConns(10)
	sqlDB.SetMaxIdleConns(5)
	sqlDB.SetConnMaxLifetime(30 * time.Minute)

	pingCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	if err := sqlDB.PingContext(pingCtx); err != nil {
		_ = sqlDB.Close()
		return nil, fmt.Errorf("ping: %w", err)
	}

	d := &DB{DB: sqlDB, dialect: dialect}
	if err := d.Migrate(ctx); err != nil {
		_ = sqlDB.Close()
		return nil, fmt.Errorf("migrate: %w", err)
	}
	return d, nil
}

// parseDSN translates the wrapper's scheme-prefixed DSN into a (driver, dsn,
// dialect) triple. The scheme prefix (`postgres://` or `sqlite://`) is the only
// thing we strip — everything after is forwarded verbatim to the driver.
func parseDSN(dsn string) (driver, rest string, dialect Dialect, err error) {
	switch {
	case strings.HasPrefix(dsn, "postgres://"), strings.HasPrefix(dsn, "postgresql://"):
		// Keep the full URL; pgx/lib-pq both accept it.
		return "postgres", dsn, DialectPostgres, nil
	case strings.HasPrefix(dsn, "sqlite://"):
		return "sqlite", strings.TrimPrefix(dsn, "sqlite://"), DialectSQLite, nil
	default:
		// Bare sqlite path — convenient for tests.
		if _, perr := url.Parse(dsn); perr == nil && strings.HasSuffix(dsn, ".db") {
			return "sqlite", dsn, DialectSQLite, nil
		}
		return "", "", 0, errors.New("dsn must start with postgres://, postgresql://, or sqlite://")
	}
}

// Migrate applies any pending migrations. Safe to call at startup; idempotent.
func (d *DB) Migrate(ctx context.Context) error {
	if _, err := d.ExecContext(ctx,
		`CREATE TABLE IF NOT EXISTS schema_migrations (
            version INTEGER PRIMARY KEY,
            name    TEXT NOT NULL,
            applied_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP
        )`,
	); err != nil {
		return fmt.Errorf("create schema_migrations: %w", err)
	}

	migs, err := loadMigrations(d.dialect)
	if err != nil {
		return err
	}

	for _, m := range migs {
		var applied int
		if err := d.QueryRowContext(ctx,
			`SELECT COUNT(*) FROM schema_migrations WHERE version = ?`, m.version,
		).Scan(&applied); err != nil {
			// Postgres uses $1 positional args, not ?. Retry with $1 syntax.
			if d.dialect == DialectPostgres {
				if err2 := d.QueryRowContext(ctx,
					`SELECT COUNT(*) FROM schema_migrations WHERE version = $1`, m.version,
				).Scan(&applied); err2 != nil {
					return fmt.Errorf("check migration %d: %w", m.version, err2)
				}
			} else {
				return fmt.Errorf("check migration %d: %w", m.version, err)
			}
		}
		if applied > 0 {
			continue
		}

		// Run the up script inside a transaction so a failure leaves no
		// half-applied tables behind.
		tx, err := d.BeginTx(ctx, nil)
		if err != nil {
			return fmt.Errorf("begin tx for %04d_%s: %w", m.version, m.name, err)
		}
		if strings.TrimSpace(m.upSQL) != "" {
			if _, err := tx.ExecContext(ctx, m.upSQL); err != nil {
				_ = tx.Rollback()
				return fmt.Errorf("apply %04d_%s: %w", m.version, m.name, err)
			}
		}
		recordQuery := `INSERT INTO schema_migrations (version, name) VALUES (?, ?)`
		if d.dialect == DialectPostgres {
			recordQuery = `INSERT INTO schema_migrations (version, name) VALUES ($1, $2)`
		}
		if _, err := tx.ExecContext(ctx, recordQuery, m.version, m.name); err != nil {
			_ = tx.Rollback()
			return fmt.Errorf("record %04d_%s: %w", m.version, m.name, err)
		}
		if err := tx.Commit(); err != nil {
			return fmt.Errorf("commit %04d_%s: %w", m.version, m.name, err)
		}
	}
	return nil
}

// AppliedVersions returns the list of applied migration versions, ascending.
// Useful for diagnostics and for the multi-tenant isolation chaos test
// (KAI-235) to assert the expected schema shape.
func (d *DB) AppliedVersions(ctx context.Context) ([]int, error) {
	rows, err := d.QueryContext(ctx, `SELECT version FROM schema_migrations ORDER BY version ASC`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []int
	for rows.Next() {
		var v int
		if err := rows.Scan(&v); err != nil {
			return nil, err
		}
		out = append(out, v)
	}
	return out, rows.Err()
}
