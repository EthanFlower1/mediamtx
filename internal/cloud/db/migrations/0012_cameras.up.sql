-- KAI-249: cameras — the cloud camera registry. One row per camera, scoped to
-- a customer tenant and a directory. RTSP credentials are stored only as an
-- encrypted blob (cryptostore KAI-251); plaintext is never persisted.
--
-- FKs:
--   tenant_id            → customer_tenants(id)     CASCADE
--   directory_id         → on_prem_directories(id)  CASCADE
--   assigned_recorder_id → recorders(id)             SET NULL
--   schedule_id          → recording_schedules(id)   SET NULL
--   retention_policy_id  → retention_policies(id)    SET NULL
--
-- NOTE: 0008_cameras_lpr_enabled.up.sql conditionally adds lpr_enabled only
-- if the cameras table already exists. Since 0008 runs before 0012, that block
-- is always a no-op. We add lpr_enabled here as a native column.

CREATE TABLE IF NOT EXISTS cameras (
    id                          TEXT PRIMARY KEY,
    tenant_id                   TEXT NOT NULL REFERENCES customer_tenants(id) ON DELETE CASCADE,
    directory_id                TEXT NOT NULL REFERENCES on_prem_directories(id) ON DELETE CASCADE,
    display_name                TEXT NOT NULL,
    location_label              TEXT,
    manufacturer                TEXT,
    model                       TEXT,
    onvif_endpoint              TEXT,
    rtsp_url                    TEXT,
    rtsp_credentials_encrypted  BYTEA,
    assigned_recorder_id        TEXT REFERENCES recorders(id) ON DELETE SET NULL,
    schedule_id                 TEXT REFERENCES recording_schedules(id) ON DELETE SET NULL,
    retention_policy_id         TEXT REFERENCES retention_policies(id) ON DELETE SET NULL,
    ai_features                 JSONB NOT NULL DEFAULT '{}'::jsonb,
    lpr_enabled                 BOOLEAN NOT NULL DEFAULT FALSE,
    status                      TEXT NOT NULL DEFAULT 'unconfigured'
        CHECK (status IN ('unconfigured', 'connecting', 'online', 'degraded', 'offline', 'archived')),
    region                      TEXT NOT NULL DEFAULT 'us-east-2',
    created_at                  TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at                  TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

-- Seam #4: all common queries are scoped to (tenant_id, ...).
CREATE INDEX IF NOT EXISTS idx_cameras_tenant
    ON cameras(tenant_id);

-- Seam #9: region + tenant composite for multi-region reads.
CREATE INDEX IF NOT EXISTS idx_cameras_region_tenant
    ON cameras(region, tenant_id);

CREATE INDEX IF NOT EXISTS idx_cameras_directory
    ON cameras(directory_id);

-- Partial indexes for assigned cameras — Postgres only.
-- SQLite falls back to the non-partial indexes above.
-- postgres-only:begin
CREATE INDEX IF NOT EXISTS idx_cameras_recorder
    ON cameras(assigned_recorder_id)
    WHERE assigned_recorder_id IS NOT NULL;

CREATE INDEX IF NOT EXISTS idx_cameras_tenant_recorder
    ON cameras(tenant_id, assigned_recorder_id)
    WHERE assigned_recorder_id IS NOT NULL;
-- postgres-only:end
