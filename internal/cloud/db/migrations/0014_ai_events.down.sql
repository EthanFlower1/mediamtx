-- KAI-254 rollback: remove ai_events, camera_state, segment_index_stub.
DROP TABLE IF EXISTS segment_index_stub;
DROP TABLE IF EXISTS camera_state;
-- postgres-only:begin
DROP TABLE IF EXISTS ai_events;
-- postgres-only:end
