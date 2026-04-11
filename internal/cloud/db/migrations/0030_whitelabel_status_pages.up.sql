-- KAI-376: per-integrator white-label status pages.
--
-- integrator_status_configs: subdomain routing, custom branding overrides,
--   and component filter for each integrator's public status page.
-- status_page_subscribers: email subscribers per integrator who receive
--   incident notifications for that integrator's status page.
--
-- SQLite tests: JSONB → TEXT, TIMESTAMPTZ → DATETIME, BOOLEAN → INTEGER
-- via translateToSQLite in migrations.go.

CREATE TABLE IF NOT EXISTS integrator_status_configs (
    integrator_id   TEXT        PRIMARY KEY,
    subdomain       TEXT        NOT NULL,
    custom_domain   TEXT        NOT NULL DEFAULT '',
    page_title      TEXT        NOT NULL DEFAULT '',
    logo_url        TEXT        NOT NULL DEFAULT '',
    favicon_url     TEXT        NOT NULL DEFAULT '',
    primary_color   TEXT        NOT NULL DEFAULT '#0066FF',
    secondary_color TEXT        NOT NULL DEFAULT '#FFFFFF',
    accent_color    TEXT        NOT NULL DEFAULT '#333333',
    header_bg_color TEXT        NOT NULL DEFAULT '#FFFFFF',
    footer_text     TEXT        NOT NULL DEFAULT '',
    custom_css      TEXT        NOT NULL DEFAULT '',
    component_ids   JSONB       NOT NULL DEFAULT '[]'::jsonb,
    support_url     TEXT        NOT NULL DEFAULT '',
    enabled         BOOLEAN     NOT NULL DEFAULT true,
    created_at      TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at      TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE UNIQUE INDEX IF NOT EXISTS idx_integrator_status_subdomain
    ON integrator_status_configs (subdomain);

CREATE TABLE IF NOT EXISTS status_page_subscribers (
    subscriber_id   TEXT        NOT NULL,
    integrator_id   TEXT        NOT NULL,
    email           TEXT        NOT NULL,
    confirmed       BOOLEAN     NOT NULL DEFAULT false,
    confirm_token   TEXT        NOT NULL DEFAULT '',
    created_at      TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at      TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    PRIMARY KEY (subscriber_id, integrator_id)
);

CREATE UNIQUE INDEX IF NOT EXISTS idx_status_subscribers_email
    ON status_page_subscribers (integrator_id, email);

CREATE INDEX IF NOT EXISTS idx_status_subscribers_confirmed
    ON status_page_subscribers (integrator_id, confirmed);
