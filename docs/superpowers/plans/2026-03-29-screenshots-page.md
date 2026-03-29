# Screenshots Page Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Add screenshot capture from live view and a paginated Screenshots gallery page for viewing, downloading, and managing saved screenshots.

**Architecture:** New `screenshots` DB table, API endpoints for capture/list/download/delete, a Flutter gallery screen with grid layout and pagination, and the fullscreen snapshot button wired to the capture API.

**Tech Stack:** Go, SQLite, Flutter/Dart, Riverpod, GoRouter

**Spec:** `docs/superpowers/specs/2026-03-29-screenshots-page-design.md`

---

## File Structure

```
internal/nvr/
├── db/
│   ├── migrations.go          # MODIFY — migration 25
│   └── screenshots.go         # CREATE — Screenshot CRUD with pagination
├── api/
│   ├── screenshots.go         # CREATE — capture, list, download, delete handlers
│   └── router.go              # MODIFY — register routes + static /screenshots

clients/flutter/lib/
├── screens/
│   ├── screenshots/
│   │   └── screenshots_screen.dart  # CREATE — gallery page
│   └── live_view/
│       └── fullscreen_view.dart     # MODIFY — wire snapshot button
├── router/
│   └── app_router.dart              # MODIFY — add /screenshots route
├── widgets/shell/
│   ├── icon_rail.dart               # MODIFY — add Screenshots nav item
│   ├── mobile_bottom_nav.dart       # MODIFY — add Screenshots item
│   └── navigation_shell.dart        # MODIFY — update index mapping
```

---

### Task 1: DB Migration and Screenshot CRUD

**Files:**
- Modify: `internal/nvr/db/migrations.go`
- Create: `internal/nvr/db/screenshots.go`
- Modify: `internal/nvr/db/db_test.go`

- [ ] **Step 1: Add migration 25**

Append to the `migrations` slice in `internal/nvr/db/migrations.go`:

```go
// Migration 25: Screenshots table.
{
    version: 25,
    sql: `
        CREATE TABLE screenshots (
            id INTEGER PRIMARY KEY AUTOINCREMENT,
            camera_id TEXT NOT NULL,
            file_path TEXT NOT NULL,
            file_size INTEGER NOT NULL DEFAULT 0,
            created_at TEXT NOT NULL,
            FOREIGN KEY (camera_id) REFERENCES cameras(id) ON DELETE CASCADE
        );
        CREATE INDEX idx_screenshots_camera ON screenshots(camera_id);
        CREATE INDEX idx_screenshots_created ON screenshots(created_at);
    `,
},
```

Update `db_test.go`: change migration version assertion to `25`, add `"screenshots"` to table existence check list.

- [ ] **Step 2: Create screenshots.go**

```go
// internal/nvr/db/screenshots.go
package db

import (
	"fmt"
	"time"
)

// Screenshot is a user-captured JPEG from a camera's live view.
type Screenshot struct {
	ID        int64  `json:"id"`
	CameraID  string `json:"camera_id"`
	FilePath  string `json:"file_path"`
	FileSize  int64  `json:"file_size"`
	CreatedAt string `json:"created_at"`
}

// InsertScreenshot inserts a new screenshot record.
func (d *DB) InsertScreenshot(s *Screenshot) error {
	if s.CreatedAt == "" {
		s.CreatedAt = time.Now().UTC().Format("2006-01-02T15:04:05.000Z")
	}
	res, err := d.Exec(`INSERT INTO screenshots (camera_id, file_path, file_size, created_at)
		VALUES (?, ?, ?, ?)`, s.CameraID, s.FilePath, s.FileSize, s.CreatedAt)
	if err != nil {
		return err
	}
	s.ID, _ = res.LastInsertId()
	return nil
}

// ListScreenshots returns paginated screenshots with optional camera filter.
// sort is "newest" or "oldest". Returns (screenshots, totalCount, error).
func (d *DB) ListScreenshots(cameraID, sort string, page, perPage int) ([]*Screenshot, int, error) {
	if page < 1 {
		page = 1
	}
	if perPage < 1 || perPage > 100 {
		perPage = 20
	}

	orderDir := "DESC"
	if sort == "oldest" {
		orderDir = "ASC"
	}

	// Count total.
	countSQL := "SELECT COUNT(*) FROM screenshots"
	args := []interface{}{}
	if cameraID != "" {
		countSQL += " WHERE camera_id = ?"
		args = append(args, cameraID)
	}
	var total int
	if err := d.QueryRow(countSQL, args...).Scan(&total); err != nil {
		return nil, 0, err
	}

	// Fetch page.
	querySQL := fmt.Sprintf("SELECT id, camera_id, file_path, file_size, created_at FROM screenshots")
	queryArgs := []interface{}{}
	if cameraID != "" {
		querySQL += " WHERE camera_id = ?"
		queryArgs = append(queryArgs, cameraID)
	}
	querySQL += fmt.Sprintf(" ORDER BY created_at %s LIMIT ? OFFSET ?", orderDir)
	queryArgs = append(queryArgs, perPage, (page-1)*perPage)

	rows, err := d.Query(querySQL, queryArgs...)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()

	var screenshots []*Screenshot
	for rows.Next() {
		s := &Screenshot{}
		if err := rows.Scan(&s.ID, &s.CameraID, &s.FilePath, &s.FileSize, &s.CreatedAt); err != nil {
			return nil, 0, err
		}
		screenshots = append(screenshots, s)
	}
	return screenshots, total, nil
}

// GetScreenshot returns a screenshot by ID.
func (d *DB) GetScreenshot(id int64) (*Screenshot, error) {
	s := &Screenshot{}
	err := d.QueryRow(`SELECT id, camera_id, file_path, file_size, created_at
		FROM screenshots WHERE id = ?`, id).Scan(&s.ID, &s.CameraID, &s.FilePath, &s.FileSize, &s.CreatedAt)
	if err != nil {
		return nil, ErrNotFound
	}
	return s, nil
}

// DeleteScreenshot deletes a screenshot by ID.
func (d *DB) DeleteScreenshot(id int64) error {
	res, err := d.Exec("DELETE FROM screenshots WHERE id = ?", id)
	if err != nil {
		return err
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return ErrNotFound
	}
	return nil
}
```

- [ ] **Step 3: Verify**

Run: `go test ./internal/nvr/db/ -v -count=1`
Run: `go build ./internal/nvr/db/`
Expected: pass

- [ ] **Step 4: Commit**

```bash
git add internal/nvr/db/
git commit -m "feat(db): add screenshots table with paginated CRUD"
```

---

### Task 2: Screenshots API Handlers

**Files:**
- Create: `internal/nvr/api/screenshots.go`
- Modify: `internal/nvr/api/router.go`

- [ ] **Step 1: Create screenshots.go handler**

```go
// internal/nvr/api/screenshots.go
package api

import (
	"errors"
	"fmt"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"time"

	"github.com/gin-gonic/gin"

	"github.com/bluenviron/mediamtx/internal/nvr/db"
	"github.com/bluenviron/mediamtx/internal/nvr/onvif"
)

// ScreenshotHandler handles screenshot capture and management.
type ScreenshotHandler struct {
	DB            *db.DB
	EncryptionKey []byte
}

func (h *ScreenshotHandler) decryptPassword(stored string) string {
	if len(h.EncryptionKey) == 0 || stored == "" {
		return stored
	}
	decrypted, err := decrypt(h.EncryptionKey, stored)
	if err != nil {
		return stored
	}
	return decrypted
}

// Capture handles POST /cameras/:id/screenshot — captures a JPEG from the
// camera's ONVIF snapshot URI and saves it.
func (h *ScreenshotHandler) Capture(c *gin.Context) {
	cameraID := c.Param("id")

	cam, err := h.DB.GetCamera(cameraID)
	if err != nil {
		if errors.Is(err, db.ErrNotFound) {
			c.JSON(http.StatusNotFound, gin.H{"error": "camera not found"})
			return
		}
		apiError(c, http.StatusInternalServerError, "get camera", err)
		return
	}

	if cam.SnapshotURI == "" && cam.ONVIFEndpoint == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "camera has no snapshot URI configured"})
		return
	}

	password := h.decryptPassword(cam.ONVIFPassword)

	// Capture screenshot using ONVIF snapshot.
	outputDir := filepath.Join(".", "screenshots", cam.ID)
	filePath, err := onvif.CaptureSnapshot(
		cam.RTSPURL, cam.ONVIFUsername, password,
		outputDir, cam.ID, cam.SnapshotURI,
	)
	if err != nil {
		log.Printf("[screenshots] capture failed for camera %s: %v", cameraID, err)
		c.JSON(http.StatusServiceUnavailable, gin.H{"error": "failed to capture screenshot: " + err.Error()})
		return
	}

	// Stat the file for size.
	var fileSize int64
	if info, statErr := os.Stat(filePath); statErr == nil {
		fileSize = info.Size()
	}

	// The CaptureSnapshot returns a web path like /thumbnails/event_xxx.jpg
	// but we saved to ./screenshots/{cameraID}/. Fix the path to be the actual file path.
	// Actually, CaptureSnapshot saves to the outputDir we provide. Let's use the returned path.
	// But CaptureSnapshot returns /thumbnails/filename — we need /screenshots/cameraID/filename.
	// We need to construct the web path ourselves from the actual file path.
	webPath := "/screenshots/" + cam.ID + "/" + filepath.Base(filePath)

	s := &db.Screenshot{
		CameraID:  cameraID,
		FilePath:  webPath,
		FileSize:  fileSize,
		CreatedAt: time.Now().UTC().Format("2006-01-02T15:04:05.000Z"),
	}
	if err := h.DB.InsertScreenshot(s); err != nil {
		apiError(c, http.StatusInternalServerError, "save screenshot record", err)
		return
	}

	c.JSON(http.StatusCreated, s)
}

// List handles GET /screenshots — paginated list with optional camera filter.
func (h *ScreenshotHandler) List(c *gin.Context) {
	cameraID := c.Query("camera_id")
	sort := c.DefaultQuery("sort", "newest")
	page, _ := strconv.Atoi(c.DefaultQuery("page", "1"))
	perPage, _ := strconv.Atoi(c.DefaultQuery("per_page", "20"))

	screenshots, total, err := h.DB.ListScreenshots(cameraID, sort, page, perPage)
	if err != nil {
		apiError(c, http.StatusInternalServerError, "list screenshots", err)
		return
	}
	if screenshots == nil {
		screenshots = []*db.Screenshot{}
	}

	c.JSON(http.StatusOK, gin.H{
		"screenshots": screenshots,
		"total":       total,
		"page":        page,
		"per_page":    perPage,
	})
}

// Download handles GET /screenshots/:id/download — serves the JPEG file.
func (h *ScreenshotHandler) Download(c *gin.Context) {
	id, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid id"})
		return
	}

	s, err := h.DB.GetScreenshot(id)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "screenshot not found"})
		return
	}

	// Convert web path to disk path: /screenshots/xxx/file.jpg → ./screenshots/xxx/file.jpg
	diskPath := "." + s.FilePath
	if _, err := os.Stat(diskPath); err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "screenshot file not found on disk"})
		return
	}

	c.FileAttachment(diskPath, filepath.Base(diskPath))
}

// Delete handles DELETE /screenshots/:id — deletes file and DB record.
func (h *ScreenshotHandler) Delete(c *gin.Context) {
	id, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid id"})
		return
	}

	s, err := h.DB.GetScreenshot(id)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "screenshot not found"})
		return
	}

	// Delete file from disk.
	diskPath := "." + s.FilePath
	os.Remove(diskPath) // ignore error — file may already be gone

	if err := h.DB.DeleteScreenshot(id); err != nil {
		apiError(c, http.StatusInternalServerError, "delete screenshot", err)
		return
	}

	c.JSON(http.StatusOK, gin.H{"message": "screenshot deleted"})
}
```

IMPORTANT: The `CaptureSnapshot` function from `onvif/snapshot.go` saves files to the `outputDir` you provide, but returns a path prefixed with `/thumbnails/`. You need to either:
- Use the returned path and adapt it, OR
- Construct the web path yourself from the outputDir + filename

Read the `CaptureSnapshot` and `saveSnapshot` functions in `onvif/snapshot.go` to understand exactly what path is returned, and adjust the handler accordingly. The key is that the web path stored in the DB should start with `/screenshots/` so the static file server can serve it.

Note: The `decrypt` function may not exist in the `api` package. Check if there's a `decryptPassword` helper on CameraHandler and use the same approach. If needed, use `crypto.Decrypt(h.EncryptionKey, stored)` from the crypto package, or check how other handlers handle encrypted passwords.

- [ ] **Step 2: Register routes in router.go**

In `internal/nvr/api/router.go`:

Add handler setup:
```go
screenshotHandler := &ScreenshotHandler{DB: cfg.DB, EncryptionKey: cfg.EncryptionKey}
```

Add static file serving (near the existing `/thumbnails` line):
```go
engine.Static("/screenshots", "./screenshots")
```

Add protected routes:
```go
// Screenshots.
protected.POST("/cameras/:id/screenshot", screenshotHandler.Capture)
protected.GET("/screenshots", screenshotHandler.List)
protected.GET("/screenshots/:id/download", screenshotHandler.Download)
protected.DELETE("/screenshots/:id", screenshotHandler.Delete)
```

- [ ] **Step 3: Verify**

Run: `go build .`
Expected: builds

- [ ] **Step 4: Commit**

```bash
git add internal/nvr/api/screenshots.go internal/nvr/api/router.go
git commit -m "feat(api): add screenshot capture, list, download, delete endpoints"
```

---

### Task 3: Flutter Screenshots Screen

**Files:**
- Create: `clients/flutter/lib/screens/screenshots/screenshots_screen.dart`

- [ ] **Step 1: Create the screen**

A `ConsumerStatefulWidget` called `ScreenshotsScreen` with:

**State:**
```dart
List<dynamic> _screenshots = [];
int _total = 0;
int _page = 1;
final int _perPage = 20;
String _cameraFilter = '';
String _sort = 'newest';
bool _loading = true;
```

**Header:** Row with "SCREENSHOTS" (pageTitle), spacer, and filter controls.

**Filter bar:** Row with:
- Camera dropdown: "All Cameras" + cameras from API (`GET /cameras`)
- Sort dropdown: "Newest" / "Oldest"

**Grid body:** `GridView.builder` with:
- `crossAxisCount`: 4 on desktop (width ≥ 800), 2 on mobile
- `childAspectRatio`: 1.2
- Each card:
  - `Container` with NvrColors.bgSecondary background, 1px border, 8px radius
  - `Image.network(serverUrl + screenshot['file_path'])` with fit: BoxFit.cover for the thumbnail
  - Below image: camera name (looked up from cameras) + timestamp in small text
  - GestureDetector onTap → opens full-size dialog

**Full-size dialog:** `showDialog` with:
- Full-width image
- Camera name + formatted timestamp
- Row of buttons: "DOWNLOAD" (HudButton tactical) and "DELETE" (HudButton danger)
- Download calls `GET /screenshots/:id/download` (can use `launchUrl` or just show a message that the file is served at the URL)
- Delete calls `DELETE /screenshots/:id`, refreshes list

**Pagination:** At bottom of grid, show "Showing X of Y" text and "LOAD MORE" HudButton (if more pages exist). Load more increments page and appends results.

**Fetch method:**
```dart
Future<void> _fetchScreenshots({bool append = false}) async {
  final api = ref.read(apiClientProvider);
  if (api == null) return;
  if (!append) setState(() => _loading = true);
  try {
    final res = await api.get<dynamic>('/screenshots', queryParameters: {
      if (_cameraFilter.isNotEmpty) 'camera_id': _cameraFilter,
      'sort': _sort,
      'page': '${append ? _page + 1 : 1}',
      'per_page': '$_perPage',
    });
    final data = res.data as Map<String, dynamic>;
    final list = data['screenshots'] as List<dynamic>? ?? [];
    if (mounted) {
      setState(() {
        if (append) {
          _screenshots.addAll(list);
          _page++;
        } else {
          _screenshots = list;
          _page = 1;
        }
        _total = data['total'] as int? ?? 0;
        _loading = false;
      });
    }
  } catch (e) {
    if (mounted) setState(() => _loading = false);
  }
}
```

Use NvrColors, NvrTypography, HudButton consistently. Follow existing screen patterns.

- [ ] **Step 2: Verify**

Run: `flutter analyze lib/screens/screenshots/`
Expected: No errors

- [ ] **Step 3: Commit**

```bash
git add clients/flutter/lib/screens/screenshots/
git commit -m "feat(flutter): add Screenshots gallery screen with pagination"
```

---

### Task 4: Add Screenshots to Navigation

**Files:**
- Modify: `clients/flutter/lib/router/app_router.dart`
- Modify: `clients/flutter/lib/widgets/shell/icon_rail.dart`
- Modify: `clients/flutter/lib/widgets/shell/mobile_bottom_nav.dart`
- Modify: `clients/flutter/lib/widgets/shell/navigation_shell.dart`

Screenshots goes after Search (index 3), shifting Devices→4, Settings→5, Schedules→6.

- [ ] **Step 1: Update app_router.dart**

Add import:
```dart
import '../screens/screenshots/screenshots_screen.dart';
```

Update `_indexFromPath()`:
```dart
int _indexFromPath(String path) {
  if (path.startsWith('/live')) return 0;
  if (path.startsWith('/playback')) return 1;
  if (path.startsWith('/search')) return 2;
  if (path.startsWith('/screenshots')) return 3;
  if (path.startsWith('/devices')) return 4;
  if (path.startsWith('/settings')) return 5;
  if (path.startsWith('/schedules')) return 6;
  return 0;
}
```

Update `_navigateToIndex()`:
```dart
const paths = ['/live', '/playback', '/search', '/screenshots', '/devices', '/settings', '/schedules'];
```

Add GoRoute in ShellRoute routes (after search, before devices):
```dart
GoRoute(
  path: '/screenshots',
  builder: (context, state) => const ScreenshotsScreen(),
),
```

- [ ] **Step 2: Update icon_rail.dart**

Insert Screenshots after Search in `_navItems`:
```dart
static const _navItems = [
  (icon: Icons.videocam_outlined, activeIcon: Icons.videocam, label: 'Live'),
  (icon: Icons.access_time_outlined, activeIcon: Icons.access_time_filled, label: 'Playback'),
  (icon: Icons.search_outlined, activeIcon: Icons.search, label: 'Search'),
  (icon: Icons.photo_library_outlined, activeIcon: Icons.photo_library, label: 'Screenshots'),
  (icon: Icons.camera_alt_outlined, activeIcon: Icons.camera_alt, label: 'Devices'),
  (icon: Icons.calendar_month_outlined, activeIcon: Icons.calendar_month, label: 'Schedules'),
];
```

Update the `onTap` index mapping and `isActive` check. The rail now has 6 items mapping to router indices 0,1,2,3,4,6 (5 is Settings, handled separately). Update:
- Items 0-4: map to router index `i`
- Item 5 (Schedules): map to router index `6`

So: `widget.onDestinationSelected(i < 5 ? i : 6)`

For active state: router index 6 → rail item 5.

Also update the separator — it should render before index 4 (Devices) now. Check the existing separator logic and adjust.

- [ ] **Step 3: Update mobile_bottom_nav.dart**

Insert Screenshots after Search:
```dart
static const _items = [
  (icon: Icons.videocam_outlined, activeIcon: Icons.videocam, label: 'LIVE'),
  (icon: Icons.access_time_outlined, activeIcon: Icons.access_time_filled, label: 'PLAYBACK'),
  (icon: Icons.search_outlined, activeIcon: Icons.search, label: 'SEARCH'),
  (icon: Icons.photo_library_outlined, activeIcon: Icons.photo_library, label: 'PHOTOS'),
  (icon: Icons.calendar_month_outlined, activeIcon: Icons.calendar_month, label: 'SCHED'),
  (icon: Icons.settings_outlined, activeIcon: Icons.settings, label: 'SETTINGS'),
];
```

Update mobile index mapping: 0→0, 1→1, 2→2, 3→3 (screenshots), 4→6 (schedules), 5→5 (settings).

Update selectedIndex mapping for highlights.

- [ ] **Step 4: Update navigation_shell.dart**

Update index clamping/mapping to handle 7 routes (0-6). Ensure router index 6 (schedules) maps correctly in both desktop and mobile contexts.

- [ ] **Step 5: Verify**

Run: `flutter analyze lib/router/ lib/widgets/shell/`
Expected: No errors

- [ ] **Step 6: Commit**

```bash
git add clients/flutter/lib/router/ clients/flutter/lib/widgets/shell/
git commit -m "feat(flutter): add Screenshots to navigation after Search"
```

---

### Task 5: Wire Fullscreen Snapshot Button

**Files:**
- Modify: `clients/flutter/lib/screens/live_view/fullscreen_view.dart`

- [ ] **Step 1: Replace _takeScreenshot()**

Replace the placeholder method (lines 86-93) with:

```dart
bool _capturing = false;

Future<void> _takeScreenshot() async {
  if (_capturing || !mounted) return;
  setState(() => _capturing = true);
  final api = ref.read(apiClientProvider);
  if (api == null) {
    setState(() => _capturing = false);
    return;
  }
  try {
    await api.post<dynamic>('/cameras/${widget.camera.id}/screenshot');
    if (mounted) {
      ScaffoldMessenger.of(context).showSnackBar(
        const SnackBar(backgroundColor: NvrColors.success, content: Text('Screenshot saved')),
      );
    }
  } catch (e) {
    if (mounted) {
      ScaffoldMessenger.of(context).showSnackBar(
        SnackBar(backgroundColor: NvrColors.danger, content: Text('Screenshot failed: $e')),
      );
    }
  } finally {
    if (mounted) setState(() => _capturing = false);
  }
}
```

Add `_capturing` to the state variables (near other state like `_muted`, `_aiEnabled`).

- [ ] **Step 2: Update the Snapshot pill button to show loading state**

Find the Snapshot `_PillButton` (around line 250-255) and update it:

```dart
_PillButton(
  icon: _capturing ? Icons.hourglass_empty : Icons.photo_camera,
  label: _capturing ? 'Saving...' : 'Snapshot',
  onTap: _takeScreenshot,
),
```

- [ ] **Step 3: Verify**

Run: `flutter analyze lib/screens/live_view/fullscreen_view.dart`
Expected: No errors

- [ ] **Step 4: Commit**

```bash
git add clients/flutter/lib/screens/live_view/fullscreen_view.dart
git commit -m "feat(flutter): wire fullscreen snapshot button to capture API"
```

---

### Task 6: End-to-End Verification

- [ ] **Step 1: Run all Go tests**

Run: `cd /Users/ethanflower/personal_projects/mediamtx && go test ./internal/nvr/... -count=1`
Expected: all pass

- [ ] **Step 2: Build**

Run: `go build .`
Expected: builds

- [ ] **Step 3: Flutter analyze**

Run: `cd clients/flutter && flutter analyze lib/`
Expected: no errors

- [ ] **Step 4: Manual smoke test**

1. Start server
2. Open fullscreen view → tap Snapshot → verify "Screenshot saved" snackbar
3. Navigate to Screenshots page in sidebar → verify screenshot appears in grid
4. Tap screenshot → verify full-size dialog opens
5. Delete screenshot → verify it disappears
6. Test camera filter and sort dropdowns
7. Test pagination with many screenshots

- [ ] **Step 5: Commit**

```bash
git add -A
git commit -m "test: verify screenshots feature end-to-end"
```
