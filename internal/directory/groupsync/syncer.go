// Package groupsync reconciles external IdP group memberships with the
// Directory's local authorization groups (Casbin g-rules). On each login,
// the IdP returns a list of groups the user belongs to. This package:
//
//  1. Maps external group names to local GroupIDs via a configurable mapping table
//  2. Adds the user to any mapped groups they are not already in
//  3. Removes the user from any mapped groups the IdP no longer lists
//  4. Never touches groups that are not in the mapping table (admin-managed groups)
//
// This is the "group sync on login" pattern described in KAI-147.
package groupsync

import (
	"context"
	"fmt"
	"log/slog"
	"sync"
)

// GroupStore abstracts the authorization group membership operations.
// The Directory wires this to the Casbin enforcer's role management API.
type GroupStore interface {
	// UserGroups returns the set of group IDs the user currently belongs to.
	UserGroups(ctx context.Context, userID string) ([]string, error)

	// AddUserToGroup adds the user to the given group. Idempotent.
	AddUserToGroup(ctx context.Context, userID, groupID string) error

	// RemoveUserFromGroup removes the user from the given group. Idempotent.
	RemoveUserFromGroup(ctx context.Context, userID, groupID string) error
}

// Mapping maps an external IdP group name to a local group ID.
type Mapping struct {
	ExternalGroup string // e.g., "cn=admins,ou=groups,dc=example,dc=com" or "security-team"
	LocalGroupID  string // e.g., "admin" or "operator"
}

// Syncer reconciles IdP group claims with local group memberships on login.
type Syncer struct {
	store GroupStore
	log   *slog.Logger

	mu       sync.RWMutex
	mappings map[string]string // external group name → local group ID
	managed  map[string]bool   // set of local group IDs under sync control
}

// NewSyncer creates a group syncer with the given mappings.
func NewSyncer(store GroupStore, mappings []Mapping, log *slog.Logger) *Syncer {
	if log == nil {
		log = slog.Default()
	}
	m := make(map[string]string, len(mappings))
	managed := make(map[string]bool, len(mappings))
	for _, mp := range mappings {
		m[mp.ExternalGroup] = mp.LocalGroupID
		managed[mp.LocalGroupID] = true
	}
	return &Syncer{
		store:    store,
		log:      log.With("component", "groupsync"),
		mappings: m,
		managed:  managed,
	}
}

// SyncResult reports what changed during a sync operation.
type SyncResult struct {
	Added   []string // local group IDs added
	Removed []string // local group IDs removed
}

// Sync reconciles the user's group memberships based on the external groups
// from their IdP token. Only groups that appear in the mapping table are
// touched; admin-managed groups are left alone.
func (s *Syncer) Sync(ctx context.Context, userID string, externalGroups []string) (*SyncResult, error) {
	if userID == "" {
		return nil, fmt.Errorf("groupsync: userID is required")
	}

	s.mu.RLock()
	mappings := s.mappings
	managed := s.managed
	s.mu.RUnlock()

	// Resolve external groups to local group IDs.
	desired := make(map[string]bool)
	for _, ext := range externalGroups {
		if local, ok := mappings[ext]; ok {
			desired[local] = true
		}
	}

	// Get current group memberships.
	currentGroups, err := s.store.UserGroups(ctx, userID)
	if err != nil {
		return nil, fmt.Errorf("groupsync: get user groups: %w", err)
	}
	current := make(map[string]bool, len(currentGroups))
	for _, g := range currentGroups {
		current[g] = true
	}

	result := &SyncResult{}

	// Add user to desired groups they're not in.
	for g := range desired {
		if current[g] {
			continue
		}
		if err := s.store.AddUserToGroup(ctx, userID, g); err != nil {
			return nil, fmt.Errorf("groupsync: add to %s: %w", g, err)
		}
		result.Added = append(result.Added, g)
	}

	// Remove user from managed groups they're no longer in via IdP.
	for _, g := range currentGroups {
		if !managed[g] {
			continue // not a synced group, leave it alone
		}
		if desired[g] {
			continue // still assigned by IdP
		}
		if err := s.store.RemoveUserFromGroup(ctx, userID, g); err != nil {
			return nil, fmt.Errorf("groupsync: remove from %s: %w", g, err)
		}
		result.Removed = append(result.Removed, g)
	}

	if len(result.Added) > 0 || len(result.Removed) > 0 {
		s.log.InfoContext(ctx, "group sync completed",
			slog.String("user", userID),
			slog.Any("added", result.Added),
			slog.Any("removed", result.Removed))
	}

	return result, nil
}

// UpdateMappings replaces the current mapping table. Thread-safe.
func (s *Syncer) UpdateMappings(mappings []Mapping) {
	m := make(map[string]string, len(mappings))
	managed := make(map[string]bool, len(mappings))
	for _, mp := range mappings {
		m[mp.ExternalGroup] = mp.LocalGroupID
		managed[mp.LocalGroupID] = true
	}
	s.mu.Lock()
	s.mappings = m
	s.managed = managed
	s.mu.Unlock()
}

// ManagedGroups returns the set of local group IDs currently under sync control.
func (s *Syncer) ManagedGroups() []string {
	s.mu.RLock()
	defer s.mu.RUnlock()
	out := make([]string, 0, len(s.managed))
	for g := range s.managed {
		out = append(out, g)
	}
	return out
}
