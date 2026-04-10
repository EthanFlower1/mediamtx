-- KAI-366: notification preferences store + delivery routing.
--
-- notification_channels: per-tenant channel configs (email, push, SMS, webhook).
-- notification_preferences: per-user-per-tenant delivery preferences by event type.
-- notification_log: delivery audit trail.
--
-- SQLite tests: JSONB → TEXT, TIMESTAMPTZ → DATETIME via translateToSQLite.

CREATE TABLE IF NOT EXISTS notification_channels (
    channel_id      TEXT        NOT NULL,
    tenant_id       TEXT        NOT NULL,
    channel_type    TEXT        NOT NULL CHECK (channel_type IN ('email', 'push', 'sms', 'webhook')),
    config          JSONB       NOT NULL DEFAULT '{}'::jsonb,
    enabled         BOOLEAN     NOT NULL DEFAULT true,
    created_at      TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at      TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    PRIMARY KEY (channel_id, tenant_id)
);

CREATE INDEX IF NOT EXISTS idx_notification_channels_tenant
    ON notification_channels (tenant_id, channel_type);

CREATE TABLE IF NOT EXISTS notification_preferences (
    preference_id   TEXT        NOT NULL,
    tenant_id       TEXT        NOT NULL,
    user_id         TEXT        NOT NULL,
    event_type      TEXT        NOT NULL,
    channel_type    TEXT        NOT NULL,
    enabled         BOOLEAN     NOT NULL DEFAULT true,
    created_at      TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at      TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    PRIMARY KEY (preference_id, tenant_id)
);

CREATE UNIQUE INDEX IF NOT EXISTS idx_notification_prefs_user_event_channel
    ON notification_preferences (tenant_id, user_id, event_type, channel_type);

CREATE TABLE IF NOT EXISTS notification_log (
    log_id          TEXT        NOT NULL,
    tenant_id       TEXT        NOT NULL,
    user_id         TEXT        NOT NULL,
    event_type      TEXT        NOT NULL,
    channel_type    TEXT        NOT NULL,
    status          TEXT        NOT NULL DEFAULT 'pending' CHECK (status IN ('pending', 'sent', 'failed', 'suppressed')),
    error_message   TEXT        NOT NULL DEFAULT '',
    sent_at         TIMESTAMPTZ,
    created_at      TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    PRIMARY KEY (log_id, tenant_id)
);

CREATE INDEX IF NOT EXISTS idx_notification_log_tenant_status
    ON notification_log (tenant_id, status, created_at DESC);
