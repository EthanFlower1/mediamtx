# ONVIF Profile S Complete Implementation

**Date:** 2026-03-31
**Scope:** Full ONVIF Profile S client support (server + Flutter) across 4 phases

## Overview

Comprehensive end-to-end ONVIF Profile S implementation. The server already has extensive ONVIF support (PTZ, relay, imaging, events, analytics, edge recording, discovery, snapshots, media profiles). This spec covers:

1. Wiring existing server features to Flutter
2. Adding new ONVIF capabilities (enhanced PTZ, media configuration, device management)
3. Full Flutter UI for all features

All new ONVIF calls use the `onvif-go` library's `Client` methods. No new custom SOAP. All features exposed via REST API under `/api/nvr/cameras/:id/` and rendered as collapsible sections in the existing camera detail screen.

No database migrations — all data is live from cameras, not stored in the NVR (capability flags already exist in the cameras table).

---

## Phase 1: Wire-up Existing Features

Connect server capabilities that already exist to the Flutter client.

### Server Changes

**One new API endpoint:**

- `GET /cameras/:id/device-info` — calls `client.Dev.GetDeviceInformation(ctx)`, returns:

```json
{
  "manufacturer": "string",
  "model": "string",
  "firmware_version": "string",
  "serial_number": "string",
  "hardware_id": "string"
}
```

All other endpoints already exist:

- `GET/PUT /cameras/:id/settings` (imaging)
- `GET /cameras/:id/relay-outputs`, `POST /cameras/:id/relay-outputs/:token/state` (relay)
- `GET /cameras/:id/ptz/presets`, `POST /cameras/:id/ptz` with preset/home actions (PTZ)
- `GET /cameras/:id/audio/capabilities` (audio)

### Flutter Changes

**New collapsible sections in camera detail screen:**

1. **Device Info** (top, always visible) — manufacturer, model, firmware, serial. Fetched once on load.

2. **Imaging Settings** — existing brightness/contrast/saturation/sharpness sliders wired to `GET /cameras/:id/settings` on load and `PUT /cameras/:id/settings` on change. Debounced save (500ms after last slider move).

3. **Relay Outputs** — list of relay outputs with toggle switches. Each toggle calls `POST /cameras/:id/relay-outputs/:token/state` with `{"active": true/false}`.

4. **PTZ Presets** (shown if `ptz_capable`) — list of presets with "Go To" buttons. "Home" button at top. Fetched from `GET /cameras/:id/ptz/presets`.

5. **Audio Capabilities** — read-only: has microphone (yes/no), has speaker/backchannel (yes/no). From `GET /cameras/:id/audio/capabilities`.

**New Riverpod providers:**

- `deviceInfoProvider(cameraId)` — fetches device info
- `imagingSettingsProvider(cameraId)` — fetches/updates imaging settings
- `relayOutputsProvider(cameraId)` — fetches relay outputs
- `ptzPresetsProvider(cameraId)` — fetches presets list
- `audioCapabilitiesProvider(cameraId)` — fetches audio caps

---

## Phase 2: Enhanced PTZ

### Server Changes

**New methods in `ptz.go` on `PTZController`:**

- `AbsoluteMove(profileToken string, panPos, tiltPos, zoomPos float64) error`
- `RelativeMove(profileToken string, panDelta, tiltDelta, zoomDelta float64) error`
- `SetPreset(profileToken, presetName string) (string, error)` — returns new token
- `RemovePreset(profileToken, presetToken string) error`
- `SetHomePosition(profileToken string) error`
- `GetStatus(profileToken string) (*PTZStatus, error)`

**New type:**

```go
type PTZStatus struct {
    PanPosition  float64 `json:"pan_position"`
    TiltPosition float64 `json:"tilt_position"`
    ZoomPosition float64 `json:"zoom_position"`
    IsMoving     bool    `json:"is_moving"`
}
```

**Extended `POST /cameras/:id/ptz` actions:**

- `{"action": "absolute_move", "pan": 0.5, "tilt": -0.3, "zoom": 0.2}`
- `{"action": "relative_move", "pan": 0.1, "tilt": 0.0, "zoom": 0.0}`
- `{"action": "set_preset", "name": "Front Door"}` — returns `{"token": "..."}`
- `{"action": "remove_preset", "preset_token": "..."}`
- `{"action": "set_home"}`

**New endpoint:**

- `GET /cameras/:id/ptz/status` — current position and movement state

### Flutter Changes

**Enhanced PTZ control panel:**

1. Directional pad — existing, unchanged
2. Preset management — list with "Go To" buttons, "Save Current Position" (prompts for name), swipe-to-delete
3. Home controls — "Go Home" + "Set Home" buttons
4. Position display — pan/tilt/zoom readout, polled every 2s while PTZ panel is open
5. Virtual joystick — overlay on live view, sends continuous move while held, stops on release

**New provider:**

- `ptzStatusProvider(cameraId)` — polls status every 2s when PTZ panel expanded

---

## Phase 3: Media Configuration

### Server Changes

**New file `internal/nvr/onvif/media_config.go`:**

Profile management:

- `GetProfilesFull(xaddr, user, pass) ([]*ProfileInfo, error)`
- `GetProfileFull(xaddr, user, pass, token) (*ProfileInfo, error)`
- `CreateMediaProfile(xaddr, user, pass, name) (*ProfileInfo, error)`
- `DeleteMediaProfile(xaddr, user, pass, token) error`

Video configuration:

- `GetVideoSources(xaddr, user, pass) ([]*VideoSourceInfo, error)`
- `GetVideoEncoderConfig(xaddr, user, pass, token) (*VideoEncoderConfig, error)`
- `SetVideoEncoderConfig(xaddr, user, pass, config *VideoEncoderConfig) error`
- `GetVideoEncoderOptions(xaddr, user, pass, configToken string) (*VideoEncoderOptions, error)`
- `GetCompatibleVideoEncoderConfigs(xaddr, user, pass, profileToken string) ([]*VideoEncoderConfig, error)`
- `AddVideoEncoderToProfile(xaddr, user, pass, profileToken, configToken string) error`
- `RemoveVideoEncoderFromProfile(xaddr, user, pass, profileToken string) error`

Audio configuration:

- `GetAudioEncoderConfig(xaddr, user, pass, token) (*AudioEncoderConfig, error)`
- `SetAudioEncoderConfig(xaddr, user, pass, config *AudioEncoderConfig) error`
- `GetAudioEncoderOptions(xaddr, user, pass, configToken string) (*AudioEncoderOptions, error)`
- `AddAudioEncoderToProfile(xaddr, user, pass, profileToken, configToken string) error`
- `RemoveAudioEncoderFromProfile(xaddr, user, pass, profileToken string) error`

**Types:**

```go
type ProfileInfo struct {
    Token        string              `json:"token"`
    Name         string              `json:"name"`
    VideoSource  *VideoSourceInfo    `json:"video_source,omitempty"`
    VideoEncoder *VideoEncoderConfig `json:"video_encoder,omitempty"`
    AudioEncoder *AudioEncoderConfig `json:"audio_encoder,omitempty"`
    PTZConfig    *PTZConfigInfo      `json:"ptz_config,omitempty"`
}

type VideoSourceInfo struct {
    Token     string  `json:"token"`
    Framerate float64 `json:"framerate"`
    Width     int     `json:"width"`
    Height    int     `json:"height"`
}

type VideoEncoderConfig struct {
    Token            string  `json:"token"`
    Name             string  `json:"name"`
    Encoding         string  `json:"encoding"`
    Width            int     `json:"width"`
    Height           int     `json:"height"`
    Quality          float64 `json:"quality"`
    FrameRate        int     `json:"frame_rate"`
    BitrateLimit     int     `json:"bitrate_limit"`
    EncodingInterval int     `json:"encoding_interval"`
    GovLength        int     `json:"gov_length,omitempty"`
    H264Profile      string  `json:"h264_profile,omitempty"`
}

type VideoEncoderOptions struct {
    Encodings             []string     `json:"encodings"`
    Resolutions           []Resolution `json:"resolutions"`
    FrameRateRange        Range        `json:"frame_rate_range"`
    QualityRange          Range        `json:"quality_range"`
    BitrateRange          Range        `json:"bitrate_range,omitempty"`
    GovLengthRange        Range        `json:"gov_length_range,omitempty"`
    H264Profiles          []string     `json:"h264_profiles,omitempty"`
    EncodingIntervalRange Range        `json:"encoding_interval_range,omitempty"`
}

type AudioEncoderConfig struct {
    Token      string `json:"token"`
    Name       string `json:"name"`
    Encoding   string `json:"encoding"`
    Bitrate    int    `json:"bitrate"`
    SampleRate int    `json:"sample_rate"`
}

type AudioEncoderOptions struct {
    Encodings   []string `json:"encodings"`
    BitrateList []int    `json:"bitrate_list"`
    SampleRates []int    `json:"sample_rate_list"`
}

type Resolution struct {
    Width  int `json:"width"`
    Height int `json:"height"`
}

type Range struct {
    Min int `json:"min"`
    Max int `json:"max"`
}

type PTZConfigInfo struct {
    Token     string `json:"token"`
    Name      string `json:"name"`
    NodeToken string `json:"node_token"`
}
```

**API endpoints:**

Profiles:

- `GET /cameras/:id/media/profiles`
- `GET /cameras/:id/media/profiles/:token`
- `POST /cameras/:id/media/profiles` — `{"name": "..."}`
- `DELETE /cameras/:id/media/profiles/:token`

Video:

- `GET /cameras/:id/media/video-sources`
- `GET /cameras/:id/media/video-encoder/:token`
- `PUT /cameras/:id/media/video-encoder/:token`
- `GET /cameras/:id/media/video-encoder/:token/options`
- `POST /cameras/:id/media/profiles/:token/video-encoder` — `{"config_token": "..."}`
- `DELETE /cameras/:id/media/profiles/:token/video-encoder`

Audio:

- `GET /cameras/:id/media/audio-encoder/:token`
- `PUT /cameras/:id/media/audio-encoder/:token`
- `GET /cameras/:id/media/audio-encoder/:token/options`
- `POST /cameras/:id/media/profiles/:token/audio-encoder` — `{"config_token": "..."}`
- `DELETE /cameras/:id/media/profiles/:token/audio-encoder`

### Flutter Changes

**Media Configuration section** (collapsible):

1. Profile list — cards with codec/resolution summary. "Add Profile" button. Swipe to delete.
2. Profile detail (tap to expand inline):
   - Video encoder: codec dropdown, resolution dropdown, quality/framerate/bitrate sliders (all populated from options endpoint), H.264 profile dropdown if applicable. Save button.
   - Audio encoder: codec dropdown, bitrate dropdown, sample rate dropdown. Save button.
   - Remove encoder buttons.
3. Video Sources — read-only list (token, native resolution, framerate).

**New providers:**

- `mediaProfilesProvider(cameraId)`
- `videoEncoderOptionsProvider(cameraId, configToken)`
- `audioEncoderOptionsProvider(cameraId, configToken)`
- `videoSourcesProvider(cameraId)`

---

## Phase 4: Device Management

### Server Changes

**New file `internal/nvr/onvif/device_mgmt.go`:**

System:

- `GetSystemDateAndTime(xaddr, user, pass) (*DateTimeInfo, error)`
- `SetSystemDateAndTime(xaddr, user, pass, info *DateTimeInfo) error`
- `GetDeviceHostname(xaddr, user, pass) (*HostnameInfo, error)`
- `SetDeviceHostname(xaddr, user, pass, name string) error`
- `DeviceReboot(xaddr, user, pass) (string, error)`
- `GetDeviceScopes(xaddr, user, pass) ([]string, error)`

Network:

- `GetNetworkInterfaces(xaddr, user, pass) ([]*NetworkInterfaceInfo, error)`
- `GetNetworkProtocols(xaddr, user, pass) ([]*NetworkProtocolInfo, error)`
- `SetNetworkProtocols(xaddr, user, pass, protocols []*NetworkProtocolInfo) error`
- `GetDNSConfig(xaddr, user, pass) (*DNSInfo, error)`
- `GetNTPConfig(xaddr, user, pass) (*NTPInfo, error)`
- `SetNTPConfig(xaddr, user, pass, fromDHCP bool, servers []string) error`

Device users:

- `GetDeviceUsers(xaddr, user, pass) ([]*DeviceUser, error)`
- `CreateDeviceUser(xaddr, user, pass, username, password, role string) error`
- `DeleteDeviceUser(xaddr, user, pass, username string) error`
- `SetDeviceUser(xaddr, user, pass, username, password, role string) error`

**Types:**

```go
type DateTimeInfo struct {
    Type           string `json:"type"` // "Manual" or "NTP"
    DaylightSaving bool   `json:"daylight_saving"`
    Timezone       string `json:"timezone"`
    UTCTime        string `json:"utc_time"`
    LocalTime      string `json:"local_time"`
}

type HostnameInfo struct {
    FromDHCP bool   `json:"from_dhcp"`
    Name     string `json:"name"`
}

type NetworkInterfaceInfo struct {
    Token   string      `json:"token"`
    Enabled bool        `json:"enabled"`
    MAC     string      `json:"mac"`
    IPv4    *IPv4Config `json:"ipv4,omitempty"`
    IPv6    *IPv6Config `json:"ipv6,omitempty"`
}

type IPv4Config struct {
    Enabled bool   `json:"enabled"`
    DHCP    bool   `json:"dhcp"`
    Address string `json:"address"`
    Prefix  int    `json:"prefix_length"`
}

type IPv6Config struct {
    Enabled bool   `json:"enabled"`
    DHCP    bool   `json:"dhcp"`
    Address string `json:"address"`
    Prefix  int    `json:"prefix_length"`
}

type NetworkProtocolInfo struct {
    Name    string `json:"name"` // HTTP, HTTPS, RTSP
    Enabled bool   `json:"enabled"`
    Port    int    `json:"port"`
}

type DNSInfo struct {
    FromDHCP bool     `json:"from_dhcp"`
    Servers  []string `json:"servers"`
}

type NTPInfo struct {
    FromDHCP bool     `json:"from_dhcp"`
    Servers  []string `json:"servers"`
}

type DeviceUser struct {
    Username string `json:"username"`
    Role     string `json:"role"` // Administrator, Operator, User
}
```

**API endpoints:**

System:

- `GET /cameras/:id/device/datetime`
- `PUT /cameras/:id/device/datetime`
- `GET /cameras/:id/device/hostname`
- `PUT /cameras/:id/device/hostname` — `{"name": "..."}`
- `POST /cameras/:id/device/reboot`
- `GET /cameras/:id/device/scopes`

Network:

- `GET /cameras/:id/device/network/interfaces`
- `GET /cameras/:id/device/network/protocols`
- `PUT /cameras/:id/device/network/protocols`
- `GET /cameras/:id/device/network/dns`
- `GET /cameras/:id/device/network/ntp`
- `PUT /cameras/:id/device/network/ntp`

Device users:

- `GET /cameras/:id/device/users`
- `POST /cameras/:id/device/users` — `{"username", "password", "role"}`
- `PUT /cameras/:id/device/users/:username`
- `DELETE /cameras/:id/device/users/:username`

### Flutter Changes

**Device Management section** (collapsible):

1. **System** — date/time with NTP toggle and server field, timezone display, hostname field with save, "Reboot Device" with confirmation dialog.

2. **Network** — interfaces list (read-only: name, MAC, IP, DHCP), protocols list (HTTP/HTTPS/RTSP with enabled toggle and port, save button), DNS servers (read-only), NTP config (DHCP toggle, manual server list).

3. **Device Users** — user list with role badges, "Add User" dialog (username, password, role dropdown), edit user (password/role), delete with confirmation.

**New providers:**

- `deviceDateTimeProvider(cameraId)`
- `deviceHostnameProvider(cameraId)`
- `networkInterfacesProvider(cameraId)`
- `networkProtocolsProvider(cameraId)`
- `deviceUsersProvider(cameraId)`
- `ntpConfigProvider(cameraId)`

---

## File Organization

### Server (Go)

New/modified files:

- `internal/nvr/onvif/media_config.go` — Phase 3 media configuration methods
- `internal/nvr/onvif/device_mgmt.go` — Phase 4 device management methods
- `internal/nvr/onvif/ptz.go` — Phase 2 enhanced PTZ methods (extend existing)
- `internal/nvr/api/cameras.go` — new endpoint handlers for all phases

### Flutter

New files per phase:

- `lib/providers/device_info_provider.dart`
- `lib/providers/imaging_settings_provider.dart`
- `lib/providers/relay_outputs_provider.dart`
- `lib/providers/ptz_presets_provider.dart`
- `lib/providers/audio_capabilities_provider.dart`
- `lib/providers/ptz_status_provider.dart`
- `lib/providers/media_profiles_provider.dart`
- `lib/providers/video_sources_provider.dart`
- `lib/providers/device_management_provider.dart`
- `lib/models/device_info.dart`
- `lib/models/imaging_settings.dart`
- `lib/models/relay_output.dart`
- `lib/models/ptz_preset.dart` (may already exist)
- `lib/models/ptz_status.dart`
- `lib/models/audio_capabilities.dart`
- `lib/models/media_profile.dart`
- `lib/models/video_encoder_config.dart`
- `lib/models/audio_encoder_config.dart`
- `lib/models/device_management.dart`
- `lib/widgets/device_info_section.dart`
- `lib/widgets/imaging_settings_section.dart`
- `lib/widgets/relay_outputs_section.dart`
- `lib/widgets/ptz_presets_section.dart`
- `lib/widgets/audio_capabilities_section.dart`
- `lib/widgets/ptz_joystick.dart`
- `lib/widgets/media_config_section.dart`
- `lib/widgets/device_management_section.dart`

Modified:

- `lib/screens/camera_detail_screen.dart` — add collapsible sections for each phase
