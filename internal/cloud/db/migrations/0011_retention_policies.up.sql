-- KAI-249: retention_policies — named tiered retention configs owned by a tenant.
-- hot/warm/cold/archive_days control the age at which video moves through tiers.
-- encryption_mode selects the key management strategy for archived footage.

CREATE TABLE IF NOT EXISTS retention_policies (
    id              TEXT PRIMARY KEY,
    tenant_id       TEXT NOT NULL REFERENCES customer_tenants(id) ON DELETE CASCADE,
    name            TEXT NOT NULL,
    hot_days        INTEGER NOT NULL DEFAULT 7 CHECK (hot_days >= 0),
    warm_days       INTEGER NOT NULL DEFAULT 14 CHECK (warm_days >= 0),
    cold_days       INTEGER NOT NULL DEFAULT 30 CHECK (cold_days >= 0),
    archive_days    INTEGER NOT NULL DEFAULT 365 CHECK (archive_days >= 0),
    encryption_mode TEXT NOT NULL DEFAULT 'standard'
        CHECK (encryption_mode IN ('standard', 'sse_kms', 'cse_cmk')),
    region          TEXT NOT NULL DEFAULT 'us-east-2',
    created_at      TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at      TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_retention_tenant
    ON retention_policies(tenant_id);

CREATE INDEX IF NOT EXISTS idx_retention_region_tenant
    ON retention_policies(region, tenant_id);
