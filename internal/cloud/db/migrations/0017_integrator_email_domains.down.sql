DROP INDEX IF EXISTS idx_dkim_keys_tenant;
DROP INDEX IF EXISTS idx_dkim_keys_domain_selector;
DROP TABLE IF EXISTS dkim_keys;

DROP INDEX IF EXISTS idx_integrator_email_domains_state;
DROP INDEX IF EXISTS idx_integrator_email_domains_tenant_domain;
DROP TABLE IF EXISTS integrator_email_domains;
