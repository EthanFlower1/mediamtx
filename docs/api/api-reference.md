# MediaMTX NVR API Reference

Base URL: `/api/nvr`

All protected endpoints require a JWT bearer token in the `Authorization` header:

```
Authorization: Bearer <token>
```

Tokens are obtained via `POST /api/nvr/auth/login` and refreshed via `POST /api/nvr/auth/refresh`.

---

## Table of Contents

- [Authentication](#authentication)
- [Cameras](#cameras)
- [Camera Discovery](#camera-discovery)
- [Camera PTZ and Settings](#camera-ptz-and-settings)
- [Media Configuration](#media-configuration)
- [Multicast Streaming](#multicast-streaming)
- [Media2 Configuration](#media2-configuration)
- [Device Info and Services](#device-info-and-services)
- [Device Management](#device-management)
- [Relay Outputs](#relay-outputs)
- [Audio](#audio)
- [Backchannel Audio](#backchannel-audio)
- [Edge Recordings](#edge-recordings)
- [Replay Control](#replay-control)
- [Recording Control](#recording-control)
- [Track Management](#track-management)
- [Camera AI Configuration](#camera-ai-configuration)
- [Detections](#detections)
- [Analytics](#analytics)
- [Metadata Configuration](#metadata-configuration)
- [OSD Management](#osd-management)
- [Recordings](#recordings)
- [Bulk Export](#bulk-export)
- [Recording Integrity](#recording-integrity)
- [Recording Statistics](#recording-statistics)
- [Recording Health](#recording-health)
- [Motion and Events](#motion-and-events)
- [Saved Clips](#saved-clips)
- [Bookmarks](#bookmarks)
- [Screenshots](#screenshots)
- [Timeline Thumbnails](#timeline-thumbnails)
- [Camera Streams](#camera-streams)
- [Recording Rules](#recording-rules)
- [Schedule Templates](#schedule-templates)
- [Stream Schedule Assignment](#stream-schedule-assignment)
- [Sessions](#sessions)
- [Users](#users)
- [System](#system)
- [Alerts and SMTP](#alerts-and-smtp)
- [Backups](#backups)
- [Security Configuration](#security-configuration)
- [System Updates](#system-updates)
- [TLS Certificate Management](#tls-certificate-management)
- [HLS VoD Playback](#hls-vod-playback)
- [Storage](#storage)
- [Storage Quotas](#storage-quotas)
- [AI Semantic Search](#ai-semantic-search)
- [Evidence Exports](#evidence-exports)
- [Edge Search](#edge-search)
- [Audit Log](#audit-log)
- [Camera Groups](#camera-groups)
- [Devices](#devices)
- [Tours](#tours)
- [Export Jobs](#export-jobs)
- [Camera Connection Resilience](#camera-connection-resilience)

---

## Authentication

### POST `/auth/login`

**Auth:** None

Log in with username and password. Returns JWT access and refresh tokens.

**Request:**
```json
{
  "username": "admin",
  "password": "secret"
}
```

**Response (200):**
```json
{
  "accessToken": "eyJhbGciOiJSUzI1NiIs...",
  "refreshToken": "eyJhbGciOiJSUzI1NiIs...",
  "expiresIn": 900
}
```

### POST `/auth/setup`

**Auth:** None

Create the initial admin user during first-time setup.

**Request:**
```json
{
  "username": "admin",
  "password": "securepassword123"
}
```

**Response (201):**
```json
{
  "accessToken": "eyJhbGciOiJSUzI1NiIs...",
  "refreshToken": "eyJhbGciOiJSUzI1NiIs..."
}
```

### POST `/auth/refresh`

**Auth:** None

Exchange a refresh token for a new access token.

**Request:**
```json
{
  "refreshToken": "eyJhbGciOiJSUzI1NiIs..."
}
```

**Response (200):**
```json
{
  "accessToken": "eyJhbGciOiJSUzI1NiIs...",
  "expiresIn": 900
}
```

### POST `/auth/revoke`

**Auth:** None

Revoke a refresh token.

**Request:**
```json
{
  "refreshToken": "eyJhbGciOiJSUzI1NiIs..."
}
```

**Response (204):** No content.

### GET `/.well-known/jwks.json`

**Auth:** None

Returns the JSON Web Key Set for token verification.

**Response (200):**
```json
{
  "keys": [
    {
      "kty": "RSA",
      "n": "...",
      "e": "AQAB",
      "kid": "1",
      "use": "sig",
      "alg": "RS256"
    }
  ]
}
```

### PUT `/auth/password`

**Auth:** JWT required

Change the authenticated user's password.

**Request:**
```json
{
  "currentPassword": "oldpass",
  "newPassword": "newpass"
}
```

**Response (204):** No content.

---

## Cameras

### GET `/cameras`

**Auth:** JWT required

List all cameras.

**Response (200):**
```json
[
  {
    "id": "cam-abc123",
    "name": "Front Door",
    "path": "front-door",
    "rtspUrl": "rtsp://192.168.1.100:554/stream1",
    "onvifHost": "192.168.1.100",
    "status": "online",
    "retentionDays": 30,
    "createdAt": "2025-01-15T10:00:00Z"
  }
]
```

### POST `/cameras`

**Auth:** JWT required

Add a new camera.

**Request:**
```json
{
  "name": "Front Door",
  "path": "front-door",
  "rtspUrl": "rtsp://192.168.1.100:554/stream1",
  "onvifHost": "192.168.1.100",
  "onvifPort": 80,
  "username": "admin",
  "password": "camera123"
}
```

**Response (201):**
```json
{
  "id": "cam-abc123",
  "name": "Front Door",
  "path": "front-door"
}
```

### POST `/cameras/multi-channel`

**Auth:** JWT required

Add a multi-channel camera (e.g., NVR with multiple video sources).

**Request:**
```json
{
  "name": "NVR Unit",
  "onvifHost": "192.168.1.200",
  "onvifPort": 80,
  "username": "admin",
  "password": "password",
  "channels": [1, 2, 3, 4]
}
```

**Response (201):**
```json
[
  { "id": "cam-ch1", "name": "NVR Unit - Channel 1", "channel": 1 },
  { "id": "cam-ch2", "name": "NVR Unit - Channel 2", "channel": 2 }
]
```

### GET `/cameras/:id`

**Auth:** JWT required

Get a single camera by ID.

**Response (200):**
```json
{
  "id": "cam-abc123",
  "name": "Front Door",
  "path": "front-door",
  "rtspUrl": "rtsp://192.168.1.100:554/stream1",
  "status": "online"
}
```

### PUT `/cameras/:id`

**Auth:** JWT required

Update a camera's configuration.

**Request:**
```json
{
  "name": "Front Entrance",
  "rtspUrl": "rtsp://192.168.1.100:554/stream2"
}
```

**Response (200):**
```json
{
  "id": "cam-abc123",
  "name": "Front Entrance"
}
```

### DELETE `/cameras/:id`

**Auth:** JWT required

Delete a camera and its associated configuration.

**Response (204):** No content.

---

## Camera Discovery

### POST `/cameras/discover`

**Auth:** JWT required

Start an ONVIF camera discovery scan on the local network. Returns immediately; poll status via the status endpoint.

**Response (202):**
```json
{
  "message": "discovery started"
}
```

### GET `/cameras/discover/status`

**Auth:** JWT required

Get the status of a running discovery scan.

**Response (200):**
```json
{
  "running": true,
  "elapsed": "3s"
}
```

### GET `/cameras/discover/results`

**Auth:** JWT required

Get the results from the most recent discovery scan.

**Response (200):**
```json
[
  {
    "xaddr": "http://192.168.1.100:80/onvif/device_service",
    "name": "HIKVISION DS-2CD2143G2-I",
    "hardware": "DS-2CD2143G2-I",
    "scopes": ["onvif://www.onvif.org/type/video_encoder"]
  }
]
```

### POST `/cameras/probe`

**Auth:** JWT required

Probe a specific ONVIF device for its capabilities and streams.

**Request:**
```json
{
  "host": "192.168.1.100",
  "port": 80,
  "username": "admin",
  "password": "camera123"
}
```

**Response (200):**
```json
{
  "name": "HIKVISION DS-2CD2143G2-I",
  "profiles": ["mainStream", "subStream"],
  "ptzSupported": true,
  "audioSupported": true
}
```

### POST `/cameras/:id/refresh`

**Auth:** JWT required

Refresh a camera's ONVIF capabilities from the device.

**Response (200):**
```json
{
  "message": "capabilities refreshed"
}
```

### POST `/cameras/:id/rotate-credentials`

**Auth:** JWT required

Rotate the stored ONVIF credentials for a camera.

**Request:**
```json
{
  "username": "admin",
  "password": "newPassword123"
}
```

**Response (200):**
```json
{
  "message": "credentials updated"
}
```

---

## Camera PTZ and Settings

### POST `/cameras/:id/ptz`

**Auth:** JWT required

Send a PTZ (Pan-Tilt-Zoom) command to a camera.

**Request:**
```json
{
  "action": "continuousMove",
  "panSpeed": 0.5,
  "tiltSpeed": 0.0,
  "zoomSpeed": 0.0
}
```

**Response (200):**
```json
{
  "message": "ok"
}
```

### GET `/cameras/:id/ptz/presets`

**Auth:** JWT required

List PTZ presets configured on the camera.

**Response (200):**
```json
[
  { "token": "1", "name": "Home" },
  { "token": "2", "name": "Parking Lot" }
]
```

### GET `/cameras/:id/ptz/capabilities`

**Auth:** JWT required

Get PTZ capability details for a camera.

**Response (200):**
```json
{
  "absoluteMove": true,
  "relativeMove": true,
  "continuousMove": true,
  "homeSupported": true,
  "maxPresets": 128
}
```

### GET `/cameras/:id/ptz/status`

**Auth:** JWT required

Get current PTZ position and move status.

**Response (200):**
```json
{
  "position": { "pan": 0.0, "tilt": 0.0, "zoom": 1.0 },
  "moveStatus": { "panTilt": "IDLE", "zoom": "IDLE" }
}
```

### GET `/cameras/:id/settings`

**Auth:** JWT required

Get current imaging settings (brightness, contrast, etc.) for a camera.

**Response (200):**
```json
{
  "brightness": 50.0,
  "contrast": 50.0,
  "colorSaturation": 50.0,
  "sharpness": 50.0,
  "irCutFilter": "AUTO"
}
```

### PUT `/cameras/:id/settings`

**Auth:** JWT required

Update imaging settings on the camera.

**Request:**
```json
{
  "brightness": 60.0,
  "contrast": 55.0
}
```

**Response (200):**
```json
{
  "message": "settings updated"
}
```

### GET `/cameras/:id/settings/options`

**Auth:** JWT required

Get available imaging setting ranges and options.

**Response (200):**
```json
{
  "brightness": { "min": 0, "max": 100 },
  "contrast": { "min": 0, "max": 100 }
}
```

### GET `/cameras/:id/settings/status`

**Auth:** JWT required

Get the current imaging status from the camera.

**Response (200):**
```json
{
  "focusStatus": { "position": 0.5, "moveStatus": "IDLE" }
}
```

### GET `/cameras/:id/settings/focus/move-options`

**Auth:** JWT required

Get available focus move options.

**Response (200):**
```json
{
  "absolute": { "min": 0.0, "max": 1.0 },
  "relative": { "min": -1.0, "max": 1.0 },
  "continuous": { "min": -1.0, "max": 1.0 }
}
```

### POST `/cameras/:id/settings/focus/move`

**Auth:** JWT required

Move the camera focus.

**Request:**
```json
{
  "absolute": { "position": 0.5, "speed": 0.5 }
}
```

**Response (200):**
```json
{
  "message": "ok"
}
```

### POST `/cameras/:id/settings/focus/stop`

**Auth:** JWT required

Stop an in-progress focus move.

**Response (200):**
```json
{
  "message": "ok"
}
```

### PUT `/cameras/:id/retention`

**Auth:** JWT required

Update the retention period for a camera's recordings.

**Request:**
```json
{
  "retentionDays": 60
}
```

**Response (200):**
```json
{
  "message": "retention updated"
}
```

### GET `/cameras/:id/storage-estimate`

**Auth:** JWT required

Get an estimated storage usage for a camera based on current settings.

**Response (200):**
```json
{
  "estimatedDailyGB": 12.5,
  "estimatedTotalGB": 375.0,
  "retentionDays": 30
}
```

### PUT `/cameras/:id/motion-timeout`

**Auth:** JWT required

Update the motion detection timeout for a camera.

**Request:**
```json
{
  "motionTimeoutSec": 30
}
```

**Response (200):**
```json
{
  "message": "motion timeout updated"
}
```

---

## Media Configuration

### GET `/cameras/:id/media/profiles`

**Auth:** JWT required

List ONVIF media profiles for a camera.

**Response (200):**
```json
[
  { "token": "mainStream", "name": "Main Stream", "videoSourceToken": "V_SRC_000" },
  { "token": "subStream", "name": "Sub Stream", "videoSourceToken": "V_SRC_000" }
]
```

### POST `/cameras/:id/media/profiles`

**Auth:** JWT required

Create a new media profile on the camera.

**Request:**
```json
{
  "name": "Custom Profile"
}
```

**Response (201):**
```json
{
  "token": "profile_3",
  "name": "Custom Profile"
}
```

### DELETE `/cameras/:id/media/profiles/:token`

**Auth:** JWT required

Delete a media profile from the camera.

**Response (204):** No content.

### GET `/cameras/:id/media/video-sources`

**Auth:** JWT required

List available video sources on the camera.

**Response (200):**
```json
[
  {
    "token": "V_SRC_000",
    "resolution": { "width": 2560, "height": 1440 },
    "framerate": 25.0
  }
]
```

### GET `/cameras/:id/media/video-encoder/:token`

**Auth:** JWT required

Get the video encoder configuration for a specific profile token.

**Response (200):**
```json
{
  "token": "V_ENC_001",
  "encoding": "H264",
  "resolution": { "width": 1920, "height": 1080 },
  "quality": 4.0,
  "rateControl": { "bitrateLimit": 4096, "encodingInterval": 1 },
  "govLength": 50
}
```

### PUT `/cameras/:id/media/video-encoder/:token`

**Auth:** JWT required

Update the video encoder configuration.

**Request:**
```json
{
  "resolution": { "width": 1920, "height": 1080 },
  "quality": 5.0,
  "rateControl": { "bitrateLimit": 8192 }
}
```

**Response (200):**
```json
{
  "message": "encoder updated"
}
```

### GET `/cameras/:id/media/video-encoder/:token/options`

**Auth:** JWT required

Get available video encoder configuration options (supported resolutions, bitrate ranges, etc.).

**Response (200):**
```json
{
  "resolutions": [
    { "width": 1920, "height": 1080 },
    { "width": 2560, "height": 1440 }
  ],
  "qualityRange": { "min": 1, "max": 6 },
  "bitrateRange": { "min": 256, "max": 16384 }
}
```

---

## Multicast Streaming

### GET `/cameras/:id/multicast`

**Auth:** JWT required

Get the multicast streaming configuration for a camera.

**Response (200):**
```json
{
  "enabled": false,
  "address": "239.0.0.1",
  "port": 5004,
  "ttl": 5
}
```

### PUT `/cameras/:id/multicast`

**Auth:** JWT required

Update the multicast streaming configuration.

**Request:**
```json
{
  "enabled": true,
  "address": "239.0.0.1",
  "port": 5004,
  "ttl": 5
}
```

**Response (200):**
```json
{
  "message": "multicast updated"
}
```

---

## Media2 Configuration

### POST `/cameras/:id/media2/profiles`

**Auth:** JWT required

Create a Media2 profile on the camera.

**Request:**
```json
{
  "name": "Media2 Profile"
}
```

**Response (201):**
```json
{
  "token": "m2_profile_1",
  "name": "Media2 Profile"
}
```

### DELETE `/cameras/:id/media2/profiles/:token`

**Auth:** JWT required

Delete a Media2 profile.

**Response (204):** No content.

### POST `/cameras/:id/media2/profiles/:token/configurations`

**Auth:** JWT required

Add a configuration to a Media2 profile.

**Request:**
```json
{
  "type": "VideoEncoder",
  "token": "V_ENC_001"
}
```

**Response (200):**
```json
{
  "message": "configuration added"
}
```

### DELETE `/cameras/:id/media2/profiles/:token/configurations`

**Auth:** JWT required

Remove a configuration from a Media2 profile.

**Request:**
```json
{
  "type": "VideoEncoder",
  "token": "V_ENC_001"
}
```

**Response (200):**
```json
{
  "message": "configuration removed"
}
```

### GET `/cameras/:id/media2/video-source-configs`

**Auth:** JWT required

List video source configurations (Media2).

**Response (200):**
```json
[
  { "token": "VSC_001", "sourceToken": "V_SRC_000", "name": "Main Source" }
]
```

### PUT `/cameras/:id/media2/video-source-configs/:token`

**Auth:** JWT required

Update a video source configuration.

**Request:**
```json
{
  "bounds": { "x": 0, "y": 0, "width": 1920, "height": 1080 }
}
```

**Response (200):**
```json
{
  "message": "video source config updated"
}
```

### GET `/cameras/:id/media2/video-source-configs/:token/options`

**Auth:** JWT required

Get options for a video source configuration.

**Response (200):**
```json
{
  "maxWidth": 2560,
  "maxHeight": 1440
}
```

### GET `/cameras/:id/media2/audio-source-configs`

**Auth:** JWT required

List audio source configurations (Media2).

**Response (200):**
```json
[
  { "token": "ASC_001", "sourceToken": "A_SRC_000", "name": "Microphone" }
]
```

### PUT `/cameras/:id/media2/audio-source-configs/:token`

**Auth:** JWT required

Update an audio source configuration.

**Response (200):**
```json
{
  "message": "audio source config updated"
}
```

---

## Device Info and Services

### GET `/cameras/:id/device-info`

**Auth:** JWT required

Get ONVIF device information (manufacturer, model, firmware, serial number).

**Response (200):**
```json
{
  "manufacturer": "HIKVISION",
  "model": "DS-2CD2143G2-I",
  "firmwareVersion": "V5.7.16",
  "serialNumber": "DS-2CD2143G2I20210101AAWRG12345678",
  "hardwareId": "88"
}
```

### GET `/cameras/:id/services`

**Auth:** JWT required

Get the list of ONVIF services supported by the camera.

**Response (200):**
```json
[
  { "namespace": "http://www.onvif.org/ver10/device/wsdl", "xaddr": "http://192.168.1.100/onvif/device_service", "version": { "major": 17, "minor": 6 } },
  { "namespace": "http://www.onvif.org/ver10/media/wsdl", "xaddr": "http://192.168.1.100/onvif/Media" }
]
```

---

## Device Management

### GET `/cameras/:id/device/datetime`

**Auth:** JWT required

Get the device date/time configuration.

**Response (200):**
```json
{
  "dateTimeType": "NTP",
  "daylightSavings": true,
  "timeZone": "CST-8",
  "utcDateTime": "2025-06-15T12:30:00Z"
}
```

### PUT `/cameras/:id/device/datetime`

**Auth:** JWT required

Set the device date/time.

**Request:**
```json
{
  "dateTimeType": "Manual",
  "utcDateTime": "2025-06-15T12:30:00Z"
}
```

**Response (200):**
```json
{
  "message": "datetime updated"
}
```

### GET `/cameras/:id/device/hostname`

**Auth:** JWT required

Get the device hostname.

**Response (200):**
```json
{
  "name": "IPCAM-FRONT"
}
```

### PUT `/cameras/:id/device/hostname`

**Auth:** JWT required

Set the device hostname.

**Request:**
```json
{
  "name": "IPCAM-ENTRANCE"
}
```

**Response (200):**
```json
{
  "message": "hostname updated"
}
```

### POST `/cameras/:id/device/reboot`

**Auth:** JWT required

Reboot the camera device.

**Response (200):**
```json
{
  "message": "reboot initiated"
}
```

### GET `/cameras/:id/device/scopes`

**Auth:** JWT required

Get ONVIF scopes from the device.

**Response (200):**
```json
{
  "scopes": [
    "onvif://www.onvif.org/type/video_encoder",
    "onvif://www.onvif.org/name/HIKVISION"
  ]
}
```

### PUT `/cameras/:id/device/scopes`

**Auth:** JWT required

Set (replace) device scopes.

**Request:**
```json
{
  "scopes": ["onvif://www.onvif.org/name/CustomScope"]
}
```

**Response (200):**
```json
{
  "message": "scopes updated"
}
```

### POST `/cameras/:id/device/scopes`

**Auth:** JWT required

Add scopes to the device.

**Request:**
```json
{
  "scopes": ["onvif://www.onvif.org/location/building1"]
}
```

**Response (200):**
```json
{
  "message": "scopes added"
}
```

### DELETE `/cameras/:id/device/scopes`

**Auth:** JWT required

Remove scopes from the device.

**Request:**
```json
{
  "scopes": ["onvif://www.onvif.org/location/building1"]
}
```

**Response (200):**
```json
{
  "message": "scopes removed"
}
```

### GET `/cameras/:id/device/discovery-mode`

**Auth:** JWT required

Get the device's ONVIF discovery mode.

**Response (200):**
```json
{
  "discoveryMode": "Discoverable"
}
```

### PUT `/cameras/:id/device/discovery-mode`

**Auth:** JWT required

Set the device's discovery mode.

**Request:**
```json
{
  "discoveryMode": "NonDiscoverable"
}
```

**Response (200):**
```json
{
  "message": "discovery mode updated"
}
```

### GET `/cameras/:id/device/system-log`

**Auth:** JWT required

Retrieve the device system log.

**Response (200):**
```json
{
  "log": "2025-06-15 12:00:00 System started\n..."
}
```

### GET `/cameras/:id/device/support-info`

**Auth:** JWT required

Get device support information.

**Response (200):**
```json
{
  "info": "Manufacturer support data..."
}
```

### GET `/cameras/:id/device/network/interfaces`

**Auth:** JWT required

List network interfaces on the device.

**Response (200):**
```json
[
  {
    "token": "eth0",
    "enabled": true,
    "ipv4": { "address": "192.168.1.100", "prefixLength": 24, "dhcp": false }
  }
]
```

### PUT `/cameras/:id/device/network/interfaces/:token`

**Auth:** JWT required

Update a network interface configuration on the device.

**Request:**
```json
{
  "ipv4": { "address": "192.168.1.101", "prefixLength": 24, "dhcp": false }
}
```

**Response (200):**
```json
{
  "message": "interface updated"
}
```

### GET `/cameras/:id/device/network/protocols`

**Auth:** JWT required

Get network protocols configuration from the device.

**Response (200):**
```json
[
  { "name": "HTTP", "enabled": true, "port": 80 },
  { "name": "RTSP", "enabled": true, "port": 554 }
]
```

### PUT `/cameras/:id/device/network/protocols`

**Auth:** JWT required

Set network protocols on the device.

**Response (200):**
```json
{
  "message": "protocols updated"
}
```

### GET `/cameras/:id/device/network/dns`

**Auth:** JWT required

Get DNS configuration from the device.

**Response (200):**
```json
{
  "fromDHCP": false,
  "searchDomain": ["local"],
  "dnsManual": [{ "type": "IPv4", "address": "8.8.8.8" }]
}
```

### PUT `/cameras/:id/device/network/dns`

**Auth:** JWT required

Set DNS configuration on the device.

**Response (200):**
```json
{
  "message": "dns updated"
}
```

### GET `/cameras/:id/device/network/ntp`

**Auth:** JWT required

Get NTP configuration from the device.

**Response (200):**
```json
{
  "fromDHCP": false,
  "ntpManual": [{ "type": "IPv4", "address": "pool.ntp.org" }]
}
```

### PUT `/cameras/:id/device/network/ntp`

**Auth:** JWT required

Set NTP configuration on the device.

**Response (200):**
```json
{
  "message": "ntp updated"
}
```

### GET `/cameras/:id/device/network/gateway`

**Auth:** JWT required

Get the default gateway configuration.

**Response (200):**
```json
{
  "ipv4Address": "192.168.1.1"
}
```

### PUT `/cameras/:id/device/network/gateway`

**Auth:** JWT required

Set the default gateway.

**Request:**
```json
{
  "ipv4Address": "192.168.1.1"
}
```

**Response (200):**
```json
{
  "message": "gateway updated"
}
```

### GET `/cameras/:id/device/users`

**Auth:** JWT required

List user accounts on the camera device.

**Response (200):**
```json
[
  { "username": "admin", "level": "Administrator" }
]
```

### POST `/cameras/:id/device/users`

**Auth:** JWT required

Create a user account on the camera device.

**Request:**
```json
{
  "username": "operator",
  "password": "oppass123",
  "level": "Operator"
}
```

**Response (201):**
```json
{
  "message": "user created"
}
```

### PUT `/cameras/:id/device/users/:username`

**Auth:** JWT required

Update a user account on the camera device.

**Request:**
```json
{
  "password": "newpass",
  "level": "Administrator"
}
```

**Response (200):**
```json
{
  "message": "user updated"
}
```

### DELETE `/cameras/:id/device/users/:username`

**Auth:** JWT required

Delete a user account from the camera device.

**Response (204):** No content.

---

## Relay Outputs

### GET `/cameras/:id/relay-outputs`

**Auth:** JWT required

List relay outputs available on the camera.

**Response (200):**
```json
[
  { "token": "relay1", "properties": { "mode": "Bistable", "idleState": "open" } }
]
```

### POST `/cameras/:id/relay-outputs/:token/state`

**Auth:** JWT required

Set the state of a relay output (trigger alarm output, gate, etc.).

**Request:**
```json
{
  "logicalState": "active"
}
```

**Response (200):**
```json
{
  "message": "relay state set"
}
```

---

## Audio

### GET `/cameras/:id/audio/capabilities`

**Auth:** JWT required

Get audio capabilities of the camera.

**Response (200):**
```json
{
  "inputSupported": true,
  "outputSupported": true,
  "encodings": ["G711", "AAC"]
}
```

### GET `/cameras/:id/audio/sources`

**Auth:** JWT required

List available audio sources on the camera.

**Response (200):**
```json
[
  { "token": "A_SRC_000", "channels": 1 }
]
```

### GET `/cameras/:id/audio/source-configs`

**Auth:** JWT required

List audio source configurations.

**Response (200):**
```json
[
  { "token": "ASC_001", "name": "Microphone", "sourceToken": "A_SRC_000", "useCount": 1 }
]
```

### GET `/cameras/:id/audio/source-configs/compatible/:profileToken`

**Auth:** JWT required

List audio source configurations compatible with a specific profile.

**Response (200):**
```json
[
  { "token": "ASC_001", "name": "Microphone" }
]
```

### GET `/cameras/:id/audio/source-configs/:token`

**Auth:** JWT required

Get a specific audio source configuration.

**Response (200):**
```json
{
  "token": "ASC_001",
  "name": "Microphone",
  "sourceToken": "A_SRC_000"
}
```

### PUT `/cameras/:id/audio/source-configs/:token`

**Auth:** JWT required

Update an audio source configuration.

**Response (200):**
```json
{
  "message": "audio source config updated"
}
```

### GET `/cameras/:id/audio/source-configs/:token/options`

**Auth:** JWT required

Get available options for an audio source configuration.

**Response (200):**
```json
{
  "inputTokens": ["A_SRC_000"],
  "inputGainRange": { "min": 0, "max": 100 }
}
```

### POST `/cameras/:id/audio/source-configs/add`

**Auth:** JWT required

Add an audio source configuration to a profile.

**Request:**
```json
{
  "profileToken": "mainStream",
  "configToken": "ASC_001"
}
```

**Response (200):**
```json
{
  "message": "audio source added to profile"
}
```

### POST `/cameras/:id/audio/source-configs/remove`

**Auth:** JWT required

Remove an audio source configuration from a profile.

**Request:**
```json
{
  "profileToken": "mainStream",
  "configToken": "ASC_001"
}
```

**Response (200):**
```json
{
  "message": "audio source removed from profile"
}
```

---

## Backchannel Audio

### GET `/cameras/:id/audio/backchannel/ws`

**Auth:** JWT required

Open a WebSocket connection for two-way audio (talk-back) with the camera.

**Protocol:** WebSocket upgrade. Binary frames contain audio data (G.711/PCM).

### GET `/cameras/:id/audio/backchannel/info`

**Auth:** JWT required

Get backchannel audio capabilities and status.

**Response (200):**
```json
{
  "supported": true,
  "encoding": "G711",
  "sampleRate": 8000,
  "bitrate": 64
}
```

### GET `/cameras/:id/audio/outputs`

**Auth:** JWT required

List audio outputs on the camera.

**Response (200):**
```json
[
  { "token": "A_OUT_000", "name": "Speaker" }
]
```

### GET `/cameras/:id/audio/output-configs`

**Auth:** JWT required

List audio output configurations.

**Response (200):**
```json
[
  { "token": "AOC_001", "name": "Speaker Output", "outputToken": "A_OUT_000" }
]
```

### PUT `/cameras/:id/audio/output-configs/:token`

**Auth:** JWT required

Update an audio output configuration.

**Response (200):**
```json
{
  "message": "audio output config updated"
}
```

### GET `/cameras/:id/audio/decoder-configs`

**Auth:** JWT required

List audio decoder configurations.

**Response (200):**
```json
[
  { "token": "ADC_001", "name": "G711 Decoder" }
]
```

### PUT `/cameras/:id/audio/decoder-configs/:token`

**Auth:** JWT required

Update an audio decoder configuration.

**Response (200):**
```json
{
  "message": "decoder config updated"
}
```

### GET `/cameras/:id/audio/decoder-options/:token`

**Auth:** JWT required

Get options for an audio decoder configuration.

**Response (200):**
```json
{
  "encodings": ["G711", "AAC", "PCM"]
}
```

---

## Edge Recordings

### GET `/cameras/:id/edge-recordings`

**Auth:** JWT required

List recordings stored on the camera's SD card (ONVIF Profile G).

**Query parameters:** `start`, `end` (ISO 8601 timestamps)

**Response (200):**
```json
[
  {
    "recordingToken": "rec001",
    "trackToken": "track001",
    "startTime": "2025-06-15T08:00:00Z",
    "endTime": "2025-06-15T08:30:00Z"
  }
]
```

### GET `/cameras/:id/edge-recordings/playback`

**Auth:** JWT required

Get a playback URI for an edge recording.

**Query parameters:** `recordingToken`, `start`, `end`

**Response (200):**
```json
{
  "uri": "rtsp://192.168.1.100:554/playback?token=rec001"
}
```

### POST `/cameras/:id/edge-recordings/replay-session`

**Auth:** JWT required

Start a replay session for edge playback.

**Request:**
```json
{
  "recordingToken": "rec001"
}
```

**Response (200):**
```json
{
  "sessionId": "session-abc",
  "uri": "rtsp://192.168.1.100:554/replay?session=session-abc"
}
```

### POST `/cameras/:id/edge-recordings/import`

**Auth:** JWT required

Import a recording from the camera's SD card to the NVR.

**Request:**
```json
{
  "recordingToken": "rec001",
  "start": "2025-06-15T08:00:00Z",
  "end": "2025-06-15T08:30:00Z"
}
```

**Response (202):**
```json
{
  "message": "import started",
  "jobId": "import-xyz"
}
```

---

## Replay Control

### POST `/cameras/:id/replay/session`

**Auth:** JWT required

Start a Profile G RTSP replay session.

**Request:**
```json
{
  "recordingToken": "rec001"
}
```

**Response (200):**
```json
{
  "sessionId": "session-123",
  "uri": "rtsp://..."
}
```

### GET `/cameras/:id/replay/uri`

**Auth:** JWT required

Get the replay URI for a recording.

**Query parameters:** `recordingToken`

**Response (200):**
```json
{
  "uri": "rtsp://192.168.1.100:554/replay"
}
```

### GET `/cameras/:id/replay/capabilities`

**Auth:** JWT required

Get replay service capabilities.

**Response (200):**
```json
{
  "rtp_rtsp_tcp": true,
  "reversePlayback": false,
  "speedControl": true
}
```

---

## Recording Control

### GET `/cameras/:id/recording-control/config`

**Auth:** JWT required

Get recording configuration on the device.

**Response (200):**
```json
{
  "maxRecordings": 20,
  "maxRecordingJobs": 10
}
```

### POST `/cameras/:id/recording-control/recordings`

**Auth:** JWT required

Create a recording on the device.

**Request:**
```json
{
  "sourceToken": "mainStream"
}
```

**Response (201):**
```json
{
  "recordingToken": "rec_new_001"
}
```

### DELETE `/cameras/:id/recording-control/recordings/:token`

**Auth:** JWT required

Delete a recording from the device.

**Response (204):** No content.

### POST `/cameras/:id/recording-control/jobs`

**Auth:** JWT required

Create a recording job on the device.

**Request:**
```json
{
  "recordingToken": "rec_new_001",
  "mode": "Active"
}
```

**Response (201):**
```json
{
  "jobToken": "job_001"
}
```

### DELETE `/cameras/:id/recording-control/jobs/:token`

**Auth:** JWT required

Delete a recording job from the device.

**Response (204):** No content.

### GET `/cameras/:id/recording-control/jobs/:token/state`

**Auth:** JWT required

Get the state of a recording job.

**Response (200):**
```json
{
  "state": "Active",
  "recordingToken": "rec_new_001"
}
```

---

## Track Management

### POST `/cameras/:id/recording-control/recordings/:token/tracks`

**Auth:** JWT required

Create a track within a recording on the device.

**Request:**
```json
{
  "trackType": "Video",
  "description": "Main video track"
}
```

**Response (201):**
```json
{
  "trackToken": "track_001"
}
```

### DELETE `/cameras/:id/recording-control/recordings/:token/tracks/:trackToken`

**Auth:** JWT required

Delete a track from a recording on the device.

**Response (204):** No content.

### GET `/cameras/:id/recording-control/tracks/:trackToken/config`

**Auth:** JWT required

Get configuration for a specific track.

**Response (200):**
```json
{
  "trackToken": "track_001",
  "trackType": "Video",
  "description": "Main video track"
}
```

---

## Camera AI Configuration

### PUT `/cameras/:id/ai`

**Auth:** JWT required

Update AI detection settings for a camera.

**Request:**
```json
{
  "enabled": true,
  "detectPeople": true,
  "detectVehicles": true,
  "confidenceThreshold": 0.6,
  "interval": 2
}
```

**Response (200):**
```json
{
  "message": "AI config updated"
}
```

### PUT `/cameras/:id/audio-transcode`

**Auth:** JWT required

Update audio transcoding settings for a camera.

**Request:**
```json
{
  "enabled": true,
  "codec": "aac",
  "bitrate": 128
}
```

**Response (200):**
```json
{
  "message": "audio transcode updated"
}
```

---

## Detections

### GET `/cameras/:id/detections/latest`

**Auth:** JWT required

Get the most recent AI detections for a camera (for live overlay).

**Response (200):**
```json
[
  {
    "label": "person",
    "confidence": 0.92,
    "bbox": { "x": 100, "y": 200, "width": 50, "height": 120 },
    "timestamp": "2025-06-15T12:30:05Z"
  }
]
```

### GET `/cameras/:id/detections/stream`

**Auth:** JWT required

Server-Sent Events (SSE) stream of real-time detections.

**Response:** `text/event-stream`

```
data: {"label":"person","confidence":0.91,"bbox":{"x":100,"y":200,"width":50,"height":120}}

data: {"label":"car","confidence":0.87,"bbox":{"x":300,"y":150,"width":200,"height":100}}
```

### GET `/cameras/:id/detections`

**Auth:** JWT required

Query historical detections for a camera.

**Query parameters:** `start`, `end`, `label`, `minConfidence`, `limit`, `offset`

**Response (200):**
```json
{
  "detections": [
    {
      "id": 1,
      "label": "person",
      "confidence": 0.92,
      "timestamp": "2025-06-15T12:30:05Z"
    }
  ],
  "total": 150
}
```

---

## Analytics

### GET `/cameras/:id/analytics/rules`

**Auth:** JWT required

List analytics rules configured on the camera.

**Response (200):**
```json
[
  { "name": "LineCross1", "type": "LineDetector", "enabled": true }
]
```

### POST `/cameras/:id/analytics/rules`

**Auth:** JWT required

Create an analytics rule on the camera.

**Request:**
```json
{
  "name": "LineCross1",
  "type": "LineDetector",
  "enabled": true,
  "parameters": {}
}
```

**Response (201):**
```json
{
  "message": "rule created"
}
```

### PUT `/cameras/:id/analytics/rules/:name`

**Auth:** JWT required

Update an analytics rule.

**Response (200):**
```json
{
  "message": "rule updated"
}
```

### DELETE `/cameras/:id/analytics/rules/:name`

**Auth:** JWT required

Delete an analytics rule.

**Response (204):** No content.

### GET `/cameras/:id/analytics/modules`

**Auth:** JWT required

List analytics modules available on the camera.

**Response (200):**
```json
[
  { "name": "MotionDetector", "type": "tt:MotionDetection", "maxInstances": 16 }
]
```

---

## Metadata Configuration

### GET `/cameras/:id/metadata/configurations`

**Auth:** JWT required

List metadata configurations on the camera (Profile T).

**Response (200):**
```json
[
  { "token": "META_001", "name": "MetadataConfig", "analyticsEnabled": true, "eventsEnabled": true }
]
```

### GET `/cameras/:id/metadata/configurations/:token`

**Auth:** JWT required

Get a specific metadata configuration.

**Response (200):**
```json
{
  "token": "META_001",
  "name": "MetadataConfig",
  "analyticsEnabled": true,
  "eventsEnabled": true,
  "ptzStatusEnabled": false
}
```

### PUT `/cameras/:id/metadata/configurations/:token`

**Auth:** JWT required

Update a metadata configuration.

**Request:**
```json
{
  "analyticsEnabled": true,
  "eventsEnabled": true,
  "ptzStatusEnabled": true
}
```

**Response (200):**
```json
{
  "message": "metadata config updated"
}
```

### POST `/cameras/:id/metadata/profile`

**Auth:** JWT required

Add a metadata configuration to a profile.

**Request:**
```json
{
  "profileToken": "mainStream",
  "configToken": "META_001"
}
```

**Response (200):**
```json
{
  "message": "metadata added to profile"
}
```

### DELETE `/cameras/:id/metadata/profile/:profileToken`

**Auth:** JWT required

Remove a metadata configuration from a profile.

**Response (200):**
```json
{
  "message": "metadata removed from profile"
}
```

---

## OSD Management

### GET `/cameras/:id/osd`

**Auth:** JWT required

List all on-screen display (OSD) overlays on the camera.

**Response (200):**
```json
[
  { "token": "OSD_001", "type": "Text", "position": { "type": "UpperLeft" }, "textString": "Camera 1" }
]
```

### GET `/cameras/:id/osd/options`

**Auth:** JWT required

Get available OSD options (fonts, positions, etc.).

**Response (200):**
```json
{
  "maxOSDs": 8,
  "positions": ["UpperLeft", "UpperRight", "LowerLeft", "LowerRight", "Custom"],
  "fontSizeRange": { "min": 16, "max": 64 }
}
```

### POST `/cameras/:id/osd`

**Auth:** JWT required

Create an OSD overlay.

**Request:**
```json
{
  "type": "Text",
  "position": { "type": "UpperLeft" },
  "textString": "Front Door"
}
```

**Response (201):**
```json
{
  "token": "OSD_002"
}
```

### PUT `/cameras/:id/osd/:token`

**Auth:** JWT required

Update an OSD overlay.

**Response (200):**
```json
{
  "message": "OSD updated"
}
```

### DELETE `/cameras/:id/osd/:token`

**Auth:** JWT required

Delete an OSD overlay.

**Response (204):** No content.

---

## Recordings

### GET `/recordings`

**Auth:** JWT required

Query NVR recordings. Supports filtering and pagination.

**Query parameters:** `cameraId`, `start`, `end`, `limit`, `offset`, `hasMotion`

**Response (200):**
```json
{
  "recordings": [
    {
      "id": "rec-abc123",
      "cameraId": "cam-abc123",
      "path": "/recordings/front-door/2025-06-15_12-00-00.mp4",
      "startTime": "2025-06-15T12:00:00Z",
      "endTime": "2025-06-15T12:30:00Z",
      "sizeBytes": 524288000
    }
  ],
  "total": 42
}
```

### GET `/recordings/:id/download`

**Auth:** JWT required

Download a recording file.

**Response (200):** Binary `video/mp4` content with `Content-Disposition` header.

### POST `/recordings/export`

**Auth:** JWT required

Export a time range of recordings for a camera.

**Request:**
```json
{
  "cameraId": "cam-abc123",
  "start": "2025-06-15T08:00:00Z",
  "end": "2025-06-15T09:00:00Z",
  "format": "mp4"
}
```

**Response (202):**
```json
{
  "message": "export started",
  "jobId": "export-xyz"
}
```

### DELETE `/recordings/cleanup`

**Auth:** JWT required

Trigger manual cleanup of expired recordings.

**Response (200):**
```json
{
  "deleted": 15,
  "freedBytes": 7864320000
}
```

### GET `/timeline`

**Auth:** JWT required

Get a timeline of recordings for a single camera.

**Query parameters:** `cameraId`, `start`, `end`, `bucketMinutes`

**Response (200):**
```json
[
  {
    "start": "2025-06-15T08:00:00Z",
    "end": "2025-06-15T08:05:00Z",
    "hasMotion": true,
    "sizeBytes": 52428800
  }
]
```

### GET `/timeline/multi`

**Auth:** JWT required

Get timelines for multiple cameras in one request.

**Query parameters:** `cameraIds` (comma-separated), `start`, `end`

**Response (200):**
```json
{
  "cam-abc123": [ { "start": "...", "end": "...", "hasMotion": false } ],
  "cam-def456": [ { "start": "...", "end": "...", "hasMotion": true } ]
}
```

### GET `/timeline/intensity`

**Auth:** JWT required

Get a motion intensity heatmap over a time range.

**Query parameters:** `cameraId`, `start`, `end`, `buckets`

**Response (200):**
```json
[
  { "start": "2025-06-15T08:00:00Z", "end": "2025-06-15T08:05:00Z", "intensity": 0.75 }
]
```

---

## Bulk Export

### POST `/exports/bulk`

**Auth:** JWT required

Create a bulk export job spanning multiple cameras and time ranges.

**Request:**
```json
{
  "items": [
    { "cameraId": "cam-abc123", "start": "2025-06-15T08:00:00Z", "end": "2025-06-15T09:00:00Z" }
  ],
  "format": "mp4"
}
```

**Response (201):**
```json
{
  "id": "bulk-export-001",
  "status": "pending"
}
```

### GET `/exports/bulk`

**Auth:** JWT required

List bulk export jobs.

**Response (200):**
```json
[
  { "id": "bulk-export-001", "status": "completed", "createdAt": "2025-06-15T10:00:00Z" }
]
```

### GET `/exports/bulk/:id`

**Auth:** JWT required

Get the status of a bulk export job.

**Response (200):**
```json
{
  "id": "bulk-export-001",
  "status": "completed",
  "progress": 100,
  "totalItems": 3,
  "completedItems": 3
}
```

### GET `/exports/bulk/:id/download`

**Auth:** JWT required

Download the completed bulk export archive.

**Response (200):** Binary `application/zip` content.

### DELETE `/exports/bulk/:id`

**Auth:** JWT required

Delete a bulk export job and its files.

**Response (204):** No content.

---

## Recording Integrity

### GET `/recordings/integrity`

**Auth:** JWT required

Get a summary of recording integrity status.

**Response (200):**
```json
{
  "total": 1000,
  "healthy": 990,
  "corrupted": 5,
  "quarantined": 5
}
```

### POST `/recordings/verify`

**Auth:** JWT required

Trigger an integrity verification scan.

**Response (202):**
```json
{
  "message": "verification started"
}
```

### POST `/recordings/:id/quarantine`

**Auth:** JWT required

Move a corrupted recording to quarantine.

**Response (200):**
```json
{
  "message": "recording quarantined"
}
```

### POST `/recordings/:id/unquarantine`

**Auth:** JWT required

Restore a quarantined recording.

**Response (200):**
```json
{
  "message": "recording restored"
}
```

---

## Recording Statistics

### GET `/recordings/stats`

**Auth:** JWT required

Get aggregate recording statistics.

**Response (200):**
```json
{
  "totalRecordings": 5000,
  "totalSizeBytes": 1099511627776,
  "oldestRecording": "2025-01-01T00:00:00Z",
  "newestRecording": "2025-06-15T12:30:00Z",
  "byCameraId": {
    "cam-abc123": { "count": 1200, "sizeBytes": 268435456000 }
  }
}
```

### GET `/recordings/stats/:camera_id/gaps`

**Auth:** JWT required

Get recording gaps for a specific camera.

**Response (200):**
```json
[
  {
    "start": "2025-06-15T03:00:00Z",
    "end": "2025-06-15T03:15:00Z",
    "durationSec": 900
  }
]
```

---

## Recording Health

### GET `/recordings/health`

**Auth:** JWT required

Get recording health status per camera.

**Response (200):**
```json
[
  {
    "cameraId": "cam-abc123",
    "status": "healthy",
    "lastRecording": "2025-06-15T12:30:00Z",
    "gapCount": 0
  }
]
```

---

## Motion and Events

### GET `/cameras/:id/motion-events`

**Auth:** JWT required

List motion events for a camera.

**Query parameters:** `start`, `end`, `limit`, `offset`

**Response (200):**
```json
[
  {
    "id": 1,
    "cameraId": "cam-abc123",
    "startTime": "2025-06-15T12:00:00Z",
    "endTime": "2025-06-15T12:00:30Z"
  }
]
```

### GET `/cameras/:id/events`

**Auth:** JWT required

List all events (motion, AI detections, etc.) for a camera.

**Query parameters:** `start`, `end`, `type`, `limit`, `offset`

**Response (200):**
```json
[
  {
    "id": 1,
    "type": "motion",
    "cameraId": "cam-abc123",
    "timestamp": "2025-06-15T12:00:00Z",
    "data": {}
  }
]
```

### DELETE `/cameras/:id/events`

**Auth:** JWT required

Purge events for a camera.

**Query parameters:** `before` (ISO 8601 timestamp, optional)

**Response (200):**
```json
{
  "deleted": 500
}
```

---

## Saved Clips

### GET `/saved-clips`

**Auth:** JWT required

List saved video clips.

**Response (200):**
```json
[
  {
    "id": "clip-001",
    "cameraId": "cam-abc123",
    "name": "Incident 2025-06-15",
    "startTime": "2025-06-15T12:00:00Z",
    "endTime": "2025-06-15T12:05:00Z",
    "createdAt": "2025-06-15T12:10:00Z"
  }
]
```

### POST `/saved-clips`

**Auth:** JWT required

Save a video clip from recordings.

**Request:**
```json
{
  "cameraId": "cam-abc123",
  "name": "Incident clip",
  "startTime": "2025-06-15T12:00:00Z",
  "endTime": "2025-06-15T12:05:00Z"
}
```

**Response (201):**
```json
{
  "id": "clip-002",
  "name": "Incident clip"
}
```

### DELETE `/saved-clips/:id`

**Auth:** JWT required

Delete a saved clip.

**Response (204):** No content.

---

## Bookmarks

### GET `/bookmarks`

**Auth:** JWT required

List all bookmarks.

**Query parameters:** `cameraId`, `limit`, `offset`

**Response (200):**
```json
[
  {
    "id": "bm-001",
    "cameraId": "cam-abc123",
    "timestamp": "2025-06-15T12:30:00Z",
    "title": "Suspicious activity",
    "notes": "Person loitering near entrance",
    "createdBy": "admin"
  }
]
```

### GET `/bookmarks/search`

**Auth:** JWT required

Search bookmarks by title or notes.

**Query parameters:** `q`, `limit`, `offset`

**Response (200):**
```json
[
  { "id": "bm-001", "title": "Suspicious activity" }
]
```

### GET `/bookmarks/mine`

**Auth:** JWT required

List bookmarks created by the authenticated user.

**Response (200):**
```json
[
  { "id": "bm-001", "title": "Suspicious activity" }
]
```

### GET `/bookmarks/:id`

**Auth:** JWT required

Get a single bookmark.

**Response (200):**
```json
{
  "id": "bm-001",
  "cameraId": "cam-abc123",
  "timestamp": "2025-06-15T12:30:00Z",
  "title": "Suspicious activity",
  "notes": "Person loitering near entrance"
}
```

### POST `/bookmarks`

**Auth:** JWT required

Create a bookmark.

**Request:**
```json
{
  "cameraId": "cam-abc123",
  "timestamp": "2025-06-15T12:30:00Z",
  "title": "Suspicious activity",
  "notes": "Person loitering near entrance"
}
```

**Response (201):**
```json
{
  "id": "bm-002"
}
```

### PUT `/bookmarks/:id`

**Auth:** JWT required

Update a bookmark.

**Request:**
```json
{
  "title": "Updated title",
  "notes": "Updated notes"
}
```

**Response (200):**
```json
{
  "message": "bookmark updated"
}
```

### DELETE `/bookmarks/:id`

**Auth:** JWT required

Delete a bookmark.

**Response (204):** No content.

---

## Screenshots

### POST `/cameras/:id/screenshot`

**Auth:** JWT required

Capture a screenshot from a camera's live stream.

**Response (201):**
```json
{
  "id": "ss-001",
  "cameraId": "cam-abc123",
  "filename": "cam-abc123_2025-06-15T12-30-00.jpg",
  "createdAt": "2025-06-15T12:30:00Z"
}
```

### GET `/screenshots`

**Auth:** JWT required

List all screenshots.

**Query parameters:** `cameraId`, `limit`, `offset`

**Response (200):**
```json
[
  {
    "id": "ss-001",
    "cameraId": "cam-abc123",
    "filename": "cam-abc123_2025-06-15T12-30-00.jpg",
    "createdAt": "2025-06-15T12:30:00Z"
  }
]
```

### GET `/screenshots/:id/download`

**Auth:** JWT required

Download a screenshot image.

**Response (200):** Binary `image/jpeg` content.

### DELETE `/screenshots/:id`

**Auth:** JWT required

Delete a screenshot.

**Response (204):** No content.

---

## Timeline Thumbnails

### GET `/cameras/:id/thumbnails`

**Auth:** JWT required

List available timeline thumbnails for a camera.

**Query parameters:** `start`, `end`

**Response (200):**
```json
[
  { "filename": "thumb_2025-06-15T12-00-00.jpg", "timestamp": "2025-06-15T12:00:00Z" }
]
```

### GET `/cameras/:id/thumbnails/:filename`

**Auth:** JWT required

Serve a specific thumbnail image.

**Response (200):** Binary `image/jpeg` content.

---

## Camera Streams

### GET `/cameras/:id/streams`

**Auth:** JWT required

List streams for a camera (main, sub, etc.).

**Response (200):**
```json
[
  {
    "id": "stream-001",
    "cameraId": "cam-abc123",
    "role": "main",
    "rtspUrl": "rtsp://192.168.1.100:554/Streaming/Channels/101",
    "retentionDays": 30
  }
]
```

### POST `/cameras/:id/streams`

**Auth:** JWT required

Create a new stream for a camera.

**Request:**
```json
{
  "role": "sub",
  "rtspUrl": "rtsp://192.168.1.100:554/Streaming/Channels/102"
}
```

**Response (201):**
```json
{
  "id": "stream-002",
  "role": "sub"
}
```

### PUT `/streams/:id`

**Auth:** JWT required

Update a stream.

**Request:**
```json
{
  "rtspUrl": "rtsp://192.168.1.100:554/Streaming/Channels/103"
}
```

**Response (200):**
```json
{
  "message": "stream updated"
}
```

### PUT `/streams/:id/roles`

**Auth:** JWT required

Update the role assignments for a stream.

**Request:**
```json
{
  "role": "main"
}
```

**Response (200):**
```json
{
  "message": "roles updated"
}
```

### DELETE `/streams/:id`

**Auth:** JWT required

Delete a stream.

**Response (204):** No content.

### PUT `/streams/:id/retention`

**Auth:** JWT required

Update retention settings for a specific stream.

**Request:**
```json
{
  "retentionDays": 14
}
```

**Response (200):**
```json
{
  "message": "retention updated"
}
```

### GET `/cameras/:id/stream-storage`

**Auth:** JWT required

Get storage usage breakdown by stream for a camera.

**Response (200):**
```json
[
  { "streamId": "stream-001", "role": "main", "sizeBytes": 107374182400 },
  { "streamId": "stream-002", "role": "sub", "sizeBytes": 21474836480 }
]
```

---

## Recording Rules

### GET `/cameras/:id/recording-rules`

**Auth:** JWT required

List recording rules for a camera.

**Response (200):**
```json
[
  {
    "id": "rule-001",
    "cameraId": "cam-abc123",
    "type": "continuous",
    "schedule": "always",
    "enabled": true
  }
]
```

### POST `/cameras/:id/recording-rules`

**Auth:** JWT required

Create a recording rule.

**Request:**
```json
{
  "type": "motion",
  "schedule": "business_hours",
  "enabled": true
}
```

**Response (201):**
```json
{
  "id": "rule-002"
}
```

### PUT `/recording-rules/:id`

**Auth:** JWT required

Update a recording rule.

**Request:**
```json
{
  "enabled": false
}
```

**Response (200):**
```json
{
  "message": "rule updated"
}
```

### DELETE `/recording-rules/:id`

**Auth:** JWT required

Delete a recording rule.

**Response (204):** No content.

### GET `/cameras/:id/recording-status`

**Auth:** JWT required

Get the current recording status for a camera (active rules, recording state).

**Response (200):**
```json
{
  "recording": true,
  "activeRules": ["rule-001"],
  "trigger": "continuous"
}
```

---

## Schedule Templates

### GET `/schedule-templates`

**Auth:** JWT required

List schedule templates.

**Response (200):**
```json
[
  {
    "id": "tpl-001",
    "name": "Business Hours",
    "schedule": { "mon": ["08:00-18:00"], "tue": ["08:00-18:00"] }
  }
]
```

### POST `/schedule-templates`

**Auth:** JWT required

Create a schedule template.

**Request:**
```json
{
  "name": "Night Shift",
  "schedule": { "mon": ["22:00-06:00"], "tue": ["22:00-06:00"] }
}
```

**Response (201):**
```json
{
  "id": "tpl-002"
}
```

### PUT `/schedule-templates/:id`

**Auth:** JWT required

Update a schedule template.

**Response (200):**
```json
{
  "message": "template updated"
}
```

### DELETE `/schedule-templates/:id`

**Auth:** JWT required

Delete a schedule template.

**Response (204):** No content.

---

## Stream Schedule Assignment

### PUT `/cameras/:id/stream-schedule`

**Auth:** JWT required

Assign a recording schedule to a camera's stream.

**Request:**
```json
{
  "streamId": "stream-001",
  "templateId": "tpl-001"
}
```

**Response (200):**
```json
{
  "message": "schedule assigned"
}
```

---

## Sessions

### GET `/sessions`

**Auth:** JWT required

List active sessions for the current user.

**Response (200):**
```json
[
  {
    "id": "sess-001",
    "userId": "user-001",
    "ipAddress": "192.168.1.50",
    "userAgent": "Mozilla/5.0...",
    "createdAt": "2025-06-15T08:00:00Z",
    "expiresAt": "2025-06-15T20:00:00Z"
  }
]
```

### DELETE `/sessions/:id`

**Auth:** JWT required

Revoke a specific session.

**Response (204):** No content.

### GET `/sessions/timeout`

**Auth:** JWT required

Get the session timeout configuration.

**Response (200):**
```json
{
  "timeoutMinutes": 720
}
```

### PUT `/sessions/timeout`

**Auth:** JWT required

Set the session timeout.

**Request:**
```json
{
  "timeoutMinutes": 480
}
```

**Response (200):**
```json
{
  "message": "timeout updated"
}
```

---

## Users

### GET `/users`

**Auth:** JWT required

List all NVR users.

**Response (200):**
```json
[
  {
    "id": "user-001",
    "username": "admin",
    "role": "admin",
    "createdAt": "2025-01-01T00:00:00Z"
  }
]
```

### POST `/users`

**Auth:** JWT required

Create a new user.

**Request:**
```json
{
  "username": "operator1",
  "password": "securepass",
  "role": "operator"
}
```

**Response (201):**
```json
{
  "id": "user-002",
  "username": "operator1",
  "role": "operator"
}
```

### GET `/users/:id`

**Auth:** JWT required

Get a user by ID.

**Response (200):**
```json
{
  "id": "user-001",
  "username": "admin",
  "role": "admin"
}
```

### PUT `/users/:id`

**Auth:** JWT required

Update a user.

**Request:**
```json
{
  "role": "viewer"
}
```

**Response (200):**
```json
{
  "message": "user updated"
}
```

### DELETE `/users/:id`

**Auth:** JWT required

Delete a user.

**Response (204):** No content.

### DELETE `/users/:id/sessions`

**Auth:** JWT required

Revoke all sessions for a specific user.

**Response (204):** No content.

---

## System

### GET `/system/health`

**Auth:** None

Health check endpoint.

**Response (200):**
```json
{
  "status": "ok"
}
```

### GET `/system/info`

**Auth:** JWT required

Get system information (version, uptime, OS details).

**Response (200):**
```json
{
  "version": "1.5.0",
  "uptime": "72h15m",
  "os": "linux",
  "arch": "amd64",
  "goVersion": "go1.22.0"
}
```

### GET `/system/storage`

**Auth:** JWT required

Get storage usage information.

**Response (200):**
```json
{
  "totalBytes": 1099511627776,
  "usedBytes": 549755813888,
  "freeBytes": 549755813888,
  "recordingsBytes": 524288000000,
  "usagePercent": 50.0
}
```

### GET `/system/metrics`

**Auth:** JWT required

Get system performance metrics from the ring-buffer collector.

**Response (200):**
```json
{
  "cpuPercent": 12.5,
  "memoryUsedBytes": 2147483648,
  "memoryTotalBytes": 8589934592,
  "diskReadBytesPerSec": 52428800,
  "diskWriteBytesPerSec": 26214400,
  "activeCameras": 8,
  "activeRecordings": 6
}
```

### GET `/system/disk-io`

**Auth:** JWT required

Get disk I/O statistics.

**Response (200):**
```json
{
  "readBytesPerSec": 52428800,
  "writeBytesPerSec": 26214400,
  "iops": 150,
  "thresholds": { "warningPercent": 80, "criticalPercent": 95 }
}
```

### PUT `/system/disk-io/thresholds`

**Auth:** JWT required

Update disk I/O warning thresholds.

**Request:**
```json
{
  "warningPercent": 75,
  "criticalPercent": 90
}
```

**Response (200):**
```json
{
  "message": "thresholds updated"
}
```

### GET `/system/db/health`

**Auth:** JWT required

Get database health status.

**Response (200):**
```json
{
  "status": "healthy",
  "sizeBytes": 104857600,
  "tables": 25,
  "integrityCheck": "ok"
}
```

### GET `/system/config`

**Auth:** JWT required

Get a summary of the current system configuration.

**Response (200):**
```json
{
  "nvr": true,
  "api": true,
  "playback": true,
  "logLevel": "debug",
  "recordingsPath": "/recordings"
}
```

### GET `/system/config/export`

**Auth:** JWT required

Export the full system configuration as a downloadable file.

**Response (200):** Binary `application/json` or `application/x-yaml` content.

### POST `/system/config/import`

**Auth:** JWT required

Import a system configuration file.

**Request:** `multipart/form-data` with a config file.

**Response (200):**
```json
{
  "message": "configuration imported",
  "restartRequired": true
}
```

---

## Alerts and SMTP

### GET `/system/smtp/config`

**Auth:** JWT required

Get the current SMTP configuration for email alerts.

**Response (200):**
```json
{
  "host": "smtp.example.com",
  "port": 587,
  "username": "alerts@example.com",
  "encryption": "STARTTLS",
  "fromAddress": "alerts@example.com"
}
```

### POST `/system/smtp/config`

**Auth:** JWT required

Update the SMTP configuration.

**Request:**
```json
{
  "host": "smtp.example.com",
  "port": 587,
  "username": "alerts@example.com",
  "password": "smtp-password",
  "encryption": "STARTTLS",
  "fromAddress": "alerts@example.com"
}
```

**Response (200):**
```json
{
  "message": "SMTP config updated"
}
```

### POST `/system/smtp/test`

**Auth:** JWT required

Send a test email to verify SMTP configuration.

**Request:**
```json
{
  "to": "admin@example.com"
}
```

**Response (200):**
```json
{
  "message": "test email sent"
}
```

### GET `/alert-rules`

**Auth:** JWT required

List alert rules.

**Response (200):**
```json
[
  {
    "id": "ar-001",
    "name": "Camera Offline",
    "type": "camera_offline",
    "enabled": true,
    "recipients": ["admin@example.com"],
    "cooldownMinutes": 30
  }
]
```

### POST `/alert-rules`

**Auth:** JWT required

Create an alert rule.

**Request:**
```json
{
  "name": "Camera Offline Alert",
  "type": "camera_offline",
  "enabled": true,
  "recipients": ["admin@example.com"],
  "cooldownMinutes": 30
}
```

**Response (201):**
```json
{
  "id": "ar-002"
}
```

### PUT `/alert-rules/:id`

**Auth:** JWT required

Update an alert rule.

**Response (200):**
```json
{
  "message": "alert rule updated"
}
```

### DELETE `/alert-rules/:id`

**Auth:** JWT required

Delete an alert rule.

**Response (204):** No content.

### GET `/alerts`

**Auth:** JWT required

List triggered alerts.

**Query parameters:** `acknowledged`, `limit`, `offset`

**Response (200):**
```json
[
  {
    "id": "alert-001",
    "ruleId": "ar-001",
    "type": "camera_offline",
    "cameraId": "cam-abc123",
    "message": "Camera 'Front Door' went offline",
    "acknowledged": false,
    "triggeredAt": "2025-06-15T12:00:00Z"
  }
]
```

### POST `/alerts/:id/acknowledge`

**Auth:** JWT required

Acknowledge an alert.

**Response (200):**
```json
{
  "message": "alert acknowledged"
}
```

---

## Backups

### POST `/system/backups`

**Auth:** JWT required

Create a system backup.

**Response (201):**
```json
{
  "filename": "backup_2025-06-15T12-00-00.tar.gz",
  "sizeBytes": 52428800,
  "createdAt": "2025-06-15T12:00:00Z"
}
```

### GET `/system/backups`

**Auth:** JWT required

List available backups.

**Response (200):**
```json
[
  {
    "filename": "backup_2025-06-15T12-00-00.tar.gz",
    "sizeBytes": 52428800,
    "createdAt": "2025-06-15T12:00:00Z"
  }
]
```

### GET `/system/backups/:filename/download`

**Auth:** JWT required

Download a backup file.

**Response (200):** Binary `application/gzip` content.

### DELETE `/system/backups/:filename`

**Auth:** JWT required

Delete a backup file.

**Response (204):** No content.

### POST `/system/backups/validate`

**Auth:** JWT required

Validate a backup file before restoring.

**Request:**
```json
{
  "filename": "backup_2025-06-15T12-00-00.tar.gz"
}
```

**Response (200):**
```json
{
  "valid": true,
  "version": "1.5.0",
  "createdAt": "2025-06-15T12:00:00Z",
  "components": ["database", "config", "certificates"]
}
```

### POST `/system/backups/restore`

**Auth:** JWT required

Restore from a backup.

**Request:**
```json
{
  "filename": "backup_2025-06-15T12-00-00.tar.gz"
}
```

**Response (202):**
```json
{
  "message": "restore initiated, system will restart"
}
```

### PUT `/system/backups/schedule`

**Auth:** JWT required

Configure automatic backup schedule.

**Request:**
```json
{
  "enabled": true,
  "cronExpression": "0 2 * * *",
  "retainCount": 7
}
```

**Response (200):**
```json
{
  "message": "backup schedule updated"
}
```

### GET `/system/backups/schedule`

**Auth:** JWT required

Get the current backup schedule.

**Response (200):**
```json
{
  "enabled": true,
  "cronExpression": "0 2 * * *",
  "retainCount": 7,
  "lastRun": "2025-06-15T02:00:00Z",
  "nextRun": "2025-06-16T02:00:00Z"
}
```

---

## Security Configuration

### GET `/system/security/config`

**Auth:** JWT required

Get the active security configuration (CORS, CSP, rate limiting).

**Response (200):**
```json
{
  "cors": {
    "allowedOrigins": ["*"],
    "allowedMethods": ["GET", "POST", "PUT", "DELETE"],
    "allowedHeaders": ["Authorization", "Content-Type"],
    "maxAge": 3600
  },
  "csp": {
    "contentSecurityPolicy": "default-src 'self'",
    "frameOptions": "DENY"
  },
  "rateLimit": {
    "enabled": true,
    "perSecond": 10,
    "burst": 20,
    "cleanupSec": 300
  }
}
```

---

## System Updates

### GET `/system/updates/check`

**Auth:** JWT required

Check for available system updates.

**Response (200):**
```json
{
  "currentVersion": "1.5.0",
  "latestVersion": "1.6.0",
  "updateAvailable": true,
  "releaseNotes": "Bug fixes and new features..."
}
```

### POST `/system/updates/apply`

**Auth:** JWT required

Apply an available update.

**Response (202):**
```json
{
  "message": "update started, system will restart"
}
```

### POST `/system/updates/rollback`

**Auth:** JWT required

Rollback to the previous version.

**Response (202):**
```json
{
  "message": "rollback initiated"
}
```

### GET `/system/updates/history`

**Auth:** JWT required

Get update history.

**Response (200):**
```json
[
  {
    "version": "1.5.0",
    "appliedAt": "2025-06-01T02:00:00Z",
    "status": "success"
  }
]
```

---

## TLS Certificate Management

### GET `/system/tls/status`

**Auth:** JWT required

Get TLS certificate status.

**Response (200):**
```json
{
  "enabled": true,
  "issuer": "Let's Encrypt",
  "subject": "nvr.example.com",
  "notBefore": "2025-06-01T00:00:00Z",
  "notAfter": "2025-09-01T00:00:00Z",
  "selfSigned": false
}
```

### POST `/system/tls/upload`

**Auth:** JWT required

Upload a TLS certificate and private key.

**Request:** `multipart/form-data` with `cert` and `key` files.

**Response (200):**
```json
{
  "message": "certificate uploaded",
  "restartRequired": true
}
```

### POST `/system/tls/generate`

**Auth:** JWT required

Generate a self-signed TLS certificate.

**Request:**
```json
{
  "commonName": "nvr.local",
  "validDays": 365
}
```

**Response (200):**
```json
{
  "message": "self-signed certificate generated",
  "restartRequired": true
}
```

---

## HLS VoD Playback

### GET `/vod/:cameraId/playlist.m3u8`

**Auth:** JWT required

Get an HLS playlist for video-on-demand playback.

**Query parameters:** `start`, `end`

**Response (200):** `application/vnd.apple.mpegurl` content.

### GET `/vod/thumbnail`

**Auth:** JWT required

Get a thumbnail for a VoD segment.

**Query parameters:** `cameraId`, `timestamp`

**Response (200):** Binary `image/jpeg` content.

### GET `/vod/segments/:id`

**Auth:** None (token embedded in playlist URL)

Serve an HLS video segment.

**Response (200):** Binary `video/mp2t` content.

---

## Storage

### GET `/storage/status`

**Auth:** JWT required

Get storage health and sync status.

**Response (200):**
```json
{
  "healthy": true,
  "syncRunning": false,
  "lastSync": "2025-06-15T12:00:00Z",
  "pendingDeletes": 0
}
```

### GET `/storage/pending`

**Auth:** JWT required

List files pending deletion or sync.

**Response (200):**
```json
[
  { "path": "/recordings/old-file.mp4", "reason": "retention_expired", "scheduledAt": "2025-06-15T00:00:00Z" }
]
```

### POST `/storage/sync/:camera_id`

**Auth:** JWT required

Trigger a storage sync for a specific camera.

**Response (202):**
```json
{
  "message": "sync triggered"
}
```

---

## Storage Quotas

### GET `/quotas`

**Auth:** JWT required

List all storage quotas (global and per-camera).

**Response (200):**
```json
{
  "global": { "maxBytes": 1099511627776, "usedBytes": 549755813888 },
  "cameras": [
    { "cameraId": "cam-abc123", "maxBytes": 214748364800, "usedBytes": 107374182400 }
  ]
}
```

### PUT `/quotas/global`

**Auth:** JWT required

Set the global storage quota.

**Request:**
```json
{
  "maxBytes": 2199023255552
}
```

**Response (200):**
```json
{
  "message": "global quota updated"
}
```

### GET `/quotas/status`

**Auth:** JWT required

Get overall quota utilization status.

**Response (200):**
```json
{
  "usagePercent": 50.0,
  "warningThreshold": 80,
  "criticalThreshold": 95,
  "status": "ok"
}
```

### PUT `/cameras/:id/quota`

**Auth:** JWT required

Set a per-camera storage quota.

**Request:**
```json
{
  "maxBytes": 214748364800
}
```

**Response (200):**
```json
{
  "message": "camera quota updated"
}
```

---

## AI Semantic Search

### GET `/search`

**Auth:** JWT required

Search recordings and detections using natural language (CLIP embeddings).

**Query parameters:** `q`, `cameraId`, `start`, `end`, `limit`

**Response (200):**
```json
{
  "results": [
    {
      "cameraId": "cam-abc123",
      "timestamp": "2025-06-15T12:30:00Z",
      "score": 0.87,
      "thumbnailUrl": "/thumbnails/result_001.jpg",
      "label": "person with red jacket"
    }
  ]
}
```

### POST `/search/backfill`

**Auth:** JWT required

Trigger a backfill of CLIP embeddings for existing recordings.

**Response (202):**
```json
{
  "message": "backfill started"
}
```

---

## Evidence Exports

### POST `/exports/evidence`

**Auth:** JWT required

Create an evidence export package (for law enforcement or legal use). Audit-logged.

**Request:**
```json
{
  "cameraIds": ["cam-abc123"],
  "start": "2025-06-15T08:00:00Z",
  "end": "2025-06-15T09:00:00Z",
  "caseNumber": "CASE-2025-001",
  "notes": "Requested by Officer Smith"
}
```

**Response (201):**
```json
{
  "id": "ev-001",
  "status": "pending"
}
```

### GET `/exports/evidence`

**Auth:** JWT required

List evidence exports.

**Response (200):**
```json
[
  {
    "id": "ev-001",
    "caseNumber": "CASE-2025-001",
    "status": "completed",
    "createdAt": "2025-06-15T10:00:00Z"
  }
]
```

### GET `/exports/evidence/:id/download`

**Auth:** JWT required

Download a completed evidence export package.

**Response (200):** Binary `application/zip` content.

---

## Edge Search

### GET `/edge-search/recordings`

**Auth:** JWT required

Search recordings on camera edge storage (ONVIF Profile G).

**Query parameters:** `cameraId`, `start`, `end`

**Response (200):**
```json
[
  {
    "recordingToken": "rec001",
    "startTime": "2025-06-15T08:00:00Z",
    "endTime": "2025-06-15T08:30:00Z"
  }
]
```

### GET `/edge-search/events`

**Auth:** JWT required

Search events on camera edge storage.

**Query parameters:** `cameraId`, `start`, `end`, `type`

**Response (200):**
```json
[
  {
    "topic": "tns1:VideoAnalytics/MotionDetection",
    "time": "2025-06-15T08:15:00Z",
    "source": "VideoSource_1"
  }
]
```

---

## Audit Log

### GET `/audit`

**Auth:** JWT required (admin only)

List audit log entries.

**Query parameters:** `userId`, `action`, `start`, `end`, `limit`, `offset`

**Response (200):**
```json
[
  {
    "id": 1,
    "userId": "user-001",
    "action": "camera.create",
    "resource": "cam-abc123",
    "ipAddress": "192.168.1.50",
    "timestamp": "2025-06-15T12:00:00Z",
    "details": {}
  }
]
```

### GET `/audit/export`

**Auth:** JWT required (admin only)

Export audit logs as CSV or JSON.

**Query parameters:** `format` (`csv` or `json`), `start`, `end`

**Response (200):** Binary content with appropriate Content-Type.

### GET `/audit/retention`

**Auth:** JWT required (admin only)

Get audit log retention settings.

**Response (200):**
```json
{
  "retentionDays": 90
}
```

### PUT `/audit/retention`

**Auth:** JWT required (admin only)

Update audit log retention settings.

**Request:**
```json
{
  "retentionDays": 180
}
```

**Response (200):**
```json
{
  "message": "retention updated"
}
```

---

## Camera Groups

### GET `/camera-groups`

**Auth:** JWT required

List camera groups.

**Response (200):**
```json
[
  { "id": "grp-001", "name": "Building A", "cameraIds": ["cam-abc123", "cam-def456"] }
]
```

### POST `/camera-groups`

**Auth:** JWT required

Create a camera group.

**Request:**
```json
{
  "name": "Building A",
  "cameraIds": ["cam-abc123", "cam-def456"]
}
```

**Response (201):**
```json
{
  "id": "grp-001"
}
```

### GET `/camera-groups/:id`

**Auth:** JWT required

Get a camera group by ID.

**Response (200):**
```json
{
  "id": "grp-001",
  "name": "Building A",
  "cameraIds": ["cam-abc123", "cam-def456"]
}
```

### PUT `/camera-groups/:id`

**Auth:** JWT required

Update a camera group.

**Request:**
```json
{
  "name": "Building A - Main",
  "cameraIds": ["cam-abc123", "cam-def456", "cam-ghi789"]
}
```

**Response (200):**
```json
{
  "message": "group updated"
}
```

### DELETE `/camera-groups/:id`

**Auth:** JWT required

Delete a camera group.

**Response (204):** No content.

---

## Devices

### GET `/devices`

**Auth:** JWT required

List ONVIF devices (physical devices that may have multiple cameras/channels).

**Response (200):**
```json
[
  {
    "id": "dev-001",
    "host": "192.168.1.100",
    "manufacturer": "HIKVISION",
    "model": "DS-2CD2143G2-I",
    "cameraCount": 1
  }
]
```

### GET `/devices/:id`

**Auth:** JWT required

Get a device by ID.

**Response (200):**
```json
{
  "id": "dev-001",
  "host": "192.168.1.100",
  "manufacturer": "HIKVISION",
  "model": "DS-2CD2143G2-I",
  "cameras": ["cam-abc123"]
}
```

### DELETE `/devices/:id`

**Auth:** JWT required

Delete a device and all its associated cameras.

**Response (204):** No content.

---

## Tours

### GET `/tours`

**Auth:** JWT required

List PTZ tours.

**Response (200):**
```json
[
  {
    "id": "tour-001",
    "name": "Perimeter Check",
    "cameraId": "cam-abc123",
    "presets": [
      { "presetToken": "1", "dwellSec": 10, "speed": 0.5 },
      { "presetToken": "2", "dwellSec": 15, "speed": 0.5 }
    ]
  }
]
```

### POST `/tours`

**Auth:** JWT required

Create a PTZ tour.

**Request:**
```json
{
  "name": "Perimeter Check",
  "cameraId": "cam-abc123",
  "presets": [
    { "presetToken": "1", "dwellSec": 10, "speed": 0.5 },
    { "presetToken": "2", "dwellSec": 15, "speed": 0.5 }
  ]
}
```

**Response (201):**
```json
{
  "id": "tour-001"
}
```

### GET `/tours/:id`

**Auth:** JWT required

Get a tour by ID.

**Response (200):**
```json
{
  "id": "tour-001",
  "name": "Perimeter Check",
  "cameraId": "cam-abc123",
  "presets": [
    { "presetToken": "1", "dwellSec": 10, "speed": 0.5 }
  ]
}
```

### PUT `/tours/:id`

**Auth:** JWT required

Update a tour.

**Response (200):**
```json
{
  "message": "tour updated"
}
```

### DELETE `/tours/:id`

**Auth:** JWT required

Delete a tour.

**Response (204):** No content.

---

## Export Jobs

### POST `/exports`

**Auth:** JWT required

Create an export job to produce a downloadable clip.

**Request:**
```json
{
  "cameraId": "cam-abc123",
  "start": "2025-06-15T08:00:00Z",
  "end": "2025-06-15T08:30:00Z",
  "format": "mp4"
}
```

**Response (201):**
```json
{
  "id": "exp-001",
  "status": "queued"
}
```

### GET `/exports`

**Auth:** JWT required

List export jobs.

**Response (200):**
```json
[
  { "id": "exp-001", "status": "completed", "progress": 100 }
]
```

### GET `/exports/:id`

**Auth:** JWT required

Get an export job's status.

**Response (200):**
```json
{
  "id": "exp-001",
  "status": "completed",
  "progress": 100,
  "filename": "export_cam-abc123_2025-06-15.mp4"
}
```

### DELETE `/exports/:id`

**Auth:** JWT required

Delete an export job and its file.

**Response (204):** No content.

### GET `/exports/:id/download`

**Auth:** JWT required

Download the completed export file.

**Response (200):** Binary video content with `Content-Disposition` header.

---

## Camera Connection Resilience

### GET `/cameras/:id/connection`

**Auth:** JWT required

Get the current connection state for a camera.

**Response (200):**
```json
{
  "cameraId": "cam-abc123",
  "state": "connected",
  "since": "2025-06-15T08:00:00Z",
  "reconnectAttempts": 0
}
```

### GET `/cameras/:id/connection/history`

**Auth:** JWT required

Get connection state history for a camera.

**Query parameters:** `limit`, `offset`

**Response (200):**
```json
[
  { "state": "connected", "timestamp": "2025-06-15T08:00:00Z" },
  { "state": "disconnected", "timestamp": "2025-06-15T07:55:00Z", "reason": "timeout" }
]
```

### GET `/cameras/:id/connection/summary`

**Auth:** JWT required

Get a connection reliability summary.

**Response (200):**
```json
{
  "uptimePercent": 99.5,
  "totalDisconnections": 3,
  "avgReconnectTimeSec": 12,
  "lastDisconnection": "2025-06-15T07:55:00Z"
}
```

### GET `/cameras/:id/connection/queue`

**Auth:** JWT required

Get commands queued while camera was disconnected.

**Response (200):**
```json
[
  { "id": "cmd-001", "type": "ptz", "queuedAt": "2025-06-15T07:56:00Z", "status": "pending" }
]
```

### GET `/connections`

**Auth:** JWT required

Get connection states for all cameras.

**Response (200):**
```json
[
  { "cameraId": "cam-abc123", "state": "connected" },
  { "cameraId": "cam-def456", "state": "reconnecting", "reconnectAttempts": 2 }
]
```

---

## Static File Routes

### `/thumbnails/*`

**Auth:** None

Serves event thumbnail images as static files.

### `/screenshots/*`

**Auth:** None

Serves screenshot images as static files.

---

## ONVIF Callback

### POST `/onvif-callback/:cameraId`

**Auth:** None (camera-to-NVR callback)

Receives ONVIF event notifications from cameras. The camera POSTs XML notification payloads to this endpoint. Not intended for client use.

**Request body:** ONVIF XML notification (max 1 MB).

**Response (200):** Empty body.
