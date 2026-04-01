# KAI-10: ONVIF Profile G Recording Control Design

## Overview

Add Recording Control service operations to the existing ONVIF Profile G implementation. This complements the already-implemented Recording Search (`tse:`) and Replay (`trp:`) services with the Recording Control service (`trc:` = `http://www.onvif.org/ver10/recording/wsdl`), enabling the NVR to manage recordings and recording jobs on camera edge storage.

## Operations

### Recordings (storage containers)

| Operation | SOAP Action | Purpose |
|-----------|-------------|---------|
| CreateRecording | `trc:CreateRecording` | Create a recording container on the device |
| DeleteRecording | `trc:DeleteRecording` | Delete a recording container |
| GetRecordingConfiguration | `trc:GetRecordingConfiguration` | Get config for a specific recording |

### Recording Jobs (triggers)

| Operation | SOAP Action | Purpose |
|-----------|-------------|---------|
| CreateRecordingJob | `trc:CreateRecordingJob` | Start recording into a container (mode: Idle/Active) |
| DeleteRecordingJob | `trc:DeleteRecordingJob` | Stop and remove a recording job |
| GetRecordingJobState | `trc:GetRecordingJobState` | Check if a job is actively recording |

## Types

### RecordingSource
- SourceId (string) — profile token or source identifier
- Name (string) — human-readable source name
- Location (string) — location description
- Description (string) — source description
- Address (string) — ONVIF profile token or media URI

### RecordingConfiguration
- RecordingToken (string) — token returned by CreateRecording
- Source (RecordingSource)
- MaximumRetentionTime (string) — xs:duration, e.g. "PT48H"
- Content (string) — content description

### RecordingJobConfiguration
- JobToken (string) — token returned by CreateRecordingJob
- RecordingToken (string) — which recording container to record into
- Mode (string) — "Idle" or "Active"
- Priority (int) — job priority (higher wins on conflict)

### RecordingJobState
- JobToken (string)
- RecordingToken (string)
- State (string) — "Idle", "Active", or "Error"
- Sources ([]RecordingJobStateSource) — per-source state info

### RecordingJobStateSource
- SourceToken (string)
- State (string) — per-source recording state

## File Changes

### New Files
- `internal/nvr/onvif/recording_control.go` — SOAP envelope, XML types, 6 public functions
- `internal/nvr/api/recording_control.go` — API handler struct + 6 handler methods

### Modified Files
- `internal/nvr/onvif/client.go` — add `"recording_control"` service mapping in `buildServiceMap`
- `internal/nvr/api/router.go` — register 6 new routes

## API Endpoints

| Method | Path | Handler |
|--------|------|---------|
| GET | `/cameras/:id/recording-control/config` | GetRecordingConfig (query param: `token`) |
| POST | `/cameras/:id/recording-control/recordings` | CreateRecording |
| DELETE | `/cameras/:id/recording-control/recordings/:token` | DeleteRecording |
| POST | `/cameras/:id/recording-control/jobs` | CreateRecordingJob |
| DELETE | `/cameras/:id/recording-control/jobs/:token` | DeleteRecordingJob |
| GET | `/cameras/:id/recording-control/jobs/:token/state` | GetRecordingJobState |

### Request/Response Examples

**POST /cameras/:id/recording-control/recordings**
```json
// Request
{
  "source": {
    "source_id": "VideoSource_1",
    "name": "Front Camera",
    "location": "Entrance",
    "description": "Main entrance camera",
    "address": "Profile_1"
  },
  "maximum_retention_time": "PT48H",
  "content": "Continuous recording"
}
// Response
{
  "recording_token": "Recording_001"
}
```

**POST /cameras/:id/recording-control/jobs**
```json
// Request
{
  "recording_token": "Recording_001",
  "mode": "Active",
  "priority": 1
}
// Response
{
  "job_token": "Job_001",
  "recording_token": "Recording_001"
}
```

**GET /cameras/:id/recording-control/jobs/:token/state**
```json
// Response
{
  "job_token": "Job_001",
  "recording_token": "Recording_001",
  "state": "Active",
  "sources": [
    {"source_token": "VideoSource_1", "state": "Active"}
  ]
}
```

## Service URL Discovery

Update `buildServiceMap` in `client.go` to distinguish:
- Recording Control: namespace contains `recording/wsdl` → key `"recording_control"`
- Recording Search: namespace contains `search/wsdl` → key `"search"` (existing)

The current `"recording"` key matches broadly on `recording` in namespace. Refine to:
- `strings.Contains(ns, "recording/wsdl")` → `"recording_control"`
- Keep `strings.Contains(ns, "search")` → `"search"` (already exists)

## SOAP Patterns

Follow existing conventions from `recording.go` and `replay.go`:
- `recordingControlSoap(innerBody)` envelope builder with `trc:` and `tt:` namespaces
- `doRecordingControlSOAP(ctx, url, user, pass, body)` executor with WS-Security injection
- XML response types with `recordingControlEnvelope` / `recordingControlBody`
- SOAP fault detection, HTTP status checks, `io.LimitReader(1<<20)`, error truncation
