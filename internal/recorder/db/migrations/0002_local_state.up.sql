-- KAI-140: local_state — single-row-per-key KV table for Recorder runtime state.
--
-- Expected keys (populated by various subsystems, not pre-seeded here):
--   recorder_id             — stable UUID assigned at pairing time (KAI-243)
--   last_directory_revision — monotonic revision from last successful Directory
--                             StreamAssignments reconcile (KAI-143)
--   last_checkin_at         — RFC3339 timestamp of last successful Directory checkin
--   mesh_bound_address      — local mesh-network bind address (KAI-259)
--   directory_url           — resolved Directory URL from discovery
--
-- Values are stored as TEXT; callers are responsible for encoding (RFC3339,
-- decimal integers, JSON as needed). This table intentionally has no schema
-- for the value — it is a scratchpad for small state blobs.

CREATE TABLE IF NOT EXISTS local_state (
    key        TEXT     NOT NULL PRIMARY KEY,
    value      TEXT     NOT NULL,
    updated_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP
);
