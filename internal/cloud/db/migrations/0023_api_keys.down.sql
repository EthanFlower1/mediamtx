-- KAI-400: rollback API key management tables
DROP TABLE IF EXISTS api_key_audit_log;
DROP TABLE IF EXISTS api_keys;
