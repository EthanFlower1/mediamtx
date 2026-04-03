# KAI-113: Audio Source Configuration and Enumeration

## Summary

Extend the existing audio capabilities detection in `internal/nvr/onvif/audio.go` to support full audio source configuration and enumeration. Expose via NVR API endpoints under `/cameras/:id/audio/`.

## ONVIF Layer

### New Types (`internal/nvr/onvif/audio.go`)

```go
type AudioSourceInfo struct {
    Token    string `json:"token"`
    Channels int    `json:"channels"`
}

type AudioSourceConfig struct {
    Token       string `json:"token"`
    Name        string `json:"name"`
    UseCount    int    `json:"use_count"`
    SourceToken string `json:"source_token"`
}

type AudioSourceConfigOptions struct {
    InputTokensAvailable []string `json:"input_tokens_available"`
}
```

### New Functions

All functions follow the existing pattern: `NewClient(xaddr, user, pass)` then delegate to `client.Dev.*` library methods, mapping results to local types.

| Function | Library Method | Returns |
|---|---|---|
| `GetAudioSources(xaddr, user, pass)` | `Dev.GetAudioSources` | `[]*AudioSourceInfo` |
| `GetAudioSourceConfigurations(xaddr, user, pass)` | Media2 first via `GetMedia2AudioSourceConfigurations`, fallback Media1 via `Dev.GetAudioSourceConfigurations` | `[]*AudioSourceConfig` |
| `GetAudioSourceConfiguration(xaddr, user, pass, token)` | `Dev.GetAudioSourceConfiguration` | `*AudioSourceConfig` |
| `SetAudioSourceConfiguration(xaddr, user, pass, cfg)` | `Dev.SetAudioSourceConfiguration` (Media1 only) | `error` |
| `GetAudioSourceConfigOptions(xaddr, user, pass, token, profileToken)` | `Dev.GetAudioSourceConfigurationOptions` | `*AudioSourceConfigOptions` |
| `GetCompatibleAudioSourceConfigs(xaddr, user, pass, profileToken)` | `Dev.GetCompatibleAudioSourceConfigurations` | `[]*AudioSourceConfig` |
| `AddAudioSourceToProfile(xaddr, user, pass, profileToken, configToken)` | `Dev.AddAudioSourceConfiguration` (Media1) | `error` |
| `RemoveAudioSourceFromProfile(xaddr, user, pass, profileToken)` | `Dev.RemoveAudioSourceConfiguration` (Media1) | `error` |

### Media2 Strategy

`GetAudioSourceConfigurations` checks `client.HasService("media2")` and uses `GetMedia2AudioSourceConfigurations` when available, falling back to Media1. All mutation operations (Set, Add, Remove) use Media1, which all ONVIF cameras support.

## API Layer

### New Endpoints (`internal/nvr/api/router.go`)

All endpoints are protected (JWT auth required) and registered under the existing `protected` group.

| Method | Path | Handler | Description |
|---|---|---|---|
| GET | `/cameras/:id/audio/sources` | `CameraHandler.AudioSources` | List audio sources (microphones) |
| GET | `/cameras/:id/audio/source-configs` | `CameraHandler.AudioSourceConfigs` | List all audio source configurations |
| GET | `/cameras/:id/audio/source-configs/:token` | `CameraHandler.GetAudioSourceConfig` | Get specific audio source configuration |
| PUT | `/cameras/:id/audio/source-configs/:token` | `CameraHandler.UpdateAudioSourceConfig` | Update audio source configuration |
| GET | `/cameras/:id/audio/source-configs/:token/options` | `CameraHandler.AudioSourceConfigOptions` | Get available options for configuration |
| GET | `/cameras/:id/audio/source-configs/compatible/:profileToken` | `CameraHandler.CompatibleAudioSourceConfigs` | List configs compatible with a profile |
| POST | `/cameras/:id/audio/source-configs/add` | `CameraHandler.AddAudioSourceToProfile` | Add audio source config to profile |
| POST | `/cameras/:id/audio/source-configs/remove` | `CameraHandler.RemoveAudioSourceFromProfile` | Remove audio source config from profile |

### Handler Pattern

All handlers live on `CameraHandler` and follow the standard pattern:

1. Parse camera ID from path
2. Fetch camera from DB
3. Check ONVIF endpoint exists
4. Call `onvif.*` function with decrypted password
5. Return JSON response

### Request/Response Bodies

**PUT `/audio/source-configs/:token`:**
```json
{
    "name": "AudioSourceConfig1",
    "source_token": "AudioSource_1"
}
```
Token comes from the URL path. `use_count` is read-only (managed by device).

**POST `/audio/source-configs/add`:**
```json
{
    "profile_token": "Profile1",
    "config_token": "AudioSourceConfig1"
}
```

**POST `/audio/source-configs/remove`:**
```json
{
    "profile_token": "Profile1"
}
```

## Files Modified

- `internal/nvr/onvif/audio.go` — Add types and 8 new functions
- `internal/nvr/api/cameras.go` — Add 8 handler methods on `CameraHandler`
- `internal/nvr/api/router.go` — Register 8 new routes under `/audio/`

## Out of Scope

- Media2 mutations (Set/Add/Remove via tr2 namespace)
- Audio output configuration (separate feature)
- Audio encoder source configuration (already exists in `media_config.go`)
