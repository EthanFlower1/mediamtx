package legacydb

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestCreateRole(t *testing.T) {
	d := newTestDB(t)

	r := &Role{
		Name:        "custom_viewer",
		Description: "Custom viewer role",
		Permissions: []string{PermViewLive, PermViewPlayback},
	}
	err := d.CreateRole(r)
	require.NoError(t, err)
	require.NotEmpty(t, r.ID)
	require.NotEmpty(t, r.CreatedAt)
}

func TestGetRole(t *testing.T) {
	d := newTestDB(t)

	r := &Role{
		Name:        "test_role",
		Description: "A test role",
		Permissions: []string{PermViewLive, PermExport},
	}
	require.NoError(t, d.CreateRole(r))

	got, err := d.GetRole(r.ID)
	require.NoError(t, err)
	require.Equal(t, "test_role", got.Name)
	require.Equal(t, "A test role", got.Description)
	require.ElementsMatch(t, []string{PermViewLive, PermExport}, got.Permissions)
	require.False(t, got.IsSystem)
}

func TestGetRoleNotFound(t *testing.T) {
	d := newTestDB(t)

	_, err := d.GetRole("nonexistent")
	require.ErrorIs(t, err, ErrNotFound)
}

func TestGetRoleByName(t *testing.T) {
	d := newTestDB(t)

	r := &Role{
		Name:        "by_name_role",
		Description: "Found by name",
		Permissions: []string{PermAdmin},
	}
	require.NoError(t, d.CreateRole(r))

	got, err := d.GetRoleByName("by_name_role")
	require.NoError(t, err)
	require.Equal(t, r.ID, got.ID)
}

func TestGetRoleByNameNotFound(t *testing.T) {
	d := newTestDB(t)

	_, err := d.GetRoleByName("nope")
	require.ErrorIs(t, err, ErrNotFound)
}

func TestListRolesIncludesSystemRoles(t *testing.T) {
	d := newTestDB(t)

	// System roles are created by migration 40.
	roles, err := d.ListRoles()
	require.NoError(t, err)
	require.GreaterOrEqual(t, len(roles), 3) // admin, operator, viewer

	names := make(map[string]bool)
	for _, r := range roles {
		names[r.Name] = true
	}
	require.True(t, names["admin"])
	require.True(t, names["operator"])
	require.True(t, names["viewer"])
}

func TestUpdateRole(t *testing.T) {
	d := newTestDB(t)

	r := &Role{
		Name:        "mutable_role",
		Description: "Will be updated",
		Permissions: []string{PermViewLive},
	}
	require.NoError(t, d.CreateRole(r))

	r.Name = "updated_role"
	r.Permissions = []string{PermViewLive, PermPTZControl}
	require.NoError(t, d.UpdateRole(r))

	got, err := d.GetRole(r.ID)
	require.NoError(t, err)
	require.Equal(t, "updated_role", got.Name)
	require.ElementsMatch(t, []string{PermViewLive, PermPTZControl}, got.Permissions)
}

func TestUpdateSystemRoleFails(t *testing.T) {
	d := newTestDB(t)

	// The admin role is a system role.
	admin, err := d.GetRoleByName("admin")
	require.NoError(t, err)

	admin.Name = "super_admin"
	err = d.UpdateRole(admin)
	require.Error(t, err)
	require.Contains(t, err.Error(), "cannot modify system role")
}

func TestDeleteRole(t *testing.T) {
	d := newTestDB(t)

	r := &Role{
		Name:        "deletable",
		Description: "Can be deleted",
		Permissions: []string{PermViewLive},
	}
	require.NoError(t, d.CreateRole(r))

	require.NoError(t, d.DeleteRole(r.ID))

	_, err := d.GetRole(r.ID)
	require.ErrorIs(t, err, ErrNotFound)
}

func TestDeleteSystemRoleFails(t *testing.T) {
	d := newTestDB(t)

	admin, err := d.GetRoleByName("admin")
	require.NoError(t, err)

	err = d.DeleteRole(admin.ID)
	require.Error(t, err)
	require.Contains(t, err.Error(), "cannot delete system role")
}

func TestUserCameraPermissions(t *testing.T) {
	d := newTestDB(t)

	// Create a user.
	u := &User{Username: "cam_user", PasswordHash: "h"}
	require.NoError(t, d.CreateUser(u))

	// Create a camera.
	cam := &Camera{Name: "test_cam", MediaMTXPath: "test_cam"}
	require.NoError(t, d.CreateCamera(cam))

	// Set per-camera permissions.
	err := d.SetUserCameraPermissions(u.ID, cam.ID, []string{PermViewLive, PermViewPlayback})
	require.NoError(t, err)

	// Get per-camera permissions.
	perms, err := d.GetUserCameraPermissions(u.ID)
	require.NoError(t, err)
	require.Len(t, perms, 1)
	require.Equal(t, cam.ID, perms[0].CameraID)
	require.ElementsMatch(t, []string{PermViewLive, PermViewPlayback}, perms[0].Permissions)

	// Get single camera permission.
	perm, err := d.GetUserCameraPermission(u.ID, cam.ID)
	require.NoError(t, err)
	require.ElementsMatch(t, []string{PermViewLive, PermViewPlayback}, perm.Permissions)

	// Update permissions (upsert).
	err = d.SetUserCameraPermissions(u.ID, cam.ID, []string{PermViewLive})
	require.NoError(t, err)

	perm, err = d.GetUserCameraPermission(u.ID, cam.ID)
	require.NoError(t, err)
	require.ElementsMatch(t, []string{PermViewLive}, perm.Permissions)

	// Delete by setting empty permissions.
	err = d.SetUserCameraPermissions(u.ID, cam.ID, nil)
	require.NoError(t, err)

	perms, err = d.GetUserCameraPermissions(u.ID)
	require.NoError(t, err)
	require.Len(t, perms, 0)
}

func TestDeleteUserCameraPermissions(t *testing.T) {
	d := newTestDB(t)

	u := &User{Username: "multi_cam_user", PasswordHash: "h"}
	require.NoError(t, d.CreateUser(u))

	cam1 := &Camera{Name: "cam1", MediaMTXPath: "cam1"}
	cam2 := &Camera{Name: "cam2", MediaMTXPath: "cam2"}
	require.NoError(t, d.CreateCamera(cam1))
	require.NoError(t, d.CreateCamera(cam2))

	require.NoError(t, d.SetUserCameraPermissions(u.ID, cam1.ID, []string{PermViewLive}))
	require.NoError(t, d.SetUserCameraPermissions(u.ID, cam2.ID, []string{PermViewLive, PermPTZControl}))

	perms, err := d.GetUserCameraPermissions(u.ID)
	require.NoError(t, err)
	require.Len(t, perms, 2)

	// Delete all at once.
	require.NoError(t, d.DeleteUserCameraPermissions(u.ID))

	perms, err = d.GetUserCameraPermissions(u.ID)
	require.NoError(t, err)
	require.Len(t, perms, 0)
}
