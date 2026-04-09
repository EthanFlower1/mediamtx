-- KAI-362: align billing schema for invoice calculator.
--
-- Background: the billing-related columns themselves were introduced earlier:
--   * 0001 (integrators):   billing_mode, wholesale_discount_percent
--   * 0002 (customer_tenants): billing_mode (NOT NULL, no default)
--   * 0003 (customer_integrator_relationships): markup_percent (0..500)
--
-- This migration finalises the billing contract for the KAI-362 calculator:
--   * customer_tenants.billing_mode gains DEFAULT 'direct' so newly-provisioned
--     direct tenants don't have to pass it explicitly.
--   * customer_integrator_relationships.markup_percent tightens its CHECK
--     range from 0..500 down to 0..100 (the calculator and the public docs
--     express markup as a percentage <= 100).
--
-- SQLite cannot ALTER COLUMN / drop CHECK constraints in-place, but the
-- 0001..0003 schemas it builds in test mode already match the post-migration
-- shape closely enough for the calculator + Store to exercise it (the SQLite
-- CHECK on markup_percent stays 0..500 in tests, which is a strict superset of
-- the Postgres production range; tests stay within 0..100 anyway). The
-- Postgres-only block below performs the real schema mutation in production.

-- postgres-only:begin
ALTER TABLE customer_tenants
    ALTER COLUMN billing_mode SET DEFAULT 'direct';

ALTER TABLE customer_integrator_relationships
    DROP CONSTRAINT IF EXISTS customer_integrator_relationships_markup_percent_check;

ALTER TABLE customer_integrator_relationships
    ADD CONSTRAINT customer_integrator_relationships_markup_percent_check
    CHECK (markup_percent >= 0 AND markup_percent <= 100);
-- postgres-only:end
