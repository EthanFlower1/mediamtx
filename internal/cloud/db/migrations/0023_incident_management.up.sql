-- KAI-382: incident management — PagerDuty integration, on-call rotation,
-- runbook linkage, and post-mortem workflow.
--
-- Design notes:
--   - paging_incidents: tracks incidents originating from Prometheus alerts
--     and paged through PagerDuty. Distinct from the status-page "incidents"
--     table (0021) which is customer-facing; these are operational/on-call.
--   - runbook_mappings: per-alert-name runbook URL mapping so alerts link
--     directly to the relevant runbook.
--   - oncall_schedules: simple on-call rotation entries per service.
--   - post_mortems: auto-populated from incident data (timeline, components,
--     metrics snapshot) with a review workflow.
--   - Cross-tenant isolation: every query MUST include tenant_id.

CREATE TABLE IF NOT EXISTS paging_incidents (
    incident_id        TEXT        NOT NULL,
    tenant_id          TEXT        NOT NULL,
    alert_name         TEXT        NOT NULL DEFAULT '',
    severity           TEXT        NOT NULL DEFAULT 'info' CHECK (severity IN (
                                       'critical',
                                       'error',
                                       'warning',
                                       'info'
                                   )),
    status             TEXT        NOT NULL DEFAULT 'triggered' CHECK (status IN (
                                       'triggered',
                                       'acknowledged',
                                       'resolved'
                                   )),
    summary            TEXT        NOT NULL DEFAULT '',
    source             TEXT        NOT NULL DEFAULT '',
    affected_component TEXT        NOT NULL DEFAULT '',
    pagerduty_key      TEXT        NOT NULL DEFAULT '',
    pagerduty_dedup_key TEXT       NOT NULL DEFAULT '',
    runbook_url        TEXT        NOT NULL DEFAULT '',
    triggered_at       TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    acknowledged_at    TIMESTAMPTZ,
    resolved_at        TIMESTAMPTZ,
    created_at         TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at         TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    PRIMARY KEY (incident_id, tenant_id)
);

CREATE INDEX IF NOT EXISTS idx_paging_incidents_tenant_status
    ON paging_incidents (tenant_id, status);

CREATE INDEX IF NOT EXISTS idx_paging_incidents_dedup
    ON paging_incidents (tenant_id, pagerduty_dedup_key);

CREATE TABLE IF NOT EXISTS runbook_mappings (
    mapping_id  TEXT        NOT NULL,
    tenant_id   TEXT        NOT NULL,
    alert_name  TEXT        NOT NULL,
    runbook_url TEXT        NOT NULL DEFAULT '',
    created_at  TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at  TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    PRIMARY KEY (mapping_id),
    UNIQUE (tenant_id, alert_name)
);

CREATE TABLE IF NOT EXISTS oncall_schedules (
    schedule_id  TEXT        NOT NULL,
    tenant_id    TEXT        NOT NULL,
    service_name TEXT        NOT NULL DEFAULT '',
    user_id      TEXT        NOT NULL DEFAULT '',
    start_time   TIMESTAMPTZ NOT NULL,
    end_time     TIMESTAMPTZ NOT NULL,
    created_at   TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at   TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    PRIMARY KEY (schedule_id)
);

CREATE INDEX IF NOT EXISTS idx_oncall_schedules_tenant_service
    ON oncall_schedules (tenant_id, service_name, start_time);

CREATE TABLE IF NOT EXISTS post_mortems (
    post_mortem_id      TEXT        NOT NULL,
    incident_id         TEXT        NOT NULL,
    tenant_id           TEXT        NOT NULL,
    title               TEXT        NOT NULL DEFAULT '',
    status              TEXT        NOT NULL DEFAULT 'draft' CHECK (status IN (
                                        'draft',
                                        'in_review',
                                        'published'
                                    )),
    summary             TEXT        NOT NULL DEFAULT '',
    timeline            JSONB       NOT NULL DEFAULT '[]'::jsonb,
    affected_components TEXT        NOT NULL DEFAULT '',
    root_cause          TEXT        NOT NULL DEFAULT '',
    action_items        JSONB       NOT NULL DEFAULT '[]'::jsonb,
    metrics_snapshot    JSONB       NOT NULL DEFAULT '{}'::jsonb,
    created_at          TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at          TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    PRIMARY KEY (post_mortem_id, tenant_id)
);

CREATE INDEX IF NOT EXISTS idx_post_mortems_incident
    ON post_mortems (tenant_id, incident_id);
