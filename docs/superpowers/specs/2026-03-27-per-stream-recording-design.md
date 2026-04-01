# Per-Stream Recording Schedules with Unified Playback

**Date:** 2026-03-27
**Status:** Approved
**Goal:** Make recording rules stream-aware so different streams can have independent recording schedules, with the playback timeline seamlessly showing the best available quality.

---

## Context

Recording rules already have a `stream_id` field (migration 21) but the scheduler ignores it — it always records on the camera's single MediaMTX path. Users want to record the sub-stream 24/7 (low storage cost) while recording the main stream only on motion events (high quality when it matters). The playback timeline should transparently show recordings from all streams as one unified bar, preferring the highest quality when multiple streams have recordings for the same time.

## Decisions

| Decision | Choice | Rationale |
|----------|--------|-----------|
| Timeline display | Single bar, best quality wins | Keep UI simple; system picks best available |
| Quality preference | Main (highest res) preferred, sub as fallback | Users want best quality when available |
| Stream on rules | Optional, defaults to main path | Backward compatible, simple for most users |
| Per-camera recording_stream_id | Remove (superseded by per-rule stream_id) | Per-rule is more flexible, avoids duplication |

---

## Backend: Scheduler Stream-Aware Path Management

### Current behavior

The scheduler's `evaluate()` groups rules by camera, resolves one `EffectiveMode` per camera, and writes `record: true/false` to a single MediaMTX path.

### New behavior

The scheduler groups rules by **(camera, stream_id)** pair. Each unique pair gets its own path and independent recording state.

For each camera during evaluation:
1. Group active matching rules by `stream_id` (empty string = default/main)
2. Evaluate `EffectiveMode` per group independently
3. For the default group (empty `stream_id`): use `cam.MediaMTXPath` as before
4. For non-default groups: resolve the stream's RTSP URL from `camera_streams`, create a MediaMTX path named `{mediamtxPath}~{streamID[:8]}` on demand via `yamlWriter.AddPath()`
5. Set `record: true/false` per path based on that group's effective mode
6. When a non-default group has no matching rules, set `record: false` and remove the path via `yamlWriter.RemovePath()`

### Motion state machine per stream

Each (camera, stream_id) pair gets its own `MotionSM` instance. If the main stream has an "events" rule and the sub-stream has "always", they operate independently. The `MotionSM` map key changes from `cameraID` to `cameraID + ":" + streamID`.

### Credential embedding

Non-default stream paths need credentials embedded in the RTSP URL. The scheduler already has `decryptPassword()` and access to camera ONVIF credentials. The same `embedCredentials` pattern from `nvr.go` is reused.

### Path naming

- Default (empty stream_id): `nvr/{camera-name}` (existing path, unchanged)
- Non-default: `nvr/{camera-name}~{first 8 chars of stream UUID}`

The 8-char prefix keeps paths short while avoiding collisions. The `~` separator is already used by the recording stream feature.

---

## Backend: Best-Quality Playback Resolution

### Recording identification

Recordings already store `file_path` which contains the MediaMTX path. The `~{streamID}` suffix naturally tags which stream a recording came from. No schema changes needed.

### New query: QueryRecordingsBestQuality

```go
func (d *DB) QueryRecordingsBestQuality(cameraID string, start, end time.Time) ([]*Recording, error)
```

Logic:
1. Query all recordings for the camera in the time range (same overlap logic as `QueryRecordings`)
2. For each recording, determine the stream resolution from the file path:
   - Path contains `~{streamID}` → look up stream width×height from `camera_streams`
   - Path has no `~` suffix → it's the main stream, use the camera's primary stream resolution
3. Sort by start_time
4. For overlapping time periods, keep only the highest-resolution recording
5. Return the merged list

The existing `QueryRecordings` stays unchanged for backward compatibility. The timeline provider switches to calling `QueryRecordingsBestQuality` via a new endpoint or by enhancing the existing one.

### API change

Add a `best_quality=true` query parameter to `GET /recordings`:
- When `best_quality=true`: use `QueryRecordingsBestQuality`
- When absent or false: use `QueryRecordings` (current behavior, returns all)

---

## Backend: Remove Per-Camera recording_stream_id

The `recording_stream_id` column (migration 23), the `PUT /cameras/:id/recording-stream` endpoint, the `UpdateCameraRecordingStream` DB function, and the `configureRecordingPaths` logic are all removed. Per-rule `stream_id` replaces them entirely.

Migration 24 drops the column:
```sql
-- SQLite doesn't support DROP COLUMN before 3.35.0, so we leave the column
-- but stop using it. The API and UI no longer reference it.
```

In practice: remove the endpoint, remove the Flutter dropdown from the recording section, remove `configureRecordingPaths`. The column stays in the DB but is ignored.

---

## Flutter: Recording Rules Stream Selector

### Rules creation/edit UI

The recording rules screen (create/edit dialog) adds a stream dropdown:

- `DropdownButtonFormField<String>` with "Default" + all camera streams
- Display format: `"{stream.name} ({width}x{height})"` or `"Default"`
- Value: stream ID string or empty string for default
- Positioned after the mode selector, before the days/time pickers
- Saved as `stream_id` on the rule

### Rules list display

Each rule in the list shows its stream name if non-default (e.g., "24/7 — Sub Stream (640x480)").

---

## Flutter: Playback Timeline (Minimal Changes)

### Timeline bar

No visual changes. The single bar per camera stays. Recordings from all streams are merged into one continuous bar. The `recordingSegmentsProvider` already fetches by `camera_id` which returns all recordings.

### Playback stream selection

When the user taps a point on the timeline to play:
1. The provider fetches recordings with `best_quality=true`
2. Finds the segment covering the requested timestamp
3. Plays that segment — user doesn't know or care which stream it's from

### Recording segments provider change

Update the `recordingSegmentsProvider` to pass `best_quality=true` when fetching for timeline display:

```dart
final res = await api.get('/recordings', queryParameters: {
  'camera_id': params.cameraId,
  'start': ...,
  'end': ...,
  'best_quality': 'true',
});
```

---

## Files Changed

### Backend
- `internal/nvr/scheduler/scheduler.go` — per-stream rule grouping, path management, per-stream MotionSM
- `internal/nvr/db/recordings.go` — `QueryRecordingsBestQuality`
- `internal/nvr/api/recordings.go` — `best_quality` query param
- `internal/nvr/api/cameras.go` — remove `UpdateRecordingStream`, `configureRecordingPaths`
- `internal/nvr/api/router.go` — remove `PUT /cameras/:id/recording-stream`
- `internal/nvr/db/cameras.go` — stop reading/writing `recording_stream_id`

### Flutter
- `lib/screens/cameras/camera_detail_screen.dart` — remove recording stream dropdown
- `lib/screens/cameras/recording_rules_screen.dart` — add stream dropdown to create/edit dialog
- `lib/models/recording_rule.dart` — ensure `streamId` field exists
- `lib/providers/recordings_provider.dart` — pass `best_quality=true`
- `lib/models/camera.dart` — remove `recordingStreamId` field
