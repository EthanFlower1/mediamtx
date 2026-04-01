# KAI-14: Recording Statistics API Design

## Overview

Expose per-camera recording statistics via two API endpoints: a summary endpoint for all cameras and a per-camera gap history endpoint. Statistics include continuous recording uptime, total storage used, segment count, and complete gap history.

## Endpoints

### GET /api/nvr/recordings/stats

Per-camera recording summary. Optional `?camera_id=X` query param to filter to a single camera.

**Response:**

```json
{
  "cameras": [
    {
      "camera_id": "cam-1",
      "camera_name": "Front Door",
      "total_bytes": 53687091200,
      "segment_count": 142,
      "total_recorded_ms": 511200000,
      "current_uptime_ms": 23040000,
      "last_gap_end": "2026-04-01T08:36:00Z",
      "oldest_recording": "2026-03-15T00:00:00Z",
      "newest_recording": "2026-04-01T14:59:00Z",
      "gap_count": 7
    }
  ]
}
```

**Field definitions:**

- `total_bytes`: Sum of `file_size` across all recordings for the camera.
- `segment_count`: Count of recording rows for the camera.
- `total_recorded_ms`: Sum of `duration_ms` across all recordings.
- `current_uptime_ms`: Time from the most recent gap's end (or the oldest recording start if no gaps) to the newest recording end. Represents how long the camera has been continuously recording.
- `last_gap_end`: Timestamp of the end of the most recent gap. Null if no gaps exist.
- `oldest_recording`: Start time of the earliest recording.
- `newest_recording`: End time of the latest recording.
- `gap_count`: Total number of recording gaps all-time.

### GET /api/nvr/recordings/stats/:camera_id/gaps

Complete gap history for a single camera, ordered chronologically.

**Response:**

```json
{
  "camera_id": "cam-1",
  "gaps": [
    {
      "start": "2026-03-28T02:15:00Z",
      "end": "2026-03-28T02:18:30Z",
      "duration_ms": 210000
    }
  ]
}
```

**Gap definition:** A gap exists when the interval between `recording[n].end_time` and `recording[n+1].start_time` exceeds 2000ms (2x the default segment part duration). This avoids treating normal sub-second segment boundaries as gaps.

## Database Layer

No new tables or migrations. All data is derived from the existing `recordings` table.

### New DB Methods

**GetRecordingStats(cameraID string) -> []RecordingStats**

Single aggregate query per camera (or all cameras if cameraID is empty):

```sql
SELECT
  r.camera_id,
  c.name AS camera_name,
  COALESCE(SUM(r.file_size), 0) AS total_bytes,
  COUNT(*) AS segment_count,
  COALESCE(SUM(r.duration_ms), 0) AS total_recorded_ms,
  MIN(r.start_time) AS oldest_recording,
  MAX(r.end_time) AS newest_recording
FROM recordings r
JOIN cameras c ON c.id = r.camera_id
GROUP BY r.camera_id
```

Uptime and gap count are computed from the gap detection query results.

**GetRecordingGaps(cameraID string, gapThresholdMs int64) -> []Gap**

Uses SQL window function to detect gaps efficiently:

```sql
SELECT
  end_time AS gap_start,
  next_start AS gap_end,
  (strftime('%s', next_start) - strftime('%s', end_time)) * 1000 AS duration_ms
FROM (
  SELECT
    end_time,
    LEAD(start_time) OVER (ORDER BY start_time) AS next_start
  FROM recordings
  WHERE camera_id = ?
)
WHERE duration_ms > ?
ORDER BY gap_start
```

## Handler & Router

- New file: `internal/nvr/api/stats.go` containing `StatsHandler` struct
- Methods: `GetStats(c *gin.Context)` and `GetGaps(c *gin.Context)`
- Registered in `router.go` under the existing NVR recording group
- Uses `hasCameraPermission()` for the per-camera gaps endpoint
- Error handling via existing `apiError()` pattern

## Constants

- Gap threshold: 2000ms (hardcoded constant `defaultGapThresholdMs`)

## Testing

Unit tests in `internal/nvr/api/stats_test.go` and `internal/nvr/db/recordings_test.go`:

- No recordings: returns empty cameras array / empty gaps array
- Single recording, no gaps: `current_uptime_ms` equals recording duration, `gap_count` is 0
- Multiple recordings with gaps: correct gap detection and uptime calculation
- Camera ID filter: summary returns only the requested camera
- Permission check: gaps endpoint respects camera permissions
