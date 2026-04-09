# KAI-27: ONVIF Backchannel Audio Design Spec

## Overview

Implement two-way audio (intercom) for ONVIF cameras. The Flutter client sends audio to the NVR backend over WebSocket, and the NVR relays it to the camera via RTSP backchannel. Half-duplex (push-to-talk) initially, with architecture supporting full-duplex later.

## Architecture

```
Flutter Client                NVR Backend                  ONVIF Camera
┌──────────┐    WebSocket    ┌──────────────┐    RTSP     ┌──────────┐
│ Mic/PTT  │───────────────▶│ Backchannel  │──────────▶│ Speaker  │
│ (encoded │  binary audio  │ Manager      │  RTP pkts  │          │
│  frames) │                │              │            │          │
│          │◀───────────────│              │            │          │
│          │  JSON control  │              │            │          │
└──────────┘                └──────────────┘            └──────────┘
```

**Key decisions:**

- Audio proxied through NVR (not direct client-to-camera) for remote access and credential isolation
- WebSocket transport from Flutter to NVR for low-latency bidirectional communication
- Auto-negotiate codec with camera (prefer AAC, fall back to G.711)
- On-demand RTSP connections with 30s keep-alive window after session ends

## ONVIF Layer

### New File: `internal/nvr/onvif/backchannel.go`

**Types:**

```go
type AudioOutputConfig struct {
    Token       string `json:"token"`
    Name        string `json:"name"`
    OutputToken string `json:"output_token"`
}

type AudioDecoderConfig struct {
    Token string `json:"token"`
    Name  string `json:"name"`
}

type AudioDecoderOptions struct {
    AACSupported  bool          `json:"aac_supported"`
    G711Supported bool          `json:"g711_supported"`
    AAC           *CodecOptions `json:"aac,omitempty"`
    G711          *CodecOptions `json:"g711,omitempty"`
}

type CodecOptions struct {
    Bitrates    []int `json:"bitrates"`
    SampleRates []int `json:"sample_rates"`
}

type BackchannelCodec struct {
    Encoding   string `json:"encoding"`   // "AAC" or "G711"
    Bitrate    int    `json:"bitrate"`
    SampleRate int    `json:"sample_rate"`
}
```

**Functions:**

| Function                                                                 | Purpose                                                           |
| ------------------------------------------------------------------------ | ----------------------------------------------------------------- |
| `GetAudioOutputs(xaddr, user, pass)`                                     | List audio output tokens from camera                              |
| `GetAudioOutputConfigurations(xaddr, user, pass)`                        | List audio output configurations                                  |
| `SetAudioOutputConfiguration(xaddr, user, pass, cfg)`                    | Update an audio output configuration                              |
| `GetAudioDecoderConfigurations(xaddr, user, pass)`                       | List audio decoder configurations                                 |
| `SetAudioDecoderConfiguration(xaddr, user, pass, cfg)`                   | Update an audio decoder configuration                             |
| `GetAudioDecoderOptions(xaddr, user, pass, token)`                       | Query supported codecs, bitrates, sample rates                    |
| `NegotiateBackchannelCodec(xaddr, user, pass)`                           | Auto-select best codec (AAC > G.711) based on camera capabilities |
| `GetBackchannelStreamURI(xaddr, user, pass, profileToken)`               | Custom SOAP call for backchannel RTSP URI                         |
| `AddAudioOutputToProfile(xaddr, user, pass, profileToken, configToken)`  | Attach audio output config to media profile                       |
| `AddAudioDecoderToProfile(xaddr, user, pass, profileToken, configToken)` | Attach audio decoder config to media profile                      |

**Custom SOAP for GetBackchannelStreamURI:** The onvif-go library's `GetStreamUri` does not support backchannel parameters. A custom SOAP request is needed, following the existing pattern in `media2.go`. The request includes `<StreamType>RTP-Unicast</StreamType>` with backchannel transport per the ONVIF streaming specification.

**Update to `audio.go`:** `GetAudioCapabilities` will populate the `AudioBackchannel` field in the `Capabilities` struct (currently always false).

## Backchannel Session Manager

### New Package: `internal/nvr/backchannel/`

#### `manager.go` — Session Lifecycle

```go
type SessionState int

const (
    StateIdle SessionState = iota
    StateConnecting
    StateActive
    StateClosing
)

type Session struct {
    CameraID   string
    State      SessionState
    Codec      string        // negotiated: "AAC" or "G711"
    SampleRate int
    Bitrate    int
    RTSPConn   *RTSPBackchannelConn
    IdleTimer  *time.Timer   // 30s keep-alive after stop
    mu         sync.Mutex
}

type Manager struct {
    sessions   map[string]*Session  // cameraID -> session
    mu         sync.RWMutex
    onvifCreds func(cameraID string) (xaddr, user, pass string, err error)
}
```

**Manager methods:**

| Method                           | Behavior                                                                                                                                 |
| -------------------------------- | ---------------------------------------------------------------------------------------------------------------------------------------- |
| `NewManager(credentialFunc)`     | Constructor; takes function to look up decrypted ONVIF credentials by camera ID                                                          |
| `StartSession(ctx, cameraID)`    | Negotiate codec, establish RTSP backchannel (or reuse idle connection). Returns `BackchannelCodec` so client knows what format to encode |
| `SendAudio(cameraID, audioData)` | Forward audio frames to camera via RTP over the active RTSP connection                                                                   |
| `StopSession(cameraID)`          | Transition to idle, start 30s teardown timer                                                                                             |
| `CloseAll()`                     | Graceful shutdown — tear down all RTSP connections                                                                                       |

**Concurrency rules:**

- One active session per camera. Second client gets `"camera busy"` error.
- When `StopSession` is called, RTSP connection stays open for 30s. If `StartSession` is called again within that window, it reuses the connection and skips codec negotiation.
- On RTSP connection failure, session is cleaned up; next `StartSession` retries from scratch.

#### `rtsp.go` — RTSP Backchannel Connection

Handles RTSP session for sending audio to the camera:

- RTSP DESCRIBE, SETUP (with backchannel transport), PLAY
- Sends RTP audio packets over the established interleaved TCP channel
- RTSP OPTIONS keep-alive pings to prevent timeout
- Connection error detection and cleanup

#### `rtp.go` — RTP Audio Packet Construction

- Pack audio frames into RTP packets with correct payload type:
  - PT 0: G.711 mu-law
  - PT 8: G.711 A-law
  - PT 96+: AAC (dynamic)
- Manage sequence numbers and timestamps
- Handle RTP header construction per RFC 3550

## WebSocket API

### Endpoint: `ws://.../api/cameras/:id/audio/backchannel`

Protected by existing JWT middleware.

**Client → Server:**

| Message          | Format                     | Purpose                                  |
| ---------------- | -------------------------- | ---------------------------------------- |
| Start session    | Text: `{"type":"start"}`   | Begin push-to-talk                       |
| Audio data       | Binary: raw encoded frames | Audio in negotiated codec                |
| Stop session     | Text: `{"type":"stop"}`    | End push-to-talk (connection stays open) |
| Connection close | —                          | Implicitly stops session                 |

**Server → Client:**

| Message         | Format                                                                         | Purpose                            |
| --------------- | ------------------------------------------------------------------------------ | ---------------------------------- |
| Session started | `{"type":"session_started","codec":"G711","sample_rate":8000,"bitrate":64000}` | Tells client what format to encode |
| Session stopped | `{"type":"session_stopped"}`                                                   | Confirms stop                      |
| Error           | `{"type":"error","message":"..."}`                                             | Error conditions                   |

**Error conditions:** `"backchannel not supported"`, `"camera busy"`, `"connection failed"`, `"connection lost"`

## REST API Endpoints

All under `/api/cameras/:id/audio/` prefix, JWT-protected.

| Method | Path                      | Purpose                                                                  |
| ------ | ------------------------- | ------------------------------------------------------------------------ |
| GET    | `/outputs`                | List audio outputs                                                       |
| GET    | `/output-configs`         | List audio output configurations                                         |
| PUT    | `/output-configs/:token`  | Update audio output configuration                                        |
| GET    | `/decoder-configs`        | List audio decoder configurations                                        |
| PUT    | `/decoder-configs/:token` | Update audio decoder configuration                                       |
| GET    | `/decoder-options/:token` | Get supported codecs/bitrates/sample rates                               |
| GET    | `/backchannel/info`       | Get backchannel capability + negotiated codec without starting a session |

### Handler: `internal/nvr/api/backchannel.go`

`BackchannelHandler` struct holds:

- Reference to `backchannel.Manager`
- Credential decryption function
- Database access for camera lookup

## Integration & Lifecycle

**NVR startup** (`internal/nvr/nvr.go`):

1. Create `backchannel.Manager` with credential lookup function
2. Create `BackchannelHandler` with manager reference
3. Register WebSocket and REST routes on the Gin router
4. On shutdown, call `manager.CloseAll()`

**No database changes required.** `Camera.SupportsAudioBackchannel` already exists. Backchannel sessions are transient.

## File Summary

| File                                  | Purpose                                                                 |
| ------------------------------------- | ----------------------------------------------------------------------- |
| `internal/nvr/onvif/backchannel.go`   | ONVIF audio output/decoder operations + custom SOAP for backchannel URI |
| `internal/nvr/onvif/audio.go`         | Updated: populate `AudioBackchannel` capability                         |
| `internal/nvr/backchannel/manager.go` | Session lifecycle management (start, send, stop, keep-alive)            |
| `internal/nvr/backchannel/rtsp.go`    | RTSP backchannel connection handling                                    |
| `internal/nvr/backchannel/rtp.go`     | RTP audio packet construction                                           |
| `internal/nvr/api/backchannel.go`     | WebSocket + REST API handlers                                           |
| `internal/nvr/nvr.go`                 | Updated: initialize manager, register routes, shutdown cleanup          |

## Future Work (Not In Scope)

- Full-duplex mode: remove "mute incoming while sending" gate on client side (backend already supports it)
- Echo cancellation (Flutter client responsibility)
- Multiple simultaneous talkers to same camera
- Audio recording/logging of backchannel sessions
