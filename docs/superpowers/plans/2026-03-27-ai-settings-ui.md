# AI Settings UI Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Add stream selection dropdown and track timeout slider to the camera detail screen's AI Detection section.

**Architecture:** Add 3 fields to the Freezed Camera model, create a lightweight CameraStream model, expand the AI section in camera_detail_screen.dart with a dropdown and slider, and update the save function to send the new fields.

**Tech Stack:** Flutter, Riverpod, Freezed, Dart

**Spec:** `docs/superpowers/specs/2026-03-27-ai-settings-ui-design.md`

---

## File Structure

```
clients/flutter/lib/
├── models/
│   ├── camera.dart              # MODIFY — add 3 AI config fields
│   ├── camera.freezed.dart      # REGENERATE
│   ├── camera.g.dart            # REGENERATE
│   └── camera_stream.dart       # CREATE — lightweight stream model
└── screens/cameras/
    └── camera_detail_screen.dart # MODIFY — expand AI section
```

---

### Task 1: Add CameraStream Model

**Files:**
- Create: `clients/flutter/lib/models/camera_stream.dart`

- [ ] **Step 1: Create the CameraStream model**

```dart
// clients/flutter/lib/models/camera_stream.dart

class CameraStream {
  final String id;
  final String cameraId;
  final String name;
  final String rtspUrl;
  final int width;
  final int height;
  final String roles;

  const CameraStream({
    required this.id,
    required this.cameraId,
    required this.name,
    required this.rtspUrl,
    this.width = 0,
    this.height = 0,
    this.roles = '',
  });

  factory CameraStream.fromJson(Map<String, dynamic> json) {
    return CameraStream(
      id: json['id'] as String? ?? '',
      cameraId: json['camera_id'] as String? ?? '',
      name: json['name'] as String? ?? '',
      rtspUrl: json['rtsp_url'] as String? ?? '',
      width: json['width'] as int? ?? 0,
      height: json['height'] as int? ?? 0,
      roles: json['roles'] as String? ?? '',
    );
  }

  /// Display label for the dropdown: "Sub Stream (640x480)"
  String get displayLabel {
    if (width > 0 && height > 0) {
      return '$name (${width}x$height)';
    }
    return name;
  }
}
```

- [ ] **Step 2: Verify it compiles**

Run: `cd /Users/ethanflower/personal_projects/mediamtx/clients/flutter && flutter analyze lib/models/camera_stream.dart`
Expected: No issues found

- [ ] **Step 3: Commit**

```bash
git add clients/flutter/lib/models/camera_stream.dart
git commit -m "feat(flutter): add CameraStream model"
```

---

### Task 2: Update Camera Model with AI Config Fields

**Files:**
- Modify: `clients/flutter/lib/models/camera.dart`
- Regenerate: `clients/flutter/lib/models/camera.freezed.dart`
- Regenerate: `clients/flutter/lib/models/camera.g.dart`

- [ ] **Step 1: Add 3 new fields to camera.dart**

Add these lines after the `aiEnabled` field (line 16) in the `Camera` factory constructor:

```dart
    @JsonKey(name: 'ai_stream_id') @Default('') String aiStreamId,
    @JsonKey(name: 'ai_confidence') @Default(0.5) double aiConfidence,
    @JsonKey(name: 'ai_track_timeout') @Default(5) int aiTrackTimeout,
```

The full factory constructor should now have these AI-related fields together:

```dart
    @JsonKey(name: 'ai_enabled') @Default(false) bool aiEnabled,
    @JsonKey(name: 'ai_stream_id') @Default('') String aiStreamId,
    @JsonKey(name: 'ai_confidence') @Default(0.5) double aiConfidence,
    @JsonKey(name: 'ai_track_timeout') @Default(5) int aiTrackTimeout,
    @JsonKey(name: 'sub_stream_url') @Default('') String subStreamUrl,
```

- [ ] **Step 2: Run build_runner to regenerate freezed/json files**

Run: `cd /Users/ethanflower/personal_projects/mediamtx/clients/flutter && dart run build_runner build --delete-conflicting-outputs`
Expected: Generates updated `camera.freezed.dart` and `camera.g.dart`

- [ ] **Step 3: Verify it compiles**

Run: `flutter analyze lib/models/camera.dart`
Expected: No issues found

- [ ] **Step 4: Commit**

```bash
git add clients/flutter/lib/models/camera.dart clients/flutter/lib/models/camera.freezed.dart clients/flutter/lib/models/camera.g.dart
git commit -m "feat(flutter): add aiStreamId, aiConfidence, aiTrackTimeout to Camera model"
```

---

### Task 3: Expand AI Detection Section in Camera Detail Screen

**Files:**
- Modify: `clients/flutter/lib/screens/cameras/camera_detail_screen.dart`

This is the main task — adding the stream dropdown, track timeout slider, and save button to the AI section.

- [ ] **Step 1: Add imports and state variables**

At the top of `camera_detail_screen.dart`, add the import:

```dart
import '../../models/camera_stream.dart';
```

In the `_CameraDetailScreenState` class, add these state variables in the "AI controls" section (after line 38):

```dart
  // ── AI controls ─────────────────────────────────────────────────────────
  bool _aiEnabled = false;
  double _confidence = 0.5;
  String _aiStreamId = '';
  double _trackTimeout = 5;
  List<CameraStream> _streams = [];
```

Remove the existing `_confidence` line (line 38) since it's being replaced.

- [ ] **Step 2: Fetch streams and populate AI state from camera data**

In the `_fetchCamera()` method, after populating the existing camera state (around line 107), add:

```dart
        _aiEnabled = camera.aiEnabled;
        _confidence = camera.aiConfidence.clamp(0.2, 0.9);
        _aiStreamId = camera.aiStreamId;
        _trackTimeout = camera.aiTrackTimeout.toDouble().clamp(1, 30);
        _retentionDays = camera.retentionDays.toDouble().clamp(7, 90);
```

Remove the existing `_aiEnabled = camera.aiEnabled;` line (107) since it's replaced above.

Then add a stream fetch after the main try block's setState (after line 109, still inside try):

```dart
      // Fetch streams for the AI stream selector.
      try {
        final streamsRes = await api.get<dynamic>('/cameras/${widget.cameraId}/streams');
        final streamsList = (streamsRes.data as List)
            .map((e) => CameraStream.fromJson(e as Map<String, dynamic>))
            .toList();
        if (mounted) setState(() => _streams = streamsList);
      } catch (_) {
        // Streams may not exist yet — dropdown will show only "Default".
      }
```

- [ ] **Step 3: Update the _saveAi() method**

Replace the existing `_saveAi()` method (lines 147-172) with:

```dart
  Future<void> _saveAi() async {
    final api = ref.read(apiClientProvider);
    if (api == null) return;
    if (mounted) setState(() => _savingAi = true);
    try {
      await api.put('/cameras/${widget.cameraId}/ai', data: {
        'ai_enabled': _aiEnabled,
        'stream_id': _aiStreamId,
        'confidence': _confidence,
        'track_timeout': _trackTimeout.round(),
      });
      _fetchCamera();
      if (mounted) {
        ScaffoldMessenger.of(context).showSnackBar(
          const SnackBar(backgroundColor: NvrColors.success, content: Text('AI settings saved')),
        );
      }
    } catch (e) {
      if (mounted) {
        ScaffoldMessenger.of(context).showSnackBar(
          SnackBar(backgroundColor: NvrColors.danger, content: Text('Error: $e')),
        );
      }
    } finally {
      if (mounted) setState(() => _savingAi = false);
    }
  }
```

- [ ] **Step 4: Replace the AI Detection section widget**

Replace the entire AI Detection `_SectionCard` (lines 459-493) with:

```dart
        // AI Detection section
        _SectionCard(
          header: 'AI DETECTION',
          child: Column(
            crossAxisAlignment: CrossAxisAlignment.start,
            children: [
              Row(
                children: [
                  HudToggle(
                    value: _aiEnabled,
                    onChanged: (v) => setState(() => _aiEnabled = v),
                  ),
                  const SizedBox(width: 12),
                  const Text('Enable AI detection', style: NvrTypography.body),
                ],
              ),
              if (_aiEnabled) ...[
                const SizedBox(height: 12),
                AnalogSlider(
                  label: 'CONFIDENCE',
                  value: _confidence,
                  min: 0.2,
                  max: 0.9,
                  onChanged: (v) => setState(() => _confidence = v),
                  valueFormatter: (v) => '${(v * 100).round()}%',
                ),
                const SizedBox(height: 12),
                DropdownButtonFormField<String>(
                  value: _aiStreamId,
                  dropdownColor: NvrColors.bgTertiary,
                  style: NvrTypography.monoData,
                  decoration: InputDecoration(
                    labelText: 'DETECTION STREAM',
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
                    ..._streams.map((s) => DropdownMenuItem(
                      value: s.id,
                      child: Text(s.displayLabel, style: NvrTypography.monoData),
                    )),
                  ],
                  onChanged: (v) => setState(() => _aiStreamId = v ?? ''),
                ),
                const SizedBox(height: 12),
                AnalogSlider(
                  label: 'TRACK TIMEOUT',
                  value: _trackTimeout,
                  min: 1,
                  max: 30,
                  onChanged: (v) => setState(() => _trackTimeout = v),
                  valueFormatter: (v) => '${v.round()}s',
                ),
                const SizedBox(height: 12),
                SizedBox(
                  width: double.infinity,
                  child: HudButton(
                    style: HudButtonStyle.tactical,
                    onPressed: _savingAi ? null : _saveAi,
                    label: _savingAi ? 'SAVING...' : 'SAVE AI SETTINGS',
                  ),
                ),
              ],
            ],
          ),
        ),
```

- [ ] **Step 5: Verify it compiles**

Run: `flutter analyze lib/screens/cameras/camera_detail_screen.dart`
Expected: No issues found (or only warnings, no errors)

- [ ] **Step 6: Commit**

```bash
git add clients/flutter/lib/screens/cameras/camera_detail_screen.dart
git commit -m "feat(flutter): add stream selection and track timeout to AI settings"
```

---

### Task 4: Verify End-to-End

- [ ] **Step 1: Run full Flutter analyze**

Run: `cd /Users/ethanflower/personal_projects/mediamtx/clients/flutter && flutter analyze lib/`
Expected: No errors (warnings acceptable)

- [ ] **Step 2: Verify backend build still clean**

Run: `cd /Users/ethanflower/personal_projects/mediamtx && go build .`
Expected: Builds successfully

- [ ] **Step 3: Manual smoke test**

1. Open the Flutter app, navigate to a camera's detail screen
2. Scroll to the AI DETECTION section
3. Toggle AI detection on — verify dropdown, sliders, and save button appear
4. Select a stream from the dropdown (or leave as "Default")
5. Adjust confidence and track timeout sliders
6. Press SAVE AI SETTINGS — verify green "AI settings saved" snackbar
7. Navigate away and back — verify settings persisted

- [ ] **Step 4: Final commit**

```bash
git add -A
git commit -m "feat: AI settings UI with stream selection complete"
```
