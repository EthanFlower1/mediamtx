// Package connect provides cloud-side components for managing connections
// between on-prem Directory instances and the cloud control plane.
package connect

import (
	"sync"
	"time"
)

const (
	StatusOnline  = "online"
	StatusOffline = "offline"
)

// Session represents an active connection from an on-prem Directory.
type Session struct {
	SiteID       string
	TenantID     string
	SiteAlias    string
	PublicIP     string
	LANCIDRs     []string
	Capabilities map[string]bool
	Status       string
	LastSeen     time.Time

	CameraCount   int
	RecorderCount int
	DiskUsedPct   float64
}

// HeartbeatUpdate carries mutable fields sent in periodic heartbeats.
type HeartbeatUpdate struct {
	CameraCount   int
	RecorderCount int
	DiskUsedPct   float64
	PublicIP      string
}

// Registry is an in-memory, thread-safe registry of connected site sessions.
type Registry struct {
	mu       sync.RWMutex
	bySiteID map[string]*Session
	byAlias  map[string]string // "tenantID:alias" → siteID
}

// NewRegistry creates an empty Registry.
func NewRegistry() *Registry {
	return &Registry{
		bySiteID: make(map[string]*Session),
		byAlias:  make(map[string]string),
	}
}

func aliasKey(tenantID, alias string) string {
	return tenantID + ":" + alias
}

// Add registers (or re-registers) a session. If a session with the same
// SiteID already exists, the old alias mapping is cleaned up first.
func (r *Registry) Add(s Session) {
	r.mu.Lock()
	defer r.mu.Unlock()

	// Clean up old alias if this site was already registered.
	if old, ok := r.bySiteID[s.SiteID]; ok {
		delete(r.byAlias, aliasKey(old.TenantID, old.SiteAlias))
	}

	s.LastSeen = time.Now()
	r.bySiteID[s.SiteID] = &s
	r.byAlias[aliasKey(s.TenantID, s.SiteAlias)] = s.SiteID
}

// Remove deletes a session from both indexes.
func (r *Registry) Remove(siteID string) {
	r.mu.Lock()
	defer r.mu.Unlock()

	s, ok := r.bySiteID[siteID]
	if !ok {
		return
	}
	delete(r.byAlias, aliasKey(s.TenantID, s.SiteAlias))
	delete(r.bySiteID, siteID)
}

// LookupByAlias finds a session by tenant and alias. Returns a copy of the
// session and true if found, or a zero Session and false otherwise.
func (r *Registry) LookupByAlias(tenantID, alias string) (Session, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	siteID, ok := r.byAlias[aliasKey(tenantID, alias)]
	if !ok {
		return Session{}, false
	}
	s, ok := r.bySiteID[siteID]
	if !ok {
		return Session{}, false
	}
	return *s, true
}

// UpdateHeartbeat refreshes mutable fields on an existing session.
// PublicIP is only updated when the provided value is non-empty.
func (r *Registry) UpdateHeartbeat(siteID string, u HeartbeatUpdate) {
	r.mu.Lock()
	defer r.mu.Unlock()

	s, ok := r.bySiteID[siteID]
	if !ok {
		return
	}

	s.CameraCount = u.CameraCount
	s.RecorderCount = u.RecorderCount
	s.DiskUsedPct = u.DiskUsedPct
	if u.PublicIP != "" {
		s.PublicIP = u.PublicIP
	}
	s.LastSeen = time.Now()
}

// ListByTenant returns copies of all sessions belonging to the given tenant.
func (r *Registry) ListByTenant(tenantID string) []Session {
	r.mu.RLock()
	defer r.mu.RUnlock()

	var out []Session
	for _, s := range r.bySiteID {
		if s.TenantID == tenantID {
			out = append(out, *s)
		}
	}
	return out
}
