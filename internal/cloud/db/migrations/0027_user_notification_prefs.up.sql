-- KAI-371: Per-user per-camera notification preferences with quiet hours
-- and severity thresholds.
--
-- This table extends the simple notification_preferences (0020) with camera
-- granularity, severity thresholds, and quiet-hours suppression windows.
-- A NULL camera_id means "all cameras"; a NULL event_type means "all events".
-- Resolution order: most-specific (camera+event) wins over wildcards.

CREATE TABLE IF NOT EXISTS user_notification_prefs (
    pref_id         TEXT        NOT NULL,
    tenant_id       TEXT        NOT NULL,
    user_id         TEXT        NOT NULL,
    camera_id       TEXT,                           -- NULL = all cameras
    event_type      TEXT,                           -- NULL = all event types
    channels        JSONB       NOT NULL DEFAULT '[]'::jsonb,  -- array of channel types
    severity_min    TEXT        NOT NULL DEFAULT 'info' CHECK (severity_min IN ('info', 'warning', 'critical')),
    quiet_start     TEXT,                           -- HH:MM in user's timezone, NULL = no quiet hours
    quiet_end       TEXT,                           -- HH:MM in user's timezone
    quiet_timezone  TEXT        NOT NULL DEFAULT 'UTC',
    quiet_days      JSONB       NOT NULL DEFAULT '[]'::jsonb,  -- array of ints 0=Sun..6=Sat; empty = every day
    enabled         BOOLEAN     NOT NULL DEFAULT true,
    created_at      TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at      TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    PRIMARY KEY (pref_id)
);

CREATE UNIQUE INDEX IF NOT EXISTS idx_user_notif_prefs_lookup
    ON user_notification_prefs (tenant_id, user_id, camera_id, event_type);

CREATE INDEX IF NOT EXISTS idx_user_notif_prefs_tenant_user
    ON user_notification_prefs (tenant_id, user_id);
