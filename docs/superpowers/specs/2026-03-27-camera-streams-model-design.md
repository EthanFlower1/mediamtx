# Camera Multi-Stream Model

**Date:** 2026-03-27
**Scope:** Data model change to support multiple streams per camera with configurable roles and per-stream recording rules

## Problem

Cameras currently store a single `rtsp_url` and optional `sub_stream_url` with implicit roles. Users can't control which stream is used for recording, live view, AI detection, or mobile viewing. Recording rules target the camera as a whole — there's no way to record continuous on the sub stream while recording events-only on the main stream.

## Design Decisions

- Streams are stored in a `camera_streams` table, each belonging to a camera
- Each stream is tagged with one or more **predefined roles**: `live_view`, `recording`, `mobile`, `ai_detection`
- Roles are stored as a comma-separated string on the stream record
- A single stream can have multiple roles; multiple streams can share roles (system uses first match)
- Recording rules get a `stream_id` field to target a specific stream, enabling per-stream recording schedules
- When a camera is added with ONVIF profiles, streams are auto-populated with default role assignments

## Data Model

### New table: `camera_streams`

```sql
CREATE TABLE camera_streams (
  id            TEXT PRIMARY KEY,
  camera_id     TEXT NOT NULL REFERENCES cameras(id) ON DELETE CASCADE,
  name          TEXT NOT NULL,
  rtsp_url      TEXT NOT NULL,
  profile_token TEXT NOT NULL DEFAULT '',
  video_codec   TEXT NOT NULL DEFAULT '',
  width         INTEGER NOT NULL DEFAULT 0,
  height        INTEGER NOT NULL DEFAULT 0,
  roles         TEXT NOT NULL DEFAULT '',
  created_at    TEXT NOT NULL
);
```

### Modified table: `recording_rules`

Add column:

```sql
ALTER TABLE recording_rules ADD COLUMN stream_id TEXT DEFAULT '';
```

When `stream_id` is empty, the rule uses the camera's stream tagged with the `recording` role (backward compatibility with existing rules).

### Predefined roles

| Role           | System behavior                                                         |
| -------------- | ----------------------------------------------------------------------- |
| `live_view`    | Desktop/tablet live view uses this stream's MediaMTX path               |
| `recording`    | Default target for recording rules without explicit stream_id           |
| `mobile`       | Mobile live view uses this stream (falls back to `live_view` if absent) |
| `ai_detection` | AI pipeline reads frames from this stream                               |

### Deprecated fields on `cameras`

`rtsp_url` and `sub_stream_url` become legacy. A migration creates `camera_streams` records from existing data:

- If `rtsp_url` is set → create stream with roles `live_view,recording,ai_detection`
- If `sub_stream_url` is also set → move `recording,ai_detection,mobile` roles to the sub stream

After migration, all code reads from `camera_streams`. The old columns remain in the schema but are no longer written to.

## Auto-Population

When a camera is added with ONVIF profiles (from discovery or probe):

1. Create a `camera_streams` record for each profile
2. Sort streams by resolution (width × height) descending
3. Highest resolution stream gets `live_view`
4. Lowest resolution stream gets `recording`, `ai_detection`, `mobile`
5. If only one stream exists, it gets all four roles

## API Changes

### New endpoints

- `GET /cameras/:id/streams` — list all streams for a camera
- `POST /cameras/:id/streams` — add a stream (name, rtsp_url, profile_token, codec, resolution, roles)
- `PUT /streams/:id` — update a stream (name, roles, rtsp_url)
- `DELETE /streams/:id` — delete a stream

### Modified endpoints

- `POST /cameras` — when ONVIF profiles are provided during camera creation, auto-create stream records
- `GET /cameras/:id` — include `streams` array in response
- `GET /cameras` — include `streams` array for each camera

### Recording rules

- `POST /cameras/:id/recording-rules` — accepts optional `stream_id` field
- `PUT /recording-rules/:id` — accepts optional `stream_id` field
- Existing rules without `stream_id` continue to work (resolve to `recording`-role stream)

## Backend Changes

### Stream resolution helper

New function `ResolveStreamURL(cameraID, role)` in the DB layer:

1. Query `camera_streams` where `camera_id = ?` and `roles LIKE '%role%'`
2. Return the first match's `rtsp_url`
3. If no match, fall back to the camera's legacy `rtsp_url`

### MediaMTX YAML writer

Currently writes one path per camera (`nvr/{camera_id}/main`). Changes:

- Write a path for each stream that has the `recording` role: `nvr/{camera_id}/{stream_id}`
- The `source` for each path is the stream's `rtsp_url`
- Recording path pattern: `./recordings/nvr/{camera_id}/{stream_id}/...`

### AI pipeline

Currently reads from `sub_stream_url` or falls back to `rtsp_url`. Change to:

- Resolve the `ai_detection`-role stream URL for the camera
- Fall back to `recording`, then `live_view` if no `ai_detection` stream is set

### Recording scheduler

Currently creates one recording config per camera. Change to:

- For each recording rule, resolve the target stream (explicit `stream_id` or `recording`-role default)
- Generate MediaMTX path entries per-stream, not per-camera
- Multiple rules on different streams → multiple concurrent recordings

### Camera creation flow

When `POST /cameras` includes ONVIF profile data (from discovery probe):

1. Create the camera record
2. Create `camera_streams` records from profiles with auto-assigned roles
3. Write MediaMTX YAML paths for streams with recording roles

## Flutter Changes

### Camera detail/settings screen

Add a "Streams" section showing each stream as a card:

- Stream name, resolution, codec
- RTSP URL (muted text)
- Role chips as toggles (live_view, recording, mobile, ai_detection) — tap to toggle on/off
- Save updates via `PUT /streams/:id`

### Recording rules UI

When creating/editing a recording rule, add a stream selector dropdown:

- Lists all streams for the camera
- Default: "Auto (recording stream)"
- Selecting a specific stream sets the `stream_id`

### Discovery detail sheet

The stream cards in the discovery detail sheet (from the discovery redesign) already show available profiles. After adding the camera, the roles are auto-assigned per the auto-population rules.

## Migration Strategy

1. Create `camera_streams` table
2. For each existing camera:
   - Create stream from `rtsp_url` with roles `live_view,recording,ai_detection`
   - If `sub_stream_url` is set, create second stream with roles `recording,ai_detection,mobile` and remove those roles from the first stream (first stream keeps `live_view` only)
3. Add `stream_id` column to `recording_rules` (default empty)
4. All existing recording rules continue to work — empty `stream_id` resolves to `recording`-role stream

## Error Handling

- If a camera has no streams, the system falls back to the legacy `rtsp_url` field
- If a requested role has no matching stream, log a warning and fall back to any available stream
- Deleting a stream that has recording rules referencing it: reject with error (must reassign rules first)
- Auto-population silently skips streams with empty RTSP URIs

## Testing

- Unit test: stream CRUD operations in DB layer
- Unit test: `ResolveStreamURL` with various role configurations and fallbacks
- Unit test: auto-population role assignment logic (single stream, two streams, many streams)
- Unit test: migration creates correct stream records from legacy fields
- Unit test: recording rule resolution with explicit stream_id vs default
- Integration: create camera with ONVIF profiles → verify streams created with correct roles
- Integration: create two recording rules on different streams → verify both record simultaneously
