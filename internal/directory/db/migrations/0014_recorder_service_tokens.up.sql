-- Add service token columns to recorders for managed-mode bearer auth.
ALTER TABLE recorders ADD COLUMN service_token_hash BLOB;
ALTER TABLE recorders ADD COLUMN service_token_salt BLOB;
ALTER TABLE recorders ADD COLUMN health_status TEXT NOT NULL DEFAULT 'unknown';
ALTER TABLE recorders ADD COLUMN internal_api_addr TEXT NOT NULL DEFAULT '';
