# Screenshots Page

**Date:** 2026-03-29
**Status:** Approved
**Goal:** Add screenshot capture from live view and a dedicated Screenshots page for viewing, downloading, and managing saved screenshots.

---

## Context

The fullscreen live view has a "Snapshot" button that is currently a placeholder (shows a snackbar but doesn't save anything). The backend already has `CaptureSnapshot()` in `onvif/snapshot.go` that fetches a JPEG from the camera's ONVIF snapshot URI. This feature wires them together and adds a gallery page.

## Capture Flow

1. User taps "Snapshot" button in fullscreen view
2. Flutter calls `POST /cameras/:id/screenshot`
3. Backend calls `CaptureSnapshot()` using the camera's ONVIF snapshot URI with digest/basic auth
4. Image saved to `./screenshots/{cameraID}/{timestamp}.jpg`
5. Row inserted into `screenshots` table
6. Backend returns the screenshot record as JSON
7. Flutter shows green snackbar "Screenshot saved"

---

## Backend: Database

### New Table: screenshots (Migration 25)

```sql
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
```

### Screenshot struct

```go
type Screenshot struct {
    ID        int64  `json:"id"`
    CameraID  string `json:"camera_id"`
    FilePath  string `json:"file_path"`
    FileSize  int64  `json:"file_size"`
    CreatedAt string `json:"created_at"`
}
```

### DB Operations

- `InsertScreenshot(s *Screenshot) error`
- `ListScreenshots(cameraID string, sort string, page, perPage int) ([]*Screenshot, int, error)` — returns screenshots + total count for pagination. `sort` is "newest" or "oldest". `cameraID` empty = all cameras.
- `GetScreenshot(id int64) (*Screenshot, error)`
- `DeleteScreenshot(id int64) error`

---

## Backend: API Endpoints

### Capture Screenshot

```
POST /api/nvr/cameras/:id/screenshot
Response: Screenshot (201)
```

Logic:

1. Get camera from DB
2. Call `onvif.CaptureSnapshot()` with camera's RTSP URL, ONVIF credentials, `./screenshots` dir, camera ID, and snapshot URI
3. Stat the file for size
4. Insert screenshot record
5. Return the record

### List Screenshots (Paginated)

```
GET /api/nvr/screenshots?camera_id=&sort=newest&page=1&per_page=20
Response: { "screenshots": [...], "total": 42, "page": 1, "per_page": 20 }
```

- `camera_id` optional filter
- `sort`: "newest" (default) or "oldest"
- `page`: 1-indexed, default 1
- `per_page`: default 20, max 100

### Download Screenshot

```
GET /api/nvr/screenshots/:id/download
Response: JPEG file (Content-Type: image/jpeg)
```

### Delete Screenshot

```
DELETE /api/nvr/screenshots/:id
Response: { "message": "screenshot deleted" }
```

Deletes both the DB record and the file on disk.

### Serve Screenshot Images

For displaying thumbnails in the gallery, screenshots are served as static files. Add a static file server route:

```
/screenshots/* → serves files from ./screenshots/ directory
```

This already exists in the router for thumbnails at `/thumbnails`. Add an equivalent for screenshots.

---

## Backend: File Structure

New files:

- `internal/nvr/db/screenshots.go` — Screenshot struct + CRUD
- `internal/nvr/api/screenshots.go` — ScreenshotHandler with Capture, List, Download, Delete

Modified files:

- `internal/nvr/db/migrations.go` — Migration 25
- `internal/nvr/api/router.go` — Register new routes + static file server

---

## Flutter: Screenshots Page

### Navigation

Icon: `Icons.photo_library_outlined` / `Icons.photo_library`. Position: after Search, before the separator (index 3 in the sidebar, pushing Devices to 4, Schedules to 5). Route: `/screenshots`.

Router index mapping (after insertion):

- 0: /live
- 1: /playback
- 2: /search
- 3: /screenshots (NEW)
- 4: /devices
- 5: /settings
- 6: /schedules

All navigation components (icon_rail, mobile_bottom_nav, navigation_shell) must be updated to handle the shifted indices. The separator in the sidebar moves to before index 4 (Devices).

### Page Layout

**Header row:** "SCREENSHOTS" title (pageTitle style)

**Filter bar:** Row with:

- Camera dropdown: "All Cameras" + list of cameras from camerasProvider
- Sort dropdown: "Newest" / "Oldest"

**Grid body:** Responsive grid of screenshot cards:

- Desktop (≥600px): 4 columns
- Mobile: 2 columns
- Each card: thumbnail image (aspect ratio 16:9), camera name below, timestamp below that
- Card background: NvrColors.bgSecondary, 1px border, 8px radius

**Tap action:** Opens a dialog/overlay showing the full-size screenshot with:

- Camera name + timestamp in header
- Full-size image
- "DOWNLOAD" button (HudButton tactical)
- "DELETE" button (HudButton danger)

**Pagination:** "LOAD MORE" button at bottom of grid when more pages exist. Shows "{showing} of {total}" count.

### Fullscreen View: Wire Screenshot Button

Replace the placeholder `_takeScreenshot()`:

1. Set loading state on the button
2. Call `POST /cameras/${camera.id}/screenshot`
3. On success: green snackbar "Screenshot saved"
4. On error: red snackbar with error
5. Clear loading state

---

## Flutter: Files

### New Files

- `lib/screens/screenshots/screenshots_screen.dart` — main gallery page

### Modified Files

- `lib/router/app_router.dart` — add `/screenshots` route at index 3
- `lib/widgets/shell/icon_rail.dart` — add Screenshots nav item after Search
- `lib/widgets/shell/mobile_bottom_nav.dart` — add Screenshots item
- `lib/widgets/shell/navigation_shell.dart` — update index mapping
- `lib/screens/live_view/fullscreen_view.dart` — wire \_takeScreenshot to API
