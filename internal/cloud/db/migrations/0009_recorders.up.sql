-- KAI-249: recorders table — one row per on-prem Recorder appliance.
-- A Recorder belongs to exactly one on_prem_directory, which in turn
-- belongs to exactly one customer tenant. tenant_id is denormalised here
-- so every query that only needs the recorder can avoid the join.
--
-- CASCADE: deleting a directory cascades to its recorders (the hardware
-- is gone). Deleting a customer tenant RESTRICT-blocks on directories first.

CREATE TABLE IF NOT EXISTS recorders (
    id                      TEXT PRIMARY KEY,
    tenant_id               TEXT NOT NULL REFERENCES customer_tenants(id) ON DELETE CASCADE,
    directory_id            TEXT NOT NULL REFERENCES on_prem_directories(id) ON DELETE CASCADE,
    display_name            TEXT NOT NULL DEFAULT '',
    hardware_summary        JSONB NOT NULL DEFAULT '{}'::jsonb,
    status                  TEXT NOT NULL DEFAULT 'offline'
        CHECK (status IN ('online', 'degraded', 'offline', 'maintenance')),
    last_checkin_at         TIMESTAMPTZ,
    assigned_camera_count   INTEGER NOT NULL DEFAULT 0,
    storage_used_bytes      BIGINT NOT NULL DEFAULT 0,
    sidecar_status          JSONB NOT NULL DEFAULT '{}'::jsonb,
    lan_base_url            TEXT,
    relay_base_url          TEXT,
    lan_subnets             JSONB NOT NULL DEFAULT '[]'::jsonb,
    region                  TEXT NOT NULL DEFAULT 'us-east-2',
    created_at              TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at              TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

-- Seam #4: every query must be scoped to (tenant_id, ...).
CREATE INDEX IF NOT EXISTS idx_recorders_tenant
    ON recorders(tenant_id);

-- Seam #9: region + tenant composite.
CREATE INDEX IF NOT EXISTS idx_recorders_region_tenant
    ON recorders(region, tenant_id);

CREATE INDEX IF NOT EXISTS idx_recorders_directory
    ON recorders(directory_id);
