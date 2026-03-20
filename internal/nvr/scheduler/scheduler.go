// Package scheduler evaluates recording rules on a 30-second tick and
// manages the recording state for each camera by writing to the MediaMTX
// YAML configuration.
package scheduler

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/bluenviron/mediamtx/internal/nvr/db"
	"github.com/bluenviron/mediamtx/internal/nvr/onvif"
	"github.com/bluenviron/mediamtx/internal/nvr/yamlwriter"
)

// EventPublisher is an interface for publishing system events from the scheduler.
// This avoids a circular import with the api package.
type EventPublisher interface {
	PublishMotion(cameraName string)
	PublishCameraOffline(cameraName string)
	PublishCameraOnline(cameraName string)
	PublishRecordingStarted(cameraName string)
	PublishRecordingStopped(cameraName string)
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

// Scheduler evaluates recording rules every 30 seconds and applies
// recording state changes to the MediaMTX YAML configuration.
type Scheduler struct {
	db         *db.DB
	yamlWriter *yamlwriter.Writer
	eventPub   EventPublisher

	mu     sync.Mutex
	states map[string]*CameraState // camera ID -> state
	stopCh chan struct{}
	wg     sync.WaitGroup

	motionSMs map[string]*MotionSM             // camera ID -> motion state machine
	eventSubs map[string]*onvif.EventSubscriber // camera ID -> event subscriber

	pendingWrites   map[string]bool // mediamtx path -> desired record state
	pendingWritesMu sync.Mutex
	writeTimer      *time.Timer
}

// New creates a new Scheduler.
func New(database *db.DB, writer *yamlwriter.Writer) *Scheduler {
	return &Scheduler{
		db:            database,
		yamlWriter:    writer,
		states:        make(map[string]*CameraState),
		stopCh:        make(chan struct{}),
		pendingWrites: make(map[string]bool),
		motionSMs:     make(map[string]*MotionSM),
		eventSubs:     make(map[string]*onvif.EventSubscriber),
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
func (s *Scheduler) Stop() {
	close(s.stopCh)
	s.wg.Wait()

	// Stop all event subscribers and motion state machines.
	s.mu.Lock()
	for camID := range s.eventSubs {
		s.stopEventPipelineLocked(camID)
	}
	s.mu.Unlock()

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
}

// RemoveCamera removes tracked state for the given camera ID and stops
// any active event subscriber or motion state machine.
func (s *Scheduler) RemoveCamera(cameraID string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.stopEventPipelineLocked(cameraID)
	delete(s.states, cameraID)
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

	ticker := time.NewTicker(evalInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			s.evaluate()
		case <-s.stopCh:
			return
		}
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

	// Evaluate each camera that has rules or had state previously.
	s.mu.Lock()
	// Collect all camera IDs we need to evaluate: those with rules + those with existing state.
	evalCameras := make(map[string]struct{})
	for camID := range rulesByCam {
		evalCameras[camID] = struct{}{}
	}
	for camID := range s.states {
		evalCameras[camID] = struct{}{}
	}
	s.mu.Unlock()

	for camID := range evalCameras {
		cam, ok := camByID[camID]
		if !ok {
			continue
		}

		camRules := rulesByCam[camID]
		mode, activeIDs := EvaluateRules(camRules, now)

		// Determine desired recording state.
		// ModeAlways -> record: true
		// ModeEvents -> record: false (motion trigger will handle it later)
		// ModeOff    -> record: false
		desiredRecording := mode == ModeAlways

		s.mu.Lock()
		prev, exists := s.states[camID]
		changed := !exists || prev.EffectiveMode != mode

		s.states[camID] = &CameraState{
			EffectiveMode: mode,
			Recording:     desiredRecording,
			MotionState:   "idle",
			ActiveRules:   activeIDs,
		}
		// Preserve motion state from previous if it existed.
		if exists && prev.MotionState != "" {
			s.states[camID].MotionState = prev.MotionState
		}

		// Handle event pipeline transitions when mode changes.
		if changed {
			prevMode := ModeOff
			if exists {
				prevMode = prev.EffectiveMode
			}
			s.handleEventPipelineTransitionLocked(camID, cam, prevMode, mode, camRules)
		}
		s.mu.Unlock()

		if changed && cam.MediaMTXPath != "" {
			s.queueWrite(cam.MediaMTXPath, desiredRecording)
			// Publish recording state change events for always-mode transitions.
			if s.eventPub != nil {
				if desiredRecording {
					s.eventPub.PublishRecordingStarted(cam.Name)
				} else if exists && prev.Recording {
					s.eventPub.PublishRecordingStopped(cam.Name)
				}
			}
		}
	}
}

// handleEventPipelineTransitionLocked manages the event subscriber and motion
// state machine lifecycle when a camera's effective mode changes.
// Must be called with s.mu held.
func (s *Scheduler) handleEventPipelineTransitionLocked(
	camID string,
	cam *db.Camera,
	prevMode, newMode EffectiveMode,
	activeRules []*db.RecordingRule,
) {
	// Transitioning away from events -> stop pipeline
	if prevMode == ModeEvents && newMode != ModeEvents {
		s.stopEventPipelineLocked(camID)
		return
	}

	// Transitioning to events -> start pipeline
	if newMode == ModeEvents && prevMode != ModeEvents {
		s.startEventPipelineLocked(camID, cam, activeRules)
	}
}

// startEventPipelineLocked creates and starts an EventSubscriber and MotionSM
// for the given camera. Must be called with s.mu held.
func (s *Scheduler) startEventPipelineLocked(camID string, cam *db.Camera, activeRules []*db.RecordingRule) {
	if cam.ONVIFEndpoint == "" {
		log.Printf("scheduler: camera %s has no ONVIF endpoint, cannot start event subscription", camID)
		return
	}

	// Compute max PostEventSeconds from active rules.
	maxPostEvent := 30 // default 30 seconds
	for _, r := range activeRules {
		if r.Mode == "events" && r.PostEventSeconds > maxPostEvent {
			maxPostEvent = r.PostEventSeconds
		}
	}
	postEventDuration := time.Duration(maxPostEvent) * time.Second

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

	// Create the event subscriber. Wrap the motion callback to also publish events.
	motionCallback := func(detected bool) {
		sm.OnMotion(detected)
		if detected && s.eventPub != nil {
			s.eventPub.PublishMotion(cam.Name)
		}
	}
	sub, err := onvif.NewEventSubscriber(cam.ONVIFEndpoint, cam.ONVIFUsername, cam.ONVIFPassword, motionCallback)
	if err != nil {
		log.Printf("scheduler: create event subscriber for camera %s: %v", camID, err)
		delete(s.motionSMs, camID)
		return
	}
	s.eventSubs[camID] = sub

	log.Printf("scheduler: starting ONVIF event subscription for camera %s at %s", camID, cam.ONVIFEndpoint)
	go sub.Start(context.Background())
}

// stopEventPipelineLocked stops and removes the EventSubscriber and MotionSM
// for the given camera. Must be called with s.mu held.
func (s *Scheduler) stopEventPipelineLocked(camID string) {
	if sub, ok := s.eventSubs[camID]; ok {
		sub.Stop()
		delete(s.eventSubs, camID)
	}
	if sm, ok := s.motionSMs[camID]; ok {
		sm.Stop()
		delete(s.motionSMs, camID)
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
