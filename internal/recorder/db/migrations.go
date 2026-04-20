package db

type migration struct {
	version int
	sql     string
}

var migrations = []migration{
	{
		version: 1,
		sql: `
CREATE TABLE recordings (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    camera_id TEXT NOT NULL,
    stream_id TEXT DEFAULT '',
    start_time TEXT NOT NULL,
    end_time TEXT,
    duration_ms INTEGER,
    file_path TEXT NOT NULL,
    file_size INTEGER,
    format TEXT NOT NULL DEFAULT 'fmp4',
    init_size INTEGER NOT NULL DEFAULT 0,
    status TEXT NOT NULL DEFAULT 'unverified',
    status_detail TEXT,
    verified_at TEXT,
    media_start_time TEXT
);
CREATE INDEX idx_recordings_camera_time ON recordings (camera_id, start_time, end_time);
CREATE INDEX IF NOT EXISTS idx_recordings_camera_start ON recordings(camera_id, start_time);
CREATE INDEX IF NOT EXISTS idx_recordings_camera_end ON recordings(camera_id, end_time);
CREATE INDEX IF NOT EXISTS idx_recordings_stream ON recordings(stream_id);
CREATE INDEX idx_recordings_status ON recordings(status);

CREATE TABLE recording_fragments (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    recording_id INTEGER NOT NULL,
    fragment_index INTEGER NOT NULL,
    byte_offset INTEGER NOT NULL,
    size INTEGER NOT NULL,
    duration_ms REAL NOT NULL,
    is_keyframe INTEGER NOT NULL DEFAULT 1,
    timestamp_ms INTEGER NOT NULL DEFAULT 0,
    FOREIGN KEY (recording_id) REFERENCES recordings(id) ON DELETE CASCADE,
    UNIQUE(recording_id, fragment_index)
);
CREATE INDEX idx_fragments_recording ON recording_fragments(recording_id, fragment_index);

CREATE TABLE saved_clips (
    id TEXT PRIMARY KEY,
    camera_id TEXT NOT NULL,
    name TEXT NOT NULL,
    start_time TEXT NOT NULL,
    end_time TEXT NOT NULL,
    tags TEXT DEFAULT '',
    notes TEXT DEFAULT '',
    created_at TEXT NOT NULL
);
CREATE INDEX idx_saved_clips_camera ON saved_clips(camera_id);

CREATE TABLE bookmarks (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    camera_id TEXT NOT NULL,
    timestamp TEXT NOT NULL,
    label TEXT NOT NULL,
    notes TEXT NOT NULL DEFAULT '',
    created_by TEXT,
    created_at TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%fZ', 'now'))
);
CREATE INDEX idx_bookmarks_camera_time ON bookmarks(camera_id, timestamp);
CREATE INDEX idx_bookmarks_timestamp ON bookmarks(timestamp);

CREATE TABLE motion_events (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    camera_id TEXT NOT NULL,
    started_at TEXT NOT NULL,
    ended_at TEXT,
    thumbnail_path TEXT DEFAULT '',
    event_type TEXT NOT NULL DEFAULT 'motion',
    object_class TEXT DEFAULT '',
    confidence REAL DEFAULT 0,
    embedding BLOB,
    description TEXT DEFAULT '',
    detection_summary TEXT DEFAULT '',
    metadata TEXT
);
CREATE INDEX idx_motion_events_camera_time ON motion_events(camera_id, started_at);
CREATE INDEX IF NOT EXISTS idx_motion_events_object_class ON motion_events(camera_id, object_class);

CREATE TABLE detections (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    motion_event_id INTEGER NOT NULL,
    frame_time TEXT NOT NULL,
    class TEXT NOT NULL,
    confidence REAL NOT NULL,
    box_x REAL NOT NULL,
    box_y REAL NOT NULL,
    box_w REAL NOT NULL,
    box_h REAL NOT NULL,
    embedding BLOB,
    attributes TEXT DEFAULT '',
    FOREIGN KEY (motion_event_id) REFERENCES motion_events(id) ON DELETE CASCADE
);
CREATE INDEX idx_detections_event ON detections(motion_event_id);
CREATE INDEX idx_detections_class ON detections(class);

CREATE TABLE IF NOT EXISTS detection_zones (
    id TEXT PRIMARY KEY,
    camera_id TEXT NOT NULL,
    name TEXT NOT NULL DEFAULT '',
    points TEXT NOT NULL DEFAULT '[]',
    class_filter TEXT NOT NULL DEFAULT '[]',
    enabled INTEGER NOT NULL DEFAULT 1,
    created_at TEXT NOT NULL DEFAULT '',
    updated_at TEXT NOT NULL DEFAULT ''
);

CREATE TABLE IF NOT EXISTS detection_events (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    camera_id TEXT NOT NULL,
    zone_id TEXT NOT NULL DEFAULT '',
    class TEXT NOT NULL DEFAULT '',
    start_time TEXT NOT NULL DEFAULT '',
    end_time TEXT NOT NULL DEFAULT '',
    peak_confidence REAL NOT NULL DEFAULT 0,
    thumbnail_path TEXT NOT NULL DEFAULT '',
    detection_count INTEGER NOT NULL DEFAULT 0
);

CREATE TABLE IF NOT EXISTS detection_schedules (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    camera_id TEXT NOT NULL,
    day_of_week INTEGER NOT NULL,
    start_time TEXT NOT NULL DEFAULT '00:00',
    end_time TEXT NOT NULL DEFAULT '23:59',
    enabled INTEGER NOT NULL DEFAULT 1
);
CREATE INDEX IF NOT EXISTS idx_detection_schedules_camera ON detection_schedules(camera_id);

CREATE TABLE screenshots (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    camera_id TEXT NOT NULL,
    file_path TEXT NOT NULL,
    file_size INTEGER NOT NULL DEFAULT 0,
    created_at TEXT NOT NULL
);
CREATE INDEX idx_screenshots_camera ON screenshots(camera_id);
CREATE INDEX idx_screenshots_created ON screenshots(created_at);

CREATE TABLE IF NOT EXISTS storage_quotas (
    id TEXT PRIMARY KEY,
    name TEXT NOT NULL,
    quota_bytes INTEGER NOT NULL,
    warning_percent INTEGER NOT NULL DEFAULT 80,
    critical_percent INTEGER NOT NULL DEFAULT 90,
    enabled INTEGER NOT NULL DEFAULT 1,
    created_at TEXT NOT NULL,
    updated_at TEXT NOT NULL
);

CREATE TABLE connection_events (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    camera_id TEXT NOT NULL,
    state TEXT NOT NULL,
    error_message TEXT,
    created_at TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%fZ', 'now'))
);
CREATE INDEX idx_connection_events_camera ON connection_events(camera_id, created_at);

CREATE TABLE queued_commands (
    id TEXT PRIMARY KEY,
    camera_id TEXT NOT NULL,
    command_type TEXT NOT NULL,
    payload TEXT NOT NULL,
    status TEXT NOT NULL DEFAULT 'pending',
    error_message TEXT,
    queued_at TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%fZ', 'now')),
    executed_at TEXT
);
CREATE INDEX idx_queued_commands_camera ON queued_commands(camera_id, status);

CREATE TABLE pending_syncs (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    recording_id INTEGER NOT NULL,
    camera_id TEXT NOT NULL,
    local_path TEXT NOT NULL,
    target_path TEXT NOT NULL,
    status TEXT NOT NULL DEFAULT 'pending',
    attempts INTEGER NOT NULL DEFAULT 0,
    error_message TEXT,
    created_at TEXT NOT NULL,
    last_attempt_at TEXT,
    FOREIGN KEY (recording_id) REFERENCES recordings(id) ON DELETE CASCADE
);
CREATE INDEX idx_pending_syncs_status ON pending_syncs(status);
CREATE INDEX idx_pending_syncs_camera ON pending_syncs(camera_id);

CREATE TABLE export_jobs (
    id TEXT PRIMARY KEY,
    camera_id TEXT NOT NULL,
    start_time TEXT NOT NULL,
    end_time TEXT NOT NULL,
    status TEXT NOT NULL DEFAULT 'pending',
    progress REAL NOT NULL DEFAULT 0,
    output_path TEXT NOT NULL DEFAULT '',
    error TEXT NOT NULL DEFAULT '',
    created_at TEXT NOT NULL,
    completed_at TEXT NOT NULL DEFAULT ''
);
CREATE INDEX idx_export_jobs_camera ON export_jobs(camera_id);
CREATE INDEX idx_export_jobs_status ON export_jobs(status);

CREATE TABLE evidence_exports (
    id TEXT PRIMARY KEY,
    camera_id TEXT NOT NULL,
    camera_name TEXT NOT NULL DEFAULT '',
    start_time TEXT NOT NULL,
    end_time TEXT NOT NULL,
    exported_by TEXT NOT NULL DEFAULT '',
    exported_at TEXT NOT NULL,
    sha256_hash TEXT NOT NULL DEFAULT '',
    zip_path TEXT NOT NULL DEFAULT '',
    notes TEXT NOT NULL DEFAULT ''
);
CREATE INDEX idx_evidence_exports_camera ON evidence_exports(camera_id);
CREATE INDEX idx_evidence_exports_time ON evidence_exports(exported_at);

CREATE TABLE IF NOT EXISTS bulk_export_jobs (
    id TEXT PRIMARY KEY,
    status TEXT NOT NULL DEFAULT 'pending',
    zip_path TEXT NOT NULL DEFAULT '',
    error TEXT NOT NULL DEFAULT '',
    created_at TEXT NOT NULL,
    completed_at TEXT NOT NULL DEFAULT ''
);
CREATE TABLE IF NOT EXISTS bulk_export_items (
    id TEXT PRIMARY KEY,
    job_id TEXT NOT NULL,
    camera_id TEXT NOT NULL,
    camera_name TEXT NOT NULL DEFAULT '',
    start_time TEXT NOT NULL,
    end_time TEXT NOT NULL,
    status TEXT NOT NULL DEFAULT 'pending',
    file_count INTEGER NOT NULL DEFAULT 0,
    total_bytes INTEGER NOT NULL DEFAULT 0,
    error TEXT,
    FOREIGN KEY (job_id) REFERENCES bulk_export_jobs(id) ON DELETE CASCADE
);
CREATE INDEX IF NOT EXISTS idx_bulk_export_items_job ON bulk_export_items(job_id);

CREATE TABLE IF NOT EXISTS cross_camera_tracks (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    label TEXT NOT NULL DEFAULT '',
    status TEXT NOT NULL DEFAULT 'active',
    created_at TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%fZ', 'now')),
    updated_at TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%fZ', 'now')),
    detection_id INTEGER NOT NULL DEFAULT 0
);
CREATE TABLE IF NOT EXISTS cross_camera_sightings (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    track_id INTEGER NOT NULL,
    camera_id TEXT NOT NULL,
    timestamp TEXT NOT NULL,
    end_time TEXT NOT NULL DEFAULT '',
    confidence REAL NOT NULL DEFAULT 0.0,
    thumbnail TEXT NOT NULL DEFAULT '',
    FOREIGN KEY (track_id) REFERENCES cross_camera_tracks(id) ON DELETE CASCADE
);
CREATE INDEX idx_sightings_track ON cross_camera_sightings (track_id);
CREATE INDEX idx_sightings_camera ON cross_camera_sightings (camera_id);
CREATE INDEX idx_tracks_detection ON cross_camera_tracks (detection_id);

CREATE TABLE IF NOT EXISTS tours (
    id TEXT PRIMARY KEY,
    name TEXT NOT NULL,
    camera_ids TEXT NOT NULL DEFAULT '[]',
    dwell_seconds INTEGER NOT NULL DEFAULT 10,
    created_at TEXT NOT NULL DEFAULT (datetime('now')),
    updated_at TEXT NOT NULL DEFAULT (datetime('now'))
);

CREATE TABLE IF NOT EXISTS config (
    key TEXT PRIMARY KEY,
    value TEXT NOT NULL
);
`,
	},
}
