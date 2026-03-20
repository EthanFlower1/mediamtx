# Recording Schedules Design

## Overview

Add per-camera recording schedules to the NVR subsystem. Cameras can have multiple recording rules with different modes (always, events-only) that apply at different times. Rules are unioned — if any rule says to record, recording happens.

This extends the v1 NVR scope to include motion-event-driven recording. The v1 design deferred motion detection; this spec adds it as part of the recording schedules feature.

## Recording Modes

- **Always** — continuous 24/7 recording for the matched time window
- **Events** — record only when ONVIF motion is detected, plus a configurable post-event buffer
- **Off** — the absence of any matching rule; no recording

"Scheduled" and "Schedule+Events" from the original requirements are expressed by creating multiple rules with different time ranges and modes. There is no explicit "off" rule type — disabling all rules or having no matching rule results in no recording.

## Data Model

### `recording_rules` table

```sql
CREATE TABLE recording_rules (
    id TEXT PRIMARY KEY,
    camera_id TEXT NOT NULL,
    name TEXT NOT NULL,
    mode TEXT NOT NULL CHECK(mode IN ('always', 'events')),
    days TEXT NOT NULL,  -- JSON array of day numbers: 0=Sun .. 6=Sat
    start_time TEXT NOT NULL,  -- "HH:MM"
    end_time TEXT NOT NULL,    -- "HH:MM"
    post_event_seconds INTEGER NOT NULL DEFAULT 30,
    enabled INTEGER NOT NULL DEFAULT 1,
    created_at TEXT NOT NULL,
    updated_at TEXT NOT NULL,
    FOREIGN KEY (camera_id) REFERENCES cameras(id) ON DELETE CASCADE
);
```

### Validation rules

- `name`: non-empty, max 100 characters
- `mode`: must be `always` or `events`
- `days`: non-empty JSON array, values must be integers 0-6
- `start_time`, `end_time`: valid HH:MM format (00:00 to 23:59)
- `start_time` equal to `end_time`: treated as 24-hour coverage (full day)
- `post_event_seconds`: 0-3600 range; ignored when mode is `always`

### Cross-midnight handling

A rule with `start_time: "22:00"` and `end_time: "06:00"` means 10pm to 6am. The `days` array specifies the day the rule **starts** on. A Monday 22:00-06:00 rule covers Monday night into Tuesday morning.

**Matching logic:** A rule matches the current time if:
- Today is in `days` AND `current_time >= start_time`, OR
- Yesterday is in `days` AND `current_time < end_time` (for cross-midnight rules where `start > end`)

### Timezone

Schedule evaluation uses the server's local timezone. DST transitions are a known edge case — rules spanning the DST change hour may match for 0 or 2 hours on those days. A per-server timezone override can be added later if needed.

## Schedule Evaluator

A background goroutine running every 30 seconds in the NVR subsystem.

### Evaluation logic per camera

1. Collect all enabled rules for the camera.
2. Filter to rules matching the current day of week and time of day (using the cross-midnight logic above).
3. Determine effective mode using union logic:
   - If any matching rule has mode `always` → effective mode is `always`
   - Else if any matching rule has mode `events` → effective mode is `events`
   - No matching rules → effective mode is `off`
4. Compare to the camera's current effective mode.
5. If changed:
   - `always`: set `record: true` in YAML for the camera's path, stop any ONVIF event subscription
   - `events`: set `record: false` initially (recording toggled by motion), start ONVIF event subscription if not already running
   - `off`: set `record: false`, stop ONVIF event subscription

### Startup behavior

On NVR start, the scheduler defers its first evaluation until after MediaMTX has completed its initial config load. The scheduler starts a goroutine that waits 5 seconds, then runs the first evaluation. This avoids racing with MediaMTX's initial startup.

### Camera deletion cleanup

When a camera is deleted via the API, the handler calls `scheduler.RemoveCamera(id)` to immediately stop any ONVIF subscription and clean up in-memory state. The `ON DELETE CASCADE` on `recording_rules` handles DB cleanup automatically.

## YAML Writer Changes

### New method: `SetPathValue`

Add a `SetPathValue(pathName, key string, value interface{}) error` method to the YAML writer that modifies a single key within an existing path entry without touching other keys. This is the safe primitive for toggling `record: true/false` without overwriting the `source` URL or other path properties.

The scheduler uses `SetPathValue` exclusively — never `AddPath`.

### Write coalescing and hysteresis

To prevent YAML write storms from rapid motion on/off events:

1. **Hysteresis window**: When the post-buffer expires for events mode, add a 10-second "re-trigger window" before writing `record: false`. If motion recurs within this window, reset the post-buffer timer without any YAML write.

2. **Write coalescing**: The scheduler maintains a pending-writes queue. All YAML changes are batched and written in a single atomic operation with a 500ms coalesce window. Multiple cameras changing state within 500ms result in one YAML write, not N writes.

3. **State-change-only writes**: Only write to YAML when the desired recording state differs from the current state. Track current state in memory.

## ONVIF Event Handling

### Motion detection via PullPoint subscription

When a camera has any active `events` mode rules, the NVR subscribes to the camera's ONVIF event service using PullPoint subscription (poll-based, more reliable than WS-Notify across camera brands). Polls every 2 seconds.

Target event topics:
- `tns1:RuleEngine/CellMotionDetector/Motion`
- `tns1:VideoSource/MotionAlarm`

### Implementation approach

The `use-go/onvif` library's event package has type definitions but no SDK convenience functions for events. The implementation will use raw SOAP calls via `dev.CallMethod()` for:
- `CreatePullPointSubscription` — create the subscription
- `PullMessages` — poll for events
- `Unsubscribe` — clean up on shutdown

The `PullMessagesResponse.NotificationMessage` field is a single struct in the library but cameras return multiple messages. The implementation must unmarshal the raw XML response directly (using `encoding/xml`) to handle multiple notification messages correctly, rather than relying on the library's response type.

### Subscription lifecycle

- **Creation**: Call `CreatePullPointSubscription` with a `TerminationTime` of 60 seconds.
- **Renewal**: Renew the subscription at 80% of the termination interval (every ~48 seconds) via `Renew`.
- **Polling**: Call `PullMessages` every 2 seconds with a 1-second timeout.
- **Error recovery**: On connection error during `PullMessages`, wait 5 seconds, then re-create the subscription from scratch. Log the error.
- **Camera reboot**: Detected as a connection error; same recovery path.
- **Shutdown**: Call `Unsubscribe` on graceful shutdown to free camera resources.
- **Max subscriptions**: If `CreatePullPointSubscription` fails with a resource limit error, log a warning and fall back to `always` mode for that camera.

### Motion state machine per camera

```
idle → [motion detected] → recording → [motion stopped] → post_buffer → [timer expires] → hysteresis → [no motion] → idle
                                                                           ↑                                    |
                                                                           └── [motion during hysteresis] ──────┘
```

- **idle**: no motion, `record: false` (when effective mode is `events`)
- **recording**: motion detected, `record: true` via `SetPathValue`
- **post_buffer**: motion stopped, recording continues for `post_event_seconds`
- **hysteresis**: 10-second window after post-buffer. If motion recurs, go back to `recording` without writing YAML. If no motion, write `record: false` and go to `idle`.

### Fallback

If ONVIF event subscription fails (camera doesn't support events), log a warning and treat `events` rules as `always` for that camera. Better to over-record than miss footage.

## API Endpoints

All endpoints are under `/api/nvr` and require JWT authentication.

### Recording Rules CRUD

```
GET    /cameras/:id/recording-rules      — list all rules for a camera
POST   /cameras/:id/recording-rules      — create a new rule
PUT    /recording-rules/:id              — update an existing rule
DELETE /recording-rules/:id              — delete a rule
```

All endpoints return 404 if the camera or rule ID does not exist.

### Recording Status

```
GET    /cameras/:id/recording-status     — current effective mode, motion state, active rules
```

Returns 404 if camera does not exist.

Response:
```json
{
  "effective_mode": "always|events|off",
  "motion_state": "idle|recording|post_buffer|hysteresis",
  "active_rules": ["rule-id-1", "rule-id-2"],
  "recording": true
}
```

### Rule request body

```json
{
  "name": "Weeknight coverage",
  "mode": "events",
  "days": [1, 2, 3, 4, 5],
  "start_time": "18:00",
  "end_time": "06:00",
  "post_event_seconds": 30,
  "enabled": true
}
```

## UI

### Recording Rules section (per camera)

Located on the Camera Management page or accessible from a camera's detail view.

**Rules list:**
- Table/card list showing all rules for the selected camera
- Columns: name, mode (badge), days (abbreviated), time range, post-buffer (events only), enabled toggle
- Edit and delete actions per rule

**Add/Edit Rule form:**
- Name text input
- Mode selector: Always / Events
- Day checkboxes with shortcut buttons: "Weekdays" (Mon-Fri), "Weekends" (Sat-Sun), "Every day"
- Start time and end time pickers
- Post-event buffer seconds input (shown only when mode is Events)
- Enabled toggle

**Schedule preview:**
- A 7-column (days) × 48-row (30-minute slots) grid
- Color-coded cells: blue = always, amber = events, gray = no coverage
- Overlapping rules show the effective mode per cell (union logic: always wins over events)

**Live status indicator:**
- Per camera, shows: current effective mode, whether recording is active, motion state (for events mode)

## Implementation Notes

### Files to create/modify

**New files:**
- `internal/nvr/db/recording_rules.go` — CRUD queries for recording_rules table
- `internal/nvr/api/recording_rules.go` — HTTP handlers
- `internal/nvr/scheduler/scheduler.go` — background evaluator goroutine with write coalescing
- `internal/nvr/scheduler/motion.go` — ONVIF event subscription and motion state machine
- `ui/src/pages/RecordingRules.tsx` or section in CameraManagement

**Modified files:**
- `internal/nvr/db/migrations.go` — add recording_rules table migration
- `internal/nvr/api/router.go` — register new endpoints
- `internal/nvr/api/cameras.go` — call scheduler.RemoveCamera on delete
- `internal/nvr/nvr.go` — start/stop scheduler on init/close
- `internal/nvr/yamlwriter/writer.go` — add SetPathValue method
- `internal/nvr/onvif/` — add ONVIF event subscription support using raw SOAP via `use-go/onvif`

### Concurrency

- The scheduler goroutine owns all recording state decisions
- ONVIF motion callbacks send events to the scheduler via a channel
- Write coalescing is handled within the scheduler goroutine (no additional locks needed)
- The scheduler is the single writer to camera recording state — no races
