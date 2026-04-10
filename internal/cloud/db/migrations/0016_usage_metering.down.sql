-- KAI-364 rollback: remove usage_events + usage_aggregates.
--
-- Under Postgres production (KAI-216) usage_events is a partitioned parent;
-- DROP TABLE cascades the partitions. Under SQLite the same DROP removes the
-- non-partitioned fallback table created by the .up.sql.

DROP INDEX IF EXISTS idx_usage_aggregates_tenant_period;
DROP TABLE IF EXISTS usage_aggregates;

DROP INDEX IF EXISTS idx_usage_events_tenant_metric_ts;

-- postgres-only:begin
-- Drop partman-managed partitions first so the parent drop succeeds cleanly.
DO $$
BEGIN
    IF EXISTS (SELECT 1 FROM pg_extension WHERE extname = 'pg_partman') THEN
        PERFORM partman.undo_partition(
            p_parent_table => 'public.usage_events',
            p_keep_table   => false
        );
    END IF;
END $$;
-- postgres-only:end

DROP TABLE IF EXISTS usage_events;
