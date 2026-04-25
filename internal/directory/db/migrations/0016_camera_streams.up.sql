CREATE TABLE IF NOT EXISTS camera_streams (
    id TEXT PRIMARY KEY,
    camera_id TEXT NOT NULL,
    name TEXT NOT NULL,
    rtsp_url TEXT NOT NULL,
    profile_token TEXT NOT NULL DEFAULT '',
    video_codec TEXT NOT NULL DEFAULT '',
    audio_codec TEXT NOT NULL DEFAULT '',
    width INTEGER NOT NULL DEFAULT 0,
    height INTEGER NOT NULL DEFAULT 0,
    roles TEXT NOT NULL DEFAULT '',
    retention_days INTEGER NOT NULL DEFAULT 0,
    event_retention_days INTEGER NOT NULL DEFAULT 0,
    created_at TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%fZ', 'now'))
);
CREATE INDEX IF NOT EXISTS idx_camera_streams_camera ON camera_streams(camera_id);
