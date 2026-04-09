# KAI-25: ONVIF Profile T OSD Management Design

## Overview

Implement ONVIF Profile T On-Screen Display (OSD) management for the NVR system. This enables creating, reading, updating, and deleting text and image overlays on camera video streams via the ONVIF Media2 service.

## Scope

- **In scope:** GetOSDs, GetOSDOptions, CreateOSD, SetOSD, DeleteOSD via Media2 service
- **Out of scope:** Media1 fallback (Profile T cameras use Media2), OSD preview rendering, font upload

## Architecture

### ONVIF Layer (`internal/nvr/onvif/osd.go`)

New file with custom SOAP against the Media2 service (`ver20/media/wsdl` / `tr2` namespace). Follows the same pattern as `analytics.go` — dedicated SOAP helper, XML response types, public functions.

#### Public Functions

| Function                                        | SOAP Operation      | Returns                 |
| ----------------------------------------------- | ------------------- | ----------------------- |
| `GetOSDs(xaddr, user, pass, configToken)`       | `tr2:GetOSDs`       | `[]OSD, error`          |
| `GetOSDOptions(xaddr, user, pass, configToken)` | `tr2:GetOSDOptions` | `*OSDOptions, error`    |
| `CreateOSD(xaddr, user, pass, osd OSDConfig)`   | `tr2:CreateOSD`     | `string (token), error` |
| `SetOSD(xaddr, user, pass, osd OSDConfig)`      | `tr2:SetOSD`        | `error`                 |
| `DeleteOSD(xaddr, user, pass, token)`           | `tr2:DeleteOSD`     | `error`                 |

#### SOAP Helper

`doMedia2OSD()` — builds SOAP envelope with `tr2` namespace, injects WS-Security via `injectWSSecurity()`, posts to the `media2` service URL from `client.ServiceURL("media2")`. Returns `ErrOSDNotSupported` if `media2` service is absent.

#### Data Types

```go
type OSD struct {
    Token            string      `json:"token"`
    VideoSourceToken string      `json:"video_source_token"`
    Type             string      `json:"type"`              // "Text" or "Image"
    Position         OSDPosition `json:"position"`
    TextString       *OSDText    `json:"text_string,omitempty"`
    Image            *OSDImage   `json:"image,omitempty"`
}

type OSDPosition struct {
    Type string   `json:"type"` // "UpperLeft", "UpperRight", "LowerLeft", "LowerRight", "Custom"
    X    *float64 `json:"x,omitempty"`    // Only for Custom position
    Y    *float64 `json:"y,omitempty"`    // Only for Custom position
}

type OSDText struct {
    IsPersistentText bool   `json:"is_persistent_text"`
    Type             string `json:"type"`              // "Plain", "DateAndTime", "DateOnly", "TimeOnly"
    PlainText        string `json:"plain_text,omitempty"`
    FontSize         *int   `json:"font_size,omitempty"`
    FontColor        string `json:"font_color,omitempty"`
    BackgroundColor  string `json:"background_color,omitempty"`
}

type OSDImage struct {
    ImagePath string `json:"image_path"`
}

type OSDOptions struct {
    MaximumNumberOfOSDs MaxOSDs       `json:"maximum_number_of_osds"`
    Types               []string      `json:"types"`
    PositionOptions     []string      `json:"position_options"`
    TextOptions         *OSDTextOpts  `json:"text_options,omitempty"`
    ImageOptions        *OSDImageOpts `json:"image_options,omitempty"`
}

type MaxOSDs struct {
    Total       int `json:"total"`
    PlainText   int `json:"plain_text"`
    DateAndTime int `json:"date_and_time"`
    Image       int `json:"image"`
}
```

#### Error Sentinel

```go
var ErrOSDNotSupported = fmt.Errorf("camera does not support OSD via Media2 service")
```

### REST API Layer

#### Routes

Registered in `router.go` under the protected group:

```
GET    /cameras/:id/osd              → GetOSDs
GET    /cameras/:id/osd/options      → GetOSDOptions
POST   /cameras/:id/osd              → CreateOSD
PUT    /cameras/:id/osd/:token       → SetOSD
DELETE /cameras/:id/osd/:token       → DeleteOSD
```

#### Handler Pattern

Each handler in `cameras.go` follows the established pattern:

1. Get camera from DB by `:id`
2. Check ONVIF endpoint exists (400 if not)
3. Decrypt password via `h.decryptPassword()`
4. Call corresponding `onvif.*` function
5. Error mapping: `ErrOSDNotSupported` → 501, other errors → 503
6. Return JSON response

#### Request/Response Examples

**GET /cameras/:id/osd**

```json
[
  {
    "token": "OSD_1",
    "video_source_token": "VS_1",
    "type": "Text",
    "position": { "type": "LowerLeft" },
    "text_string": { "type": "DateAndTime", "is_persistent_text": true }
  }
]
```

**POST /cameras/:id/osd**

```json
// Request
{
  "video_source_token": "VS_1",
  "type": "Text",
  "position": {"type": "UpperRight"},
  "text_string": {"type": "Plain", "plain_text": "Camera 1", "is_persistent_text": true}
}
// Response 201
{"token": "OSD_NEW_1"}
```

**PUT /cameras/:id/osd/:token**

```json
// Request
{
  "token": "OSD_1",
  "video_source_token": "VS_1",
  "type": "Text",
  "position": {"type": "LowerRight"},
  "text_string": {"type": "Plain", "plain_text": "Updated", "is_persistent_text": true}
}
// Response 200
{"message": "OSD updated"}
```

**DELETE /cameras/:id/osd/:token**

```json
// Response 200
{ "message": "OSD deleted" }
```

#### Validation (Create/Set)

- `type` must be "Text" or "Image"
- `position.type` must be one of: "UpperLeft", "UpperRight", "LowerLeft", "LowerRight", "Custom"
- If `position.type` is "Custom", `x` and `y` are required
- If `type` is "Text", `text_string` is required
- If `type` is "Image", `image` is required with non-empty `image_path`

## Files Changed

| File                          | Change                                            |
| ----------------------------- | ------------------------------------------------- |
| `internal/nvr/onvif/osd.go`   | **New** — SOAP types, helpers, 5 public functions |
| `internal/nvr/api/cameras.go` | Add 5 handler methods                             |
| `internal/nvr/api/router.go`  | Register 5 routes                                 |

## Error Handling

| Condition                                 | HTTP Status |
| ----------------------------------------- | ----------- |
| Camera not found                          | 404         |
| No ONVIF endpoint                         | 400         |
| Media2 not supported (ErrOSDNotSupported) | 501         |
| Device unreachable / SOAP fault           | 503         |
| Invalid request body / validation failure | 400         |
