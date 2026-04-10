-- KAI-369: support ticket integration — per-tenant support tickets with
-- external provider sync (Zendesk, Freshdesk) and internal comment thread.
--
-- Design notes:
--   - support_tickets: core ticket entity. external_id + provider track the
--     upstream ticket in Zendesk/Freshdesk. status mirrors the external
--     provider's lifecycle.
--   - support_ticket_comments: internal + synced comment thread per ticket.
--     source distinguishes user-created vs webhook-synced comments.
--   - support_provider_configs: per-tenant webhook + API credentials for
--     Zendesk/Freshdesk. api_credentials is JSONB (encrypted at rest via
--     KAI-251 cryptostore in production; plaintext in SQLite tests).
--   - Cross-tenant isolation: every query MUST include tenant_id.
--   - SQLite tests: JSONB → TEXT, TIMESTAMPTZ → DATETIME, BOOLEAN → INTEGER
--     (translateToSQLite in migrations.go handles the rewrite).

CREATE TABLE IF NOT EXISTS support_tickets (
    ticket_id       TEXT        NOT NULL,
    tenant_id       TEXT        NOT NULL,
    external_id     TEXT,
    provider        TEXT        NOT NULL DEFAULT 'internal' CHECK (provider IN (
                                    'internal',
                                    'zendesk',
                                    'freshdesk'
                                )),
    subject         TEXT        NOT NULL DEFAULT '',
    description     TEXT        NOT NULL DEFAULT '',
    status          TEXT        NOT NULL DEFAULT 'open' CHECK (status IN (
                                    'open',
                                    'pending',
                                    'in_progress',
                                    'waiting_on_customer',
                                    'resolved',
                                    'closed'
                                )),
    priority        TEXT        NOT NULL DEFAULT 'normal' CHECK (priority IN (
                                    'low',
                                    'normal',
                                    'high',
                                    'urgent'
                                )),
    requester_id    TEXT        NOT NULL DEFAULT '',
    assignee_id     TEXT,
    tags            JSONB       NOT NULL DEFAULT '[]'::jsonb,
    metadata        JSONB       NOT NULL DEFAULT '{}'::jsonb,
    created_at      TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at      TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    PRIMARY KEY (ticket_id, tenant_id)
);

CREATE INDEX IF NOT EXISTS idx_support_tickets_tenant_status
    ON support_tickets (tenant_id, status);

CREATE INDEX IF NOT EXISTS idx_support_tickets_tenant_requester
    ON support_tickets (tenant_id, requester_id);

CREATE UNIQUE INDEX IF NOT EXISTS idx_support_tickets_external
    ON support_tickets (tenant_id, provider, external_id)
    WHERE external_id IS NOT NULL;

CREATE TABLE IF NOT EXISTS support_ticket_comments (
    comment_id      TEXT        NOT NULL,
    ticket_id       TEXT        NOT NULL,
    tenant_id       TEXT        NOT NULL,
    author_id       TEXT        NOT NULL DEFAULT '',
    body            TEXT        NOT NULL DEFAULT '',
    source          TEXT        NOT NULL DEFAULT 'user' CHECK (source IN (
                                    'user',
                                    'agent',
                                    'system',
                                    'webhook'
                                )),
    is_public       BOOLEAN     NOT NULL DEFAULT true,
    created_at      TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    PRIMARY KEY (comment_id, tenant_id)
);

CREATE INDEX IF NOT EXISTS idx_support_ticket_comments_ticket
    ON support_ticket_comments (tenant_id, ticket_id, created_at);

CREATE TABLE IF NOT EXISTS support_provider_configs (
    config_id       TEXT        NOT NULL,
    tenant_id       TEXT        NOT NULL,
    provider        TEXT        NOT NULL CHECK (provider IN (
                                    'zendesk',
                                    'freshdesk'
                                )),
    webhook_secret  TEXT        NOT NULL DEFAULT '',
    api_credentials JSONB       NOT NULL DEFAULT '{}'::jsonb,
    enabled         BOOLEAN     NOT NULL DEFAULT true,
    created_at      TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at      TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    PRIMARY KEY (config_id, tenant_id)
);

CREATE UNIQUE INDEX IF NOT EXISTS idx_support_provider_configs_tenant_provider
    ON support_provider_configs (tenant_id, provider);
