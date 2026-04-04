# Flutter Client vs React Web UI -- Feature Parity Audit

**Ticket:** KAI-59
**Date:** 2026-04-03
**Scope:** The React web UI is being scoped down to admin-only (setup/config). This audit focuses on features that should exist in the Flutter client as the PRIMARY user-facing app.

---

## 1. Architecture Context

| Concern | React Web UI (`ui/src/`) | Flutter Client (`clients/flutter/lib/`) |
|---|---|---|
| Role | Admin console (setup/config) | Primary user app (live view, playback, monitoring) |
| Router | React Router (BrowserRouter) | GoRouter with ShellRoute |
| State | Hooks + Context | Riverpod providers |
| Video | WebRTC WHEP (browser) | WebRTC WHEP + RTSP via media_kit (native) |

---

## 2. Feature Comparison Matrix

### 2.1 Authentication & Onboarding

| Feature | React | Flutter | Status | Priority | Effort |
|---|---|---|---|---|---|
| Login screen | Yes | Yes | **Parity** | -- | -- |
| Initial setup wizard | Yes (`Setup.tsx`) | Yes (`setup_screen.dart`) | **Parity** | -- | -- |
| Server URL configuration | N/A (same-origin) | Yes (`server_setup_screen.dart`) | **Flutter-only** | -- | -- |
| Protected route guard | Yes | Yes (GoRouter redirect) | **Parity** | -- | -- |
| User menu / avatar dropdown | Yes | No (logout only in settings) | **Partial** | Low | S |

### 2.2 Live View

| Feature | React | Flutter | Status | Priority | Effort |
|---|---|---|---|---|---|
| Camera grid with layout selector (1x1 to 4x4) | Yes | Yes | **Parity** | -- | -- |
| Camera status indicators (online/offline) | Yes | Yes | **Parity** | -- | -- |
| Online/offline count badges | Yes | No | **Missing** | Low | S |
| Fullscreen single-camera modal | Yes (modal overlay) | Yes (dedicated screen) | **Parity** | -- | -- |
| PTZ controls overlay | Yes | Yes | **Parity** | -- | -- |
| Analytics/detection overlay | Yes | Yes | **Parity** | -- | -- |
| Screenshot capture (client-side) | Yes (canvas snapshot) | Yes (in fullscreen view) | **Parity** | -- | -- |
| Two-way audio intercom | Yes (`AudioIntercom`) | No | **Missing** | High | L |
| View recordings quick-link from live | Yes | No | **Missing** | Medium | S |
| Keyboard shortcuts for layout (1-4, F) | Yes | N/A (mobile-first) | N/A | -- | -- |
| Camera groups panel | No | Yes (side panel) | **Flutter-only** | -- | -- |
| Camera tours (auto-cycle cameras) | No | Yes (tours panel + active pill) | **Flutter-only** | -- | -- |
| H.265/HEVC fallback to RTSP | No (browser limitation) | Yes (media_kit) | **Flutter-only** | -- | -- |
| Mute/unmute audio toggle | No | Yes (fullscreen) | **Flutter-only** | -- | -- |

### 2.3 Recordings Browser

| Feature | React | Flutter | Status | Priority | Effort |
|---|---|---|---|---|---|
| Dedicated recordings list page | Yes (`Recordings.tsx`) | No (no equivalent page) | **Missing** | High | L |
| Recording calendar with date picker | Yes (`RecordingCalendar`) | No | **Missing** | High | M |
| Recording minimap (today's coverage per camera) | Yes (in camera management) | No | **Missing** | Medium | M |
| Per-camera recording timeline | Yes (Timeline component) | Partially (in playback only) | **Partial** | Medium | M |
| Motion event markers on timeline | Yes | Yes (in playback) | **Parity** | -- | -- |
| Saved clips (create/name/tag/notes) | Yes (in Recordings page) | Yes (`saved_clip.dart` model) | **Partial** | Medium | M |
| Multi-camera synchronized playback | Yes (`MultiCameraPlayer`) | Yes (playback grid mode) | **Parity** | -- | -- |
| Camera storage browser (ONVIF edge storage) | Yes (`CameraStorageBrowser`) | Yes (`storage_tab.dart`) | **Parity** | -- | -- |

### 2.4 Playback

| Feature | React | Flutter | Status | Priority | Effort |
|---|---|---|---|---|---|
| Single/multi-camera playback | Yes | Yes | **Parity** | -- | -- |
| Playback speed control | Yes | Yes (transport bar) | **Parity** | -- | -- |
| Timeline with recording ranges | Yes | Yes (fixed-playhead timeline) | **Parity** | -- | -- |
| Zoom presets on timeline | Yes | Yes | **Parity** | -- | -- |
| Mini overview bar | No | Yes | **Flutter-only** | -- | -- |
| Playback detection overlay (bounding boxes) | No | Yes (`playback_detection_overlay.dart`) | **Flutter-only** | -- | -- |
| Bookmarks (create/jump-to) | No | Yes (`bookmarks_provider.dart`) | **Flutter-only** | -- | -- |
| Timeline intensity heatmap | No | Yes (`timeline_intensity_provider.dart`) | **Flutter-only** | -- | -- |

### 2.5 Clip/Event Search

| Feature | React | Flutter | Status | Priority | Effort |
|---|---|---|---|---|---|
| Search by event type (motion/person/vehicle/animal/tampering) | Yes | Yes | **Parity** | -- | -- |
| Camera filter | Yes | Yes | **Parity** | -- | -- |
| Time range presets | Yes | Yes | **Parity** | -- | -- |
| Sort modes (newest/oldest/longest) | Yes | No | **Missing** | Low | S |
| Paginated results | Yes | Yes | **Parity** | -- | -- |
| Clip thumbnail preview | Yes | Yes | **Parity** | -- | -- |
| Clip player (inline playback) | Yes | Yes (bottom sheet) | **Parity** | -- | -- |
| Confidence threshold filter | No | Yes | **Flutter-only** | -- | -- |
| Navigate to playback from search result | Yes | Yes | **Parity** | -- | -- |

### 2.6 Camera Management

| Feature | React | Flutter | Status | Priority | Effort |
|---|---|---|---|---|---|
| Camera list with status | Yes | Yes | **Parity** | -- | -- |
| Add camera (ONVIF discovery + manual) | Yes | Yes | **Parity** | -- | -- |
| Edit camera settings (name, URLs, credentials) | Yes | Yes (detail screen) | **Parity** | -- | -- |
| Delete camera with confirmation | Yes | Yes | **Parity** | -- | -- |
| Recording rules (continuous/motion/schedule) | Yes (`RecordingRules`) | Yes (`recording_rules_screen.dart`) | **Parity** | -- | -- |
| Schedule preview (week grid visualization) | Yes (`SchedulePreview`) | Partial (day names shown, no visual grid) | **Partial** | Low | M |
| Detection zone editor | Yes (`DetectionZoneEditor`) | Yes (`zone_editor_screen.dart`) | **Parity** | -- | -- |
| Imaging settings (brightness/contrast/saturation/sharpness) | Yes (`CameraSettings`) | Yes (`imaging_section.dart`) | **Parity** | -- | -- |
| Analytics config (modules + rules) | Yes (`AnalyticsConfig`) | Yes (AI section in detail) | **Parity** | -- | -- |
| Relay output controls | Yes (`RelayControls`) | Yes (`relay_section.dart`) | **Parity** | -- | -- |
| ONVIF device info | No (inline in camera card) | Yes (`device_info_section.dart`) | **Flutter-only** | -- | -- |
| ONVIF device management (reboot/factory reset) | No | Yes (`device_mgmt_section.dart`) | **Flutter-only** | -- | -- |
| ONVIF media config (profile details) | No | Yes (`media_config_section.dart`) | **Flutter-only** | -- | -- |
| ONVIF audio capabilities section | No | Yes (`audio_section.dart`) | **Flutter-only** | -- | -- |
| PTZ presets management | No | Yes (`ptz_presets_section.dart`, `ptz_enhanced_section.dart`) | **Flutter-only** | -- | -- |
| Per-stream recording/quality settings | No | Yes (expandable per-stream cards) | **Flutter-only** | -- | -- |
| Storage estimates per stream | No | Yes (`StreamStorageEstimate`) | **Flutter-only** | -- | -- |
| Recording mode badge per camera | Yes (`RecordingModeBadge`) | No | **Missing** | Low | S |

### 2.7 Schedules

| Feature | React | Flutter | Status | Priority | Effort |
|---|---|---|---|---|---|
| Dedicated schedule templates page | No | Yes (`schedules_screen.dart`) | **Flutter-only** | -- | -- |

### 2.8 Screenshots Gallery

| Feature | React | Flutter | Status | Priority | Effort |
|---|---|---|---|---|---|
| Screenshots gallery with filters/pagination | No | Yes (`screenshots_screen.dart`) | **Flutter-only** | -- | -- |

### 2.9 Notifications & Alerts

| Feature | React | Flutter | Status | Priority | Effort |
|---|---|---|---|---|---|
| Real-time notification bell (WebSocket) | Yes (`NotificationBell`) | Yes (`alerts_panel.dart`) | **Parity** | -- | -- |
| Notification types (motion/camera offline/online/recording) | Yes | Yes | **Parity** | -- | -- |
| Mark all read | Yes | Yes | **Parity** | -- | -- |
| Notification settings (configure alert preferences) | Yes (Settings > Notifications tab) | No | **Missing** | Medium | M |
| Storage warning banner | Yes (`StorageBanner`) | No | **Missing** | Medium | S |

### 2.10 Settings

| Feature | React | Flutter | Status | Priority | Effort |
|---|---|---|---|---|---|
| System info (version, platform, uptime) | Yes | Yes (`_SystemPanel`) | **Parity** | -- | -- |
| Storage dashboard (disk usage, per-camera breakdown) | Yes | Yes (`storage_panel.dart`) | **Parity** | -- | -- |
| AI Analytics settings tab | Yes | No | **Missing** | Medium | M |
| Notification preferences tab | Yes | No | **Missing** | Medium | M |
| Appearance/theme settings | Yes | No | **Missing** | Low | S |
| Config export/import | Yes | Yes (`backup_panel.dart`) | **Parity** | -- | -- |
| Audit log viewer (with filters) | Yes | Yes (`audit_panel.dart`) | **Parity** | -- | -- |
| Performance metrics (CPU, memory, goroutines) | Yes | Yes (`performance_panel.dart`) | **Parity** | -- | -- |
| User management (CRUD users, roles, camera permissions) | Yes (`UserManagement.tsx`) | Partial (change password only) | **Partial** | High | L |
| Change password | Yes | Yes | **Parity** | -- | -- |

### 2.11 Navigation & UX

| Feature | React | Flutter | Status | Priority | Effort |
|---|---|---|---|---|---|
| Desktop top nav bar | Yes | Icon rail (left side) | **Parity** (different layout) | -- | -- |
| Mobile sidebar menu | Yes (hamburger + slide-in) | Bottom nav bar | **Parity** (different layout) | -- | -- |
| Keyboard shortcuts help overlay | Yes (`KeyboardShortcutsHelp`) | N/A (mobile-first) | N/A | -- | -- |
| Error boundary (crash recovery) | Yes (`ErrorBoundary`) | No | **Missing** | Medium | S |
| Toast notifications (transient messages) | Yes (`Toast`) | Yes (SnackBar helper) | **Parity** | -- | -- |
| Camera side panel (quick access tree) | No | Yes (`camera_panel.dart`) | **Flutter-only** | -- | -- |
| Tour active indicator pill | No | Yes (`tour_active_pill.dart`) | **Flutter-only** | -- | -- |

---

## 3. Gap Summary -- Items Missing from Flutter

Sorted by priority (High first), then effort (smallest first).

### High Priority

| # | Feature | Status | Effort | Notes |
|---|---|---|---|---|
| 1 | Two-way audio intercom | Missing | L | React has `AudioIntercom` component using ONVIF audio backchannel API. Flutter needs WebRTC audio send or native audio implementation. |
| 2 | Recordings browser page | Missing | L | React has a dedicated page with calendar, timeline, per-camera recording ranges, and saved clips management. Flutter users must use the playback screen directly. |
| 3 | User management (admin CRUD) | Partial | L | Flutter only has change-password. Missing: create/edit/delete users, role assignment, camera permission scoping. |

### Medium Priority

| # | Feature | Status | Effort | Notes |
|---|---|---|---|---|
| 4 | Notification preferences | Missing | M | React Settings has a Notifications tab for configuring alert types and delivery. |
| 5 | AI Analytics settings | Missing | M | React Settings has an AI Analytics tab for global detection/classification config. |
| 6 | Storage warning banner | Missing | S | Persistent banner when disk usage exceeds warning/critical thresholds. Simple poll of `/system/storage`. |
| 7 | Saved clips management | Partial | M | Model exists but no UI for creating/naming/tagging clips from playback timeline. |
| 8 | View recordings quick-link from live view | Missing | S | In React, fullscreen modal has a button linking directly to that camera's recordings. |
| 9 | Error boundary (crash recovery) | Missing | S | Global error handler to prevent white-screen crashes. |

### Low Priority

| # | Feature | Status | Effort | Notes |
|---|---|---|---|---|
| 10 | Online/offline camera count badges | Missing | S | Small stat bar on live view showing N online, M offline. |
| 11 | Sort modes in clip search | Missing | S | React supports newest/oldest/longest sorting. |
| 12 | Schedule visual preview grid | Partial | M | React renders a 7-day x 48-slot heatmap grid. Flutter shows day names but no visual calendar grid. |
| 13 | Appearance/theme settings | Missing | S | Theme toggle (dark mode is default, but no user preference persistence). |
| 14 | Recording mode badge per camera | Missing | S | Shows effective mode (Always/Events/Off) inline in camera list. |

---

## 4. Features Exclusive to Flutter (Not in React)

These features exist only in the Flutter client and represent areas where Flutter is ahead.

| Feature | Location | Notes |
|---|---|---|
| Camera groups (create/manage/filter) | `camera_panel_groups.dart`, `groups_provider.dart` | Organize cameras into logical groups |
| Camera tours (auto-cycling) | `camera_panel_tours.dart`, `tours_provider.dart` | Automated camera rotation with configurable dwell time |
| Tour active pill indicator | `tour_active_pill.dart` | Shows which tour is running |
| Screenshots gallery | `screenshots_screen.dart` | Browse/filter captured screenshots |
| Schedule templates (standalone page) | `schedules_screen.dart` | CRUD schedule templates separate from camera config |
| Playback detection overlay (bounding boxes) | `playback_detection_overlay.dart` | AI detections rendered on playback video |
| Playback bookmarks | `bookmarks_provider.dart`, `bookmark.dart` | Save/jump-to timestamps during playback |
| Timeline intensity heatmap | `timeline_intensity_provider.dart` | Visual density indicator on timeline |
| Confidence threshold filter (clip search) | `clip_search_screen.dart` | Filter search results by AI confidence score |
| ONVIF device management (reboot/reset) | `device_mgmt_section.dart` | Remote device operations |
| ONVIF media profile details | `media_config_section.dart` | View/manage ONVIF media profiles |
| ONVIF audio capabilities section | `audio_section.dart` | Audio source/output inspection |
| PTZ presets management | `ptz_presets_section.dart`, `ptz_enhanced_section.dart` | Save/recall/delete PTZ positions |
| Per-stream recording settings | Camera detail screen | Independent quality/retention per stream |
| Storage estimates per stream | Camera detail screen | Projected storage usage |
| H.265/HEVC native playback | `fullscreen_view.dart` | RTSP fallback for codecs browsers cannot decode |
| Camera side panel (desktop) | `camera_panel.dart` | Quick-access camera tree with thumbnails |
| Server URL configuration | `server_setup_screen.dart` | Connect to remote NVR instances |

---

## 5. Recommendations

1. **Two-way audio intercom (High/L):** This is the most impactful missing feature for security use cases. Consider implementing via WebRTC audio send channel or a native audio bridge.

2. **Recordings browser (High/L):** Flutter users currently lack a way to browse recorded footage by date/camera without already knowing what to play. A dedicated recordings screen with calendar + timeline would close the biggest workflow gap.

3. **User management (High/L):** Admin users on the Flutter client cannot manage other users. This is essential if the React UI is scoped down to setup-only and the Flutter app becomes the primary interface.

4. **Storage warning banner (Medium/S):** Quick win. Poll `/system/storage` and show a banner when thresholds are exceeded.

5. **Notification & AI settings (Medium/M):** These settings panels should be ported so that mobile users can configure their alert and detection preferences without needing the web UI.
