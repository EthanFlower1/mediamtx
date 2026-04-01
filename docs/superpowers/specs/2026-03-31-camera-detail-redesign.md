# Camera Detail Screen Redesign

## Goal

Redesign the camera detail/configuration screen to use stream-centric settings cards, a single save button with no auto-saves, per-stream retention with inline storage estimates, and a logical section order.

## Current Problems

1. AI settings have their own save button, separate from the main save button
2. AI settings sit between retention and recording sections, breaking logical flow
3. Inconsistent save behavior: stream roles and recording schedules auto-save on change, AI requires its own button, retention/ONVIF require the main save button
4. Retention is per-camera but should be per-stream (different streams may warrant different retention)
5. No storage estimates — users have no guidance on how retention settings translate to disk usage

## Design

### Layout: Two-Column (Preserved)

The existing responsive two-column layout is preserved:

- **Left column**: Live preview (16:9 CameraTile) + 2x2 stat tiles (Uptime, Storage, Events Today, Retention). Unchanged from current implementation.
- **Right column**: All configuration controls, redesigned as described below. Single-column scroll.
- **Narrow screens**: Stacks to single column (left on top, right below). Unchanged behavior.

### Right Column Section Order

1. **Streams** — collapsible per-stream cards
2. **AI Detection** — camera-level settings
3. **Advanced** — collapsed section (ONVIF, connection, imaging)
4. **Save Changes** — single button at bottom

### Stream Cards

Each camera stream is rendered as a collapsible card.

**Collapsed state** (default for all streams): Single row showing:
- Stream name + resolution (e.g., "Main Stream · 1920×1080")
- Recording mode + retention summary (e.g., "Always · 3d/365d")
- Total estimated storage for this stream (e.g., "~57 GB")

**Expanded state** (tap to toggle): Full card containing three sub-sections:

1. **Roles** — Tappable tag chips for `live_view`, `recording`, `ai_detection`, `mobile`. Active roles are highlighted, inactive are dimmed. Toggling a role updates local state only (no auto-save).

2. **Recording Schedule** — Dropdown to select a schedule template (None, Always 24/7, Events Only, or custom templates). Changing updates local state only.

3. **Retention** — Two `AnalogSlider` widgets:
   - **No-Event Recordings** (0–90 days): How long to keep recordings that don't overlap any motion event. Shows "OFF" at 0.
   - **Event Recordings** (0–730 days): How long to keep recordings that overlap a motion event. Shows "OFF" at 0.
   
   Each slider shows an inline storage estimate below it that updates live as the user drags:
   - No-event estimate: `~45 GB` (based on bitrate × hours/day × retention days)
   - Event estimate: `~4.2 GB (1.3 events/day avg)` (based on historical event frequency, or fixed assumption for new streams)

### Storage Estimation

**Server-side endpoint**: `GET /api/nvr/cameras/:id/storage-estimate`

Returns per-stream estimated bytes for given retention settings. The server calculates estimates using:

1. **Actual bitrate** — derived from `SUM(file_size) / SUM(duration_ms)` on recent recordings for each stream. This is the most accurate source.
2. **Codec-based fallback** — for streams with no recording history, estimate bitrate from resolution + codec (e.g., 1080p H.264 ≈ 4 Mbps, 1080p H.265 ≈ 2.5 Mbps, 480p H.264 ≈ 1 Mbps).
3. **Event frequency** — for event retention estimates, use average events/day from the last 7 days of `motion_events` data. Fall back to a fixed assumption (1 hour of events per day) when no history exists.

**Request**: Query parameters for the retention values to estimate:
```
GET /cameras/:id/storage-estimate?retention_days=3&event_retention_days=365
```

**Response**:
```json
{
  "streams": [
    {
      "stream_id": "abc-123",
      "stream_name": "Main Stream",
      "bitrate_bps": 4200000,
      "bitrate_source": "observed",
      "no_event_bytes": 48318382080,
      "event_bytes": 12884901888,
      "event_frequency": 1.3,
      "event_frequency_source": "historical",
      "total_bytes": 61203283968
    }
  ],
  "total_bytes": 61203283968
}
```

The Flutter client calls this endpoint on initial load and debounces re-calls as the user drags retention sliders (e.g., 300ms debounce). The response populates the inline estimates below each slider.

### AI Detection Section

Camera-level section (not per-stream). Contains:
- **Enable toggle** (HudToggle)
- **Confidence** slider (AnalogSlider, 0.2–0.9)
- **Track Timeout** slider (AnalogSlider, 1–30s)
- **Detection Stream** dropdown (select from available streams)

All update local state only. No dedicated save button — saves with the main Save Changes button.

### Advanced Section

Single collapsible section containing expandable sub-sections:
- **ONVIF Configuration** — endpoint, username, password, probe button
- **Stream Settings** — camera name, RTSP URL, sub-stream URL, snapshot URI
- **Imaging** — brightness, contrast, saturation sliders

All update local state only. Save with main button.

### Single Save Button

One "SAVE CHANGES" button at the bottom of the right column. Clicking it:

1. Saves general settings (name, RTSP, ONVIF) via `PUT /cameras/:id`
2. Saves per-stream retention via new endpoint (see backend changes below)
3. Saves per-stream roles via `PUT /streams/:id/roles` for each changed stream
4. Saves per-stream recording schedule via `PUT /cameras/:id/stream-schedule` for each changed stream
5. Saves AI settings via `PUT /cameras/:id/ai`
6. Refreshes camera data from server
7. Shows success/error snackbar

No settings auto-save on change. Every change is held in local widget state until the user explicitly saves.

### Per-Stream Retention: Backend Changes

Currently retention is per-camera (`cameras.event_retention_days`, `cameras.detection_retention_days`, `cameras.retention_days`). This needs to move to per-stream.

**Schema migration 28**: Add retention columns to `camera_streams` table:
```sql
ALTER TABLE camera_streams ADD COLUMN retention_days INTEGER NOT NULL DEFAULT 0;
ALTER TABLE camera_streams ADD COLUMN event_retention_days INTEGER NOT NULL DEFAULT 0;
```

The `detection_retention_days` remains camera-level (event data cleanup is camera-scoped since events are per-camera, not per-stream).

**New endpoint**: `PUT /api/nvr/streams/:id/retention`
```json
{
  "retention_days": 3,
  "event_retention_days": 365
}
```

**Scheduler changes**: The retention cleanup loop iterates streams instead of cameras for recording deletion. For each stream with `retention_days > 0`:
- Delete no-event recordings for that stream older than `retention_days`
- If `event_retention_days > 0`, delete event recordings for that stream older than `event_retention_days`
- Otherwise (legacy), delete all recordings older than `retention_days`

The camera-level `retention_days` and `event_retention_days` columns remain as fallbacks for streams that haven't been configured (both 0). The scheduler checks the stream-level values first, falls back to camera-level.

**Recording-to-stream association**: Recordings are currently associated with cameras via `camera_id`. To support per-stream retention, recordings need a `stream_id`. This requires:
- Migration: `ALTER TABLE recordings ADD COLUMN stream_id TEXT DEFAULT ''`
- The recording writer (scheduler/YAML config) must tag recordings with the stream ID they came from
- Retention queries filter by `stream_id` when deleting

### CameraStream Model Update (Flutter)

Add to the Freezed `CameraStream` model:
```dart
@JsonKey(name: 'retention_days') @Default(0) int retentionDays,
@JsonKey(name: 'event_retention_days') @Default(0) int eventRetentionDays,
```

### Stat Tile: Retention Display

The "RETENTION" stat tile in the left column shows a summary derived from all streams. If all streams share the same retention, show `3d / 365d`. If they differ, show `Mixed` or the range.

## Out of Scope

- Recording rules full editor (separate screen, unchanged)
- Detection zones editor (placeholder, unchanged)
- Audio settings (placeholder, unchanged)
- Mobile/narrow layout changes beyond existing responsive behavior
