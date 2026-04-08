-- KAI-283: Remove lpr_enabled column from cameras table.
-- postgres-only:begin
DO $$
BEGIN
    IF EXISTS (
        SELECT 1 FROM information_schema.tables
        WHERE table_schema = 'public' AND table_name = 'cameras'
    ) THEN
        ALTER TABLE cameras DROP COLUMN IF EXISTS lpr_enabled;
    END IF;
END $$;
-- postgres-only:end
