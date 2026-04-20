-- Users and roles — authoritative identity store for the Directory.
-- Recorders validate Directory-issued JWTs but do not manage users.

CREATE TABLE IF NOT EXISTS users (
    id                  TEXT    NOT NULL PRIMARY KEY,
    tenant_id           TEXT    NOT NULL DEFAULT 'local',
    username            TEXT    NOT NULL UNIQUE,
    password_hash       TEXT    NOT NULL,
    role_id             TEXT,
    camera_permissions  TEXT    NOT NULL DEFAULT '',
    locked_until        DATETIME,
    failed_login_attempts INTEGER NOT NULL DEFAULT 0,
    created_at          DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at          DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP
);

CREATE TABLE IF NOT EXISTS roles (
    id          TEXT    NOT NULL PRIMARY KEY,
    tenant_id   TEXT    NOT NULL DEFAULT 'local',
    name        TEXT    NOT NULL,
    description TEXT    NOT NULL DEFAULT '',
    permissions TEXT    NOT NULL DEFAULT '[]',   -- JSON array
    is_system   INTEGER NOT NULL DEFAULT 0 CHECK (is_system IN (0, 1)),
    created_at  DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at  DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP
);

-- Alert rules — fleet-level alert configuration, pushed to recorders.

CREATE TABLE IF NOT EXISTS alert_rules (
    id                TEXT    NOT NULL PRIMARY KEY,
    tenant_id         TEXT    NOT NULL DEFAULT 'local',
    name              TEXT    NOT NULL,
    rule_type         TEXT    NOT NULL CHECK (rule_type IN ('disk_usage', 'camera_offline', 'recording_gap', 'motion', 'detection')),
    threshold_value   REAL    NOT NULL DEFAULT 0,
    camera_id         TEXT,
    recorder_id       TEXT,
    enabled           INTEGER NOT NULL DEFAULT 1 CHECK (enabled IN (0, 1)),
    notify_email      INTEGER NOT NULL DEFAULT 0 CHECK (notify_email IN (0, 1)),
    cooldown_minutes  INTEGER NOT NULL DEFAULT 15,
    created_at        DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at        DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
    FOREIGN KEY (camera_id) REFERENCES cameras (id) ON DELETE CASCADE
);

-- Audit log — centralized fleet-wide audit trail.

CREATE TABLE IF NOT EXISTS audit_entries (
    id            INTEGER PRIMARY KEY AUTOINCREMENT,
    tenant_id     TEXT    NOT NULL DEFAULT 'local',
    user_id       TEXT    NOT NULL DEFAULT '',
    username      TEXT    NOT NULL DEFAULT '',
    recorder_id   TEXT    NOT NULL DEFAULT '',
    action        TEXT    NOT NULL,
    resource_type TEXT    NOT NULL DEFAULT '',
    resource_id   TEXT    NOT NULL DEFAULT '',
    details       TEXT    NOT NULL DEFAULT '',
    ip_address    TEXT    NOT NULL DEFAULT '',
    created_at    DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP
);

CREATE INDEX IF NOT EXISTS idx_audit_entries_tenant_time
    ON audit_entries (tenant_id, created_at);

-- Export jobs — Directory-orchestrated export requests.

CREATE TABLE IF NOT EXISTS export_jobs (
    id            TEXT    NOT NULL PRIMARY KEY,
    tenant_id     TEXT    NOT NULL DEFAULT 'local',
    recorder_id   TEXT    NOT NULL,
    camera_id     TEXT    NOT NULL,
    start_time    DATETIME NOT NULL,
    end_time      DATETIME NOT NULL,
    format        TEXT    NOT NULL DEFAULT 'mp4' CHECK (format IN ('mp4', 'mkv', 'ts')),
    status        TEXT    NOT NULL DEFAULT 'pending' CHECK (status IN ('pending', 'running', 'completed', 'failed')),
    requested_by  TEXT    NOT NULL DEFAULT '',
    error_message TEXT    NOT NULL DEFAULT '',
    download_url  TEXT    NOT NULL DEFAULT '',
    created_at    DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at    DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP
);

CREATE INDEX IF NOT EXISTS idx_export_jobs_status
    ON export_jobs (status);
