-- KAI-218: declare the audit_log parent table for pg_partman monthly partitioning.
-- KAI-233 (audit log service) builds the writer, partition management, and
-- retention policy. This ticket only declares the parent table and its indexes
-- so sibling work can reference the schema shape.
--
-- NOTE on Postgres-only syntax: PARTITION BY RANGE and pg_partman config live in
-- the .postgres suffix block below. SQLite tests skip this migration (see the Go
-- runner's `postgresOnly` marker). Manual Postgres integration validation lands
-- with KAI-216 when RDS is provisioned.

-- postgres-only:begin
CREATE TABLE IF NOT EXISTS audit_log_partitioned (
    id               BIGSERIAL,
    occurred_at      TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    actor_type       TEXT NOT NULL
        CHECK (actor_type IN ('user', 'integrator_staff', 'system', 'api_key', 'federation_peer')),
    actor_id         TEXT NOT NULL,
    tenant_ref_type  TEXT NOT NULL
        CHECK (tenant_ref_type IN ('integrator', 'customer_tenant', 'platform')),
    tenant_ref_id    TEXT NOT NULL,
    action           TEXT NOT NULL,
    resource_type    TEXT NOT NULL,
    resource_id      TEXT,
    result           TEXT NOT NULL
        CHECK (result IN ('allow', 'deny', 'error')),
    metadata         JSONB NOT NULL DEFAULT '{}'::jsonb,
    region           TEXT NOT NULL DEFAULT 'us-east-2',
    PRIMARY KEY (id, occurred_at)
) PARTITION BY RANGE (occurred_at);

CREATE INDEX IF NOT EXISTS idx_audit_region_tenant_time
    ON audit_log_partitioned (region, tenant_ref_type, tenant_ref_id, occurred_at DESC);

CREATE INDEX IF NOT EXISTS idx_audit_actor_time
    ON audit_log_partitioned (actor_type, actor_id, occurred_at DESC);

CREATE INDEX IF NOT EXISTS idx_audit_resource
    ON audit_log_partitioned (resource_type, resource_id, occurred_at DESC);
-- postgres-only:end
