// Package scheduler evaluates recording rules on a 30-second tick and
// manages the recording state for each camera by writing to the MediaMTX
// YAML configuration.
package scheduler

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"log"
	"net/url"
	"os"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/bluenviron/mediamtx/internal/nvr/crypto"
	"github.com/bluenviron/mediamtx/internal/nvr/db"
	"github.com/bluenviron/mediamtx/internal/nvr/onvif"
	"github.com/bluenviron/mediamtx/internal/nvr/yamlwriter"
)

// EventPublisher is an interface for publishing system events from the scheduler.
// This avoids a circular import with the api package.
type EventPublisher interface {
	PublishMotion(cameraName string)
	PublishTampering(cameraName string)
	PublishCameraOffline(cameraName string)
	PublishCameraOnline(cameraName string)
	PublishRecordingStarted(cameraName string)
	PublishRecordingStopped(cameraName string)
	PublishRecordingStalled(cameraName string)
	PublishRecordingRecovered(cameraName string)
	PublishRecordingFailed(cameraName string)
}

// EffectiveMode represents the resolved recording mode for a camera after
// evaluating all matching rules.
type EffectiveMode string

const (
	// ModeOff means no recording.
	ModeOff EffectiveMode = "off"
	// ModeAlways means continuous recording.
	ModeAlways EffectiveMode = "always"
	// ModeEvents means record only on motion/event triggers.
	ModeEvents EffectiveMode = "events"
)

const (
	evalInterval    = 30 * time.Second
	startupDelay    = 5 * time.Second
	writeCoalesceMs = 500
)

// CameraState holds the evaluated recording state for a single camera.
type CameraState struct {
	EffectiveMode EffectiveMode
	Recording     bool
	MotionState   string   // "idle", "recording", "post_buffer", "hysteresis"
	ActiveRules   []string // IDs of currently matching rules
}

// retentionCheckInterval is the minimum interval between retention cleanup runs.
const retentionCheckInterval = 1 * time.Hour

// Scheduler evaluates recording rules every 30 seconds and applies
// recording state changes to the MediaMTX YAML configuration.
type Scheduler struct {
	db              *db.DB
	yamlWriter      *yamlwriter.Writer
	eventPub        EventPublisher
	encryptionKey   []byte // for decrypting ONVIF passwords from DB
	callbackMgr     *onvif.CallbackManager
	apiAddress      string // e.g., ":9997" for building callback URLs

	mu     sync.Mutex
	states map[string]*CameraState // camera ID -> state
	stopCh chan struct{}
	wg     sync.WaitGroup

	motionSMs map[string]*MotionSM             // camera ID -> motion state machine
	eventSubs map[string]*onvif.EventSubscriber // camera ID -> event subscriber

	pendingWrites   map[string]bool // mediamtx path -> desired record state
	pendingWritesMu sync.Mutex

	motionTimers   map[string]*time.Timer // camera ID -> auto-close timer
	motionTimersMu sync.Mutex
	writeTimer      *time.Timer

	lastRetentionCheck time.Time // timestamp of last retention cleanup run

	healthStates map[string]*RecordingHealth // camera ID -> recording health
}

// New creates a new Scheduler.
func New(database *db.DB, writer *yamlwriter.Writer, encKey []byte, callbackMgr *onvif.CallbackManager, apiAddress string) *Scheduler {
	return &Scheduler{
		db:            database,
		yamlWriter:    writer,
		encryptionKey: encKey,
		callbackMgr:   callbackMgr,
		apiAddress:    apiAddress,
		states:        make(map[string]*CameraState),
		stopCh:        make(chan struct{}),
		pendingWrites: make(map[string]bool),
		motionSMs:     make(map[string]*MotionSM),
		eventSubs:     make(map[string]*onvif.EventSubscriber),
		motionTimers:  make(map[string]*time.Timer),
		healthStates: make(map[string]*RecordingHealth),
	}
}

// SetEventBroadcaster sets the event publisher used to broadcast system events
// such as motion detection and camera status changes.
func (s *Scheduler) SetEventBroadcaster(pub EventPublisher) {
	s.eventPub = pub
}

// Start launches the background evaluation goroutine. The first evaluation
// is deferred by 5 seconds to avoid racing with MediaMTX config load.
func (s *Scheduler) Start() {
	s.wg.Add(1)
	go s.run()
}

// Stop signals the scheduler goroutine to exit and waits for it to finish.
// After Stop returns the scheduler can be restarted with Start().
func (s *Scheduler) Stop() {
	close(s.stopCh)
	s.wg.Wait()

	// Stop all event subscribers and motion state machines.
	s.mu.Lock()
	for camID := range s.eventSubs {
		s.stopEventPipelineLocked(camID)
	}
	s.mu.Unlock()

	// Cancel all pending motion auto-close timers.
	s.motionTimersMu.Lock()
	for _, timer := range s.motionTimers {
		timer.Stop()
	}
	s.motionTimers = make(map[string]*time.Timer)
	s.motionTimersMu.Unlock()

	// Flush any remaining pending writes.
	s.pendingWritesMu.Lock()
	if s.writeTimer != nil {
		s.writeTimer.Stop()
	}
	pending := s.pendingWrites
	s.pendingWrites = make(map[string]bool)
	s.pendingWritesMu.Unlock()

	for path, record := range pending {
		if err := s.yamlWriter.SetPathValue(path, "record", record); err != nil {
			log.Printf("scheduler: flush write for %s: %v", path, err)
		}
	}

	// Reinitialize the stop channel so Start() can be called again.
	s.stopCh = make(chan struct{})
}

// RemoveCamera removes tracked state for the given camera ID (and all its
// per-stream keys) and stops any active event subscriber or motion state machine.
func (s *Scheduler) RemoveCamera(cameraID string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.stopEventPipelineLocked(cameraID)
	// Delete the camera's own state plus any stream-keyed states.
	for sk := range s.states {
		if sk == cameraID || strings.HasPrefix(sk, cameraID+":") {
			delete(s.states, sk)
		}
	}
	delete(s.healthStates, cameraID)
}

// NotifySegmentForCamera is called when a recording segment completes for the
// given camera. It updates the recording health state and publishes a recovery
// event if the camera was previously stalled or failed.
func (s *Scheduler) NotifySegmentForCamera(cameraID string) {
	s.mu.Lock()
	defer s.mu.Unlock()

	h, ok := s.healthStates[cameraID]
	if !ok {
		h = NewRecordingHealth()
		s.healthStates[cameraID] = h
	}
	prev := h.RecordSegment(time.Now())

	if (prev == HealthStalled || prev == HealthFailed) && h.Status == HealthHealthy {
		if s.eventPub != nil {
			cam, err := s.db.GetCamera(cameraID)
			if err == nil {
				s.eventPub.PublishRecordingRecovered(cam.Name)
			}
		}
	}
}

// GetCameraState returns a copy of the current state for a camera, or nil.
func (s *Scheduler) GetCameraState(cameraID string) *CameraState {
	s.mu.Lock()
	defer s.mu.Unlock()
	st, ok := s.states[cameraID]
	if !ok {
		return nil
	}
	// Return a copy to avoid data races.
	cp := *st
	cp.ActiveRules = make([]string, len(st.ActiveRules))
	copy(cp.ActiveRules, st.ActiveRules)
	return &cp
}

// GetRecordingHealth returns a copy of the recording health for the given camera.
// Returns nil if no health state exists.
func (s *Scheduler) GetRecordingHealth(cameraID string) *RecordingHealth {
	s.mu.Lock()
	defer s.mu.Unlock()
	h, ok := s.healthStates[cameraID]
	if !ok {
		return nil
	}
	cp := *h
	return &cp
}

// GetAllRecordingHealth returns a copy of all recording health states.
func (s *Scheduler) GetAllRecordingHealth() map[string]*RecordingHealth {
	s.mu.Lock()
	defer s.mu.Unlock()
	result := make(map[string]*RecordingHealth, len(s.healthStates))
	for k, v := range s.healthStates {
		cp := *v
		result[k] = &cp
	}
	return result
}

// run is the main scheduler loop.
func (s *Scheduler) run() {
	defer s.wg.Done()

	// Defer first evaluation to let MediaMTX load its config.
	select {
	case <-time.After(startupDelay):
	case <-s.stopCh:
		return
	}

	s.evaluate()

	evalTicker := time.NewTicker(evalInterval)
	defer evalTicker.Stop()

	for {
		select {
		case <-evalTicker.C:
			s.evaluate()
		case <-s.stopCh:
			return
		}
	}
}

// streamKey builds a map key for per-stream state from camera ID and stream ID.
func streamKey(cameraID, streamID string) string {
	if streamID == "" {
		return cameraID
	}
	return cameraID + ":" + streamID
}

// streamPath returns the MediaMTX path for a stream.
// Uses the first 8 characters of the stream ID as a stable suffix.
// Stream names are for display only — paths use IDs so renaming a stream
// doesn't orphan recordings or YAML entries.
func streamPath(cam *db.Camera, streamID string) string {
	if streamID == "" || cam.MediaMTXPath == "" {
		return cam.MediaMTXPath
	}
	prefix := streamID
	if len(prefix) > 8 {
		prefix = prefix[:8]
	}
	return cam.MediaMTXPath + "~" + prefix
}

// ensureStreamPath creates a MediaMTX path for a non-default stream.
// The record parameter sets the initial recording state in the YAML.
func (s *Scheduler) ensureStreamPath(cam *db.Camera, streamID string, record bool) string {
	if streamID == "" {
		return cam.MediaMTXPath
	}

	stream, err := s.db.GetCameraStream(streamID)
	if err != nil {
		log.Printf("scheduler: stream %s not found for camera %s", streamID, cam.ID)
		return ""
	}

	path := streamPath(cam, streamID)

	streamURL := stream.RTSPURL
	// Validate the URL is a usable RTSP source.
	if streamURL == "" || !strings.HasPrefix(streamURL, "rtsp://") {
		log.Printf("scheduler: stream %s has invalid RTSP URL %q for camera %s, skipping", streamID, streamURL, cam.ID)
		return ""
	}
	if u, parseErr := url.Parse(streamURL); parseErr == nil && u.Host != "" && (u.User == nil || u.User.Username() == "") {
		username := cam.ONVIFUsername
		password := s.decryptPassword(cam.ONVIFPassword)
		if username != "" {
			u.User = url.UserPassword(username, password)
			streamURL = u.String()
		}
	}

	recordBase := "./recordings/"
	if cam.StoragePath != "" {
		recordBase = cam.StoragePath + "/"
	}
	recordPath := recordBase + "%path/%Y-%m/%d/%H-%M-%S-%f"

	s.yamlWriter.AddPath(path, map[string]interface{}{
		"source":     streamURL,
		"record":     record,
		"recordPath": recordPath,
	})

	return path
}

// handleStreamTransition manages the MotionSM lifecycle when a stream's
// effective mode changes.
func (s *Scheduler) handleStreamTransition(
	sk, camID string,
	cam *db.Camera,
	path, streamID string,
	prevMode, newMode EffectiveMode,
	rules []*db.RecordingRule,
) {
	if prevMode == ModeEvents && newMode != ModeEvents {
		if sm, ok := s.motionSMs[sk]; ok {
			sm.Stop()
			delete(s.motionSMs, sk)
		}
	}

	if newMode == ModeEvents && prevMode != ModeEvents {
		postEvent := 30 * time.Second
		for _, r := range rules {
			if r.PostEventSeconds > 0 {
				postEvent = time.Duration(r.PostEventSeconds) * time.Second
				break
			}
		}
		sm := NewMotionSM(camID, path, postEvent, func(p string, record bool) {
			s.queueWrite(p, record)
		})
		s.motionSMs[sk] = sm
	}
}

// evaluate fetches all enabled rules from the DB, groups them by camera,
// evaluates the effective mode, and queues YAML writes for any state changes.
func (s *Scheduler) evaluate() {
	now := time.Now()

	rules, err := s.db.ListAllEnabledRecordingRules()
	if err != nil {
		log.Printf("scheduler: list rules: %v", err)
		return
	}

	cameras, err := s.db.ListCameras()
	if err != nil {
		log.Printf("scheduler: list cameras: %v", err)
		return
	}

	// Build camera ID -> Camera lookup.
	camByID := make(map[string]*db.Camera, len(cameras))
	for _, c := range cameras {
		camByID[c.ID] = c
	}

	// Group rules by camera ID.
	rulesByCam := make(map[string][]*db.RecordingRule)
	for _, r := range rules {
		rulesByCam[r.CameraID] = append(rulesByCam[r.CameraID], r)
	}

	// Evaluate ALL cameras (not just those with rules) so we can subscribe
	// to ONVIF events for motion alerts on every camera.
	// Also start ONVIF event subscriptions for any camera that has an ONVIF
	// endpoint but doesn't yet have a subscriber — regardless of recording rules.
	// Both operations are done under a single lock to avoid a race on s.eventSubs.
	s.mu.Lock()
	evalCameras := make(map[string]struct{})
	for _, c := range cameras {
		evalCameras[c.ID] = struct{}{}
	}
	// Include cameras that have existing state (stream keys contain the camera ID
	// before any ":" separator).
	for sk := range s.states {
		camID := sk
		if idx := strings.Index(sk, ":"); idx >= 0 {
			camID = sk[:idx]
		}
		evalCameras[camID] = struct{}{}
	}
	for _, cam := range cameras {
		if cam.ONVIFEndpoint != "" {
			if _, hasSub := s.eventSubs[cam.ID]; !hasSub {
				s.startMotionAlertSubscription(cam)
			}
		}
	}
	s.mu.Unlock()

	for camID := range evalCameras {
		cam, ok := camByID[camID]
		if !ok {
			continue
		}

		camRules := rulesByCam[camID]

		// Group rules by stream ID.
		rulesByStream := make(map[string][]*db.RecordingRule)
		for _, r := range camRules {
			rulesByStream[r.StreamID] = append(rulesByStream[r.StreamID], r)
		}

		// Also evaluate streams that previously had state but no longer have rules.
		s.mu.Lock()
		for sk := range s.states {
			if sk == camID || strings.HasPrefix(sk, camID+":") {
				streamID := ""
				if idx := strings.Index(sk, ":"); idx >= 0 {
					streamID = sk[idx+1:]
				}
				if _, hasRules := rulesByStream[streamID]; !hasRules {
					rulesByStream[streamID] = nil
				}
			}
		}

		s.mu.Unlock()

		for streamID, streamRules := range rulesByStream {
			sk := streamKey(camID, streamID)
			mode, activeIDs := EvaluateRules(streamRules, now)
			desiredRecording := mode == ModeAlways

			path := ""
			if streamID == "" {
				path = cam.MediaMTXPath
			} else {
				path = s.ensureStreamPath(cam, streamID, desiredRecording)
			}
			if path == "" && streamID != "" {
				// Stream no longer exists in DB — clean up orphaned YAML paths and state.
				s.mu.Lock()
				if prev, exists := s.states[sk]; exists {
					delete(s.states, sk)
					// Try to remove any YAML path that was created with the old naming.
					// Check both old format (UUID prefix) and new format (stream name).
					oldPrefix := streamID
					if len(oldPrefix) > 8 {
						oldPrefix = oldPrefix[:8]
					}
					oldPath := cam.MediaMTXPath + "~" + oldPrefix
					s.yamlWriter.RemovePath(oldPath)
					if prev.Recording && s.eventPub != nil {
						s.eventPub.PublishRecordingStopped(cam.Name)
					}
				}
				if sm, ok := s.motionSMs[sk]; ok {
					sm.Stop()
					delete(s.motionSMs, sk)
				}
				s.mu.Unlock()
				continue
			}
			if path == "" {
				continue
			}

			s.mu.Lock()
			prev, exists := s.states[sk]
			changed := !exists || prev.EffectiveMode != mode

			s.states[sk] = &CameraState{
				EffectiveMode: mode,
				Recording:     desiredRecording,
				MotionState:   "idle",
				ActiveRules:   activeIDs,
			}
			if exists && prev.MotionState != "" {
				s.states[sk].MotionState = prev.MotionState
			}

			// Initialize recording health when recording starts.
			if desiredRecording {
				if _, hasHealth := s.healthStates[camID]; !hasHealth {
					s.healthStates[camID] = NewRecordingHealth()
					s.healthStates[camID].Status = HealthHealthy
					s.healthStates[camID].LastSegmentTime = now
				}
			}

			if changed {
				prevMode := ModeOff
				if exists {
					prevMode = prev.EffectiveMode
				}
				s.handleStreamTransition(sk, camID, cam, path, streamID, prevMode, mode, streamRules)
			}
			s.mu.Unlock()

			if changed && path != "" {
				s.queueWrite(path, desiredRecording)
				if s.eventPub != nil {
					if desiredRecording {
						s.eventPub.PublishRecordingStarted(cam.Name)
					} else if exists && prev.Recording {
						s.eventPub.PublishRecordingStopped(cam.Name)
					}
				}
			}

			// Clean up non-default stream paths when mode is off.
			if mode == ModeOff && streamID != "" {
				s.yamlWriter.RemovePath(path)
			}
		}
	}

	// Clean up orphaned sub-stream YAML paths. These are paths containing "~"
	// that don't correspond to any active stream state in the scheduler.
	nvrPaths, nvrErr := s.yamlWriter.GetNVRPaths()
	if nvrErr != nil {
		log.Printf("scheduler: GetNVRPaths error: %v", nvrErr)
	}
	s.mu.Lock()
	activeSubPaths := make(map[string]bool)
	for sk := range s.states {
		if idx := strings.Index(sk, ":"); idx >= 0 {
			camID := sk[:idx]
			if cam, ok := camByID[camID]; ok {
				streamID := sk[idx+1:]
				activeSubPaths[streamPath(cam, streamID)] = true
			}
		}
	}
	s.mu.Unlock()
	log.Printf("scheduler: orphan sweep: %d YAML paths, %d active sub-paths", len(nvrPaths), len(activeSubPaths))
	for _, p := range nvrPaths {
		if strings.Contains(p, "~") {
			log.Printf("scheduler: orphan check: path=%q active=%v", p, activeSubPaths[p])
			if !activeSubPaths[p] {
				log.Printf("scheduler: removing orphaned sub-stream path %q from YAML", p)
				s.yamlWriter.RemovePath(p)
			}
		}
	}

	// Check recording health for stalls. Collect stalled camera IDs under
	// the lock, then handle recovery outside the lock to avoid holding the
	// mutex during DB calls and goroutine spawning.
	type stalledEntry struct {
		camID       string
		shouldRestart bool
		shouldFail    bool
	}
	var stalledCameras []stalledEntry

	s.mu.Lock()
	for camID, h := range s.healthStates {
		st := s.states[camID]
		if st == nil || !st.Recording {
			if h.Status != HealthInactive {
				h.Status = HealthInactive
			}
			continue
		}
		if h.Status == HealthInactive {
			h.Status = HealthHealthy
		}
		if h.CheckStall(now) && h.Status == HealthStalled {
			if h.ShouldRestart(now) {
				h.MarkRestarted(now)
				stalledCameras = append(stalledCameras, stalledEntry{camID: camID, shouldRestart: true})
			} else if h.RestartAttempts >= MaxRestartAttempts {
				h.MarkFailed()
				stalledCameras = append(stalledCameras, stalledEntry{camID: camID, shouldFail: true})
			}
		}
	}
	s.mu.Unlock()

	// Handle recovery actions outside the mutex.
	for _, entry := range stalledCameras {
		cam, err := s.db.GetCamera(entry.camID)
		if err != nil {
			continue
		}
		if entry.shouldRestart {
			log.Printf("scheduler: recording stalled for %s, attempting restart", cam.Name)
			go func(p string) {
				_ = s.yamlWriter.SetPathValue(p, "record", false)
				time.Sleep(2 * time.Second)
				_ = s.yamlWriter.SetPathValue(p, "record", true)
			}(cam.MediaMTXPath)
			if s.eventPub != nil {
				s.eventPub.PublishRecordingStalled(cam.Name)
			}
		} else if entry.shouldFail {
			log.Printf("scheduler: recording recovery failed for %s after %d attempts", cam.Name, MaxRestartAttempts)
			if s.eventPub != nil {
				s.eventPub.PublishRecordingFailed(cam.Name)
			}
		}
	}

	// Run retention cleanup if enough time has passed.
	if time.Since(s.lastRetentionCheck) >= retentionCheckInterval {
		s.runRetentionCleanup(cameras)
		s.lastRetentionCheck = time.Now()
	}

}

// StartMotionTimer starts a timer for the given camera that will auto-close
// the open motion event after the camera's configured timeout. If a timer
// is already running for this camera, it is reset. Call this when motion=true
// is received. Call CancelMotionTimer when motion=false is received.
func (s *Scheduler) StartMotionTimer(cameraID string, timeoutSeconds int) {
	s.motionTimersMu.Lock()
	defer s.motionTimersMu.Unlock()

	// Cancel any existing timer for this camera.
	if existing, ok := s.motionTimers[cameraID]; ok {
		existing.Stop()
	}

	timeout := time.Duration(timeoutSeconds) * time.Second
	if timeout <= 0 {
		timeout = 8 * time.Second
	}

	s.motionTimers[cameraID] = time.AfterFunc(timeout, func() {
		now := time.Now().UTC().Format("2006-01-02T15:04:05.000Z")
		_ = s.db.EndMotionEvent(cameraID, now)

		// Also signal ALL recording state machines for this camera so they start
		// their post-buffer countdown. Without this, recording would continue indefinitely.
		s.mu.Lock()
		for sk, sm := range s.motionSMs {
			if sk == cameraID || strings.HasPrefix(sk, cameraID+":") {
				sm.OnMotion(false)
			}
		}
		s.mu.Unlock()

		s.motionTimersMu.Lock()
		delete(s.motionTimers, cameraID)
		s.motionTimersMu.Unlock()

		log.Printf("scheduler: auto-closed motion event for camera %s after %v timeout", cameraID, timeout)
	})
}

// CancelMotionTimer cancels any pending auto-close timer for the given camera.
// Call this when motion=false is received (the event is closed explicitly).
func (s *Scheduler) CancelMotionTimer(cameraID string) {
	s.motionTimersMu.Lock()
	defer s.motionTimersMu.Unlock()

	if timer, ok := s.motionTimers[cameraID]; ok {
		timer.Stop()
		delete(s.motionTimers, cameraID)
	}
}

// runRetentionCleanup consolidates old detections, applies event-aware recording
// retention, cleans up old motion events, and prunes the audit log.
func (s *Scheduler) runRetentionCleanup(cameras []*db.Camera) {
	now := time.Now().UTC()

	// Step 1: Consolidate detections from closed events (older than 1 hour)
	// into compact JSON summaries on the motion_event row.
	consolidated, err := s.db.ConsolidateClosedEvents(1 * time.Hour)
	if err != nil {
		log.Printf("scheduler: detection consolidation failed: %v", err)
	} else if consolidated > 0 {
		log.Printf("scheduler: consolidated detections for %d events", consolidated)
	}

	// Step 2: Per-camera/stream retention.
	for _, cam := range cameras {
		streams, err := s.db.ListCameraStreams(cam.ID)
		if err != nil {
			log.Printf("scheduler: failed to list streams for camera %s: %v", cam.Name, err)
			continue
		}

		// Per-stream retention: iterate streams with retention configured.
		handledByStream := false
		for _, stream := range streams {
			if stream.RetentionDays <= 0 {
				continue
			}
			handledByStream = true
			noEventCutoff := now.AddDate(0, 0, -stream.RetentionDays)

			if stream.EventRetentionDays > 0 {
				paths, err := s.db.DeleteStreamRecordingsWithoutEvents(cam.ID, stream.ID, noEventCutoff)
				if err != nil {
					log.Printf("scheduler: stream no-event retention FAILED for %s/%s: %v", cam.Name, stream.Name, err)
				} else if len(paths) > 0 {
					removed := removeFiles(paths)
					log.Printf("scheduler: stream no-event retention for %s/%s: deleted %d recordings (%d files)", cam.Name, stream.Name, len(paths), removed)
				}

				eventCutoff := now.AddDate(0, 0, -stream.EventRetentionDays)
				paths, err = s.db.DeleteStreamRecordingsWithEvents(cam.ID, stream.ID, eventCutoff)
				if err != nil {
					log.Printf("scheduler: stream event retention FAILED for %s/%s: %v", cam.Name, stream.Name, err)
				} else if len(paths) > 0 {
					removed := removeFiles(paths)
					log.Printf("scheduler: stream event retention for %s/%s: deleted %d recordings (%d files)", cam.Name, stream.Name, len(paths), removed)
				}
			} else {
				paths, err := s.db.DeleteStreamRecordingsByDateRange(cam.ID, stream.ID, noEventCutoff)
				if err != nil {
					log.Printf("scheduler: stream retention FAILED for %s/%s: %v", cam.Name, stream.Name, err)
				} else if len(paths) > 0 {
					removed := removeFiles(paths)
					log.Printf("scheduler: stream retention for %s/%s: deleted %d recordings (%d files)", cam.Name, stream.Name, len(paths), removed)
				}
			}
		}

		// Camera-level fallback for recordings not covered by stream retention.
		if !handledByStream && cam.RetentionDays > 0 {
			noEventCutoff := now.AddDate(0, 0, -cam.RetentionDays)

			if cam.EventRetentionDays > 0 {
				paths, err := s.db.DeleteRecordingsWithoutEvents(cam.ID, noEventCutoff)
				if err != nil {
					log.Printf("scheduler: no-event retention FAILED for camera %s: %v", cam.Name, err)
				} else if len(paths) > 0 {
					removed := removeFiles(paths)
					log.Printf("scheduler: no-event retention for %s: deleted %d recordings (%d files)", cam.Name, len(paths), removed)
				}

				eventCutoff := now.AddDate(0, 0, -cam.EventRetentionDays)
				paths, err = s.db.DeleteRecordingsWithEvents(cam.ID, eventCutoff)
				if err != nil {
					log.Printf("scheduler: event retention FAILED for camera %s: %v", cam.Name, err)
				} else if len(paths) > 0 {
					removed := removeFiles(paths)
					log.Printf("scheduler: event retention for %s: deleted %d recordings (%d files)", cam.Name, len(paths), removed)
				}
			} else {
				paths, err := s.db.DeleteRecordingsByDateRange(cam.ID, noEventCutoff)
				if err != nil {
					log.Printf("scheduler: retention FAILED for camera %s: %v", cam.Name, err)
				} else if len(paths) > 0 {
					removed := removeFiles(paths)
					log.Printf("scheduler: retention for %s: deleted %d recordings (%d files), cutoff %s", cam.Name, len(paths), removed, noEventCutoff.Format(time.RFC3339))
				}
			}
		}

		// Step 3: Clean old motion events.
		if cam.DetectionRetentionDays > 0 {
			eventCutoff := now.AddDate(0, 0, -cam.DetectionRetentionDays)
			thumbs, n, err := s.db.DeleteMotionEventsBefore(cam.ID, eventCutoff)
			if err != nil {
				log.Printf("scheduler: event data cleanup FAILED for camera %s: %v", cam.Name, err)
			} else if n > 0 {
				removeFiles(thumbs)
				log.Printf("scheduler: event data cleanup for %s: deleted %d events", cam.Name, n)
			}
		}
	}

	// Step 4: Clean audit log entries older than 90 days.
	auditCutoff := now.AddDate(0, 0, -90)
	_ = s.db.DeleteAuditEntriesBefore(auditCutoff)

	// Step 5: Enforce per-camera storage quotas.
	s.enforceQuotas(cameras)
}

// enforceQuotas checks per-camera and global storage quotas, deleting oldest
// recordings when usage exceeds the configured limit. Non-event recordings
// are deleted first, then event recordings, always oldest-first.
func (s *Scheduler) enforceQuotas(cameras []*db.Camera) {
	// Per-camera quota enforcement.
	for _, cam := range cameras {
		if cam.QuotaBytes <= 0 {
			continue
		}
		used, err := s.db.GetCameraStorageUsage(cam.ID)
		if err != nil {
			log.Printf("scheduler: quota check failed for camera %s: %v", cam.Name, err)
			continue
		}
		if used <= cam.QuotaBytes {
			continue
		}

		bytesToFree := used - cam.QuotaBytes
		log.Printf("scheduler: camera %s over quota by %d bytes (used=%d, quota=%d), enforcing cleanup",
			cam.Name, bytesToFree, used, cam.QuotaBytes)

		// First pass: delete oldest non-event recordings.
		paths, freed, err := s.db.DeleteOldestRecordingsWithoutEvents(cam.ID, bytesToFree)
		if err != nil {
			log.Printf("scheduler: quota enforcement (non-event) FAILED for camera %s: %v", cam.Name, err)
		} else if len(paths) > 0 {
			removed := removeFiles(paths)
			log.Printf("scheduler: quota enforcement for %s: deleted %d non-event recordings (%d files, %d bytes freed)",
				cam.Name, len(paths), removed, freed)
		}

		bytesToFree -= freed
		if bytesToFree > 0 {
			// Second pass: delete oldest event recordings.
			paths, freed, err := s.db.DeleteOldestRecordingsWithEvents(cam.ID, bytesToFree)
			if err != nil {
				log.Printf("scheduler: quota enforcement (event) FAILED for camera %s: %v", cam.Name, err)
			} else if len(paths) > 0 {
				removed := removeFiles(paths)
				log.Printf("scheduler: quota enforcement for %s: deleted %d event recordings (%d files, %d bytes freed)",
					cam.Name, len(paths), removed, freed)
			}
		}

		if s.eventPub != nil {
			s.eventPub.PublishRecordingStopped(cam.Name)
		}
	}

	// Global quota enforcement.
	globalQuota, err := s.db.GetStorageQuota("global")
	if err != nil || !globalQuota.Enabled || globalQuota.QuotaBytes <= 0 {
		return
	}

	totalUsed, err := s.db.GetTotalStorageUsage()
	if err != nil {
		log.Printf("scheduler: global quota check failed: %v", err)
		return
	}
	if totalUsed <= globalQuota.QuotaBytes {
		return
	}

	log.Printf("scheduler: global storage over quota by %d bytes (used=%d, quota=%d), enforcing cleanup",
		totalUsed-globalQuota.QuotaBytes, totalUsed, globalQuota.QuotaBytes)

	// Build per-camera usage map and find the camera using the most storage.
	storagePerCamera, err := s.db.GetStoragePerCamera()
	if err != nil {
		log.Printf("scheduler: global quota: failed to get per-camera storage: %v", err)
		return
	}

	// Delete oldest recordings from the largest camera until under quota.
	bytesToFree := totalUsed - globalQuota.QuotaBytes
	for bytesToFree > 0 && len(storagePerCamera) > 0 {
		// Find camera using the most storage.
		maxIdx := 0
		for i, cs := range storagePerCamera {
			if cs.TotalBytes > storagePerCamera[maxIdx].TotalBytes {
				maxIdx = i
			}
		}
		biggestCam := storagePerCamera[maxIdx]
		if biggestCam.TotalBytes <= 0 {
			break
		}

		// Delete non-event recordings first.
		paths, freed, err := s.db.DeleteOldestRecordingsWithoutEvents(biggestCam.CameraID, bytesToFree)
		if err != nil {
			log.Printf("scheduler: global quota enforcement FAILED for camera %s: %v", biggestCam.CameraName, err)
			break
		}
		if len(paths) > 0 {
			removed := removeFiles(paths)
			log.Printf("scheduler: global quota: deleted %d non-event recordings from %s (%d files, %d bytes freed)",
				len(paths), biggestCam.CameraName, removed, freed)
		}

		bytesToFree -= freed
		if bytesToFree > 0 && freed == 0 {
			// No non-event recordings left, try event recordings.
			paths, freed, err = s.db.DeleteOldestRecordingsWithEvents(biggestCam.CameraID, bytesToFree)
			if err != nil {
				log.Printf("scheduler: global quota enforcement (event) FAILED for camera %s: %v", biggestCam.CameraName, err)
				break
			}
			if len(paths) > 0 {
				removed := removeFiles(paths)
				log.Printf("scheduler: global quota: deleted %d event recordings from %s (%d files, %d bytes freed)",
					len(paths), biggestCam.CameraName, removed, freed)
			}
			bytesToFree -= freed
			if freed == 0 {
				// This camera has nothing left to delete, remove it from the list.
				storagePerCamera = append(storagePerCamera[:maxIdx], storagePerCamera[maxIdx+1:]...)
			}
		}
		// Update the camera's usage for the next iteration.
		if maxIdx < len(storagePerCamera) {
			storagePerCamera[maxIdx].TotalBytes -= freed
		}
	}
}

// removeFiles deletes files from disk and returns the count successfully removed.
func removeFiles(paths []string) int {
	removed := 0
	for _, p := range paths {
		if err := os.Remove(p); err == nil {
			removed++
		}
	}
	return removed
}

// startEventPipelineLocked creates and starts an EventSubscriber and MotionSM
// for the given camera. Must be called with s.mu held.
func (s *Scheduler) startEventPipelineLocked(camID string, cam *db.Camera, activeRules []*db.RecordingRule) {
	if cam.ONVIFEndpoint == "" {
		log.Printf("scheduler: camera %s has no ONVIF endpoint, cannot start event subscription", camID)
		return
	}

	// Use the camera's motion_timeout_seconds as the post-event recording buffer.
	postEventSecs := cam.MotionTimeoutSeconds
	if postEventSecs <= 0 {
		postEventSecs = 8
	}
	postEventDuration := time.Duration(postEventSecs) * time.Second

	// Create the motion state machine.
	sm := NewMotionSM(camID, cam.MediaMTXPath, postEventDuration, func(path string, record bool) {
		s.queueWrite(path, record)
		// Update the motion state in the camera state while we have it.
		s.mu.Lock()
		if st, ok := s.states[camID]; ok {
			st.Recording = record
		}
		s.mu.Unlock()
		// Publish recording state change events.
		if s.eventPub != nil {
			if record {
				s.eventPub.PublishRecordingStarted(cam.Name)
			} else {
				s.eventPub.PublishRecordingStopped(cam.Name)
			}
		}
	})
	s.motionSMs[camID] = sm

	// Update the motion state tracker in the background.
	go func() {
		for {
			s.mu.Lock()
			currentSM, ok := s.motionSMs[camID]
			if !ok || currentSM != sm {
				s.mu.Unlock()
				return
			}
			if st, ok := s.states[camID]; ok {
				st.MotionState = sm.State()
			}
			s.mu.Unlock()
			time.Sleep(1 * time.Second)
		}
	}()

	// Create the event subscriber. Wrap the event callback to also publish events
	// and persist motion events in the database.
	eventCallback := func(eventType onvif.DetectedEventType, active bool) {
		switch eventType {
		case onvif.EventMotion:
			// Dispatch to ALL MotionSMs for this camera (default + per-stream).
			s.mu.Lock()
			for sk, msm := range s.motionSMs {
				if sk == cam.ID || strings.HasPrefix(sk, cam.ID+":") {
					msm.OnMotion(active)
				}
			}
			s.mu.Unlock()
			now := time.Now().UTC().Format("2006-01-02T15:04:05.000Z")
			if active {
				s.StartMotionTimer(camID, cam.MotionTimeoutSeconds)
				if !s.db.HasOpenMotionEvent(camID) {
					_ = s.db.InsertMotionEvent(&db.MotionEvent{
						CameraID:  camID,
						StartedAt: now,
					})
					if s.eventPub != nil {
						s.eventPub.PublishMotion(cam.Name)
					}
				}
			} else {
				s.CancelMotionTimer(camID)
				_ = s.db.EndMotionEvent(camID, now)
			}
		case onvif.EventTampering:
			now := time.Now().UTC().Format("2006-01-02T15:04:05.000Z")
			if active {
				_ = s.db.InsertMotionEvent(&db.MotionEvent{
					CameraID:  camID,
					StartedAt: now,
					EventType: "tampering",
				})
				if s.eventPub != nil {
					s.eventPub.PublishTampering(cam.Name)
				}
			} else {
				_ = s.db.EndMotionEvent(camID, now)
			}
		}
	}
	sub, err := onvif.NewEventSubscriber(cam.ONVIFEndpoint, cam.ONVIFUsername, s.decryptPassword(cam.ONVIFPassword), s.callbackURL(camID), eventCallback)
	if err != nil {
		log.Printf("scheduler: create event subscriber for camera %s: %v", camID, err)
		// Clean up all MotionSMs for this camera on failure.
		for sk, msm := range s.motionSMs {
			if sk == camID || strings.HasPrefix(sk, camID+":") {
				msm.Stop()
				delete(s.motionSMs, sk)
			}
		}
		return
	}
	s.eventSubs[camID] = sub
	if s.callbackMgr != nil {
		s.callbackMgr.Register(camID, sub)
	}

	log.Printf("scheduler: starting ONVIF event subscription for camera %s at %s", camID, cam.ONVIFEndpoint)
	go sub.Start(context.Background())
}

// decryptPassword decrypts an ONVIF password from the DB if it was encrypted.
func (s *Scheduler) decryptPassword(encrypted string) string {
	if len(s.encryptionKey) == 0 || !strings.HasPrefix(encrypted, "enc:") {
		return encrypted
	}
	ciphertext, err := base64.StdEncoding.DecodeString(strings.TrimPrefix(encrypted, "enc:"))
	if err != nil {
		return encrypted
	}
	plain, err := crypto.Decrypt(s.encryptionKey, ciphertext)
	if err != nil {
		return encrypted
	}
	return string(plain)
}

// callbackURL builds the webhook URL for a camera.
func (s *Scheduler) callbackURL(cameraID string) string {
	port := strings.TrimPrefix(s.apiAddress, ":")
	localIP := onvif.GetLocalIP()
	return fmt.Sprintf("http://%s:%s/api/nvr/onvif-callback/%s", localIP, port, cameraID)
}

// startMotionAlertSubscription starts an ONVIF event subscription just for
// motion alerts (no recording control). This runs for all ONVIF cameras
// regardless of whether they have "events" recording rules.
// Must be called with s.mu held.
func (s *Scheduler) startMotionAlertSubscription(cam *db.Camera) {
	if cam.ONVIFEndpoint == "" {
		return
	}

	eventCallback := func(eventType onvif.DetectedEventType, active bool) {
		switch eventType {
		case onvif.EventMotion:
			// Dispatch to ALL MotionSMs for this camera (default + per-stream).
			s.mu.Lock()
			for sk, msm := range s.motionSMs {
				if sk == cam.ID || strings.HasPrefix(sk, cam.ID+":") {
					msm.OnMotion(active)
				}
			}
			s.mu.Unlock()

			now := time.Now().UTC().Format("2006-01-02T15:04:05.000Z")
			if active {
				// Reset the auto-close timer on every motion=true (keeps event alive).
				s.StartMotionTimer(cam.ID, cam.MotionTimeoutSeconds)

				// Only create a new DB event if there isn't one already open.
				if s.db.HasOpenMotionEvent(cam.ID) {
					break
				}

				event := &db.MotionEvent{
					CameraID:  cam.ID,
					StartedAt: now,
				}

				// Capture thumbnail in background.
				go func() {
					thumbDir := "./thumbnails"
					password := s.decryptPassword(cam.ONVIFPassword)
					thumbPath, err := onvif.CaptureSnapshot(cam.RTSPURL, cam.ONVIFUsername, password, thumbDir, cam.ID, cam.SnapshotURI)
					if err != nil {
						log.Printf("scheduler: thumbnail capture failed for camera %s: %v", cam.ID, err)
					} else {
						event.ThumbnailPath = thumbPath
					}
					_ = s.db.InsertMotionEvent(event)
				}()

				if s.eventPub != nil {
					s.eventPub.PublishMotion(cam.Name)
				}
			} else {
				// Explicit motion=false: close immediately and cancel timer.
				s.CancelMotionTimer(cam.ID)
				_ = s.db.EndMotionEvent(cam.ID, now)
			}
		case onvif.EventTampering:
			now := time.Now().UTC().Format("2006-01-02T15:04:05.000Z")
			if active {
				_ = s.db.InsertMotionEvent(&db.MotionEvent{
					CameraID:  cam.ID,
					StartedAt: now,
					EventType: "tampering",
				})
				if s.eventPub != nil {
					s.eventPub.PublishTampering(cam.Name)
				}
			} else {
				_ = s.db.EndMotionEvent(cam.ID, now)
			}
		}
	}

	cbURL := s.callbackURL(cam.ID)
	sub, err := onvif.NewEventSubscriber(cam.ONVIFEndpoint, cam.ONVIFUsername, s.decryptPassword(cam.ONVIFPassword), cbURL, eventCallback)
	if err != nil {
		log.Printf("scheduler: create motion alert subscriber for camera %s: %v", cam.ID, err)
		return
	}
	s.eventSubs[cam.ID] = sub
	if s.callbackMgr != nil {
		s.callbackMgr.Register(cam.ID, sub)
	}

	log.Printf("scheduler: starting ONVIF motion alert subscription for camera %s at %s", cam.ID, cam.ONVIFEndpoint)
	go sub.Start(context.Background())
}

// stopEventPipelineLocked stops and removes the EventSubscriber and all
// MotionSMs for the given camera (including per-stream instances).
// Must be called with s.mu held.
func (s *Scheduler) stopEventPipelineLocked(camID string) {
	if sub, ok := s.eventSubs[camID]; ok {
		sub.Stop()
		delete(s.eventSubs, camID)
		if s.callbackMgr != nil {
			s.callbackMgr.Unregister(camID)
		}
	}
	// Stop all MotionSMs for this camera (default + per-stream).
	for sk, sm := range s.motionSMs {
		if sk == camID || strings.HasPrefix(sk, camID+":") {
			sm.Stop()
			delete(s.motionSMs, sk)
		}
	}
}

// ResubscribeCamera tears down any active event subscription for the given
// camera and starts a fresh one using the current credentials from the DB.
// This should be called after credential rotation so that ONVIF event
// subscriptions use the updated credentials.
func (s *Scheduler) ResubscribeCamera(cameraID string) {
	cam, err := s.db.GetCamera(cameraID)
	if err != nil {
		log.Printf("scheduler: resubscribe camera %s: %v", cameraID, err)
		return
	}

	rules, err := s.db.ListRecordingRules(cameraID)
	if err != nil {
		log.Printf("scheduler: resubscribe camera %s: list rules: %v", cameraID, err)
		return
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	// Tear down existing subscription if any.
	s.stopEventPipelineLocked(cameraID)

	// Only resubscribe if the camera has an ONVIF endpoint.
	if cam.ONVIFEndpoint == "" {
		return
	}

	hasEventRule := false
	for _, r := range rules {
		if r.Mode == "events" && r.Enabled {
			hasEventRule = true
			break
		}
	}

	if hasEventRule {
		s.startEventPipelineLocked(cameraID, cam, rules)
	} else {
		// Even without event rules, start the motion alert subscription.
		s.startMotionAlertSubscription(cam)
	}
}

// queueWrite adds a pending YAML write and starts/resets the coalesce timer.
func (s *Scheduler) queueWrite(path string, record bool) {
	s.pendingWritesMu.Lock()
	defer s.pendingWritesMu.Unlock()

	s.pendingWrites[path] = record

	if s.writeTimer != nil {
		s.writeTimer.Stop()
	}
	s.writeTimer = time.AfterFunc(time.Duration(writeCoalesceMs)*time.Millisecond, s.flushWrites)
}

// flushWrites applies all pending YAML writes.
func (s *Scheduler) flushWrites() {
	s.pendingWritesMu.Lock()
	pending := s.pendingWrites
	s.pendingWrites = make(map[string]bool)
	s.pendingWritesMu.Unlock()

	for path, record := range pending {
		if err := s.yamlWriter.SetPathValue(path, "record", record); err != nil {
			log.Printf("scheduler: set record for %s: %v", path, err)
		}
	}
}

// EvaluateRules determines the effective recording mode for a set of rules
// at the given point in time. It returns the mode and the IDs of matching rules.
// Rules that are disabled are skipped.
// Union logic: if any matching rule is "always" -> ModeAlways;
// else if any is "events" -> ModeEvents; else ModeOff.
func EvaluateRules(rules []*db.RecordingRule, now time.Time) (EffectiveMode, []string) {
	var activeIDs []string
	hasAlways := false
	hasEvents := false

	for _, r := range rules {
		if !r.Enabled {
			continue
		}
		if !RuleMatchesTime(r, now) {
			continue
		}
		activeIDs = append(activeIDs, r.ID)
		switch r.Mode {
		case "always":
			hasAlways = true
		case "events":
			hasEvents = true
		}
	}

	if hasAlways {
		return ModeAlways, activeIDs
	}
	if hasEvents {
		return ModeEvents, activeIDs
	}
	return ModeOff, activeIDs
}

// RuleMatchesTime checks whether a single rule matches the given time,
// accounting for cross-midnight schedules. The days field is a JSON array
// of ISO weekday numbers (0=Sunday, 1=Monday, ..., 6=Saturday).
// For cross-midnight rules (start > end), the days array specifies the
// START day:
//   - Today in days AND now >= start -> match (evening portion)
//   - Yesterday in days AND now < end -> match (morning portion)
//
// When start == end, the rule covers 24 hours on matching days.
func RuleMatchesTime(rule *db.RecordingRule, now time.Time) bool {
	days, err := parseDays(rule.Days)
	if err != nil {
		return false
	}

	startMin := parseTimeOfDay(rule.StartTime)
	endMin := parseTimeOfDay(rule.EndTime)
	if startMin < 0 || endMin < 0 {
		return false
	}

	nowMin := now.Hour()*60 + now.Minute()
	today := isoWeekday(now.Weekday())

	// 24-hour coverage: start == end
	if startMin == endMin {
		return dayInSet(today, days)
	}

	// Normal range: start < end (no midnight crossing)
	if startMin < endMin {
		return dayInSet(today, days) && nowMin >= startMin && nowMin < endMin
	}

	// Cross-midnight: start > end
	// Evening portion: today is in days AND now >= start
	if dayInSet(today, days) && nowMin >= startMin {
		return true
	}
	// Morning portion: yesterday is in days AND now < end
	yesterday := isoWeekday(now.Add(-24 * time.Hour).Weekday())
	if dayInSet(yesterday, days) && nowMin < endMin {
		return true
	}

	return false
}

// parseDays parses a JSON array string like "[1,2,3,4,5]" into a set of ints.
func parseDays(s string) (map[int]struct{}, error) {
	s = strings.TrimSpace(s)
	if s == "" {
		return nil, fmt.Errorf("empty days")
	}

	var arr []json.Number
	if err := json.Unmarshal([]byte(s), &arr); err != nil {
		return nil, err
	}

	result := make(map[int]struct{}, len(arr))
	for _, n := range arr {
		v, err := strconv.Atoi(n.String())
		if err != nil {
			return nil, err
		}
		result[v] = struct{}{}
	}
	return result, nil
}

// parseTimeOfDay parses "HH:MM" into minutes since midnight. Returns -1 on error.
func parseTimeOfDay(s string) int {
	parts := strings.Split(s, ":")
	if len(parts) != 2 {
		return -1
	}
	h, err := strconv.Atoi(parts[0])
	if err != nil || h < 0 || h > 23 {
		return -1
	}
	m, err := strconv.Atoi(parts[1])
	if err != nil || m < 0 || m > 59 {
		return -1
	}
	return h*60 + m
}

// isoWeekday converts Go's time.Weekday (Sunday=0) to our ISO weekday
// representation (Sunday=0, Monday=1, ..., Saturday=6).
// Go's time.Weekday already uses 0=Sunday so this is an identity mapping.
func isoWeekday(wd time.Weekday) int {
	return int(wd)
}

// dayInSet checks if a day number is in the set.
func dayInSet(day int, set map[int]struct{}) bool {
	_, ok := set[day]
	return ok
}
