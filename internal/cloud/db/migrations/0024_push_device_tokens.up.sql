-- KAI-477: push notification device tokens + per-tenant push credentials.
--
-- device_tokens: per-user device registration for FCM, APNs, Web Push.
-- push_credentials: per-tenant platform credentials (FCM service account, APNs key, VAPID keys).

CREATE TABLE IF NOT EXISTS device_tokens (
    token_id        TEXT        NOT NULL,
    tenant_id       TEXT        NOT NULL,
    user_id         TEXT        NOT NULL,
    platform        TEXT        NOT NULL CHECK (platform IN ('fcm', 'apns', 'webpush')),
    device_token    TEXT        NOT NULL,
    device_name     TEXT        NOT NULL DEFAULT '',
    app_bundle_id   TEXT        NOT NULL DEFAULT '',
    created_at      TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at      TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    PRIMARY KEY (token_id, tenant_id)
);

CREATE UNIQUE INDEX IF NOT EXISTS idx_device_tokens_unique
    ON device_tokens (tenant_id, user_id, platform, device_token);

CREATE INDEX IF NOT EXISTS idx_device_tokens_user
    ON device_tokens (tenant_id, user_id);

CREATE TABLE IF NOT EXISTS push_credentials (
    credential_id   TEXT        NOT NULL,
    tenant_id       TEXT        NOT NULL,
    platform        TEXT        NOT NULL CHECK (platform IN ('fcm', 'apns', 'webpush')),
    credentials     JSONB       NOT NULL DEFAULT '{}'::jsonb,
    enabled         BOOLEAN     NOT NULL DEFAULT true,
    created_at      TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at      TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    PRIMARY KEY (credential_id, tenant_id)
);

CREATE UNIQUE INDEX IF NOT EXISTS idx_push_credentials_tenant_platform
    ON push_credentials (tenant_id, platform);
