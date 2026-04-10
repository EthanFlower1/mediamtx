-- KAI-364: per-tenant usage metering tables.
--
-- Two-table design:
--   - usage_events     append-only, high-cardinality, one row per record call.
--                      Source of truth for the daily aggregator job.
--   - usage_aggregates rollup table written by Aggregator.Run. Customer
--                      billing reports and Stripe usage exports read from this.
--
-- Every read AND write is keyed on tenant_id (Seam #4). There is no
-- "all tenants" surface in the metering Go package.
--
-- Postgres production (KAI-216) partitions usage_events by month via
-- pg_partman; that block is wrapped in postgres-only markers below so the
-- SQLite test runner skips it and falls through to a non-partitioned
-- equivalent. KAI-232 (CI/CD) will schedule the aggregator job; for now
-- Aggregator.Run is a callable function only.

-- postgres-only:begin
CREATE TABLE IF NOT EXISTS usage_events (
    id          BIGSERIAL,
    tenant_id   UUID        NOT NULL,
    ts          TIMESTAMPTZ NOT NULL,
    metric      TEXT        NOT NULL,
    value       DOUBLE PRECISION NOT NULL,
    PRIMARY KEY (id, ts)
) PARTITION BY RANGE (ts);

CREATE INDEX IF NOT EXISTS idx_usage_events_tenant_metric_ts
    ON usage_events (tenant_id, metric, ts);

SELECT partman.create_parent(
    p_parent_table  => 'public.usage_events',
    p_control       => 'ts',
    p_type          => 'range',
    p_interval      => 'monthly',
    p_premake       => 3
);
-- postgres-only:end

-- SQLite-compatible variant. The translator rewrites BIGSERIAL→INTEGER and
-- TIMESTAMPTZ→DATETIME. We use IF NOT EXISTS so the create is skipped when
-- the postgres-only block above already created the table; under SQLite
-- the postgres block is stripped and this CREATE runs.
CREATE TABLE IF NOT EXISTS usage_events (
    id          BIGSERIAL    PRIMARY KEY,
    tenant_id   TEXT         NOT NULL,
    ts          TIMESTAMPTZ  NOT NULL,
    metric      TEXT         NOT NULL,
    value       DOUBLE PRECISION NOT NULL
);

CREATE INDEX IF NOT EXISTS idx_usage_events_tenant_metric_ts
    ON usage_events (tenant_id, metric, ts);

-- usage_aggregates is identical in shape across dialects.
CREATE TABLE IF NOT EXISTS usage_aggregates (
    tenant_id      TEXT             NOT NULL,
    period_start   TIMESTAMPTZ      NOT NULL,
    period_end     TIMESTAMPTZ      NOT NULL,
    metric         TEXT             NOT NULL,
    sum            DOUBLE PRECISION NOT NULL,
    max            DOUBLE PRECISION NOT NULL,
    snapshot_count INTEGER          NOT NULL,
    PRIMARY KEY (tenant_id, period_start, metric)
);

CREATE INDEX IF NOT EXISTS idx_usage_aggregates_tenant_period
    ON usage_aggregates (tenant_id, period_start, period_end);
