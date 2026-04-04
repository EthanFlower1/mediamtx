package db

import (
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestRoleCRUD(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "test.db")
	d, err := Open(dbPath)
	require.NoError(t, err)
	defer d.Close()

	// System roles should be pre-seeded.
	roles, err := d.ListRoles()
	require.NoError(t, err)
	require.Len(t, roles, 4, "should have 4 system roles")

	// Create custom role.
	r := &Role{
		Name:        "security_guard",
		Permissions: `["view_live","ptz_control"]`,
	}
	require.NoError(t, d.CreateRole(r))
	require.NotEmpty(t, r.ID)

	// Get role.
	got, err := d.GetRole(r.ID)
	require.NoError(t, err)
	require.Equal(t, "security_guard", got.Name)
	require.False(t, got.IsSystem)

	// Get role by name.
	got2, err := d.GetRoleByName("security_guard")
	require.NoError(t, err)
	require.Equal(t, r.ID, got2.ID)

	// Update role.
	got.Name = "guard"
	got.Permissions = `["view_live"]`
	require.NoError(t, d.UpdateRole(got))

	updated, err := d.GetRole(r.ID)
	require.NoError(t, err)
	require.Equal(t, "guard", updated.Name)

	// Cannot update system role.
	admin, err := d.GetRole("role-admin")
	require.NoError(t, err)
	admin.Name = "super_admin"
	err = d.UpdateRole(admin)
	require.Error(t, err)
	require.Contains(t, err.Error(), "cannot modify system role")

	// Cannot delete system role.
	err = d.DeleteRole("role-admin")
	require.Error(t, err)
	require.Contains(t, err.Error(), "cannot delete system role")

	// Delete custom role.
	require.NoError(t, d.DeleteRole(r.ID))
	_, err = d.GetRole(r.ID)
	require.ErrorIs(t, err, ErrNotFound)
}

func TestCameraPermissionCRUD(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "test.db")
	d, err := Open(dbPath)
	require.NoError(t, err)
	defer d.Close()

	// Create a user first.
	u := &User{
		Username:     "testuser",
		PasswordHash: "hash",
		Role:         "viewer",
	}
	require.NoError(t, d.CreateUser(u))

	// Create a camera.
	_, err = d.Exec(`INSERT INTO cameras (id, name) VALUES ('cam1', 'Camera 1')`)
	require.NoError(t, err)
	_, err = d.Exec(`INSERT INTO cameras (id, name) VALUES ('cam2', 'Camera 2')`)
	require.NoError(t, err)

	// Set permission.
	cp := &CameraPermission{
		UserID:      u.ID,
		CameraID:    "cam1",
		Permissions: `["view_live","view_playback"]`,
	}
	require.NoError(t, d.SetCameraPermission(cp))

	// Get permission.
	got, err := d.GetCameraPermission(u.ID, "cam1")
	require.NoError(t, err)
	require.Equal(t, `["view_live","view_playback"]`, got.Permissions)

	// List permissions.
	perms, err := d.ListCameraPermissions(u.ID)
	require.NoError(t, err)
	require.Len(t, perms, 1)

	// Upsert (update existing).
	cp2 := &CameraPermission{
		UserID:      u.ID,
		CameraID:    "cam1",
		Permissions: `["view_live","view_playback","export"]`,
	}
	require.NoError(t, d.SetCameraPermission(cp2))
	got2, err := d.GetCameraPermission(u.ID, "cam1")
	require.NoError(t, err)
	require.Equal(t, `["view_live","view_playback","export"]`, got2.Permissions)

	// Bulk set.
	bulkPerms := []*CameraPermission{
		{CameraID: "cam1", Permissions: `["view_live"]`},
		{CameraID: "cam2", Permissions: `["view_live","ptz_control"]`},
	}
	require.NoError(t, d.SetBulkCameraPermissions(u.ID, bulkPerms))
	allPerms, err := d.ListCameraPermissions(u.ID)
	require.NoError(t, err)
	require.Len(t, allPerms, 2)

	// Delete specific permission.
	require.NoError(t, d.DeleteCameraPermission(u.ID, "cam1"))
	allPerms, err = d.ListCameraPermissions(u.ID)
	require.NoError(t, err)
	require.Len(t, allPerms, 1)

	// UserHasCameraPermission checks.
	has, err := d.UserHasCameraPermission(u.ID, "cam2", "view_live")
	require.NoError(t, err)
	require.True(t, has)

	has, err = d.UserHasCameraPermission(u.ID, "cam2", "export")
	require.NoError(t, err)
	require.False(t, has)

	has, err = d.UserHasCameraPermission(u.ID, "cam1", "view_live")
	require.NoError(t, err)
	require.False(t, has, "cam1 permission was deleted")
}
