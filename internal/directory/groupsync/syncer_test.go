package groupsync

import (
	"context"
	"fmt"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// --- test double ---

type fakeGroupStore struct {
	groups    map[string]map[string]bool // userID → set of groupIDs
	addErr    error
	removeErr error
}

func newFakeGroupStore() *fakeGroupStore {
	return &fakeGroupStore{groups: make(map[string]map[string]bool)}
}

func (f *fakeGroupStore) UserGroups(_ context.Context, userID string) ([]string, error) {
	gs := f.groups[userID]
	out := make([]string, 0, len(gs))
	for g := range gs {
		out = append(out, g)
	}
	return out, nil
}

func (f *fakeGroupStore) AddUserToGroup(_ context.Context, userID, groupID string) error {
	if f.addErr != nil {
		return f.addErr
	}
	if f.groups[userID] == nil {
		f.groups[userID] = make(map[string]bool)
	}
	f.groups[userID][groupID] = true
	return nil
}

func (f *fakeGroupStore) RemoveUserFromGroup(_ context.Context, userID, groupID string) error {
	if f.removeErr != nil {
		return f.removeErr
	}
	delete(f.groups[userID], groupID)
	return nil
}

func (f *fakeGroupStore) userInGroup(userID, groupID string) bool {
	return f.groups[userID][groupID]
}

// --- tests ---

func TestSync_AddNewGroups(t *testing.T) {
	store := newFakeGroupStore()
	syncer := NewSyncer(store, []Mapping{
		{ExternalGroup: "idp-admins", LocalGroupID: "admin"},
		{ExternalGroup: "idp-operators", LocalGroupID: "operator"},
	}, nil)

	result, err := syncer.Sync(context.Background(), "user-1", []string{"idp-admins", "idp-operators"})
	require.NoError(t, err)

	assert.Len(t, result.Added, 2)
	assert.Empty(t, result.Removed)
	assert.True(t, store.userInGroup("user-1", "admin"))
	assert.True(t, store.userInGroup("user-1", "operator"))
}

func TestSync_RemoveRevokedGroups(t *testing.T) {
	store := newFakeGroupStore()
	store.groups["user-1"] = map[string]bool{"admin": true, "operator": true}

	syncer := NewSyncer(store, []Mapping{
		{ExternalGroup: "idp-admins", LocalGroupID: "admin"},
		{ExternalGroup: "idp-operators", LocalGroupID: "operator"},
	}, nil)

	// IdP now only returns admins (removed from operators).
	result, err := syncer.Sync(context.Background(), "user-1", []string{"idp-admins"})
	require.NoError(t, err)

	assert.Empty(t, result.Added)
	assert.Equal(t, []string{"operator"}, result.Removed)
	assert.True(t, store.userInGroup("user-1", "admin"))
	assert.False(t, store.userInGroup("user-1", "operator"))
}

func TestSync_NoChangeWhenUpToDate(t *testing.T) {
	store := newFakeGroupStore()
	store.groups["user-1"] = map[string]bool{"admin": true}

	syncer := NewSyncer(store, []Mapping{
		{ExternalGroup: "idp-admins", LocalGroupID: "admin"},
	}, nil)

	result, err := syncer.Sync(context.Background(), "user-1", []string{"idp-admins"})
	require.NoError(t, err)

	assert.Empty(t, result.Added)
	assert.Empty(t, result.Removed)
}

func TestSync_UnmappedExternalGroupsIgnored(t *testing.T) {
	store := newFakeGroupStore()
	syncer := NewSyncer(store, []Mapping{
		{ExternalGroup: "idp-admins", LocalGroupID: "admin"},
	}, nil)

	result, err := syncer.Sync(context.Background(), "user-1", []string{"idp-admins", "idp-unknown-group"})
	require.NoError(t, err)

	assert.Equal(t, []string{"admin"}, result.Added)
	assert.False(t, store.userInGroup("user-1", "idp-unknown-group"))
}

func TestSync_AdminManagedGroupsUntouched(t *testing.T) {
	store := newFakeGroupStore()
	// User is in "admin" (managed) and "custom-role" (admin-managed, not in mapping).
	store.groups["user-1"] = map[string]bool{"admin": true, "custom-role": true}

	syncer := NewSyncer(store, []Mapping{
		{ExternalGroup: "idp-admins", LocalGroupID: "admin"},
	}, nil)

	// IdP returns nothing — admin should be removed, but custom-role stays.
	result, err := syncer.Sync(context.Background(), "user-1", nil)
	require.NoError(t, err)

	assert.Empty(t, result.Added)
	assert.Equal(t, []string{"admin"}, result.Removed)
	assert.False(t, store.userInGroup("user-1", "admin"))
	assert.True(t, store.userInGroup("user-1", "custom-role"))
}

func TestSync_EmptyUserIDFails(t *testing.T) {
	syncer := NewSyncer(newFakeGroupStore(), nil, nil)
	_, err := syncer.Sync(context.Background(), "", []string{"foo"})
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "userID is required")
}

func TestSync_AddError(t *testing.T) {
	store := newFakeGroupStore()
	store.addErr = fmt.Errorf("db write failed")

	syncer := NewSyncer(store, []Mapping{
		{ExternalGroup: "idp-admins", LocalGroupID: "admin"},
	}, nil)

	_, err := syncer.Sync(context.Background(), "user-1", []string{"idp-admins"})
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "add to admin")
}

func TestSync_RemoveError(t *testing.T) {
	store := newFakeGroupStore()
	store.groups["user-1"] = map[string]bool{"admin": true}
	store.removeErr = fmt.Errorf("db write failed")

	syncer := NewSyncer(store, []Mapping{
		{ExternalGroup: "idp-admins", LocalGroupID: "admin"},
	}, nil)

	_, err := syncer.Sync(context.Background(), "user-1", nil)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "remove from admin")
}

func TestUpdateMappings(t *testing.T) {
	store := newFakeGroupStore()
	syncer := NewSyncer(store, []Mapping{
		{ExternalGroup: "old-group", LocalGroupID: "admin"},
	}, nil)

	// Update mappings to new external group name.
	syncer.UpdateMappings([]Mapping{
		{ExternalGroup: "new-group", LocalGroupID: "admin"},
	})

	// Old group name should no longer resolve.
	result, err := syncer.Sync(context.Background(), "user-1", []string{"old-group"})
	require.NoError(t, err)
	assert.Empty(t, result.Added)

	// New group name should work.
	result, err = syncer.Sync(context.Background(), "user-1", []string{"new-group"})
	require.NoError(t, err)
	assert.Equal(t, []string{"admin"}, result.Added)
}

func TestManagedGroups(t *testing.T) {
	syncer := NewSyncer(newFakeGroupStore(), []Mapping{
		{ExternalGroup: "a", LocalGroupID: "admin"},
		{ExternalGroup: "b", LocalGroupID: "operator"},
	}, nil)

	groups := syncer.ManagedGroups()
	assert.Len(t, groups, 2)
	assert.Contains(t, groups, "admin")
	assert.Contains(t, groups, "operator")
}

func TestSync_EmptyExternalGroupsClearsManaged(t *testing.T) {
	store := newFakeGroupStore()
	store.groups["user-1"] = map[string]bool{"admin": true, "operator": true}

	syncer := NewSyncer(store, []Mapping{
		{ExternalGroup: "idp-admins", LocalGroupID: "admin"},
		{ExternalGroup: "idp-operators", LocalGroupID: "operator"},
	}, nil)

	result, err := syncer.Sync(context.Background(), "user-1", nil)
	require.NoError(t, err)

	assert.Empty(t, result.Added)
	assert.Len(t, result.Removed, 2)
	assert.False(t, store.userInGroup("user-1", "admin"))
	assert.False(t, store.userInGroup("user-1", "operator"))
}

func TestSync_MultipleExternalToSameLocal(t *testing.T) {
	store := newFakeGroupStore()
	syncer := NewSyncer(store, []Mapping{
		{ExternalGroup: "idp-admins", LocalGroupID: "admin"},
		{ExternalGroup: "idp-superadmins", LocalGroupID: "admin"},
	}, nil)

	// Either external group should map to admin.
	result, err := syncer.Sync(context.Background(), "user-1", []string{"idp-superadmins"})
	require.NoError(t, err)
	assert.Equal(t, []string{"admin"}, result.Added)
}
