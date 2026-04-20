CREATE TABLE IF NOT EXISTS recording_rules (
    id TEXT PRIMARY KEY,
    camera_id TEXT NOT NULL,
    name TEXT NOT NULL,
    mode TEXT NOT NULL CHECK(mode IN ('always', 'events')),
    days TEXT NOT NULL,
    start_time TEXT NOT NULL,
    end_time TEXT NOT NULL,
    post_event_seconds INTEGER NOT NULL DEFAULT 30,
    stream_id TEXT NOT NULL DEFAULT '',
    template_id TEXT DEFAULT '',
    enabled INTEGER NOT NULL DEFAULT 1,
    created_at TEXT NOT NULL,
    updated_at TEXT NOT NULL
);
CREATE INDEX IF NOT EXISTS idx_recording_rules_camera ON recording_rules(camera_id);

CREATE TABLE IF NOT EXISTS schedule_templates (
    id TEXT PRIMARY KEY,
    name TEXT NOT NULL,
    mode TEXT NOT NULL CHECK(mode IN ('always', 'events')),
    days TEXT NOT NULL,
    start_time TEXT NOT NULL,
    end_time TEXT NOT NULL,
    post_event_seconds INTEGER NOT NULL DEFAULT 30,
    is_default INTEGER NOT NULL DEFAULT 0,
    created_at TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%fZ', 'now'))
);
