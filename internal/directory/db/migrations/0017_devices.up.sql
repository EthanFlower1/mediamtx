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
