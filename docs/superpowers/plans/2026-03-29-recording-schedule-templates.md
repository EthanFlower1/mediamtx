# Recording Schedule Templates Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Add a top-level Schedules page with reusable recording templates and per-stream template assignment on the camera detail screen.

**Architecture:** New `schedule_templates` DB table with CRUD API, seeded defaults, a new Flutter Schedules screen in the left nav, and a reworked camera detail recording section with per-stream template dropdowns.

**Tech Stack:** Go, SQLite, Flutter/Dart, Riverpod, GoRouter

**Spec:** `docs/superpowers/specs/2026-03-29-recording-schedule-templates-design.md`

---

## File Structure

```
internal/nvr/
├── db/
│   ├── migrations.go              # MODIFY — migration 24
│   ├── schedule_templates.go      # CREATE — ScheduleTemplate CRUD + seeding
│   └── recording_rules.go         # MODIFY — add template_id to struct/queries
├── api/
│   ├── schedule_templates.go      # CREATE — template CRUD handlers
│   ├── cameras.go                 # MODIFY — stream-schedule assignment endpoint
│   └── router.go                  # MODIFY — register new routes

clients/flutter/lib/
├── models/
│   └── schedule_template.dart     # CREATE — ScheduleTemplate model
├── providers/
│   └── schedule_templates_provider.dart  # CREATE — data provider
├── screens/
│   └── schedules/
│       └── schedules_screen.dart  # CREATE — main list page
├── router/
│   └── app_router.dart            # MODIFY — add /schedules route
├── widgets/shell/
│   ├── icon_rail.dart             # MODIFY — add Schedules nav item
│   ├── mobile_bottom_nav.dart     # MODIFY — add Schedules as 5th item
│   └── navigation_shell.dart      # MODIFY — update index mapping
└── screens/cameras/
    └── camera_detail_screen.dart  # MODIFY — replace recording section
```

---

### Task 1: DB Migration and ScheduleTemplate CRUD

**Files:**
- Modify: `internal/nvr/db/migrations.go`
- Create: `internal/nvr/db/schedule_templates.go`
- Modify: `internal/nvr/db/recording_rules.go`

- [ ] **Step 1: Add migration 24**

Append to the `migrations` slice in `internal/nvr/db/migrations.go`:

```go
// Migration 24: Schedule templates and template_id on recording rules.
{
    version: 24,
    sql: `
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
        ALTER TABLE recording_rules ADD COLUMN template_id TEXT DEFAULT '';
    `,
},
```

Update the test assertion in `db_test.go` from `23` to `24`.

- [ ] **Step 2: Create schedule_templates.go**

```go
// internal/nvr/db/schedule_templates.go
package db

import (
	"encoding/json"
	"time"

	"github.com/google/uuid"
)

// ScheduleTemplate is a reusable recording schedule.
type ScheduleTemplate struct {
	ID               string `json:"id"`
	Name             string `json:"name"`
	Mode             string `json:"mode"`
	Days             string `json:"days"`
	StartTime        string `json:"start_time"`
	EndTime          string `json:"end_time"`
	PostEventSeconds int    `json:"post_event_seconds"`
	IsDefault        bool   `json:"is_default"`
	CreatedAt        string `json:"created_at"`
}

func (d *DB) ListScheduleTemplates() ([]*ScheduleTemplate, error) {
	rows, err := d.Query(`SELECT id, name, mode, days, start_time, end_time,
		post_event_seconds, is_default, created_at
		FROM schedule_templates ORDER BY is_default DESC, name`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var templates []*ScheduleTemplate
	for rows.Next() {
		t := &ScheduleTemplate{}
		if err := rows.Scan(&t.ID, &t.Name, &t.Mode, &t.Days, &t.StartTime,
			&t.EndTime, &t.PostEventSeconds, &t.IsDefault, &t.CreatedAt); err != nil {
			return nil, err
		}
		templates = append(templates, t)
	}
	return templates, nil
}

func (d *DB) GetScheduleTemplate(id string) (*ScheduleTemplate, error) {
	t := &ScheduleTemplate{}
	err := d.QueryRow(`SELECT id, name, mode, days, start_time, end_time,
		post_event_seconds, is_default, created_at
		FROM schedule_templates WHERE id = ?`, id).Scan(
		&t.ID, &t.Name, &t.Mode, &t.Days, &t.StartTime,
		&t.EndTime, &t.PostEventSeconds, &t.IsDefault, &t.CreatedAt)
	if err != nil {
		return nil, ErrNotFound
	}
	return t, nil
}

func (d *DB) CreateScheduleTemplate(t *ScheduleTemplate) error {
	if t.ID == "" {
		t.ID = uuid.New().String()
	}
	if t.CreatedAt == "" {
		t.CreatedAt = time.Now().UTC().Format("2006-01-02T15:04:05.000Z")
	}
	_, err := d.Exec(`INSERT INTO schedule_templates
		(id, name, mode, days, start_time, end_time, post_event_seconds, is_default, created_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		t.ID, t.Name, t.Mode, t.Days, t.StartTime, t.EndTime,
		t.PostEventSeconds, t.IsDefault, t.CreatedAt)
	return err
}

func (d *DB) UpdateScheduleTemplate(t *ScheduleTemplate) error {
	res, err := d.Exec(`UPDATE schedule_templates
		SET name = ?, mode = ?, days = ?, start_time = ?, end_time = ?, post_event_seconds = ?
		WHERE id = ?`,
		t.Name, t.Mode, t.Days, t.StartTime, t.EndTime, t.PostEventSeconds, t.ID)
	if err != nil {
		return err
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return ErrNotFound
	}
	return nil
}

func (d *DB) DeleteScheduleTemplate(id string) error {
	res, err := d.Exec(`DELETE FROM schedule_templates WHERE id = ? AND is_default = 0`, id)
	if err != nil {
		return err
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return ErrNotFound
	}
	return nil
}

// CountTemplateUsage returns how many recording rules reference a template.
func (d *DB) CountTemplateUsage(templateID string) (int, error) {
	var count int
	err := d.QueryRow(`SELECT COUNT(*) FROM recording_rules WHERE template_id = ?`, templateID).Scan(&count)
	return count, err
}

// SeedDefaultTemplates inserts the 5 default templates if the table is empty.
func (d *DB) SeedDefaultTemplates() error {
	var count int
	d.QueryRow(`SELECT COUNT(*) FROM schedule_templates`).Scan(&count)
	if count > 0 {
		return nil
	}

	defaults := []ScheduleTemplate{
		{Name: "24/7 Continuous", Mode: "always", Days: "[0,1,2,3,4,5,6]", StartTime: "00:00", EndTime: "00:00", PostEventSeconds: 0, IsDefault: true},
		{Name: "24/7 Motion", Mode: "events", Days: "[0,1,2,3,4,5,6]", StartTime: "00:00", EndTime: "00:00", PostEventSeconds: 30, IsDefault: true},
		{Name: "Business Hours", Mode: "always", Days: "[1,2,3,4,5]", StartTime: "08:00", EndTime: "18:00", PostEventSeconds: 0, IsDefault: true},
		{Name: "After Hours Motion", Mode: "events", Days: "[0,1,2,3,4,5,6]", StartTime: "18:00", EndTime: "08:00", PostEventSeconds: 30, IsDefault: true},
		{Name: "Weekday Only", Mode: "always", Days: "[1,2,3,4,5]", StartTime: "00:00", EndTime: "00:00", PostEventSeconds: 0, IsDefault: true},
	}

	for _, t := range defaults {
		t.ID = uuid.New().String()
		t.CreatedAt = time.Now().UTC().Format("2006-01-02T15:04:05.000Z")
		if err := d.CreateScheduleTemplate(&t); err != nil {
			return err
		}
	}
	return nil
}
```

- [ ] **Step 3: Add template_id to RecordingRule struct**

In `internal/nvr/db/recording_rules.go`, add `TemplateID` to the struct:

```go
type RecordingRule struct {
	ID               string `json:"id"`
	CameraID         string `json:"camera_id"`
	StreamID         string `json:"stream_id"`
	TemplateID       string `json:"template_id"`
	Name             string `json:"name"`
	// ... rest unchanged
}
```

Add `template_id` to all SELECT column lists and Scan calls in: `CreateRecordingRule`, `GetRecordingRule`, `ListRecordingRules`, `ListAllEnabledRecordingRules`, `UpdateRecordingRule`. Also add it to the INSERT and UPDATE SQL statements.

- [ ] **Step 4: Verify build and tests**

Run: `go test ./internal/nvr/db/ -v -count=1`
Run: `go build .`
Expected: pass

- [ ] **Step 5: Commit**

```bash
git add internal/nvr/db/
git commit -m "feat(db): add schedule_templates table, seed defaults, template_id on rules"
```

---

### Task 2: Schedule Templates API

**Files:**
- Create: `internal/nvr/api/schedule_templates.go`
- Modify: `internal/nvr/api/cameras.go`
- Modify: `internal/nvr/api/router.go`

- [ ] **Step 1: Create schedule_templates.go handler**

```go
// internal/nvr/api/schedule_templates.go
package api

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"

	"github.com/gin-gonic/gin"

	"github.com/bluenviron/mediamtx/internal/nvr/db"
)

type ScheduleTemplateHandler struct {
	DB *db.DB
}

type scheduleTemplateRequest struct {
	Name             string `json:"name" binding:"required"`
	Mode             string `json:"mode" binding:"required"`
	Days             []int  `json:"days" binding:"required"`
	StartTime        string `json:"start_time" binding:"required"`
	EndTime          string `json:"end_time" binding:"required"`
	PostEventSeconds int    `json:"post_event_seconds"`
}

func (h *ScheduleTemplateHandler) List(c *gin.Context) {
	templates, err := h.DB.ListScheduleTemplates()
	if err != nil {
		apiError(c, http.StatusInternalServerError, "failed to list templates", err)
		return
	}
	if templates == nil {
		templates = []*db.ScheduleTemplate{}
	}
	c.JSON(http.StatusOK, templates)
}

func (h *ScheduleTemplateHandler) Create(c *gin.Context) {
	var req scheduleTemplateRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid request"})
		return
	}

	if req.Mode != "always" && req.Mode != "events" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "mode must be 'always' or 'events'"})
		return
	}

	daysJSON, _ := json.Marshal(req.Days)
	t := &db.ScheduleTemplate{
		Name:             req.Name,
		Mode:             req.Mode,
		Days:             string(daysJSON),
		StartTime:        req.StartTime,
		EndTime:          req.EndTime,
		PostEventSeconds: req.PostEventSeconds,
	}
	if err := h.DB.CreateScheduleTemplate(t); err != nil {
		apiError(c, http.StatusInternalServerError, "failed to create template", err)
		return
	}
	c.JSON(http.StatusCreated, t)
}

func (h *ScheduleTemplateHandler) Update(c *gin.Context) {
	id := c.Param("id")

	existing, err := h.DB.GetScheduleTemplate(id)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "template not found"})
		return
	}

	var req scheduleTemplateRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid request"})
		return
	}

	if req.Mode != "always" && req.Mode != "events" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "mode must be 'always' or 'events'"})
		return
	}

	daysJSON, _ := json.Marshal(req.Days)
	existing.Name = req.Name
	existing.Mode = req.Mode
	existing.Days = string(daysJSON)
	existing.StartTime = req.StartTime
	existing.EndTime = req.EndTime
	existing.PostEventSeconds = req.PostEventSeconds

	if err := h.DB.UpdateScheduleTemplate(existing); err != nil {
		apiError(c, http.StatusInternalServerError, "failed to update template", err)
		return
	}
	c.JSON(http.StatusOK, existing)
}

func (h *ScheduleTemplateHandler) Delete(c *gin.Context) {
	id := c.Param("id")

	t, err := h.DB.GetScheduleTemplate(id)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "template not found"})
		return
	}
	if t.IsDefault {
		c.JSON(http.StatusBadRequest, gin.H{"error": "cannot delete default template"})
		return
	}

	count, err := h.DB.CountTemplateUsage(id)
	if err == nil && count > 0 {
		c.JSON(http.StatusConflict, gin.H{"error": fmt.Sprintf("template is assigned to %d streams, remove assignments first", count)})
		return
	}

	if err := h.DB.DeleteScheduleTemplate(id); err != nil {
		if errors.Is(err, db.ErrNotFound) {
			c.JSON(http.StatusNotFound, gin.H{"error": "template not found"})
			return
		}
		apiError(c, http.StatusInternalServerError, "failed to delete template", err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"message": "template deleted"})
}
```

- [ ] **Step 2: Add stream-schedule assignment endpoint to cameras.go**

Add to `internal/nvr/api/cameras.go`:

```go
// AssignStreamSchedule handles PUT /cameras/:id/stream-schedule — assigns a
// schedule template to a camera stream by creating/updating a recording rule.
func (h *CameraHandler) AssignStreamSchedule(c *gin.Context) {
	cameraID := c.Param("id")

	var req struct {
		StreamID   string `json:"stream_id"`
		TemplateID string `json:"template_id"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid request"})
		return
	}

	cam, err := h.DB.GetCamera(cameraID)
	if err != nil {
		if errors.Is(err, db.ErrNotFound) {
			c.JSON(http.StatusNotFound, gin.H{"error": "camera not found"})
			return
		}
		apiError(c, http.StatusInternalServerError, "get camera", err)
		return
	}

	// Delete existing rule for this camera+stream.
	rules, _ := h.DB.ListRecordingRules(cameraID)
	for _, r := range rules {
		if r.StreamID == req.StreamID {
			h.DB.DeleteRecordingRule(r.ID)
		}
	}

	// If template_id is empty, we just cleared the rule — done.
	if req.TemplateID == "" {
		c.JSON(http.StatusOK, gin.H{"message": "schedule removed"})
		return
	}

	// Look up the template.
	tmpl, err := h.DB.GetScheduleTemplate(req.TemplateID)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "template not found"})
		return
	}

	// Create a recording rule from the template.
	rule := &db.RecordingRule{
		CameraID:         cameraID,
		StreamID:         req.StreamID,
		TemplateID:       req.TemplateID,
		Name:             tmpl.Name,
		Mode:             tmpl.Mode,
		Days:             tmpl.Days,
		StartTime:        tmpl.StartTime,
		EndTime:          tmpl.EndTime,
		PostEventSeconds: tmpl.PostEventSeconds,
		Enabled:          true,
	}
	if err := h.DB.CreateRecordingRule(rule); err != nil {
		apiError(c, http.StatusInternalServerError, "create rule", err)
		return
	}

	_ = cam // used for validation above
	c.JSON(http.StatusOK, rule)
}
```

- [ ] **Step 3: Register routes in router.go**

Add to `internal/nvr/api/router.go` in the `RegisterRoutes` function:

After the existing handler setup section, add:
```go
templateHandler := &ScheduleTemplateHandler{DB: cfg.DB}
```

In the route registration section, add:
```go
// Schedule templates.
protected.GET("/schedule-templates", templateHandler.List)
protected.POST("/schedule-templates", templateHandler.Create)
protected.PUT("/schedule-templates/:id", templateHandler.Update)
protected.DELETE("/schedule-templates/:id", templateHandler.Delete)
```

And add the stream-schedule assignment route near the recording rules section:
```go
protected.PUT("/cameras/:id/stream-schedule", cameraHandler.AssignStreamSchedule)
```

- [ ] **Step 4: Seed templates on NVR init**

In `internal/nvr/nvr.go`, in the `Initialize()` function, after the database is opened, add:
```go
if err := n.database.SeedDefaultTemplates(); err != nil {
    log.Printf("nvr: failed to seed default templates: %v", err)
}
```

- [ ] **Step 5: Verify build**

Run: `go build .`
Run: `go test ./internal/nvr/db/ -count=1`
Expected: pass

- [ ] **Step 6: Commit**

```bash
git add internal/nvr/api/schedule_templates.go internal/nvr/api/cameras.go internal/nvr/api/router.go internal/nvr/nvr.go
git commit -m "feat(api): add schedule template CRUD and stream-schedule assignment"
```

---

### Task 3: Flutter ScheduleTemplate Model and Provider

**Files:**
- Create: `clients/flutter/lib/models/schedule_template.dart`
- Create: `clients/flutter/lib/providers/schedule_templates_provider.dart`

- [ ] **Step 1: Create the model**

```dart
// clients/flutter/lib/models/schedule_template.dart

class ScheduleTemplate {
  final String id;
  final String name;
  final String mode;
  final List<int> days;
  final String startTime;
  final String endTime;
  final int postEventSeconds;
  final bool isDefault;

  const ScheduleTemplate({
    required this.id,
    required this.name,
    required this.mode,
    required this.days,
    required this.startTime,
    required this.endTime,
    this.postEventSeconds = 30,
    this.isDefault = false,
  });

  factory ScheduleTemplate.fromJson(Map<String, dynamic> json) {
    final rawDays = json['days'];
    List<int> days;
    if (rawDays is String) {
      days = (List<dynamic>.from(
        rawDays.isNotEmpty ? (rawDays.startsWith('[') ? _parseJsonList(rawDays) : []) : [],
      )).cast<int>();
    } else if (rawDays is List) {
      days = rawDays.map((d) => d as int).toList();
    } else {
      days = [];
    }

    return ScheduleTemplate(
      id: json['id'] as String? ?? '',
      name: json['name'] as String? ?? '',
      mode: json['mode'] as String? ?? 'always',
      days: days,
      startTime: json['start_time'] as String? ?? '00:00',
      endTime: json['end_time'] as String? ?? '00:00',
      postEventSeconds: json['post_event_seconds'] as int? ?? 30,
      isDefault: json['is_default'] == true || json['is_default'] == 1,
    );
  }

  static List<dynamic> _parseJsonList(String s) {
    try {
      return List<dynamic>.from(
        (s.replaceAll('[', '').replaceAll(']', '').split(','))
            .where((e) => e.trim().isNotEmpty)
            .map((e) => int.parse(e.trim())),
      );
    } catch (_) {
      return [];
    }
  }

  String get modeLabel => mode == 'events' ? 'Motion' : 'Continuous';

  String get daysLabel {
    if (days.length == 7) return 'All days';
    if (days.length == 5 && !days.contains(0) && !days.contains(6)) return 'Mon-Fri';
    if (days.length == 2 && days.contains(0) && days.contains(6)) return 'Weekends';
    const names = ['Sun', 'Mon', 'Tue', 'Wed', 'Thu', 'Fri', 'Sat'];
    return days.map((d) => names[d]).join(', ');
  }

  String get timeLabel {
    if (startTime == '00:00' && endTime == '00:00') return 'All day';
    return '$startTime-$endTime';
  }

  String get description => '$daysLabel • $timeLabel';
}
```

- [ ] **Step 2: Create the provider**

```dart
// clients/flutter/lib/providers/schedule_templates_provider.dart
import 'package:flutter_riverpod/flutter_riverpod.dart';
import '../models/schedule_template.dart';
import 'auth_provider.dart';

final scheduleTemplatesProvider =
    FutureProvider<List<ScheduleTemplate>>((ref) async {
  final api = ref.watch(apiClientProvider);
  if (api == null) return [];

  final res = await api.get<dynamic>('/schedule-templates');
  final data = res.data as List<dynamic>? ?? [];
  return data
      .map((e) => ScheduleTemplate.fromJson(e as Map<String, dynamic>))
      .toList();
});
```

- [ ] **Step 3: Verify**

Run: `cd /Users/ethanflower/personal_projects/mediamtx/clients/flutter && flutter analyze lib/models/schedule_template.dart lib/providers/schedule_templates_provider.dart`
Expected: No errors

- [ ] **Step 4: Commit**

```bash
git add clients/flutter/lib/models/schedule_template.dart clients/flutter/lib/providers/schedule_templates_provider.dart
git commit -m "feat(flutter): add ScheduleTemplate model and provider"
```

---

### Task 4: Schedules Screen

**Files:**
- Create: `clients/flutter/lib/screens/schedules/schedules_screen.dart`

- [ ] **Step 1: Create the screen**

Create the file at `clients/flutter/lib/screens/schedules/schedules_screen.dart`. This is a list page showing all schedule templates with create/edit/delete functionality.

The screen should:
- Be a `ConsumerStatefulWidget`
- Fetch templates via `ref.watch(scheduleTemplatesProvider)`
- Show a header row with "RECORDING SCHEDULES" title and "+ NEW TEMPLATE" HudButton
- Show templates as a list with colored dots (orange for always, green for events), name, description, usage count, and chevron
- Tapping a row opens an edit dialog (same pattern as the existing recording rules dialog)
- The create/edit dialog has: name input, mode dropdown (Continuous/Motion), day chips (Mon-Sun), time pickers (start/end), post-event slider (for motion mode)
- Delete button on non-default templates (with confirmation)

Follow the patterns from `recording_rules_screen.dart` for dialog structure, and use `NvrColors`, `NvrTypography`, `HudButton`, `HudToggle` consistently.

The screen fetches template usage counts via `GET /schedule-templates` (the `is_default` field is already returned). For usage counts, call `GET /schedule-templates` which returns the templates — the usage count can be derived client-side by cross-referencing with recording rules, or we can add it to the API later. For now, skip the usage count display to keep it simple.

- [ ] **Step 2: Verify**

Run: `flutter analyze lib/screens/schedules/schedules_screen.dart`
Expected: No errors

- [ ] **Step 3: Commit**

```bash
git add clients/flutter/lib/screens/schedules/
git commit -m "feat(flutter): add Schedules screen with template CRUD"
```

---

### Task 5: Add Schedules to Navigation

**Files:**
- Modify: `clients/flutter/lib/router/app_router.dart`
- Modify: `clients/flutter/lib/widgets/shell/icon_rail.dart`
- Modify: `clients/flutter/lib/widgets/shell/mobile_bottom_nav.dart`
- Modify: `clients/flutter/lib/widgets/shell/navigation_shell.dart`

- [ ] **Step 1: Add route to app_router.dart**

In `app_router.dart`:

1. Add import: `import '../screens/schedules/schedules_screen.dart';`

2. Update `_indexFromPath()` to add index 5 for schedules:
```dart
int _indexFromPath(String path) {
  if (path.startsWith('/live')) return 0;
  if (path.startsWith('/playback')) return 1;
  if (path.startsWith('/search')) return 2;
  if (path.startsWith('/devices')) return 3;
  if (path.startsWith('/settings')) return 4;
  if (path.startsWith('/schedules')) return 5;
  return 0;
}
```

3. Update `_navigateToIndex()` paths array:
```dart
const paths = ['/live', '/playback', '/search', '/devices', '/settings', '/schedules'];
```

4. Add the route inside the ShellRoute's `routes` list, after the settings route:
```dart
GoRoute(
  path: '/schedules',
  builder: (context, state) => const SchedulesScreen(),
),
```

- [ ] **Step 2: Add nav item to icon_rail.dart**

In `icon_rail.dart`, add to the `_navItems` array after the Devices entry:

```dart
static const _navItems = [
  (icon: Icons.videocam_outlined, activeIcon: Icons.videocam, label: 'Live'),
  (icon: Icons.access_time_outlined, activeIcon: Icons.access_time_filled, label: 'Playback'),
  (icon: Icons.search_outlined, activeIcon: Icons.search, label: 'Search'),
  (icon: Icons.camera_alt_outlined, activeIcon: Icons.camera_alt, label: 'Devices'),
  (icon: Icons.calendar_month_outlined, activeIcon: Icons.calendar_month, label: 'Schedules'),
];
```

Update the separator logic if needed — the separator currently renders before index 3 (Devices). It should now render before index 3 (before Devices and Schedules, which are both config items). Check the existing loop that renders the separator and ensure it still works correctly with 5 items.

Update the Settings icon's `onDestinationSelected` call from `4` to `4` (Settings is still index 4 in the router — the rail maps its 5 items to router indices 0-3 and 5, with Settings at index 4 handled separately).

Actually, the rail items map directly to router indices. Since Schedules is index 5 in the router, the rail needs to map item 4 (Schedules) to router index 5. Update the `onDestinationSelected` callback in the nav item loop: for index `i`, the router index should be `i < 4 ? i : 5` (since items 0-3 map to router 0-3, and item 4 maps to router 5). Settings remains a separate button mapping to router index 4.

- [ ] **Step 3: Add to mobile_bottom_nav.dart**

Add Schedules to the mobile items:
```dart
static const _items = [
  (icon: Icons.videocam_outlined, activeIcon: Icons.videocam, label: 'LIVE'),
  (icon: Icons.access_time_outlined, activeIcon: Icons.access_time_filled, label: 'PLAYBACK'),
  (icon: Icons.search_outlined, activeIcon: Icons.search, label: 'SEARCH'),
  (icon: Icons.calendar_month_outlined, activeIcon: Icons.calendar_month, label: 'SCHED'),
  (icon: Icons.settings_outlined, activeIcon: Icons.settings, label: 'SETTINGS'),
];
```

Update the index mapping in `onTap` callback — mobile item indices need to map to router indices correctly: 0→0, 1→1, 2→2, 3→5 (schedules), 4→4 (settings).

- [ ] **Step 4: Update navigation_shell.dart index mapping**

Update the `selectedIndex` clamping/mapping in `navigation_shell.dart` to handle the new 6-route system (indices 0-5). The desktop rail has 5 items (0-4 where 4=Schedules), settings is separate. Mobile has 5 items.

- [ ] **Step 5: Verify**

Run: `flutter analyze lib/router/ lib/widgets/shell/`
Expected: No errors

- [ ] **Step 6: Commit**

```bash
git add clients/flutter/lib/router/ clients/flutter/lib/widgets/shell/
git commit -m "feat(flutter): add Schedules to navigation (sidebar, mobile nav, router)"
```

---

### Task 6: Replace Camera Detail Recording Section

**Files:**
- Modify: `clients/flutter/lib/screens/cameras/camera_detail_screen.dart`

- [ ] **Step 1: Add imports**

```dart
import '../../models/schedule_template.dart';
import '../../providers/schedule_templates_provider.dart';
```

- [ ] **Step 2: Add template state and fetch**

Add state variable:
```dart
List<ScheduleTemplate> _templates = [];
Map<String, String> _streamTemplateMap = {}; // streamID → templateID
```

In `_fetchCamera()`, after the streams fetch, add:
```dart
// Fetch templates and current assignments.
try {
  final tmplRes = await api.get<dynamic>('/schedule-templates');
  final tmplList = (tmplRes.data as List)
      .map((e) => ScheduleTemplate.fromJson(e as Map<String, dynamic>))
      .toList();
  if (mounted) setState(() => _templates = tmplList);
} catch (_) {}

// Build stream → template assignment map from recording rules.
try {
  final rulesRes = await api.get<dynamic>('/cameras/${widget.cameraId}/recording-rules');
  final rules = rulesRes.data as List<dynamic>? ?? [];
  final map = <String, String>{};
  for (final r in rules) {
    final rule = r as Map<String, dynamic>;
    final streamId = rule['stream_id'] as String? ?? '';
    final templateId = rule['template_id'] as String? ?? '';
    if (templateId.isNotEmpty) {
      map[streamId] = templateId;
    } else {
      map[streamId] = '__custom__';
    }
  }
  if (mounted) setState(() => _streamTemplateMap = map);
} catch (_) {}
```

- [ ] **Step 3: Add assignment save method**

```dart
Future<void> _assignSchedule(String streamId, String templateId) async {
  final api = ref.read(apiClientProvider);
  if (api == null) return;
  try {
    await api.put('/cameras/${widget.cameraId}/stream-schedule', data: {
      'stream_id': streamId,
      'template_id': templateId,
    });
    setState(() {
      if (templateId.isEmpty) {
        _streamTemplateMap.remove(streamId);
      } else {
        _streamTemplateMap[streamId] = templateId;
      }
    });
    if (mounted) {
      ScaffoldMessenger.of(context).showSnackBar(
        SnackBar(
          backgroundColor: NvrColors.success,
          content: Text(templateId.isEmpty ? 'Schedule removed' : 'Schedule updated'),
        ),
      );
    }
  } catch (e) {
    if (mounted) {
      ScaffoldMessenger.of(context).showSnackBar(
        SnackBar(backgroundColor: NvrColors.danger, content: Text('Error: $e')),
      );
    }
  }
}
```

- [ ] **Step 4: Replace the recording section widget**

Replace the entire RECORDING `_SectionCard` with per-stream dropdowns:

```dart
_SectionCard(
  header: 'RECORDING',
  child: Column(
    crossAxisAlignment: CrossAxisAlignment.start,
    children: [
      if (_streams.isEmpty) ...[
        // No streams — single dropdown for default.
        _buildScheduleDropdown('', 'Default'),
      ] else ...[
        for (final stream in _streams) ...[
          _buildScheduleDropdown(stream.id, stream.displayLabel),
          if (stream != _streams.last) const SizedBox(height: 8),
        ],
      ],
    ],
  ),
),
```

And add the helper widget method:

```dart
Widget _buildScheduleDropdown(String streamId, String label) {
  final currentTemplateId = _streamTemplateMap[streamId] ?? '';
  // Validate the value exists in templates.
  final validValue = currentTemplateId == '__custom__'
      ? '__custom__'
      : (_templates.any((t) => t.id == currentTemplateId) ? currentTemplateId : '');

  return Column(
    crossAxisAlignment: CrossAxisAlignment.start,
    children: [
      Text(label, style: NvrTypography.monoLabel),
      const SizedBox(height: 4),
      DropdownButtonFormField<String>(
        value: validValue,
        dropdownColor: NvrColors.bgTertiary,
        style: NvrTypography.monoData,
        isExpanded: true,
        decoration: InputDecoration(
          filled: true,
          fillColor: NvrColors.bgTertiary,
          border: OutlineInputBorder(
            borderRadius: BorderRadius.circular(4),
            borderSide: const BorderSide(color: NvrColors.border),
          ),
          enabledBorder: OutlineInputBorder(
            borderRadius: BorderRadius.circular(4),
            borderSide: const BorderSide(color: NvrColors.border),
          ),
          focusedBorder: OutlineInputBorder(
            borderRadius: BorderRadius.circular(4),
            borderSide: const BorderSide(color: NvrColors.accent),
          ),
          contentPadding: const EdgeInsets.symmetric(horizontal: 12, vertical: 10),
        ),
        items: [
          const DropdownMenuItem(
            value: '',
            child: Text('None', style: NvrTypography.monoData),
          ),
          ..._templates.map((t) => DropdownMenuItem(
            value: t.id,
            child: Text('${t.name} (${t.description})', style: NvrTypography.monoData),
          )),
          if (validValue == '__custom__')
            const DropdownMenuItem(
              value: '__custom__',
              child: Text('Custom', style: TextStyle(
                fontFamily: 'JetBrainsMono',
                fontSize: 12,
                fontStyle: FontStyle.italic,
                color: Color(0xFF737373),
              )),
            ),
        ],
        onChanged: (v) {
          if (v != null && v != '__custom__') {
            _assignSchedule(streamId, v);
          }
        },
      ),
    ],
  );
}
```

- [ ] **Step 5: Remove old recording controls**

Remove: `_recordingEnabled`, `_recordingMode` state variables, the `HudToggle`, `HudSegmentedControl`, and `MANAGE SCHEDULES` button from the recording section. Also remove the `import 'recording_rules_screen.dart';` if no longer needed.

- [ ] **Step 6: Verify**

Run: `flutter analyze lib/screens/cameras/camera_detail_screen.dart`
Expected: No errors

- [ ] **Step 7: Commit**

```bash
git add clients/flutter/lib/screens/cameras/camera_detail_screen.dart
git commit -m "feat(flutter): replace recording section with per-stream template dropdowns"
```

---

### Task 7: End-to-End Verification

- [ ] **Step 1: Run all Go tests**

Run: `cd /Users/ethanflower/personal_projects/mediamtx && go test ./internal/nvr/... -count=1`
Expected: all pass

- [ ] **Step 2: Build full binary**

Run: `go build .`
Expected: builds

- [ ] **Step 3: Flutter analyze**

Run: `cd clients/flutter && flutter analyze lib/`
Expected: no errors

- [ ] **Step 4: Manual smoke test**

1. Start server → verify default templates are seeded (check DB or API: `curl localhost:9997/api/nvr/schedule-templates`)
2. Open app → verify "Schedules" icon appears in left sidebar after Devices
3. Tap Schedules → verify 5 default templates listed
4. Create a custom template → verify it appears in the list
5. Navigate to a camera's detail screen → verify per-stream dropdowns in recording section
6. Assign "24/7 Continuous" to main stream → verify green snackbar
7. Assign "After Hours Motion" to sub stream → verify both persist after navigating away and back
8. Go back to Schedules page → verify usage would reflect (future improvement)

- [ ] **Step 5: Commit**

```bash
git add -A
git commit -m "test: verify recording schedule templates end-to-end"
```
