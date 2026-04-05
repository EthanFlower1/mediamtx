package db

import "time"

// UpgradeMigration represents a tracked upgrade migration.
type UpgradeMigration struct {
	ID                  int64  `json:"id"`
	FromVersion         string `json:"from_version"`
	ToVersion           string `json:"to_version"`
	Status              string `json:"status"` // pending, running, completed, failed, rolled_back
	ConfigBackupPath    string `json:"config_backup_path"`
	DBBackupPath        string `json:"db_backup_path"`
	StartedAt           string `json:"started_at"`
	CompletedAt         string `json:"completed_at,omitempty"`
	ErrorMessage        string `json:"error_message,omitempty"`
	RollbackCompletedAt string `json:"rollback_completed_at,omitempty"`
}

// InsertUpgradeMigration creates a new upgrade migration record and returns its ID.
func (d *DB) InsertUpgradeMigration(m *UpgradeMigration) (int64, error) {
	result, err := d.Exec(`
		INSERT INTO upgrade_migrations (from_version, to_version, status, config_backup_path, db_backup_path, started_at)
		VALUES (?, ?, ?, ?, ?, ?)`,
		m.FromVersion, m.ToVersion, m.Status, m.ConfigBackupPath, m.DBBackupPath, m.StartedAt,
	)
	if err != nil {
		return 0, err
	}
	return result.LastInsertId()
}

// UpdateUpgradeMigrationStatus updates the status and optional error of an upgrade migration.
func (d *DB) UpdateUpgradeMigrationStatus(id int64, status, errMsg string) error {
	completedAt := ""
	if status == "completed" || status == "failed" || status == "rolled_back" {
		completedAt = time.Now().UTC().Format(time.RFC3339)
	}
	_, err := d.Exec(`
		UPDATE upgrade_migrations
		SET status = ?, error_message = ?, completed_at = ?
		WHERE id = ?`,
		status, errMsg, completedAt, id,
	)
	return err
}

// SetUpgradeMigrationRollback marks an upgrade migration as rolled back.
func (d *DB) SetUpgradeMigrationRollback(id int64) error {
	now := time.Now().UTC().Format(time.RFC3339)
	_, err := d.Exec(`
		UPDATE upgrade_migrations
		SET status = 'rolled_back', rollback_completed_at = ?
		WHERE id = ?`,
		now, id,
	)
	return err
}

// GetLatestUpgradeMigration returns the most recent upgrade migration, or nil if none exist.
func (d *DB) GetLatestUpgradeMigration() (*UpgradeMigration, error) {
	row := d.QueryRow(`
		SELECT id, from_version, to_version, status, config_backup_path, db_backup_path,
		       started_at, COALESCE(completed_at, ''), COALESCE(error_message, ''),
		       COALESCE(rollback_completed_at, '')
		FROM upgrade_migrations
		ORDER BY started_at DESC
		LIMIT 1`)

	var m UpgradeMigration
	err := row.Scan(&m.ID, &m.FromVersion, &m.ToVersion, &m.Status,
		&m.ConfigBackupPath, &m.DBBackupPath,
		&m.StartedAt, &m.CompletedAt, &m.ErrorMessage, &m.RollbackCompletedAt)
	if err != nil {
		return nil, err
	}
	return &m, nil
}

// ListUpgradeMigrations returns upgrade migration records, most recent first.
func (d *DB) ListUpgradeMigrations(limit int) ([]*UpgradeMigration, error) {
	if limit <= 0 {
		limit = 50
	}
	rows, err := d.Query(`
		SELECT id, from_version, to_version, status, config_backup_path, db_backup_path,
		       started_at, COALESCE(completed_at, ''), COALESCE(error_message, ''),
		       COALESCE(rollback_completed_at, '')
		FROM upgrade_migrations
		ORDER BY started_at DESC
		LIMIT ?`, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var records []*UpgradeMigration
	for rows.Next() {
		var m UpgradeMigration
		if err := rows.Scan(&m.ID, &m.FromVersion, &m.ToVersion, &m.Status,
			&m.ConfigBackupPath, &m.DBBackupPath,
			&m.StartedAt, &m.CompletedAt, &m.ErrorMessage, &m.RollbackCompletedAt); err != nil {
			return nil, err
		}
		records = append(records, &m)
	}
	if records == nil {
		records = []*UpgradeMigration{}
	}
	return records, rows.Err()
}

// GetCurrentSchemaVersion returns the highest applied schema migration version.
func (d *DB) GetCurrentSchemaVersion() (int, error) {
	var version int
	err := d.QueryRow("SELECT COALESCE(MAX(version), 0) FROM schema_migrations").Scan(&version)
	return version, err
}
