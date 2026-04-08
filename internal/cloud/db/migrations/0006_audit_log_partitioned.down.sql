-- postgres-only:begin
DROP INDEX IF EXISTS idx_audit_resource;
DROP INDEX IF EXISTS idx_audit_actor_time;
DROP INDEX IF EXISTS idx_audit_region_tenant_time;
DROP TABLE IF EXISTS audit_log_partitioned;
-- postgres-only:end
