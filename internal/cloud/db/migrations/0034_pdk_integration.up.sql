-- KAI-PDK: PDK access control integration tables

CREATE TABLE IF NOT EXISTS pdk_configs (
    config_id      TEXT NOT NULL PRIMARY KEY,
    tenant_id      TEXT NOT NULL UNIQUE,
    api_endpoint   TEXT NOT NULL,
    client_id      TEXT NOT NULL,
    client_secret  TEXT NOT NULL,
    panel_id       TEXT NOT NULL,
    webhook_secret TEXT NOT NULL DEFAULT '',
    enabled        BOOLEAN NOT NULL DEFAULT FALSE,
    status         TEXT NOT NULL DEFAULT 'disconnected',
    last_sync_at   TIMESTAMP,
    created_at     TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at     TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP
);

CREATE TABLE IF NOT EXISTS pdk_doors (
    door_id      TEXT NOT NULL PRIMARY KEY,
    tenant_id    TEXT NOT NULL,
    pdk_door_id  TEXT NOT NULL,
    name         TEXT NOT NULL,
    location     TEXT NOT NULL DEFAULT '',
    is_locked    BOOLEAN NOT NULL DEFAULT FALSE,
    created_at   TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at   TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
    UNIQUE(tenant_id, pdk_door_id)
);

CREATE TABLE IF NOT EXISTS pdk_door_events (
    event_id      TEXT NOT NULL PRIMARY KEY,
    tenant_id     TEXT NOT NULL,
    door_id       TEXT NOT NULL,
    pdk_event_id  TEXT NOT NULL,
    event_type    TEXT NOT NULL,
    person_name   TEXT NOT NULL DEFAULT '',
    credential    TEXT NOT NULL DEFAULT '',
    occurred_at   TIMESTAMP NOT NULL,
    raw_payload   TEXT NOT NULL DEFAULT '',
    created_at    TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP
);

CREATE INDEX IF NOT EXISTS idx_pdk_door_events_tenant_occurred
    ON pdk_door_events (tenant_id, occurred_at DESC);

CREATE TABLE IF NOT EXISTS pdk_door_camera_mappings (
    mapping_id      TEXT NOT NULL PRIMARY KEY,
    tenant_id       TEXT NOT NULL,
    door_id         TEXT NOT NULL,
    camera_path     TEXT NOT NULL,
    pre_buffer_sec  INTEGER NOT NULL DEFAULT 10,
    post_buffer_sec INTEGER NOT NULL DEFAULT 30,
    created_at      TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
    UNIQUE(tenant_id, door_id, camera_path)
);

CREATE TABLE IF NOT EXISTS pdk_video_correlations (
    correlation_id  TEXT NOT NULL PRIMARY KEY,
    tenant_id       TEXT NOT NULL,
    event_id        TEXT NOT NULL,
    camera_path     TEXT NOT NULL,
    clip_start      TIMESTAMP NOT NULL,
    clip_end        TIMESTAMP NOT NULL,
    created_at      TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP
);

CREATE INDEX IF NOT EXISTS idx_pdk_video_correlations_tenant_event
    ON pdk_video_correlations (tenant_id, event_id);
