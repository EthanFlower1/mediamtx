-- KAI-283: Add lpr_enabled column to cameras table for per-camera LPR control.
-- Default false so existing cameras do not silently start running LPR.
--
-- Dependency note: this migration assumes a `cameras` table exists in the
-- cloud schema. That table is created in a forthcoming camera-management
-- migration (the per-tenant recorder ↔ camera registry, not yet numbered
-- at time of writing). If `cameras` does not exist, this ALTER TABLE is a
-- no-op (PostgreSQL IF EXISTS behaviour). In SQLite test mode the entire
-- statement is skipped to avoid schema errors in hermetic unit tests.
--
-- postgres-only:begin
DO $$
BEGIN
    IF EXISTS (
        SELECT 1 FROM information_schema.tables
        WHERE table_schema = 'public' AND table_name = 'cameras'
    ) THEN
        ALTER TABLE cameras ADD COLUMN IF NOT EXISTS lpr_enabled BOOLEAN NOT NULL DEFAULT FALSE;
    END IF;
END $$;
-- postgres-only:end
