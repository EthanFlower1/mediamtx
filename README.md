# MediaMTX NVR

> Open-source network video recorder built on [MediaMTX](https://github.com/bluenviron/mediamtx)

MediaMTX NVR transforms the MediaMTX streaming server into a full-featured network video recorder. It adds a React-based management UI, ONVIF camera discovery and control, recording schedules, motion detection, multi-camera playback, and user authentication — all backed by a zero-dependency SQLite database.

## Features

**Camera Management**
- ONVIF auto-discovery and manual RTSP/RTMP/HLS/SRT camera setup
- PTZ (pan/tilt/zoom) controls with preset support
- Camera settings: imaging, analytics, relay outputs, audio intercom
- Edge recording import from camera storage

**Recording & Playback**
- Continuous and event-driven recording (fMP4 or MPEG-TS)
- Visual timeline with multi-camera synchronized playback
- Clip search, saved clips, and MP4 export/download
- Configurable retention and automatic cleanup

**Motion & Events**
- ONVIF push-based motion detection (no polling)
- Configurable motion timeout per camera
- Real-time WebSocket notifications
- Motion event timeline overlay

**Recording Schedules**
- Always-on or events-only recording modes
- Day-of-week and time-range rules per camera
- Schedule preview visualization

**Security**
- JWT authentication with RSA key pairs and JWKS endpoint
- Role-based user management (admin/viewer)
- AES-256 encrypted credential storage
- Audit logging for all administrative actions
- Argon2 password hashing

**Administration**
- Storage monitoring with low-space warnings
- Configuration export/import
- Prometheus-compatible metrics endpoint
- Dark mode UI

## Quick Start

### Prerequisites

- **Go 1.25+**
- **Node.js 20+** (for building the UI)
- An ONVIF-compatible IP camera, or any RTSP camera

### Installation

#### From Source

```bash
# Clone the repository
git clone https://github.com/bluenviron/mediamtx.git
cd mediamtx

# Build the React UI
cd ui && npm ci && npm run build && cd ..
# Or: make nvr-ui

# Copy built UI into the embed directory
cp -r ui/dist/* internal/nvr/ui/dist/

# Generate required files
go generate ./...

# Build and run
go build -o mediamtx .
./mediamtx
```

#### Docker

```bash
docker run --rm -it \
  -v "$(pwd)/mediamtx.yml:/mediamtx.yml" \
  -v "$(pwd)/recordings:/recordings" \
  -p 9997:9997 \
  -p 8554:8554 \
  -p 8889:8889/udp \
  -p 8189:8189/udp \
  -p 9996:9996 \
  -p 9998:9998 \
  bluenviron/mediamtx
```

### First Run

1. Open the NVR UI at **http://localhost:9997**
2. You will be directed to the **Setup** page to create an admin account
3. Log in, then navigate to **Camera Management** to add your first camera
4. Use **ONVIF Discovery** to auto-detect cameras on your network, or add one manually with an RTSP URL

## Configuration

### mediamtx.yml

The NVR is configured through the standard `mediamtx.yml` file. Key settings:

```yaml
# ---- NVR ----
nvr: yes                              # Enable NVR functionality
nvrDatabase: ~/.mediamtx/nvr.db       # SQLite database path
nvrJWTSecret: ""                      # Auto-generated on first run if empty

# ---- Recording ----
pathDefaults:
  record: true                        # Enable recording for all paths
  recordPath: ./recordings/%path/%Y-%m-%d_%H-%M-%S-%f
  recordFormat: fmp4                  # fmp4 or mpegts
  recordPartDuration: 1s             # RPO -- data lost on crash
  recordSegmentDuration: 1h          # Segment rotation interval
  recordDeleteAfter: 1d              # Retention period (0s = keep forever)
```

### Ports

| Port | Protocol | Service |
|------|----------|---------|
| 9997 | TCP | NVR UI + Control API |
| 8554 | TCP | RTSP server |
| 8322 | TCP | RTSPS (RTSP over TLS) |
| 1935 | TCP | RTMP server |
| 8888 | TCP | HLS server |
| 8889 | TCP | WebRTC HTTP server |
| 8189 | UDP | WebRTC ICE/UDP |
| 8890 | TCP | SRT server |
| 9996 | TCP | Playback server (recording download) |
| 9998 | TCP | WebSocket notifications |

## Usage Guide

### Adding a Camera

**ONVIF Discovery** -- Go to Camera Management and click **Discover Cameras**. The system sends a WS-Discovery probe on your local network and returns all ONVIF-compatible devices. Select a camera, enter credentials, and the system auto-configures the RTSP stream.

**Manual** -- Click **Add Camera** and provide a name and RTSP URL (e.g., `rtsp://user:pass@192.168.1.100:554/stream1`). The camera will appear in your live view grid immediately.

### Live View

- Arrange cameras in configurable grid layouts (1x1 through 4x4)
- Click a camera tile to view fullscreen with PTZ controls
- Use the audio intercom for two-way audio on supported cameras
- Take screenshots directly from the live view

### Recordings & Playback

- Navigate to **Recordings** to browse by camera and date
- Use the **Timeline** to scrub through recordings visually
- Open **Playback** for multi-camera synchronized playback
- Use **Clip Search** to find and save clips by time range
- Export clips as MP4 files for download

### Motion Detection

Motion detection uses ONVIF push events -- the camera notifies MediaMTX NVR when motion starts and stops. No CPU-intensive video analysis required.

- Enable motion detection per camera in Camera Settings
- Configure **motion timeout** to control how long after the last event motion is considered active
- Motion events appear on the recording timeline as highlighted regions
- Real-time notifications are delivered via WebSocket to the UI

### Recording Schedules

Each camera can have independent recording rules:

- **Always** -- record continuously 24/7
- **Events** -- record only when motion is detected
- Rules can be scoped to specific days of the week and time ranges
- Preview your schedule visually before saving

### User Management

- **Admin** -- full access to all features, camera configuration, and user management
- **Viewer** -- live view and playback access only
- Change passwords from the UI; passwords are hashed with Argon2
- All admin actions are recorded in the audit log

### Settings

- **Storage** -- monitor disk usage, configure retention, trigger manual cleanup
- **Notifications** -- real-time alerts for motion, camera disconnect, low storage
- **Configuration** -- export/import `mediamtx.yml` settings from the UI
- **Audit Log** -- review all administrative actions with timestamps and user attribution

## Architecture

```
+-------------------------------------------------+
|            React UI (Vite + Tailwind)            |
|   LiveView / Playback / Cameras / Settings       |
+------------------------+------------------------+
                         | HTTP/WS
+------------------------v------------------------+
|            Go Backend (Gin HTTP router)          |
|                                                  |
|  +----------+ +----------+ +------------------+  |
|  | NVR API  | |Scheduler | | ONVIF Discovery  |  |
|  | (CRUD,   | |(rules,   | | & Event Callback |  |
|  |  auth,   | | motion,  | |  Manager         |  |
|  |  audit)  | | record)  | |                  |  |
|  +----+-----+ +----+-----+ +--------+---------+  |
|       |            |                |             |
|  +----v------------v----------------v----------+  |
|  |           SQLite Database                   |  |
|  |  cameras, users, rules, clips, audit, events|  |
|  +---------------------------------------------+  |
|                                                  |
|  +---------------------------------------------+  |
|  |         MediaMTX Streaming Core             |  |
|  |  RTSP / RTMP / HLS / WebRTC / SRT / Record |  |
|  +---------------------------------------------+  |
+--------------------------------------------------+
```

**Key components:**
- **Go backend** -- Gin-based HTTP server serves both the API and the embedded React UI
- **SQLite** -- single-file database via `modernc.org/sqlite` (pure Go, no CGO)
- **ONVIF client** -- WS-Discovery, PTZ, analytics, events, imaging, relay, and edge recording
- **Scheduler** -- evaluates recording rules and manages motion-triggered recording
- **WebSocket server** -- real-time event delivery on port 9998
- **YAML writer** -- programmatic updates to `mediamtx.yml` when cameras are added/removed

## API Reference

All NVR endpoints are under `/api/nvr`. Protected routes require a JWT in the `Authorization: Bearer` header.

### Authentication
| Method | Endpoint | Description |
|--------|----------|-------------|
| POST | `/auth/setup` | Create initial admin account |
| POST | `/auth/login` | Obtain JWT tokens |
| POST | `/auth/refresh` | Refresh access token |
| POST | `/auth/revoke` | Revoke refresh token |

### Cameras
| Method | Endpoint | Description |
|--------|----------|-------------|
| GET | `/cameras` | List all cameras |
| POST | `/cameras` | Add a camera |
| GET | `/cameras/:id` | Get camera details |
| PUT | `/cameras/:id` | Update a camera |
| DELETE | `/cameras/:id` | Remove a camera |
| POST | `/cameras/discover` | Start ONVIF discovery |
| GET | `/cameras/discover/results` | Get discovery results |
| POST | `/cameras/probe` | Probe a camera URL |
| POST | `/cameras/:id/ptz` | Send PTZ command |

### Recordings
| Method | Endpoint | Description |
|--------|----------|-------------|
| GET | `/recordings` | Query recordings |
| GET | `/timeline` | Get recording timeline |
| GET | `/recordings/:id/download` | Download a segment |
| POST | `/recordings/export` | Export clip as MP4 |
| GET | `/cameras/:id/motion-events` | Get motion events |

### Recording Rules
| Method | Endpoint | Description |
|--------|----------|-------------|
| GET | `/cameras/:id/recording-rules` | List rules |
| POST | `/cameras/:id/recording-rules` | Create a rule |
| PUT | `/recording-rules/:id` | Update a rule |
| DELETE | `/recording-rules/:id` | Delete a rule |

### Users & System
| Method | Endpoint | Description |
|--------|----------|-------------|
| GET | `/users` | List users |
| POST | `/users` | Create user |
| PUT | `/auth/password` | Change password |
| GET | `/system/info` | Server info |
| GET | `/system/storage` | Storage stats |
| GET | `/audit` | Audit log |

## Development

### Project Structure

```
mediamtx/
├── internal/
│   ├── nvr/               # NVR subsystem
│   │   ├── api/           # HTTP handlers and middleware
│   │   ├── db/            # SQLite database and migrations
│   │   ├── onvif/         # ONVIF client (discovery, PTZ, events, etc.)
│   │   ├── scheduler/     # Recording rule evaluation and motion tracking
│   │   ├── crypto/        # AES encryption for stored credentials
│   │   ├── yamlwriter/    # Programmatic mediamtx.yml updates
│   │   └── ui/            # Embedded UI assets (dist/)
│   ├── core/              # MediaMTX core server
│   ├── conf/              # Configuration parsing
│   ├── recorder/          # Stream-to-disk recording engine
│   ├── playback/          # Recording playback server
│   └── servers/           # Protocol servers (RTSP, RTMP, HLS, WebRTC, SRT)
├── ui/                    # React frontend source
│   ├── src/
│   │   ├── pages/         # Page components (LiveView, Playback, Settings, etc.)
│   │   ├── components/    # Reusable components (Timeline, PTZ, VideoPlayer, etc.)
│   │   └── api/           # API client functions
│   └── package.json
├── mediamtx.yml           # Configuration file
├── docker/                # Dockerfiles (standard, ffmpeg, rpi)
├── scripts/               # Build scripts (Makefile includes)
└── go.mod
```

### Building

```bash
# Build UI
make nvr-ui

# Copy UI dist into embed directory
cp -r ui/dist/* internal/nvr/ui/dist/

# Generate files (version, etc.)
go generate ./...

# Build binary
go build -o mediamtx .

# Build release binaries for all platforms (requires Docker)
make binaries
```

### Running Tests

```bash
# Run all tests locally
go generate ./...
go test -race ./internal/...

# Run tests in Docker (matches CI)
make test

# Run end-to-end tests
make test-e2e

# Lint
make lint
```

### Contributing

1. Fork the repository and create a feature branch
2. Ensure `make lint` and `make test` pass
3. Follow the existing code style -- `make format` will auto-format
4. Write tests for new functionality
5. Submit a pull request with a clear description of the change

## License

This project is licensed under the [MIT License](LICENSE).
