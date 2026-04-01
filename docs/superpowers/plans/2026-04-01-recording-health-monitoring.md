# Recording Health Monitoring Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Detect stalled recordings per camera, attempt automatic recovery, and expose health via API and SSE events.

**Architecture:** Add a `RecordingHealth` struct to the scheduler, updated by `OnSegmentComplete` callbacks. The scheduler's 30-second tick checks for stalls (no segment for 30s while recording expected), attempts recovery (toggle record off/on via YAML writer, up to 3 retries with exponential backoff), and publishes SSE events on transitions. A new API endpoint exposes per-camera health, and camera responses include a `recording_health` field.

**Tech Stack:** Go, Gin, SQLite, testify

---

### Task 1: Add RecordingHealth struct and scheduler fields

**Files:**

- Create: `internal/nvr/scheduler/health.go`
- Modify: `internal/nvr/scheduler/scheduler.go:68-109`
- Test: `internal/nvr/scheduler/health_test.go`

- [ ] **Step 1: Write the failing test**

Create `internal/nvr/scheduler/health_test.go`:

```go
package scheduler

import (
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestNewRecordingHealth(t *testing.T) {
	h := NewRecordingHealth()
	require.Equal(t, HealthInactive, h.Status)
	require.True(t, h.LastSegmentTime.IsZero())
	require.Equal(t, 0, h.RestartAttempts)
}

func TestRecordingHealth_RecordSegment(t *testing.T) {
	h := NewRecordingHealth()
	h.Status = HealthHealthy
	now := time.Now()
	h.RecordSegment(now)
	require.Equal(t, HealthHealthy, h.Status)
	require.Equal(t, now, h.LastSegmentTime)
	require.Equal(t, 0, h.RestartAttempts)
}

func TestRecordingHealth_RecordSegmentClearsStall(t *testing.T) {
	h := NewRecordingHealth()
	h.Status = HealthStalled
	h.RestartAttempts = 2
	h.StallDetectedAt = time.Now().Add(-time.Minute)
	now := time.Now()
	h.RecordSegment(now)
	require.Equal(t, HealthHealthy, h.Status)
	require.Equal(t, now, h.LastSegmentTime)
	require.Equal(t, 0, h.RestartAttempts)
	require.True(t, h.StallDetectedAt.IsZero())
}

func TestRecordingHealth_RecordSegmentClearsFailed(t *testing.T) {
	h := NewRecordingHealth()
	h.Status = HealthFailed
	h.RestartAttempts = 3
	now := time.Now()
	h.RecordSegment(now)
	require.Equal(t, HealthHealthy, h.Status)
	require.Equal(t, 0, h.RestartAttempts)
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `cd /Users/ethanflower/personal_projects/mediamtx && go test ./internal/nvr/scheduler/ -run TestNewRecordingHealth -v`
Expected: FAIL — `NewRecordingHealth` not defined

- [ ] **Step 3: Write the RecordingHealth implementation**

Create `internal/nvr/scheduler/health.go`:

```go
package scheduler

import "time"

// Health status constants.
const (
	HealthInactive = "inactive"
	HealthHealthy  = "healthy"
	HealthStalled  = "stalled"
	HealthFailed   = "failed"
)

const (
	// StallThreshold is how long without a segment before a recording is stalled.
	StallThreshold = 30 * time.Second

	// MaxRestartAttempts is the number of recovery attempts before giving up.
	MaxRestartAttempts = 3
)

// backoffDuration returns the backoff delay for the given attempt (0-indexed).
// Attempts: 5s, 15s, 45s.
func backoffDuration(attempt int) time.Duration {
	switch attempt {
	case 0:
		return 5 * time.Second
	case 1:
		return 15 * time.Second
	default:
		return 45 * time.Second
	}
}

// RecordingHealth tracks the health of a camera's recording pipeline.
type RecordingHealth struct {
	Status          string
	LastSegmentTime time.Time
	StallDetectedAt time.Time
	RestartAttempts int
	LastRestartAt   time.Time
	LastError       string
}

// NewRecordingHealth returns a RecordingHealth in the inactive state.
func NewRecordingHealth() *RecordingHealth {
	return &RecordingHealth{
		Status: HealthInactive,
	}
}

// RecordSegment updates health when a new segment is received.
// Clears any stall/failed state and resets restart attempts.
func (h *RecordingHealth) RecordSegment(t time.Time) {
	h.LastSegmentTime = t
	h.RestartAttempts = 0
	h.StallDetectedAt = time.Time{}
	h.LastError = ""
	if h.Status == HealthStalled || h.Status == HealthFailed || h.Status == HealthHealthy {
		h.Status = HealthHealthy
	}
}
```

- [ ] **Step 4: Run tests to verify they pass**

Run: `cd /Users/ethanflower/personal_projects/mediamtx && go test ./internal/nvr/scheduler/ -run TestRecordingHealth -v`
Expected: PASS

- [ ] **Step 5: Add health map to Scheduler struct**

In `internal/nvr/scheduler/scheduler.go`, add the `healthStates` field to the `Scheduler` struct (after line 91, before the closing brace):

```go
	healthStates map[string]*RecordingHealth // camera ID -> recording health
```

In the `New()` function (after line 107, `motionTimers` init), add:

```go
		healthStates: make(map[string]*RecordingHealth),
```

- [ ] **Step 6: Run all scheduler tests**

Run: `cd /Users/ethanflower/personal_projects/mediamtx && go test ./internal/nvr/scheduler/ -v`
Expected: PASS

- [ ] **Step 7: Commit**

```bash
git add internal/nvr/scheduler/health.go internal/nvr/scheduler/health_test.go internal/nvr/scheduler/scheduler.go
git commit -m "feat(scheduler): add RecordingHealth struct and scheduler fields (KAI-9)"
```

---

### Task 2: Add stall detection to the scheduler evaluation loop

**Files:**

- Modify: `internal/nvr/scheduler/scheduler.go:320-519` (the `evaluate()` method)
- Modify: `internal/nvr/scheduler/health.go`
- Test: `internal/nvr/scheduler/health_test.go`

- [ ] **Step 1: Write the failing test for stall detection**

Add to `internal/nvr/scheduler/health_test.go`:

```go
func TestRecordingHealth_CheckStall_Healthy(t *testing.T) {
	h := NewRecordingHealth()
	h.Status = HealthHealthy
	h.LastSegmentTime = time.Now() // segment just arrived
	stalled := h.CheckStall(time.Now())
	require.False(t, stalled)
	require.Equal(t, HealthHealthy, h.Status)
}

func TestRecordingHealth_CheckStall_Stalled(t *testing.T) {
	h := NewRecordingHealth()
	h.Status = HealthHealthy
	h.LastSegmentTime = time.Now().Add(-40 * time.Second) // 40s ago
	stalled := h.CheckStall(time.Now())
	require.True(t, stalled)
	require.Equal(t, HealthStalled, h.Status)
	require.False(t, h.StallDetectedAt.IsZero())
	require.Equal(t, "no segment received for 30s", h.LastError)
}

func TestRecordingHealth_CheckStall_AlreadyStalled(t *testing.T) {
	h := NewRecordingHealth()
	h.Status = HealthStalled
	h.LastSegmentTime = time.Now().Add(-60 * time.Second)
	h.StallDetectedAt = time.Now().Add(-30 * time.Second)
	stalled := h.CheckStall(time.Now())
	require.True(t, stalled)
	require.Equal(t, HealthStalled, h.Status) // stays stalled
}

func TestRecordingHealth_CheckStall_Failed(t *testing.T) {
	h := NewRecordingHealth()
	h.Status = HealthFailed
	h.LastSegmentTime = time.Now().Add(-120 * time.Second)
	stalled := h.CheckStall(time.Now())
	require.False(t, stalled) // failed cameras don't re-trigger stall
}

func TestRecordingHealth_CheckStall_Inactive(t *testing.T) {
	h := NewRecordingHealth()
	h.Status = HealthInactive
	stalled := h.CheckStall(time.Now())
	require.False(t, stalled) // inactive cameras don't stall
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `cd /Users/ethanflower/personal_projects/mediamtx && go test ./internal/nvr/scheduler/ -run TestRecordingHealth_CheckStall -v`
Expected: FAIL — `CheckStall` not defined

- [ ] **Step 3: Implement CheckStall**

Add to `internal/nvr/scheduler/health.go`:

```go
// CheckStall checks whether the recording has stalled. Returns true if a
// new stall was detected (transition from healthy to stalled). Does nothing
// for inactive or already-failed cameras.
func (h *RecordingHealth) CheckStall(now time.Time) bool {
	if h.Status != HealthHealthy && h.Status != HealthStalled {
		return false
	}
	if h.LastSegmentTime.IsZero() {
		return false
	}
	if now.Sub(h.LastSegmentTime) <= StallThreshold {
		if h.Status == HealthStalled {
			// Recovered via time check (shouldn't happen normally — RecordSegment handles this)
			h.Status = HealthHealthy
			h.StallDetectedAt = time.Time{}
			h.LastError = ""
		}
		return false
	}

	// Stalled.
	if h.Status == HealthHealthy {
		h.Status = HealthStalled
		h.StallDetectedAt = now
		h.LastError = "no segment received for 30s"
		return true // new stall
	}
	return true // still stalled
}
```

- [ ] **Step 4: Run tests to verify they pass**

Run: `cd /Users/ethanflower/personal_projects/mediamtx && go test ./internal/nvr/scheduler/ -run TestRecordingHealth_CheckStall -v`
Expected: PASS

- [ ] **Step 5: Commit**

```bash
git add internal/nvr/scheduler/health.go internal/nvr/scheduler/health_test.go
git commit -m "feat(scheduler): add stall detection to RecordingHealth (KAI-9)"
```

---

### Task 3: Add recovery logic with exponential backoff

**Files:**

- Modify: `internal/nvr/scheduler/health.go`
- Test: `internal/nvr/scheduler/health_test.go`

- [ ] **Step 1: Write the failing tests**

Add to `internal/nvr/scheduler/health_test.go`:

```go
func TestRecordingHealth_ShouldRestart_FirstAttempt(t *testing.T) {
	h := NewRecordingHealth()
	h.Status = HealthStalled
	h.StallDetectedAt = time.Now()
	h.RestartAttempts = 0
	should := h.ShouldRestart(time.Now())
	require.True(t, should)
}

func TestRecordingHealth_ShouldRestart_BackoffNotElapsed(t *testing.T) {
	h := NewRecordingHealth()
	h.Status = HealthStalled
	h.RestartAttempts = 1
	h.LastRestartAt = time.Now().Add(-3 * time.Second) // only 3s ago, need 15s
	should := h.ShouldRestart(time.Now())
	require.False(t, should)
}

func TestRecordingHealth_ShouldRestart_BackoffElapsed(t *testing.T) {
	h := NewRecordingHealth()
	h.Status = HealthStalled
	h.RestartAttempts = 1
	h.LastRestartAt = time.Now().Add(-20 * time.Second) // 20s ago, need 15s
	should := h.ShouldRestart(time.Now())
	require.True(t, should)
}

func TestRecordingHealth_ShouldRestart_MaxReached(t *testing.T) {
	h := NewRecordingHealth()
	h.Status = HealthStalled
	h.RestartAttempts = 3
	should := h.ShouldRestart(time.Now())
	require.False(t, should)
}

func TestRecordingHealth_MarkRestarted(t *testing.T) {
	h := NewRecordingHealth()
	h.Status = HealthStalled
	h.RestartAttempts = 0
	now := time.Now()
	h.MarkRestarted(now)
	require.Equal(t, 1, h.RestartAttempts)
	require.Equal(t, now, h.LastRestartAt)
	require.Equal(t, HealthStalled, h.Status)
}

func TestRecordingHealth_MarkFailed(t *testing.T) {
	h := NewRecordingHealth()
	h.Status = HealthStalled
	h.RestartAttempts = 3
	h.MarkFailed()
	require.Equal(t, HealthFailed, h.Status)
	require.Equal(t, "recovery failed after 3 attempts", h.LastError)
}

func TestBackoffDuration(t *testing.T) {
	require.Equal(t, 5*time.Second, backoffDuration(0))
	require.Equal(t, 15*time.Second, backoffDuration(1))
	require.Equal(t, 45*time.Second, backoffDuration(2))
	require.Equal(t, 45*time.Second, backoffDuration(5)) // capped
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `cd /Users/ethanflower/personal_projects/mediamtx && go test ./internal/nvr/scheduler/ -run "TestRecordingHealth_Should|TestRecordingHealth_Mark|TestBackoff" -v`
Expected: FAIL — `ShouldRestart`, `MarkRestarted`, `MarkFailed` not defined

- [ ] **Step 3: Implement recovery methods**

Add to `internal/nvr/scheduler/health.go`:

```go
// ShouldRestart returns true if a restart attempt should be made now.
// Returns false if max attempts reached or backoff period hasn't elapsed.
func (h *RecordingHealth) ShouldRestart(now time.Time) bool {
	if h.Status != HealthStalled {
		return false
	}
	if h.RestartAttempts >= MaxRestartAttempts {
		return false
	}
	if h.RestartAttempts > 0 {
		backoff := backoffDuration(h.RestartAttempts - 1)
		if now.Sub(h.LastRestartAt) < backoff {
			return false
		}
	}
	return true
}

// MarkRestarted records that a restart attempt was made.
func (h *RecordingHealth) MarkRestarted(now time.Time) {
	h.RestartAttempts++
	h.LastRestartAt = now
}

// MarkFailed transitions the health to failed state.
func (h *RecordingHealth) MarkFailed() {
	h.Status = HealthFailed
	h.LastError = fmt.Sprintf("recovery failed after %d attempts", MaxRestartAttempts)
}
```

Also add `"fmt"` to the imports in `health.go`.

- [ ] **Step 4: Run tests to verify they pass**

Run: `cd /Users/ethanflower/personal_projects/mediamtx && go test ./internal/nvr/scheduler/ -run "TestRecordingHealth_Should|TestRecordingHealth_Mark|TestBackoff" -v`
Expected: PASS

- [ ] **Step 5: Commit**

```bash
git add internal/nvr/scheduler/health.go internal/nvr/scheduler/health_test.go
git commit -m "feat(scheduler): add recovery logic with exponential backoff (KAI-9)"
```

---

### Task 4: Wire segment notifications and stall checks into the scheduler

**Files:**

- Modify: `internal/nvr/scheduler/scheduler.go`
- Modify: `internal/nvr/scheduler/health.go`

- [ ] **Step 1: Add NotifySegment method to the scheduler**

Add to `internal/nvr/scheduler/scheduler.go` (after the `RemoveCamera` method, around line 176):

```go
// NotifySegment is called when a recording segment completes for the given
// media path. It updates the recording health for the corresponding camera.
func (s *Scheduler) NotifySegment(mediaPath string) {
	s.mu.Lock()
	defer s.mu.Unlock()

	// Find the camera ID for this media path by checking state keys.
	// The path might be the main camera path or a sub-stream path (contains "~").
	for sk, state := range s.states {
		if !state.Recording {
			continue
		}
		camID := sk
		if idx := strings.Index(sk, ":"); idx >= 0 {
			camID = sk[:idx]
		}
		h, ok := s.healthStates[camID]
		if !ok {
			h = NewRecordingHealth()
			s.healthStates[camID] = h
		}
		h.RecordSegment(time.Now())
		return
	}
}
```

This is a simplified version. We'll refine it in step 3 to properly match paths.

- [ ] **Step 2: Add NotifySegmentForCamera method (path-independent)**

Replace the `NotifySegment` method with a camera-ID-based version that's easier to wire:

```go
// NotifySegmentForCamera is called when a recording segment completes for the
// given camera. It updates the recording health state.
func (s *Scheduler) NotifySegmentForCamera(cameraID string) {
	s.mu.Lock()
	defer s.mu.Unlock()

	h, ok := s.healthStates[cameraID]
	if !ok {
		h = NewRecordingHealth()
		s.healthStates[cameraID] = h
	}
	h.RecordSegment(time.Now())

	if h.Status == HealthHealthy && s.eventPub != nil {
		// If we transitioned from stalled/failed to healthy, publish recovery event.
		// The RecordSegment call already set status to healthy, but we need to
		// check what it was before. Use a flag approach instead.
	}
}
```

Actually, let's handle the recovery event properly. Update `RecordSegment` in `health.go` to return the previous status:

In `internal/nvr/scheduler/health.go`, change `RecordSegment` signature:

```go
// RecordSegment updates health when a new segment is received.
// Returns the previous status so callers can detect recovery transitions.
func (h *RecordingHealth) RecordSegment(t time.Time) (prevStatus string) {
	prevStatus = h.Status
	h.LastSegmentTime = t
	h.RestartAttempts = 0
	h.StallDetectedAt = time.Time{}
	h.LastRestartAt = time.Time{}
	h.LastError = ""
	if h.Status == HealthStalled || h.Status == HealthFailed || h.Status == HealthHealthy {
		h.Status = HealthHealthy
	}
	return prevStatus
}
```

Then the scheduler method becomes:

```go
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
			// Find camera name for the event.
			cam, err := s.db.GetCamera(cameraID)
			if err == nil {
				s.eventPub.PublishRecordingRecovered(cam.Name)
			}
		}
	}
}
```

- [ ] **Step 3: Update the existing RecordSegment tests**

Update the tests in `health_test.go` to use the new return value:

```go
func TestRecordingHealth_RecordSegment(t *testing.T) {
	h := NewRecordingHealth()
	h.Status = HealthHealthy
	now := time.Now()
	prev := h.RecordSegment(now)
	require.Equal(t, HealthHealthy, prev)
	require.Equal(t, HealthHealthy, h.Status)
	require.Equal(t, now, h.LastSegmentTime)
	require.Equal(t, 0, h.RestartAttempts)
}

func TestRecordingHealth_RecordSegmentClearsStall(t *testing.T) {
	h := NewRecordingHealth()
	h.Status = HealthStalled
	h.RestartAttempts = 2
	h.StallDetectedAt = time.Now().Add(-time.Minute)
	now := time.Now()
	prev := h.RecordSegment(now)
	require.Equal(t, HealthStalled, prev)
	require.Equal(t, HealthHealthy, h.Status)
	require.Equal(t, now, h.LastSegmentTime)
	require.Equal(t, 0, h.RestartAttempts)
	require.True(t, h.StallDetectedAt.IsZero())
}

func TestRecordingHealth_RecordSegmentClearsFailed(t *testing.T) {
	h := NewRecordingHealth()
	h.Status = HealthFailed
	h.RestartAttempts = 3
	now := time.Now()
	prev := h.RecordSegment(now)
	require.Equal(t, HealthFailed, prev)
	require.Equal(t, HealthHealthy, h.Status)
	require.Equal(t, 0, h.RestartAttempts)
}
```

- [ ] **Step 4: Add health event methods to EventPublisher interface**

In `internal/nvr/scheduler/scheduler.go`, add three methods to the `EventPublisher` interface (lines 26-33):

```go
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
```

- [ ] **Step 5: Add stall check to the evaluate() method**

At the end of the `evaluate()` method in `scheduler.go` (before the retention cleanup block at line 513), add the health check loop:

```go
	// Check recording health for stalls and attempt recovery.
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
			cam, err := s.db.GetCamera(camID)
			if err != nil {
				continue
			}
			if h.ShouldRestart(now) {
				log.Printf("scheduler: recording stalled for %s, attempting restart (attempt %d)", cam.Name, h.RestartAttempts+1)
				h.MarkRestarted(now)
				// Toggle recording off then on.
				go func(path string) {
					_ = s.yamlWriter.SetPathValue(path, "record", false)
					time.Sleep(2 * time.Second)
					_ = s.yamlWriter.SetPathValue(path, "record", true)
				}(cam.MediaMTXPath)
				if s.eventPub != nil {
					s.eventPub.PublishRecordingStalled(cam.Name)
				}
			} else if h.RestartAttempts >= MaxRestartAttempts && h.Status == HealthStalled {
				h.MarkFailed()
				log.Printf("scheduler: recording recovery failed for %s after %d attempts", cam.Name, MaxRestartAttempts)
				if s.eventPub != nil {
					s.eventPub.PublishRecordingFailed(cam.Name)
				}
			}
		}
	}
	s.mu.Unlock()
```

- [ ] **Step 6: Initialize health state when recording starts**

In the `evaluate()` method, after the block that sets `s.states[sk]` (around line 447-454), add health state initialization:

```go
			// Initialize recording health when recording starts.
			if desiredRecording {
				if _, hasHealth := s.healthStates[camID]; !hasHealth {
					s.healthStates[camID] = NewRecordingHealth()
					s.healthStates[camID].Status = HealthHealthy
					s.healthStates[camID].LastSegmentTime = now
				}
			}
```

- [ ] **Step 7: Run all scheduler tests**

Run: `cd /Users/ethanflower/personal_projects/mediamtx && go test ./internal/nvr/scheduler/ -v`
Expected: PASS

- [ ] **Step 8: Commit**

```bash
git add internal/nvr/scheduler/scheduler.go internal/nvr/scheduler/health.go internal/nvr/scheduler/health_test.go
git commit -m "feat(scheduler): wire stall detection and recovery into evaluation loop (KAI-9)"
```

---

### Task 5: Add health event methods to EventBroadcaster

**Files:**

- Modify: `internal/nvr/api/events.go`

- [ ] **Step 1: Add the three new Publish methods**

Add to `internal/nvr/api/events.go` (after `PublishRecordingStopped`, around line 173):

```go
// PublishRecordingStalled publishes a recording_stalled event.
func (b *EventBroadcaster) PublishRecordingStalled(cameraName string) {
	b.Publish(Event{
		Type:    "recording_stalled",
		Camera:  cameraName,
		Message: fmt.Sprintf("Recording stalled on %s — attempting recovery", cameraName),
	})
}

// PublishRecordingRecovered publishes a recording_recovered event.
func (b *EventBroadcaster) PublishRecordingRecovered(cameraName string) {
	b.Publish(Event{
		Type:    "recording_recovered",
		Camera:  cameraName,
		Message: fmt.Sprintf("Recording recovered on %s", cameraName),
	})
}

// PublishRecordingFailed publishes a recording_failed event.
func (b *EventBroadcaster) PublishRecordingFailed(cameraName string) {
	b.Publish(Event{
		Type:    "recording_failed",
		Camera:  cameraName,
		Message: fmt.Sprintf("Recording recovery failed on %s — manual intervention required", cameraName),
	})
}
```

- [ ] **Step 2: Verify compilation**

Run: `cd /Users/ethanflower/personal_projects/mediamtx && go build ./internal/nvr/...`
Expected: Success (no errors)

- [ ] **Step 3: Commit**

```bash
git add internal/nvr/api/events.go
git commit -m "feat(api): add recording health SSE event methods (KAI-9)"
```

---

### Task 6: Add GetRecordingHealth method to the scheduler

**Files:**

- Modify: `internal/nvr/scheduler/scheduler.go`
- Test: `internal/nvr/scheduler/health_test.go`

- [ ] **Step 1: Write the failing test**

Add to `internal/nvr/scheduler/health_test.go`:

```go
func TestScheduler_GetRecordingHealth(t *testing.T) {
	s := New(nil, nil, nil, nil, "")
	// No health state yet — should return nil.
	h := s.GetRecordingHealth("cam-1")
	require.Nil(t, h)

	// Set up health state.
	s.mu.Lock()
	s.healthStates["cam-1"] = &RecordingHealth{
		Status:          HealthHealthy,
		LastSegmentTime: time.Date(2026, 4, 1, 12, 0, 0, 0, time.UTC),
	}
	s.mu.Unlock()

	h = s.GetRecordingHealth("cam-1")
	require.NotNil(t, h)
	require.Equal(t, HealthHealthy, h.Status)
}

func TestScheduler_GetAllRecordingHealth(t *testing.T) {
	s := New(nil, nil, nil, nil, "")
	s.mu.Lock()
	s.healthStates["cam-1"] = &RecordingHealth{Status: HealthHealthy}
	s.healthStates["cam-2"] = &RecordingHealth{Status: HealthStalled}
	s.mu.Unlock()

	all := s.GetAllRecordingHealth()
	require.Len(t, all, 2)
	require.Equal(t, HealthHealthy, all["cam-1"].Status)
	require.Equal(t, HealthStalled, all["cam-2"].Status)

	// Verify it's a copy — mutations don't affect internal state.
	all["cam-1"].Status = HealthFailed
	h := s.GetRecordingHealth("cam-1")
	require.Equal(t, HealthHealthy, h.Status)
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `cd /Users/ethanflower/personal_projects/mediamtx && go test ./internal/nvr/scheduler/ -run "TestScheduler_Get" -v`
Expected: FAIL — `GetRecordingHealth` not defined

- [ ] **Step 3: Implement the methods**

Add to `internal/nvr/scheduler/scheduler.go` (after `GetCameraState`, around line 190):

```go
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
```

- [ ] **Step 4: Run tests to verify they pass**

Run: `cd /Users/ethanflower/personal_projects/mediamtx && go test ./internal/nvr/scheduler/ -run "TestScheduler_Get" -v`
Expected: PASS

- [ ] **Step 5: Commit**

```bash
git add internal/nvr/scheduler/scheduler.go internal/nvr/scheduler/health_test.go
git commit -m "feat(scheduler): add GetRecordingHealth and GetAllRecordingHealth (KAI-9)"
```

---

### Task 7: Add recording health API endpoint

**Files:**

- Create: `internal/nvr/api/recording_health.go`
- Test: `internal/nvr/api/recording_health_test.go`
- Modify: `internal/nvr/api/router.go`

- [ ] **Step 1: Write the failing test**

Create `internal/nvr/api/recording_health_test.go`:

```go
package api

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/bluenviron/mediamtx/internal/nvr/scheduler"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

type mockSchedulerHealth struct {
	health map[string]*scheduler.RecordingHealth
}

func (m *mockSchedulerHealth) GetAllRecordingHealth() map[string]*scheduler.RecordingHealth {
	return m.health
}

func (m *mockSchedulerHealth) GetRecordingHealth(cameraID string) *scheduler.RecordingHealth {
	return m.health[cameraID]
}

func TestRecordingHealthHandler_List(t *testing.T) {
	gin.SetMode(gin.TestMode)

	now := time.Date(2026, 4, 1, 12, 0, 0, 0, time.UTC)
	mock := &mockSchedulerHealth{
		health: map[string]*scheduler.RecordingHealth{
			"cam-1": {
				Status:          scheduler.HealthHealthy,
				LastSegmentTime: now,
			},
			"cam-2": {
				Status:          scheduler.HealthStalled,
				LastSegmentTime: now.Add(-40 * time.Second),
				StallDetectedAt: now.Add(-10 * time.Second),
				RestartAttempts: 1,
				LastError:       "no segment received for 30s",
			},
		},
	}

	d := setupTestDBForHealth(t)
	defer d.Close()

	// Insert cameras so we can resolve names.
	_, _ = d.CreateCamera(&db.Camera{ID: "cam-1", Name: "Front Door"})
	_, _ = d.CreateCamera(&db.Camera{ID: "cam-2", Name: "Garage"})

	h := &RecordingHealthHandler{
		DB:             d,
		HealthProvider: mock,
	}

	router := gin.New()
	router.GET("/recordings/health", h.List)

	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/recordings/health", nil)
	router.ServeHTTP(w, req)

	require.Equal(t, http.StatusOK, w.Code)

	var resp struct {
		Cameras []recordingHealthEntry `json:"cameras"`
	}
	err := json.Unmarshal(w.Body.Bytes(), &resp)
	require.NoError(t, err)
	require.Len(t, resp.Cameras, 2)
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `cd /Users/ethanflower/personal_projects/mediamtx && go test ./internal/nvr/api/ -run TestRecordingHealthHandler -v`
Expected: FAIL — `RecordingHealthHandler` not defined

- [ ] **Step 3: Create the handler**

Create `internal/nvr/api/recording_health.go`:

```go
package api

import (
	"net/http"

	"github.com/bluenviron/mediamtx/internal/nvr/db"
	"github.com/bluenviron/mediamtx/internal/nvr/scheduler"
	"github.com/gin-gonic/gin"
)

// HealthProvider abstracts the scheduler's recording health methods so the
// handler can be tested without a full scheduler.
type HealthProvider interface {
	GetAllRecordingHealth() map[string]*scheduler.RecordingHealth
	GetRecordingHealth(cameraID string) *scheduler.RecordingHealth
}

// RecordingHealthHandler serves recording health status.
type RecordingHealthHandler struct {
	DB             *db.DB
	HealthProvider HealthProvider
}

type recordingHealthEntry struct {
	CameraID        string  `json:"camera_id"`
	CameraName      string  `json:"camera_name"`
	Status          string  `json:"status"`
	LastSegmentTime *string `json:"last_segment_time"`
	StallDetectedAt *string `json:"stall_detected_at,omitempty"`
	RestartAttempts int     `json:"restart_attempts"`
	LastError       string  `json:"last_error,omitempty"`
}

// List returns recording health for all cameras (or a single camera if
// ?camera_id is provided).
func (h *RecordingHealthHandler) List(c *gin.Context) {
	filterID := c.Query("camera_id")
	allHealth := h.HealthProvider.GetAllRecordingHealth()

	cameras, err := h.DB.ListCameras()
	if err != nil {
		apiError(c, http.StatusInternalServerError, "failed to list cameras", err)
		return
	}

	nameByID := make(map[string]string, len(cameras))
	for _, cam := range cameras {
		nameByID[cam.ID] = cam.Name
	}

	entries := make([]recordingHealthEntry, 0, len(allHealth))
	for camID, rh := range allHealth {
		if filterID != "" && camID != filterID {
			continue
		}
		entry := recordingHealthEntry{
			CameraID:        camID,
			CameraName:      nameByID[camID],
			Status:          rh.Status,
			RestartAttempts: rh.RestartAttempts,
			LastError:       rh.LastError,
		}
		if !rh.LastSegmentTime.IsZero() {
			t := rh.LastSegmentTime.UTC().Format("2006-01-02T15:04:05Z")
			entry.LastSegmentTime = &t
		}
		if !rh.StallDetectedAt.IsZero() {
			t := rh.StallDetectedAt.UTC().Format("2006-01-02T15:04:05Z")
			entry.StallDetectedAt = &t
		}
		entries = append(entries, entry)
	}

	c.JSON(http.StatusOK, gin.H{"cameras": entries})
}
```

- [ ] **Step 4: Add test helper**

Add to `internal/nvr/api/recording_health_test.go`:

```go
func setupTestDBForHealth(t *testing.T) *db.DB {
	t.Helper()
	tmpDir := t.TempDir()
	d, err := db.Open(tmpDir + "/test.db")
	require.NoError(t, err)
	return d
}
```

Add the `db` import:

```go
import (
	// ... existing imports ...
	"github.com/bluenviron/mediamtx/internal/nvr/db"
)
```

- [ ] **Step 5: Run test to verify it passes**

Run: `cd /Users/ethanflower/personal_projects/mediamtx && go test ./internal/nvr/api/ -run TestRecordingHealthHandler -v`
Expected: PASS

- [ ] **Step 6: Wire the endpoint in the router**

In `internal/nvr/api/router.go`, add after the `recordingHandler` construction (around line 74):

```go
	healthHandler := &RecordingHealthHandler{
		DB:             cfg.DB,
		HealthProvider: cfg.Scheduler,
	}
```

Add the route after the recordings routes (around line 249):

```go
	// Recording health.
	protected.GET("/recordings/health", healthHandler.List)
```

- [ ] **Step 7: Verify compilation**

Run: `cd /Users/ethanflower/personal_projects/mediamtx && go build ./internal/nvr/...`
Expected: Success

- [ ] **Step 8: Commit**

```bash
git add internal/nvr/api/recording_health.go internal/nvr/api/recording_health_test.go internal/nvr/api/router.go
git commit -m "feat(api): add GET /recordings/health endpoint (KAI-9)"
```

---

### Task 8: Add recording_health field to camera responses

**Files:**

- Modify: `internal/nvr/api/cameras.go`
- Test: `internal/nvr/api/cameras_test.go`

- [ ] **Step 1: Write the failing test**

Add to `internal/nvr/api/cameras_test.go`:

```go
func TestCameraResponseIncludesRecordingHealth(t *testing.T) {
	gin.SetMode(gin.TestMode)

	handler, cleanup := setupCameraTest(t)
	defer cleanup()

	// Create a camera.
	cam := &db.Camera{Name: "Test Cam", RTSPURL: "rtsp://test", MediaMTXPath: "test"}
	id, err := handler.DB.CreateCamera(cam)
	require.NoError(t, err)

	// GET the camera.
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest(http.MethodGet, "/cameras/"+id, nil)
	c.Params = gin.Params{{Key: "id", Value: id}}
	c.Set("camera_permissions", "*")
	handler.Get(c)

	require.Equal(t, http.StatusOK, w.Code)

	var resp map[string]interface{}
	err = json.Unmarshal(w.Body.Bytes(), &resp)
	require.NoError(t, err)

	// recording_health should be present (defaults to "inactive" when no scheduler).
	rh, ok := resp["recording_health"]
	require.True(t, ok, "response should include recording_health field")
	require.Equal(t, "inactive", rh)
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `cd /Users/ethanflower/personal_projects/mediamtx && go test ./internal/nvr/api/ -run TestCameraResponseIncludesRecordingHealth -v`
Expected: FAIL — no `recording_health` field in response

- [ ] **Step 3: Add recording_health to cameraResponse**

In `internal/nvr/api/cameras.go`, update the `cameraResponse` struct (lines 55-60):

```go
type cameraResponse struct {
	db.Camera
	StorageStatus   string            `json:"storage_status"`
	LiveViewPath    string            `json:"live_view_path"`
	StreamPaths     []streamPathEntry `json:"stream_paths"`
	RecordingHealth string            `json:"recording_health"`
}
```

- [ ] **Step 4: Set recording_health in buildCameraResponse**

In `internal/nvr/api/cameras.go`, in the `buildCameraResponse` method, set the field before the return statement (around line 109):

```go
	recordingHealth := scheduler.HealthInactive
	if h.Scheduler != nil {
		if rh := h.Scheduler.GetRecordingHealth(cam.ID); rh != nil {
			recordingHealth = rh.Status
		}
	}

	return cameraResponse{
		Camera:          *cam,
		StorageStatus:   status,
		LiveViewPath:    lvPath,
		StreamPaths:     streamPaths,
		RecordingHealth: recordingHealth,
	}
```

Make sure `scheduler` is imported (it already is, as `"github.com/bluenviron/mediamtx/internal/nvr/scheduler"`).

- [ ] **Step 5: Run test to verify it passes**

Run: `cd /Users/ethanflower/personal_projects/mediamtx && go test ./internal/nvr/api/ -run TestCameraResponseIncludesRecordingHealth -v`
Expected: PASS

- [ ] **Step 6: Run all camera tests**

Run: `cd /Users/ethanflower/personal_projects/mediamtx && go test ./internal/nvr/api/ -run TestCamera -v`
Expected: PASS

- [ ] **Step 7: Commit**

```bash
git add internal/nvr/api/cameras.go internal/nvr/api/cameras_test.go
git commit -m "feat(api): add recording_health field to camera responses (KAI-9)"
```

---

### Task 9: Wire OnSegmentComplete callback to scheduler

**Files:**

- Modify: `internal/nvr/nvr.go` (or wherever OnSegmentComplete is wired)

- [ ] **Step 1: Find and read the OnSegmentComplete wiring**

Run: `grep -n "OnSegmentComplete\|NotifySegment" internal/nvr/nvr.go`

The NVR's `OnSegmentComplete` callback receives a file path and duration. It already extracts the camera ID to insert recording metadata into the DB.

- [ ] **Step 2: Add scheduler notification to OnSegmentComplete**

In the `OnSegmentComplete` callback (in `internal/nvr/nvr.go`), after the DB insert, add:

```go
	if n.scheduler != nil {
		n.scheduler.NotifySegmentForCamera(cameraID)
	}
```

Where `cameraID` is the variable already extracted from the file path in the existing code.

- [ ] **Step 3: Verify compilation**

Run: `cd /Users/ethanflower/personal_projects/mediamtx && go build ./...`
Expected: Success

- [ ] **Step 4: Commit**

```bash
git add internal/nvr/nvr.go
git commit -m "feat(nvr): notify scheduler on segment completion for health tracking (KAI-9)"
```

---

### Task 10: Make Scheduler implement HealthProvider interface

**Files:**

- Modify: `internal/nvr/scheduler/scheduler.go`

- [ ] **Step 1: Verify the Scheduler already satisfies HealthProvider**

The `Scheduler` already has `GetAllRecordingHealth()` and `GetRecordingHealth()` from Task 6. The `HealthProvider` interface (Task 7) matches these signatures. Verify with a compile-time check.

Add to `internal/nvr/scheduler/scheduler.go` (at package level, after imports):

```go
// Compile-time check: Scheduler does not directly implement HealthProvider
// (which lives in the api package), but its methods match the interface.
// The router wires cfg.Scheduler into the HealthProvider field.
```

Actually, we can't do a compile-time check across packages. The router in Task 7 step 6 already assigns `cfg.Scheduler` to `HealthProvider`, which will fail at compile time if the methods don't match. This is sufficient.

- [ ] **Step 2: Handle nil scheduler in router**

In `internal/nvr/api/router.go`, the `healthHandler` construction (added in Task 7) should guard against nil scheduler:

```go
	var healthHandler *RecordingHealthHandler
	if cfg.Scheduler != nil {
		healthHandler = &RecordingHealthHandler{
			DB:             cfg.DB,
			HealthProvider: cfg.Scheduler,
		}
	}
```

And the route registration:

```go
	if healthHandler != nil {
		protected.GET("/recordings/health", healthHandler.List)
	}
```

- [ ] **Step 3: Verify full build**

Run: `cd /Users/ethanflower/personal_projects/mediamtx && go build ./...`
Expected: Success

- [ ] **Step 4: Run all tests**

Run: `cd /Users/ethanflower/personal_projects/mediamtx && go test ./internal/nvr/... -v`
Expected: PASS

- [ ] **Step 5: Commit**

```bash
git add internal/nvr/api/router.go
git commit -m "feat(api): guard recording health handler against nil scheduler (KAI-9)"
```

---

### Task 11: Final integration test and cleanup

**Files:**

- Test: `internal/nvr/scheduler/health_test.go`

- [ ] **Step 1: Write integration-style test for full stall → recovery cycle**

Add to `internal/nvr/scheduler/health_test.go`:

```go
func TestRecordingHealth_FullStallRecoveryCycle(t *testing.T) {
	h := NewRecordingHealth()

	// Start recording — set to healthy.
	h.Status = HealthHealthy
	now := time.Date(2026, 4, 1, 12, 0, 0, 0, time.UTC)
	h.RecordSegment(now)
	require.Equal(t, HealthHealthy, h.Status)

	// 35 seconds pass, no segment — stall detected.
	stallTime := now.Add(35 * time.Second)
	stalled := h.CheckStall(stallTime)
	require.True(t, stalled)
	require.Equal(t, HealthStalled, h.Status)

	// First restart attempt.
	require.True(t, h.ShouldRestart(stallTime))
	h.MarkRestarted(stallTime)
	require.Equal(t, 1, h.RestartAttempts)

	// 3 seconds later — backoff not elapsed (need 5s).
	require.False(t, h.ShouldRestart(stallTime.Add(3*time.Second)))

	// 6 seconds later — backoff elapsed, second attempt.
	t2 := stallTime.Add(6 * time.Second)
	require.True(t, h.ShouldRestart(t2))
	h.MarkRestarted(t2)
	require.Equal(t, 2, h.RestartAttempts)

	// 20 seconds later — third attempt (need 15s backoff).
	t3 := t2.Add(20 * time.Second)
	require.True(t, h.ShouldRestart(t3))
	h.MarkRestarted(t3)
	require.Equal(t, 3, h.RestartAttempts)

	// Max attempts reached — should not restart.
	require.False(t, h.ShouldRestart(t3.Add(time.Minute)))

	// Mark failed.
	h.MarkFailed()
	require.Equal(t, HealthFailed, h.Status)

	// Segment arrives — recovery!
	recoveryTime := t3.Add(2 * time.Minute)
	prev := h.RecordSegment(recoveryTime)
	require.Equal(t, HealthFailed, prev)
	require.Equal(t, HealthHealthy, h.Status)
	require.Equal(t, 0, h.RestartAttempts)
	require.True(t, h.StallDetectedAt.IsZero())
}
```

- [ ] **Step 2: Run the full test suite**

Run: `cd /Users/ethanflower/personal_projects/mediamtx && go test ./internal/nvr/scheduler/ -v`
Expected: PASS

- [ ] **Step 3: Run all NVR tests**

Run: `cd /Users/ethanflower/personal_projects/mediamtx && go test ./internal/nvr/... -v`
Expected: PASS

- [ ] **Step 4: Commit**

```bash
git add internal/nvr/scheduler/health_test.go
git commit -m "test(scheduler): add full stall/recovery cycle integration test (KAI-9)"
```
