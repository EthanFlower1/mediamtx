CREATE TABLE IF NOT EXISTS camera_groups (
    id TEXT PRIMARY KEY,
    name TEXT NOT NULL,
    created_at TEXT NOT NULL DEFAULT (datetime('now')),
    updated_at TEXT NOT NULL DEFAULT (datetime('now'))
);

CREATE TABLE IF NOT EXISTS camera_group_members (
    group_id TEXT NOT NULL REFERENCES camera_groups(id) ON DELETE CASCADE,
    camera_id TEXT NOT NULL,
    sort_order INTEGER NOT NULL DEFAULT 0,
    PRIMARY KEY (group_id, camera_id)
);

CREATE INDEX IF NOT EXISTS idx_group_members_group ON camera_group_members(group_id);
CREATE INDEX IF NOT EXISTS idx_group_members_camera ON camera_group_members(camera_id);
