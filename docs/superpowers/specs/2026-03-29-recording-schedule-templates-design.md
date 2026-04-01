# Recording Schedule Templates

**Date:** 2026-03-29
**Status:** Approved
**Goal:** Add a top-level Schedules page with reusable recording templates, and simplify the camera detail recording section to per-stream template assignment via dropdowns.

---

## Context

Recording rules are currently created per-camera through a buried "MANAGE SCHEDULES" button. Users want:
1. A dedicated page for managing recording schedule templates
2. Easy defaults (pre-built templates for common patterns)
3. Per-stream template assignment directly on the camera detail page

## Navigation

Add "Schedules" to the sidebar after Devices (after the separator). Icon: `Icons.calendar_month_outlined` / `Icons.calendar_month`. Route: `/schedules`. Also add to mobile bottom nav as a 5th item.

---

## Schedules Page

### Layout: List View

Compact list of all schedule templates. Each row:
- Colored dot: orange = continuous (always), green = motion (events)
- Template name (bold)
- Description: days + time range
- Usage count: "X streams"
- Chevron → opens edit dialog

Header: "RECORDING SCHEDULES" title + "+ NEW TEMPLATE" button (tactical style).

### Create/Edit Dialog

Same dialog for both create and edit. Fields:
- Name (text input)
- Mode selector: Continuous / Motion
- Days: chip toggles for Mon-Sun (all selected by default)
- Time range: start and end time pickers (only shown when not all-day)
- Post-event buffer: slider 0-120s (only shown for Motion mode)

### Delete

Templates can only be deleted if:
- They are not a default template (`is_default = false`)
- They are not assigned to any streams

Show a confirmation dialog. If assigned, show "Remove from X streams first" error.

---

## Default Templates

Seeded on first launch when the `schedule_templates` table is empty.

| Name | Mode | Days | Start | End | Post-Event |
|------|------|------|-------|-----|------------|
| 24/7 Continuous | always | 0,1,2,3,4,5,6 | 00:00 | 00:00 | 0 |
| 24/7 Motion | events | 0,1,2,3,4,5,6 | 00:00 | 00:00 | 30 |
| Business Hours | always | 1,2,3,4,5 | 08:00 | 18:00 | 0 |
| After Hours Motion | events | 0,1,2,3,4,5,6 | 18:00 | 08:00 | 30 |
| Weekday Only | always | 1,2,3,4,5 | 00:00 | 00:00 | 0 |

Days use ISO weekday numbers: 0=Sunday, 1=Monday, ... 6=Saturday.

Start/End of 00:00/00:00 means 24-hour rule (the scheduler already handles this: when start == end, the rule matches all day if the day matches).

---

## Camera Detail: Stream Assignment

### Replace Recording Section

Remove the current recording section contents (toggle, segmented control, MANAGE SCHEDULES button). Replace with per-stream template dropdowns:

```
RECORDING
  Main Stream (1920x1080)    [24/7 Continuous     ▾]
  Sub Stream (640x480)       [After Hours Motion  ▾]
```

Each row:
- Stream label: `"{name} ({width}x{height})"` using `CameraStream.displayLabel`
- Dropdown: "None" + all templates from `GET /schedule-templates`
- The currently active template is pre-selected (matched by `template_id` on the recording rule)

If camera has no streams, show a single dropdown labeled "Default" for the main RTSP URL.

### Assignment Behavior

- Selecting a template: creates a recording rule for that camera+stream, copying template fields. The rule stores the `template_id` for UI matching.
- Selecting "None": deletes the recording rule for that camera+stream.
- Changes auto-save immediately (like the current toggle behavior).
- Show green snackbar on success, red on error.

### Existing Custom Rules

Recording rules without a `template_id` (created via the old UI or API) show as "Custom" in the dropdown with italic styling. Selecting a template replaces the custom rule. The old RecordingRulesScreen remains accessible for power users who want fine-grained rule management, but the MANAGE SCHEDULES button is removed from the camera detail page.

---

## Backend: Database

### New Table: schedule_templates (Migration 24)

```sql
CREATE TABLE schedule_templates (
    id TEXT PRIMARY KEY,
    name TEXT NOT NULL,
    mode TEXT NOT NULL CHECK(mode IN ('always', 'events')),
    days TEXT NOT NULL,
    start_time TEXT NOT NULL,
    end_time TEXT NOT NULL,
    post_event_seconds INTEGER NOT NULL DEFAULT 30,
    is_default INTEGER NOT NULL DEFAULT 0,
    created_at TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%fZ', 'now'))
);
```

### Modify: recording_rules table

Add `template_id` column (Migration 24):

```sql
ALTER TABLE recording_rules ADD COLUMN template_id TEXT DEFAULT '';
```

### Seed Default Templates

In the migration or in the Go initialization code, check if `schedule_templates` is empty and insert the 5 default templates with `is_default = 1`.

---

## Backend: API Endpoints

### Schedule Templates CRUD

```
GET    /api/nvr/schedule-templates           → list all templates
POST   /api/nvr/schedule-templates           → create custom template
PUT    /api/nvr/schedule-templates/:id       → update template
DELETE /api/nvr/schedule-templates/:id       → delete (if not default, not assigned)
```

### Stream Schedule Assignment

```
PUT /api/nvr/cameras/:id/stream-schedule
Body: { "stream_id": "...", "template_id": "..." }
```

Logic:
1. If `template_id` is empty: delete existing recording rule for this camera+stream, return 200.
2. If `template_id` is set: look up the template, create or update a recording rule for this camera+stream copying the template's mode, days, start_time, end_time, post_event_seconds. Set `template_id` on the rule. Return 200 with the rule.

The `stream_id` can be empty string for the camera's default/main stream.

---

## Backend: Template Handler

New file: `internal/nvr/api/schedule_templates.go`

```go
type ScheduleTemplateHandler struct {
    DB *db.DB
}
```

Methods: List, Create, Update, Delete. Standard CRUD with validation matching the existing recording rule validation (mode in always/events, days 0-6, time format HH:MM).

---

## Flutter: Files

### New Files
- `lib/models/schedule_template.dart` — ScheduleTemplate model
- `lib/screens/schedules/schedules_screen.dart` — main list page
- `lib/providers/schedule_templates_provider.dart` — data provider

### Modified Files
- `lib/router/app_router.dart` — add `/schedules` route
- `lib/widgets/shell/icon_rail.dart` — add Schedules nav item after Devices
- `lib/widgets/shell/mobile_bottom_nav.dart` — add Schedules as 5th item
- `lib/screens/cameras/camera_detail_screen.dart` — replace recording section with per-stream dropdowns

---

## No Changes To

- The scheduler backend — it still evaluates recording rules the same way. Templates just make it easier to create rules with consistent settings.
- The existing RecordingRulesScreen — it stays as a power-user option, just not linked from camera detail anymore.
- Playback timeline — unchanged.
