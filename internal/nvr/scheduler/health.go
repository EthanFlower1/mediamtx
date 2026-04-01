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
