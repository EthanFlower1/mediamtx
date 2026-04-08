-- KAI-218: users — an authenticated identity belonging to EITHER an integrator
-- OR a customer tenant (never both). The tenant_ref_type/tenant_ref_id columns
-- form a polymorphic tenant reference so Casbin (KAI-225) can scope on a single
-- (type, id) pair regardless of whether the user is integrator-side or
-- customer-side.
--
-- zitadel_user_id is nullable only for fixtures / pre-provisioning flows. Once
-- the Zitadel adapter (KAI-223) lands, production writes must set it.

CREATE TABLE IF NOT EXISTS users (
    id               TEXT PRIMARY KEY,
    tenant_ref_type  TEXT NOT NULL
        CHECK (tenant_ref_type IN ('integrator', 'customer_tenant')),
    tenant_ref_id    TEXT NOT NULL,
    email            TEXT NOT NULL,
    display_name     TEXT,
    status           TEXT NOT NULL DEFAULT 'active'
        CHECK (status IN ('active', 'suspended', 'invited', 'archived')),
    zitadel_user_id  TEXT,
    region           TEXT NOT NULL DEFAULT 'us-east-2',
    created_at       TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at       TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

-- Email is globally unique across the identity layer. When Zitadel lands it
-- becomes the source of truth and this unique constraint stays as a safety net.
CREATE UNIQUE INDEX IF NOT EXISTS idx_users_email_unique
    ON users(email);

CREATE UNIQUE INDEX IF NOT EXISTS idx_users_zitadel_unique
    ON users(zitadel_user_id) WHERE zitadel_user_id IS NOT NULL;

-- Seam #9 + seam #4: region + tenant composite indexes for tenant-scoped reads.
CREATE INDEX IF NOT EXISTS idx_users_region_tenant
    ON users(region, tenant_ref_type, tenant_ref_id);

CREATE INDEX IF NOT EXISTS idx_users_region_status
    ON users(region, status);
