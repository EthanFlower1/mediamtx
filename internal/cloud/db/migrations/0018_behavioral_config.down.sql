-- KAI-429: reverse migration for 0015_behavioral_config.
DROP INDEX IF EXISTS idx_behavioral_config_enabled;
DROP TABLE IF EXISTS behavioral_config;
