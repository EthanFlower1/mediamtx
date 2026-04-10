-- KAI-356: Custom domains table for integrator white-label domain provisioning.

CREATE TABLE IF NOT EXISTS custom_domains (
    id              TEXT PRIMARY KEY,
    tenant_id       TEXT NOT NULL,
    domain          TEXT NOT NULL,
    cname_target    TEXT NOT NULL DEFAULT 'verify.kaivue.io.',
    status          TEXT NOT NULL DEFAULT 'pending'
                    CHECK (status IN ('pending', 'cname_verified', 'cert_provisioning', 'active', 'failed', 'revoked')),
    certificate_arn TEXT NOT NULL DEFAULT '',
    failure_reason  TEXT NOT NULL DEFAULT '',
    verified_at     TIMESTAMPTZ,
    activated_at    TIMESTAMPTZ,
    created_at      TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at      TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

-- Per-tenant uniqueness: one domain per tenant.
CREATE UNIQUE INDEX IF NOT EXISTS idx_custom_domains_tenant_domain
    ON custom_domains (tenant_id, domain);

-- Fast lookup by status for background verification/provisioning jobs.
CREATE INDEX IF NOT EXISTS idx_custom_domains_status
    ON custom_domains (status)
    WHERE status IN ('pending', 'cert_provisioning');
