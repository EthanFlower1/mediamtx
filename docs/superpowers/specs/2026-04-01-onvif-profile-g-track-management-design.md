# KAI-11: ONVIF Profile G Track Management Design

## Overview

Add track management operations to the existing ONVIF Profile G Recording Control service. Tracks are sub-components of a recording container — each recording can have multiple tracks for video, audio, and metadata streams. This complements KAI-10's recording and job management with fine-grained track-level control.

All three operations use the same `trc:` namespace (`http://www.onvif.org/ver10/recording/wsdl`) and follow the identical SOAP patterns established in KAI-10's `recording_control.go`.

## Operations

| Operation             | SOAP Action                 | Purpose                                         |
| --------------------- | --------------------------- | ----------------------------------------------- |
| CreateTrack           | `trc:CreateTrack`           | Add a video/audio/metadata track to a recording |
| DeleteTrack           | `trc:DeleteTrack`           | Remove a track from a recording                 |
| GetTrackConfiguration | `trc:GetTrackConfiguration` | Get configuration for a specific track          |

## Types

### TrackConfiguration (JSON API type)

```go
type TrackConfiguration struct {
    TrackToken  string `json:"track_token"`
    TrackType   string `json:"track_type"`   // "Video", "Audio", or "Metadata"
    Description string `json:"description"`
}
```

### SOAP XML Types

```go
// Response types for the recordingControlBody union
type createTrackResponse struct {
    TrackToken string `xml:"TrackToken"`
}

type deleteTrackResponse struct{}

type getTrackConfigurationResponse struct {
    TrackConfiguration trackConfigurationXML `xml:"TrackConfiguration"`
}

type trackConfigurationXML struct {
    TrackToken  string `xml:"token,attr"`
    TrackType   string `xml:"TrackType"`
    Description string `xml:"Description"`
}
```

## SOAP Envelopes

### CreateTrack

```xml
<trc:CreateTrack>
  <trc:RecordingToken>{recordingToken}</trc:RecordingToken>
  <trc:TrackConfiguration>
    <tt:TrackType>{Video|Audio|Metadata}</tt:TrackType>
    <tt:Description>{description}</tt:Description>
  </trc:TrackConfiguration>
</trc:CreateTrack>
```

### DeleteTrack

```xml
<trc:DeleteTrack>
  <trc:RecordingToken>{recordingToken}</trc:RecordingToken>
  <trc:TrackToken>{trackToken}</trc:TrackToken>
</trc:DeleteTrack>
```

### GetTrackConfiguration

```xml
<trc:GetTrackConfiguration>
  <trc:RecordingToken>{recordingToken}</trc:RecordingToken>
  <trc:TrackToken>{trackToken}</trc:TrackToken>
</trc:GetTrackConfiguration>
```

## API Endpoints

| Method | Path                                                                  | Handler               |
| ------ | --------------------------------------------------------------------- | --------------------- |
| POST   | `/cameras/:id/recording-control/recordings/:token/tracks`             | CreateTrack           |
| DELETE | `/cameras/:id/recording-control/recordings/:token/tracks/:trackToken` | DeleteTrack           |
| GET    | `/cameras/:id/recording-control/tracks/:trackToken/config`            | GetTrackConfiguration |

### Request/Response Examples

**POST /cameras/:id/recording-control/recordings/:token/tracks**

```json
// Request
{
  "track_type": "Video",
  "description": "Main video track"
}
// Response (201)
{
  "track_token": "Track_001"
}
```

**DELETE /cameras/:id/recording-control/recordings/:token/tracks/:trackToken**

```json
// Response (200)
{
  "message": "track deleted"
}
```

**GET /cameras/:id/recording-control/tracks/:trackToken/config?recording_token=Recording_001**

```json
// Response (200)
{
  "track_token": "Track_001",
  "track_type": "Video",
  "description": "Main video track"
}
```

## File Changes

### Modified Files

| File                                      | Change                                                                                                                                      |
| ----------------------------------------- | ------------------------------------------------------------------------------------------------------------------------------------------- |
| `internal/nvr/onvif/recording_control.go` | Add TrackConfiguration type, 3 XML response types, extend recordingControlBody, add CreateTrack/DeleteTrack/GetTrackConfiguration functions |
| `internal/nvr/api/recording_control.go`   | Add 3 handler methods: CreateEdgeTrack, DeleteEdgeTrack, GetEdgeTrackConfig                                                                 |
| `internal/nvr/api/router.go`              | Register 3 new routes under the recording-control group                                                                                     |

### No New Files

All changes extend existing files, following the established patterns from KAI-10.

## Implementation Patterns

All three functions follow the same pattern as the existing KAI-10 operations:

1. `getRecordingControlURL()` to discover the service endpoint
2. `context.WithTimeout(context.Background(), 15*time.Second)` for the SOAP call
3. `fmt.Sprintf` with `xmlEscape()` for SOAP body construction
4. `doRecordingControlSOAP()` to execute the request
5. `xml.Unmarshal` into `recordingControlEnvelope`
6. Fault check → nil response check → extract and return result

API handlers follow the same pattern as existing handlers:

1. Extract camera ID from path param
2. Look up camera from DB, check ONVIF endpoint exists
3. Decrypt password, call ONVIF function
4. Return JSON response or error
