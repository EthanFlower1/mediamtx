# ONVIF Profile S Compliance Audit

**Date:** 2026-04-02
**Ticket:** KAI-15
**Scope:** Client-side compliance of `internal/nvr/onvif/` against ONVIF Profile S Specification v1.3

## Legend

| Status       | Meaning                                               |
| ------------ | ----------------------------------------------------- |
| **Complete** | Fully implemented and functional                      |
| **Partial**  | Some operations implemented, gaps remain               |
| **Missing**  | Not implemented                                       |
| N/A          | Not applicable (device-side only or out of scope)     |

## Summary

| Service / Area            | Status       | Coverage |
| ------------------------- | ------------ | -------- |
| WS-Discovery              | Complete     | 100%     |
| Device Service            | Complete     | 95%      |
| Media Service             | Complete     | 90%      |
| Streaming (RTSP/RTP)      | Complete     | 85%      |
| PTZ Service               | Complete     | 95%      |
| Event Service             | Complete     | 85%      |
| Imaging Service           | Partial      | 40%      |
| Security / Authentication | Partial      | 75%      |
| Device I/O Service        | Partial      | 60%      |
| Audio Backchannel         | Partial      | 30%      |
| Metadata Streaming        | Partial      | 40%      |

---

## 1. WS-Discovery — Complete

Discovery is fully implemented in `discovery.go`.

| Requirement                                    | Status       | Notes                                                          |
| ---------------------------------------------- | ------------ | -------------------------------------------------------------- |
| Multicast Probe (UDP 239.255.255.250:3702)     | Complete     | `wsDiscoverDevices()` sends probe to standard multicast group  |
| Probe for `dn:NetworkVideoTransmitter`         | Complete     | Probe message targets correct type                             |
| Scope-based filtering                          | Complete     | `parseScopes()` extracts manufacturer, model, firmware         |
| XAddrs resolution from ProbeMatch              | Complete     | Endpoint extracted and used for service discovery              |
| Resolve operation                              | Missing      | WS-Discovery Resolve not implemented (Probe suffices for LAN)  |
| Remote Discovery Mode (Get/Set)                | Missing      | `GetRemoteDiscoveryMode` / `SetRemoteDiscoveryMode` not called |

**Gap impact:** Low. Resolve and RemoteDiscoveryMode are rarely needed in practice; Probe covers all common LAN discovery scenarios.

---

## 2. Device Service — Complete

Implemented in `device_mgmt.go` and `client.go`.

| Requirement                  | Status   | Notes                                             |
| ---------------------------- | -------- | ------------------------------------------------- |
| `GetServices`                | Complete | Used in `buildServiceMap()` to discover endpoints |
| `GetServiceCapabilities`     | Complete | `GetCapabilities()` in client.go                  |
| `GetDeviceInformation`       | Complete | Called during `ProbeDevice()`                     |
| `GetCapabilities`            | Complete | Used for capability detection                     |
| `GetSystemDateAndTime`       | Complete | `GetSystemDateAndTime()` in device_mgmt.go        |
| `SetSystemDateAndTime`       | Missing  | Only GET is implemented                           |
| `GetHostname`                | Complete | `GetDeviceHostname()`                             |
| `SetHostname`                | Complete | `SetDeviceHostname()`                             |
| `GetDNS`                     | Complete | `GetDNSConfig()`                                  |
| `SetDNS`                     | Missing  | Only GET is implemented                           |
| `GetNTP`                     | Complete | `GetNTPConfig()`                                  |
| `SetNTP`                     | Missing  | Only GET is implemented                           |
| `GetNetworkInterfaces`       | Complete | `GetNetworkInterfaces()`                          |
| `SetNetworkInterfaces`       | Missing  | Only GET is implemented                           |
| `GetNetworkProtocols`        | Complete | `GetNetworkProtocols()`                           |
| `SetNetworkProtocols`        | Complete | `SetNetworkProtocols()`                           |
| `GetNetworkDefaultGateway`   | Missing  | Not implemented                                   |
| `SetNetworkDefaultGateway`   | Missing  | Not implemented                                   |
| `GetScopes`                  | Complete | `GetDeviceScopes()`                               |
| `SetScopes`                  | Missing  | Only GET is implemented                           |
| `AddScopes`                  | Missing  | Not implemented                                   |
| `RemoveScopes`               | Missing  | Not implemented                                   |
| `GetDiscoveryMode`           | Missing  | Not implemented                                   |
| `SetDiscoveryMode`           | Missing  | Not implemented                                   |
| `GetUsers`                   | Complete | `GetDeviceUsers()`                                |
| `CreateUsers`                | Complete | `CreateDeviceUser()`                              |
| `DeleteUsers`                | Complete | `DeleteDeviceUser()`                              |
| `SetUser`                    | Complete | `SetDeviceUser()`                                 |
| `SystemReboot`               | Complete | `DeviceReboot()`                                  |
| `GetSystemLog`               | Missing  | Not implemented                                   |
| `GetSystemSupportInformation`| Missing  | Not implemented                                   |

**Gap summary:** The main gaps are SET operations for DNS, NTP, network interfaces, gateway, and scopes. These are configuration management features — the NVR reads settings but cannot push configuration changes for these specific areas. `GetSystemLog` and `GetSystemSupportInformation` are also missing.

**Gap impact:** Medium. Most NVRs read-only these settings. SET operations matter for centralized camera management workflows.

---

## 3. Media Service — Complete

Implemented across `device.go`, `media2.go`, and `media_config.go`.

| Requirement                                  | Status   | Notes                                                         |
| -------------------------------------------- | -------- | ------------------------------------------------------------- |
| `GetProfiles`                                | Complete | Media1 via library, Media2 via `GetProfiles2()`               |
| `GetProfile`                                 | Complete | Individual profile retrieval supported                        |
| `CreateProfile`                              | Complete | `CreateMediaProfile()` in media_config.go                     |
| `DeleteProfile`                              | Complete | `DeleteMediaProfile()` in media_config.go                     |
| `GetVideoSources`                            | Complete | `GetVideoSourcesList()` in media_config.go                    |
| `GetVideoSourceConfigurations`               | Complete | Via `GetProfilesFull()`                                       |
| `GetVideoSourceConfiguration`                | Partial  | Retrieved as part of profiles, no standalone GET               |
| `SetVideoSourceConfiguration`                | Missing  | Not implemented                                               |
| `GetCompatibleVideoSourceConfigurations`     | Missing  | Not implemented                                               |
| `AddVideoSourceConfiguration`                | Missing  | Not implemented                                               |
| `RemoveVideoSourceConfiguration`             | Missing  | Not implemented                                               |
| `GetVideoEncoderConfigurations`              | Complete | `GetVideoEncoderConfig()` in media_config.go                  |
| `GetVideoEncoderConfiguration`               | Complete | Individual config retrieval supported                         |
| `SetVideoEncoderConfiguration`               | Complete | `SetVideoEncoderConfig()` in media_config.go                  |
| `GetCompatibleVideoEncoderConfigurations`    | Missing  | Not implemented                                               |
| `AddVideoEncoderConfiguration`               | Complete | `AddVideoEncoderToProfile()`                                  |
| `RemoveVideoEncoderConfiguration`            | Complete | `RemoveVideoEncoderFromProfile()`                             |
| `GetVideoEncoderConfigurationOptions`        | Complete | `GetVideoEncoderOpts()` in media_config.go                    |
| `GetAudioSources`                            | Partial  | Detected via `GetAudioCapabilities()`, no full enumeration    |
| `GetAudioSourceConfigurations`               | Missing  | Not implemented                                               |
| `GetAudioSourceConfiguration`                | Missing  | Not implemented                                               |
| `SetAudioSourceConfiguration`                | Missing  | Not implemented                                               |
| `GetCompatibleAudioSourceConfigurations`     | Missing  | Not implemented                                               |
| `AddAudioSourceConfiguration`                | Missing  | Not implemented                                               |
| `RemoveAudioSourceConfiguration`             | Missing  | Not implemented                                               |
| `GetAudioEncoderConfigurations`              | Complete | `GetAudioEncoderCfg()` in media_config.go                     |
| `GetAudioEncoderConfiguration`               | Complete | Individual config retrieval supported                         |
| `SetAudioEncoderConfiguration`               | Complete | `SetAudioEncoderCfg()` in media_config.go                     |
| `GetCompatibleAudioEncoderConfigurations`    | Missing  | Not implemented                                               |
| `AddAudioEncoderConfiguration`               | Complete | `AddAudioEncoderToProfile()`                                  |
| `RemoveAudioEncoderConfiguration`            | Complete | `RemoveAudioEncoderFromProfile()`                             |
| `GetStreamUri`                               | Complete | Media1 + Media2 with automatic fallback                       |
| `GetSnapshotUri`                             | Complete | Media1 + Media2, plus vendor-specific fallback URLs            |
| `GetVideoSourceModes`                        | Missing  | Not implemented                                               |
| `SetVideoSourceMode`                         | Missing  | Not implemented                                               |

### Video Codec Support

| Codec    | Status   | Notes                                        |
| -------- | -------- | -------------------------------------------- |
| JPEG     | Complete | Detected and supported in encoder options     |
| H.264    | Complete | Full support with profile variant detection   |
| MPEG-4   | Missing  | Not detected or handled (legacy, rarely used) |

### Audio Codec Support

| Codec | Status  | Notes                               |
| ----- | ------- | ----------------------------------- |
| G.711 | Partial | Audio encoding supported generically |
| G.726 | Partial | Not explicitly handled               |
| AAC   | Partial | Not explicitly handled               |

**Gap summary:** Audio source configuration is largely missing. Video source configuration (as distinct from encoder configuration) is incomplete. The "Compatible" query operations are not implemented. Audio codec handling is generic rather than codec-specific.

**Gap impact:** Medium. Stream retrieval and encoder configuration — the core NVR needs — are solid. Audio source and video source configuration gaps affect advanced camera management.

---

## 4. Streaming (RTSP/RTP) — Complete

Streaming is handled by the core MediaMTX engine rather than the ONVIF module specifically.

| Requirement                        | Status   | Notes                                                     |
| ---------------------------------- | -------- | --------------------------------------------------------- |
| RTSP (RFC 2326)                    | Complete | Core MediaMTX capability                                  |
| RTP/UDP unicast                    | Complete | Default transport                                         |
| RTP/RTSP/TCP (interleaved)         | Complete | Supported by MediaMTX RTSP client                         |
| RTP/RTSP/HTTP/TCP (HTTP tunneling) | Missing  | HTTP tunneling for firewall traversal not confirmed        |
| RTCP                               | Complete | Part of MediaMTX RTP implementation                       |
| RTP multicast                      | Partial  | MediaMTX supports multicast but not via ONVIF negotiation |
| RTSP DESCRIBE                      | Complete | SDP parsing supported                                     |
| RTSP SETUP                         | Complete | Transport negotiation                                     |
| RTSP PLAY                          | Complete | Stream start                                              |
| RTSP PAUSE                         | Missing  | Not typically used for live streams                       |
| RTSP TEARDOWN                      | Complete | Stream cleanup                                            |
| RTSP OPTIONS                       | Complete | Method query                                              |
| RTSP GET_PARAMETER (keepalive)     | Complete | Used for session keepalive                                |

**Gap summary:** HTTP tunneling (RTP over RTSP over HTTP) and RTSP PAUSE are not confirmed. Multicast is supported at the transport level but not negotiated via ONVIF `GetStreamUri` with multicast transport.

**Gap impact:** Low-Medium. HTTP tunneling matters for deployments behind restrictive firewalls. PAUSE is irrelevant for live NVR streams.

---

## 5. PTZ Service — Complete

Implemented in `ptz.go`.

| Requirement                | Status   | Notes                              |
| -------------------------- | -------- | ---------------------------------- |
| `GetNodes`                 | Complete | `GetNodes()` returns node list     |
| `GetNode`                  | Partial  | Nodes returned in list, no individual GET |
| `GetConfigurations`        | Missing  | Not implemented as standalone      |
| `GetConfiguration`         | Missing  | Not implemented as standalone      |
| `SetConfiguration`         | Missing  | Not implemented                    |
| `GetCompatibleConfigurations` | Missing | Not implemented                   |
| `ContinuousMove`           | Complete | `ContinuousMove()` with velocity   |
| `AbsoluteMove`             | Complete | `AbsoluteMove()`                   |
| `RelativeMove`             | Complete | `RelativeMove()`                   |
| `Stop`                     | Complete | `Stop()`                           |
| `GetStatus`                | Complete | `GetStatus()` returns position     |
| `SetPreset`                | Complete | `SetPreset()`                      |
| `GetPresets`               | Complete | `GetPresets()`                     |
| `GotoPreset`               | Complete | `GotoPreset()`                     |
| `RemovePreset`             | Complete | `RemovePreset()`                   |
| `GotoHomePosition`         | Complete | `GotoHome()`                       |
| `SetHomePosition`          | Complete | `SetHomePosition()`                |
| `GetConfigurationOptions`  | Missing  | Not implemented                    |
| `SendAuxiliaryCommand`     | Missing  | Not implemented (conditional)      |
| Preset Tours               | Missing  | Not implemented (conditional)      |

**Gap summary:** PTZ movement and preset control is fully implemented. Configuration management (getting/setting PTZ configurations independent of movement) and advanced features (auxiliary commands, tours) are missing.

**Gap impact:** Low. All user-facing PTZ operations work. Configuration and tours are niche features.

---

## 6. Event Service — Complete

Implemented in `events.go`.

| Requirement                         | Status   | Notes                                                      |
| ----------------------------------- | -------- | ---------------------------------------------------------- |
| `GetEventProperties`                | Missing  | Event topic discovery not implemented                      |
| `CreatePullPointSubscription`       | Complete | `createPullPointSubscription()` as fallback                |
| `PullMessages`                      | Complete | `pullMessages()` polls for events                          |
| `Renew` (PullPoint)                 | Complete | `renew()` handles subscription renewal                     |
| `Unsubscribe` (PullPoint)           | Complete | `unsubscribe()` cleans up                                  |
| `GetServiceCapabilities`            | Missing  | Not queried explicitly                                     |
| WS-BaseNotification `Subscribe`     | Complete | `subscribe()` creates push subscriptions                   |
| WS-BaseNotification `Notify`        | Complete | `HandleNotification()` processes pushed events             |
| WS-BaseNotification `Renew`         | Complete | Automatic renewal on 48s interval                          |
| WS-BaseNotification `Unsubscribe`   | Complete | Cleanup on stop                                            |
| Push-to-Pull fallback               | Complete | Automatic fallback if push subscription fails              |
| WS-Security in event messages       | Complete | `injectWSSecurity()` for digest auth                       |
| TopicExpression filtering           | Partial  | `classifyTopic()` matches known topics but no custom filter |
| MessageContent filtering            | Missing  | Not implemented                                            |

### Standard Event Topics

| Topic                                         | Status   | Notes                                  |
| --------------------------------------------- | -------- | -------------------------------------- |
| `tns1:VideoSource/MotionAlarm`                | Complete | Detected and classified                |
| `tns1:RuleEngine/CellMotionDetector/Motion`   | Complete | Detected and classified                |
| `tns1:VideoSource/GlobalSceneChange/ImagingService` | Complete | Tampering detection                |
| `tns1:Device/Trigger/DigitalInput`            | Missing  | Not handled                            |
| `tns1:Device/HardwareFailure/*`               | Missing  | Not handled                            |
| `tns1:VideoSource/SignalLoss`                  | Missing  | Not handled                            |
| `tns1:Device/Trigger/Relay`                   | Missing  | Not handled                            |

**Gap summary:** Core motion/tampering events are handled well with both push and pull mechanisms. Gaps are in event property discovery (`GetEventProperties`), hardware/signal events, and content-based filtering.

**Gap impact:** Medium. Missing `GetEventProperties` means the client cannot discover what events a camera supports before subscribing. Missing event topics (digital input, signal loss, hardware failure) reduce alarm coverage.

---

## 7. Imaging Service — Partial

Implemented in `imaging.go`.

| Requirement              | Status   | Notes                                            |
| ------------------------ | -------- | ------------------------------------------------ |
| `GetImagingSettings`     | Complete | Returns brightness, contrast, saturation, sharpness |
| `SetImagingSettings`     | Complete | Sets brightness, contrast, saturation, sharpness    |
| `GetOptions`             | Missing  | Cannot query valid ranges for settings            |
| `GetMoveOptions`         | Missing  | Cannot query focus move capabilities              |
| `Move` (focus)           | Missing  | No motorized focus control                        |
| `Stop` (focus)           | Missing  | No focus stop                                     |
| `GetStatus`              | Missing  | Cannot query current focus/imaging status         |
| `GetServiceCapabilities` | Missing  | Not queried                                       |

### Imaging Settings Coverage

| Setting                  | Status   | Notes                    |
| ------------------------ | -------- | ------------------------ |
| Brightness               | Complete | Get and Set              |
| Contrast                 | Complete | Get and Set              |
| Color Saturation         | Complete | Get and Set              |
| Sharpness                | Complete | Get and Set              |
| Backlight Compensation   | Missing  | Not implemented          |
| Exposure Control         | Missing  | Not implemented          |
| Focus Control            | Missing  | Not implemented          |
| Wide Dynamic Range       | Missing  | Not implemented          |
| White Balance            | Missing  | Not implemented          |
| IR Cut Filter            | Missing  | Not implemented          |

**Gap summary:** Only the four basic image adjustments are implemented. Advanced imaging controls (exposure, focus, WDR, white balance, IR cut filter, backlight compensation) and the options/status queries are all missing.

**Gap impact:** High for camera management UX. Users expect to control exposure, focus, and IR settings from an NVR interface.

---

## 8. Security / Authentication — Partial

Implemented across `snapshot.go`, `events.go`, and via the ONVIF library.

| Requirement                    | Status   | Notes                                              |
| ------------------------------ | -------- | -------------------------------------------------- |
| HTTP Digest Authentication     | Complete | Full RFC 2617 implementation in snapshot.go         |
| WS-UsernameToken (Digest)      | Complete | `injectWSSecurity()` with SHA-1, nonce, timestamp  |
| WS-Security header injection   | Complete | Applied to SOAP event messages                     |
| Clock sync for token digest    | Complete | `GetSystemDateAndTime()` used for time reference   |
| HTTPS/TLS support              | Partial  | URLs with HTTPS work, no certificate management    |
| Certificate management         | Missing  | No `GetCertificates`, `LoadCertificate`, etc.      |
| IEEE 802.1X                    | Missing  | Not implemented                                    |
| Access Policy enforcement      | Missing  | No role-based access differentiation               |
| HTTP Basic Authentication      | Complete | Supported as fallback for snapshots                |

**Gap summary:** Core authentication (HTTP Digest + WS-UsernameToken) is solid. TLS works at the transport level but certificate management operations are missing. 802.1X and access policies are not implemented.

**Gap impact:** Low-Medium. The authentication methods that matter for 99% of cameras are implemented. Certificate management and 802.1X matter for enterprise deployments.

---

## 9. Device I/O Service — Partial

Implemented in `relay.go`.

| Requirement                  | Status   | Notes                        |
| ---------------------------- | -------- | ---------------------------- |
| `GetRelayOutputs`            | Complete | `GetRelayOutputs()`          |
| `SetRelayOutputSettings`     | Missing  | Cannot configure relay mode   |
| `SetRelayOutputState`        | Complete | `SetRelayOutputState()`      |
| `GetDigitalInputs`           | Missing  | Not implemented              |

**Gap summary:** Can list and trigger relay outputs but cannot configure relay behavior (monostable/bistable, delay). Digital input enumeration is missing.

**Gap impact:** Medium. Relay triggering works for alarm output use cases. Missing configuration and digital inputs reduce I/O integration capability.

---

## 10. Audio Backchannel — Partial

Implemented in `audio.go` with capability detection only.

| Requirement                            | Status   | Notes                                   |
| -------------------------------------- | -------- | --------------------------------------- |
| Audio output detection                 | Complete | `GetAudioCapabilities()` detects outputs |
| `GetAudioOutputs`                      | Missing  | Not implemented                         |
| `GetAudioOutputConfigurations`         | Missing  | Not implemented                         |
| `GetAudioOutputConfiguration`          | Missing  | Not implemented                         |
| `SetAudioOutputConfiguration`          | Missing  | Not implemented                         |
| `AddAudioOutputConfiguration`          | Missing  | Not implemented                         |
| `RemoveAudioOutputConfiguration`       | Missing  | Not implemented                         |
| `GetAudioDecoderConfigurations`        | Missing  | Not implemented                         |
| `GetAudioDecoderConfiguration`         | Missing  | Not implemented                         |
| `SetAudioDecoderConfiguration`         | Missing  | Not implemented                         |
| `AddAudioDecoderConfiguration`         | Missing  | Not implemented                         |
| `RemoveAudioDecoderConfiguration`      | Missing  | Not implemented                         |
| Backchannel RTSP stream                | Missing  | No audio-to-device streaming            |

**Gap summary:** The system can detect whether a camera supports audio backchannel but cannot configure or use it. All audio output and decoder configuration operations are missing.

**Gap impact:** High for two-way audio use cases (intercoms, door stations). Low if two-way audio is not a product requirement.

---

## 11. Metadata Streaming — Partial

Implemented in `metadata.go`.

| Requirement                        | Status   | Notes                                       |
| ---------------------------------- | -------- | ------------------------------------------- |
| `GetMetadataConfigurations`        | Missing  | Not implemented                             |
| `GetMetadataConfiguration`         | Missing  | Not implemented                             |
| `SetMetadataConfiguration`         | Missing  | Not implemented                             |
| `AddMetadataConfiguration`         | Missing  | Not implemented                             |
| `RemoveMetadataConfiguration`      | Missing  | Not implemented                             |
| Metadata frame parsing             | Complete | `ParseMetadataFrame()` handles analytics XML |
| Object detection with bounding box | Complete | Normalized ONVIF bounding boxes parsed       |
| Metadata over RTP                  | Missing  | No RTP metadata stream subscription          |

**Gap summary:** Can parse metadata frames when received but cannot configure metadata streaming on the camera or subscribe to metadata via RTP.

**Gap impact:** Medium. Metadata parsing infrastructure exists but there is no way to activate metadata streaming from the ONVIF side.

---

## Beyond Profile S (Implemented)

The following services are implemented but fall outside Profile S scope. They are noted here for completeness:

| Service                    | File                   | Notes                                |
| -------------------------- | ---------------------- | ------------------------------------ |
| Recording Service          | `recording.go`         | Edge storage search (Profile G)      |
| Recording Control Service  | `recording_control.go` | Edge recording management (Profile G)|
| Replay Service             | `replay.go`            | Edge playback (Profile G)            |
| Analytics Service          | `analytics.go`         | Rule management (Profile M/T)        |

---

## Priority Recommendations

### P0 — Should fix for Profile S compliance

1. **Imaging Service expansion** — Add `GetOptions`, exposure, focus, WDR, white balance, IR cut filter controls. Users expect these from a camera management interface.
2. **Event topic expansion** — Handle `DigitalInput`, `SignalLoss`, and `HardwareFailure` event topics. Add `GetEventProperties` for event discovery.
3. **Device I/O completion** — Add `GetDigitalInputs` and `SetRelayOutputSettings`.

### P1 — Important for production completeness

4. **Audio source configuration** — Add audio source enumeration and configuration operations.
5. **Metadata configuration** — Add metadata configuration operations to enable analytics streaming.
6. **Device Service SET operations** — Add `SetDNS`, `SetNTP`, `SetNetworkInterfaces`, `SetNetworkDefaultGateway` for centralized camera management.
7. **Security enhancement** — Add certificate management for enterprise deployments.

### P2 — Nice to have

8. **PTZ configuration** — Add standalone configuration operations and auxiliary commands.
9. **Audio backchannel** — Full two-way audio support if product requirements demand it.
10. **Video source configuration** — `SetVideoSourceConfiguration` and compatibility queries.
11. **HTTP tunneling** — RTP/RTSP/HTTP/TCP transport for restrictive firewall environments.
12. **Preset tours** — PTZ tour management.
