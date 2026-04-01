# Recording Health Monitoring Design

**Date:** 2026-04-01
**Ticket:** KAI-9

## Overview

Add per-camera recording health monitoring that tracks active recording status, detects stalled recordings (no new segment data for 30 seconds while recording is expected), attempts automatic recovery, and exposes health via API and SSE events.

## Architecture

Recording health monitoring lives inside the existing `Scheduler` (`internal/nvr/scheduler/scheduler.go`). Each camera gets a `RecordingHealth` struct tracked alongside the existing `CameraState`.

### RecordingHealth Struct

```go
type RecordingHealth struct {
    Status          string    // "healthy", "stalled", "failed", "inactive"
    LastSegmentTime time.Time // updated via OnSegmentComplete callback
    StallDetectedAt time.Time // when stall was first noticed
    RestartAttempts int       // 0-3
    LastRestartAt   time.Time // for exponential backoff
    LastError       string    // reason for current state
}
```

### State Transitions

```
inactive -> healthy:  recording starts, first segment arrives
healthy  -> stalled:  no segment for 30s while recording expected
stalled  -> healthy:  segment arrives after recovery
stalled  -> failed:   3 restart attempts exhausted
failed   -> healthy:  manual re-enable or segment arrives
any      -> inactive: recording stops (rule deactivated)
```

### Stall Threshold

Fixed at 30 seconds. A camera is considered stalled if `time.Since(LastSegmentTime) > 30s` and the camera is expected to be recording (`EffectiveMode != "off"` and `Recording == true`).

## Data Flow & Integration

### Segment Arrival Notification

The existing `OnSegmentComplete` callback (called by the recorder when a segment file is finalized) signals segment arrival. The scheduler receives this and updates `LastSegmentTime` for the corresponding camera (matched via the media path).

### Stall Detection

During the scheduler's existing 30-second evaluation tick:

1. For each camera where `EffectiveMode != "off"` and `Recording == true`
2. Check if `time.Since(LastSegmentTime) > 30s`
3. If stalled and not already in `stalled`/`failed` state, transition to `stalled` and attempt recovery

### Recovery Mechanism

On stall detection:

1. Call `yamlWriter.SetPathValue(path, "record", false)` then `true` after a brief pause
2. Trigger config reload via the existing HTTP API call
3. Increment `RestartAttempts`, set `LastRestartAt`
4. Exponential backoff: skip restart if `time.Since(LastRestartAt) < backoffDuration`
   - Attempt 1: 5s backoff
   - Attempt 2: 15s backoff
   - Attempt 3: 45s backoff
5. After 3 failed attempts, mark status as `failed` and stop retrying

Recovery resets: when a segment arrives (at any point), `RestartAttempts` resets to 0 and status returns to `healthy`.

### Event Publishing

On state transitions, publish via the existing `EventPublisher`:

- `recording_stalled` — camera name, stall duration
- `recording_recovered` — camera name, number of attempts it took
- `recording_failed` — camera name, attempts exhausted

These are SSE events delivered to connected clients via the existing `EventBroadcaster`.

## API

### New Endpoint: Recording Health

`GET /api/nvr/recordings/health`

Returns an array of per-camera recording health status. Protected route (JWT required).

Response:
```json
{
  "cameras": [
    {
      "camera_id": "uuid",
      "camera_name": "Front Door",
      "status": "healthy",
      "last_segment_time": "2026-04-01T12:00:00Z",
      "stall_detected_at": null,
      "restart_attempts": 0,
      "last_error": ""
    },
    {
      "camera_id": "uuid",
      "camera_name": "Garage",
      "status": "stalled",
      "last_segment_time": "2026-04-01T11:58:30Z",
      "stall_detected_at": "2026-04-01T11:59:00Z",
      "restart_attempts": 1,
      "last_error": "no segment received for 30s"
    }
  ]
}
```

Optional query parameter: `?camera_id=<uuid>` to filter to a single camera.

### Extended Camera Responses

`GET /api/nvr/cameras` and `GET /api/nvr/cameras/:id` responses gain a `recording_health` field:

```json
{
  "id": "uuid",
  "name": "Front Door",
  "recording_health": "healthy",
  ...
}
```

The field is a simple status string: `"healthy"`, `"stalled"`, `"failed"`, or `"inactive"`.

## Storage

No new database tables. Health state is ephemeral (in-memory only). It resets on process restart, which is correct since recording state itself resets on restart and the scheduler re-evaluates from scratch.

## Testing

### Unit Tests

- `TestStallDetection` — mock time, verify `healthy` -> `stalled` after 30s with no segment
- `TestRecoverySuccess` — simulate stall, trigger restart, send segment, verify -> `healthy`
- `TestRecoveryExhausted` — simulate 3 failed restarts, verify -> `failed`
- `TestBackoffTiming` — verify restart attempts respect exponential backoff (5s, 15s, 45s)
- `TestInactiveCamera` — verify cameras not expected to record stay `inactive`
- `TestSegmentUpdateResetsState` — verify segment arrival clears stall state and resets attempts

### API Tests

- `TestRecordingHealthEndpoint` — verify response shape and status codes
- `TestCameraResponseIncludesHealth` — verify `recording_health` field in camera responses

### Integration

- Verify SSE events are published on state transitions (`recording_stalled`, `recording_recovered`, `recording_failed`)
