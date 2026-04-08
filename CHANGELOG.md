# Changelog

All notable changes to MediaMTX NVR are documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.1.0/).

## [Unreleased]

### Added

- Runtime mode selection via the top-level `mode:` field in `mediamtx.yml`
  (KAI-237). Supported values: `directory`, `recorder`, `all-in-one`.
  Omitting the field preserves the pre-KAI-237 single-NVR behavior.
  Non-legacy modes currently log stub messages; real subsystem wiring
  lands in KAI-226, KAI-246, KAI-243, KAI-244, and KAI-250.
- `internal/shared/runtime` package exposing `ModeDirectory`,
  `ModeRecorder`, `ModeAllInOne` constants and a `Dispatch` shim used
  by `internal/core` at boot.

## [1.0.0] - 2026-04-03

### Highlights

MediaMTX NVR v1.0 transforms the MediaMTX streaming server into a full-featured Network Video Recorder with ONVIF camera management, AI-powered detection, a Flutter client, a React admin console, and comprehensive security and storage controls.

### Added

#### Core NVR

- ONVIF camera auto-discovery and manual addition with Profile S/T/G support
- Multi-channel camera support with per-device grouping (KAI-26)
- Camera connection resilience with exponential backoff reconnection (KAI-24)
- Camera capabilities refresh endpoint (KAI-111)
- OSD (On-Screen Display) management via ONVIF (KAI-25)
- Backchannel (two-way) audio via ONVIF (KAI-27)
- Multicast streaming support (KAI-21)
- Camera credential rotation API
- Camera groups and tours
- RTSP URL format validation for stream creation

#### Recording and Playback

- Segment-based recording with fMP4 fragment indexing
- Per-stream retention policies with camera-level fallback
- Event-aware retention with detection consolidation
- Storage quota management with configurable thresholds (KAI-8)
- Recording health monitoring endpoint (KAI-9)
- Frame-accurate seek with keyframe snapping (KAI-31)
- ONVIF Profile G replay control with Range/Scale/Speed support (KAI-29)
- Profile G reverse playback via negative Scale (KAI-30)
- Profile G event search and time-range filtered recording search (KAI-28)
- Multi-camera synchronized playback (KAI-32)
- Playback speed control with 16x preset and auto-mute outside audible range (KAI-36)
- Cross-day continuous playback with date navigation
- RTSP playback for H.265 cameras (WebRTC fallback)
- Thumbnail timeline generator and API (KAI-37)
- LRU playlist cache with TTL and invalidation

#### Export and Evidence

- Export jobs queue with cross-segment concatenation (KAI-33)
- Bulk export endpoint for multi-camera ZIP archives (KAI-34)
- Evidence export with chain-of-custody metadata (KAI-38)
- Screenshot capture, list, download, and delete endpoints

#### Bookmarks

- Bookmarks CRUD API with notes field, search, and per-user listing (KAI-35)

#### AI Detection Pipeline

- Modular detection pipeline: FrameSrc (FFmpeg), ONVIFSrc, IoU tracker, ByteTrack multi-object tracker, Kalman filter, Publisher (KAI-39)
- Per-track per-zone state machine with enter/loiter/leave transitions
- Per-zone per-class cooldown manager
- Detection zones with polygon validation, overlapping zone support, and per-zone class filters (KAI-41)
- Detection event deduplication filter (KAI-42)
- Per-class confidence thresholds for AI detections (KAI-43)
- Detection event aggregation into compact summaries (KAI-44)
- CLIP search index management (KAI-45)
- Detection scheduling for AI pipelines (KAI-46)
- Model management API for hot-swapping detection models (KAI-47)
- Detection webhook dispatch with retry and delivery log (KAI-48)
- Detection performance metrics (KAI-49)
- Detection pipeline auto-scaling based on system load (KAI-40)
- Load-based frame rate auto-scaling
- ONVIF metadata stream event parsing for motion/tampering
- Push-based camera status and detection updates (replaces polling)
- Detection frame WebSocket events for Flutter overlay

#### ONVIF Extended Services

- Device Service SET operations (KAI-111)
- Media2 profile and source configuration handlers
- Imaging service: exposure, focus, WDR, white balance, IR cut filter controls
- Audio source configurations with Media2 fallback
- Metadata configuration CRUD
- GetServices and GetServiceCapabilities for Profile T negotiation

#### Security and Access Control

- Role-based access control with per-camera permissions (KAI-75)
- Session management with device tracking and configurable timeouts (KAI-76)
- Brute-force protection with IP rate limiting and account lockout (KAI-77)
- Network security hardening middleware (KAI-81)
- TLS certificate management (KAI-74)

#### System Administration

- Encrypted configuration backup and restore (KAI-78)
- Automatic database maintenance with vacuum and optimization (KAI-79)
- System update mechanism with check, apply, rollback, and history (KAI-80)
- Audit log with configurable retention and CSV/JSON export (KAI-82)
- System alerts and email notifications (KAI-83)
- System health metrics with ring buffer history
- Disk I/O status and threshold monitoring
- Schedule template CRUD and stream-schedule assignment
- Branding customization for product name, logo, and accent color (KAI-58)

#### React Admin Console (UI)

- Guided setup wizard with 4 steps (KAI-50)
- System health dashboard (KAI-51)
- Camera management with edit modal and status indicators (KAI-52)
- Storage management page with retention, quotas, and cleanup (KAI-53)
- User management with admin password reset (KAI-54)
- System configuration page: network, TLS, backup, updates (KAI-55)
- Audit log viewer (KAI-56)
- Stripped non-admin features from web console (KAI-57)

#### Flutter Client

- Live view with named grid layouts, double-tap fullscreen, and animated transitions (KAI-60)
- Visual playback timeline with zoom presets (1H/4H/12H/24H) and color-coded segments (KAI-61)
- Multi-camera synchronized playback UI (KAI-62)
- Clip export and sharing (KAI-67)
- Notification center (KAI-66)
- Camera status dashboard (KAI-68)
- User preferences persistence (KAI-69)
- Dark/light theme with system detection (KAI-70)
- Keyboard shortcuts for PTZ, playback, and grid selection (KAI-71)
- Responsive design with 3-tier breakpoints (KAI-72)
- Offline mode basics (KAI-73)
- Bookmark-to-playback navigation (KAI-35)
- Motion intensity heatmap timeline layer
- AI overlay rendering in live view
- Detection zones polygon editor in camera detail
- Transport controls, jog slider, and event detail popup
- RTSP playback for H.265 via fvp direct file serving
- Fullscreen mute/AI toggle, WebRTC goroutine leak fix, WS-based detections

#### Infrastructure

- Docker image with multi-arch support (KAI-85)
- SQLite database with pure Go driver (no CGO)
- Automated database migrations (39+ versions)
- Gin HTTP router for API
- Installation guide (KAI-94)
- Quick-start guide (KAI-100)

### Fixed

- AI overlay rendering in live view (KAI-63)
- Detection zones polygon parsing crash (KAI-65)
- Clip search type casting crashes (KAI-64)
- Export pipeline panic recovery and progress logging
- Migration conflict resolution across multiple feature branches
- WebRTC goroutine leak in fullscreen mode

---

*For the release notes template, see [docs/RELEASE_NOTES_TEMPLATE.md](docs/RELEASE_NOTES_TEMPLATE.md).*
*For the release process, see [docs/guides/release-process.md](docs/guides/release-process.md).*
