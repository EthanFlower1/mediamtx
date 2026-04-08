// Package db is the cloud control plane's schema and data access layer.
//
// The authoritative migrations in the `migrations/` subdirectory are written in
// PostgreSQL dialect. Production runs against AWS RDS Postgres (provisioned in
// KAI-216). Unit tests in this package run against modernc.org/sqlite in a
// compatibility mode — the runner rewrites a small set of Postgres-specific
// syntax so the migrations apply against SQLite for fast, hermetic tests. Any
// Postgres-only migration (for example pg_partman partitioning, KAI-233) is
// wrapped in `-- postgres-only:begin` / `-- postgres-only:end` markers and
// skipped entirely in SQLite mode.
//
// See README.md in this package for the test strategy.
package db

import (
	"embed"
	"fmt"
	"io/fs"
	"regexp"
	"sort"
	"strings"
)

//go:embed migrations/*.sql
var migrationsFS embed.FS

// Dialect is the SQL dialect the runner should target.
type Dialect int

const (
	// DialectPostgres is the production dialect (RDS Postgres, KAI-216).
	DialectPostgres Dialect = iota
	// DialectSQLite is used for hermetic unit tests via modernc.org/sqlite.
	// A subset of Postgres-specific syntax is rewritten at load time.
	DialectSQLite
)

// migration is a single up/down pair loaded from the embedded FS.
type migration struct {
	version int
	name    string
	upSQL   string
	downSQL string
}

// loadMigrations loads every migration from the embedded filesystem, groups
// them by version, and rewrites the SQL to the target dialect.
func loadMigrations(dialect Dialect) ([]migration, error) {
	entries, err := fs.ReadDir(migrationsFS, "migrations")
	if err != nil {
		return nil, fmt.Errorf("read migrations dir: %w", err)
	}

	byVersion := map[int]*migration{}
	nameRe := regexp.MustCompile(`^(\d+)_([a-zA-Z0-9_]+)\.(up|down)\.sql$`)

	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		match := nameRe.FindStringSubmatch(e.Name())
		if match == nil {
			return nil, fmt.Errorf("migration %q does not match NNNN_name.(up|down).sql", e.Name())
		}
		var version int
		if _, err := fmt.Sscanf(match[1], "%d", &version); err != nil {
			return nil, fmt.Errorf("parse version in %q: %w", e.Name(), err)
		}
		name := match[2]
		direction := match[3]

		body, err := fs.ReadFile(migrationsFS, "migrations/"+e.Name())
		if err != nil {
			return nil, fmt.Errorf("read %q: %w", e.Name(), err)
		}
		sqlText := string(body)
		if dialect == DialectSQLite {
			sqlText = translateToSQLite(sqlText)
		}

		m, ok := byVersion[version]
		if !ok {
			m = &migration{version: version, name: name}
			byVersion[version] = m
		}
		if m.name != name {
			return nil, fmt.Errorf("migration version %d has mismatched names %q vs %q", version, m.name, name)
		}
		switch direction {
		case "up":
			m.upSQL = sqlText
		case "down":
			m.downSQL = sqlText
		}
	}

	out := make([]migration, 0, len(byVersion))
	for _, m := range byVersion {
		if m.upSQL == "" {
			return nil, fmt.Errorf("migration %04d_%s missing up.sql", m.version, m.name)
		}
		out = append(out, *m)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].version < out[j].version })
	return out, nil
}

// postgresOnlyRe matches `-- postgres-only:begin` ... `-- postgres-only:end`
// blocks so the SQLite runner can strip them. The match is non-greedy and
// multi-line.
var postgresOnlyRe = regexp.MustCompile(`(?s)-- postgres-only:begin.*?-- postgres-only:end`)

// translateToSQLite rewrites a subset of Postgres-specific syntax into a form
// that the modernc.org/sqlite driver accepts. This is NOT a general-purpose
// Postgres-to-SQLite translator — it exists solely to let unit tests exercise
// CRUD against the schema without spinning up a real Postgres. Anything
// Postgres-exclusive (partitioning, JSONB operators, functional indexes,
// pg_partman) must be wrapped in postgres-only markers.
func translateToSQLite(s string) string {
	// Strip entire postgres-only blocks.
	s = postgresOnlyRe.ReplaceAllString(s, "")

	// Type mappings (case-sensitive because all migrations use upper-case types).
	// Partial indexes (WHERE clauses) must be wrapped in postgres-only markers
	// in the migration files rather than stripped here — this keeps the
	// translator simple and the migration files self-documenting.
	replacements := []struct{ from, to string }{
		{"TIMESTAMPTZ", "DATETIME"},
		{"JSONB", "TEXT"},
		{"NUMERIC(5,2)", "REAL"},
		{"BIGSERIAL", "INTEGER"},
		{"BIGINT", "INTEGER"},
		{"BYTEA", "BLOB"},
		{"BOOLEAN", "INTEGER"},
		{"NOW()", "CURRENT_TIMESTAMP"},
		{"::jsonb", ""},
	}
	for _, r := range replacements {
		s = strings.ReplaceAll(s, r.from, r.to)
	}
	return s
}
