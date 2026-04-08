-- KAI-218: the many-to-many between customer tenants and integrators.
-- Permissions are scoped; markup applies only for via_integrator billing.
-- Composite PK ensures a given (customer, integrator) pair is unique.
-- CASCADE: deleting an integrator or customer is blocked (RESTRICT) — relationships
-- must be explicitly revoked first so audit history is preserved.

CREATE TABLE IF NOT EXISTS customer_integrator_relationships (
    customer_tenant_id   TEXT NOT NULL REFERENCES customer_tenants(id) ON DELETE RESTRICT,
    integrator_id        TEXT NOT NULL REFERENCES integrators(id) ON DELETE RESTRICT,
    scoped_permissions   JSONB NOT NULL DEFAULT '{}'::jsonb,
    role_template        TEXT NOT NULL DEFAULT 'full_management'
        CHECK (role_template IN (
            'full_management',
            'monitoring_only',
            'emergency_access',
            'custom'
        )),
    markup_percent       NUMERIC(5,2) NOT NULL DEFAULT 0
        CHECK (markup_percent >= 0 AND markup_percent <= 500),
    status               TEXT NOT NULL DEFAULT 'pending_acceptance'
        CHECK (status IN ('pending_acceptance', 'active', 'suspended', 'revoked')),
    granted_at           TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    granted_by_user_id   TEXT,
    revoked_at           TIMESTAMPTZ,
    PRIMARY KEY (customer_tenant_id, integrator_id)
);

CREATE INDEX IF NOT EXISTS idx_cir_integrator
    ON customer_integrator_relationships(integrator_id);

CREATE INDEX IF NOT EXISTS idx_cir_customer
    ON customer_integrator_relationships(customer_tenant_id);

CREATE INDEX IF NOT EXISTS idx_cir_status
    ON customer_integrator_relationships(status);
