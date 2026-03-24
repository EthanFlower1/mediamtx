package playback

import (
	"sync"
	"time"

	"github.com/google/uuid"

	"github.com/bluenviron/mediamtx/internal/nvr/db"
)

// SessionManager creates, tracks, and cleans up playback sessions.
type SessionManager struct {
	mu sync.Mutex

	DB         *db.DB
	RecordPath string // Full RecordPath pattern

	sessions map[string]*PlaybackSession

	GracePeriod time.Duration
	IdleTimeout time.Duration
}

// NewSessionManager returns an initialised SessionManager and starts the
// background cleanup goroutine.
func NewSessionManager(database *db.DB, recordPath string) *SessionManager {
	m := &SessionManager{
		DB:          database,
		RecordPath:  recordPath,
		sessions:    make(map[string]*PlaybackSession),
		GracePeriod: 30 * time.Second,
		IdleTimeout: 10 * time.Minute,
	}
	go m.cleanupLoop()
	return m
}

// CreateSession creates a new PlaybackSession for the given cameras starting at
// the day that contains startTime, at startPositionSecs seconds into that day.
// Cameras that have no recordings are silently skipped.
func (m *SessionManager) CreateSession(
	cameraIDs []string,
	startTime time.Time,
	startPositionSecs float64,
	onEvent func(Event),
) (*PlaybackSession, error) {
	sessionID := uuid.New().String()

	dayStart := time.Date(
		startTime.Year(), startTime.Month(), startTime.Day(),
		0, 0, 0, 0, startTime.Location(),
	)

	session := NewPlaybackSession(
		sessionID, dayStart, startPositionSecs, m.RecordPath, onEvent,
	)

	for _, camID := range cameraIDs {
		cam, err := m.DB.GetCamera(camID)
		if err != nil {
			// Non-fatal — camera may have no recordings or may not exist.
			continue
		}
		if err := session.AddCamera(camID, cam.MediaMTXPath, m.RecordPath); err != nil {
			continue
		}
	}

	m.mu.Lock()
	m.sessions[sessionID] = session
	m.mu.Unlock()

	return session, nil
}

// GetSession returns the session with the given ID, or nil.
func (m *SessionManager) GetSession(id string) *PlaybackSession {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.sessions[id]
}

// DisposeSession removes the session from the manager and calls Dispose on it.
func (m *SessionManager) DisposeSession(id string) {
	m.mu.Lock()
	session, ok := m.sessions[id]
	if ok {
		delete(m.sessions, id)
	}
	m.mu.Unlock()

	if session != nil {
		session.Dispose()
	}
}

// cleanupLoop periodically disposes sessions that have been idle for longer
// than IdleTimeout.
func (m *SessionManager) cleanupLoop() {
	ticker := time.NewTicker(30 * time.Second)
	defer ticker.Stop()

	for range ticker.C {
		now := time.Now()

		m.mu.Lock()
		var toRemove []string
		for id, s := range m.sessions {
			if now.Sub(s.LastActivity()) > m.IdleTimeout {
				toRemove = append(toRemove, id)
			}
		}
		m.mu.Unlock()

		for _, id := range toRemove {
			m.DisposeSession(id)
		}
	}
}
