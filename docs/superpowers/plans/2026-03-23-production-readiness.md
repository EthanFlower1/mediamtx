# NVR Production Readiness Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Fix 30 production-readiness issues (8 critical, 8 high, 8 medium, 6 low) so the NVR can be sold to customers.

**Architecture:** Fixes are grouped by feature area to minimize file touches per task. Each task is independently committable. Tasks are ordered by severity — critical fixes first, then high, medium, low.

**Tech Stack:** Go (Gin, SQLite), React + TypeScript + Tailwind CSS

---

## File Structure

| File                                     | Task(s) | Changes                                                       |
| ---------------------------------------- | ------- | ------------------------------------------------------------- |
| `ui/src/pages/LiveView.tsx`              | 1       | WebRTC retry UI, offline camera indicator                     |
| `ui/src/pages/Playback.tsx`              | 2       | HTTPS URLs, sync fix, keyboard shortcuts, responsive timeline |
| `ui/src/hooks/useNotifications.ts`       | 3       | WS error handling, increase history                           |
| `ui/src/components/Toast.tsx`            | 3       | Longer dismiss, never auto-dismiss errors                     |
| `internal/nvr/api/auth.go`               | 4       | CSRF validation, refresh token rotation                       |
| `internal/nvr/api/router.go`             | 4       | CSRF middleware                                               |
| `internal/nvr/db/db.go`                  | 5       | Backup/restore methods                                        |
| `ui/src/pages/Settings.tsx`              | 5, 8    | Backup UI, help text, threshold config, audit CSV export      |
| `ui/src/pages/ClipSearch.tsx`            | 6       | Error toasts on save failure, semantic search status          |
| `ui/src/pages/CameraManagement.tsx`      | 7       | Discovery timeout/cancel, RTSP validation, confidence slider  |
| `ui/src/pages/UserManagement.tsx`        | 9       | Self-service password change                                  |
| `ui/src/pages/Recordings.tsx`            | 10      | Calendar date picker                                          |
| `ui/src/components/AnalyticsOverlay.tsx` | 11      | Adaptive polling, FPS monitoring                              |
| `ui/src/components/PTZControls.tsx`      | 12      | Click feedback, error toast                                   |
| Various UI files                         | 13      | Mobile viewport fixes                                         |

---

### Task 1: Live View — WebRTC Retry + Offline Indicator (Critical #1, High #9)

**Files:**

- Modify: `ui/src/pages/LiveView.tsx`

**Issues fixed:**

- #1 CRITICAL: WebRTC failures show blank video with no retry
- #9 HIGH: No offline indicator on grid tiles

- [ ] **Step 1: Add connection state tracking and retry UI to the video component**

Find the WebRTC connection code (the `useEffect` that creates `RTCPeerConnection`). Add state:

```typescript
const [connState, setConnState] = useState<
  "connecting" | "connected" | "failed"
>("connecting");
const retryCountRef = useRef(0);
```

In the peer connection setup:

- Set `setConnState('connecting')` at start
- On `pc.onconnectionstatechange`: if `connected` → `setConnState('connected')`, if `failed`/`disconnected` → `setConnState('failed')`
- In the `.catch()` handler (currently empty): `setConnState('failed')`
- Add auto-retry with exponential backoff (3s, 6s, 12s, max 30s) up to 5 attempts

Add overlay UI inside the video container:

```tsx
{connState === 'failed' && (
  <div className="absolute inset-0 bg-black/80 flex flex-col items-center justify-center gap-3 z-10">
    <svg className="w-8 h-8 text-nvr-danger" ...><path d="..." /></svg>
    <p className="text-sm text-nvr-text-secondary">Connection failed</p>
    <button onClick={retryConnection} className="text-xs bg-nvr-accent hover:bg-nvr-accent-hover text-white px-3 py-1.5 rounded-lg">
      Retry
    </button>
  </div>
)}
{connState === 'connecting' && (
  <div className="absolute inset-0 bg-black/60 flex items-center justify-center z-10">
    <svg className="w-6 h-6 text-nvr-accent animate-spin" .../>
  </div>
)}
```

- [ ] **Step 2: Add offline indicator to grid tiles**

In the camera grid tile rendering, add an overlay when `camera.status !== 'online'`:

```tsx
{
  camera.status !== "online" && (
    <div className="absolute top-2 right-2 bg-red-500/90 text-white text-[10px] font-bold px-1.5 py-0.5 rounded z-20">
      OFFLINE
    </div>
  );
}
```

- [ ] **Step 3: Verify and commit**

Run: `cd ui && npx tsc --noEmit`
Expected: No type errors

```bash
git add ui/src/pages/LiveView.tsx
git commit -m "fix(ui): add WebRTC retry UI and offline camera indicator in live view"
```

---

### Task 2: Playback — HTTPS URLs + Sync + Keyboard Shortcuts + Responsive Timeline (Critical #2, #8, High #10, Medium #22)

**Files:**

- Modify: `ui/src/pages/Playback.tsx`

**Issues fixed:**

- #2 CRITICAL: Hardcoded `http://` breaks HTTPS deployments
- #8 CRITICAL: Multi-camera desync
- #10 HIGH: No keyboard shortcuts
- #22 MEDIUM: Timeline goes off-screen on small displays

- [ ] **Step 1: Fix playback URL to respect protocol**

Replace all instances of:

```typescript
`http://${window.location.hostname}:9996/get?...`;
```

With:

```typescript
`${window.location.protocol}//${window.location.hostname}:9996/get?...`;
```

Do the same in `ui/src/pages/ClipSearch.tsx` (lines 671, 689, 716, 734) — there are 4 instances there too.

- [ ] **Step 2: Fix multi-camera sync**

In the `CameraTile` component, add a `readyState` callback. In the parent, track which cameras are loaded:

```typescript
const [readyCameras, setReadyCameras] = useState<Set<string>>(new Set());

// When seeking, pause all videos until all are loaded at the new position
const handleSeek = (time: Date) => {
  setReadyCameras(new Set());
  setPlaybackTime(time);
};

// Each CameraTile reports when it's ready after a seek
// Only start playback when all tiles report ready
useEffect(() => {
  if (readyCameras.size === selectedCameras.length && playing) {
    selectedCameras.forEach((cam) => {
      const video = videoRefs.current.get(cam.id);
      if (video) video.play();
    });
  }
}, [readyCameras.size]);
```

In `CameraTile`, add `onCanPlay` handler to the video element that calls `onReady(camera.id)`.

- [ ] **Step 3: Add keyboard shortcuts**

```typescript
useKeyboardShortcuts([
  { key: " ", handler: () => setPlaying((p) => !p), description: "Play/Pause" },
  {
    key: "ArrowLeft",
    handler: () => seekRelative(-10),
    description: "Seek back 10s",
  },
  {
    key: "ArrowRight",
    handler: () => seekRelative(10),
    description: "Seek forward 10s",
  },
  {
    key: "ArrowUp",
    handler: () => setSpeed((s) => Math.min(s * 2, 16)),
    description: "Speed up",
  },
  {
    key: "ArrowDown",
    handler: () => setSpeed((s) => Math.max(s / 2, 0.25)),
    description: "Slow down",
  },
]);

const seekRelative = (seconds: number) => {
  if (!playbackTime) return;
  setPlaybackTime(new Date(playbackTime.getTime() + seconds * 1000));
};
```

- [ ] **Step 4: Make timeline responsive**

Change the hardcoded `TOTAL_HEIGHT = 960` to:

```typescript
const TOTAL_HEIGHT = Math.min(960, window.innerHeight * 0.6);
```

Or better, use a ref to measure the container:

```typescript
const [timelineHeight, setTimelineHeight] = useState(960);
useEffect(() => {
  const h = Math.min(960, window.innerHeight - 200);
  setTimelineHeight(h);
}, []);
```

- [ ] **Step 5: Commit**

```bash
git add ui/src/pages/Playback.tsx ui/src/pages/ClipSearch.tsx
git commit -m "fix(ui): HTTPS playback URLs, multi-camera sync, keyboard shortcuts, responsive timeline"
```

---

### Task 3: Notifications — WebSocket Resilience + History + Toast Timing (Critical #3, Medium #19, #25)

**Files:**

- Modify: `ui/src/hooks/useNotifications.ts`
- Modify: `ui/src/components/Toast.tsx`

**Issues fixed:**

- #3 CRITICAL: WebSocket port hardcoded, no fallback
- #19 MEDIUM: History truncates at 20
- #25 LOW: Toast auto-dismiss too fast for errors

- [ ] **Step 1: Add WebSocket connection status tracking**

Add state: `const [wsConnected, setWsConnected] = useState(false)`

In `ws.onopen`: `setWsConnected(true)`
In `ws.onclose`: `setWsConnected(false)`

Export `wsConnected` from the hook so the UI can show a "Notifications unavailable" indicator.

- [ ] **Step 2: Increase history and add connection validation**

Change `MAX_HISTORY` from `20` to `100`.

Add a connection test on first connect — if WebSocket fails 3 times in a row, show a persistent warning:

```typescript
const failCountRef = useRef(0);
// In ws.onerror: failCountRef.current++
// In ws.onopen: failCountRef.current = 0
```

Export `wsConnected` from the hook return.

- [ ] **Step 3: Fix toast auto-dismiss timing**

In `Toast.tsx`:

- Change `AUTO_DISMISS_MS` from `8000` to `12000`
- For error toasts, don't auto-dismiss — require manual close:

```typescript
useEffect(() => {
  if (toast.type === "error") return; // never auto-dismiss errors
  const timer = setTimeout(() => onDismiss(toast.id), AUTO_DISMISS_MS);
  return () => clearTimeout(timer);
}, [toast.id, toast.type, onDismiss]);
```

- [ ] **Step 4: Commit**

```bash
git add ui/src/hooks/useNotifications.ts ui/src/components/Toast.tsx
git commit -m "fix(ui): WebSocket connection status, notification history to 100, error toasts persist"
```

---

### Task 4: Auth Security — CSRF + Token Rotation (Critical #4, Medium #24)

**Files:**

- Modify: `internal/nvr/api/auth.go`
- Modify: `internal/nvr/api/router.go`

**Issues fixed:**

- #4 CRITICAL: No CSRF protection
- #24 MEDIUM: Refresh token never rotates

- [ ] **Step 1: Add Origin/Referer CSRF validation**

In `router.go`, add CSRF middleware for state-changing methods:

```go
func csrfMiddleware() gin.HandlerFunc {
    return func(c *gin.Context) {
        if c.Request.Method == "GET" || c.Request.Method == "HEAD" || c.Request.Method == "OPTIONS" {
            c.Next()
            return
        }
        origin := c.GetHeader("Origin")
        if origin == "" {
            origin = c.GetHeader("Referer")
        }
        if origin == "" {
            // Allow non-browser clients (curl, API integrations) — they don't send Origin
            c.Next()
            return
        }
        // Verify origin matches the host
        host := c.Request.Host
        if idx := strings.LastIndex(host, ":"); idx >= 0 {
            host = host[:idx]
        }
        if !strings.Contains(origin, host) {
            c.AbortWithStatusJSON(http.StatusForbidden, gin.H{"error": "CSRF validation failed"})
            return
        }
        c.Next()
    }
}
```

Register it on the protected route group: `protected.Use(csrfMiddleware())`

- [ ] **Step 2: Implement refresh token rotation**

In `auth.go`'s `Refresh` handler, after validating the old token:

1. Revoke the old refresh token: `h.DB.RevokeRefreshToken(tokenHash)`
2. Generate a new refresh token
3. Store the new token in DB
4. Set the new cookie

```go
// Revoke old token
_ = h.DB.RevokeRefreshToken(tokenHash)

// Issue new refresh token
newRawToken := generateSecureToken()
newTokenHash := sha256Hash(newRawToken)
newExpiry := time.Now().Add(7 * 24 * time.Hour)
_ = h.DB.InsertRefreshToken(newTokenHash, rt.UserID, newExpiry)

// Set new cookie
c.SetSameSite(http.SameSiteStrictMode)
c.SetCookie("refresh_token", newRawToken, 7*24*3600, "/", "", false, true)
```

- [ ] **Step 3: Verify and commit**

Run: `go build ./internal/nvr/...`

```bash
git add internal/nvr/api/auth.go internal/nvr/api/router.go
git commit -m "fix(security): add CSRF origin validation and refresh token rotation"
```

---

### Task 5: Database Backup/Restore (Critical #5, Medium #23)

**Files:**

- Modify: `internal/nvr/db/db.go`
- Create: `internal/nvr/api/backup.go`
- Modify: `internal/nvr/api/router.go`
- Modify: `ui/src/pages/Settings.tsx`

**Issues fixed:**

- #5 CRITICAL: No backup/restore functionality
- #23 MEDIUM: No migration rollback (mitigated by having backups)

- [ ] **Step 1: Add Backup method to DB**

In `db.go`:

```go
// Backup creates a copy of the database at the given path using SQLite's backup API.
func (d *DB) Backup(destPath string) error {
    _, err := d.Exec(fmt.Sprintf("VACUUM INTO '%s'", destPath))
    return err
}
```

- [ ] **Step 2: Add backup/restore API endpoints**

Create `internal/nvr/api/backup.go`:

```go
// BackupHandler handles POST /api/nvr/system/backup
// Creates a timestamped backup in ~/.mediamtx/backups/
func (h *BackupHandler) CreateBackup(c *gin.Context) {
    backupDir := filepath.Join(filepath.Dir(h.DBPath), "backups")
    os.MkdirAll(backupDir, 0755)
    filename := fmt.Sprintf("nvr-backup-%s.db", time.Now().Format("2006-01-02-150405"))
    destPath := filepath.Join(backupDir, filename)
    if err := h.DB.Backup(destPath); err != nil {
        c.JSON(500, gin.H{"error": err.Error()})
        return
    }
    c.JSON(200, gin.H{"path": destPath, "filename": filename})
}

// ListBackups handles GET /api/nvr/system/backups
func (h *BackupHandler) ListBackups(c *gin.Context) {
    // List files in backups dir, return [{filename, size, created_at}]
}

// DownloadBackup handles GET /api/nvr/system/backups/:filename
func (h *BackupHandler) DownloadBackup(c *gin.Context) {
    // Serve file for download
}
```

Register routes in `router.go`:

```go
protected.POST("/system/backup", backupHandler.CreateBackup)
protected.GET("/system/backups", backupHandler.ListBackups)
protected.GET("/system/backups/:filename", backupHandler.DownloadBackup)
```

- [ ] **Step 3: Add backup UI in Settings page**

In `Settings.tsx`, add a "Backups" section:

- "Create Backup Now" button
- List of existing backups with download links
- Show last backup date

- [ ] **Step 4: Add automatic daily backup**

In the scheduler (or NVR initialization), add a daily timer:

```go
go func() {
    ticker := time.NewTicker(24 * time.Hour)
    defer ticker.Stop()
    for range ticker.C {
        n.database.Backup(autoBackupPath)
        // Keep only last 7 backups
    }
}()
```

- [ ] **Step 5: Commit**

```bash
git add internal/nvr/db/db.go internal/nvr/api/backup.go internal/nvr/api/router.go ui/src/pages/Settings.tsx
git commit -m "feat(db): add database backup/restore with automatic daily backups"
```

---

### Task 6: Clip Search — Error Handling + Semantic Search Status (Critical #6, High #14)

**Files:**

- Modify: `ui/src/pages/ClipSearch.tsx`

**Issues fixed:**

- #6 CRITICAL: Save clip silently fails on error
- #14 HIGH: No indication when semantic search is unavailable

- [ ] **Step 1: Fix silent error on clip save**

Find the save clip catch block (currently `catch { // silently fail }`). Replace with:

```typescript
} catch (err) {
  pushToast({
    id: crypto.randomUUID(),
    type: 'error',
    title: 'Save Failed',
    message: 'Failed to save clip. Please try again.',
    timestamp: new Date(),
  })
} finally {
```

Also handle the non-ok response:

```typescript
if (res.ok) {
  onSaved();
  onClose();
} else {
  const data = await res.json().catch(() => ({}));
  pushToast({
    id: crypto.randomUUID(),
    type: "error",
    title: "Save Failed",
    message: data.error || "Server error saving clip",
    timestamp: new Date(),
  });
}
```

Import `pushToast` from `'../components/Toast'`.

- [ ] **Step 2: Show semantic search availability status**

Add a check on mount for whether the embedder is loaded:

```typescript
const [searchCapability, setSearchCapability] = useState<
  "full" | "basic" | "loading"
>("loading");

useEffect(() => {
  apiFetch("/system/info")
    .then((res) => res.json())
    .then((data) => {
      setSearchCapability(data.clip_search_available ? "full" : "basic");
    })
    .catch(() => setSearchCapability("basic"));
}, []);
```

Show status below the search input:

```tsx
{
  searchCapability === "basic" && (
    <p className="text-xs text-amber-400 mt-1">
      Basic search only (class name matching). Install CLIP models for natural
      language search.
    </p>
  );
}
```

This requires adding `clip_search_available: embedder != nil` to the `/system/info` API response (backend change in `internal/nvr/api/system.go` or equivalent).

- [ ] **Step 3: Commit**

```bash
git add ui/src/pages/ClipSearch.tsx
git commit -m "fix(ui): show errors on clip save failure, indicate semantic search availability"
```

---

### Task 7: Camera Management — Discovery Timeout + RTSP Validation + AI Confidence (Critical #7, Medium #20, Low #30)

**Files:**

- Modify: `ui/src/pages/CameraManagement.tsx`

**Issues fixed:**

- #7 CRITICAL: Discovery hangs forever
- #20 MEDIUM: No RTSP URL validation
- #30 LOW: No confidence threshold slider

- [ ] **Step 1: Add discovery timeout and cancel**

Wrap the discovery API call with an AbortController:

```typescript
const [discoverController, setDiscoverController] = useState<AbortController | null>(null)

const handleDiscover = async () => {
  const controller = new AbortController()
  setDiscoverController(controller)
  setDiscovering(true)

  const timeout = setTimeout(() => controller.abort(), 30000)

  try {
    const res = await apiFetch('/onvif/discover', { signal: controller.signal })
    // ... handle response
  } catch (err) {
    if ((err as Error).name === 'AbortError') {
      pushToast({ type: 'warning', title: 'Discovery Timeout', message: 'No cameras found in 30 seconds', ... })
    }
  } finally {
    clearTimeout(timeout)
    setDiscovering(false)
    setDiscoverController(null)
  }
}
```

Add cancel button:

```tsx
{
  discovering && (
    <button onClick={() => discoverController?.abort()} className="...">
      Cancel
    </button>
  );
}
```

- [ ] **Step 2: Add RTSP URL validation**

When the user enters an RTSP URL in the manual add form, validate format:

```typescript
const isValidRtspUrl = (url: string) => /^rtsp:\/\/.+/.test(url)

// Show warning below input:
{addRtspUrl && !isValidRtspUrl(addRtspUrl) && (
  <p className="text-xs text-nvr-danger mt-1">URL must start with rtsp://</p>
)}
```

Disable the submit button if URL is invalid.

- [ ] **Step 3: Add confidence threshold slider to AI panel**

In the `AIDetectionPanel` component, add a slider:

```tsx
{
  aiEnabled && (
    <div className="mb-3">
      <label className="text-xs text-nvr-text-secondary mb-1 block">
        Minimum Confidence: {Math.round(confThreshold * 100)}%
      </label>
      <input
        type="range"
        min="20"
        max="90"
        step="5"
        value={confThreshold * 100}
        onChange={(e) => setConfThreshold(parseInt(e.target.value) / 100)}
        className="w-full"
      />
      <p className="text-xs text-nvr-text-muted mt-1">
        Only report detections above this confidence level
      </p>
    </div>
  );
}
```

This needs a backend change: add `confidence_threshold` field to the `PUT /cameras/:id/ai` endpoint and store it in the cameras table.

- [ ] **Step 4: Commit**

```bash
git add ui/src/pages/CameraManagement.tsx
git commit -m "fix(ui): discovery timeout/cancel, RTSP validation, AI confidence slider"
```

---

### Task 8: Settings — Help Text + Threshold Config + Audit Export (High #11, Medium #21, Low #29)

**Files:**

- Modify: `ui/src/pages/Settings.tsx`

**Issues fixed:**

- #11 HIGH: No help text explaining settings
- #21 MEDIUM: Disk warning thresholds not configurable
- #29 LOW: No audit log CSV export

- [ ] **Step 1: Add tooltips/help text to all settings**

For each setting, add a help text paragraph below it:

```tsx
<p className="text-xs text-nvr-text-muted mt-1">
  How many days to keep recordings before automatic deletion. Higher values use
  more storage.
</p>
```

Key settings needing help text:

- Retention days: "How many days to keep recordings before automatic deletion"
- Motion timeout: "Seconds of inactivity before a motion event is considered ended"
- Recording mode: "Continuous records 24/7. Motion-only records when movement is detected. Scheduled follows your recording rules."
- Storage warning: "Percentage of disk usage that triggers a warning banner"

- [ ] **Step 2: Make disk warning thresholds configurable**

Add two number inputs for warning (default 85) and critical (default 95) thresholds. Save via `PUT /system/settings` API (may need to create this endpoint, or store in a `settings` table).

- [ ] **Step 3: Add audit log CSV export**

Add an "Export CSV" button in the audit log tab:

```typescript
const handleExportAudit = () => {
  const headers = ["Timestamp", "User", "Action", "Resource", "Details", "IP"];
  const rows = auditEntries.map((e) => [
    e.created_at,
    e.username,
    e.action,
    `${e.resource_type}/${e.resource_id}`,
    e.details,
    e.ip_address,
  ]);
  const csv = [headers, ...rows]
    .map((r) => r.map((c) => `"${String(c).replace(/"/g, '""')}"`).join(","))
    .join("\n");
  const blob = new Blob([csv], { type: "text/csv" });
  const url = URL.createObjectURL(blob);
  const a = document.createElement("a");
  a.href = url;
  a.download = `audit-log-${new Date().toISOString().split("T")[0]}.csv`;
  a.click();
  URL.revokeObjectURL(url);
};
```

Button:

```tsx
<button
  onClick={handleExportAudit}
  className="text-xs text-nvr-accent hover:text-nvr-accent-hover"
>
  Export CSV
</button>
```

- [ ] **Step 4: Commit**

```bash
git add ui/src/pages/Settings.tsx
git commit -m "feat(ui): settings help text, configurable disk thresholds, audit CSV export"
```

---

### Task 9: User Management — Password Change (High #12)

**Files:**

- Modify: `ui/src/pages/UserManagement.tsx`
- Modify: `internal/nvr/api/auth.go`
- Modify: `internal/nvr/api/router.go`

**Issues fixed:**

- #12 HIGH: Users can't change their own password

- [ ] **Step 1: Add password change API endpoint**

In `auth.go`, add:

```go
// ChangePassword handles PUT /api/nvr/auth/password
func (h *AuthHandler) ChangePassword(c *gin.Context) {
    var req struct {
        CurrentPassword string `json:"current_password" binding:"required"`
        NewPassword     string `json:"new_password" binding:"required"`
    }
    if err := c.ShouldBindJSON(&req); err != nil {
        c.JSON(400, gin.H{"error": "current_password and new_password required"})
        return
    }
    // Get user from JWT claims
    userID := c.GetString("user_id")
    user, err := h.DB.GetUser(userID)
    // Verify current password with argon2
    // Hash new password
    // Update in DB
    c.JSON(200, gin.H{"status": "password changed"})
}
```

Register: `protected.PUT("/auth/password", authHandler.ChangePassword)`

- [ ] **Step 2: Add password change form in UserManagement or a dedicated section**

Since the Settings page already has the user menu pointing to "Change Password", add a section in `Settings.tsx` under a "Security" tab, or create a modal in `UserManagement.tsx`:

```tsx
function ChangePasswordForm() {
  const [currentPw, setCurrentPw] = useState("");
  const [newPw, setNewPw] = useState("");
  const [confirmPw, setConfirmPw] = useState("");
  const [error, setError] = useState("");
  const [success, setSuccess] = useState(false);

  const handleSubmit = async (e: FormEvent) => {
    e.preventDefault();
    if (newPw !== confirmPw) {
      setError("Passwords do not match");
      return;
    }
    if (newPw.length < 8) {
      setError("Password must be at least 8 characters");
      return;
    }
    const res = await apiFetch("/auth/password", {
      method: "PUT",
      body: JSON.stringify({
        current_password: currentPw,
        new_password: newPw,
      }),
    });
    if (res.ok) {
      setSuccess(true);
      setCurrentPw("");
      setNewPw("");
      setConfirmPw("");
    } else {
      const d = await res.json();
      setError(d.error || "Failed");
    }
  };
  // ... render form
}
```

- [ ] **Step 3: Commit**

```bash
git add internal/nvr/api/auth.go internal/nvr/api/router.go ui/src/pages/Settings.tsx
git commit -m "feat(auth): add self-service password change"
```

---

### Task 10: Recordings — Calendar Date Picker (High #13)

**Files:**

- Modify: `ui/src/pages/Recordings.tsx`

**Issues fixed:**

- #13 HIGH: Calendar view imported but not functional

- [ ] **Step 1: Verify RecordingCalendar component exists**

Check if `ui/src/components/RecordingCalendar.tsx` exists and is functional. If it's a stub, implement a simple month calendar:

- Grid of days showing the current month
- Days with recordings get a colored dot indicator
- Clicking a day sets the selected date
- Previous/next month navigation

- [ ] **Step 2: Wire calendar into Recordings page**

The import `RecordingCalendar` already exists. Add it to the page layout — either as a sidebar calendar or a popover from a "Jump to Date" button:

```tsx
<RecordingCalendar
  selectedDate={selectedDate}
  onSelectDate={setSelectedDate}
  recordingDates={datesWithRecordings}
/>
```

Fetch recording dates by querying the API or scanning the recordings directory listing for dates that have segments.

- [ ] **Step 3: Commit**

```bash
git add ui/src/pages/Recordings.tsx ui/src/components/RecordingCalendar.tsx
git commit -m "feat(ui): functional calendar date picker for recordings navigation"
```

---

### Task 11: Analytics Overlay — Adaptive Polling (High #15)

**Files:**

- Modify: `ui/src/components/AnalyticsOverlay.tsx`

**Issues fixed:**

- #15 HIGH: No FPS monitoring or adaptive throttling

- [ ] **Step 1: Add performance monitoring and adaptive polling**

Track draw times and adjust polling interval:

```typescript
const drawTimeRef = useRef(0);
const pollIntervalRef = useRef(500);

// In the draw callback:
const drawStart = performance.now();
// ... existing draw code ...
drawTimeRef.current = performance.now() - drawStart;

// Adapt polling interval based on draw time
if (drawTimeRef.current > 16) {
  pollIntervalRef.current = Math.min(pollIntervalRef.current + 100, 1000);
} else if (drawTimeRef.current < 8 && pollIntervalRef.current > 500) {
  pollIntervalRef.current = Math.max(pollIntervalRef.current - 50, 500);
}
```

Update the polling `setInterval` to use the dynamic interval (use a self-scheduling `setTimeout` instead of `setInterval`):

```typescript
const scheduleNextPoll = () => {
  setTimeout(async () => {
    await poll();
    if (!cancelled) scheduleNextPoll();
  }, pollIntervalRef.current);
};
```

- [ ] **Step 2: Commit**

```bash
git add ui/src/components/AnalyticsOverlay.tsx
git commit -m "fix(ui): adaptive polling for analytics overlay based on draw performance"
```

---

### Task 12: Error Feedback — PTZ + Video Player (Medium #17, #18)

**Files:**

- Modify: `ui/src/components/PTZControls.tsx`
- Modify: `ui/src/components/VideoPlayer.tsx` (if exists)

**Issues fixed:**

- #17 MEDIUM: Generic error messages
- #18 MEDIUM: PTZ controls give no feedback

- [ ] **Step 1: Add PTZ click feedback and error handling**

In `PTZControls.tsx`, add visual feedback on button press:

```typescript
const [activeButton, setActiveButton] = useState<string | null>(null)

const handlePTZ = async (direction: string) => {
  setActiveButton(direction)
  try {
    const res = await apiFetch(`/cameras/${cameraId}/ptz/${direction}`, { method: 'POST' })
    if (!res.ok) {
      pushToast({ type: 'error', title: 'PTZ Failed', message: `Camera did not respond to ${direction} command`, ... })
    }
  } catch {
    pushToast({ type: 'error', title: 'PTZ Error', message: 'Could not reach camera', ... })
  } finally {
    setTimeout(() => setActiveButton(null), 200)
  }
}
```

Add active state styling: `className={`... ${activeButton === dir ? 'bg-nvr-accent scale-95' : ''}`}`

- [ ] **Step 2: Improve video player error messages**

If a `VideoPlayer` component exists, update error messages to include context:

```typescript
const errorMessage =
  error.includes("timeout") || error.includes("network")
    ? "Connection timed out. The server may be restarting — try again in a few seconds."
    : error.includes("404")
      ? "No recording found for this time range."
      : `Playback error: ${error}`;
```

- [ ] **Step 3: Commit**

```bash
git add ui/src/components/PTZControls.tsx ui/src/components/VideoPlayer.tsx
git commit -m "fix(ui): PTZ click feedback, contextual video player error messages"
```

---

### Task 13: Mobile Responsiveness (High #16)

**Files:**

- Modify: `ui/src/pages/LiveView.tsx`
- Modify: `ui/src/pages/Playback.tsx`
- Modify: `ui/src/pages/Settings.tsx`
- Modify: `ui/src/pages/CameraManagement.tsx`

**Issues fixed:**

- #16 HIGH: Mobile responsiveness not tested

- [ ] **Step 1: Fix LiveView grid for small screens**

Ensure the grid switches to single column on mobile:

```tsx
<div className="grid grid-cols-1 sm:grid-cols-2 lg:grid-cols-3 xl:grid-cols-4 gap-3">
```

Make camera modals full-screen on mobile:

```tsx
className = "fixed inset-0 sm:inset-auto sm:relative ...";
```

- [ ] **Step 2: Fix Playback layout for mobile**

Stack the timeline below the video instead of beside it on mobile:

```tsx
<div className="flex flex-col lg:flex-row gap-4">
  <div className="flex-1">{/* video area */}</div>
  <div className="w-full lg:w-64">{/* timeline */}</div>
</div>
```

- [ ] **Step 3: Fix Settings tabs for mobile**

Use horizontal scrolling tabs on mobile instead of wrapping:

```tsx
<div className="flex overflow-x-auto gap-1 pb-2 -mx-4 px-4 sm:mx-0 sm:px-0 sm:flex-wrap">
```

- [ ] **Step 4: Fix CameraManagement table for mobile**

Replace the table with card layout on mobile:

```tsx
<div className="hidden sm:block">{/* Table view */}</div>
<div className="sm:hidden">{/* Card view */}</div>
```

- [ ] **Step 5: Commit**

```bash
git add ui/src/pages/LiveView.tsx ui/src/pages/Playback.tsx ui/src/pages/Settings.tsx ui/src/pages/CameraManagement.tsx
git commit -m "fix(ui): mobile-responsive layouts for all pages"
```
