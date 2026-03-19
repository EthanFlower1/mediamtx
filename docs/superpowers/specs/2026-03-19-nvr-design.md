# MediaMTX NVR — v1 Design Spec

## Overview

Transform MediaMTX into a production-ready Network Video Recorder (NVR) that ships as a single binary on any platform. Target audience for v1 is prosumer/self-hosters (4-32 cameras), architected to scale for commercial/enterprise use in the future.

NVR functionality is added as internal packages within the existing MediaMTX codebase (not a wrapper or sidecar). When disabled via config, MediaMTX behaves exactly as it does today with zero overhead.

## Guiding Principles

- **Single binary** — no external dependencies, no separate processes
- **Additive** — NVR layers on top of MediaMTX, doesn't replace its core
- **YAML stays** — server config remains in `mediamtx.yml`; NVR manages cameras by writing to YAML and leveraging hot-reload
- **SQLite for metadata** — camera metadata, recording indexes, users
- **v1 scope** — recording, playback, camera management, ONVIF, UI. No motion detection or event system.

## 1. Database Layer

### Technology

`modernc.org/sqlite` — pure Go SQLite implementation. No CGO required, cross-compiles cleanly for all target platforms.

### Package

`internal/nvr/db/`

Thin wrapper around SQLite providing:
- Connection management (single writer, multiple readers via WAL mode)
- Embedded migrations (Go files, run on startup)
- Query helpers

### Schema

**cameras**
| Column | Type | Description |
|--------|------|-------------|
| id | TEXT (UUID) | Primary key |
| name | TEXT | User-friendly name |
| onvif_endpoint | TEXT | ONVIF device service URL |
| onvif_username | TEXT | ONVIF credentials |
| onvif_password | TEXT | ONVIF credentials (encrypted at rest) |
| onvif_profile_token | TEXT | Selected media profile |
| rtsp_url | TEXT | Resolved RTSP stream URL |
| ptz_capable | BOOLEAN | Whether camera supports PTZ |
| mediamtx_path | TEXT | Corresponding path name in mediamtx.yml |
| status | TEXT | online/offline/error |
| tags | TEXT (JSON) | Grouping/organization |
| created_at | DATETIME | |
| updated_at | DATETIME | |

**recordings**
| Column | Type | Description |
|--------|------|-------------|
| id | INTEGER | Primary key, autoincrement |
| camera_id | TEXT (FK) | Reference to cameras table |
| start_time | DATETIME | Segment start (indexed) |
| end_time | DATETIME | Segment end (indexed) |
| file_path | TEXT | Path to recording file on disk |
| file_size | INTEGER | Bytes |
| format | TEXT | fmp4 or mpegts |

Composite index on (camera_id, start_time, end_time) for fast timeline queries.

**users**
| Column | Type | Description |
|--------|------|-------------|
| id | TEXT (UUID) | Primary key |
| username | TEXT | Unique |
| password_hash | TEXT | Argon2id hash |
| role | TEXT | admin or viewer |
| camera_permissions | TEXT (JSON) | Array of camera IDs, or "*" for all |
| created_at | DATETIME | |
| updated_at | DATETIME | |

**refresh_tokens**
| Column | Type | Description |
|--------|------|-------------|
| id | TEXT (UUID) | Primary key |
| user_id | TEXT (FK) | Reference to users table |
| token_hash | TEXT | SHA-256 hash of refresh token |
| expires_at | DATETIME | |
| revoked_at | DATETIME | Nullable, set on revocation |

### Relationship to YAML

- Server-level config (ports, logging, TLS, protocol settings) stays in `mediamtx.yml`
- When a camera is added via the NVR UI, the NVR writes a new path entry into `mediamtx.yml` with the RTSP source URL, recording settings, etc.
- MediaMTX's existing `confwatcher` detects the change and hot-reloads — no restart, no client disconnection
- YAML-defined paths that weren't created by the NVR continue to work unchanged
- SQLite stores NVR-specific metadata (ONVIF endpoints, PTZ capabilities, friendly names, tags) that doesn't belong in the YAML

## 2. ONVIF Integration

### Package

`internal/nvr/onvif/` using `github.com/kerberos-io/onvif`

### Discovery Flow

1. User triggers network scan from the UI (or configures periodic scanning)
2. WS-Discovery probes the local subnet for ONVIF-compliant devices
3. For each discovered device, fetch:
   - Device information (manufacturer, model, firmware)
   - Media profiles (resolution, codec, framerate options)
   - PTZ capabilities
4. Present discovered cameras in the UI with available profiles
5. User selects a camera, picks a media profile, assigns a friendly name
6. NVR resolves the RTSP stream URL from the selected profile
7. NVR writes the RTSP URL as a new path in `mediamtx.yml` (triggers hot-reload)
8. NVR stores ONVIF metadata in SQLite

### Camera Settings Management

Expose ONVIF service calls through NVR API endpoints:

- **Media service** — list/change profiles, resolution, framerate, encoding
- **Imaging service** — brightness, contrast, saturation, sharpness, IR cut filter
- **PTZ service** — continuous move, absolute/relative positioning, preset management

Settings are pushed directly to the camera via ONVIF — not stored locally (the camera is the source of truth for its own settings).

### Data Storage

- **SQLite:** ONVIF endpoint URL, credentials, device info, profile tokens, PTZ capability flags, friendly name, tags
- **YAML (via hot-reload write):** RTSP source URL, recording enable/disable, recording format, retention settings

## 3. Recording Metadata & Timeline

### Current State

MediaMTX already records to fMP4/MPEG-TS segments on disk with configurable retention and cleanup. The playback API can list and serve segments. But timeline queries require scanning the filesystem — no indexed metadata exists.

### What We Add

- Hook into MediaMTX's existing recorder segment-completion callbacks to insert rows into the `recordings` table (camera_id, start_time, end_time, file_path, file_size, format)
- Hook into the record cleaner to delete corresponding database rows when old files are removed
- This enables fast timeline queries via SQL instead of directory walks

### Timeline API

- `GET /api/nvr/recordings?camera_id=X&start=T1&end=T2` — returns recording segments for a time range
- `GET /api/nvr/timeline?camera_id=X&date=D` — returns a compact representation of recording coverage (for painting the timeline bar in the UI — array of {start, end} ranges)
- `GET /api/nvr/recordings/{id}/download` — download a specific segment

Actual video playback continues to use MediaMTX's existing playback server — the NVR layer provides the index only.

## 4. HTTP API

### Router

Uses MediaMTX's existing gin router. NVR endpoints are registered as a new route group on the existing API server.

### NVR API Endpoints

All under `/api/nvr/`:

**Auth**
- `POST /auth/login` — validate credentials, return access JWT + httpOnly refresh cookie
- `POST /auth/refresh` — issue new access JWT from refresh token
- `POST /auth/revoke` — revoke refresh token (logout)

**Cameras**
- `GET /cameras` — list all cameras
- `POST /cameras` — add camera (manual or from ONVIF discovery)
- `GET /cameras/{id}` — get camera details
- `PUT /cameras/{id}` — update camera
- `DELETE /cameras/{id}` — remove camera (also removes path from YAML)
- `POST /cameras/discover` — trigger ONVIF network scan
- `GET /cameras/discover/results` — get discovery results

**Camera Controls**
- `POST /cameras/{id}/ptz` — send PTZ command (move, stop, goto preset)
- `GET /cameras/{id}/ptz/presets` — list PTZ presets
- `GET /cameras/{id}/settings` — get current camera settings via ONVIF
- `PUT /cameras/{id}/settings` — push settings to camera via ONVIF

**Recordings**
- `GET /recordings?camera_id=X&start=T1&end=T2` — query recording segments
- `GET /timeline?camera_id=X&date=D` — get timeline coverage
- `GET /recordings/{id}/download` — download segment

**Users** (admin only)
- `GET /users` — list users
- `POST /users` — create user
- `GET /users/{id}` — get user
- `PUT /users/{id}` — update user
- `DELETE /users/{id}` — delete user

**System**
- `GET /system/info` — version, uptime, platform
- `GET /system/storage` — disk usage, recording stats

### Auth Middleware

Gin middleware on all `/api/nvr/` routes (except `/auth/login`) that:
- Validates JWT signature and expiration
- Extracts user context (id, role, camera permissions)
- Enforces role-based access (admin-only routes)
- Filters camera-scoped responses based on permissions

### Embedded UI

React app served via gin static file handler from `go:embed` filesystem. Catchall route serves `index.html` for client-side routing.

## 5. Web UI

### Technology

React with Vite. Built as a pre-step, output embedded into the Go binary via `go:embed`.

### Source Location

`ui/` at the repository root (keeps frontend separate from Go internals).

### Views

**1. Dashboard / Live View** (default landing page)
- Multi-camera grid with selectable layouts (1x1, 2x2, 3x3, custom)
- Each cell renders a WebRTC or HLS stream via MediaMTX's existing player endpoints
- Click a cell to expand to single-camera fullscreen view
- PTZ overlay controls on cameras that support it (directional pad, zoom, presets dropdown)
- Camera status indicators (online/offline)

**2. Camera Management**
- ONVIF discovery panel: scan button, results list with device info
- Camera list with status, thumbnail (if available), stream info
- Add camera form: ONVIF auto-populate or manual RTSP URL entry
- Edit camera: friendly name, recording on/off, retention, ONVIF profile selection
- Camera settings panel: resolution, framerate, encoding, imaging adjustments (proxied to camera via ONVIF)

**3. Recordings Browser**
- Calendar picker to select date
- Timeline bar per camera showing recording coverage (colored segments, gaps visible)
- Multi-camera stacked timeline view
- Click/drag on timeline to scrub playback
- Clip export: select start/end points on timeline, download as MP4
- Playback controls: play/pause, speed (0.5x-8x), skip forward/back

**4. Settings**
- Storage: recording path, retention policies, disk usage visualization
- Server: ports, TLS toggle, log level (maps to mediamtx.yml fields)
- System info: version, uptime, platform, resource usage

**5. User Management** (admin only)
- User list with roles
- Create/edit user: username, password, role (admin/viewer)
- Camera permission assignment: select which cameras a user can access, or grant access to all

### Auth Flow

- Login page → `POST /api/nvr/auth/login` → receives access JWT (short-lived, 15min)
- Access JWT stored in memory (not localStorage, for XSS protection)
- Refresh token in httpOnly cookie (CSRF-safe, auto-sent)
- All API calls include `Authorization: Bearer <token>`
- Token refresh happens transparently on 401 responses
- Logout revokes refresh token server-side

## 6. User Management & Auth

### JWT Integration with MediaMTX

The NVR uses the same JWT signing key as MediaMTX's existing JWT auth system. This means:
- Access tokens issued by the NVR automatically work for stream access (RTSP, WebRTC, HLS)
- No separate authentication needed for viewing streams in the UI
- The `mediamtx_permissions` JWT claim is populated based on the user's role and camera permissions

### First-Run Setup

On first startup with an empty `users` table:
1. NVR enters setup mode
2. All routes redirect to a setup page
3. User creates an admin account (username + password)
4. Setup completes, normal operation begins
5. No access to any NVR features until setup is complete

### Password Storage

Argon2id hashing with recommended parameters (memory: 64MB, iterations: 3, parallelism: 4, salt: 16 bytes, key: 32 bytes).

## 7. Build & Distribution

### Build Pipeline

1. `make nvr-ui` — builds React app via Vite (`cd ui && npm ci && npm run build`)
2. Output placed in `internal/nvr/ui/dist/`
3. `make binaries` — updated to run `nvr-ui` first, then compile Go binary
4. `go:embed` directive in `internal/nvr/ui/embed.go` includes the dist directory

### Cross-Platform

- `modernc/sqlite` is pure Go — no CGO, cross-compilation works for all targets
- Same platform targets as current MediaMTX: linux/amd64, linux/arm/v6, linux/arm/v7, linux/arm64, darwin/amd64, darwin/arm64, windows/amd64
- Docker images include NVR functionality (same images, not separate)

### Configuration

```yaml
# mediamtx.yml additions
nvr: yes                              # enable NVR features (default: no)
nvrDatabase: ~/.mediamtx/nvr.db       # SQLite database path
nvrJWTSecret: ""                      # JWT signing key (auto-generated on first run if empty)
```

When `nvr: no` (default), no NVR code runs — no SQLite connection, no ONVIF, no UI serving. MediaMTX behaves identically to today.

## 8. Package Structure

```
internal/nvr/
  nvr.go              # NVR subsystem initialization, lifecycle
  db/
    db.go             # SQLite connection, migration runner
    migrations/       # Embedded migration files
    cameras.go        # Camera CRUD queries
    recordings.go     # Recording metadata queries
    users.go          # User CRUD queries
    tokens.go         # Refresh token queries
  onvif/
    discovery.go      # WS-Discovery network scanning
    device.go         # Device info, profile fetching
    media.go          # Media profile management
    imaging.go        # Imaging settings
    ptz.go            # PTZ control
  api/
    router.go         # Gin route group registration
    middleware.go      # JWT auth middleware
    cameras.go        # Camera endpoints
    recordings.go     # Recording/timeline endpoints
    users.go          # User management endpoints
    auth.go           # Login/refresh/revoke endpoints
    system.go         # System info endpoints
    discover.go       # ONVIF discovery endpoints
  ui/
    embed.go          # go:embed directive for React build output
    dist/             # Vite build output (gitignored, built at compile time)

ui/                   # React application source
  src/
    App.tsx
    pages/
      LiveView.tsx
      CameraManagement.tsx
      Recordings.tsx
      Settings.tsx
      UserManagement.tsx
      Login.tsx
      Setup.tsx
    components/
      CameraGrid.tsx
      Timeline.tsx
      PTZControls.tsx
      CameraSettingsPanel.tsx
      PlayerCell.tsx
    api/
      client.ts       # API client with JWT refresh logic
    auth/
      context.tsx     # Auth context provider
    hooks/
      useAuth.ts
      useCameras.ts
      useRecordings.ts
  vite.config.ts
  package.json
```

## 9. Future Considerations (Not in v1)

These are explicitly out of scope for v1 but the architecture supports them:

- **Motion detection / event system** — event table in SQLite, webhook API for external detectors, timeline markers
- **Multi-site management** — central server managing multiple NVR instances
- **Role-based access control (granular)** — beyond admin/viewer, custom roles with fine-grained permissions
- **Audit logging** — all user actions logged for compliance
- **Mobile app** — the API supports it, just needs a native client
- **AI/ML integration** — object detection, facial recognition, license plate reading via external services
- **Tiered storage** — hot/cold storage, cloud backup of recordings
- **LDAP/SAML/OIDC** — enterprise identity provider integration
