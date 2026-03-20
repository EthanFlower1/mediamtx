package db

var migrations = []struct {
	version int
	sql     string
}{
	{
		version: 1,
		sql: `
CREATE TABLE cameras (
	id TEXT PRIMARY KEY,
	name TEXT NOT NULL,
	onvif_endpoint TEXT,
	onvif_username TEXT,
	onvif_password TEXT,
	onvif_profile_token TEXT,
	rtsp_url TEXT,
	ptz_capable INTEGER NOT NULL DEFAULT 0,
	mediamtx_path TEXT UNIQUE,
	status TEXT NOT NULL DEFAULT 'disconnected',
	tags TEXT,
	created_at TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%fZ', 'now')),
	updated_at TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%fZ', 'now'))
);

CREATE TABLE recordings (
	id INTEGER PRIMARY KEY AUTOINCREMENT,
	camera_id TEXT NOT NULL,
	start_time TEXT NOT NULL,
	end_time TEXT,
	duration_ms INTEGER,
	file_path TEXT NOT NULL,
	file_size INTEGER,
	format TEXT NOT NULL DEFAULT 'fmp4',
	FOREIGN KEY (camera_id) REFERENCES cameras(id) ON DELETE CASCADE
);

CREATE INDEX idx_recordings_camera_time ON recordings (camera_id, start_time, end_time);

CREATE TABLE users (
	id TEXT PRIMARY KEY,
	username TEXT UNIQUE NOT NULL,
	password_hash TEXT NOT NULL,
	role TEXT NOT NULL DEFAULT 'viewer',
	camera_permissions TEXT,
	created_at TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%fZ', 'now')),
	updated_at TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%fZ', 'now'))
);

CREATE TABLE refresh_tokens (
	id TEXT PRIMARY KEY,
	user_id TEXT NOT NULL,
	token_hash TEXT NOT NULL,
	expires_at TEXT NOT NULL,
	revoked_at TEXT,
	FOREIGN KEY (user_id) REFERENCES users(id) ON DELETE CASCADE
);

CREATE INDEX idx_refresh_tokens_user_id ON refresh_tokens (user_id);

CREATE TABLE config (
	key TEXT PRIMARY KEY,
	value TEXT
);
`,
	},
	{
		version: 2,
		sql: `
CREATE TABLE recording_rules (
	id TEXT PRIMARY KEY,
	camera_id TEXT NOT NULL,
	name TEXT NOT NULL,
	mode TEXT NOT NULL CHECK(mode IN ('always', 'events')),
	days TEXT NOT NULL,
	start_time TEXT NOT NULL,
	end_time TEXT NOT NULL,
	post_event_seconds INTEGER NOT NULL DEFAULT 30,
	enabled INTEGER NOT NULL DEFAULT 1,
	created_at TEXT NOT NULL,
	updated_at TEXT NOT NULL,
	FOREIGN KEY (camera_id) REFERENCES cameras(id) ON DELETE CASCADE
);
CREATE INDEX idx_recording_rules_camera ON recording_rules(camera_id);
`,
	},
	{
		version: 3,
		sql:     `ALTER TABLE cameras ADD COLUMN retention_days INTEGER NOT NULL DEFAULT 0;`,
	},
}
