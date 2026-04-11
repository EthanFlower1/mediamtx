-- KAI-405: ProdataKey (PDK) cloud access control integration tables.

CREATE TABLE IF NOT EXISTS pdk_configs (
    config_id      TEXT NOT NULL,
    tenant_id      TEXT NOT NULL,
    api_endpoint   TEXT NOT NULL DEFAULT '',
    client_id      TEXT NOT NULL DEFAULT '',
    client_secret  TEXT NOT NULL DEFAULT '',
    panel_id       TEXT NOT NULL DEFAULT '',
    webhook_secret TEXT NOT NULL DEFAULT '',
    enabled        BOOLEAN NOT NULL DEFAULT 0,
    status         TEXT NOT NULL DEFAULT 'disconnected',
    last_sync_at   TIMESTAMPTZ,
    created_at     TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at     TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    PRIMARY KEY (config_id),
    UNIQUE (tenant_id)
);

CREATE TABLE IF NOT EXISTS pdk_doors (
    door_id      TEXT NOT NULL,
    tenant_id    TEXT NOT NULL,
    pdk_door_id  TEXT NOT NULL,
    name         TEXT NOT NULL DEFAULT '',
    location     TEXT NOT NULL DEFAULT '',
    is_locked    BOOLEAN NOT NULL DEFAULT 0,
    created_at   TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at   TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    PRIMARY KEY (door_id),
    UNIQUE (tenant_id, pdk_door_id)
);

CREATE TABLE IF NOT EXISTS pdk_door_events (
    event_id     TEXT NOT NULL,
    tenant_id    TEXT NOT NULL,
    door_id      TEXT NOT NULL,
    pdk_event_id TEXT NOT NULL DEFAULT '',
    event_type   TEXT NOT NULL DEFAULT '',
    person_name  TEXT NOT NULL DEFAULT '',
    credential   TEXT NOT NULL DEFAULT '',
    occurred_at  TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    raw_payload  TEXT NOT NULL DEFAULT '',
    created_at   TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    PRIMARY KEY (event_id)
);

-- postgres-only:begin
CREATE INDEX IF NOT EXISTS idx_pdk_door_events_tenant_occurred
    ON pdk_door_events (tenant_id, occurred_at DESC);
-- postgres-only:end

CREATE TABLE IF NOT EXISTS pdk_door_camera_mappings (
    mapping_id      TEXT NOT NULL,
    tenant_id       TEXT NOT NULL,
    door_id         TEXT NOT NULL,
    camera_path     TEXT NOT NULL,
    pre_buffer_sec  INTEGER NOT NULL DEFAULT 10,
    post_buffer_sec INTEGER NOT NULL DEFAULT 30,
    created_at      TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    PRIMARY KEY (mapping_id),
    UNIQUE (tenant_id, door_id, camera_path)
);

CREATE TABLE IF NOT EXISTS pdk_video_correlations (
    correlation_id TEXT NOT NULL,
    tenant_id      TEXT NOT NULL,
    event_id       TEXT NOT NULL,
    camera_path    TEXT NOT NULL,
    clip_start     TIMESTAMPTZ NOT NULL,
    clip_end       TIMESTAMPTZ NOT NULL,
    created_at     TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    PRIMARY KEY (correlation_id)
);

-- postgres-only:begin
CREATE INDEX IF NOT EXISTS idx_pdk_video_correlations_event
    ON pdk_video_correlations (tenant_id, event_id);
-- postgres-only:end
