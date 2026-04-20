package legacydb

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestCreateUser(t *testing.T) {
	d := newTestDB(t)

	u := &User{Username: "admin", PasswordHash: "hashed123"}
	err := d.CreateUser(u)
	require.NoError(t, err)
	require.NotEmpty(t, u.ID)
	require.Equal(t, "viewer", u.Role)
	require.NotEmpty(t, u.CreatedAt)
}

func TestGetUser(t *testing.T) {
	d := newTestDB(t)

	u := &User{Username: "alice", PasswordHash: "hash", Role: "admin"}
	require.NoError(t, d.CreateUser(u))

	got, err := d.GetUser(u.ID)
	require.NoError(t, err)
	require.Equal(t, "alice", got.Username)
	require.Equal(t, "admin", got.Role)
	require.Equal(t, "hash", got.PasswordHash)
}

func TestGetUserNotFound(t *testing.T) {
	d := newTestDB(t)

	_, err := d.GetUser("nonexistent")
	require.ErrorIs(t, err, ErrNotFound)
}

func TestGetUserByUsername(t *testing.T) {
	d := newTestDB(t)

	u := &User{Username: "bob", PasswordHash: "hash"}
	require.NoError(t, d.CreateUser(u))

	got, err := d.GetUserByUsername("bob")
	require.NoError(t, err)
	require.Equal(t, u.ID, got.ID)

	_, err = d.GetUserByUsername("nonexistent")
	require.ErrorIs(t, err, ErrNotFound)
}

func TestListUsers(t *testing.T) {
	d := newTestDB(t)

	require.NoError(t, d.CreateUser(&User{Username: "charlie", PasswordHash: "h"}))
	require.NoError(t, d.CreateUser(&User{Username: "alice", PasswordHash: "h"}))
	require.NoError(t, d.CreateUser(&User{Username: "bob", PasswordHash: "h"}))

	users, err := d.ListUsers()
	require.NoError(t, err)
	require.Len(t, users, 3)
	require.Equal(t, "alice", users[0].Username)
	require.Equal(t, "bob", users[1].Username)
	require.Equal(t, "charlie", users[2].Username)
}

func TestUpdateUser(t *testing.T) {
	d := newTestDB(t)

	u := &User{Username: "dave", PasswordHash: "old"}
	require.NoError(t, d.CreateUser(u))

	u.PasswordHash = "new"
	u.Role = "admin"
	require.NoError(t, d.UpdateUser(u))

	got, err := d.GetUser(u.ID)
	require.NoError(t, err)
	require.Equal(t, "new", got.PasswordHash)
	require.Equal(t, "admin", got.Role)
}

func TestUpdateUserNotFound(t *testing.T) {
	d := newTestDB(t)

	err := d.UpdateUser(&User{ID: "nonexistent", Username: "x", PasswordHash: "x"})
	require.ErrorIs(t, err, ErrNotFound)
}

func TestDeleteUser(t *testing.T) {
	d := newTestDB(t)

	u := &User{Username: "eve", PasswordHash: "h"}
	require.NoError(t, d.CreateUser(u))

	require.NoError(t, d.DeleteUser(u.ID))
	_, err := d.GetUser(u.ID)
	require.ErrorIs(t, err, ErrNotFound)

	require.ErrorIs(t, d.DeleteUser(u.ID), ErrNotFound)
}

func TestCountUsers(t *testing.T) {
	d := newTestDB(t)

	count, err := d.CountUsers()
	require.NoError(t, err)
	require.Equal(t, 0, count)

	require.NoError(t, d.CreateUser(&User{Username: "u1", PasswordHash: "h"}))
	require.NoError(t, d.CreateUser(&User{Username: "u2", PasswordHash: "h"}))

	count, err = d.CountUsers()
	require.NoError(t, err)
	require.Equal(t, 2, count)
}
