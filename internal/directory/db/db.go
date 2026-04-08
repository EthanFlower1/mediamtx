// Package db is the on-prem Directory's local SQLite persistence layer.
//
// This is distinct from internal/cloud/db (which is Postgres-backed and lives
// in the cloud control plane). The Directory DB is a single SQLite file that
// lives alongside the binary on customer hardware and never leaves the site.
//
// Boundary rules:
//   - Only internal/directory/... and internal/shared/... may import this
//     package. internal/recorder/... is forbidden (depguard, KAI-236).
//
// Migration files are embedded at build time from the migrations/ subdirectory.
// Each migration is a pair of NNNN_name.(up|down).sql files, applied in
// version order on first open and whenever a new version is present.
package db

import (
	"context"
	"database/sql"
	"embed"
	"fmt"
	"io/fs"
	"regexp"
	"sort"
	"strings"
	"time"

	_ "modernc.org/sqlite" // pure-Go SQLite driver; no CGO.
)

//go:embed migrations/*.sql
var migrationsFS embed.FS

// DB wraps *sql.DB with migration helpers for the on-prem Directory schema.
// It is safe for concurrent use.
type DB struct {
	*sql.DB
}

// Open opens (or creates) the SQLite database at path, runs pending
// migrations, and returns a ready-to-use handle.
//
// path may be ":memory:" for tests.
func Open(ctx context.Context, path string) (*DB, error) {
	// Enable WAL mode and foreign keys via the query string.
	dsn := path
	if path != ":memory:" {
		dsn = path + "?_journal=WAL&_foreign_keys=on"
	}
	sqlDB, err := sql.Open("sqlite", dsn)
	if err != nil {
		return nil, fmt.Errorf("directory/db: open: %w", err)
	}

	// SQLite allows only one writer at a time; a pool of more than 1 writer
	// deadlocks. Keep 1 writer + several readers.
	sqlDB.SetMaxOpenConns(1)
	sqlDB.SetMaxIdleConns(1)
	sqlDB.SetConnMaxLifetime(0)

	pingCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	if err := sqlDB.PingContext(pingCtx); err != nil {
		_ = sqlDB.Close()
		return nil, fmt.Errorf("directory/db: ping: %w", err)
	}

	d := &DB{DB: sqlDB}
	if err := d.Migrate(ctx); err != nil {
		_ = sqlDB.Close()
		return nil, fmt.Errorf("directory/db: migrate: %w", err)
	}
	return d, nil
}

// Migrate applies any pending migrations from the embedded filesystem.
// It is idempotent — already-applied migrations are skipped.
func (d *DB) Migrate(ctx context.Context) error {
	if _, err := d.ExecContext(ctx,
		`CREATE TABLE IF NOT EXISTS schema_migrations (
            version    INTEGER PRIMARY KEY,
            name       TEXT NOT NULL,
            applied_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP
        )`,
	); err != nil {
		return fmt.Errorf("directory/db: create schema_migrations: %w", err)
	}

	migs, err := loadMigrations()
	if err != nil {
		return err
	}

	for _, m := range migs {
		var applied int
		if err := d.QueryRowContext(ctx,
			`SELECT COUNT(*) FROM schema_migrations WHERE version = ?`, m.version,
		).Scan(&applied); err != nil {
			return fmt.Errorf("directory/db: check migration %d: %w", m.version, err)
		}
		if applied > 0 {
			continue
		}

		tx, err := d.BeginTx(ctx, nil)
		if err != nil {
			return fmt.Errorf("directory/db: begin tx for %04d_%s: %w", m.version, m.name, err)
		}
		if strings.TrimSpace(m.upSQL) != "" {
			if _, err := tx.ExecContext(ctx, m.upSQL); err != nil {
				_ = tx.Rollback()
				return fmt.Errorf("directory/db: apply %04d_%s: %w", m.version, m.name, err)
			}
		}
		if _, err := tx.ExecContext(ctx,
			`INSERT INTO schema_migrations (version, name) VALUES (?, ?)`, m.version, m.name,
		); err != nil {
			_ = tx.Rollback()
			return fmt.Errorf("directory/db: record %04d_%s: %w", m.version, m.name, err)
		}
		if err := tx.Commit(); err != nil {
			return fmt.Errorf("directory/db: commit %04d_%s: %w", m.version, m.name, err)
		}
	}
	return nil
}

// AppliedVersions returns ascending migration version numbers that have been
// applied. Useful in tests to assert the schema is at the expected version.
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

// --- migration loader -------------------------------------------------------

type migration struct {
	version int
	name    string
	upSQL   string
	downSQL string
}

var nameRe = regexp.MustCompile(`^(\d+)_([a-zA-Z0-9_]+)\.(up|down)\.sql$`)

func loadMigrations() ([]migration, error) {
	entries, err := fs.ReadDir(migrationsFS, "migrations")
	if err != nil {
		return nil, fmt.Errorf("directory/db: read migrations dir: %w", err)
	}

	byVersion := map[int]*migration{}
	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		match := nameRe.FindStringSubmatch(e.Name())
		if match == nil {
			return nil, fmt.Errorf("directory/db: migration %q does not match NNNN_name.(up|down).sql", e.Name())
		}
		var version int
		if _, err := fmt.Sscanf(match[1], "%d", &version); err != nil {
			return nil, fmt.Errorf("directory/db: parse version in %q: %w", e.Name(), err)
		}
		name, dir := match[2], match[3]
		body, err := fs.ReadFile(migrationsFS, "migrations/"+e.Name())
		if err != nil {
			return nil, fmt.Errorf("directory/db: read %q: %w", e.Name(), err)
		}

		m, ok := byVersion[version]
		if !ok {
			m = &migration{version: version, name: name}
			byVersion[version] = m
		}
		if m.name != name {
			return nil, fmt.Errorf("directory/db: version %d has mismatched names %q vs %q", version, m.name, name)
		}
		switch dir {
		case "up":
			m.upSQL = string(body)
		case "down":
			m.downSQL = string(body)
		}
	}

	out := make([]migration, 0, len(byVersion))
	for _, m := range byVersion {
		if m.upSQL == "" {
			return nil, fmt.Errorf("directory/db: migration %04d_%s missing up.sql", m.version, m.name)
		}
		out = append(out, *m)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].version < out[j].version })
	return out, nil
}
