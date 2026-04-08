-- KAI-218: on_prem_directories — the cloud's registry of on-prem Directory
-- instances. A Directory belongs to exactly one customer tenant. Pairing and
-- check-in timestamps are updated by KAI-243 / KAI-246 flows.
--
-- CASCADE: deleting a customer tenant blocks (RESTRICT) — Directories must be
-- decommissioned first.

CREATE TABLE IF NOT EXISTS on_prem_directories (
    id                  TEXT PRIMARY KEY,
    customer_tenant_id  TEXT NOT NULL REFERENCES customer_tenants(id) ON DELETE RESTRICT,
    display_name        TEXT NOT NULL,
    site_label          TEXT NOT NULL,
    deployment_mode     TEXT NOT NULL DEFAULT 'cloud_connected'
        CHECK (deployment_mode IN ('cloud_connected', 'hybrid', 'air_gapped')),
    paired_at           TIMESTAMPTZ,
    last_checkin_at     TIMESTAMPTZ,
    software_version    TEXT,
    capabilities        JSONB NOT NULL DEFAULT '{}'::jsonb,
    status              TEXT NOT NULL DEFAULT 'pending_pairing'
        CHECK (status IN ('pending_pairing', 'online', 'degraded', 'offline', 'archived')),
    region              TEXT NOT NULL DEFAULT 'us-east-2',
    created_at          TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at          TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_directories_customer
    ON on_prem_directories(customer_tenant_id);

-- Seam #9: region + tenant composite for tenant-scoped reads.
CREATE INDEX IF NOT EXISTS idx_directories_region_customer
    ON on_prem_directories(region, customer_tenant_id);

CREATE INDEX IF NOT EXISTS idx_directories_region_status
    ON on_prem_directories(region, status);
