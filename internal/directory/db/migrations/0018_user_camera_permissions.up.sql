CREATE TABLE IF NOT EXISTS user_camera_permissions (
    id TEXT PRIMARY KEY,
    user_id TEXT NOT NULL,
    camera_id TEXT NOT NULL,
    permissions TEXT NOT NULL DEFAULT '[]',
    UNIQUE(user_id, camera_id)
);
CREATE INDEX IF NOT EXISTS idx_user_camera_perms_user ON user_camera_permissions(user_id);
CREATE INDEX IF NOT EXISTS idx_user_camera_perms_camera ON user_camera_permissions(camera_id);
