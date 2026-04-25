// Package migration provides utilities for migrating legacy NVR database
// state into the split directory.db and recorder.db layout.
package migration

import (
	"database/sql"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"strings"

	_ "modernc.org/sqlite" // Pure-Go SQLite driver.
)

// directoryTables is the canonical set of tables owned by the Directory role.
var directoryTables = map[string]struct{}{
	"cameras":                     {},
	"camera_streams":               {},
	"devices":                     {},
	"users":                       {},
	"roles":                       {},
	"user_camera_permissions":      {},
	"refresh_tokens":               {},
	"api_keys":                    {},
	"api_key_audit_log":            {},
	"recording_rules":              {},
	"config":                      {},
	"camera_groups":                {},
	"camera_group_members":         {},
	"webhook_configs":              {},
	"webhook_deliveries":           {},
	"alert_rules":                  {},
	"alerts":                      {},
	"smtp_config":                  {},
	"notification_preferences":     {},
	"notification_quiet_hours":     {},
	"escalation_rules":             {},
	"notifications":               {},
	"notification_read_state":      {},
	"federations":                  {},
	"federation_peers":             {},
	"integration_configs":          {},
	"upgrade_migrations":           {},
	"update_history":               {},
}

// recorderTables is the canonical set of tables owned by the Recorder role.
var recorderTables = map[string]struct{}{
	"recordings":            {},
	"saved_clips":           {},
	"clips":                 {},
	"bookmarks":             {},
	"tracks":                {},
	"tours":                 {},
	"detections":            {},
	"detection_events":      {},
	"detection_zones":       {},
	"detection_schedules":   {},
	"motion_events":         {},
	"screenshots":           {},
	"storage_quotas":        {},
	"connection_events":     {},
	"pending_syncs":         {},
	"export_jobs":           {},
	"evidence_exports":      {},
	"bulk_export_jobs":      {},
	"bulk_export_items":     {},
	"cross_camera_tracks":   {},
	"cross_camera_sightings":{},
	"queued_commands":       {},
}

// MigrateResult summarises what MigrateLegacyDB did.
type MigrateResult struct {
	Skipped       bool              // true when directory.db already existed
	DirectoryRows map[string]int64  // table -> row count copied into directory.db
	RecorderRows  map[string]int64  // table -> row count copied into recorder.db
	BackupPath    string            // path of the renamed nvr.db.backup
}

// MigrateLegacyDB checks whether the legacy nvr.db exists inside dataDir and,
// when directory.db does not yet exist, copies tables into directory.db and
// recorder.db, then renames nvr.db to nvr.db.backup.
//
// The function is idempotent: if directory.db already exists it returns
// immediately with MigrateResult.Skipped == true.
//
// logger may be nil (slog.Default() is used in that case).
func MigrateLegacyDB(dataDir string, logger *slog.Logger) (*MigrateResult, error) {
	if logger == nil {
		logger = slog.Default()
	}

	nvrPath := filepath.Join(dataDir, "nvr.db")
	dirPath := filepath.Join(dataDir, "directory.db")
	recPath := filepath.Join(dataDir, "recorder.db")
	backupPath := nvrPath + ".backup"

	// Idempotency check.
	if _, err := os.Stat(dirPath); err == nil {
		logger.Info("migration: directory.db already exists — skipping legacy migration",
			slog.String("path", dirPath))
		return &MigrateResult{Skipped: true}, nil
	}

	// Check that nvr.db exists.
	if _, err := os.Stat(nvrPath); os.IsNotExist(err) {
		logger.Info("migration: nvr.db not found — nothing to migrate",
			slog.String("path", nvrPath))
		return &MigrateResult{Skipped: true}, nil
	}

	logger.Info("migration: starting legacy DB migration",
		slog.String("source", nvrPath),
		slog.String("directory_dest", dirPath),
		slog.String("recorder_dest", recPath))

	// Open nvr.db read-only.
	srcDB, err := openSQLite(nvrPath + "?mode=ro")
	if err != nil {
		return nil, fmt.Errorf("migration: open nvr.db read-only: %w", err)
	}
	defer srcDB.Close()

	// List all tables in nvr.db.
	tables, err := listTables(srcDB)
	if err != nil {
		return nil, fmt.Errorf("migration: list tables in nvr.db: %w", err)
	}
	logger.Info("migration: found tables in nvr.db", slog.Int("count", len(tables)))

	result := &MigrateResult{
		DirectoryRows: make(map[string]int64),
		RecorderRows:  make(map[string]int64),
		BackupPath:    backupPath,
	}

	// Split tables into directory-owned and recorder-owned buckets.
	var dirTables, recTables []string
	for _, t := range tables {
		if _, ok := directoryTables[t]; ok {
			dirTables = append(dirTables, t)
		} else if _, ok := recorderTables[t]; ok {
			recTables = append(recTables, t)
		} else {
			logger.Warn("migration: unknown table — skipping", slog.String("table", t))
		}
	}

	// Copy directory-owned tables.
	if len(dirTables) > 0 {
		dstDB, err := openSQLite(dirPath)
		if err != nil {
			return nil, fmt.Errorf("migration: create directory.db: %w", err)
		}
		defer dstDB.Close()

		for _, t := range dirTables {
			n, err := copyTable(srcDB, dstDB, t)
			if err != nil {
				return nil, fmt.Errorf("migration: copy table %q to directory.db: %w", t, err)
			}
			result.DirectoryRows[t] = n
			logger.Info("migration: copied directory table",
				slog.String("table", t), slog.Int64("rows", n))
		}
	}

	// Copy recorder-owned tables.
	if len(recTables) > 0 {
		rDB, err := openSQLite(recPath)
		if err != nil {
			return nil, fmt.Errorf("migration: create recorder.db: %w", err)
		}
		defer rDB.Close()

		for _, t := range recTables {
			n, err := copyTable(srcDB, rDB, t)
			if err != nil {
				return nil, fmt.Errorf("migration: copy table %q to recorder.db: %w", t, err)
			}
			result.RecorderRows[t] = n
			logger.Info("migration: copied recorder table",
				slog.String("table", t), slog.Int64("rows", n))
		}
	}

	// Rename nvr.db → nvr.db.backup.
	if err := os.Rename(nvrPath, backupPath); err != nil {
		return nil, fmt.Errorf("migration: rename nvr.db to backup: %w", err)
	}
	logger.Info("migration: nvr.db renamed to backup", slog.String("backup", backupPath))

	// Summary.
	var totalDir, totalRec int64
	for _, n := range result.DirectoryRows {
		totalDir += n
	}
	for _, n := range result.RecorderRows {
		totalRec += n
	}
	logger.Info("migration: complete",
		slog.Int("directory_tables", len(result.DirectoryRows)),
		slog.Int64("directory_rows", totalDir),
		slog.Int("recorder_tables", len(result.RecorderRows)),
		slog.Int64("recorder_rows", totalRec))

	return result, nil
}

// openSQLite opens a SQLite database at dsn (which may already include query
// parameters). It enables WAL mode and foreign keys.
func openSQLite(dsn string) (*sql.DB, error) {
	// If there are no query parameters yet, add WAL + FK settings.
	if !strings.Contains(dsn, "?") {
		dsn += "?_pragma=foreign_keys(1)&_pragma=journal_mode(WAL)&_pragma=busy_timeout(10000)"
	}
	db, err := sql.Open("sqlite", dsn)
	if err != nil {
		return nil, err
	}
	if err := db.Ping(); err != nil {
		db.Close()
		return nil, err
	}
	return db, nil
}

// listTables returns the names of all user-defined tables in db, excluding
// SQLite internal tables.
func listTables(db *sql.DB) ([]string, error) {
	rows, err := db.Query(
		`SELECT name FROM sqlite_master WHERE type='table' AND name NOT LIKE 'sqlite_%' ORDER BY name`,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var tables []string
	for rows.Next() {
		var name string
		if err := rows.Scan(&name); err != nil {
			return nil, err
		}
		tables = append(tables, name)
	}
	return tables, rows.Err()
}

// copyTable creates the table in dst (using the same DDL as src) and then
// copies all rows. It returns the number of rows inserted.
func copyTable(src, dst *sql.DB, table string) (int64, error) {
	// Get the CREATE TABLE statement from the source.
	var createSQL string
	if err := src.QueryRow(
		`SELECT sql FROM sqlite_master WHERE type='table' AND name=?`, table,
	).Scan(&createSQL); err != nil {
		return 0, fmt.Errorf("get DDL for %q: %w", table, err)
	}

	// Create the table in the destination.
	if _, err := dst.Exec(createSQL); err != nil {
		// If it already exists, that's fine.
		if !strings.Contains(err.Error(), "already exists") {
			return 0, fmt.Errorf("create table %q in dest: %w", table, err)
		}
	}

	// Read column names.
	cols, err := tableColumns(src, table)
	if err != nil {
		return 0, fmt.Errorf("list columns of %q: %w", table, err)
	}
	if len(cols) == 0 {
		return 0, nil
	}

	colList := strings.Join(cols, ", ")
	placeholders := strings.Repeat("?,", len(cols))
	placeholders = placeholders[:len(placeholders)-1] // trim trailing comma

	insertSQL := fmt.Sprintf(
		"INSERT OR IGNORE INTO %q (%s) VALUES (%s)",
		table, colList, placeholders,
	)

	selectSQL := fmt.Sprintf("SELECT %s FROM %q", colList, table)
	srcRows, err := src.Query(selectSQL) //nolint:rowserrcheck
	if err != nil {
		return 0, fmt.Errorf("select from %q: %w", table, err)
	}
	defer srcRows.Close()

	tx, err := dst.Begin()
	if err != nil {
		return 0, fmt.Errorf("begin transaction for %q: %w", table, err)
	}

	stmt, err := tx.Prepare(insertSQL)
	if err != nil {
		tx.Rollback() //nolint:errcheck
		return 0, fmt.Errorf("prepare insert for %q: %w", table, err)
	}
	defer stmt.Close()

	vals := make([]any, len(cols))
	ptrs := make([]any, len(cols))
	for i := range vals {
		ptrs[i] = &vals[i]
	}

	var count int64
	for srcRows.Next() {
		if err := srcRows.Scan(ptrs...); err != nil {
			tx.Rollback() //nolint:errcheck
			return 0, fmt.Errorf("scan row from %q: %w", table, err)
		}
		if _, err := stmt.Exec(vals...); err != nil {
			tx.Rollback() //nolint:errcheck
			return 0, fmt.Errorf("insert row into %q: %w", table, err)
		}
		count++
	}
	if err := srcRows.Err(); err != nil {
		tx.Rollback() //nolint:errcheck
		return 0, fmt.Errorf("read rows from %q: %w", table, err)
	}

	if err := tx.Commit(); err != nil {
		return 0, fmt.Errorf("commit rows for %q: %w", table, err)
	}
	return count, nil
}

// tableColumns returns the column names for the given table.
func tableColumns(db *sql.DB, table string) ([]string, error) {
	rows, err := db.Query(fmt.Sprintf("PRAGMA table_info(%q)", table))
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var cols []string
	for rows.Next() {
		// PRAGMA table_info columns: cid, name, type, notnull, dflt_value, pk
		var cid int
		var name, colType string
		var notNull int
		var dfltValue sql.NullString
		var pk int
		if err := rows.Scan(&cid, &name, &colType, &notNull, &dfltValue, &pk); err != nil {
			return nil, err
		}
		cols = append(cols, name)
	}
	return cols, rows.Err()
}
