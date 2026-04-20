// Package db provides SQLite database access for the recorder subsystem.
package db

import (
	"database/sql"
	"fmt"
	"net/url"
	"time"

	_ "modernc.org/sqlite" // Pure-Go SQLite driver.
)

// DB wraps a *sql.DB connected to the recorder SQLite database.
type DB struct {
	*sql.DB
	path string
}

// Path returns the file path of the database.
func (d *DB) Path() string {
	return d.path
}

// Open opens (or creates) the SQLite database at path, enables foreign keys,
// sets WAL journal mode, and runs any pending migrations.
func Open(path string) (*DB, error) {
	// Build DSN with _pragma parameters so that every connection in the pool
	// inherits these settings automatically.
	dsn := path + "?" + url.Values{
		"_pragma": []string{
			"foreign_keys(1)",
			"journal_mode(WAL)",
			"busy_timeout(10000)",
		},
	}.Encode()

	sqlDB, err := sql.Open("sqlite", dsn)
	if err != nil {
		return nil, fmt.Errorf("open database: %w", err)
	}

	// Configure connection pool for SQLite. SQLite only supports one writer
	// at a time. We use 2 connections to avoid deadlocks when a query holds
	// an open cursor while another statement executes on the same DB handle.
	// The busy_timeout pragma ensures the second writer retries rather than
	// returning SQLITE_BUSY immediately.
	sqlDB.SetMaxOpenConns(2)
	sqlDB.SetMaxIdleConns(2)
	sqlDB.SetConnMaxLifetime(5 * time.Minute)

	// Validate connectivity.
	if err := sqlDB.Ping(); err != nil {
		sqlDB.Close()
		return nil, fmt.Errorf("database ping: %w", err)
	}

	d := &DB{DB: sqlDB, path: path}

	if err := d.runMigrations(); err != nil {
		sqlDB.Close()
		return nil, fmt.Errorf("run migrations: %w", err)
	}

	return d, nil
}

// runMigrations applies any migrations that have not yet been recorded in the
// schema_migrations table. Each migration runs inside its own transaction.
func (d *DB) runMigrations() error {
	// Ensure the schema_migrations table exists before querying it.
	if _, err := d.Exec(`CREATE TABLE IF NOT EXISTS schema_migrations (version INTEGER PRIMARY KEY)`); err != nil {
		return fmt.Errorf("create schema_migrations: %w", err)
	}

	for _, m := range migrations {
		var count int
		if err := d.QueryRow("SELECT COUNT(*) FROM schema_migrations WHERE version = ?", m.version).Scan(&count); err != nil {
			return fmt.Errorf("check migration %d: %w", m.version, err)
		}
		if count > 0 {
			continue
		}

		tx, err := d.Begin()
		if err != nil {
			return fmt.Errorf("begin migration %d: %w", m.version, err)
		}

		if _, err := tx.Exec(m.sql); err != nil {
			tx.Rollback()
			return fmt.Errorf("exec migration %d: %w", m.version, err)
		}

		if _, err := tx.Exec("INSERT INTO schema_migrations (version) VALUES (?)", m.version); err != nil {
			tx.Rollback()
			return fmt.Errorf("record migration %d: %w", m.version, err)
		}

		if err := tx.Commit(); err != nil {
			return fmt.Errorf("commit migration %d: %w", m.version, err)
		}
	}

	return nil
}
