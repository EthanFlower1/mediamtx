package scheduler

import (
	"sync"
	"time"
)

const hysteresisTimeout = 10 * time.Second

// motionState represents the state of the motion state machine.
type motionState string

const (
	stateIdle        motionState = "idle"
	stateRecording   motionState = "recording"
	statePostBuffer  motionState = "post_buffer"
	stateHysteresis  motionState = "hysteresis"
)

// SetRecordingFunc is called by the motion state machine to enable or disable
// recording for a given MediaMTX path.
type SetRecordingFunc func(path string, record bool)

// MotionSM is a per-camera motion state machine that manages recording state
// based on ONVIF motion detection events. The state transitions are:
//
//	idle -> recording (motion detected)
//	recording -> post_buffer (motion stopped)
//	post_buffer -> hysteresis (post-event timer expires)
//	hysteresis -> idle (hysteresis timer expires)
//	post_buffer or hysteresis + motion -> recording (re-trigger)
type MotionSM struct {
	cameraID         string
	path             string
	postEventTimeout time.Duration
	setRecording     SetRecordingFunc

	mu              sync.Mutex
	state           motionState
	postBufferTimer *time.Timer
	hysteresisTimer *time.Timer
}

// NewMotionSM creates a new motion state machine for a camera.
func NewMotionSM(cameraID, path string, postEventTimeout time.Duration, setRec SetRecordingFunc) *MotionSM {
	return &MotionSM{
		cameraID:         cameraID,
		path:             path,
		postEventTimeout: postEventTimeout,
		setRecording:     setRec,
		state:            stateIdle,
	}
}

// State returns the current state of the motion state machine.
func (sm *MotionSM) State() string {
	sm.mu.Lock()
	defer sm.mu.Unlock()
	return string(sm.state)
}

// OnMotion is called when a motion event is received from the ONVIF event
// subscriber. detected=true means motion started; detected=false means
// motion stopped.
func (sm *MotionSM) OnMotion(detected bool) {
	sm.mu.Lock()
	defer sm.mu.Unlock()

	if detected {
		sm.onMotionDetected()
	} else {
		sm.onMotionStopped()
	}
}

// onMotionDetected handles a motion-start event. Must be called with mu held.
func (sm *MotionSM) onMotionDetected() {
	switch sm.state {
	case stateIdle:
		sm.state = stateRecording
		sm.setRecording(sm.path, true)

	case statePostBuffer:
		sm.cancelTimers()
		sm.state = stateRecording
		// Already recording, no YAML write needed.

	case stateHysteresis:
		sm.cancelTimers()
		sm.state = stateRecording
		// Already recording, no YAML write needed.

	case stateRecording:
		// Already recording, no-op.
	}
}

// onMotionStopped handles a motion-stop event. Must be called with mu held.
func (sm *MotionSM) onMotionStopped() {
	switch sm.state {
	case stateRecording:
		sm.state = statePostBuffer
		sm.postBufferTimer = time.AfterFunc(sm.postEventTimeout, sm.expirePostBuffer)

	case stateIdle, statePostBuffer, stateHysteresis:
		// No-op in these states.
	}
}

// expirePostBuffer is called when the post-event timer fires.
func (sm *MotionSM) expirePostBuffer() {
	sm.mu.Lock()
	defer sm.mu.Unlock()

	if sm.state != statePostBuffer {
		return
	}

	sm.postBufferTimer = nil
	sm.state = stateHysteresis
	sm.hysteresisTimer = time.AfterFunc(hysteresisTimeout, sm.expireHysteresis)
}

// expireHysteresis is called when the hysteresis timer fires.
func (sm *MotionSM) expireHysteresis() {
	sm.mu.Lock()
	defer sm.mu.Unlock()

	if sm.state != stateHysteresis {
		return
	}

	sm.hysteresisTimer = nil
	sm.state = stateIdle
	sm.setRecording(sm.path, false)
}

// Stop cancels all timers and transitions to idle with recording disabled.
// It is safe to call multiple times.
func (sm *MotionSM) Stop() {
	sm.mu.Lock()
	defer sm.mu.Unlock()

	wasActive := sm.state != stateIdle
	sm.cancelTimers()
	sm.state = stateIdle

	if wasActive {
		sm.setRecording(sm.path, false)
	}
}

// cancelTimers stops any running timers. Must be called with mu held.
func (sm *MotionSM) cancelTimers() {
	if sm.postBufferTimer != nil {
		sm.postBufferTimer.Stop()
		sm.postBufferTimer = nil
	}
	if sm.hysteresisTimer != nil {
		sm.hysteresisTimer.Stop()
		sm.hysteresisTimer = nil
	}
}
