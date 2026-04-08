-- KAI-249: recording_schedules — named recording schedules owned by a tenant.
-- schedule_type controls when recording triggers: continuous, motion-triggered,
-- or event-triggered. weekly_grid is a JSON array of 7 × 48 half-hour slots.
--
-- A schedule can be referenced by many cameras (FK on cameras.schedule_id).

CREATE TABLE IF NOT EXISTS recording_schedules (
    id              TEXT PRIMARY KEY,
    tenant_id       TEXT NOT NULL REFERENCES customer_tenants(id) ON DELETE CASCADE,
    name            TEXT NOT NULL,
    schedule_type   TEXT NOT NULL DEFAULT 'continuous'
        CHECK (schedule_type IN ('continuous', 'motion', 'event')),
    weekly_grid     JSONB NOT NULL DEFAULT '{}'::jsonb,
    region          TEXT NOT NULL DEFAULT 'us-east-2',
    created_at      TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at      TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_schedules_tenant
    ON recording_schedules(tenant_id);

CREATE INDEX IF NOT EXISTS idx_schedules_region_tenant
    ON recording_schedules(region, tenant_id);
