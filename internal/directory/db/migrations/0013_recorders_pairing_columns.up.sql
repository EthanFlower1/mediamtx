-- KAI-244: Add pairing check-in columns to recorders table.
ALTER TABLE recorders ADD COLUMN os_release TEXT NOT NULL DEFAULT '';
ALTER TABLE recorders ADD COLUMN hardware_json TEXT NOT NULL DEFAULT '{}';
ALTER TABLE recorders ADD COLUMN token_id TEXT NOT NULL DEFAULT '';
ALTER TABLE recorders ADD COLUMN enrolled_at TEXT NOT NULL DEFAULT '';
