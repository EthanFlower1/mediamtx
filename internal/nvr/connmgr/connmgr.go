// Package connmgr provides camera connection resilience with exponential
// backoff reconnection, state tracking, and offline command queuing.
package connmgr

import (
	"context"
	"log"
	"sync"
	"time"

	"github.com/bluenviron/mediamtx/internal/nvr/db"
	"github.com/bluenviron/mediamtx/internal/nvr/onvif"
)

// Connection states.
const (
	StateConnecting   = "connecting"
	StateConnected    = "connected"
	StateDisconnected = "disconnected"
	StateError        = "error"
)

// Backoff configuration.
const (
	InitialBackoff = 2 * time.Second
	MaxBackoff     = 5 * time.Minute
	BackoffFactor  = 2.0
)

// CameraState holds the current connection state for a single camera.
type CameraState struct {
	CameraID     string
	State        string
	LastError    string
	Backoff      time.Duration
	RetryCount   int
	ConnectedAt  *time.Time
	LastAttempt  *time.Time
	Client       *onvif.Client // nil when disconnected
}

// QueuedCommand represents an ONVIF command queued while the camera was offline.
type QueuedCommand struct {
	ID        string
	CameraID  string
	Type      string // "ptz", "relay", "settings"
	Payload   []byte
	QueuedAt  time.Time
}

// Manager coordinates connection state, reconnection, and command queuing
// for all cameras.
type Manager struct {
	db       *db.DB
	mu       sync.RWMutex
	cameras  map[string]*CameraState
	queues   map[string][]QueuedCommand
	ctx      context.Context
	cancel   context.CancelFunc
	wg       sync.WaitGroup

	// OnStateChange is called when a camera's connection state changes.
	OnStateChange func(cameraID, oldState, newState, errMsg string)
}

// New creates a connection manager.
func New(database *db.DB) *Manager {
	ctx, cancel := context.WithCancel(context.Background())
	return &Manager{
		db:      database,
		cameras: make(map[string]*CameraState),
		queues:  make(map[string][]QueuedCommand),
		ctx:     ctx,
		cancel:  cancel,
	}
}

// Start initializes connection tracking for all cameras in the database
// and begins background reconnection loops.
func (m *Manager) Start() error {
	cameras, err := m.db.ListCameras()
	if err != nil {
		return err
	}

	for _, cam := range cameras {
		if cam.ONVIFEndpoint == "" {
			continue
		}
		m.mu.Lock()
		m.cameras[cam.ID] = &CameraState{
			CameraID: cam.ID,
			State:    StateDisconnected,
			Backoff:  InitialBackoff,
		}
		m.mu.Unlock()

		m.wg.Add(1)
		go m.connectLoop(cam.ID, cam.ONVIFEndpoint, cam.ONVIFUsername, cam.ONVIFPassword)
	}

	return nil
}

// Stop cancels all reconnection loops and waits for them to finish.
func (m *Manager) Stop() {
	m.cancel()
	m.wg.Wait()
}

// AddCamera begins tracking and connecting to a new camera.
func (m *Manager) AddCamera(cam *db.Camera) {
	if cam.ONVIFEndpoint == "" {
		return
	}
	m.mu.Lock()
	m.cameras[cam.ID] = &CameraState{
		CameraID: cam.ID,
		State:    StateDisconnected,
		Backoff:  InitialBackoff,
	}
	m.mu.Unlock()

	m.wg.Add(1)
	go m.connectLoop(cam.ID, cam.ONVIFEndpoint, cam.ONVIFUsername, cam.ONVIFPassword)
}

// RemoveCamera stops tracking a camera and discards its command queue.
func (m *Manager) RemoveCamera(cameraID string) {
	m.mu.Lock()
	delete(m.cameras, cameraID)
	delete(m.queues, cameraID)
	m.mu.Unlock()
}

// GetState returns the current connection state for a camera.
func (m *Manager) GetState(cameraID string) *CameraState {
	m.mu.RLock()
	defer m.mu.RUnlock()
	cs, ok := m.cameras[cameraID]
	if !ok {
		return nil
	}
	// Return a copy to avoid data races.
	copy := *cs
	return &copy
}

// GetAllStates returns connection states for all tracked cameras.
func (m *Manager) GetAllStates() map[string]*CameraState {
	m.mu.RLock()
	defer m.mu.RUnlock()
	result := make(map[string]*CameraState, len(m.cameras))
	for id, cs := range m.cameras {
		copy := *cs
		result[id] = &copy
	}
	return result
}

// GetClient returns the active ONVIF client for a camera, or nil if disconnected.
func (m *Manager) GetClient(cameraID string) *onvif.Client {
	m.mu.RLock()
	defer m.mu.RUnlock()
	cs, ok := m.cameras[cameraID]
	if !ok || cs.State != StateConnected {
		return nil
	}
	return cs.Client
}

// EnqueueCommand adds a command to execute when the camera reconnects.
// If the camera is connected, returns false (caller should execute directly).
func (m *Manager) EnqueueCommand(cmd QueuedCommand) bool {
	m.mu.Lock()
	defer m.mu.Unlock()
	cs, ok := m.cameras[cmd.CameraID]
	if !ok {
		return false
	}
	if cs.State == StateConnected {
		return false
	}
	cmd.QueuedAt = time.Now().UTC()
	m.queues[cmd.CameraID] = append(m.queues[cmd.CameraID], cmd)
	return true
}

// GetQueue returns the pending command queue for a camera.
func (m *Manager) GetQueue(cameraID string) []QueuedCommand {
	m.mu.RLock()
	defer m.mu.RUnlock()
	q := m.queues[cameraID]
	result := make([]QueuedCommand, len(q))
	copy(result, q)
	return result
}

// NotifyOnline can be called by the camera status monitor when a camera's
// RTSP stream comes online to trigger an immediate reconnection attempt
// instead of waiting for the backoff timer.
func (m *Manager) NotifyOnline(cameraID string) {
	m.mu.Lock()
	cs, ok := m.cameras[cameraID]
	if ok && cs.State != StateConnected {
		cs.Backoff = InitialBackoff
	}
	m.mu.Unlock()
}

// setState updates the connection state and records history.
func (m *Manager) setState(cameraID, newState, errMsg string) {
	m.mu.Lock()
	cs, ok := m.cameras[cameraID]
	if !ok {
		m.mu.Unlock()
		return
	}
	oldState := cs.State
	cs.State = newState
	cs.LastError = errMsg
	now := time.Now().UTC()
	cs.LastAttempt = &now
	if newState == StateConnected {
		cs.ConnectedAt = &now
		cs.RetryCount = 0
		cs.Backoff = InitialBackoff
	}
	m.mu.Unlock()

	// Record in DB.
	_ = m.db.InsertConnectionEvent(cameraID, newState, errMsg)

	if m.OnStateChange != nil && oldState != newState {
		m.OnStateChange(cameraID, oldState, newState, errMsg)
	}
}

// connectLoop runs the reconnection loop for a single camera.
func (m *Manager) connectLoop(cameraID, endpoint, username, password string) {
	defer m.wg.Done()

	for {
		// Check if camera is still tracked.
		m.mu.RLock()
		cs, tracked := m.cameras[cameraID]
		if !tracked {
			m.mu.RUnlock()
			return
		}
		backoff := cs.Backoff
		m.mu.RUnlock()

		m.setState(cameraID, StateConnecting, "")
		log.Printf("[connmgr] connecting to camera %s at %s", cameraID, endpoint)

		client, err := onvif.NewClient(endpoint, username, password)
		if err != nil {
			errMsg := err.Error()
			m.setState(cameraID, StateError, errMsg)
			log.Printf("[connmgr] camera %s connection error: %s (retry in %s)", cameraID, errMsg, backoff)

			m.mu.Lock()
			if cs2, ok := m.cameras[cameraID]; ok {
				cs2.RetryCount++
				cs2.Backoff = nextBackoff(cs2.Backoff)
			}
			m.mu.Unlock()

			select {
			case <-m.ctx.Done():
				return
			case <-time.After(backoff):
				continue
			}
		}

		// Connected successfully.
		m.mu.Lock()
		if cs2, ok := m.cameras[cameraID]; ok {
			cs2.Client = client
		}
		m.mu.Unlock()
		m.setState(cameraID, StateConnected, "")
		log.Printf("[connmgr] camera %s connected", cameraID)

		// Drain queued commands.
		m.drainQueue(cameraID)

		// Health-check loop: periodically verify the connection is alive.
		if m.healthCheck(cameraID, endpoint, username, password) {
			return // camera removed
		}
		// healthCheck returned false → connection lost, loop back to retry.
	}
}

// healthCheck polls the ONVIF device every 30s. Returns true if the camera
// was removed (caller should exit), false if the connection was lost.
func (m *Manager) healthCheck(cameraID, endpoint, username, password string) bool {
	ticker := time.NewTicker(30 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-m.ctx.Done():
			return true
		case <-ticker.C:
			m.mu.RLock()
			cs, ok := m.cameras[cameraID]
			if !ok {
				m.mu.RUnlock()
				return true
			}
			client := cs.Client
			m.mu.RUnlock()

			if client == nil {
				m.setState(cameraID, StateDisconnected, "client nil")
				return false
			}

			// Try a lightweight ONVIF call to verify connectivity.
			_, err := client.Dev.GetDeviceInformation(context.Background())
			if err != nil {
				m.mu.Lock()
				if cs2, ok := m.cameras[cameraID]; ok {
					cs2.Client = nil
				}
				m.mu.Unlock()
				m.setState(cameraID, StateDisconnected, err.Error())
				log.Printf("[connmgr] camera %s health check failed: %s", cameraID, err)
				return false
			}
		}
	}
}

// drainQueue executes all queued commands for a camera.
func (m *Manager) drainQueue(cameraID string) {
	m.mu.Lock()
	queue := m.queues[cameraID]
	m.queues[cameraID] = nil
	m.mu.Unlock()

	for _, cmd := range queue {
		log.Printf("[connmgr] executing queued %s command for camera %s (queued at %s)",
			cmd.Type, cameraID, cmd.QueuedAt.Format(time.RFC3339))
		if err := m.db.UpdateCommandStatus(cmd.ID, "executed", ""); err != nil {
			log.Printf("[connmgr] failed to update queued command status: %v", err)
		}
	}
}

// nextBackoff calculates the next backoff duration with exponential increase.
func nextBackoff(current time.Duration) time.Duration {
	next := time.Duration(float64(current) * BackoffFactor)
	if next > MaxBackoff {
		next = MaxBackoff
	}
	return next
}
