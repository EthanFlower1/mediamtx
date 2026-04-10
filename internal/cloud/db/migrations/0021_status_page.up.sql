-- KAI-368: status page — per-tenant service health checks, incidents, and
-- incident updates for a customer-facing status page.
--
-- Design notes:
--   - service_health_checks: tracks individual services (e.g. "cloud_api",
--     "recording_pipeline", "live_view"). status is one of: operational,
--     degraded, partial_outage, major_outage.
--   - incidents: tracks ongoing or resolved incidents that affect one or more
--     services. severity: minor, major, critical.
--   - incident_updates: timestamped updates posted to an incident (status
--     update log visible to customers).
--   - Cross-tenant isolation: every query MUST include tenant_id.
--   - SQLite tests: JSONB → TEXT, TIMESTAMPTZ → DATETIME, BOOLEAN → INTEGER
--     (translateToSQLite in migrations.go handles the rewrite).

CREATE TABLE IF NOT EXISTS service_health_checks (
    check_id        TEXT        NOT NULL,
    tenant_id       TEXT        NOT NULL,
    service_name    TEXT        NOT NULL,
    display_name    TEXT        NOT NULL DEFAULT '',
    status          TEXT        NOT NULL DEFAULT 'operational' CHECK (status IN (
                                    'operational',
                                    'degraded',
                                    'partial_outage',
                                    'major_outage'
                                )),
    last_checked_at TIMESTAMPTZ,
    metadata        JSONB       NOT NULL DEFAULT '{}'::jsonb,
    enabled         BOOLEAN     NOT NULL DEFAULT true,
    created_at      TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at      TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    PRIMARY KEY (check_id, tenant_id)
);

CREATE UNIQUE INDEX IF NOT EXISTS idx_service_health_checks_tenant_service
    ON service_health_checks (tenant_id, service_name);

CREATE TABLE IF NOT EXISTS incidents (
    incident_id     TEXT        NOT NULL,
    tenant_id       TEXT        NOT NULL,
    title           TEXT        NOT NULL,
    severity        TEXT        NOT NULL DEFAULT 'minor' CHECK (severity IN (
                                    'minor',
                                    'major',
                                    'critical'
                                )),
    status          TEXT        NOT NULL DEFAULT 'investigating' CHECK (status IN (
                                    'investigating',
                                    'identified',
                                    'monitoring',
                                    'resolved'
                                )),
    affected_services JSONB     NOT NULL DEFAULT '[]'::jsonb,
    started_at      TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    resolved_at     TIMESTAMPTZ,
    created_at      TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at      TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    PRIMARY KEY (incident_id, tenant_id)
);

CREATE INDEX IF NOT EXISTS idx_incidents_tenant_status
    ON incidents (tenant_id, status);

CREATE INDEX IF NOT EXISTS idx_incidents_tenant_started
    ON incidents (tenant_id, started_at DESC);

CREATE TABLE IF NOT EXISTS incident_updates (
    update_id       TEXT        NOT NULL,
    incident_id     TEXT        NOT NULL,
    tenant_id       TEXT        NOT NULL,
    status          TEXT        NOT NULL CHECK (status IN (
                                    'investigating',
                                    'identified',
                                    'monitoring',
                                    'resolved'
                                )),
    message         TEXT        NOT NULL DEFAULT '',
    created_at      TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    PRIMARY KEY (update_id, tenant_id)
);

CREATE INDEX IF NOT EXISTS idx_incident_updates_incident
    ON incident_updates (tenant_id, incident_id, created_at);
