-- KAI-140: rollback assigned_cameras.

DROP INDEX IF EXISTS idx_assigned_cameras_enabled;
DROP TABLE IF EXISTS assigned_cameras;
