# Playback Session Protocol Design

## Problem

The current playback API is stateless — every seek opens a new HTTP request to `/get?path=X&start=Y&duration=Z`, causing visible loading delay. The fMP4 stream is non-seekable (`Accept-Ranges: none`), so the client must tear down and rebuild the player connection on every scrub. There's no server-side support for trick play (reverse, frame stepping, variable speed with frame decimation) or synchronized multi-camera playback.

## Approach

Add a WebSocket-controlled playback session to the NVR API layer. The client opens a WebSocket for commands (play, pause, seek, speed, step) and persistent HTTP chunked streams for media. The server owns an fMP4 muxer per camera that can splice fragments from new positions into the same stream without breaking the player's decoder. Built as a new NVR endpoint wrapping MediaMTX's existing `recordstore` and `segmentFMP4` packages — does not modify MediaMTX's built-in `/get` handler.

---

## 1. Session Lifecycle

### WebSocket Endpoint

`GET /api/nvr/playback/ws` — upgrades to WebSocket.

### Session Create Flow

1. Client connects WebSocket
2. Client sends: `{"cmd": "create", "camera_ids": ["cam1", "cam2"], "start": "2026-03-24T10:00:00Z"}`
3. Server creates a `PlaybackSession` — allocates per-camera fMP4 muxers, opens segment files from disk
4. Server responds: `{"event": "created", "session_id": "abc123", "streams": {"cam1": "/api/nvr/playback/stream/abc123/cam1", "cam2": "/api/nvr/playback/stream/abc123/cam2"}}`
5. Client opens each stream URL — server responds with `Transfer-Encoding: chunked`, writes fMP4 init segment immediately, then pauses (session starts paused)
6. Client sends `{"cmd": "play"}` — server starts writing fMP4 fragments into the chunked streams

### Session Scope

One session controls all cameras — single WebSocket controls multiple cameras, server keeps them in sync. Seek/play/pause applies to all cameras atomically.

### Commands (client → server)

| Command | Payload | Effect |
|---------|---------|--------|
| `create` | `camera_ids`, `start` | Create session |
| `play` | — | Resume writing fragments |
| `pause` | — | Stop writing fragments |
| `seek` | `position` (RFC3339 or seconds-into-day) | Splice to new position |
| `speed` | `rate` (0.5, 1.0, 2.0, etc.) | Change fragment output rate |
| `step` | `direction` (1 or -1) | Write single frame, stay paused |
| `add_camera` | `camera_id` | Add camera to session |
| `remove_camera` | `camera_id` | Remove camera from session |
| `close` | — | Tear down session |

### Events (server → client)

| Event | Payload | When |
|-------|---------|------|
| `created` | `session_id`, `streams` | Session ready |
| `position` | `time` (RFC3339) | Every ~500ms during playback |
| `state` | `playing`, `speed`, `position` | After any state change |
| `buffering` | `camera_id`, `buffering` (bool) | Stream buffer state |
| `segment_gap` | `gap_start`, `next_segment` | Playback hit a recording gap |
| `stream_restart` | `camera_id`, `new_url` | Codec changed, client must reconnect this stream |
| `end` | — | Reached end of recordings |
| `error` | `message` | Something went wrong |

### Session Cleanup

- WebSocket disconnect → 30 second grace period (reconnect window) → dispose session
- Idle timeout (no commands for 10 minutes) → dispose
- Explicit `close` command → immediate dispose
- Dispose closes all muxers, flushes and closes HTTP streams

---

## 2. fMP4 Stream Splicing

The core technical challenge — writing fMP4 fragments from a new position into an already-open HTTP chunked stream without breaking the player's decoder.

### fMP4 Structure Recap

- `ftyp` + `moov` (init segment) — sent once at stream start, describes codecs/tracks
- Repeating `moof` + `mdat` pairs (fragments) — each contains ~1 second of frames with a sequence number and base decode timestamp

### Splice on Seek

1. Server receives `seek` command
2. Server finds the target segment file on disk, reads its `moov` to verify codec compatibility with the init segment already sent
3. Server scans to the nearest keyframe at or before the target time
4. Server continues the `moof` sequence number from where the previous fragment left off (e.g., if last fragment was seq 47, next splice starts at seq 48)
5. Server adjusts the `baseDecodeTime` in the `moof` to be continuous with the previous fragment's end — the player sees no discontinuity in decode timestamps
6. Server writes the new `moof+mdat` into the chunked HTTP response

The player sees a continuous fMP4 stream with no gaps in sequence numbers or timestamps. The video content jumps, but the container format is seamless. The player doesn't need to reinitialize its decoder.

### Codec Mismatch Handling

If a camera's codec changes between segments (rare but possible after reconfiguration), the server sends a `stream_restart` event over WebSocket. The client closes that camera's HTTP stream and opens a new one to get a fresh init segment.

### Gap Handling

When playback reaches a recording gap:
1. Server sends `segment_gap` event with gap boundaries
2. Server automatically skips to the next segment's first keyframe
3. Adjusts timestamps so the player sees no pause — the gap is invisible in the stream, only visible on the timeline via the event

---

## 3. Server Architecture

### New Go Files in `internal/nvr/playback/`

| File | Responsibility |
|------|---------------|
| `session.go` | `PlaybackSession` struct — owns muxer state, segment reader, position tracking per camera |
| `muxer.go` | Wraps MediaMTX's `segmentFMP4` reading + custom fMP4 writer that handles splice logic (sequence continuity, timestamp remapping) |
| `stream.go` | HTTP handler for `/api/nvr/playback/stream/:session/:camera` — chunked response writer, blocks on a channel fed by the muxer |
| `ws.go` | WebSocket handler for `/api/nvr/playback/ws` — command parsing, session dispatch, event broadcasting |
| `manager.go` | `SessionManager` — session lifecycle, cleanup on disconnect, timeout for orphaned sessions |

### Data Flow

```
Client WebSocket ──cmd──> ws.go ──> SessionManager ──> PlaybackSession
                                                          │
                                                   ┌──────┴──────┐
                                                   ▼              ▼
                                              cam1 muxer     cam2 muxer
                                                   │              │
                                              fMP4 frags     fMP4 frags
                                                   │              │
                                              chan []byte     chan []byte
                                                   │              │
                                                   ▼              ▼
                                              stream.go      stream.go
                                              (HTTP chunked)  (HTTP chunked)
                                                   │              │
                                                   ▼              ▼
                                              Client player  Client player
```

### Segment Reading

Imports `internal/recordstore` to find segment files on disk, and `internal/playback/segment_fmp4.go` to parse fMP4 headers and samples. Does NOT import the playback HTTP handler — only the low-level reading code.

---

## 4. Trick Play

Built on top of the session protocol — these are how the muxer writes fragments differently.

### Variable Speed (0.5x–8x forward)

- Server reads fragments at normal rate but adjusts the timing of when it writes them to the HTTP stream
- At 2x: writes fragments twice as fast (halved delay between writes)
- At 0.5x: writes fragments at half rate (doubled delay)
- No frame dropping below 4x — all frames sent, player decodes at arrival rate
- Above 4x: server decimates to keyframes only (drops inter-frames), reducing bandwidth and decode load

### Frame Step Forward

- Server reads the next frame (or next keyframe), wraps it in a `moof+mdat`, writes it to the stream
- Stays paused — only one fragment written per `step` command
- Sequence numbers and timestamps continue normally

### Frame Step Backward

- fMP4 is forward-only by nature — can't decode a B/P-frame without its reference
- Server finds the previous keyframe (GOP boundary), re-reads from there, writes only that keyframe as a fragment
- Effectively jumps back by one GOP (~2-5 seconds depending on encoding)
- The `position` event tells the client exactly where it landed

### Reverse Playback

- Server reads GOPs in reverse order
- For each GOP: reads forward within it (must decode in order), collects keyframe, writes it as a fragment
- Timestamps are mapped to go backward in the stream so the player's position counter decrements
- Limited to keyframe boundaries — not frame-accurate reverse, but smooth enough for review
- Only supported at 1x reverse — no fast reverse (too CPU intensive for real-time)

---

## 5. Flutter Client Changes

### PlaybackController Rewrite

The controller's public API does not change — `seek()`, `play()`, `pause()`, `position`, `isPlaying`, `speed` all stay the same. The internal implementation switches from HTTP-per-seek to WebSocket commands.

### New Client Flow

1. `PlaybackController.init()` → opens WebSocket to `/api/nvr/playback/ws`
2. Sends `create` with selected cameras and current position
3. Receives `created` event with stream URLs
4. Opens each stream URL in a `media_kit` Player (once, never reconnects unless `stream_restart`)
5. `seek()` → sends `{"cmd": "seek", "position": "..."}` over WebSocket
6. `play()`/`pause()` → sends command over WebSocket
7. `stepFrame()` → sends `{"cmd": "step", "direction": 1}`
8. `setSpeed()` → sends `{"cmd": "speed", "rate": 2.0}`
9. Position tracking comes from WebSocket `position` events — no longer derived from the player's stream position

### What Gets Simpler

- No `_streamOrigin` translation — server sends absolute position
- No stale position rejection — server is authoritative
- No stream reopening on seek — same HTTP connection throughout
- `media_kit` Player opened once per camera, disposed on session close

### What Gets Added

- `WebSocketChannel` connection management (connect, reconnect, dispose)
- Command serialization / event deserialization (JSON)
- Handling `stream_restart` event (close + reopen one camera's player)
- Handling `segment_gap` event (update timeline UI)

### Files Changed

- `lib/screens/playback/playback_controller.dart` — major rewrite, WebSocket-based
- `lib/services/playback_service.dart` — add WebSocket URL construction, remove URL-per-seek logic

### Files Unchanged

All timeline widgets, transport controls, jog slider, event popup — they talk to the controller through the same interface. The controller's public API doesn't change.

---

## Files Affected

### New files (Go backend):
- `internal/nvr/playback/session.go`
- `internal/nvr/playback/muxer.go`
- `internal/nvr/playback/stream.go`
- `internal/nvr/playback/ws.go`
- `internal/nvr/playback/manager.go`

### Modified files (Go backend):
- `internal/nvr/api/router.go` — register new playback endpoints

### Modified files (Flutter client):
- `lib/screens/playback/playback_controller.dart` — rewrite internals to use WebSocket
- `lib/services/playback_service.dart` — WebSocket URL construction

### Unchanged files:
- All timeline layers, controls, event popup — public API unchanged
- MediaMTX's built-in playback server — not modified
