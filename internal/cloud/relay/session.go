package relay

import (
	"crypto/rand"
	"encoding/hex"
	"sync"
	"time"
)

// RelaySession represents an active relay session between a client and a
// Directory site, tracked by a unique hex ID.
type RelaySession struct {
	ID         string
	SiteID     string
	ClientID   string
	CreatedAt  time.Time
	LastActive time.Time
}

// SessionManager tracks active relay sessions with TTL-based expiry.
type SessionManager struct {
	mu         sync.RWMutex
	sessions   map[string]*RelaySession
	sessionTTL time.Duration
}

// NewSessionManager creates a SessionManager with a default 5-minute TTL.
func NewSessionManager() *SessionManager {
	return &SessionManager{
		sessions:   make(map[string]*RelaySession),
		sessionTTL: 5 * time.Minute,
	}
}

// Create registers a new relay session and returns it.
func (sm *SessionManager) Create(siteID, clientID string) RelaySession {
	b := make([]byte, 16)
	if _, err := rand.Read(b); err != nil {
		panic("crypto/rand failed: " + err.Error())
	}
	id := hex.EncodeToString(b)

	now := time.Now()
	sess := &RelaySession{
		ID:         id,
		SiteID:     siteID,
		ClientID:   clientID,
		CreatedAt:  now,
		LastActive: now,
	}

	sm.mu.Lock()
	sm.sessions[id] = sess
	sm.mu.Unlock()

	return *sess
}

// Get retrieves a session by ID. Returns false if not found.
func (sm *SessionManager) Get(id string) (RelaySession, bool) {
	sm.mu.RLock()
	defer sm.mu.RUnlock()

	sess, ok := sm.sessions[id]
	if !ok {
		return RelaySession{}, false
	}
	return *sess, true
}

// Touch updates the LastActive timestamp for the given session.
func (sm *SessionManager) Touch(id string) {
	sm.mu.Lock()
	defer sm.mu.Unlock()

	if sess, ok := sm.sessions[id]; ok {
		sess.LastActive = time.Now()
	}
}

// Remove deletes a session by ID.
func (sm *SessionManager) Remove(id string) {
	sm.mu.Lock()
	defer sm.mu.Unlock()

	delete(sm.sessions, id)
}

// Cleanup removes all sessions whose LastActive is older than sessionTTL.
func (sm *SessionManager) Cleanup() {
	cutoff := time.Now().Add(-sm.sessionTTL)

	sm.mu.Lock()
	defer sm.mu.Unlock()

	for id, sess := range sm.sessions {
		if sess.LastActive.Before(cutoff) {
			delete(sm.sessions, id)
		}
	}
}
