-- KAI-139: recording_schedules — per-camera (or per-tenant default) recording
-- mode and schedule configuration.
--
-- A row with camera_id = NULL defines a tenant-wide default; any row with a
-- concrete camera_id overrides the default for that camera.
--
-- mode values:
--   'continuous' — always record while camera is enabled.
--   'motion'     — record only on motion detection events.
--   'schedule'   — record per schedule_cron.
--   'off'        — no recording.
--
-- schedule_cron is a standard 5-field cron expression and is only consulted
-- when mode = 'schedule'.

CREATE TABLE IF NOT EXISTS recording_schedules (
    id                 TEXT    NOT NULL PRIMARY KEY,
    tenant_id          TEXT    NOT NULL,
    camera_id          TEXT,
    mode               TEXT    NOT NULL DEFAULT 'continuous'
                               CHECK (mode IN ('continuous', 'motion', 'schedule', 'off')),
    schedule_cron      TEXT,
    pre_roll_seconds   INTEGER NOT NULL DEFAULT 0,
    post_roll_seconds  INTEGER NOT NULL DEFAULT 0,
    created_at         DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at         DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
    FOREIGN KEY (camera_id) REFERENCES cameras (id) ON DELETE CASCADE
);

CREATE INDEX IF NOT EXISTS idx_recording_schedules_tenant
    ON recording_schedules (tenant_id);

CREATE INDEX IF NOT EXISTS idx_recording_schedules_camera
    ON recording_schedules (camera_id);
