package backchannel

import (
	"context"
	"errors"
	"fmt"
	"log"
	"sync"
	"time"

	"github.com/bluenviron/mediamtx/internal/recorder/onvif"
)

// Session state constants.
type SessionState int

const (
	StateIdle       SessionState = iota // 0
	StateConnecting                     // 1
	StateActive                         // 2
	StateClosing                        // 3
)

// idleTimeout is how long an idle session is kept before teardown.
const idleTimeout = 30 * time.Second

// Sentinel errors for session management.
var (
	ErrNoSession     = errors.New("no active backchannel session for this camera")
	ErrCameraBusy    = errors.New("backchannel session already active for this camera")
	ErrNoBackchannel = errors.New("camera does not support audio backchannel")
	ErrNoCodec       = errors.New("no compatible audio codec found on camera")
)

// CredentialFunc returns ONVIF connection info for a given camera ID.
type CredentialFunc func(cameraID string) (xaddr, user, pass string, err error)

// SessionInfo is the public-facing session state returned to callers.
type SessionInfo struct {
	Codec      string `json:"codec"`
	SampleRate int    `json:"sample_rate"`
	Bitrate    int    `json:"bitrate"`
}

// Session holds all state for a single camera's backchannel session.
type Session struct {
	CameraID   string
	State      SessionState
	Codec      string
	SampleRate int
	Bitrate    int
	rtspConn   *RTSPConn
	idleTimer  *time.Timer
	mu         sync.Mutex
}

// Manager orchestrates backchannel sessions across multiple cameras.
type Manager struct {
	sessions   map[string]*Session
	mu         sync.RWMutex
	onvifCreds CredentialFunc
}

// NewManager creates a Manager with the given credential lookup function.
func NewManager(creds CredentialFunc) *Manager {
	return &Manager{
		sessions:   make(map[string]*Session),
		onvifCreds: creds,
	}
}

// GetSessionInfo returns the current SessionInfo for cameraID, or (nil, false).
func (m *Manager) GetSessionInfo(cameraID string) (*SessionInfo, bool) {
	m.mu.RLock()
	s, ok := m.sessions[cameraID]
	m.mu.RUnlock()
	if !ok {
		return nil, false
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	return &SessionInfo{
		Codec:      s.Codec,
		SampleRate: s.SampleRate,
		Bitrate:    s.Bitrate,
	}, true
}

// StartSession establishes a backchannel session for cameraID.
// It negotiates the codec via ONVIF and connects an RTSPConn.
func (m *Manager) StartSession(ctx context.Context, cameraID string) (*SessionInfo, error) {
	m.mu.Lock()
	existing, exists := m.sessions[cameraID]
	if exists {
		existing.mu.Lock()
		state := existing.State
		hasConn := existing.rtspConn != nil
		existing.mu.Unlock()

		switch state {
		case StateActive, StateConnecting:
			m.mu.Unlock()
			return nil, ErrCameraBusy

		case StateIdle:
			if hasConn {
				// Reuse idle session: cancel idle timer and reactivate.
				existing.mu.Lock()
				if existing.idleTimer != nil {
					existing.idleTimer.Stop()
					existing.idleTimer = nil
				}
				existing.State = StateActive
				info := &SessionInfo{
					Codec:      existing.Codec,
					SampleRate: existing.SampleRate,
					Bitrate:    existing.Bitrate,
				}
				existing.mu.Unlock()
				m.mu.Unlock()
				return info, nil
			}
			// Stale idle session with no connection — clean up and re-create below.
			delete(m.sessions, cameraID)

		case StateClosing:
			// Let it finish closing and create a new one below.
			delete(m.sessions, cameraID)
		}
	}

	// Create new session in StateConnecting.
	sess := &Session{
		CameraID: cameraID,
		State:    StateConnecting,
	}
	m.sessions[cameraID] = sess
	m.mu.Unlock()

	info, err := m.negotiate(ctx, cameraID, sess)
	if err != nil {
		m.removeSession(cameraID)
		return nil, err
	}

	return info, nil
}

// negotiate performs ONVIF discovery, codec negotiation, and RTSP connection.
func (m *Manager) negotiate(ctx context.Context, cameraID string, sess *Session) (*SessionInfo, error) {
	xaddr, user, pass, err := m.onvifCreds(cameraID)
	if err != nil {
		return nil, fmt.Errorf("backchannel: lookup credentials for %s: %w", cameraID, err)
	}

	// Check audio outputs.
	outputs, err := onvif.GetAudioOutputs(xaddr, user, pass)
	if err != nil {
		return nil, fmt.Errorf("backchannel: get audio outputs: %w", err)
	}
	if len(outputs) == 0 {
		return nil, ErrNoBackchannel
	}

	// Get decoder configs and the first token.
	decoderCfgs, err := onvif.GetAudioDecoderConfigs(xaddr, user, pass)
	if err != nil {
		return nil, fmt.Errorf("backchannel: get audio decoder configs: %w", err)
	}
	var decoderToken string
	if len(decoderCfgs) > 0 {
		decoderToken = decoderCfgs[0].Token
	}

	// Get decoder options for that token.
	opts, err := onvif.GetAudioDecoderOpts(xaddr, user, pass, decoderToken)
	if err != nil {
		return nil, fmt.Errorf("backchannel: get audio decoder options: %w", err)
	}

	// Negotiate codec.
	codec := onvif.NegotiateCodec(opts)
	if codec == nil {
		return nil, ErrNoCodec
	}

	// Get profiles and the first profile token.
	profiles, err := onvif.GetProfilesFull(xaddr, user, pass)
	if err != nil {
		return nil, fmt.Errorf("backchannel: get profiles: %w", err)
	}
	var profileToken string
	if len(profiles) > 0 {
		profileToken = profiles[0].Token
	}

	// Get backchannel stream URI.
	streamURI, err := onvif.GetBackchannelStreamURI(xaddr, user, pass, profileToken)
	if err != nil {
		return nil, fmt.Errorf("backchannel: get stream URI: %w", err)
	}

	// Connect RTSP.
	rtspConn := NewRTSPConn(streamURI, user, pass)
	if err := rtspConn.Connect(ctx, codec.Encoding, codec.SampleRate); err != nil {
		return nil, fmt.Errorf("backchannel: RTSP connect: %w", err)
	}

	// Activate session.
	sess.mu.Lock()
	sess.Codec = codec.Encoding
	sess.SampleRate = codec.SampleRate
	sess.Bitrate = codec.Bitrate
	sess.rtspConn = rtspConn
	sess.State = StateActive
	sess.mu.Unlock()

	return &SessionInfo{
		Codec:      codec.Encoding,
		SampleRate: codec.SampleRate,
		Bitrate:    codec.Bitrate,
	}, nil
}

// SendAudio forwards audio data to the active RTSP connection for cameraID.
func (m *Manager) SendAudio(cameraID string, audioData []byte) error {
	m.mu.RLock()
	sess, ok := m.sessions[cameraID]
	m.mu.RUnlock()
	if !ok {
		return ErrNoSession
	}

	sess.mu.Lock()
	state := sess.State
	conn := sess.rtspConn
	sess.mu.Unlock()

	if state != StateActive || conn == nil {
		return ErrNoSession
	}

	return conn.SendAudio(audioData)
}

// StopSession transitions cameraID to idle and starts a 30-second teardown timer.
func (m *Manager) StopSession(cameraID string) error {
	m.mu.RLock()
	sess, ok := m.sessions[cameraID]
	m.mu.RUnlock()
	if !ok {
		return ErrNoSession
	}

	sess.mu.Lock()
	if sess.State != StateActive {
		sess.mu.Unlock()
		return ErrNoSession
	}
	sess.State = StateIdle

	// Cancel any existing idle timer.
	if sess.idleTimer != nil {
		sess.idleTimer.Stop()
	}
	sess.idleTimer = time.AfterFunc(idleTimeout, func() {
		m.teardownSession(cameraID)
	})
	sess.mu.Unlock()

	return nil
}

// CloseAll tears down all sessions immediately.
func (m *Manager) CloseAll() {
	m.mu.Lock()
	defer m.mu.Unlock()

	for id, sess := range m.sessions {
		sess.mu.Lock()
		if sess.idleTimer != nil {
			sess.idleTimer.Stop()
			sess.idleTimer = nil
		}
		if sess.rtspConn != nil {
			if err := sess.rtspConn.Close(); err != nil {
				log.Printf("backchannel: CloseAll: close RTSP for %s: %v", id, err)
			}
			sess.rtspConn = nil
		}
		sess.State = StateClosing
		sess.mu.Unlock()
	}

	m.sessions = make(map[string]*Session)
}

// teardownSession is called by the idle timer to close the RTSP connection and remove the session.
func (m *Manager) teardownSession(cameraID string) {
	m.mu.Lock()
	sess, ok := m.sessions[cameraID]
	if !ok {
		m.mu.Unlock()
		return
	}

	sess.mu.Lock()
	sess.State = StateClosing
	conn := sess.rtspConn
	sess.rtspConn = nil
	sess.mu.Unlock()

	delete(m.sessions, cameraID)
	m.mu.Unlock()

	if conn != nil {
		if err := conn.Close(); err != nil {
			log.Printf("backchannel: teardown %s: close RTSP: %v", cameraID, err)
		}
	}
}

// removeSession deletes a session from the map without closing anything (used on setup failure).
func (m *Manager) removeSession(cameraID string) {
	m.mu.Lock()
	delete(m.sessions, cameraID)
	m.mu.Unlock()
}
