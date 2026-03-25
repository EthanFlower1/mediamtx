# Production-Quality Playback Design

**Date:** 2026-03-25
**Goal:** Transform playback into enterprise-grade quality competing with Milestone XProtect and Verkada
**Target Scale:** 64-256+ cameras, 90+ day retention, multi-site

## Context

Audit of the current playback implementation revealed that the primary issues (slow playlist generation, seek inaccuracy, snap-back, gap confusion) stem from a small number of specific bugs rather than fundamental architectural limitations. The HLS-based approach is sound — Frigate proves HLS can deliver excellent playback UX. The fixes are targeted.

### Root Cause Analysis

**Server-side root cause:** `hls.go` hard-codes `#EXTINF:1.0` for every fragment. The `scanFragments()` function reads byte offsets but never extracts timing from fMP4 `trun`/`tfhd` boxes. This means the HLS player builds a wrong internal timeline, and all client-side position mapping operates on incorrect data.

**Client-side cascading bugs:**
- `_isSeeking` flag never reset on error → UI freeze
- Position stream race condition after seek → snap-back
- Gap snapping off-by-one → wrong segment boundary behavior
- No error handling in `_openPlayers()` → cascading failures

## Design

### 1. Fragment Index Schema & Write Path

New `recording_fragments` table stores pre-computed metadata for every fMP4 fragment:

```sql
CREATE TABLE recording_fragments (
  id INTEGER PRIMARY KEY AUTOINCREMENT,
  recording_id INTEGER NOT NULL,
  fragment_index INTEGER NOT NULL,
  byte_offset INTEGER NOT NULL,
  size INTEGER NOT NULL,
  duration_ms REAL NOT NULL,
  is_keyframe BOOLEAN NOT NULL,
  timestamp_ms INTEGER NOT NULL,
  FOREIGN KEY (recording_id) REFERENCES recordings(id) ON DELETE CASCADE
);
CREATE INDEX idx_fragments_recording ON recording_fragments(recording_id, fragment_index);
```

New column on `recordings` table: `init_size INTEGER` (ftyp + moov byte size).

**Write path:** `OnSegmentComplete` callback in `nvr.go` scans the just-written fMP4 file once and bulk-inserts all fragment rows. Real durations extracted from `trun` boxes (sample_duration / timescale = actual seconds). Init size (ftyp + moov) stored on the recording row.

**Why at segment complete:** The recorder already calls `OnSegmentComplete` when a file is done. Scanning a completed local file adds negligible delay and avoids complexity in the hot write path.

**Scale validation:** 256 cameras × 86,400 fragments/day = ~22M rows/day. SQLite in WAL mode handles 50,000-100,000+ inserts/second. At 256 inserts/second sustained load, this uses <1% of write capacity. Batching (flush every 5-10 seconds per recorder goroutine) provides additional headroom.

**Storage overhead:** ~17 bytes per fragment row. 90 days × 256 cameras ≈ 33 GB of index data — roughly 0.003% of video storage.

### 2. HLS Playlist Generation

With the fragment index, playlist generation becomes a pure database query with no file I/O.

**New flow:**
1. Client requests `/api/nvr/vod/{cameraId}/playlist.m3u8?date=2026-03-25`
2. Server queries `recordings` joined with `recording_fragments` for that camera+date
3. Builds m3u8 directly from DB rows with real durations, exact byte ranges, correct init sizes
4. No `scanFragments()` call, no file opens

**Fixes applied:**
- `#EXTINF:{actual_duration}` from `duration_ms` instead of hard-coded `1.0`
- `#EXT-X-TARGETDURATION` set to `ceil(max(duration_ms))` across all fragments
- `#EXT-X-MAP` byte range uses stored `init_size` per recording file
- Correct `#EXT-X-DISCONTINUITY` between recording files

**Gap handling:** Gaps remain invisible in the playlist — this is correct HLS behavior. The playlist is a contiguous media timeline. Gaps are handled client-side using recording segments from `/api/nvr/timeline`, which maps wall-clock time (with gaps) to player position (without gaps).

**Fallback for un-indexed recordings:** During migration, if a recording has no fragment rows, fall back to current `scanFragments()` approach. Old recordings play immediately while backfill runs.

**Caching:** In-memory LRU cache keyed by `(cameraId, date)`. Invalidate when a new recording completes for that camera+date. Historical dates (before today) cached indefinitely.

### 3. Client-Side Bug Fixes (PlaybackController)

**Fix 1: _isSeeking race condition + error safety**
- Wrap `seek()` body in try/finally — `_isSeeking = false` always executes
- After setting `_isSeeking = false`, ignore position stream updates for 100ms debounce window to prevent stale buffered events from causing snap-back

**Fix 2: Position stream validation**
- When position update arrives, compare against last known seek target
- Discard position updates that jump backward >2 seconds from current `_position` without user-initiated seek
- Eliminates snap-back from delayed events after discontinuity crossings

**Fix 3: Gap snapping boundary fix**
- Change containment check from `posTime < seg.endTime` to `posTime <= seg.endTime` (inclusive end)
- Snap to nearest segment boundary (prev end or next start) instead of always snapping forward
- Clamp to last segment's end time when seeking past all segments

**Fix 4: _openPlayers() error handling**
- Wrap `player.open()` in try/catch per camera
- If a camera fails, log error, skip, continue with remaining cameras
- If all cameras fail, set `_error` and return gracefully

**Fix 5: Discontinuity-aware position mapping**
- With accurate EXTINF durations, player's internal position matches cumulative duration in segment index
- Rebuild segment index using actual recording durations from the API

### 4. Timeline Functionality

New features added as CustomPainter layers on the existing ComposableTimeline architecture.

**Motion intensity layer:**
- Heatmap-style intensity graph behind recording bars
- Motion event counts bucketed by time interval (adapts to zoom: 1 min at high zoom, 15 min at low zoom)
- Semi-transparent filled area chart, color intensity proportional to event density
- New endpoint: `GET /api/nvr/timeline/intensity?camera_id=X&date=Y&bucket_seconds=Z`

**Bookmarks:**
- New `bookmarks` table:
  ```sql
  CREATE TABLE bookmarks (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    camera_id TEXT NOT NULL,
    timestamp TEXT NOT NULL,
    label TEXT NOT NULL,
    created_by TEXT,
    created_at TEXT NOT NULL,
    FOREIGN KEY (camera_id) REFERENCES cameras(id) ON DELETE CASCADE
  );
  CREATE INDEX idx_bookmarks_camera_time ON bookmarks(camera_id, timestamp);
  ```
- Bookmark layer painter: triangular markers at bookmark positions
- Tap to see label, long-press to edit/delete
- API: CRUD on `/api/nvr/bookmarks`
- Transport control button to skip between bookmarks

**Cross-day continuous playback:**
- When playback reaches end of current date's last segment, auto-fetch next day's playlist
- Timeline stays single-day view with forward/back date buttons
- `_continuousMode` flag on PlaybackController: segment end triggers `setSelectedDate(nextDay)` + `seek(Duration.zero)`

**Thumbnail preview:**
- Server endpoint: `GET /api/nvr/thumbnail?camera_id=X&time=RFC3339`
- Extracts single frame from nearest keyframe using fragment index
- Tooltip above timeline at hovered position (desktop) or long-press (mobile)
- Client-side LRU cache (max ~50 per session)

### 5. Performance Targets

| Metric | Target | Current |
|--------|--------|---------|
| Playlist generation | <50ms | Unbounded (file scanning) |
| Seek-to-first-frame | <500ms | Several seconds |
| Timeline render (10K+ events) | 60fps | Unknown at scale |
| Thumbnail preview | <200ms | N/A |

### 6. Migration Strategy

**Schema migration:** Adds `recording_fragments` table and `init_size` column on `recordings`.

**Background backfill:**
- Goroutine starts on server boot
- Queries recordings with no fragments
- Scans in reverse chronological order (newest first — recent playback benefits immediately)
- Progress logged every 100 files
- Idempotent — safe to restart if interrupted

**Playback during migration:** If fragments exist for a recording, use DB query path. If not, fall back to `scanFragments()`.

### 7. Rollout Order

1. Fragment index schema + write path (new recordings indexed immediately)
2. Migration backfill (old recordings indexed in background)
3. HLS playlist generation rewrite (switch from file scanning to DB queries)
4. Client-side bug fixes (seeking, snap-back, gap handling, error safety)
5. Timeline features (intensity, bookmarks, cross-day, thumbnails)

Steps 1-3 are server-only, deployable without Flutter changes. Step 4 is client-only. Step 5 requires both server and client changes.

## Files Affected

**Server (Go):**
- `internal/nvr/db/recordings.go` — new fragment table, queries, migration
- `internal/nvr/db/bookmarks.go` — new file for bookmark CRUD
- `internal/nvr/nvr.go` — OnSegmentComplete: scan + index fragments
- `internal/nvr/api/hls.go` — rewrite playlist generation to use DB
- `internal/nvr/api/recordings.go` — fix timezone, add intensity/thumbnail endpoints
- `internal/nvr/api/bookmarks.go` — new file for bookmark API
- `internal/nvr/api/router.go` — new routes

**Client (Flutter):**
- `lib/screens/playback/playback_controller.dart` — bug fixes (seeking, error handling, gap snapping, position validation)
- `lib/screens/playback/timeline/composable_timeline.dart` — new layers (intensity, bookmarks)
- `lib/screens/playback/timeline/intensity_layer.dart` — new file
- `lib/screens/playback/timeline/bookmark_layer.dart` — new file
- `lib/models/bookmark.dart` — new model
- `lib/providers/bookmarks_provider.dart` — new provider
- `lib/providers/timeline_intensity_provider.dart` — new provider
- `lib/services/playback_service.dart` — thumbnail URL builder
