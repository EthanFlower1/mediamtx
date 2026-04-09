-- KAI-362 rollback: revert the DEFAULT and the markup_percent CHECK widening.
-- The columns themselves are owned by 0001..0003 and are NOT dropped here.

-- postgres-only:begin
ALTER TABLE customer_tenants
    ALTER COLUMN billing_mode DROP DEFAULT;

ALTER TABLE customer_integrator_relationships
    DROP CONSTRAINT IF EXISTS customer_integrator_relationships_markup_percent_check;

ALTER TABLE customer_integrator_relationships
    ADD CONSTRAINT customer_integrator_relationships_markup_percent_check
    CHECK (markup_percent >= 0 AND markup_percent <= 500);
-- postgres-only:end
