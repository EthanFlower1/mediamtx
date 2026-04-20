// Package db is the Recorder's local SQLite cache.
//
// This database is distinct from internal/directory/db (the on-prem Directory's
// authoritative schema) and from internal/cloud/db (the cloud control plane's
// Postgres). The Recorder DB mirrors the subset of Directory state this node
// actually needs to drive Raikada path config and capture loops, plus a small
// KV table of local runtime state.
//
// Boundary rules:
//   - internal/recorder/... may import this package.
//   - internal/directory/... must NOT be imported here; the seam is "Directory
//     pushes StreamAssignments, Recorder applies them to this cache".
//     See KAI-236 depguard rules.
//
// Fail-open recording: the Recorder reads this cache on the capture hot path
// so that recording continues even when the Directory is unreachable. Writes
// land here only via the reconciler (KAI-143).
//
// Migration files are embedded at build time from the migrations/ subdirectory.
// Each migration is a pair of NNNN_name.(up|down).sql files, applied in version
// order on first open and whenever a new version is present.
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

// DB wraps *sql.DB with migration helpers for the Recorder-local schema.
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
		return nil, fmt.Errorf("recorder/db: open: %w", err)
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
		return nil, fmt.Errorf("recorder/db: ping: %w", err)
	}

	// PRAGMA foreign_keys = ON belt-and-braces in case the DSN flag is ignored
	// on :memory: or future driver versions.
	if _, err := sqlDB.ExecContext(ctx, `PRAGMA foreign_keys = ON`); err != nil {
		_ = sqlDB.Close()
		return nil, fmt.Errorf("recorder/db: enable foreign_keys: %w", err)
	}

	d := &DB{DB: sqlDB}
	if err := d.Migrate(ctx); err != nil {
		_ = sqlDB.Close()
		return nil, fmt.Errorf("recorder/db: migrate: %w", err)
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
		return fmt.Errorf("recorder/db: create schema_migrations: %w", err)
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
			return fmt.Errorf("recorder/db: check migration %d: %w", m.version, err)
		}
		if applied > 0 {
			continue
		}

		tx, err := d.BeginTx(ctx, nil)
		if err != nil {
			return fmt.Errorf("recorder/db: begin tx for %04d_%s: %w", m.version, m.name, err)
		}
		if strings.TrimSpace(m.upSQL) != "" {
			if _, err := tx.ExecContext(ctx, m.upSQL); err != nil {
				_ = tx.Rollback()
				return fmt.Errorf("recorder/db: apply %04d_%s: %w", m.version, m.name, err)
			}
		}
		if _, err := tx.ExecContext(ctx,
			`INSERT INTO schema_migrations (version, name) VALUES (?, ?)`, m.version, m.name,
		); err != nil {
			_ = tx.Rollback()
			return fmt.Errorf("recorder/db: record %04d_%s: %w", m.version, m.name, err)
		}
		if err := tx.Commit(); err != nil {
			return fmt.Errorf("recorder/db: commit %04d_%s: %w", m.version, m.name, err)
		}
	}
	return nil
}

// Rollback reverts the most recently applied migration by running its down.sql
// and deleting the schema_migrations row. Primarily used by tests to verify
// up/down round-trips; production code should never call this.
func (d *DB) Rollback(ctx context.Context) error {
	migs, err := loadMigrations()
	if err != nil {
		return err
	}

	var current int
	if err := d.QueryRowContext(ctx,
		`SELECT COALESCE(MAX(version), 0) FROM schema_migrations`,
	).Scan(&current); err != nil {
		return fmt.Errorf("recorder/db: read current version: %w", err)
	}
	if current == 0 {
		return nil
	}

	var target *migration
	for i := range migs {
		if migs[i].version == current {
			target = &migs[i]
			break
		}
	}
	if target == nil {
		return fmt.Errorf("recorder/db: no migration file for applied version %d", current)
	}
	if strings.TrimSpace(target.downSQL) == "" {
		return fmt.Errorf("recorder/db: migration %04d_%s has no down.sql", target.version, target.name)
	}

	tx, err := d.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("recorder/db: begin rollback tx: %w", err)
	}
	if _, err := tx.ExecContext(ctx, target.downSQL); err != nil {
		_ = tx.Rollback()
		return fmt.Errorf("recorder/db: apply down %04d_%s: %w", target.version, target.name, err)
	}
	if _, err := tx.ExecContext(ctx,
		`DELETE FROM schema_migrations WHERE version = ?`, target.version,
	); err != nil {
		_ = tx.Rollback()
		return fmt.Errorf("recorder/db: delete migration record %d: %w", target.version, err)
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("recorder/db: commit rollback %04d_%s: %w", target.version, target.name, err)
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
		return nil, fmt.Errorf("recorder/db: read migrations dir: %w", err)
	}

	byVersion := map[int]*migration{}
	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		match := nameRe.FindStringSubmatch(e.Name())
		if match == nil {
			return nil, fmt.Errorf("recorder/db: migration %q does not match NNNN_name.(up|down).sql", e.Name())
		}
		var version int
		if _, err := fmt.Sscanf(match[1], "%d", &version); err != nil {
			return nil, fmt.Errorf("recorder/db: parse version in %q: %w", e.Name(), err)
		}
		name, dir := match[2], match[3]
		body, err := fs.ReadFile(migrationsFS, "migrations/"+e.Name())
		if err != nil {
			return nil, fmt.Errorf("recorder/db: read %q: %w", e.Name(), err)
		}

		m, ok := byVersion[version]
		if !ok {
			m = &migration{version: version, name: name}
			byVersion[version] = m
		}
		if m.name != name {
			return nil, fmt.Errorf("recorder/db: version %d has mismatched names %q vs %q", version, m.name, name)
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
			return nil, fmt.Errorf("recorder/db: migration %04d_%s missing up.sql", m.version, m.name)
		}
		out = append(out, *m)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].version < out[j].version })
	return out, nil
}
