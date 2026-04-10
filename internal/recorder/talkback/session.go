// Package talkback manages audio talkback sessions between a client
// (Flutter/Web via WebRTC) and a camera's ONVIF backchannel. The Recorder
// acts as a relay: it receives audio from the client over WebRTC and
// forwards it to the camera via ONVIF's backchannel mechanism.
//
// Architecture:
//   Client (WebRTC audio) → Recorder talkback.Session → ONVIF backchannel → Camera speaker
package talkback

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"sync"
	"time"
)

var (
	// ErrSessionExists is returned when attempting to start a talkback
	// session for a camera that already has an active session.
	ErrSessionExists = errors.New("talkback: session already active for this camera")

	// ErrSessionNotFound is returned when attempting to stop or query
	// a non-existent session.
	ErrSessionNotFound = errors.New("talkback: session not found")

	// ErrBackchannelUnavailable is returned when the camera does not
	// support audio backchannel (no ONVIF AudioOutput profile).
	ErrBackchannelUnavailable = errors.New("talkback: camera does not support audio backchannel")
)

// Session represents an active audio talkback relay for a single camera.
type Session struct {
	CameraID  string
	UserID    string
	StartedAt time.Time
	codec     string // "pcmu" (G.711 μ-law) or "pcma" (G.711 A-law)

	mu     sync.Mutex
	cancel context.CancelFunc
	active bool
}

// IsActive reports whether the session is still running.
func (s *Session) IsActive() bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.active
}

// Stop terminates the talkback session.
func (s *Session) Stop() {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.active {
		s.cancel()
		s.active = false
	}
}

// BackchannelSender sends audio frames to the camera.
type BackchannelSender interface {
	// Open initiates the backchannel to the camera. Returns an error if
	// the camera doesn't support it or if the connection fails.
	Open(ctx context.Context, cameraID string) error

	// SendAudio sends a single audio frame (G.711 or similar) to the camera.
	SendAudio(frame []byte) error

	// Close tears down the backchannel.
	Close() error

	// SupportedCodec returns the codec the camera's backchannel expects.
	SupportedCodec(ctx context.Context, cameraID string) (string, error)
}

// Manager tracks active talkback sessions and enforces the one-session-per-camera
// constraint. It is safe for concurrent use.
type Manager struct {
	mu       sync.RWMutex
	sessions map[string]*Session // keyed by cameraID
	sender   BackchannelSender
	log      *slog.Logger
}

// NewManager creates a talkback session manager.
func NewManager(sender BackchannelSender, log *slog.Logger) *Manager {
	if log == nil {
		log = slog.Default()
	}
	return &Manager{
		sessions: make(map[string]*Session),
		sender:   sender,
		log:      log,
	}
}

// StartRequest is the input for starting a talkback session.
type StartRequest struct {
	CameraID string
	UserID   string
}

// Start creates and activates a new talkback session for the given camera.
// Only one session per camera is allowed at a time.
func (m *Manager) Start(ctx context.Context, req StartRequest) (*Session, error) {
	if req.CameraID == "" {
		return nil, fmt.Errorf("talkback: camera_id is required")
	}

	m.mu.Lock()
	defer m.mu.Unlock()

	if existing, ok := m.sessions[req.CameraID]; ok && existing.IsActive() {
		return nil, ErrSessionExists
	}

	// Check codec support.
	codec, err := m.sender.SupportedCodec(ctx, req.CameraID)
	if err != nil {
		return nil, ErrBackchannelUnavailable
	}

	// Open the backchannel.
	if err := m.sender.Open(ctx, req.CameraID); err != nil {
		return nil, fmt.Errorf("talkback: open backchannel: %w", err)
	}

	sessionCtx, cancel := context.WithCancel(context.Background())

	sess := &Session{
		CameraID:  req.CameraID,
		UserID:    req.UserID,
		StartedAt: time.Now(),
		codec:     codec,
		cancel:    cancel,
		active:    true,
	}
	m.sessions[req.CameraID] = sess

	m.log.Info("talkback session started",
		slog.String("camera", req.CameraID),
		slog.String("user", req.UserID),
		slog.String("codec", codec),
	)

	// Background goroutine: wait for cancellation and clean up.
	go func() {
		<-sessionCtx.Done()
		if err := m.sender.Close(); err != nil {
			m.log.Warn("talkback: backchannel close error",
				slog.String("camera", req.CameraID),
				slog.Any("error", err),
			)
		}
		m.log.Info("talkback session ended",
			slog.String("camera", req.CameraID),
			slog.String("user", req.UserID),
		)
	}()

	return sess, nil
}

// Stop terminates the talkback session for the given camera.
func (m *Manager) Stop(cameraID string) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	sess, ok := m.sessions[cameraID]
	if !ok {
		return ErrSessionNotFound
	}

	sess.Stop()
	delete(m.sessions, cameraID)
	return nil
}

// SendAudio relays an audio frame to the active session for the given camera.
func (m *Manager) SendAudio(cameraID string, frame []byte) error {
	m.mu.RLock()
	sess, ok := m.sessions[cameraID]
	m.mu.RUnlock()

	if !ok || !sess.IsActive() {
		return ErrSessionNotFound
	}

	return m.sender.SendAudio(frame)
}

// ActiveSessions returns a snapshot of all active talkback sessions.
func (m *Manager) ActiveSessions() []SessionInfo {
	m.mu.RLock()
	defer m.mu.RUnlock()

	var result []SessionInfo
	for _, s := range m.sessions {
		if s.IsActive() {
			result = append(result, SessionInfo{
				CameraID:  s.CameraID,
				UserID:    s.UserID,
				StartedAt: s.StartedAt,
				Codec:     s.codec,
			})
		}
	}
	return result
}

// SessionInfo is the public view of an active talkback session.
type SessionInfo struct {
	CameraID  string    `json:"camera_id"`
	UserID    string    `json:"user_id"`
	StartedAt time.Time `json:"started_at"`
	Codec     string    `json:"codec"`
}
