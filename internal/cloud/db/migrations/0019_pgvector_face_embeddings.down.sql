-- KAI-292: rollback pgvector tables.
-- WARNING: dropping face_embeddings and clip_embeddings deletes ALL enrolled
-- embeddings and HNSW indexes across ALL tenants. This is irreversible in
-- production without a backup restore. Use with extreme caution.

DROP TABLE IF EXISTS clip_embeddings;
DROP TABLE IF EXISTS face_embeddings;
DROP TABLE IF EXISTS model_versions;
DROP TABLE IF EXISTS consent_records;

-- postgres-only:begin
-- Do NOT drop the vector extension — other migrations or manual tables may
-- depend on it. Extension removal is a DBA-level operation, not a migration.
-- postgres-only:end
