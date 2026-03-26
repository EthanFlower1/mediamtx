# Plan 3: New Features — Camera Groups, Tours & Drag-and-Drop Grid

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Add Camera Groups, Camera Tours, and Drag-and-Drop grid assignment — including Go backend (API + SQLite migrations) and Flutter client integration.

**Architecture:** Backend-first approach. Add SQLite migrations and Go API handlers for groups and tours, then build Flutter models/providers, then integrate into the Camera Panel and Live View grid. Drag-and-drop uses Flutter's `LongPressDraggable`/`DragTarget` system with `GridLayout` persisted via SharedPreferences.

**Tech Stack:** Go (Gin, SQLite), Flutter 3.2+, Riverpod 2.4, Freezed, SharedPreferences

**Spec:** `docs/superpowers/specs/2026-03-26-flutter-nvr-ui-redesign.md`

**Depends on:** Plan 1 (Foundation) and Plan 2 (Core Screens) should be completed first. The Camera Panel and Live View grid from those plans are where these features integrate.

---

## File Structure

### New Go Backend Files
| File | Responsibility |
|---|---|
| `internal/nvr/db/groups.go` | Database CRUD for camera_groups and camera_group_members tables |
| `internal/nvr/db/tours.go` | Database CRUD for tours table |
| `internal/nvr/api/groups.go` | HTTP handlers for `/camera-groups` endpoints |
| `internal/nvr/api/tours.go` | HTTP handlers for `/tours` endpoints |

### Modified Go Backend Files
| File | Changes |
|---|---|
| `internal/nvr/db/migrations.go` | Add migration v19 (groups tables) and v20 (tours table) |
| `internal/nvr/api/router.go` | Register group and tour routes on protected group |

### New Flutter Files
| File | Responsibility |
|---|---|
| `clients/flutter/lib/models/camera_group.dart` | CameraGroup Freezed model |
| `clients/flutter/lib/models/tour.dart` | Tour Freezed model |
| `clients/flutter/lib/providers/groups_provider.dart` | Camera groups CRUD provider |
| `clients/flutter/lib/providers/tours_provider.dart` | Tours CRUD + active tour state |
| `clients/flutter/lib/providers/grid_layout_provider.dart` | Grid slot assignments (SharedPreferences) |
| `clients/flutter/lib/widgets/shell/camera_panel_groups.dart` | Groups section in camera panel |
| `clients/flutter/lib/widgets/shell/camera_panel_tours.dart` | Tours section in camera panel |
| `clients/flutter/lib/widgets/shell/tour_active_pill.dart` | Floating tour-active indicator pill |

### Modified Flutter Files
| File | Changes |
|---|---|
| `clients/flutter/lib/widgets/shell/camera_panel.dart` | Integrate groups + tours sections |
| `clients/flutter/lib/screens/live_view/live_view_screen.dart` | Add DragTarget grid slots, use GridLayout |
| `clients/flutter/lib/screens/live_view/camera_tile.dart` | Add LongPressDraggable wrapper |
| `clients/flutter/lib/widgets/shell/navigation_shell.dart` | Add tour active pill overlay |

---

## Tasks

### Task 1: SQLite Migration — Camera Groups

**Files:**
- Modify: `internal/nvr/db/migrations.go`

- [ ] **Step 1: Add migration v19 for camera groups**

Append to the `migrations` slice in `internal/nvr/db/migrations.go`:

```go
{
    version: 19,
    sql: `
        CREATE TABLE IF NOT EXISTS camera_groups (
            id TEXT PRIMARY KEY,
            name TEXT NOT NULL,
            created_at TEXT NOT NULL DEFAULT (datetime('now')),
            updated_at TEXT NOT NULL DEFAULT (datetime('now'))
        );

        CREATE TABLE IF NOT EXISTS camera_group_members (
            group_id TEXT NOT NULL REFERENCES camera_groups(id) ON DELETE CASCADE,
            camera_id TEXT NOT NULL REFERENCES cameras(id) ON DELETE CASCADE,
            sort_order INTEGER NOT NULL DEFAULT 0,
            PRIMARY KEY (group_id, camera_id)
        );

        CREATE INDEX IF NOT EXISTS idx_group_members_group ON camera_group_members(group_id);
        CREATE INDEX IF NOT EXISTS idx_group_members_camera ON camera_group_members(camera_id);
    `,
},
```

- [ ] **Step 2: Commit**

```bash
git add internal/nvr/db/migrations.go
git commit -m "feat(db): add migration v19 — camera_groups and camera_group_members tables"
```

---

### Task 2: SQLite Migration — Tours

**Files:**
- Modify: `internal/nvr/db/migrations.go`

- [ ] **Step 1: Add migration v20 for tours**

```go
{
    version: 20,
    sql: `
        CREATE TABLE IF NOT EXISTS tours (
            id TEXT PRIMARY KEY,
            name TEXT NOT NULL,
            camera_ids TEXT NOT NULL DEFAULT '[]',
            dwell_seconds INTEGER NOT NULL DEFAULT 10,
            created_at TEXT NOT NULL DEFAULT (datetime('now')),
            updated_at TEXT NOT NULL DEFAULT (datetime('now'))
        );
    `,
},
```

Note: `camera_ids` is a JSON array string. Active state is client-side only.

- [ ] **Step 2: Commit**

```bash
git add internal/nvr/db/migrations.go
git commit -m "feat(db): add migration v20 — tours table"
```

---

### Task 3: Go Database Layer — Camera Groups

**Files:**
- Create: `internal/nvr/db/groups.go`

- [ ] **Step 1: Create groups.go with CRUD operations**

Create `internal/nvr/db/groups.go`:

```go
package db

import (
	"encoding/json"
	"fmt"
	"time"

	"github.com/google/uuid"
)

type CameraGroup struct {
	ID        string   `json:"id"`
	Name      string   `json:"name"`
	CameraIDs []string `json:"camera_ids"`
	CreatedAt string   `json:"created_at"`
	UpdatedAt string   `json:"updated_at"`
}

func (d *DB) CreateGroup(name string, cameraIDs []string) (*CameraGroup, error) {
	id := uuid.New().String()
	now := time.Now().UTC().Format(time.RFC3339)

	tx, err := d.db.Begin()
	if err != nil {
		return nil, fmt.Errorf("begin tx: %w", err)
	}
	defer tx.Rollback()

	_, err = tx.Exec(
		"INSERT INTO camera_groups (id, name, created_at, updated_at) VALUES (?, ?, ?, ?)",
		id, name, now, now,
	)
	if err != nil {
		return nil, fmt.Errorf("insert group: %w", err)
	}

	for i, camID := range cameraIDs {
		_, err = tx.Exec(
			"INSERT INTO camera_group_members (group_id, camera_id, sort_order) VALUES (?, ?, ?)",
			id, camID, i,
		)
		if err != nil {
			return nil, fmt.Errorf("insert member %s: %w", camID, err)
		}
	}

	if err := tx.Commit(); err != nil {
		return nil, fmt.Errorf("commit: %w", err)
	}

	return &CameraGroup{
		ID: id, Name: name, CameraIDs: cameraIDs,
		CreatedAt: now, UpdatedAt: now,
	}, nil
}

func (d *DB) ListGroups() ([]CameraGroup, error) {
	rows, err := d.db.Query("SELECT id, name, created_at, updated_at FROM camera_groups ORDER BY name")
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var groups []CameraGroup
	for rows.Next() {
		var g CameraGroup
		if err := rows.Scan(&g.ID, &g.Name, &g.CreatedAt, &g.UpdatedAt); err != nil {
			return nil, err
		}
		// Fetch member camera IDs
		memberRows, err := d.db.Query(
			"SELECT camera_id FROM camera_group_members WHERE group_id = ? ORDER BY sort_order",
			g.ID,
		)
		if err != nil {
			return nil, err
		}
		for memberRows.Next() {
			var camID string
			if err := memberRows.Scan(&camID); err != nil {
				memberRows.Close()
				return nil, err
			}
			g.CameraIDs = append(g.CameraIDs, camID)
		}
		memberRows.Close()
		if g.CameraIDs == nil {
			g.CameraIDs = []string{}
		}
		groups = append(groups, g)
	}
	return groups, nil
}

func (d *DB) GetGroup(id string) (*CameraGroup, error) {
	var g CameraGroup
	err := d.db.QueryRow(
		"SELECT id, name, created_at, updated_at FROM camera_groups WHERE id = ?", id,
	).Scan(&g.ID, &g.Name, &g.CreatedAt, &g.UpdatedAt)
	if err != nil {
		return nil, err
	}

	rows, err := d.db.Query(
		"SELECT camera_id FROM camera_group_members WHERE group_id = ? ORDER BY sort_order", id,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	for rows.Next() {
		var camID string
		if err := rows.Scan(&camID); err != nil {
			return nil, err
		}
		g.CameraIDs = append(g.CameraIDs, camID)
	}
	if g.CameraIDs == nil {
		g.CameraIDs = []string{}
	}
	return &g, nil
}

func (d *DB) UpdateGroup(id, name string, cameraIDs []string) error {
	now := time.Now().UTC().Format(time.RFC3339)

	tx, err := d.db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()

	_, err = tx.Exec("UPDATE camera_groups SET name = ?, updated_at = ? WHERE id = ?", name, now, id)
	if err != nil {
		return err
	}

	// Replace all members
	_, err = tx.Exec("DELETE FROM camera_group_members WHERE group_id = ?", id)
	if err != nil {
		return err
	}
	for i, camID := range cameraIDs {
		_, err = tx.Exec(
			"INSERT INTO camera_group_members (group_id, camera_id, sort_order) VALUES (?, ?, ?)",
			id, camID, i,
		)
		if err != nil {
			return err
		}
	}

	return tx.Commit()
}

func (d *DB) DeleteGroup(id string) error {
	_, err := d.db.Exec("DELETE FROM camera_groups WHERE id = ?", id)
	return err
}
```

- [ ] **Step 2: Write test for groups CRUD**

Create `internal/nvr/db/groups_test.go` with tests for Create, List, Get, Update, Delete.

- [ ] **Step 3: Run tests**

```bash
go test ./internal/nvr/db/ -run TestGroup -v
```

- [ ] **Step 4: Commit**

```bash
git add internal/nvr/db/groups.go internal/nvr/db/groups_test.go
git commit -m "feat(db): add camera groups CRUD operations"
```

---

### Task 4: Go Database Layer — Tours

**Files:**
- Create: `internal/nvr/db/tours.go`

- [ ] **Step 1: Create tours.go with CRUD operations**

Create `internal/nvr/db/tours.go`:

```go
package db

import (
	"encoding/json"
	"fmt"
	"time"

	"github.com/google/uuid"
)

type Tour struct {
	ID           string   `json:"id"`
	Name         string   `json:"name"`
	CameraIDs    []string `json:"camera_ids"`
	DwellSeconds int      `json:"dwell_seconds"`
	CreatedAt    string   `json:"created_at"`
	UpdatedAt    string   `json:"updated_at"`
}

func (d *DB) CreateTour(name string, cameraIDs []string, dwellSeconds int) (*Tour, error) {
	id := uuid.New().String()
	now := time.Now().UTC().Format(time.RFC3339)

	cameraJSON, err := json.Marshal(cameraIDs)
	if err != nil {
		return nil, fmt.Errorf("marshal camera_ids: %w", err)
	}

	_, err = d.db.Exec(
		"INSERT INTO tours (id, name, camera_ids, dwell_seconds, created_at, updated_at) VALUES (?, ?, ?, ?, ?, ?)",
		id, name, string(cameraJSON), dwellSeconds, now, now,
	)
	if err != nil {
		return nil, err
	}

	return &Tour{
		ID: id, Name: name, CameraIDs: cameraIDs,
		DwellSeconds: dwellSeconds, CreatedAt: now, UpdatedAt: now,
	}, nil
}

func (d *DB) ListTours() ([]Tour, error) {
	rows, err := d.db.Query("SELECT id, name, camera_ids, dwell_seconds, created_at, updated_at FROM tours ORDER BY name")
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var tours []Tour
	for rows.Next() {
		var t Tour
		var cameraJSON string
		if err := rows.Scan(&t.ID, &t.Name, &cameraJSON, &t.DwellSeconds, &t.CreatedAt, &t.UpdatedAt); err != nil {
			return nil, err
		}
		if err := json.Unmarshal([]byte(cameraJSON), &t.CameraIDs); err != nil {
			t.CameraIDs = []string{}
		}
		tours = append(tours, t)
	}
	return tours, nil
}

func (d *DB) GetTour(id string) (*Tour, error) {
	var t Tour
	var cameraJSON string
	err := d.db.QueryRow(
		"SELECT id, name, camera_ids, dwell_seconds, created_at, updated_at FROM tours WHERE id = ?", id,
	).Scan(&t.ID, &t.Name, &cameraJSON, &t.DwellSeconds, &t.CreatedAt, &t.UpdatedAt)
	if err != nil {
		return nil, err
	}
	if err := json.Unmarshal([]byte(cameraJSON), &t.CameraIDs); err != nil {
		t.CameraIDs = []string{}
	}
	return &t, nil
}

func (d *DB) UpdateTour(id, name string, cameraIDs []string, dwellSeconds int) error {
	now := time.Now().UTC().Format(time.RFC3339)
	cameraJSON, err := json.Marshal(cameraIDs)
	if err != nil {
		return err
	}
	_, err = d.db.Exec(
		"UPDATE tours SET name = ?, camera_ids = ?, dwell_seconds = ?, updated_at = ? WHERE id = ?",
		name, string(cameraJSON), dwellSeconds, now, id,
	)
	return err
}

func (d *DB) DeleteTour(id string) error {
	_, err := d.db.Exec("DELETE FROM tours WHERE id = ?", id)
	return err
}
```

- [ ] **Step 2: Write test and run**

```bash
go test ./internal/nvr/db/ -run TestTour -v
```

- [ ] **Step 3: Commit**

```bash
git add internal/nvr/db/tours.go internal/nvr/db/tours_test.go
git commit -m "feat(db): add tours CRUD operations"
```

---

### Task 5: Go API Handlers — Groups

**Files:**
- Create: `internal/nvr/api/groups.go`
- Modify: `internal/nvr/api/router.go`

- [ ] **Step 1: Create groups API handler**

Create `internal/nvr/api/groups.go` following the existing handler pattern (see `router.go` for how other handlers are structured with `gin.Context`):

```go
package api

import (
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/bluenviron/mediamtx/internal/nvr/db"
)

type GroupHandler struct {
	DB    *db.DB
	Audit *AuditLogger
}

type createGroupRequest struct {
	Name      string   `json:"name" binding:"required"`
	CameraIDs []string `json:"camera_ids"`
}

func (h *GroupHandler) List(c *gin.Context) {
	groups, err := h.DB.ListGroups()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	if groups == nil {
		groups = []db.CameraGroup{}
	}
	c.JSON(http.StatusOK, groups)
}

func (h *GroupHandler) Create(c *gin.Context) {
	var req createGroupRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	if req.CameraIDs == nil {
		req.CameraIDs = []string{}
	}

	group, err := h.DB.CreateGroup(req.Name, req.CameraIDs)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	h.Audit.Log(c, "create", "camera_group", group.ID, map[string]any{"name": req.Name})
	c.JSON(http.StatusCreated, group)
}

func (h *GroupHandler) Get(c *gin.Context) {
	group, err := h.DB.GetGroup(c.Param("id"))
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "group not found"})
		return
	}
	c.JSON(http.StatusOK, group)
}

func (h *GroupHandler) Update(c *gin.Context) {
	var req createGroupRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	id := c.Param("id")
	if err := h.DB.UpdateGroup(id, req.Name, req.CameraIDs); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	h.Audit.Log(c, "update", "camera_group", id, map[string]any{"name": req.Name})
	c.JSON(http.StatusOK, gin.H{"status": "updated"})
}

func (h *GroupHandler) Delete(c *gin.Context) {
	id := c.Param("id")
	if err := h.DB.DeleteGroup(id); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	h.Audit.Log(c, "delete", "camera_group", id, nil)
	c.JSON(http.StatusOK, gin.H{"status": "deleted"})
}
```

- [ ] **Step 2: Register routes in router.go**

Add to the `protected` group in `internal/nvr/api/router.go`, following the existing handler registration pattern:

```go
// Camera Groups
groupHandler := &GroupHandler{DB: cfg.DB, Audit: audit}
protected.GET("/camera-groups", groupHandler.List)
protected.POST("/camera-groups", groupHandler.Create)
protected.GET("/camera-groups/:id", groupHandler.Get)
protected.PUT("/camera-groups/:id", groupHandler.Update)
protected.DELETE("/camera-groups/:id", groupHandler.Delete)
```

- [ ] **Step 3: Commit**

```bash
git add internal/nvr/api/groups.go internal/nvr/api/router.go
git commit -m "feat(api): add camera groups endpoints — CRUD on /camera-groups"
```

---

### Task 6: Go API Handlers — Tours

**Files:**
- Create: `internal/nvr/api/tours.go`
- Modify: `internal/nvr/api/router.go`

- [ ] **Step 1: Create tours API handler**

Create `internal/nvr/api/tours.go` — same pattern as groups:

```go
package api

import (
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/bluenviron/mediamtx/internal/nvr/db"
)

type TourHandler struct {
	DB    *db.DB
	Audit *AuditLogger
}

type createTourRequest struct {
	Name         string   `json:"name" binding:"required"`
	CameraIDs    []string `json:"camera_ids" binding:"required"`
	DwellSeconds int      `json:"dwell_seconds"`
}

func (h *TourHandler) List(c *gin.Context) {
	tours, err := h.DB.ListTours()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	if tours == nil {
		tours = []db.Tour{}
	}
	c.JSON(http.StatusOK, tours)
}

func (h *TourHandler) Create(c *gin.Context) {
	var req createTourRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	if req.DwellSeconds <= 0 {
		req.DwellSeconds = 10
	}

	tour, err := h.DB.CreateTour(req.Name, req.CameraIDs, req.DwellSeconds)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	h.Audit.Log(c, "create", "tour", tour.ID, map[string]any{"name": req.Name})
	c.JSON(http.StatusCreated, tour)
}

func (h *TourHandler) Get(c *gin.Context) {
	tour, err := h.DB.GetTour(c.Param("id"))
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "tour not found"})
		return
	}
	c.JSON(http.StatusOK, tour)
}

func (h *TourHandler) Update(c *gin.Context) {
	var req createTourRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	if req.DwellSeconds <= 0 {
		req.DwellSeconds = 10
	}

	id := c.Param("id")
	if err := h.DB.UpdateTour(id, req.Name, req.CameraIDs, req.DwellSeconds); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	h.Audit.Log(c, "update", "tour", id, map[string]any{"name": req.Name})
	c.JSON(http.StatusOK, gin.H{"status": "updated"})
}

func (h *TourHandler) Delete(c *gin.Context) {
	id := c.Param("id")
	if err := h.DB.DeleteTour(id); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	h.Audit.Log(c, "delete", "tour", id, nil)
	c.JSON(http.StatusOK, gin.H{"status": "deleted"})
}
```

- [ ] **Step 2: Register routes in router.go**

```go
// Tours
tourHandler := &TourHandler{DB: cfg.DB, Audit: audit}
protected.GET("/tours", tourHandler.List)
protected.POST("/tours", tourHandler.Create)
protected.GET("/tours/:id", tourHandler.Get)
protected.PUT("/tours/:id", tourHandler.Update)
protected.DELETE("/tours/:id", tourHandler.Delete)
```

- [ ] **Step 3: Build and verify**

```bash
go build ./...
```

- [ ] **Step 4: Commit**

```bash
git add internal/nvr/api/tours.go internal/nvr/api/router.go
git commit -m "feat(api): add tours endpoints — CRUD on /tours"
```

---

### Task 7: Flutter Models — CameraGroup & Tour

**Files:**
- Create: `clients/flutter/lib/models/camera_group.dart`
- Create: `clients/flutter/lib/models/tour.dart`

- [ ] **Step 1: Create CameraGroup model**

Create `clients/flutter/lib/models/camera_group.dart`:

```dart
import 'package:freezed_annotation/freezed_annotation.dart';

part 'camera_group.freezed.dart';
part 'camera_group.g.dart';

@freezed
class CameraGroup with _$CameraGroup {
  const factory CameraGroup({
    required String id,
    required String name,
    @JsonKey(name: 'camera_ids') @Default([]) List<String> cameraIds,
    @JsonKey(name: 'created_at') required String? createdAt,
    @JsonKey(name: 'updated_at') required String? updatedAt,
  }) = _CameraGroup;

  factory CameraGroup.fromJson(Map<String, dynamic> json) =>
      _$CameraGroupFromJson(json);
}
```

- [ ] **Step 2: Create Tour model**

Create `clients/flutter/lib/models/tour.dart`:

```dart
import 'package:freezed_annotation/freezed_annotation.dart';

part 'tour.freezed.dart';
part 'tour.g.dart';

@freezed
class Tour with _$Tour {
  const factory Tour({
    required String id,
    required String name,
    @JsonKey(name: 'camera_ids') @Default([]) List<String> cameraIds,
    @JsonKey(name: 'dwell_seconds') @Default(10) int dwellSeconds,
    @JsonKey(name: 'created_at') required String? createdAt,
    @JsonKey(name: 'updated_at') required String? updatedAt,
  }) = _Tour;

  factory Tour.fromJson(Map<String, dynamic> json) =>
      _$TourFromJson(json);
}
```

- [ ] **Step 3: Run code generation**

```bash
cd clients/flutter && dart run build_runner build --delete-conflicting-outputs
```

- [ ] **Step 4: Commit**

```bash
git add clients/flutter/lib/models/camera_group.* clients/flutter/lib/models/tour.*
git commit -m "feat(models): add CameraGroup and Tour Freezed models"
```

---

### Task 8: Flutter Providers — Groups & Tours

**Files:**
- Create: `clients/flutter/lib/providers/groups_provider.dart`
- Create: `clients/flutter/lib/providers/tours_provider.dart`

- [ ] **Step 1: Create groups provider**

Create `clients/flutter/lib/providers/groups_provider.dart`:

```dart
import 'package:flutter_riverpod/flutter_riverpod.dart';
import '../models/camera_group.dart';
import 'auth_provider.dart';

final groupsProvider = FutureProvider<List<CameraGroup>>((ref) async {
  final api = ref.watch(apiClientProvider);
  if (api == null) return [];
  final res = await api.get('/camera-groups');
  return (res.data as List).map((e) => CameraGroup.fromJson(e as Map<String, dynamic>)).toList();
});
```

- [ ] **Step 2: Create tours provider with active tour state**

Create `clients/flutter/lib/providers/tours_provider.dart`:

```dart
import 'dart:async';
import 'package:flutter_riverpod/flutter_riverpod.dart';
import '../models/tour.dart';
import 'auth_provider.dart';

final toursProvider = FutureProvider<List<Tour>>((ref) async {
  final api = ref.watch(apiClientProvider);
  if (api == null) return [];
  final res = await api.get('/tours');
  return (res.data as List).map((e) => Tour.fromJson(e as Map<String, dynamic>)).toList();
});

class ActiveTourState {
  const ActiveTourState({this.tour, this.currentCameraIndex = 0, this.isPaused = false});
  final Tour? tour;
  final int currentCameraIndex;
  final bool isPaused;

  bool get isActive => tour != null;
  String? get currentCameraId =>
      tour != null && tour!.cameraIds.isNotEmpty
          ? tour!.cameraIds[currentCameraIndex % tour!.cameraIds.length]
          : null;

  ActiveTourState copyWith({Tour? tour, int? currentCameraIndex, bool? isPaused, bool clearTour = false}) {
    return ActiveTourState(
      tour: clearTour ? null : (tour ?? this.tour),
      currentCameraIndex: currentCameraIndex ?? this.currentCameraIndex,
      isPaused: isPaused ?? this.isPaused,
    );
  }
}

class ActiveTourNotifier extends StateNotifier<ActiveTourState> {
  ActiveTourNotifier() : super(const ActiveTourState());
  Timer? _timer;

  void start(Tour tour) {
    stop();
    state = ActiveTourState(tour: tour);
    _startTimer(tour.dwellSeconds);
  }

  void stop() {
    _timer?.cancel();
    _timer = null;
    state = const ActiveTourState();
  }

  void pause() {
    _timer?.cancel();
    state = state.copyWith(isPaused: true);
  }

  void resume() {
    if (state.tour == null) return;
    state = state.copyWith(isPaused: false);
    _startTimer(state.tour!.dwellSeconds);
  }

  void _startTimer(int dwellSeconds) {
    _timer = Timer.periodic(Duration(seconds: dwellSeconds), (_) {
      if (!state.isPaused && state.tour != null) {
        final nextIndex = (state.currentCameraIndex + 1) % state.tour!.cameraIds.length;
        state = state.copyWith(currentCameraIndex: nextIndex);
      }
    });
  }

  @override
  void dispose() {
    _timer?.cancel();
    super.dispose();
  }
}

final activeTourProvider = StateNotifierProvider<ActiveTourNotifier, ActiveTourState>(
  (ref) => ActiveTourNotifier(),
);
```

- [ ] **Step 3: Commit**

```bash
git add clients/flutter/lib/providers/groups_provider.dart clients/flutter/lib/providers/tours_provider.dart
git commit -m "feat(providers): add groups and tours providers with active tour cycling"
```

---

### Task 9: Grid Layout Provider

**Files:**
- Create: `clients/flutter/lib/providers/grid_layout_provider.dart`

- [ ] **Step 1: Create grid layout provider**

Create `clients/flutter/lib/providers/grid_layout_provider.dart`:

```dart
import 'dart:convert';
import 'package:flutter_riverpod/flutter_riverpod.dart';
import 'package:shared_preferences/shared_preferences.dart';
import 'auth_provider.dart';

class GridLayout {
  const GridLayout({this.gridSize = 4, this.slots = const {}});
  final int gridSize; // NxN
  final Map<int, String> slots; // slot index → camera ID

  int get totalSlots => gridSize * gridSize;

  GridLayout copyWith({int? gridSize, Map<int, String>? slots}) {
    return GridLayout(
      gridSize: gridSize ?? this.gridSize,
      slots: slots ?? this.slots,
    );
  }

  Map<String, dynamic> toJson() => {
    'gridSize': gridSize,
    'slots': slots.map((k, v) => MapEntry(k.toString(), v)),
  };

  factory GridLayout.fromJson(Map<String, dynamic> json) {
    final slotsRaw = json['slots'] as Map<String, dynamic>? ?? {};
    return GridLayout(
      gridSize: json['gridSize'] as int? ?? 4,
      slots: slotsRaw.map((k, v) => MapEntry(int.parse(k), v as String)),
    );
  }
}

class GridLayoutNotifier extends StateNotifier<GridLayout> {
  GridLayoutNotifier(this._userId) : super(const GridLayout(gridSize: 2)) {
    _load();
  }

  final String _userId;

  String get _key => 'grid_layout_$_userId';

  Future<void> _load() async {
    final prefs = await SharedPreferences.getInstance();
    final json = prefs.getString(_key);
    if (json != null) {
      state = GridLayout.fromJson(jsonDecode(json) as Map<String, dynamic>);
    }
  }

  Future<void> _save() async {
    final prefs = await SharedPreferences.getInstance();
    await prefs.setString(_key, jsonEncode(state.toJson()));
  }

  void setGridSize(int size) {
    // Preserve slots that still fit
    final newSlots = Map<int, String>.from(state.slots)
      ..removeWhere((k, _) => k >= size * size);
    state = state.copyWith(gridSize: size, slots: newSlots);
    _save();
  }

  void assignCamera(int slot, String cameraId) {
    // Remove camera from any existing slot
    final newSlots = Map<int, String>.from(state.slots)
      ..removeWhere((_, v) => v == cameraId);
    newSlots[slot] = cameraId;
    state = state.copyWith(slots: newSlots);
    _save();
  }

  void removeCamera(int slot) {
    final newSlots = Map<int, String>.from(state.slots)..remove(slot);
    state = state.copyWith(slots: newSlots);
    _save();
  }

  void swapSlots(int from, int to) {
    final newSlots = Map<int, String>.from(state.slots);
    final temp = newSlots[to];
    if (newSlots.containsKey(from)) newSlots[to] = newSlots[from]!;
    if (temp != null) newSlots[from] = temp; else newSlots.remove(from);
    state = state.copyWith(slots: newSlots);
    _save();
  }

  void fillFromGroup(List<String> cameraIds) {
    final newSlots = <int, String>{};
    for (int i = 0; i < cameraIds.length && i < state.totalSlots; i++) {
      newSlots[i] = cameraIds[i];
    }
    state = state.copyWith(slots: newSlots);
    _save();
  }
}

final gridLayoutProvider = StateNotifierProvider<GridLayoutNotifier, GridLayout>((ref) {
  final userId = ref.watch(authProvider).user?.id ?? 'default';
  return GridLayoutNotifier(userId);
});
```

- [ ] **Step 2: Commit**

```bash
git add clients/flutter/lib/providers/grid_layout_provider.dart
git commit -m "feat(providers): add GridLayout provider with per-user persistence"
```

---

### Task 10: Integrate Groups & Tours into Camera Panel

**Files:**
- Create: `clients/flutter/lib/widgets/shell/camera_panel_groups.dart`
- Create: `clients/flutter/lib/widgets/shell/camera_panel_tours.dart`
- Modify: `clients/flutter/lib/widgets/shell/camera_panel.dart`

- [ ] **Step 1: Create groups section widget**

Create `clients/flutter/lib/widgets/shell/camera_panel_groups.dart` — collapsible group headers with camera lists, `+ GROUP` button, group context menu (rename, delete).

- [ ] **Step 2: Create tours section widget**

Create `clients/flutter/lib/widgets/shell/camera_panel_tours.dart` — tour list with start/stop buttons, `+ NEW` button, active tour badge.

- [ ] **Step 3: Integrate into CameraPanel**

Update `clients/flutter/lib/widgets/shell/camera_panel.dart`:
- Replace flat camera list with grouped view from `CameraPanelGroups`
- Add Tours section at bottom from `CameraPanelTours`

- [ ] **Step 4: Commit**

```bash
git add clients/flutter/lib/widgets/shell/
git commit -m "feat(ui): integrate camera groups and tours into camera panel"
```

---

### Task 11: Integrate Drag-and-Drop into Live View

**Files:**
- Modify: `clients/flutter/lib/screens/live_view/live_view_screen.dart`
- Modify: `clients/flutter/lib/screens/live_view/camera_tile.dart`

- [ ] **Step 1: Add DragTarget to grid slots**

In `LiveViewScreen`, wrap each grid slot with `DragTarget<String>`:
- `onWillAcceptWithDetails`: return true if slot is empty or different camera
- `onAcceptWithDetails`: call `gridLayoutProvider.assignCamera(slotIndex, cameraId)`
- Empty slots: show dashed border with "DROP HERE" when not being dragged over, highlight with `accent` border when drag hovers

- [ ] **Step 2: Add LongPressDraggable to CameraTile**

Wrap `CameraTile` with `LongPressDraggable<String>`:
- `data`: camera ID
- `feedback`: semi-transparent version of the tile at 80% opacity with `accent` border
- `childWhenDragging`: empty slot placeholder
- This also works for camera items in the CameraPanel

- [ ] **Step 3: Use GridLayout provider in LiveViewScreen**

Replace the fixed camera list with `gridLayoutProvider`:
```dart
final gridLayout = ref.watch(gridLayoutProvider);
// Build grid from gridLayout.slots
```

- [ ] **Step 4: Commit**

```bash
git add clients/flutter/lib/screens/live_view/
git commit -m "feat(ui): add drag-and-drop camera assignment to live view grid"
```

---

### Task 12: Tour Active Pill & Lifecycle

**Files:**
- Create: `clients/flutter/lib/widgets/shell/tour_active_pill.dart`
- Modify: `clients/flutter/lib/widgets/shell/navigation_shell.dart`

- [ ] **Step 1: Create floating tour pill**

Create `clients/flutter/lib/widgets/shell/tour_active_pill.dart`:

```dart
import 'package:flutter/material.dart';
import 'package:flutter_riverpod/flutter_riverpod.dart';
import '../../theme/nvr_colors.dart';
import '../../providers/tours_provider.dart';

class TourActivePill extends ConsumerWidget {
  const TourActivePill({super.key});

  @override
  Widget build(BuildContext context, WidgetRef ref) {
    final tourState = ref.watch(activeTourProvider);
    if (!tourState.isActive) return const SizedBox.shrink();

    return Positioned(
      top: 12, right: 12,
      child: Container(
        padding: const EdgeInsets.symmetric(horizontal: 12, vertical: 8),
        decoration: BoxDecoration(
          color: NvrColors.bgSecondary,
          border: Border.all(color: NvrColors.accent.withOpacity(0.5)),
          borderRadius: BorderRadius.circular(20),
          boxShadow: [BoxShadow(color: NvrColors.accent.withOpacity(0.15), blurRadius: 12)],
        ),
        child: Row(
          mainAxisSize: MainAxisSize.min,
          children: [
            Icon(Icons.refresh, size: 14, color: NvrColors.accent),
            const SizedBox(width: 6),
            Text(
              tourState.tour!.name,
              style: const TextStyle(
                fontFamily: 'JetBrainsMono', fontSize: 10,
                color: NvrColors.textPrimary, letterSpacing: 0.5,
              ),
            ),
            if (tourState.isPaused) ...[
              const SizedBox(width: 6),
              Text('PAUSED', style: TextStyle(
                fontFamily: 'JetBrainsMono', fontSize: 8,
                color: NvrColors.warning, letterSpacing: 1,
              )),
            ],
            const SizedBox(width: 8),
            GestureDetector(
              onTap: () => ref.read(activeTourProvider.notifier).stop(),
              child: Container(
                padding: const EdgeInsets.all(3),
                decoration: BoxDecoration(
                  shape: BoxShape.circle,
                  color: NvrColors.danger.withOpacity(0.13),
                  border: Border.all(color: NvrColors.danger.withOpacity(0.27)),
                ),
                child: const Icon(Icons.stop, size: 10, color: NvrColors.danger),
              ),
            ),
          ],
        ),
      ),
    );
  }
}
```

- [ ] **Step 2: Add pill to NavigationShell and handle tour lifecycle**

In `navigation_shell.dart`, add the pill as a Stack overlay. Listen to route changes to pause/resume:

```dart
// In the build method, after the main content:
Stack(
  children: [
    child,
    const TourActivePill(),
  ],
)
```

For tour pause/resume on navigation: use a `ref.listen` on the router to detect when leaving/entering `/live`.

- [ ] **Step 3: Commit**

```bash
git add clients/flutter/lib/widgets/shell/tour_active_pill.dart
git add clients/flutter/lib/widgets/shell/navigation_shell.dart
git commit -m "feat(ui): add tour active pill and navigation lifecycle management"
```

---

### Task 13: Build and Verify

- [ ] **Step 1: Run Go tests**

```bash
go test ./internal/nvr/... -v
```

- [ ] **Step 2: Run Flutter analyze**

```bash
cd clients/flutter && flutter analyze
```

- [ ] **Step 3: Build Flutter app**

```bash
cd clients/flutter && flutter build apk --debug 2>&1 | tail -5
```

- [ ] **Step 4: Final commit**

```bash
git add -A
git commit -m "chore: verify build — Plan 3 complete"
```
