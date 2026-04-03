# KAI-17: Complete ONVIF Media Profile Management

## Summary

Add Media2 profile CRUD, video source configuration, and audio source configuration operations to the ONVIF subsystem. Expose all via new NVR API endpoints under `/media2/` prefix.

## Current State

- `media_config.go`: Media1 profile CRUD via onvif-go library, video/audio encoder get/set, add/remove encoder to profile
- `media2.go`: Media2 read-only operations (GetProfiles, GetStreamUri, GetSnapshotUri) using custom SOAP via `doMedia2SOAP`
- 7 API endpoints under `/cameras/:id/media/` for Media1 operations

## New Operations

### 1. Media2 Profile Operations (`media2.go`)

Using existing `doMedia2SOAP` + new XML response types:

- **CreateProfile2(client, name)**: `tr2:CreateProfile` - creates a new profile via Media2
- **DeleteProfile2(client, token)**: `tr2:DeleteProfile` - deletes a profile via Media2
- **AddConfiguration2(client, profileToken, configType, configToken)**: `tr2:AddConfiguration` - adds a configuration (VideoSource, VideoEncoder, AudioSource, AudioEncoder, PTZ, Analytics, Metadata) to a profile
- **RemoveConfiguration2(client, profileToken, configType, configToken)**: `tr2:RemoveConfiguration` - removes a configuration from a profile

### 2. Video Source Configuration via Media2 (`media2.go`)

- **GetVideoSourceConfigurations2(client)**: `tr2:GetVideoSourceConfigurations` - lists all video source configurations
- **SetVideoSourceConfiguration2(client, config)**: `tr2:SetVideoSourceConfiguration` - updates a video source configuration
- **GetVideoSourceConfigurationOptions2(client, configToken, profileToken)**: `tr2:GetVideoSourceConfigurationOptions` - returns available options for a video source configuration

### 3. Audio Source Configuration via Media2 (`media2.go`)

- **GetAudioSourceConfigurations2(client)**: `tr2:GetAudioSourceConfigurations` - lists all audio source configurations
- **SetAudioSourceConfiguration2(client, config)**: `tr2:SetAudioSourceConfiguration` - updates an audio source configuration

### 4. Public Wrapper Functions (`media_config.go`)

Each wraps the Media2 function with the standard pattern: create client from xaddr/username/password, call the Media2 function, return result. These are what API handlers call.

### 5. New API Endpoints

All under the authenticated `/api/nvr` group, prefixed with `/cameras/:id/media2/`:

| Method | Path | Handler | Description |
|--------|------|---------|-------------|
| POST | `/cameras/:id/media2/profiles` | CreateMedia2Profile | Create profile via Media2 |
| DELETE | `/cameras/:id/media2/profiles/:token` | DeleteMedia2Profile | Delete profile via Media2 |
| POST | `/cameras/:id/media2/profiles/:token/configurations` | AddMedia2Configuration | Add config to profile |
| DELETE | `/cameras/:id/media2/profiles/:token/configurations` | RemoveMedia2Configuration | Remove config from profile |
| GET | `/cameras/:id/media2/video-source-configs` | GetVideoSourceConfigs | List video source configs |
| PUT | `/cameras/:id/media2/video-source-configs/:token` | SetVideoSourceConfig | Update video source config |
| GET | `/cameras/:id/media2/video-source-configs/:token/options` | GetVideoSourceConfigOptions | Get video source config options |
| GET | `/cameras/:id/media2/audio-source-configs` | GetAudioSourceConfigs | List audio source configs |
| PUT | `/cameras/:id/media2/audio-source-configs/:token` | SetAudioSourceConfig | Update audio source config |

## Data Types

### VideoSourceConfig
```go
type VideoSourceConfig struct {
    Token       string             `json:"token"`
    Name        string             `json:"name"`
    SourceToken string             `json:"source_token"`
    Bounds      *IntRectangle      `json:"bounds,omitempty"`
}

type IntRectangle struct {
    X      int `json:"x"`
    Y      int `json:"y"`
    Width  int `json:"width"`
    Height int `json:"height"`
}
```

### VideoSourceConfigOptions
```go
type VideoSourceConfigOptions struct {
    BoundsRange              *IntRectangleRange `json:"bounds_range,omitempty"`
    MaximumNumberOfProfiles  int                `json:"maximum_number_of_profiles,omitempty"`
}

type IntRectangleRange struct {
    XRange      Range `json:"x_range"`
    YRange      Range `json:"y_range"`
    WidthRange  Range `json:"width_range"`
    HeightRange Range `json:"height_range"`
}
```

### AudioSourceConfig
```go
type AudioSourceConfig struct {
    Token       string `json:"token"`
    Name        string `json:"name"`
    SourceToken string `json:"source_token"`
}
```

## Design Decisions

- **Separate `/media2/` prefix**: Keeps Media2 endpoints distinct from existing Media1 endpoints. Clients can choose which to use based on camera capabilities.
- **All operations via custom SOAP**: Follows the established `doMedia2SOAP` pattern rather than the onvif-go library, since the library doesn't expose Media2 operations.
- **Configuration type as string**: `AddConfiguration2` and `RemoveConfiguration2` accept a `type` field (e.g., "VideoSource", "AudioEncoder") matching the ONVIF spec's `ConfigurationType` enum.
- **No changes to existing endpoints**: All existing Media1 endpoints remain unchanged.

## Files Modified

1. `internal/nvr/onvif/media2.go` — Add SOAP types and Media2 functions
2. `internal/nvr/onvif/media_config.go` — Add public wrapper functions and new data types
3. `internal/nvr/api/cameras.go` — Add 9 new handler methods
4. `internal/nvr/api/router.go` — Register 9 new routes
