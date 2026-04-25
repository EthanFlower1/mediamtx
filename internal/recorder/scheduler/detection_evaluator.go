package scheduler

import (
	"context"
	"log"
	"sync"
	"time"

	db "github.com/bluenviron/mediamtx/internal/recorder/db"
)

const detectionEvalInterval = 30 * time.Second

// DetectionPipelineController is the interface the evaluator uses to start/stop
// AI detection pipelines. Implemented by NVR.
type DetectionPipelineController interface {
	StartDetectionPipeline(cameraID string)
	StopDetectionPipeline(cameraID string)
	IsDetectionPipelineRunning(cameraID string) bool
}

// DetectionEvaluator periodically checks detection schedules and starts/stops
// AI pipelines accordingly. It runs on a 30-second tick, mirroring the
// recording scheduler's evaluation loop.
type DetectionEvaluator struct {
	db         *db.DB
	controller DetectionPipelineController

	mu       sync.Mutex
	// cameraID -> whether detection was active at last evaluation
	lastState map[string]bool
	// cameraID -> active schedule ID
	activeSchedules map[string]string

	cancel context.CancelFunc
	wg     sync.WaitGroup
}

// NewDetectionEvaluator creates a new evaluator. Call Start() to begin the
// evaluation loop.
func NewDetectionEvaluator(database *db.DB, controller DetectionPipelineController) *DetectionEvaluator {
	return &DetectionEvaluator{
		db:              database,
		controller:      controller,
		lastState:       make(map[string]bool),
		activeSchedules: make(map[string]string),
	}
}

// Start launches the background evaluation goroutine.
func (e *DetectionEvaluator) Start() {
	ctx, cancel := context.WithCancel(context.Background())
	e.cancel = cancel

	e.wg.Add(1)
	go e.run(ctx)
}

// Stop signals the evaluator to stop and waits for it to exit.
func (e *DetectionEvaluator) Stop() {
	if e.cancel != nil {
		e.cancel()
	}
	e.wg.Wait()
}

// Evaluate runs a single evaluation cycle immediately. Useful for triggering
// re-evaluation after a schedule change without waiting for the next tick.
func (e *DetectionEvaluator) Evaluate() {
	e.evaluate(time.Now())
}

// Status returns the current detection schedule status for all cameras that
// have schedules configured.
func (e *DetectionEvaluator) Status() map[string]*DetectionScheduleStatus {
	e.mu.Lock()
	defer e.mu.Unlock()

	result := make(map[string]*DetectionScheduleStatus, len(e.lastState))
	for camID, active := range e.lastState {
		result[camID] = &DetectionScheduleStatus{
			CameraID:         camID,
			DetectionActive:  active,
			ActiveScheduleID: e.activeSchedules[camID],
		}
	}
	return result
}

// CameraStatus returns detection schedule status for a single camera.
func (e *DetectionEvaluator) CameraStatus(cameraID string) *DetectionScheduleStatus {
	e.mu.Lock()
	defer e.mu.Unlock()

	active, exists := e.lastState[cameraID]
	if !exists {
		return &DetectionScheduleStatus{
			CameraID:        cameraID,
			DetectionActive: false,
		}
	}
	return &DetectionScheduleStatus{
		CameraID:         cameraID,
		DetectionActive:  active,
		ActiveScheduleID: e.activeSchedules[cameraID],
	}
}

func (e *DetectionEvaluator) run(ctx context.Context) {
	defer e.wg.Done()

	// Initial evaluation after a short startup delay.
	select {
	case <-time.After(3 * time.Second):
		e.evaluate(time.Now())
	case <-ctx.Done():
		return
	}

	ticker := time.NewTicker(detectionEvalInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case t := <-ticker.C:
			e.evaluate(t)
		}
	}
}

func (e *DetectionEvaluator) evaluate(now time.Time) {
	// Load all enabled schedules.
	allSchedules, err := e.db.ListAllDetectionSchedules()
	if err != nil {
		log.Printf("[detection-scheduler] failed to load schedules: %v", err)
		return
	}

	// Group by camera.
	byCam := make(map[string][]*db.DetectionSchedule)
	for _, s := range allSchedules {
		byCam[s.CameraID] = append(byCam[s.CameraID], s)
	}

	// Also load cameras with ai_enabled to know which cameras support detection.
	cameras, err := e.db.ListCameras()
	if err != nil {
		log.Printf("[detection-scheduler] failed to list cameras: %v", err)
		return
	}

	aiCameras := make(map[string]bool)
	for _, cam := range cameras {
		if cam.AIEnabled {
			aiCameras[cam.ID] = true
		}
	}

	e.mu.Lock()
	defer e.mu.Unlock()

	// Evaluate each camera that has AI enabled.
	for camID := range aiCameras {
		schedules := byCam[camID]
		wasActive := e.lastState[camID]

		if len(schedules) == 0 {
			// No schedule configured — detection follows ai_enabled (always on).
			if !wasActive {
				e.lastState[camID] = true
				e.activeSchedules[camID] = ""
				if !e.controller.IsDetectionPipelineRunning(camID) {
					log.Printf("[detection-scheduler] starting detection for %s (no schedule, ai_enabled)", camID)
					e.controller.StartDetectionPipeline(camID)
				}
			}
			continue
		}

		shouldBeActive, matchID := EvaluateDetectionSchedules(schedules, now)

		if shouldBeActive && !wasActive {
			log.Printf("[detection-scheduler] starting detection for %s (schedule %s)", camID, matchID)
			e.controller.StartDetectionPipeline(camID)
			e.lastState[camID] = true
			e.activeSchedules[camID] = matchID
		} else if !shouldBeActive && wasActive {
			log.Printf("[detection-scheduler] stopping detection for %s", camID)
			e.controller.StopDetectionPipeline(camID)
			e.lastState[camID] = false
			e.activeSchedules[camID] = ""
		} else if shouldBeActive {
			// Still active, update matching schedule ID in case it changed.
			e.activeSchedules[camID] = matchID
		}
	}

	// Stop detection for cameras that had AI disabled but still have state.
	for camID, wasActive := range e.lastState {
		if !aiCameras[camID] && wasActive {
			log.Printf("[detection-scheduler] stopping detection for %s (ai disabled)", camID)
			e.controller.StopDetectionPipeline(camID)
			e.lastState[camID] = false
			e.activeSchedules[camID] = ""
		}
	}
}
