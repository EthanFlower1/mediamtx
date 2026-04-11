-- KAI-377: status subscriber notifications.
--
-- status_subscribers: tracks who subscribes to status updates, via which
-- channel (email, sms, webhook, rss, slack, teams), and optionally filters
-- by specific components.
--
-- status_events: a denormalised log of every status change or incident update
-- that was fan-out dispatched. Useful for RSS feed generation and audit.

CREATE TABLE IF NOT EXISTS status_subscribers (
    subscriber_id    TEXT        NOT NULL,
    tenant_id        TEXT        NOT NULL,
    channel_type     TEXT        NOT NULL CHECK (channel_type IN (
                                    'email', 'sms', 'webhook', 'rss', 'slack', 'teams'
                                )),
    channel_config   JSONB       NOT NULL DEFAULT '{}'::jsonb,
    component_filter JSONB       NOT NULL DEFAULT '[]'::jsonb,
    confirmed        BOOLEAN     NOT NULL DEFAULT false,
    created_at       TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at       TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    PRIMARY KEY (subscriber_id, tenant_id)
);

CREATE INDEX IF NOT EXISTS idx_status_subscribers_tenant_channel
    ON status_subscribers (tenant_id, channel_type);

CREATE TABLE IF NOT EXISTS status_events (
    event_id         TEXT        NOT NULL,
    tenant_id        TEXT        NOT NULL,
    event_type       TEXT        NOT NULL CHECK (event_type IN (
                                    'status_change', 'incident_created',
                                    'incident_updated', 'incident_resolved'
                                )),
    title            TEXT        NOT NULL DEFAULT '',
    description      TEXT        NOT NULL DEFAULT '',
    affected_components JSONB    NOT NULL DEFAULT '[]'::jsonb,
    severity         TEXT        NOT NULL DEFAULT '',
    created_at       TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    PRIMARY KEY (event_id, tenant_id)
);

CREATE INDEX IF NOT EXISTS idx_status_events_tenant_created
    ON status_events (tenant_id, created_at DESC);
