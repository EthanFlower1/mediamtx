// Package scheduler evaluates recording rules on a 30-second tick and
// manages the recording state for each camera by writing to the MediaMTX
// YAML configuration.
package scheduler

import (
	"encoding/json"
	"fmt"
	"log"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/bluenviron/mediamtx/internal/nvr/db"
	"github.com/bluenviron/mediamtx/internal/nvr/yamlwriter"
)

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
	evalInterval     = 30 * time.Second
	startupDelay     = 5 * time.Second
	writeCoalesceMs  = 500
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

	mu     sync.Mutex
	states map[string]*CameraState // camera ID -> state
	stopCh chan struct{}
	wg     sync.WaitGroup

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
	}
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

// RemoveCamera removes tracked state for the given camera ID.
func (s *Scheduler) RemoveCamera(cameraID string) {
	s.mu.Lock()
	defer s.mu.Unlock()
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
		s.mu.Unlock()

		if changed && cam.MediaMTXPath != "" {
			s.queueWrite(cam.MediaMTXPath, desiredRecording)
		}
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
