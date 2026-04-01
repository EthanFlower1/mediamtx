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
	h.LastRestartAt = time.Now().Add(-3 * time.Second) // only 3s ago, need 5s
	should := h.ShouldRestart(time.Now())
	require.False(t, should)
}

func TestRecordingHealth_ShouldRestart_BackoffElapsed(t *testing.T) {
	h := NewRecordingHealth()
	h.Status = HealthStalled
	h.RestartAttempts = 1
	h.LastRestartAt = time.Now().Add(-20 * time.Second) // 20s ago, need 5s
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

func TestScheduler_GetRecordingHealth(t *testing.T) {
	s := New(nil, nil, nil, nil, "")
	h := s.GetRecordingHealth("cam-1")
	require.Nil(t, h)

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

	// Verify it's a copy.
	all["cam-1"].Status = HealthFailed
	h := s.GetRecordingHealth("cam-1")
	require.Equal(t, HealthHealthy, h.Status)
}

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

func TestBackoffDuration(t *testing.T) {
	require.Equal(t, 5*time.Second, backoffDuration(0))
	require.Equal(t, 15*time.Second, backoffDuration(1))
	require.Equal(t, 45*time.Second, backoffDuration(2))
	require.Equal(t, 45*time.Second, backoffDuration(5)) // capped
}
