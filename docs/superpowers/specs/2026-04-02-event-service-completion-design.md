# KAI-110: Event Service Completion

## Summary

Complete the ONVIF event service by adding GetEventProperties (mandatory for Profile S and T) and support for four additional event topics: DigitalInput, SignalLoss, HardwareFailure, and Relay. Currently only motion and tampering events are handled.

## New Event Types

Extend `DetectedEventType` in `events.go` with:

| Event Type           | DB Value           | ONVIF Topic Keywords                                |
| -------------------- | ------------------ | --------------------------------------------------- |
| EventDigitalInput    | "digital_input"    | "digitalinput", "digital_input", "logicalstate"     |
| EventSignalLoss      | "signal_loss"      | "signalloss", "videoloss", "videosource/signalloss" |
| EventHardwareFailure | "hardware_failure" | "failure", "hardwarefailure", "processorusage"      |
| EventRelay           | "relay"            | "relay", "relayoutput", "digitaloutput"             |

These are classified in `classifyTopic()` using lowercased topic string matching, consistent with existing motion/tampering classification.

## GetEventProperties

New `GetEventProperties()` method on `EventSubscriber` that:

1. Sends the GetEventProperties SOAP request to the camera's event service endpoint
2. Parses the `TopicSet` from the response XML
3. Maps each topic to our `DetectedEventType` enum
4. Returns `[]DetectedEventType` representing what the camera supports

Called during camera probe. The result is stored as a JSON array in a new `supported_event_topics` column on the `cameras` table (e.g., `["motion","tampering","digital_input"]`).

## Database Changes

**New migration**: Add `supported_event_topics TEXT DEFAULT ''` to the `cameras` table.

No changes to the `motion_events` table — it already has an `event_type TEXT` column that supports arbitrary event type strings. The new event types are stored there alongside existing motion and tampering events.

## Scheduler Integration

In the event callback (scheduler.go), handle new event types following the same pattern as tampering:

- **Active=true**: `InsertMotionEvent()` with the event type, broadcast via SSE
- **Active=false**: `EndMotionEvent()` to close the event

No recording triggering for these event types — they are informational only.

## SSE Broadcasting

Add publish methods to `EventBroadcaster` for each new event type:

- `PublishDigitalInput(cameraID, active)`
- `PublishSignalLoss(cameraID, active)`
- `PublishHardwareFailure(cameraID, active)`
- `PublishRelay(cameraID, active)`

These follow the existing pattern used by `PublishTampering()`.

## Files Changed

| File                                  | Change                                                                             |
| ------------------------------------- | ---------------------------------------------------------------------------------- |
| `internal/nvr/onvif/events.go`        | New event type constants, extended classifyTopic(), GetEventProperties SOAP method |
| `internal/nvr/onvif/client.go`        | Call GetEventProperties during probe, return supported topics                      |
| `internal/nvr/db/migrations.go`       | New migration for supported_event_topics column                                    |
| `internal/nvr/db/cameras.go`          | Update Camera struct and queries for supported_event_topics                        |
| `internal/nvr/scheduler/scheduler.go` | Handle new event types in event callback                                           |
| `internal/nvr/api/events.go`          | New SSE publish methods for each event type                                        |

## Out of Scope

- Using supported topics to filter subscriptions (subscribe to all, ignore unknown)
- Recording triggering for new event types
- UI changes to display new event types or supported topics
