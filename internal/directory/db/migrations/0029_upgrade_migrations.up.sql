CREATE TABLE IF NOT EXISTS upgrade_migrations (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    from_version TEXT NOT NULL DEFAULT '',
    to_version TEXT NOT NULL DEFAULT '',
    status TEXT NOT NULL DEFAULT 'pending',
    config_backup_path TEXT NOT NULL DEFAULT '',
    db_backup_path TEXT NOT NULL DEFAULT '',
    started_at TEXT NOT NULL DEFAULT '',
    completed_at TEXT NOT NULL DEFAULT '',
    error_message TEXT NOT NULL DEFAULT '',
    rollback_completed_at TEXT NOT NULL DEFAULT ''
);

CREATE TABLE IF NOT EXISTS update_history (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    from_version TEXT NOT NULL,
    to_version TEXT NOT NULL,
    status TEXT NOT NULL DEFAULT 'pending',
    started_at TEXT NOT NULL,
    completed_at TEXT,
    error_message TEXT,
    initiated_by TEXT NOT NULL DEFAULT '',
    sha256_checksum TEXT NOT NULL DEFAULT '',
    rollback_available INTEGER NOT NULL DEFAULT 0
);
CREATE INDEX IF NOT EXISTS idx_update_history_status ON update_history(status);
CREATE INDEX IF NOT EXISTS idx_update_history_started ON update_history(started_at);
