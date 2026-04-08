-- KAI-218: customer_tenants — the "end customer" tenant record.
-- billing_mode = 'direct'        → customer is billed directly by the platform.
-- billing_mode = 'via_integrator' → customer is billed by home integrator with markup.

CREATE TABLE IF NOT EXISTS customer_tenants (
    id                    TEXT PRIMARY KEY,
    display_name          TEXT NOT NULL,
    billing_mode          TEXT NOT NULL
        CHECK (billing_mode IN ('direct', 'via_integrator')),
    home_integrator_id    TEXT REFERENCES integrators(id) ON DELETE RESTRICT,
    signup_source         TEXT,
    brand_override_id     TEXT,
    billing_account_id    TEXT,
    status                TEXT NOT NULL DEFAULT 'active'
        CHECK (status IN ('active', 'suspended', 'pending_verification', 'archived')),
    region                TEXT NOT NULL DEFAULT 'us-east-2',
    created_at            TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at            TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    -- A 'via_integrator' billing mode must name a home integrator.
    CHECK (
        (billing_mode = 'direct') OR
        (billing_mode = 'via_integrator' AND home_integrator_id IS NOT NULL)
    )
);

CREATE INDEX IF NOT EXISTS idx_customer_tenants_home_integrator
    ON customer_tenants(home_integrator_id);

-- Seam #9: tenant_id + region composite indexes on every tenant-scoped query path.
CREATE INDEX IF NOT EXISTS idx_customer_tenants_region_status
    ON customer_tenants(region, status);

CREATE INDEX IF NOT EXISTS idx_customer_tenants_region_id
    ON customer_tenants(region, id);
