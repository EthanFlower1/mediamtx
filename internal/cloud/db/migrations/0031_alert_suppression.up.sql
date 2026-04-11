-- KAI-373: ML-based alert suppression tables.
--
-- suppression_settings: per-camera sensitivity knob (0.0 = no suppression, 1.0 = aggressive).
-- activity_baselines: per-camera per-hour-of-day expected activity levels.
-- event_dismissals: tracks events dismissed without action (false positive signal).
-- suppressed_alerts: records suppressed notifications for UI visibility.

CREATE TABLE IF NOT EXISTS suppression_settings (
    tenant_id       TEXT        NOT NULL,
    camera_id       TEXT        NOT NULL,
    sensitivity     NUMERIC(5,2) NOT NULL DEFAULT 0.5,
    created_at      TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at      TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    PRIMARY KEY (tenant_id, camera_id)
);

CREATE TABLE IF NOT EXISTS activity_baselines (
    tenant_id       TEXT        NOT NULL,
    camera_id       TEXT        NOT NULL,
    hour_of_day     INTEGER     NOT NULL CHECK (hour_of_day >= 0 AND hour_of_day <= 23),
    day_of_week     INTEGER     NOT NULL CHECK (day_of_week >= 0 AND day_of_week <= 6),
    event_type      TEXT        NOT NULL,
    avg_count       NUMERIC(5,2) NOT NULL DEFAULT 0,
    stddev_count    NUMERIC(5,2) NOT NULL DEFAULT 0,
    sample_days     INTEGER     NOT NULL DEFAULT 0,
    updated_at      TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    PRIMARY KEY (tenant_id, camera_id, hour_of_day, day_of_week, event_type)
);

CREATE TABLE IF NOT EXISTS event_dismissals (
    dismissal_id    TEXT        NOT NULL,
    tenant_id       TEXT        NOT NULL,
    camera_id       TEXT        NOT NULL,
    event_type      TEXT        NOT NULL,
    dismissed_at    TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    PRIMARY KEY (dismissal_id, tenant_id)
);

CREATE INDEX IF NOT EXISTS idx_event_dismissals_camera
    ON event_dismissals (tenant_id, camera_id, event_type, dismissed_at DESC);

CREATE TABLE IF NOT EXISTS suppressed_alerts (
    alert_id        TEXT        NOT NULL,
    tenant_id       TEXT        NOT NULL,
    camera_id       TEXT        NOT NULL,
    event_type      TEXT        NOT NULL,
    reason          TEXT        NOT NULL CHECK (reason IN ('clustered', 'high_activity', 'false_positive')),
    cluster_id      TEXT        NOT NULL DEFAULT '',
    cluster_size    INTEGER     NOT NULL DEFAULT 1,
    cluster_summary TEXT        NOT NULL DEFAULT '',
    original_event  JSONB       NOT NULL DEFAULT '{}'::jsonb,
    suppressed_at   TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    PRIMARY KEY (alert_id, tenant_id)
);

CREATE INDEX IF NOT EXISTS idx_suppressed_alerts_tenant
    ON suppressed_alerts (tenant_id, camera_id, suppressed_at DESC);
