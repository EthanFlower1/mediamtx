package state

// migrations is the ordered list of schema migrations applied to the
// Recorder local cache database.
//
// Rules:
//
//   - Migrations are additive. Once a migration has shipped, it is frozen:
//     never edit a past migration in place. Instead append a new migration
//     at the tail.
//   - Each migration runs inside its own transaction.
//   - The version column is a monotonically increasing integer.
var migrations = []struct {
	version int
	sql     string
}{
	{
		version: 1,
		sql: `
CREATE TABLE assigned_cameras (
	camera_id           TEXT PRIMARY KEY,
	config              TEXT NOT NULL,        -- JSON blob (CameraConfig)
	config_version      INTEGER NOT NULL DEFAULT 0,
	rtsp_credentials    BLOB,                 -- ciphertext from cryptostore, nullable
	assigned_at         TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%fZ', 'now')),
	updated_at          TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%fZ', 'now')),
	last_state_push_at  TEXT
);

CREATE TABLE local_state (
	key   TEXT PRIMARY KEY,
	value TEXT NOT NULL                       -- JSON blob
);

CREATE TABLE segment_index (
	camera_id                 TEXT NOT NULL,
	start_ts                  TEXT NOT NULL,
	end_ts                    TEXT NOT NULL,
	path                      TEXT NOT NULL,
	size_bytes                INTEGER NOT NULL DEFAULT 0,
	uploaded_to_cloud_archive INTEGER NOT NULL DEFAULT 0,
	PRIMARY KEY (camera_id, start_ts)
);

CREATE INDEX idx_segment_index_camera_range
	ON segment_index (camera_id, start_ts, end_ts);

CREATE INDEX idx_segment_index_cloud_upload
	ON segment_index (uploaded_to_cloud_archive, camera_id, start_ts);
`,
	},
}
