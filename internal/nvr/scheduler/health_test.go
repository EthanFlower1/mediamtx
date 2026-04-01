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
