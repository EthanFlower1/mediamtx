-- KAI-370: notification delivery — per-tenant rate limits and delivery
-- provider configuration for SES (email) and Twilio (SMS).
--
-- Design notes:
--   - notification_rate_limits: per-tenant, per-channel rate limit config.
--     window_seconds + max_count define a sliding window. burst allows
--     short-term spikes above max_count.
--   - notification_delivery_providers: per-tenant provider credentials for
--     SES/Twilio. credentials is JSONB (encrypted at rest in production
--     via KAI-251 cryptostore; plaintext in SQLite tests).
--   - Cross-tenant isolation: every query MUST include tenant_id.
--   - SQLite tests: JSONB → TEXT, TIMESTAMPTZ → DATETIME, BOOLEAN → INTEGER
--     (translateToSQLite in migrations.go handles the rewrite).

CREATE TABLE IF NOT EXISTS notification_rate_limits (
    rate_limit_id   TEXT        NOT NULL,
    tenant_id       TEXT        NOT NULL,
    channel_type    TEXT        NOT NULL CHECK (channel_type IN (
                                    'email',
                                    'push',
                                    'sms',
                                    'webhook'
                                )),
    window_seconds  INTEGER     NOT NULL DEFAULT 3600,
    max_count       INTEGER     NOT NULL DEFAULT 100,
    burst           INTEGER     NOT NULL DEFAULT 10,
    enabled         BOOLEAN     NOT NULL DEFAULT true,
    created_at      TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at      TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    PRIMARY KEY (rate_limit_id, tenant_id)
);

CREATE UNIQUE INDEX IF NOT EXISTS idx_notification_rate_limits_tenant_channel
    ON notification_rate_limits (tenant_id, channel_type);

CREATE TABLE IF NOT EXISTS notification_delivery_providers (
    provider_id     TEXT        NOT NULL,
    tenant_id       TEXT        NOT NULL,
    channel_type    TEXT        NOT NULL CHECK (channel_type IN (
                                    'email',
                                    'sms'
                                )),
    provider_name   TEXT        NOT NULL CHECK (provider_name IN (
                                    'ses',
                                    'twilio'
                                )),
    credentials     JSONB       NOT NULL DEFAULT '{}'::jsonb,
    from_address    TEXT        NOT NULL DEFAULT '',
    enabled         BOOLEAN     NOT NULL DEFAULT true,
    created_at      TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at      TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    PRIMARY KEY (provider_id, tenant_id)
);

CREATE UNIQUE INDEX IF NOT EXISTS idx_notification_delivery_providers_tenant_channel
    ON notification_delivery_providers (tenant_id, channel_type);
