-- KAI-218: integrators table + sub-reseller hierarchy.
-- Seam #9: every tenant-scoped table carries a region column.

CREATE TABLE IF NOT EXISTS integrators (
    id                         TEXT PRIMARY KEY,
    parent_integrator_id       TEXT REFERENCES integrators(id) ON DELETE RESTRICT,
    display_name               TEXT NOT NULL,
    legal_name                 TEXT,
    contact_email              TEXT,
    billing_mode               TEXT NOT NULL DEFAULT 'direct'
        CHECK (billing_mode IN ('direct', 'via_integrator')),
    wholesale_discount_percent NUMERIC(5,2) NOT NULL DEFAULT 0
        CHECK (wholesale_discount_percent >= 0 AND wholesale_discount_percent <= 100),
    brand_config_id            TEXT,
    billing_account_id         TEXT,
    status                     TEXT NOT NULL DEFAULT 'active'
        CHECK (status IN ('active', 'suspended', 'pending_verification', 'archived')),
    region                     TEXT NOT NULL DEFAULT 'us-east-2',
    created_at                 TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at                 TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_integrators_parent
    ON integrators(parent_integrator_id);

-- Seam #9: region-scoped query paths need composite indexes.
CREATE INDEX IF NOT EXISTS idx_integrators_region_status
    ON integrators(region, status);

CREATE INDEX IF NOT EXISTS idx_integrators_region_id
    ON integrators(region, id);
