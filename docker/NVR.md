# MediaMTX NVR - Docker

## Quick Start

```bash
# Build and run with Docker Compose
docker compose up -d

# Or build the image manually
docker build -f docker/nvr.Dockerfile -t mediamtx-nvr .
docker run -d --network host --name mediamtx-nvr mediamtx-nvr
```

## Multi-Architecture Build

The NVR Dockerfile supports `linux/amd64` and `linux/arm64` via Docker Buildx:

```bash
docker buildx create --name mediamtx-builder --use
docker buildx build \
  -f docker/nvr.Dockerfile \
  --platform linux/amd64,linux/arm64 \
  -t mediamtx-nvr:latest \
  .
```

## Configuration

### Environment Variables

Any `mediamtx.yml` setting can be overridden via environment variables using the
`MTX_` prefix. The variable name is the uppercase version of the YAML key:

| Variable | Default | Description |
|---|---|---|
| `MTX_LOGLEVEL` | `info` | Log verbosity: error, warn, info, debug |
| `MTX_API` | `yes` | Enable the REST API |
| `MTX_NVR` | `yes` | Enable NVR subsystem |
| `MTX_PLAYBACK` | `yes` | Enable playback server |
| `MTX_NVRJWTSECRET` | *(from yml)* | JWT secret for DB key encryption |
| `TZ` | `UTC` | Container timezone |

### Volumes

| Path | Purpose |
|---|---|
| `/config` | `mediamtx.yml` configuration file |
| `/data` | SQLite database and NVR state |
| `/recordings` | Video recording storage |

### Ports

| Port | Protocol | Service |
|---|---|---|
| 8554 | TCP | RTSP |
| 8000 | UDP | RTSP RTP |
| 8001 | UDP | RTSP RTCP |
| 1935 | TCP | RTMP |
| 8888 | TCP | HLS |
| 8889 | TCP | WebRTC HTTP |
| 8189 | UDP | WebRTC ICE |
| 9996 | TCP | API / Playback |
| 9997 | TCP | Metrics |

## Health Check

The container includes a built-in health check that polls the API every 30 seconds:

```
GET http://localhost:9997/v3/paths/list
```

Check container health:
```bash
docker inspect --format='{{.State.Health.Status}}' mediamtx-nvr
```

## Networking

The default `docker-compose.yml` uses `network_mode: host` for optimal
UDP performance (RTSP, WebRTC). If you need bridge networking, comment out
`network_mode: host` and uncomment the `ports:` section in `docker-compose.yml`.

## Persistence

All state is stored in named Docker volumes by default. To use bind mounts
instead (recommended for production backups):

```yaml
volumes:
  - ./config:/config
  - ./data:/data
  - /mnt/storage/recordings:/recordings
```
