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
	{
		version: 4,
		sql: `
CREATE TABLE audit_log (
	id INTEGER PRIMARY KEY AUTOINCREMENT,
	user_id TEXT,
	username TEXT NOT NULL,
	action TEXT NOT NULL,
	resource_type TEXT NOT NULL,
	resource_id TEXT,
	details TEXT,
	ip_address TEXT,
	created_at TEXT NOT NULL
);
CREATE INDEX idx_audit_log_created ON audit_log(created_at);
CREATE INDEX idx_audit_log_user ON audit_log(user_id);
`,
	},
	{
		version: 5,
		sql: `
CREATE TABLE motion_events (
	id INTEGER PRIMARY KEY AUTOINCREMENT,
	camera_id TEXT NOT NULL,
	started_at TEXT NOT NULL,
	ended_at TEXT,
	FOREIGN KEY (camera_id) REFERENCES cameras(id) ON DELETE CASCADE
);
CREATE INDEX idx_motion_events_camera_time ON motion_events(camera_id, started_at);
`,
	},
	{
		version: 6,
		sql: `
CREATE INDEX IF NOT EXISTS idx_recordings_camera_start ON recordings(camera_id, start_time);
CREATE INDEX IF NOT EXISTS idx_recordings_camera_end ON recordings(camera_id, end_time);
`,
	},
	{
		version: 7,
		sql: `
CREATE TABLE saved_clips (
	id TEXT PRIMARY KEY,
	camera_id TEXT NOT NULL,
	name TEXT NOT NULL,
	start_time TEXT NOT NULL,
	end_time TEXT NOT NULL,
	tags TEXT DEFAULT '',
	notes TEXT DEFAULT '',
	created_at TEXT NOT NULL,
	FOREIGN KEY (camera_id) REFERENCES cameras(id) ON DELETE CASCADE
);
CREATE INDEX idx_saved_clips_camera ON saved_clips(camera_id);
`,
	},
	{
		version: 8,
		sql: `
ALTER TABLE motion_events ADD COLUMN thumbnail_path TEXT DEFAULT '';
`,
	},
	{
		version: 9,
		sql: `
ALTER TABLE cameras ADD COLUMN supports_ptz INTEGER NOT NULL DEFAULT 0;
ALTER TABLE cameras ADD COLUMN supports_imaging INTEGER NOT NULL DEFAULT 0;
ALTER TABLE cameras ADD COLUMN supports_events INTEGER NOT NULL DEFAULT 0;
ALTER TABLE cameras ADD COLUMN supports_relay INTEGER NOT NULL DEFAULT 0;
ALTER TABLE cameras ADD COLUMN supports_audio_backchannel INTEGER NOT NULL DEFAULT 0;
ALTER TABLE cameras ADD COLUMN snapshot_uri TEXT DEFAULT '';
`,
	},
	{
		version: 10,
		sql: `
ALTER TABLE cameras ADD COLUMN supports_media2 INTEGER NOT NULL DEFAULT 0;
ALTER TABLE cameras ADD COLUMN supports_analytics INTEGER NOT NULL DEFAULT 0;
ALTER TABLE motion_events ADD COLUMN event_type TEXT NOT NULL DEFAULT 'motion';
`,
	},
	{
		version: 11,
		sql:     `ALTER TABLE cameras ADD COLUMN supports_edge_recording INTEGER NOT NULL DEFAULT 0;`,
	},
	{
		version: 12,
		sql: `
ALTER TABLE motion_events ADD COLUMN object_class TEXT DEFAULT '';
ALTER TABLE motion_events ADD COLUMN confidence REAL DEFAULT 0;
CREATE INDEX IF NOT EXISTS idx_motion_events_object_class ON motion_events(camera_id, object_class);
`,
	},
	{
		version: 13,
		sql:     `ALTER TABLE cameras ADD COLUMN motion_timeout_seconds INTEGER NOT NULL DEFAULT 8;`,
	},
	{
		version: 14,
		sql: `
ALTER TABLE cameras ADD COLUMN sub_stream_url TEXT DEFAULT '';
ALTER TABLE cameras ADD COLUMN ai_enabled INTEGER NOT NULL DEFAULT 0;

ALTER TABLE motion_events ADD COLUMN embedding BLOB;
ALTER TABLE motion_events ADD COLUMN description TEXT DEFAULT '';

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
`,
	},
	{
		version: 15,
		sql:     `ALTER TABLE cameras ADD COLUMN audio_transcode INTEGER NOT NULL DEFAULT 0;`,
	},
	{
		version: 16,
		sql: `
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
ALTER TABLE recordings ADD COLUMN init_size INTEGER NOT NULL DEFAULT 0;
`,
	},
	{
		version: 17,
		sql: `
CREATE TABLE bookmarks (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    camera_id TEXT NOT NULL,
    timestamp TEXT NOT NULL,
    label TEXT NOT NULL,
    created_by TEXT,
    created_at TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%fZ', 'now')),
    FOREIGN KEY (camera_id) REFERENCES cameras(id) ON DELETE CASCADE
);
CREATE INDEX idx_bookmarks_camera_time ON bookmarks(camera_id, timestamp);
CREATE INDEX idx_bookmarks_timestamp ON bookmarks(timestamp);
`,
	},
	{
		version: 18,
		sql: `
ALTER TABLE cameras ADD COLUMN storage_path TEXT NOT NULL DEFAULT '';
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
    FOREIGN KEY (recording_id) REFERENCES recordings(id) ON DELETE CASCADE,
    FOREIGN KEY (camera_id) REFERENCES cameras(id) ON DELETE CASCADE
);
CREATE INDEX idx_pending_syncs_status ON pending_syncs(status);
CREATE INDEX idx_pending_syncs_camera ON pending_syncs(camera_id);
`,
	},
	{
		version: 19,
		sql: `
        CREATE TABLE IF NOT EXISTS camera_groups (
            id TEXT PRIMARY KEY,
            name TEXT NOT NULL,
            created_at TEXT NOT NULL DEFAULT (datetime('now')),
            updated_at TEXT NOT NULL DEFAULT (datetime('now'))
        );

        CREATE TABLE IF NOT EXISTS camera_group_members (
            group_id TEXT NOT NULL REFERENCES camera_groups(id) ON DELETE CASCADE,
            camera_id TEXT NOT NULL REFERENCES cameras(id) ON DELETE CASCADE,
            sort_order INTEGER NOT NULL DEFAULT 0,
            PRIMARY KEY (group_id, camera_id)
        );

        CREATE INDEX IF NOT EXISTS idx_group_members_group ON camera_group_members(group_id);
        CREATE INDEX IF NOT EXISTS idx_group_members_camera ON camera_group_members(camera_id);
    `,
	},
	{
		version: 20,
		sql: `
        CREATE TABLE IF NOT EXISTS tours (
            id TEXT PRIMARY KEY,
            name TEXT NOT NULL,
            camera_ids TEXT NOT NULL DEFAULT '[]',
            dwell_seconds INTEGER NOT NULL DEFAULT 10,
            created_at TEXT NOT NULL DEFAULT (datetime('now')),
            updated_at TEXT NOT NULL DEFAULT (datetime('now'))
        );
    `,
	},
	{
		version: 21,
		sql: `
CREATE TABLE camera_streams (
    id TEXT PRIMARY KEY,
    camera_id TEXT NOT NULL,
    name TEXT NOT NULL,
    rtsp_url TEXT NOT NULL,
    profile_token TEXT NOT NULL DEFAULT '',
    video_codec TEXT NOT NULL DEFAULT '',
    width INTEGER NOT NULL DEFAULT 0,
    height INTEGER NOT NULL DEFAULT 0,
    roles TEXT NOT NULL DEFAULT '',
    created_at TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%fZ', 'now')),
    FOREIGN KEY (camera_id) REFERENCES cameras(id) ON DELETE CASCADE
);
CREATE INDEX idx_camera_streams_camera ON camera_streams(camera_id);

ALTER TABLE recording_rules ADD COLUMN stream_id TEXT NOT NULL DEFAULT '';

INSERT INTO camera_streams (id, camera_id, name, rtsp_url, profile_token, roles)
SELECT
    lower(hex(randomblob(4))) || '-' || lower(hex(randomblob(2))) || '-' || lower(hex(randomblob(2))) || '-' || lower(hex(randomblob(2))) || '-' || lower(hex(randomblob(6))),
    id,
    'Main Stream',
    rtsp_url,
    COALESCE(onvif_profile_token, ''),
    CASE WHEN sub_stream_url IS NOT NULL AND sub_stream_url != '' THEN 'live_view' ELSE 'live_view,recording,ai_detection,mobile' END
FROM cameras
WHERE rtsp_url IS NOT NULL AND rtsp_url != '';

INSERT INTO camera_streams (id, camera_id, name, rtsp_url, profile_token, roles)
SELECT
    lower(hex(randomblob(4))) || '-' || lower(hex(randomblob(2))) || '-' || lower(hex(randomblob(2))) || '-' || lower(hex(randomblob(2))) || '-' || lower(hex(randomblob(6))),
    id,
    'Sub Stream',
    sub_stream_url,
    '',
    'recording,ai_detection,mobile'
FROM cameras
WHERE sub_stream_url IS NOT NULL AND sub_stream_url != '';
`,
	},
	// Migration 22: Add AI pipeline stream selection and track timeout.
	{
		version: 22,
		sql: `
        ALTER TABLE cameras ADD COLUMN ai_stream_id TEXT DEFAULT '';
        ALTER TABLE cameras ADD COLUMN ai_track_timeout INTEGER DEFAULT 5;
        ALTER TABLE cameras ADD COLUMN ai_confidence REAL DEFAULT 0.5;
    `,
	},
	// Migration 23: Add recording stream selection.
	{
		version: 23,
		sql: `
        ALTER TABLE cameras ADD COLUMN recording_stream_id TEXT DEFAULT '';
    `,
	},
	// Migration 24: Schedule templates and template_id on recording rules.
	{
		version: 24,
		sql: `
        CREATE TABLE schedule_templates (
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
        ALTER TABLE recording_rules ADD COLUMN template_id TEXT DEFAULT '';
    `,
	},
	// Migration 25: Screenshots table.
	{
		version: 25,
		sql: `
        CREATE TABLE screenshots (
            id INTEGER PRIMARY KEY AUTOINCREMENT,
            camera_id TEXT NOT NULL,
            file_path TEXT NOT NULL,
            file_size INTEGER NOT NULL DEFAULT 0,
            created_at TEXT NOT NULL,
            FOREIGN KEY (camera_id) REFERENCES cameras(id) ON DELETE CASCADE
        );
        CREATE INDEX idx_screenshots_camera ON screenshots(camera_id);
        CREATE INDEX idx_screenshots_created ON screenshots(created_at);
    `,
	},
	// Migration 26: Add audio_codec column to camera_streams.
	{
		version: 26,
		sql:     `ALTER TABLE camera_streams ADD COLUMN audio_codec TEXT NOT NULL DEFAULT '';`,
	},
	// Migration 27: Event-aware retention and detection consolidation.
	{
		version: 27,
		sql: `
		ALTER TABLE cameras ADD COLUMN event_retention_days INTEGER NOT NULL DEFAULT 0;
		ALTER TABLE cameras ADD COLUMN detection_retention_days INTEGER NOT NULL DEFAULT 0;
		ALTER TABLE motion_events ADD COLUMN detection_summary TEXT DEFAULT '';
		`,
	},
	// Migration 28: Per-stream retention and recording-to-stream association.
	{
		version: 28,
		sql: `
		ALTER TABLE recordings ADD COLUMN stream_id TEXT DEFAULT '';
		CREATE INDEX IF NOT EXISTS idx_recordings_stream ON recordings(stream_id);
		ALTER TABLE camera_streams ADD COLUMN retention_days INTEGER NOT NULL DEFAULT 0;
		ALTER TABLE camera_streams ADD COLUMN event_retention_days INTEGER NOT NULL DEFAULT 0;
		`,
	},
	// Migration 29: Recording integrity verification status.
	{
		version: 29,
		sql: `
		ALTER TABLE recordings ADD COLUMN status TEXT NOT NULL DEFAULT 'unverified';
		ALTER TABLE recordings ADD COLUMN status_detail TEXT;
		ALTER TABLE recordings ADD COLUMN verified_at TEXT;
		CREATE INDEX idx_recordings_status ON recordings(status);
		`,
	},
	// Migration 30: Storage quota management (KAI-8).
	{
		version: 30,
		sql: `
		ALTER TABLE cameras ADD COLUMN quota_bytes INTEGER NOT NULL DEFAULT 0;
		ALTER TABLE cameras ADD COLUMN quota_warning_percent INTEGER NOT NULL DEFAULT 80;
		ALTER TABLE cameras ADD COLUMN quota_critical_percent INTEGER NOT NULL DEFAULT 90;
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
		`,
	},
	// Migration 31: Service capabilities cache (KAI-112).
	{
		version: 31,
		sql:     `ALTER TABLE cameras ADD COLUMN service_capabilities TEXT DEFAULT '';`,
	},
	// Migration 32: Connection resilience — event history and command queue (KAI-24).
	{
		version: 32,
		sql: `
		CREATE TABLE connection_events (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			camera_id TEXT NOT NULL,
			state TEXT NOT NULL,
			error_message TEXT,
			created_at TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%fZ', 'now')),
			FOREIGN KEY (camera_id) REFERENCES cameras(id) ON DELETE CASCADE
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
			executed_at TEXT,
			FOREIGN KEY (camera_id) REFERENCES cameras(id) ON DELETE CASCADE
		);
		CREATE INDEX idx_queued_commands_camera ON queued_commands(camera_id, status);
		`,
	},
	// Migration 33: Supported event topics from GetEventProperties (KAI-110).
	{
		version: 33,
		sql:     `ALTER TABLE cameras ADD COLUMN supported_event_topics TEXT DEFAULT '';`,
	},
	// Migration 34: Add metadata column for analytics event details (KAI-20).
	{
		version: 34,
		sql:     `ALTER TABLE motion_events ADD COLUMN metadata TEXT;`,
	},
	// Migration 35: Multi-channel camera support (KAI-26).
	{
		version: 35,
		sql: `
		CREATE TABLE IF NOT EXISTS devices (
			id TEXT PRIMARY KEY,
			name TEXT NOT NULL,
			manufacturer TEXT NOT NULL DEFAULT '',
			model TEXT NOT NULL DEFAULT '',
			firmware_version TEXT NOT NULL DEFAULT '',
			onvif_endpoint TEXT NOT NULL DEFAULT '',
			onvif_username TEXT NOT NULL DEFAULT '',
			onvif_password TEXT NOT NULL DEFAULT '',
			channel_count INTEGER NOT NULL DEFAULT 1,
			created_at TEXT NOT NULL,
			updated_at TEXT NOT NULL
		);
		ALTER TABLE cameras ADD COLUMN device_id TEXT DEFAULT NULL REFERENCES devices(id);
		ALTER TABLE cameras ADD COLUMN channel_index INTEGER DEFAULT NULL;
		CREATE INDEX IF NOT EXISTS idx_cameras_device ON cameras(device_id);
		`,
	},
	// Migration 36: Multicast streaming configuration (KAI-21).
	{
		version: 36,
		sql: `
		ALTER TABLE cameras ADD COLUMN multicast_enabled INTEGER NOT NULL DEFAULT 0;
		ALTER TABLE cameras ADD COLUMN multicast_address TEXT NOT NULL DEFAULT '';
		ALTER TABLE cameras ADD COLUMN multicast_port INTEGER NOT NULL DEFAULT 0;
		ALTER TABLE cameras ADD COLUMN multicast_ttl INTEGER NOT NULL DEFAULT 5;
		`,
	},
	// Migration 37: Export jobs queue (KAI-33).
	{
		version: 37,
		sql: `
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
			completed_at TEXT NOT NULL DEFAULT '',
			FOREIGN KEY (camera_id) REFERENCES cameras(id) ON DELETE CASCADE
		);
		CREATE INDEX idx_export_jobs_camera ON export_jobs(camera_id);
		CREATE INDEX idx_export_jobs_status ON export_jobs(status);
		`,
	},
	// Migration 38: Evidence export tracking (KAI-38).
	{
		version: 38,
		sql: `
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
			notes TEXT NOT NULL DEFAULT '',
			FOREIGN KEY (camera_id) REFERENCES cameras(id) ON DELETE CASCADE
		);
		CREATE INDEX idx_evidence_exports_camera ON evidence_exports(camera_id);
		CREATE INDEX idx_evidence_exports_time ON evidence_exports(exported_at);
		`,
	},
	// Migration 39: Add notes column to bookmarks (KAI-35).
	{
		version: 39,
		sql:     `ALTER TABLE bookmarks ADD COLUMN notes TEXT NOT NULL DEFAULT '';`,
	},
	// Migration 40: System update history (KAI-80).
	{
		version: 40,
		sql: `
		CREATE TABLE IF NOT EXISTS update_history (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			from_version TEXT NOT NULL,
			to_version TEXT NOT NULL,
			status TEXT NOT NULL DEFAULT 'pending',
			started_at TEXT NOT NULL,
			completed_at TEXT,
			error_message TEXT,
			initiated_by TEXT NOT NULL DEFAULT '',
			sha256_checksum TEXT NOT NULL DEFAULT '',
			rollback_available INTEGER NOT NULL DEFAULT 0
		);
		CREATE INDEX IF NOT EXISTS idx_update_history_status ON update_history(status);
		CREATE INDEX IF NOT EXISTS idx_update_history_started ON update_history(started_at);
		`,
	},
	// Migration 41: Bulk export jobs and items (KAI-81).
	{
		version: 41,
		sql: `
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
		`,
	},
	// Migration 42: System alerts and SMTP configuration (KAI-83).
	{
		version: 42,
		sql: `
		CREATE TABLE smtp_config (
			id INTEGER PRIMARY KEY CHECK (id = 1),
			host TEXT NOT NULL DEFAULT '',
			port INTEGER NOT NULL DEFAULT 587,
			username TEXT NOT NULL DEFAULT '',
			password TEXT NOT NULL DEFAULT '',
			from_address TEXT NOT NULL DEFAULT '',
			tls_enabled INTEGER NOT NULL DEFAULT 1,
			updated_at TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%fZ', 'now'))
		);
		INSERT INTO smtp_config (id) VALUES (1);

		CREATE TABLE alert_rules (
			id TEXT PRIMARY KEY,
			name TEXT NOT NULL,
			rule_type TEXT NOT NULL CHECK(rule_type IN ('disk_usage', 'camera_offline', 'recording_gap')),
			threshold_value REAL NOT NULL,
			camera_id TEXT DEFAULT '',
			enabled INTEGER NOT NULL DEFAULT 1,
			notify_email INTEGER NOT NULL DEFAULT 1,
			cooldown_minutes INTEGER NOT NULL DEFAULT 60,
			created_at TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%fZ', 'now')),
			updated_at TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%fZ', 'now'))
		);

		CREATE TABLE alerts (
			id TEXT PRIMARY KEY,
			rule_id TEXT NOT NULL,
			rule_type TEXT NOT NULL,
			severity TEXT NOT NULL DEFAULT 'warning',
			message TEXT NOT NULL,
			details TEXT DEFAULT '',
			acknowledged INTEGER NOT NULL DEFAULT 0,
			acknowledged_by TEXT DEFAULT '',
			acknowledged_at TEXT DEFAULT '',
			email_sent INTEGER NOT NULL DEFAULT 0,
			email_error TEXT DEFAULT '',
			created_at TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%fZ', 'now')),
			FOREIGN KEY (rule_id) REFERENCES alert_rules(id) ON DELETE CASCADE
		);
		CREATE INDEX idx_alerts_rule ON alerts(rule_id);
		CREATE INDEX idx_alerts_created ON alerts(created_at);
		CREATE INDEX idx_alerts_acknowledged ON alerts(acknowledged);
		`,
	},
	// Migration 43: Brute-force protection - account lockout (KAI-77).
	{
		version: 43,
		sql: `
		ALTER TABLE users ADD COLUMN locked_until TEXT;
		ALTER TABLE users ADD COLUMN failed_login_attempts INTEGER NOT NULL DEFAULT 0;
		`,
	},
	// Migration 44: Role-based access control (KAI-75).
	{
		version: 44,
		sql: `
		CREATE TABLE roles (
			id TEXT PRIMARY KEY,
			name TEXT UNIQUE NOT NULL,
			description TEXT NOT NULL DEFAULT '',
			permissions TEXT NOT NULL DEFAULT '[]',
			is_system INTEGER NOT NULL DEFAULT 0,
			created_at TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%fZ', 'now')),
			updated_at TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%fZ', 'now'))
		);

		CREATE TABLE user_camera_permissions (
			id TEXT PRIMARY KEY,
			user_id TEXT NOT NULL,
			camera_id TEXT NOT NULL,
			permissions TEXT NOT NULL DEFAULT '[]',
			FOREIGN KEY (user_id) REFERENCES users(id) ON DELETE CASCADE,
			FOREIGN KEY (camera_id) REFERENCES cameras(id) ON DELETE CASCADE,
			UNIQUE(user_id, camera_id)
		);
		CREATE INDEX idx_user_camera_perms_user ON user_camera_permissions(user_id);
		CREATE INDEX idx_user_camera_perms_camera ON user_camera_permissions(camera_id);

		ALTER TABLE users ADD COLUMN role_id TEXT NOT NULL DEFAULT '';

		INSERT INTO roles (id, name, description, permissions, is_system) VALUES
			('role-admin', 'admin', 'Full system access', '["view_live","view_playback","export","ptz_control","admin"]', 1),
			('role-operator', 'operator', 'View, playback, export, and PTZ control', '["view_live","view_playback","export","ptz_control"]', 1),
			('role-viewer', 'viewer', 'View live and playback only', '["view_live","view_playback"]', 1);
		`,
	},
	// Migration 45: Per-class confidence thresholds for AI detections (KAI-43).
	{
		version: 45,
		sql: `
		ALTER TABLE cameras ADD COLUMN confidence_thresholds TEXT NOT NULL DEFAULT '';
		`,
	},
	// Migration 46: Session tracking columns on refresh_tokens (KAI-76).
	{
		version: 46,
		sql: `
		ALTER TABLE refresh_tokens ADD COLUMN ip_address TEXT NOT NULL DEFAULT '';
		ALTER TABLE refresh_tokens ADD COLUMN user_agent TEXT NOT NULL DEFAULT '';
		ALTER TABLE refresh_tokens ADD COLUMN device_name TEXT NOT NULL DEFAULT '';
		ALTER TABLE refresh_tokens ADD COLUMN last_activity TEXT NOT NULL DEFAULT '';
		ALTER TABLE refresh_tokens ADD COLUMN created_at TEXT NOT NULL DEFAULT '';
		`,
	},
	// Migration 47: Create missing tables for detection zones, detection events,
	// webhook configs, and webhook deliveries. These tables were referenced by
	// the DB layer and API handlers but never created in earlier migrations.
	{
		version: 47,
		sql: `
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

		CREATE TABLE IF NOT EXISTS webhook_configs (
			id TEXT PRIMARY KEY,
			name TEXT NOT NULL DEFAULT '',
			url TEXT NOT NULL DEFAULT '',
			secret TEXT NOT NULL DEFAULT '',
			camera_id TEXT NOT NULL DEFAULT '',
			event_types TEXT NOT NULL DEFAULT 'detection',
			object_classes TEXT NOT NULL DEFAULT '',
			min_confidence REAL NOT NULL DEFAULT 0,
			enabled INTEGER NOT NULL DEFAULT 1,
			max_retries INTEGER NOT NULL DEFAULT 3,
			timeout_seconds INTEGER NOT NULL DEFAULT 10,
			created_at TEXT NOT NULL DEFAULT '',
			updated_at TEXT NOT NULL DEFAULT ''
		);

		CREATE TABLE IF NOT EXISTS webhook_deliveries (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			webhook_id TEXT NOT NULL DEFAULT '',
			event_type TEXT NOT NULL DEFAULT '',
			payload TEXT NOT NULL DEFAULT '',
			response_status INTEGER NOT NULL DEFAULT 0,
			response_body TEXT NOT NULL DEFAULT '',
			error TEXT NOT NULL DEFAULT '',
			attempt INTEGER NOT NULL DEFAULT 0,
			status TEXT NOT NULL DEFAULT 'pending',
			created_at TEXT NOT NULL DEFAULT '',
			completed_at TEXT NOT NULL DEFAULT '',
			next_retry_at TEXT NOT NULL DEFAULT ''
		);

		CREATE TABLE IF NOT EXISTS detection_schedules (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			camera_id TEXT NOT NULL,
			day_of_week INTEGER NOT NULL,
			start_time TEXT NOT NULL DEFAULT '00:00',
			end_time TEXT NOT NULL DEFAULT '23:59',
			enabled INTEGER NOT NULL DEFAULT 1,
			FOREIGN KEY (camera_id) REFERENCES cameras(id) ON DELETE CASCADE
		);
		CREATE INDEX IF NOT EXISTS idx_detection_schedules_camera ON detection_schedules(camera_id);
		`,
	},
	// Migration 48: Create upgrade_migrations table for tracking application upgrades
	// and rollback support. Referenced by upgrade_migrations.go but never created.
	{
		version: 48,
		sql: `
		CREATE TABLE IF NOT EXISTS upgrade_migrations (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			from_version TEXT NOT NULL DEFAULT '',
			to_version TEXT NOT NULL DEFAULT '',
			status TEXT NOT NULL DEFAULT 'pending',
			config_backup_path TEXT NOT NULL DEFAULT '',
			db_backup_path TEXT NOT NULL DEFAULT '',
			started_at TEXT NOT NULL DEFAULT '',
			completed_at TEXT NOT NULL DEFAULT '',
			error_message TEXT NOT NULL DEFAULT '',
			rollback_completed_at TEXT NOT NULL DEFAULT ''
		);
		`,
	},
	// Migration 49: Cache ONVIF device information (manufacturer, model,
	// firmware, serial, hardware id) so the camera detail screen does not
	// have to issue live SOAP requests on every page load.
	{
		version: 49,
		sql:     `ALTER TABLE cameras ADD COLUMN device_info TEXT NOT NULL DEFAULT '';`,
	},
	// Migration 50 (KAI-319): API keys management for integrator portal.
	// Stores hashed keys with scope, customer scope, rotation grace period,
	// and per-key audit trail.
	{
		version: 50,
		sql: `
		CREATE TABLE api_keys (
			id TEXT PRIMARY KEY,
			name TEXT NOT NULL,
			key_prefix TEXT NOT NULL,
			key_hash TEXT NOT NULL,
			scope TEXT NOT NULL DEFAULT 'read-only',
			customer_scope TEXT NOT NULL DEFAULT '',
			created_by TEXT NOT NULL,
			expires_at TEXT NOT NULL DEFAULT '',
			revoked_at TEXT NOT NULL DEFAULT '',
			rotated_from TEXT NOT NULL DEFAULT '',
			grace_expires_at TEXT NOT NULL DEFAULT '',
			last_used_at TEXT NOT NULL DEFAULT '',
			created_at TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%fZ', 'now')),
			updated_at TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%fZ', 'now'))
		);
		CREATE INDEX idx_api_keys_key_hash ON api_keys(key_hash);
		CREATE INDEX idx_api_keys_prefix ON api_keys(key_prefix);
		CREATE INDEX idx_api_keys_created_by ON api_keys(created_by);

		CREATE TABLE api_key_audit_log (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			api_key_id TEXT NOT NULL,
			action TEXT NOT NULL,
			actor_id TEXT NOT NULL DEFAULT '',
			actor_username TEXT NOT NULL DEFAULT '',
			ip_address TEXT NOT NULL DEFAULT '',
			details TEXT NOT NULL DEFAULT '',
			created_at TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%fZ', 'now')),
			FOREIGN KEY (api_key_id) REFERENCES api_keys(id) ON DELETE CASCADE
		);
		CREATE INDEX idx_api_key_audit_key ON api_key_audit_log(api_key_id);
		`,
	},
}
