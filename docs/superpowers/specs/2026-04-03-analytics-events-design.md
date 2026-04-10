# KAI-20: ONVIF Analytics Event Handling

## Summary

Extend the existing ONVIF event system to handle all Profile T analytics events: line crossing detection, field/intrusion detection, loitering detection, and object counting. These events are parsed from ONVIF notifications, stored in the database, broadcast via SSE, and trigger recording through the existing motion state machine.

## Event Types

New `DetectedEventType` constants in `events.go`:

| Constant            | ONVIF Topic Substrings                                  | Description                                |
| ------------------- | ------------------------------------------------------- | ------------------------------------------ |
| `EventLineCrossing` | "linecrossing", "linecounter"                           | Virtual line crossed by object             |
| `EventIntrusion`    | "fielddetection", "intrusiondetection", "fielddetector" | Object entered defined region              |
| `EventLoitering`    | "loitering"                                             | Object lingered in region beyond threshold |
| `EventObjectCount`  | "objectcount", "counting"                               | Count of objects in region                 |

Topic matching follows the existing pattern: case-insensitive substring match on the ONVIF notification topic string via `classifyTopic()`.

## Event Parsing

Extend `parseNotificationMessages()` to:

1. Classify topics against new event types (after existing motion/tampering checks)
2. Extract metadata from `SimpleItem` elements when available:
   - **Line crossing:** `Direction` property (e.g., "LeftToRight", "RightToLeft")
   - **Object counting:** `Count` or `ObjectCount` property (integer)
   - **Intrusion/Loitering:** no special metadata beyond active/inactive state
3. For object counting: `active = count > 0`, `inactive = count == 0`. When no count property is found, treat any notification as active (camera-dependent behavior).

## Database

### Migration

Single new migration adding a metadata column:

```sql
ALTER TABLE motion_events ADD COLUMN metadata TEXT;
```

The `metadata` column stores JSON for event-type-specific data:

- Line crossing: `{"direction":"LeftToRight"}`
- Object counting: `{"count":5}`
- Others: `null` or `{}`

No new indexes required. The existing `idx_motion_events_camera_time` on `(camera_id, started_at)` covers queries. Event type filtering is done in the WHERE clause.

### New Query Method

`QueryEvents(cameraID string, start, end time.Time, eventTypes []string) ([]MotionEvent, error)`

- Filters by `event_type IN (?)` when eventTypes is non-empty
- Returns all types when eventTypes is empty
- Same time-range logic as existing `QueryMotionEvents()`

The `MotionEvent` struct gains a `Metadata` field (`*string`, nullable JSON).

### Insert Changes

`InsertMotionEvent()` gains an optional `metadata` parameter for the new column.

## Recording Integration

All new event types trigger recording through the existing `MotionSM`:

- In `scheduler.go`, the event dispatcher switch statement is extended to handle `EventLineCrossing`, `EventIntrusion`, `EventLoitering`, and `EventObjectCount`
- Each follows the same path as motion: `active=true` starts recording + creates DB event, `active=false` stops + closes DB event
- The MotionSM's idle/recording/post_buffer/hysteresis state machine applies identically

## SSE Broadcast

New publish methods on `EventBroadcaster`:

- `PublishLineCrossing(camera string)`
- `PublishIntrusion(camera string)`
- `PublishLoitering(camera string)`
- `PublishObjectCount(camera string, count int)`

Event type strings for SSE: `"line_crossing"`, `"intrusion"`, `"loitering"`, `"object_count"`.

## API

### New Endpoint

`GET /cameras/:id/events?type=&date=YYYY-MM-DD`

- `type`: comma-separated event types to filter (e.g., `type=line_crossing,intrusion`). Omit for all types.
- `date`: required, YYYY-MM-DD format
- Response: same shape as motion-events, plus `metadata` field:

```json
[
  {
    "id": 42,
    "camera_id": "front-door",
    "started_at": "2026-04-03T10:15:00Z",
    "ended_at": "2026-04-03T10:15:05Z",
    "event_type": "line_crossing",
    "metadata": { "direction": "LeftToRight" },
    "thumbnail_path": ""
  }
]
```

### Existing Endpoints

- `GET /cameras/:id/motion-events` remains unchanged for backwards compatibility
- `GET /recordings/intensity` gains optional `event_type` query param to filter by type

### Auth

New endpoint uses existing JWT auth middleware (same as all other `/cameras/` routes).

## Files Modified

| File                                  | Changes                                                                                                    |
| ------------------------------------- | ---------------------------------------------------------------------------------------------------------- |
| `internal/nvr/onvif/events.go`        | New event type constants, extended `classifyTopic()`, metadata extraction in `parseNotificationMessages()` |
| `internal/nvr/db/migrations.go`       | New migration: `ALTER TABLE motion_events ADD COLUMN metadata TEXT`                                        |
| `internal/nvr/db/motion_events.go`    | `Metadata` field on struct, `QueryEvents()` method, updated `InsertMotionEvent()`                          |
| `internal/nvr/api/events.go`          | New publish methods for each event type                                                                    |
| `internal/nvr/api/recordings.go`      | New `Events()` handler, updated `Intensity()` for type filtering                                           |
| `internal/nvr/api/router.go`          | New route: `GET /cameras/:id/events`                                                                       |
| `internal/nvr/scheduler/scheduler.go` | Extended event dispatcher for new types                                                                    |

## Testing

- Unit tests for `classifyTopic()` with Profile T topic strings
- Unit tests for metadata extraction from SimpleItem XML
- Unit tests for `QueryEvents()` with type filtering
- Integration test for the new API endpoint
