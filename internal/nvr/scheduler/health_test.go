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
