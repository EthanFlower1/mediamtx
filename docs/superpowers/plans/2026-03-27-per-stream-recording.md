# Per-Stream Recording Schedules Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Make recording rules stream-aware so different streams record independently, with the playback timeline seamlessly preferring the highest quality.

**Architecture:** Refactor the scheduler to group rules by (camera, stream_id), manage per-stream MediaMTX paths and MotionSM instances, add a best-quality recording query, and add a stream selector to the Flutter recording rules UI.

**Tech Stack:** Go, SQLite, Flutter/Dart, Riverpod

**Spec:** `docs/superpowers/specs/2026-03-27-per-stream-recording-design.md`

---

## File Structure

```
internal/nvr/
├── scheduler/
│   └── scheduler.go           # MODIFY — per-stream rule grouping, path management, per-stream MotionSM
├── db/
│   ├── recordings.go          # MODIFY — add QueryRecordingsBestQuality
│   └── cameras.go             # MODIFY — stop using recording_stream_id
├── api/
│   ├── recordings.go          # MODIFY — add best_quality query param
│   ├── cameras.go             # MODIFY — remove UpdateRecordingStream, configureRecordingPaths
│   └── router.go              # MODIFY — remove PUT /cameras/:id/recording-stream

clients/flutter/lib/
├── screens/cameras/
│   ├── recording_rules_screen.dart  # MODIFY — add stream dropdown to rule dialog
│   └── camera_detail_screen.dart    # MODIFY — remove recording stream dropdown
├── models/
│   ├── recording_rule.dart          # MODIFY — add streamId field
│   └── camera.dart                  # MODIFY — remove recordingStreamId
├── providers/
│   └── recordings_provider.dart     # MODIFY — pass best_quality=true
```

---

### Task 1: Remove Per-Camera Recording Stream (Backend)

**Files:**

- Modify: `internal/nvr/api/cameras.go`
- Modify: `internal/nvr/api/router.go`

- [ ] **Step 1: Remove the UpdateRecordingStream handler and configureRecordingPaths**

In `internal/nvr/api/cameras.go`, delete the entire `UpdateRecordingStream` handler function and the `configureRecordingPaths` helper function. These were added in the previous commit and are being superseded by per-rule stream_id.

Also remove the `"net/url"` import if it was only used by `configureRecordingPaths`. Check if other functions use `url.Parse` first — if yes, keep the import.

- [ ] **Step 2: Remove the route**

In `internal/nvr/api/router.go`, remove:

```go
	protected.PUT("/cameras/:id/recording-stream", cameraHandler.UpdateRecordingStream)
```

- [ ] **Step 3: Verify build**

Run: `go build .`
Expected: builds successfully

- [ ] **Step 4: Commit**

```bash
git add internal/nvr/api/cameras.go internal/nvr/api/router.go
git commit -m "refactor: remove per-camera recording stream endpoint (superseded by per-rule stream_id)"
```

---

### Task 2: Add QueryRecordingsBestQuality to DB

**Files:**

- Modify: `internal/nvr/db/recordings.go`

- [ ] **Step 1: Add the function**

Add this function to `internal/nvr/db/recordings.go`:

```go
// QueryRecordingsBestQuality returns recordings for a camera in a time range,
// preferring higher-resolution streams when recordings from multiple streams
// overlap. For overlapping periods, only the recording from the stream with
// the largest resolution (width * height) is returned.
func (d *DB) QueryRecordingsBestQuality(cameraID string, start, end time.Time) ([]*Recording, error) {
	// Fetch all recordings in the time range.
	all, err := d.QueryRecordings(cameraID, start, end)
	if err != nil {
		return nil, err
	}
	if len(all) <= 1 {
		return all, nil
	}

	// Build a resolution lookup from camera_streams.
	// Recordings from non-default streams have paths containing "~{streamID}".
	type streamRes struct {
		width, height int
	}
	streamResolutions := make(map[string]streamRes)
	rows, err := d.Query(`
		SELECT id, width, height FROM camera_streams WHERE camera_id = ?`, cameraID)
	if err == nil {
		defer rows.Close()
		for rows.Next() {
			var id string
			var w, h int
			if rows.Scan(&id, &w, &h) == nil {
				streamResolutions[id] = streamRes{w, h}
			}
		}
	}

	// Score each recording by resolution. Main stream (no ~ in path) gets
	// max resolution; sub-streams get their actual resolution.
	resolutionOf := func(rec *Recording) int {
		// Check if path contains ~{streamID} suffix.
		path := rec.FilePath
		idx := strings.LastIndex(path, "~")
		if idx < 0 {
			return 9999 * 9999 // main stream: highest priority
		}
		// Extract stream ID (8 chars after ~, before next /).
		suffix := path[idx+1:]
		if slashIdx := strings.Index(suffix, "/"); slashIdx > 0 {
			suffix = suffix[:slashIdx]
		}
		for id, res := range streamResolutions {
			if strings.HasPrefix(id, suffix) {
				return res.width * res.height
			}
		}
		return 0 // unknown stream, lowest priority
	}

	// Filter: for overlapping recordings, keep highest resolution.
	var result []*Recording
	for _, rec := range all {
		overlaps := false
		for i, existing := range result {
			if recordingsOverlap(existing, rec) {
				// Keep the higher-resolution one.
				if resolutionOf(rec) > resolutionOf(existing) {
					result[i] = rec
				}
				overlaps = true
				break
			}
		}
		if !overlaps {
			result = append(result, rec)
		}
	}
	return result, nil
}

// recordingsOverlap checks if two recordings have overlapping time ranges.
func recordingsOverlap(a, b *Recording) bool {
	// a.EndTime > b.StartTime AND a.StartTime < b.EndTime
	return a.EndTime > b.StartTime && a.StartTime < b.EndTime
}
```

Note: Add `"strings"` to the imports if not already present.

- [ ] **Step 2: Verify build**

Run: `go build ./internal/nvr/db/`
Expected: builds successfully

- [ ] **Step 3: Commit**

```bash
git add internal/nvr/db/recordings.go
git commit -m "feat(db): add QueryRecordingsBestQuality for multi-stream playback"
```

---

### Task 3: Add best_quality Param to Recordings API

**Files:**

- Modify: `internal/nvr/api/recordings.go`

- [ ] **Step 1: Update the Query handler**

In `internal/nvr/api/recordings.go`, in the `Query` handler function, after parsing `start` and `end`, add a check for the `best_quality` query parameter:

Find the line that calls `h.DB.QueryRecordings(cameraID, start, end)` and replace it with:

```go
	var recordings []*db.Recording
	if c.Query("best_quality") == "true" {
		recordings, err = h.DB.QueryRecordingsBestQuality(cameraID, start, end)
	} else {
		recordings, err = h.DB.QueryRecordings(cameraID, start, end)
	}
```

- [ ] **Step 2: Verify build**

Run: `go build ./internal/nvr/api/`
Expected: builds successfully

- [ ] **Step 3: Commit**

```bash
git add internal/nvr/api/recordings.go
git commit -m "feat(api): add best_quality param to GET /recordings"
```

---

### Task 4: Refactor Scheduler for Per-Stream Rule Grouping

**Files:**

- Modify: `internal/nvr/scheduler/scheduler.go`

This is the most complex task. The scheduler's `evaluate()` currently groups rules by camera ID and resolves one effective mode per camera. It needs to group by **(camera ID, stream ID)** and manage per-stream paths and MotionSMs.

- [ ] **Step 1: Change MotionSM map key to include stream ID**

In the `Scheduler` struct, the `motionSMs` field stays `map[string]*MotionSM` but the key format changes from `cameraID` to `cameraID:streamID`. Add a helper:

```go
// streamKey builds a map key for per-stream state from camera ID and stream ID.
func streamKey(cameraID, streamID string) string {
	if streamID == "" {
		return cameraID
	}
	return cameraID + ":" + streamID
}
```

- [ ] **Step 2: Add helper to resolve stream path and URL**

```go
// streamPath returns the MediaMTX path for a stream. Default (empty stream ID)
// uses the camera's existing path. Non-default streams get a ~{prefix} suffix.
func streamPath(cam *db.Camera, streamID string) string {
	if streamID == "" || cam.MediaMTXPath == "" {
		return cam.MediaMTXPath
	}
	prefix := streamID
	if len(prefix) > 8 {
		prefix = prefix[:8]
	}
	return cam.MediaMTXPath + "~" + prefix
}
```

- [ ] **Step 3: Add helper to ensure a non-default stream path exists in YAML**

```go
// ensureStreamPath creates a MediaMTX path for a non-default stream if it
// doesn't exist yet. Returns the path name.
func (s *Scheduler) ensureStreamPath(cam *db.Camera, streamID string) string {
	path := streamPath(cam, streamID)
	if streamID == "" {
		return path // default path already exists
	}

	// Resolve RTSP URL for the stream.
	stream, err := s.db.GetCameraStream(streamID)
	if err != nil {
		log.Printf("scheduler: stream %s not found for camera %s", streamID, cam.ID)
		return ""
	}

	streamURL := stream.RTSPURL
	// Embed credentials if needed.
	if u, parseErr := url.Parse(streamURL); parseErr == nil && (u.User == nil || u.User.Username() == "") {
		username := cam.ONVIFUsername
		password := s.decryptPassword(cam.ONVIFPassword)
		if username != "" {
			u.User = url.UserPassword(username, password)
			streamURL = u.String()
		}
	}

	recordDir := "./recordings/" + path
	if cam.StoragePath != "" {
		recordDir = cam.StoragePath + "/" + path
	}
	recordPath := recordDir + "/%Y-%m-%d_%H-%M-%S"

	s.yamlWriter.AddPath(path, map[string]interface{}{
		"source":     streamURL,
		"record":     false,
		"recordPath": recordPath,
	})

	return path
}
```

Add `"net/url"` to the imports if not already present.

- [ ] **Step 4: Refactor evaluate() to group by (camera, stream)**

In `evaluate()`, after grouping rules by camera (the existing `rulesByCam` map), add per-stream grouping inside the per-camera loop. Replace the section that evaluates a camera (starting at `for camID := range evalCameras {`) with logic that:

1. Groups `camRules` by `rule.StreamID`
2. Evaluates each group independently
3. Resolves/creates paths per stream
4. Queues writes per stream path

Replace the core evaluation block (the section inside `for camID := range evalCameras` that calls `EvaluateRules`, handles transitions, and queues writes) with:

```go
		// Group rules by stream ID.
		rulesByStream := make(map[string][]*db.RecordingRule)
		for _, r := range camRules {
			rulesByStream[r.StreamID] = append(rulesByStream[r.StreamID], r)
		}

		// Also evaluate streams that previously had state but no longer have rules.
		for sk := range s.states {
			if strings.HasPrefix(sk, camID+":") || sk == camID {
				streamID := ""
				if idx := strings.Index(sk, ":"); idx >= 0 {
					streamID = sk[idx+1:]
				}
				if _, hasRules := rulesByStream[streamID]; !hasRules {
					rulesByStream[streamID] = nil
				}
			}
		}

		for streamID, streamRules := range rulesByStream {
			sk := streamKey(camID, streamID)
			mode, activeIDs := EvaluateRules(streamRules, now)
			desiredRecording := mode == ModeAlways

			// Resolve the path for this stream.
			path := ""
			if streamID == "" {
				path = cam.MediaMTXPath
			} else {
				path = s.ensureStreamPath(cam, streamID)
			}
			if path == "" {
				continue
			}

			s.mu.Lock()
			prev, exists := s.states[sk]
			changed := !exists || prev.EffectiveMode != mode

			s.states[sk] = &CameraState{
				EffectiveMode: mode,
				Recording:     desiredRecording,
				MotionState:   "idle",
				ActiveRules:   activeIDs,
			}
			if exists && prev.MotionState != "" {
				s.states[sk].MotionState = prev.MotionState
			}

			if changed {
				prevMode := ModeOff
				if exists {
					prevMode = prev.EffectiveMode
				}
				s.handleStreamTransition(sk, camID, cam, path, streamID, prevMode, mode, streamRules)
			}
			s.mu.Unlock()

			if changed && path != "" {
				s.queueWrite(path, desiredRecording)
				if s.eventPub != nil {
					if desiredRecording {
						s.eventPub.PublishRecordingStarted(cam.Name)
					} else if exists && prev.Recording {
						s.eventPub.PublishRecordingStopped(cam.Name)
					}
				}
			}

			// Clean up non-default stream paths when mode is off.
			if mode == ModeOff && streamID != "" {
				s.yamlWriter.RemovePath(path)
			}
		}
```

- [ ] **Step 5: Add handleStreamTransition method**

This replaces `handleEventPipelineTransitionLocked` for per-stream transitions:

```go
func (s *Scheduler) handleStreamTransition(
	sk, camID string,
	cam *db.Camera,
	path, streamID string,
	prevMode, newMode EffectiveMode,
	rules []*db.RecordingRule,
) {
	// Stop event pipeline if leaving Events mode.
	if prevMode == ModeEvents && newMode != ModeEvents {
		if sm, ok := s.motionSMs[sk]; ok {
			sm.Stop()
			delete(s.motionSMs, sk)
		}
	}

	// Start event pipeline if entering Events mode.
	if newMode == ModeEvents && prevMode != ModeEvents {
		postEvent := 30 * time.Second
		for _, r := range rules {
			if r.PostEventSeconds > 0 {
				postEvent = time.Duration(r.PostEventSeconds) * time.Second
				break
			}
		}
		sm := NewMotionSM(camID, path, postEvent, func(p string, record bool) {
			s.queueWrite(p, record)
		})
		s.motionSMs[sk] = sm
	}
}
```

- [ ] **Step 6: Update existing references from cameraID to streamKey**

Search the scheduler for any code that accesses `s.motionSMs[camID]` and update to use `streamKey(camID, streamID)`. The event subscription callbacks (in `startEventPipelineLocked`) that call `sm.OnMotion(active)` need to find the correct SM by iterating motionSMs for the camera prefix.

Update the event callback in `startEventPipelineLocked` (or its equivalent) to dispatch motion events to ALL motionSMs for that camera:

```go
// In the ONVIF event callback:
s.mu.Lock()
for sk, sm := range s.motionSMs {
	if strings.HasPrefix(sk, camID+":") || sk == camID {
		sm.OnMotion(active)
	}
}
s.mu.Unlock()
```

- [ ] **Step 7: Verify build and run tests**

Run: `go build ./internal/nvr/scheduler/`
Run: `go test ./internal/nvr/scheduler/ -v -count=1`
Expected: builds and tests pass

- [ ] **Step 8: Commit**

```bash
git add internal/nvr/scheduler/scheduler.go
git commit -m "feat(scheduler): per-stream rule grouping with independent path management"
```

---

### Task 5: Add Stream Dropdown to Recording Rules UI

**Files:**

- Modify: `clients/flutter/lib/models/recording_rule.dart`
- Modify: `clients/flutter/lib/screens/cameras/recording_rules_screen.dart`

- [ ] **Step 1: Add streamId field to RecordingRule model**

In `clients/flutter/lib/models/recording_rule.dart`, add `streamId` to the class:

```dart
class RecordingRule {
  final String id;
  final String cameraId;
  final String streamId;
  final String mode;
  final String? startTime;
  final String? endTime;
  final List<int>? daysOfWeek;
  final bool enabled;

  const RecordingRule({
    required this.id,
    required this.cameraId,
    this.streamId = '',
    required this.mode,
    this.startTime,
    this.endTime,
    this.daysOfWeek,
    required this.enabled,
  });

  factory RecordingRule.fromJson(Map<String, dynamic> json) {
    final rawDays = json['days_of_week'] as List<dynamic>?;
    final daysOfWeek = rawDays?.map((d) => d as int).toList();

    return RecordingRule(
      id: json['id'] as String,
      cameraId: json['camera_id'] as String,
      streamId: json['stream_id'] as String? ?? '',
      mode: json['mode'] as String,
      startTime: json['start_time'] as String?,
      endTime: json['end_time'] as String?,
      daysOfWeek: daysOfWeek,
      enabled: json['enabled'] as bool,
    );
  }

  Map<String, dynamic> toJson() {
    return {
      'id': id,
      'camera_id': cameraId,
      'stream_id': streamId,
      'mode': mode,
      'start_time': startTime,
      'end_time': endTime,
      'days_of_week': daysOfWeek,
      'enabled': enabled,
    };
  }
}
```

- [ ] **Step 2: Add stream dropdown to the create rule dialog**

In `clients/flutter/lib/screens/cameras/recording_rules_screen.dart`, the `_showAddDialog()` function needs to:

1. Accept a `List<CameraStream>` parameter for available streams
2. Add a `selectedStreamId` state variable
3. Add a `DropdownButtonFormField` before the mode selector

Add import at top:

```dart
import '../../models/camera_stream.dart';
```

In the dialog's `StatefulBuilder`, add a stream ID state variable:

```dart
String selectedStreamId = '';
```

Add the dropdown widget before the mode selector in the dialog content column:

```dart
DropdownButtonFormField<String>(
  value: selectedStreamId,
  dropdownColor: NvrColors.bgTertiary,
  style: NvrTypography.monoData,
  decoration: InputDecoration(
    labelText: 'STREAM',
    labelStyle: NvrTypography.monoLabel,
    filled: true,
    fillColor: NvrColors.bgTertiary,
    border: OutlineInputBorder(
      borderRadius: BorderRadius.circular(4),
      borderSide: const BorderSide(color: NvrColors.border),
    ),
    enabledBorder: OutlineInputBorder(
      borderRadius: BorderRadius.circular(4),
      borderSide: const BorderSide(color: NvrColors.border),
    ),
    focusedBorder: OutlineInputBorder(
      borderRadius: BorderRadius.circular(4),
      borderSide: const BorderSide(color: NvrColors.accent),
    ),
    contentPadding: const EdgeInsets.symmetric(horizontal: 12, vertical: 10),
  ),
  items: [
    const DropdownMenuItem(
      value: '',
      child: Text('Default', style: NvrTypography.monoData),
    ),
    ...streams.map((s) => DropdownMenuItem(
      value: s.id,
      child: Text(s.displayLabel, style: NvrTypography.monoData),
    )),
  ],
  onChanged: (v) => dialogSetState(() => selectedStreamId = v ?? ''),
),
const SizedBox(height: 12),
```

Pass `selectedStreamId` to `_saveNewRule()` as a `streamId` parameter.

- [ ] **Step 3: Update \_saveNewRule to include stream_id**

Add `String streamId = ''` parameter to `_saveNewRule()` and include it in the POST body:

```dart
Future<void> _saveNewRule({
  required String mode,
  String streamId = '',
  String? startTime,
  String? endTime,
  List<int>? daysOfWeek,
}) async {
  // ... existing code ...
  await api.post<dynamic>(
    '/cameras/${widget.cameraId}/recording-rules',
    data: {
      'mode': mode,
      'stream_id': streamId,
      'enabled': true,
      if (startTime != null) 'start_time': startTime,
      if (endTime != null) 'end_time': endTime,
      if (daysOfWeek != null) 'days_of_week': daysOfWeek,
    },
  );
  // ... rest of existing code ...
}
```

- [ ] **Step 4: Fetch streams on screen load**

The recording rules screen needs to fetch streams. Add state and fetch logic:

```dart
List<CameraStream> _streams = [];

@override
void initState() {
  super.initState();
  _fetchRules();
  _fetchStreams();
}

Future<void> _fetchStreams() async {
  final api = ref.read(apiClientProvider);
  if (api == null) return;
  try {
    final res = await api.get<dynamic>('/cameras/${widget.cameraId}/streams');
    final list = (res.data as List)
        .map((e) => CameraStream.fromJson(e as Map<String, dynamic>))
        .toList();
    if (mounted) setState(() => _streams = list);
  } catch (_) {}
}
```

Pass `_streams` to `_showAddDialog()`.

- [ ] **Step 5: Show stream name in rule list**

In the rule list tile, if `rule.streamId` is not empty, show the stream name. Look up stream name from `_streams`:

```dart
String streamLabel = '';
if (rule.streamId.isNotEmpty) {
  final stream = _streams.where((s) => s.id == rule.streamId).firstOrNull;
  streamLabel = stream != null ? ' — ${stream.displayLabel}' : ' — Custom stream';
}
// Display: "${rule.mode}$streamLabel"
```

- [ ] **Step 6: Verify it compiles**

Run: `cd /Users/ethanflower/personal_projects/mediamtx/clients/flutter && flutter analyze lib/screens/cameras/recording_rules_screen.dart lib/models/recording_rule.dart`
Expected: No errors

- [ ] **Step 7: Commit**

```bash
git add clients/flutter/lib/models/recording_rule.dart clients/flutter/lib/screens/cameras/recording_rules_screen.dart
git commit -m "feat(flutter): add stream selector to recording rules UI"
```

---

### Task 6: Update Flutter Recordings Provider

**Files:**

- Modify: `clients/flutter/lib/providers/recordings_provider.dart`

- [ ] **Step 1: Pass best_quality=true**

In the `recordingSegmentsProvider`, add `'best_quality': 'true'` to the query parameters:

```dart
  final res = await api.get<dynamic>('/recordings', queryParameters: {
    'camera_id': key.cameraId,
    'start': start,
    'end': end,
    'best_quality': 'true',
  });
```

- [ ] **Step 2: Verify it compiles**

Run: `flutter analyze lib/providers/recordings_provider.dart`
Expected: No errors

- [ ] **Step 3: Commit**

```bash
git add clients/flutter/lib/providers/recordings_provider.dart
git commit -m "feat(flutter): use best_quality=true for timeline recordings"
```

---

### Task 7: Remove Recording Stream Dropdown from Camera Detail

**Files:**

- Modify: `clients/flutter/lib/screens/cameras/camera_detail_screen.dart`
- Modify: `clients/flutter/lib/models/camera.dart`
- Regenerate: `clients/flutter/lib/models/camera.freezed.dart`
- Regenerate: `clients/flutter/lib/models/camera.g.dart`

- [ ] **Step 1: Remove recordingStreamId from Camera model**

In `clients/flutter/lib/models/camera.dart`, remove the line:

```dart
    @JsonKey(name: 'recording_stream_id') @Default('') String recordingStreamId,
```

- [ ] **Step 2: Regenerate freezed files**

Run: `cd /Users/ethanflower/personal_projects/mediamtx/clients/flutter && dart run build_runner build --delete-conflicting-outputs`

- [ ] **Step 3: Remove recording stream dropdown and save method from camera detail screen**

In `clients/flutter/lib/screens/cameras/camera_detail_screen.dart`:

1. Remove the `_recordingStreamId` state variable
2. Remove `_recordingStreamId = camera.recordingStreamId;` from `_fetchCamera()`
3. Remove the `_saveRecordingStream()` method
4. Remove the recording stream `DropdownButtonFormField` from the recording section (the block wrapped in `if (_streams.isNotEmpty) ...[]`)

- [ ] **Step 4: Verify it compiles**

Run: `flutter analyze lib/screens/cameras/camera_detail_screen.dart lib/models/camera.dart`
Expected: No errors

- [ ] **Step 5: Commit**

```bash
git add clients/flutter/lib/models/camera.dart clients/flutter/lib/models/camera.freezed.dart clients/flutter/lib/models/camera.g.dart clients/flutter/lib/screens/cameras/camera_detail_screen.dart
git commit -m "refactor(flutter): remove per-camera recording stream selector (superseded by per-rule stream_id)"
```

---

### Task 8: End-to-End Verification

- [ ] **Step 1: Run all Go tests**

Run: `cd /Users/ethanflower/personal_projects/mediamtx && go test ./internal/nvr/... -count=1`
Expected: all pass

- [ ] **Step 2: Build full binary**

Run: `go build .`
Expected: builds successfully

- [ ] **Step 3: Run Flutter analyze**

Run: `cd clients/flutter && flutter analyze lib/`
Expected: No errors

- [ ] **Step 4: Manual smoke test**

1. Start server, open Flutter app
2. Navigate to a camera's recording rules screen
3. Create a rule: mode=always, stream=Default → verify recording starts on main path
4. Create a second rule: mode=always, stream=Sub Stream → verify `~{streamID}` path appears in YAML and recording starts
5. Open playback timeline → verify recordings from both streams appear as one merged bar
6. Delete the sub-stream rule → verify `~{streamID}` path is removed from YAML

- [ ] **Step 5: Commit**

```bash
git add -A
git commit -m "test: verify per-stream recording end-to-end"
```
