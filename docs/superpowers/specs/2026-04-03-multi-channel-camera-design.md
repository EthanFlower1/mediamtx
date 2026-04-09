# KAI-26: Multi-Channel Camera Support Design Spec

## Overview

Handle cameras with multiple video channels (e.g., multi-sensor cameras like Hanwha PNM-9322VQP) as separate logical streams. Each channel becomes its own Camera record with independent recording and configuration, grouped under a parent Device in the API.

## Approach

**Option chosen: Channels as cameras with a parent device group.**

Each video source (channel) on a multi-sensor camera becomes a full `Camera` record. A new `devices` table holds the shared physical device metadata (ONVIF endpoint, credentials, manufacturer info). Cameras link to their device via `device_id`. This leverages the existing `Camera` + `CameraStream` architecture with minimal schema changes.

Single-channel cameras continue to work exactly as before with `device_id = NULL`.

## ONVIF Discovery Enhancement

### VideoSource Detection

During discovery, detect multiple video sources to identify multi-channel cameras:

1. Call `GetVideoSources()` (Media1/Media2 service) to enumerate physical video sources
2. Call `GetProfiles()` as today — each profile references a `VideoSourceToken`
3. Group profiles by `VideoSourceToken` — each group = one channel
4. Single video source → single-channel (current behavior)
5. Multiple video sources → multi-channel device

### New Types

```go
type VideoSource struct {
    Token  string
    Width  int
    Height int
}

type DiscoveredChannel struct {
    VideoSourceToken string         `json:"video_source_token"`
    Name             string         `json:"name"`
    Profiles         []MediaProfile `json:"profiles"`
}
```

### Changes to Existing Types

**MediaProfile** — add `VideoSourceToken string` field to track which video source the profile belongs to.

**DiscoveredDevice** — add `Channels []DiscoveredChannel` field. Single-channel devices keep the flat `Profiles` field for backward compatibility.

**ProbeResult** — add `VideoSources []VideoSource` field.

### Implementation

- **media2.go**: Add `GetVideoSources()` method returning `[]VideoSource`
- **device.go**: Update `ProbeDeviceFull()` to call `GetVideoSources()` and include in result
- **discovery.go**: During `enrichDevice()`, get video sources, group profiles by VideoSourceToken to build `Channels` array
- **Fallback**: If `GetVideoSources` fails, treat all profiles as single-channel

## Database Schema

### New `devices` Table

| Column             | Type              | Description                     |
| ------------------ | ----------------- | ------------------------------- |
| `id`               | TEXT PK           | UUID                            |
| `name`             | TEXT NOT NULL     | User-assigned device name       |
| `manufacturer`     | TEXT              | From ONVIF GetDeviceInformation |
| `model`            | TEXT              | From ONVIF                      |
| `firmware_version` | TEXT              | From ONVIF                      |
| `onvif_endpoint`   | TEXT              | Device ONVIF address            |
| `onvif_username`   | TEXT              | Shared credentials              |
| `onvif_password`   | TEXT              | Encrypted at rest               |
| `channel_count`    | INTEGER DEFAULT 1 | Number of video sources         |
| `created_at`       | TEXT NOT NULL     | Timestamp                       |
| `updated_at`       | TEXT NOT NULL     | Timestamp                       |

### Changes to `cameras` Table

| Column          | Type         | Description                                    |
| --------------- | ------------ | ---------------------------------------------- |
| `device_id`     | TEXT NULL    | FK to devices(id), NULL for standalone cameras |
| `channel_index` | INTEGER NULL | 0-based channel number within device           |

### Migration Strategy

- New migration adds `devices` table and two columns to `cameras`
- Existing cameras remain standalone (`device_id = NULL, channel_index = NULL`)
- No data migration required

## API Changes

### Discovery Response

`DiscoveredDevice` gains channel info for multi-channel devices:

```json
{
  "xaddr": "192.168.1.50",
  "manufacturer": "Hanwha",
  "model": "PNM-9322VQP",
  "channels": [
    {
      "video_source_token": "VideoSource_1",
      "name": "Channel 1",
      "profiles": [{"token": "Profile_1", "stream_uri": "rtsp://...", "width": 2560, "height": 1440}]
    },
    {
      "video_source_token": "VideoSource_2",
      "name": "Channel 2",
      "profiles": [{"token": "Profile_3", "stream_uri": "rtsp://...", "width": 2560, "height": 1440}]
    }
  ],
  "profiles": [...]
}
```

Single-channel devices: `channels` is nil/omitted, flat `profiles` used as today.

### Adding a Multi-Channel Camera

`POST /cameras` accepts new optional `device` + `channels` fields:

```json
{
  "device": {
    "name": "Front Entrance Multi-Sensor",
    "onvif_endpoint": "192.168.1.50",
    "onvif_username": "admin",
    "onvif_password": "pass"
  },
  "channels": [
    {"name": "Front Left", "rtsp_url": "rtsp://...", "profiles": [...], "channel_index": 0},
    {"name": "Front Right", "rtsp_url": "rtsp://...", "profiles": [...], "channel_index": 1}
  ]
}
```

Creates one `Device` record + N `Camera` records with streams. Single-channel `POST /cameras` without `device`/`channels` works exactly as today.

### Camera List with Device Grouping

`GET /cameras?group_by=device` returns grouped response:

```json
{
  "devices": [
    {
      "id": "device-uuid",
      "name": "Front Entrance Multi-Sensor",
      "manufacturer": "Hanwha",
      "model": "PNM-9322VQP",
      "cameras": [
        {"id": "cam-1", "name": "Front Left", "channel_index": 0, "streams": [...]},
        {"id": "cam-2", "name": "Front Right", "channel_index": 1, "streams": [...]}
      ]
    }
  ],
  "standalone": [
    {"id": "cam-3", "name": "Parking Lot", "streams": [...]}
  ]
}
```

Default `GET /cameras` (no group_by) returns flat list as today, with `device_id` and `channel_index` included on each camera.

### New Device Endpoints

- `GET /devices` — list all devices with their cameras
- `GET /devices/:id` — single device with cameras
- `DELETE /devices/:id` — deletes device + all cameras/streams/recordings (cascade)

### Credential Resolution

When a camera has `device_id` set, ONVIF operations read credentials from the `devices` table. Standalone cameras use their own credentials as today.

## Path Naming & Recording

- Paths unchanged: `nvr/<camera-id>/main` and `nvr/<camera-id>~<stream-prefix>/`
- Each channel is its own Camera with unique UUID, so paths are naturally unique
- Recording is fully independent per channel — existing `OnSegmentComplete` works unchanged
- Recording rules, retention, and storage quotas are per-camera (per-channel)

## Deletion Cascade

- `DELETE /devices/:id` deletes device → all cameras → streams → recordings → rules
- Individual channels can be deleted independently from their device
- Deleting last channel of a device does not auto-delete the device (explicit cleanup)

## Testing Strategy

1. **ONVIF unit tests**: Mock `GetVideoSources` for single (1 source) and multi-channel (2+ sources); verify profile grouping by VideoSourceToken
2. **Database migration test**: Verify `devices` table creation, new columns on `cameras`, existing data unaffected
3. **API integration tests**:
   - Add single-channel camera → no device created, works as before
   - Add multi-channel camera → device + N cameras created with correct streams
   - `GET /cameras?group_by=device` returns correct grouping
   - `DELETE /devices/:id` cascades properly
   - Discovery results show channels for multi-source devices
4. **Credential resolution test**: Camera with `device_id` reads credentials from device record
