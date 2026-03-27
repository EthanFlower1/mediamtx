# AI Settings UI — Stream Selection & Detection Config

**Date:** 2026-03-27
**Status:** Approved
**Goal:** Expand the camera detail screen's AI Detection section with stream selection, confidence threshold, and track timeout controls.

---

## Context

The backend now supports per-camera AI pipeline configuration (`ai_stream_id`, `ai_confidence`, `ai_track_timeout`) via `PUT /cameras/:id/ai`. The Flutter UI currently only exposes a toggle and confidence slider. Users need to select which stream runs detection and configure the track timeout.

## Design

### Layout

All controls live in the existing AI Detection `_SectionCard` in `camera_detail_screen.dart`:

```
AI DETECTION (section header)
├─ [HudToggle] Enable AI detection
│  (if enabled, show below)
├─ [AnalogSlider] CONFIDENCE: 50%
├─ [DropdownButtonFormField] DETECTION STREAM: Default (640x480)
├─ [AnalogSlider] TRACK TIMEOUT: 5s
└─ [HudButton.tactical] SAVE AI SETTINGS
```

### Stream Selector

- `DropdownButtonFormField<String>` styled with `_hudInputDecoration('DETECTION STREAM')`
- Items: one "Default" entry + one entry per `camera_streams` record
- Display format: `"{stream.name} ({stream.width}x{stream.height})"` or `"Default"` for empty string
- Value: stream ID string, or empty string for "Default"
- "Default" means the backend resolves via role-based fallback (ai_detection role > lowest-res stream > main RTSP URL)
- Dropdown colors: `dropdownColor: NvrColors.bgTertiary`, `style: NvrTypography.monoData`

### Track Timeout Slider

- `AnalogSlider` with label `'TRACK TIMEOUT'`
- Range: 1-30 seconds, default 5
- `valueFormatter: (v) => '${v.round()}s'`
- Controls how long a tracked object can be missing before marked "left"

### Save Behavior

- All AI settings save together via a "SAVE AI SETTINGS" `HudButton.tactical` at the bottom
- The toggle controls section visibility but does NOT auto-save — user presses save
- On save: `PUT /cameras/:id/ai` with `{ ai_enabled, stream_id, confidence, track_timeout }`
- Success: green SnackBar "AI settings saved"
- Error: red SnackBar with error message
- Button shows "SAVING..." with loading state while request is in flight

### Data Fetching

- Streams fetched via `GET /cameras/:id/streams` on screen load
- Stored in local state: `List<CameraStream> _streams = []`
- If fetch fails or returns empty, dropdown shows only "Default"

## Model Changes

### Camera model (`camera.dart`)

Add three fields to the Freezed model:

```dart
@JsonKey(name: 'ai_stream_id') @Default('') String aiStreamId,
@JsonKey(name: 'ai_confidence') @Default(0.5) double aiConfidence,
@JsonKey(name: 'ai_track_timeout') @Default(5) int aiTrackTimeout,
```

### CameraStream model

If not already present, ensure there's a model matching the backend:

```dart
class CameraStream {
  final String id;
  final String cameraId;
  final String name;
  final String rtspUrl;
  final int width;
  final int height;
  final String roles;
}
```

Check if this already exists in the codebase — the onboarding flow creates streams, so the model likely exists.

## Files Changed

- `lib/models/camera.dart` — add 3 new fields
- `lib/models/camera.freezed.dart` — regenerated
- `lib/models/camera.g.dart` — regenerated
- `lib/screens/cameras/camera_detail_screen.dart` — expand AI section with dropdown, slider, save button
- Possibly `lib/models/camera_stream.dart` — if model doesn't exist yet

## No Backend Changes

The backend already supports all fields via `PUT /cameras/:id/ai` (implemented in the pipeline redesign). This is purely a Flutter UI task.
