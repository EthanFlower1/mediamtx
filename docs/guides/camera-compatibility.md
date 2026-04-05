# Camera Compatibility Guide

This guide documents camera models tested with MediaMTX NVR, the ONVIF profiles and services we support, known vendor quirks, and how to contribute your own test results.

## Tested Cameras

| Manufacturer | Model | Firmware | ONVIF Version | Profile S | Profile T | PTZ | Events | Snapshots | Audio Backchannel | Notes |
|---|---|---|---|---|---|---|---|---|---|---|
| Hikvision | DS-2CD2143G2-I | V5.7.x | 21.06 | Yes | Yes | N/A (fixed) | Yes | Yes | No | Dual-stream (main + sub). Motion events via PullPoint. |
| Hikvision | DS-2DE4A425IWG-E | V5.7.x | 21.06 | Yes | Yes | Yes | Yes | Yes | Yes | 25x zoom PTZ. Presets and continuous move supported. |
| Dahua | IPC-HDW3849H-AS-PV | V2.820.x | 21.12 | Yes | Yes | N/A (fixed) | Yes | Yes | Yes | Full-color with active deterrence. Two-way audio works. |
| Dahua | SD49425GB-HNR | V2.820.x | 21.12 | Yes | Yes | Yes | Yes | Yes | Yes | 25x PTZ. Tour and preset support confirmed. |
| Axis | P3265-LVE | 11.x | 21.12 | Yes | Yes | N/A (fixed) | Yes | Yes | No | Rock-solid ONVIF. Edge analytics events via ONVIF. |
| Axis | Q6135-LE | 11.x | 21.12 | Yes | Yes | Yes | Yes | Yes | Yes | PTZ with autotracking. All services fully compliant. |
| Reolink | RLC-810A | V3.1.x | 2.6.1 | Yes | No | N/A (fixed) | Partial | Yes | No | No Profile T. Motion events may not fire via PullPoint; use polling. |
| Reolink | RLC-823A | V3.1.x | 2.6.1 | Yes | No | Yes (limited) | Partial | Yes | No | Pan/tilt only, no zoom. ONVIF event support is limited. |

### Legend

- **Profile S** -- Streaming (RTSP, media profiles, snapshot)
- **Profile T** -- Advanced streaming (H.265, Media2 service, imaging controls)
- **PTZ** -- Pan/Tilt/Zoom via ONVIF PTZ service
- **Events** -- Motion and analytics events via ONVIF PullPoint or metadata stream
- **Snapshots** -- JPEG snapshot via ONVIF GetSnapshotURI
- **Audio Backchannel** -- Two-way audio from NVR to camera speaker

## ONVIF Profile Support Matrix

MediaMTX NVR queries camera capabilities at connection time and adapts automatically. The table below shows which ONVIF services and profiles are supported by the NVR software itself.

| ONVIF Service | Namespace | NVR Support | Required Profile |
|---|---|---|---|
| Device Management | `devicemgmt` | Yes | Core |
| Media (Profile S) | `media` | Yes | S |
| Media2 (Profile T) | `ver20/media` | Yes | T |
| PTZ | `ptz` | Yes | S (optional) |
| Imaging | `imaging` | Yes | S (optional) |
| Events (PullPoint) | `events` | Yes | S |
| Analytics | `analytics` | Yes | T (optional) |
| DeviceIO | `deviceio` | Yes | Core |
| Recording | `recording` | Read-only query | G |
| Replay | `replay` | Read-only query | G |
| Search | `search` | Read-only query | G |

### Minimum Requirements

A camera must support at least:

1. **Device Management** -- required for discovery and connection.
2. **Media (Profile S)** -- required for RTSP stream URI retrieval and profile enumeration.
3. **Events** -- recommended for motion-triggered recording (falls back to continuous recording if absent).

### Profile T Benefits

Cameras advertising Media2 (Profile T) unlock:

- H.265/HEVC stream negotiation
- OSD (on-screen display) management via `CreateOSD` / `DeleteOSD`
- Privacy mask control
- Advanced imaging settings

## Known Quirks

### Hikvision

- **Digest auth on event subscriptions**: Hikvision cameras require digest authentication for PullPoint event subscriptions. MediaMTX handles this automatically.
- **Multi-channel NVRs**: When connecting to a Hikvision NVR (not a camera), each channel appears as a separate video source. Use the channel grouping feature (`VideoSourceToken`) to map profiles to physical channels.
- **Non-standard snapshot paths**: Some older Hikvision firmware versions return a relative path for `GetSnapshotURI`. MediaMTX resolves these against the device address.

### Dahua

- **Event topic namespace**: Dahua uses `tns1:RuleEngine/CellMotionDetector/Motion` for motion events, which differs from the more common `tns1:VideoAnalytics` topic. MediaMTX normalises both formats.
- **Audio backchannel codec**: Dahua cameras typically require G.711 mu-law for backchannel audio. Ensure your audio source matches.
- **HTTPS redirect**: Some Dahua firmware redirects ONVIF HTTP requests to HTTPS. If discovery fails, try adding the camera with an `https://` address.

### Axis

- **Analytics events**: Axis cameras can expose rich analytics (object detection, loitering, etc.) as ONVIF events. These are detected and forwarded as generic event types.
- **Firmware update resets**: After firmware updates, Axis cameras may regenerate their ONVIF service endpoints. Re-probe the camera if connections fail post-update.

### Reolink

- **Limited ONVIF implementation**: Reolink cameras implement a minimal ONVIF subset. They typically support Profile S only (no Media2/Profile T).
- **No PullPoint events**: Most Reolink models do not support ONVIF PullPoint event subscriptions. Motion detection should use the camera's proprietary API or polling-based approaches.
- **Snapshot authentication**: Reolink snapshot URIs require separate HTTP digest authentication even when the ONVIF session is authenticated. MediaMTX handles this with its digest-auth snapshot fetcher.
- **ONVIF must be enabled manually**: ONVIF is disabled by default on Reolink cameras. Enable it in the camera's web UI under Settings > Network > Advanced > Port Settings.

## Adding Your Camera

We welcome community contributions to expand this compatibility matrix. Follow these steps:

### 1. Gather Information

Connect your camera to MediaMTX NVR and use the capabilities API to collect service information:

```bash
# After adding the camera via the UI, query its capabilities
curl -s http://localhost:9997/v3/cameras/{camera-id}/capabilities | jq .
```

Record:
- Manufacturer and model
- Firmware version (check the camera's web UI)
- ONVIF version (returned in the capabilities response under `services[].version`)

### 2. Test Each Feature

Run through this checklist:

- [ ] **Discovery**: Does the camera appear in WS-Discovery scan results?
- [ ] **Stream retrieval**: Can MediaMTX pull the main and sub streams via RTSP?
- [ ] **Snapshots**: Does `GET /v3/cameras/{id}/snapshot` return a JPEG?
- [ ] **PTZ** (if applicable): Do continuous move, relative move, and preset recall work?
- [ ] **Events**: Do motion events appear in the event log within 5 seconds of triggering?
- [ ] **Audio backchannel** (if applicable): Can audio be sent from NVR to camera speaker?
- [ ] **OSD** (if Profile T): Can OSD text be created and deleted?

### 3. Submit a Pull Request

1. Fork the repository.
2. Edit this file (`docs/guides/camera-compatibility.md`).
3. Add a row to the **Tested Cameras** table with your results.
4. If you encountered any vendor-specific behaviour, add a section under **Known Quirks**.
5. Open a PR with the title: `docs: add [Manufacturer Model] to camera compatibility`.

### Report Template

If you prefer to report without a PR, open a GitHub issue with this template:

```
**Camera**: [Manufacturer] [Model]
**Firmware**: [version]
**ONVIF Version**: [version]

**Features tested**:
- [ ] Discovery
- [ ] Streaming (Profile S)
- [ ] Streaming (Profile T / H.265)
- [ ] Snapshots
- [ ] PTZ
- [ ] Motion events
- [ ] Audio backchannel
- [ ] OSD management

**Quirks or issues**:
[Describe any problems or workarounds]

**Capabilities JSON** (attach or paste):
[output of /v3/cameras/{id}/capabilities]
```

## Troubleshooting Common Issues

| Symptom | Likely Cause | Fix |
|---|---|---|
| Camera not found in discovery | ONVIF disabled on camera, or camera on different subnet | Enable ONVIF in camera web UI; ensure NVR and camera are on the same VLAN |
| "connect to ONVIF device" error | Wrong credentials or camera unreachable | Verify username/password; confirm camera IP is reachable via ping |
| Stream connects but no video | Codec mismatch or firewall blocking RTSP | Set camera to H.264 baseline; open UDP ports 6970-6999 or use TCP transport |
| Events not firing | Camera does not support PullPoint, or event service misconfigured | Check capabilities for `events: true`; for Reolink, use polling mode |
| Snapshot returns 401 | Separate auth required for snapshot URI | MediaMTX retries with digest auth automatically; verify camera credentials are correct |
| PTZ commands ignored | Wrong profile token or PTZ not supported | Confirm camera has a PTZ-capable profile; use the primary profile token |
