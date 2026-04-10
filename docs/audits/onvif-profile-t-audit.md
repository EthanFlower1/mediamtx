# ONVIF Profile T Compliance Audit

**Date:** 2026-04-02
**Ticket:** KAI-16
**Scope:** Audit of `internal/nvr/onvif/` against ONVIF Profile T specification
**Perspective:** NVR as ONVIF **client** consuming Profile T devices

---

## Summary

The current ONVIF implementation provides strong client-side coverage of Profile T features.
Core areas (discovery, device management, Media2, PTZ, events, imaging) are fully implemented.
The main gaps are in Media2 profile mutation operations, audio stream control, OSD management,
and some device service queries that are required by Profile T but not yet used by the NVR.

| Category              | Complete | Partial | Missing | Total  |
| --------------------- | -------- | ------- | ------- | ------ |
| Device Discovery      | 1        | 0       | 0       | 1      |
| Device Service        | 11       | 0       | 8       | 19     |
| Media2 Service        | 6        | 3       | 8       | 17     |
| Video Encoding        | 4        | 0       | 0       | 4      |
| Streaming / Transport | 3        | 0       | 1       | 4      |
| Audio                 | 1        | 1       | 3       | 5      |
| PTZ Service           | 12       | 0       | 0       | 12     |
| Event Service         | 5        | 0       | 1       | 6      |
| Security / Auth       | 4        | 0       | 1       | 5      |
| Imaging Service       | 2        | 0       | 2       | 4      |
| Video Analytics       | 4        | 0       | 1       | 5      |
| OSD                   | 0        | 0       | 5       | 5      |
| **Totals**            | **53**   | **4**   | **30**  | **87** |

---

## 1. Device Discovery (WS-Discovery)

| Feature                     | Requirement | Status       | Notes                                                                                               |
| --------------------------- | ----------- | ------------ | --------------------------------------------------------------------------------------------------- |
| WS-Discovery probe/response | Mandatory   | **Complete** | `discovery.go` - UDP multicast to 239.255.255.250:3702 with 3x retransmission and device enrichment |

---

## 2. Device Service

| Operation                  | Requirement | Status       | Notes                                                                                          |
| -------------------------- | ----------- | ------------ | ---------------------------------------------------------------------------------------------- |
| `GetSystemDateAndTime`     | Mandatory   | **Complete** | `device_mgmt.go` - timezone, UTC, local time                                                   |
| `GetDeviceInformation`     | Mandatory   | **Complete** | `device.go` - retrieved during `ProbeDevice()` via onvif-go                                    |
| `GetHostname`              | Mandatory   | **Complete** | `device_mgmt.go`                                                                               |
| `SetHostname`              | Mandatory   | **Complete** | `device_mgmt.go`                                                                               |
| `SystemReboot`             | Mandatory   | **Complete** | `device_mgmt.go`                                                                               |
| `GetNetworkInterfaces`     | Mandatory   | **Complete** | `device_mgmt.go`                                                                               |
| `GetNetworkProtocols`      | Mandatory   | **Complete** | `device_mgmt.go`                                                                               |
| `GetScopes`                | Mandatory   | **Complete** | `device_mgmt.go`                                                                               |
| `GetUsers`                 | Mandatory   | **Complete** | `device_mgmt.go`                                                                               |
| `CreateUsers`              | Conditional | **Complete** | `device_mgmt.go`                                                                               |
| `DeleteUsers`              | Conditional | **Complete** | `device_mgmt.go`                                                                               |
| `GetServices`              | Mandatory   | **Missing**  | Service detection uses `GetCapabilities` via onvif-go but `GetServices` is not called directly |
| `GetServiceCapabilities`   | Mandatory   | **Missing**  | Not implemented                                                                                |
| `GetCapabilities`          | Mandatory   | **Missing**  | Handled indirectly by onvif-go during client init, but not exposed as a callable operation     |
| `SetScopes`                | Mandatory   | **Missing**  | Only `GetScopes` implemented                                                                   |
| `GetDiscoveryMode`         | Mandatory   | **Missing**  | Not implemented                                                                                |
| `SetDiscoveryMode`         | Mandatory   | **Missing**  | Not implemented                                                                                |
| `GetDNS`                   | Mandatory   | **Complete** | `device_mgmt.go` - `GetDNSConfig()`                                                            |
| `GetNetworkDefaultGateway` | Mandatory   | **Missing**  | Not implemented                                                                                |
| `SetSystemDateAndTime`     | Mandatory   | **Missing**  | Only get is implemented                                                                        |

---

## 3. Media2 Service

Profile T requires Media2 (`ver20/media`) rather than Media1. The implementation has Media2 support in `media2.go` and `media_config.go`.

| Operation                             | Requirement | Status       | Notes                                                                                                                      |
| ------------------------------------- | ----------- | ------------ | -------------------------------------------------------------------------------------------------------------------------- |
| `GetProfiles`                         | Mandatory   | **Complete** | `media2.go` - `GetProfiles2()` with custom SOAP                                                                            |
| `GetStreamUri`                        | Mandatory   | **Complete** | `media2.go` - `GetStreamUri2()`                                                                                            |
| `GetSnapshotUri`                      | Mandatory   | **Complete** | `media2.go` - `GetSnapshotUri2()`                                                                                          |
| `GetVideoEncoderConfigurations`       | Mandatory   | **Complete** | `media_config.go` - `GetVideoEncoderConfig()`                                                                              |
| `SetVideoEncoderConfiguration`        | Mandatory   | **Complete** | `media_config.go` - `SetVideoEncoderConfig()`                                                                              |
| `GetVideoEncoderConfigurationOptions` | Mandatory   | **Complete** | `media_config.go` - `GetVideoEncoderOpts()`                                                                                |
| `CreateProfile`                       | Mandatory   | **Partial**  | `media_config.go` - implemented via Media1 WSDL; should also support Media2 `CreateProfile`                                |
| `DeleteProfile`                       | Mandatory   | **Partial**  | `media_config.go` - implemented via Media1 WSDL; should also support Media2 `DeleteProfile`                                |
| `GetVideoSourceConfigurations`        | Mandatory   | **Partial**  | `media_config.go` - `GetVideoSourcesList()` gets sources, but does not call Media2 `GetVideoSourceConfigurations` directly |
| `SetVideoSourceConfiguration`         | Mandatory   | **Missing**  | Not implemented                                                                                                            |
| `GetVideoSourceConfigurationOptions`  | Mandatory   | **Missing**  | Not implemented                                                                                                            |
| `AddConfiguration`                    | Mandatory   | **Missing**  | Profile/encoder binding uses Media1 operations; Media2 `AddConfiguration` not implemented                                  |
| `RemoveConfiguration`                 | Mandatory   | **Missing**  | Same as above                                                                                                              |
| `GetAudioSourceConfigurations`        | Conditional | **Missing**  | Audio capability detection exists, but Media2 audio source config not implemented                                          |
| `GetAudioEncoderConfigurations`       | Conditional | **Missing**  | `media_config.go` has `GetAudioEncoderCfg()` but via Media1, not Media2                                                    |
| `SetAudioEncoderConfiguration`        | Conditional | **Missing**  | Same - Media1 only                                                                                                         |
| `GetMetadataConfigurations`           | Conditional | **Missing**  | Metadata parsing exists (`metadata.go`) but Media2 metadata config not queried                                             |

---

## 4. Video Encoding

| Feature                                    | Requirement | Status       | Notes                                                                          |
| ------------------------------------------ | ----------- | ------------ | ------------------------------------------------------------------------------ |
| H.264 support                              | Mandatory   | **Complete** | Encoder options include H.264; stream URIs retrieved for H.264 profiles        |
| H.265 (HEVC) support                       | Mandatory   | **Complete** | RTSP playback for H.265 added in `c3ae6f66`; encoder config supports H.265     |
| JPEG snapshot                              | Mandatory   | **Complete** | `snapshot.go` - `CaptureSnapshot()` with multi-auth fallback                   |
| Resolution/framerate/bitrate configuration | Mandatory   | **Complete** | `media_config.go` - `SetVideoEncoderConfig()` supports all encoding parameters |

---

## 5. Streaming / Transport

| Feature                         | Requirement | Status       | Notes                                                      |
| ------------------------------- | ----------- | ------------ | ---------------------------------------------------------- |
| RTSP streaming                  | Mandatory   | **Complete** | Stream URIs retrieved via Media1 and Media2 `GetStreamUri` |
| RTP over UDP (unicast)          | Mandatory   | **Complete** | Default transport for RTSP streams                         |
| RTP over RTSP (TCP interleaved) | Mandatory   | **Complete** | Supported via RTSP client configuration                    |
| RTP multicast                   | Conditional | **Missing**  | No multicast stream support implemented                    |

---

## 6. Audio

| Feature                           | Requirement | Status       | Notes                                                                                            |
| --------------------------------- | ----------- | ------------ | ------------------------------------------------------------------------------------------------ |
| Audio capability detection        | Conditional | **Complete** | `audio.go` - `GetAudioCapabilities()` detects sources, outputs, backchannel                      |
| Audio source configuration        | Conditional | **Partial**  | Encoder config get/set exists in `media_config.go` but via Media1; no Media2 audio source config |
| Audio decoder configuration       | Conditional | **Missing**  | Not implemented (needed for backchannel/speaker)                                                 |
| Audio output configuration        | Conditional | **Missing**  | Not implemented                                                                                  |
| Bidirectional audio (backchannel) | Conditional | **Missing**  | Capability detection only; no backchannel stream control                                         |

---

## 7. PTZ Service (Conditional - if device has PTZ)

| Operation                                       | Requirement | Status       | Notes                                                               |
| ----------------------------------------------- | ----------- | ------------ | ------------------------------------------------------------------- |
| `ContinuousMove`                                | Conditional | **Complete** | `ptz.go`                                                            |
| `Stop`                                          | Conditional | **Complete** | `ptz.go`                                                            |
| `AbsoluteMove`                                  | Conditional | **Complete** | `ptz.go`                                                            |
| `RelativeMove`                                  | Conditional | **Complete** | `ptz.go`                                                            |
| `GetStatus`                                     | Conditional | **Complete** | `ptz.go`                                                            |
| `GetPresets`                                    | Conditional | **Complete** | `ptz.go`                                                            |
| `SetPreset`                                     | Conditional | **Complete** | `ptz.go`                                                            |
| `GotoPreset`                                    | Conditional | **Complete** | `ptz.go`                                                            |
| `RemovePreset`                                  | Conditional | **Complete** | `ptz.go`                                                            |
| `GotoHomePosition`                              | Conditional | **Complete** | `ptz.go`                                                            |
| `SetHomePosition`                               | Conditional | **Complete** | `ptz.go`                                                            |
| `GetConfigurations` / `GetConfigurationOptions` | Conditional | **Complete** | `ptz.go` - `GetNodes()` retrieves configuration and capability info |

---

## 8. Event Service

| Feature                           | Requirement | Status       | Notes                                                                                      |
| --------------------------------- | ----------- | ------------ | ------------------------------------------------------------------------------------------ |
| `CreatePullPointSubscription`     | Mandatory   | **Complete** | `events.go` - fallback from push                                                           |
| `PullMessages`                    | Mandatory   | **Complete** | `events.go` - 2s poll interval                                                             |
| `Subscribe` (WS-BaseNotification) | Mandatory   | **Complete** | `events.go` - push-first with callback manager                                             |
| `Unsubscribe`                     | Mandatory   | **Complete** | `events.go` - graceful cleanup on stop                                                     |
| `Renew`                           | Mandatory   | **Complete** | `events.go` - 48s renewal cycle, 60s termination window                                    |
| `GetEventProperties`              | Mandatory   | **Missing**  | Not implemented; event topics are pattern-matched but `GetEventProperties` is never called |

---

## 9. Security / Authentication

| Feature                     | Requirement | Status       | Notes                                                                              |
| --------------------------- | ----------- | ------------ | ---------------------------------------------------------------------------------- |
| HTTP Digest authentication  | Mandatory   | **Complete** | `snapshot.go` - full RFC 2617 with nonce, realm, qop, cnonce                       |
| WS-Security UsernameToken   | Mandatory   | **Complete** | `client.go` / custom SOAP - SHA-1 password digest with nonce and created timestamp |
| Access policy / user levels | Mandatory   | **Complete** | `device_mgmt.go` - user CRUD with role assignment (Administrator, Operator, User)  |
| Replay attack protection    | Mandatory   | **Complete** | Nonce + timestamp in WS-Security headers; nonce count in Digest auth               |
| HTTPS (TLS)                 | Conditional | **Missing**  | No TLS/HTTPS support for ONVIF SOAP communication; all traffic is HTTP             |

---

## 10. Imaging Service (Conditional)

| Operation                       | Requirement | Status       | Notes                                                      |
| ------------------------------- | ----------- | ------------ | ---------------------------------------------------------- |
| `GetImagingSettings`            | Conditional | **Complete** | `imaging.go` - brightness, contrast, saturation, sharpness |
| `SetImagingSettings`            | Conditional | **Complete** | `imaging.go`                                               |
| `GetOptions`                    | Conditional | **Missing**  | Imaging options (valid ranges) not queried                 |
| `Move` / `Stop` (focus control) | Conditional | **Missing**  | No focus motor control implemented                         |

---

## 11. Video Analytics (Conditional)

| Feature                                                 | Requirement | Status       | Notes                                                                   |
| ------------------------------------------------------- | ----------- | ------------ | ----------------------------------------------------------------------- |
| `GetSupportedRules`                                     | Conditional | **Complete** | `analytics.go`                                                          |
| `GetRules` / `CreateRule` / `ModifyRule` / `DeleteRule` | Conditional | **Complete** | `analytics.go` - full CRUD                                              |
| `GetAnalyticsModules`                                   | Conditional | **Complete** | `analytics.go`                                                          |
| Metadata streaming (analytics over RTP)                 | Conditional | **Complete** | `metadata.go` - `ParseMetadataFrame()` with object detection extraction |
| Analytics configuration in Media2 profiles              | Conditional | **Missing**  | Analytics config not managed via Media2 profile `AddConfiguration`      |

---

## 12. On-Screen Display (OSD) - Conditional

| Feature         | Requirement | Status      | Notes           |
| --------------- | ----------- | ----------- | --------------- |
| `GetOSDs`       | Conditional | **Missing** | Not implemented |
| `SetOSD`        | Conditional | **Missing** | Not implemented |
| `CreateOSD`     | Conditional | **Missing** | Not implemented |
| `DeleteOSD`     | Conditional | **Missing** | Not implemented |
| `GetOSDOptions` | Conditional | **Missing** | Not implemented |

---

## Priority Recommendations

### High Priority (Mandatory features with gaps)

1. **Media2 profile mutation** - `CreateProfile`, `DeleteProfile`, `AddConfiguration`, `RemoveConfiguration` should use Media2 WSDL in addition to Media1 fallback
2. **Media2 video source config** - `GetVideoSourceConfigurations`, `SetVideoSourceConfiguration`, `GetVideoSourceConfigurationOptions` via Media2
3. **`GetEventProperties`** - required to enumerate available event topics from a device
4. **`GetServices` / `GetServiceCapabilities`** - Profile T mandates these for capability negotiation

### Medium Priority (Mandatory features less critical for NVR client)

5. **`SetSystemDateAndTime`** - useful for time-sync with cameras
6. **`GetNetworkDefaultGateway`** - required by spec, useful for network diagnostics UI
7. **`SetScopes` / `GetDiscoveryMode` / `SetDiscoveryMode`** - device management completeness
8. **HTTPS/TLS for SOAP** - security hardening for production deployments

### Low Priority (Conditional features)

9. **OSD management** - text/image overlay control (`GetOSDs`, `SetOSD`, `CreateOSD`, `DeleteOSD`)
10. **Audio backchannel** - bidirectional audio stream control beyond capability detection
11. **Imaging options and focus** - `GetOptions`, `Move`, `Stop` for focus motor
12. **RTP multicast** - niche requirement, most NVR deployments use unicast

---

## Additional Notes

### Strengths of Current Implementation

- **Robust event handling**: Push-first with automatic pull-point fallback and exponential backoff reconnection
- **Multi-auth snapshot capture**: Four-layer authentication fallback (URL-embedded, Basic, Digest, None)
- **Complete PTZ support**: All 12 PTZ operations including presets, home position, and status
- **Edge recording integration**: Full recording search, control, and replay via ONVIF services
- **Custom SOAP for Media2**: Overcomes onvif-go library limitations for v2 profiles
- **Security at rest**: AES-256 encrypted credential storage in SQLite

### Architecture Consideration

The implementation is an ONVIF **client** (NVR consuming cameras), not a device. Profile T compliance for a client means being able to **consume** all mandatory features that Profile T devices expose. The gaps identified above represent operations that Profile T devices are required to support but that this client does not yet call.
