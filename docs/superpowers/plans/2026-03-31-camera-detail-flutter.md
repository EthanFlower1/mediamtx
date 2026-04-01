# Camera Detail Screen Redesign — Flutter UI Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Redesign the camera detail screen with stream-centric collapsible cards containing roles + recording schedule + retention sliders with inline storage estimates, a single save button, and no auto-saves.

**Architecture:** The right column of the camera detail screen is rebuilt around collapsible stream cards. Each stream card bundles roles, recording schedule, and retention with live storage estimates from a new backend endpoint. All changes are held in local state until a single "Save Changes" button commits everything. The CameraStream model gains retention fields and a storage estimate service handles debounced API calls.

**Tech Stack:** Flutter, Riverpod, Dio HTTP client, Freezed (for Camera model)

---

## File Map

**Create:**

- `clients/flutter/lib/widgets/stream_card.dart` — Collapsible stream card widget

**Modify:**

- `clients/flutter/lib/models/camera_stream.dart` — Add retention fields
- `clients/flutter/lib/screens/cameras/camera_detail_screen.dart` — Rebuild right column, single save, remove auto-saves
- `clients/flutter/lib/models/camera.dart` — No changes needed (retention fields already exist)

---

### Task 1: CameraStream Model Update

**Files:**

- Modify: `clients/flutter/lib/models/camera_stream.dart`

- [ ] **Step 1: Add retention fields to CameraStream**

In `clients/flutter/lib/models/camera_stream.dart`, add two new fields to the class after `liveHeight`:

```dart
final int retentionDays;
final int eventRetentionDays;
```

Add them to the constructor with defaults:

```dart
this.retentionDays = 0,
this.eventRetentionDays = 0,
```

Add them to `fromJson`:

```dart
retentionDays: json['retention_days'] as int? ?? 0,
eventRetentionDays: json['event_retention_days'] as int? ?? 0,
```

- [ ] **Step 2: Verify it compiles**

Run: `cd clients/flutter && flutter analyze 2>&1 | grep -E "error" | head -5`
Expected: No errors

- [ ] **Step 3: Commit**

```bash
git add clients/flutter/lib/models/camera_stream.dart
git commit -m "feat(flutter): add retention fields to CameraStream model"
```

---

### Task 2: Stream Card Widget

**Files:**

- Create: `clients/flutter/lib/widgets/stream_card.dart`

This is a new widget that renders a single stream as a collapsible card. Collapsed shows a one-line summary; expanded shows roles, recording schedule dropdown, and retention sliders with inline storage estimates.

- [ ] **Step 1: Create the stream card widget**

Create `clients/flutter/lib/widgets/stream_card.dart`:

```dart
import 'package:flutter/material.dart';

import '../models/camera_stream.dart';
import '../models/schedule_template.dart';
import '../theme/nvr_colors.dart';
import '../theme/nvr_typography.dart';
import 'hud/analog_slider.dart';

/// Storage estimate data for a single stream, returned by the backend.
class StreamStorageEstimate {
  final String streamId;
  final int noEventBytes;
  final int eventBytes;
  final double eventFrequency;
  final String freqSource;
  final int totalBytes;

  const StreamStorageEstimate({
    required this.streamId,
    this.noEventBytes = 0,
    this.eventBytes = 0,
    this.eventFrequency = 0,
    this.freqSource = 'default',
    this.totalBytes = 0,
  });

  factory StreamStorageEstimate.fromJson(Map<String, dynamic> json) {
    return StreamStorageEstimate(
      streamId: json['stream_id'] as String? ?? '',
      noEventBytes: json['no_event_bytes'] as int? ?? 0,
      eventBytes: json['event_bytes'] as int? ?? 0,
      eventFrequency: (json['event_frequency'] as num?)?.toDouble() ?? 0,
      freqSource: json['event_frequency_source'] as String? ?? 'default',
      totalBytes: json['total_bytes'] as int? ?? 0,
    );
  }
}

/// Mutable state for a stream's settings, held locally until save.
class StreamSettingsState {
  List<String> roles;
  String templateId;
  double retentionDays;
  double eventRetentionDays;

  StreamSettingsState({
    required this.roles,
    required this.templateId,
    required this.retentionDays,
    required this.eventRetentionDays,
  });

  factory StreamSettingsState.fromStream(CameraStream stream, String templateId) {
    return StreamSettingsState(
      roles: stream.roleList,
      templateId: templateId,
      retentionDays: stream.retentionDays.toDouble(),
      eventRetentionDays: stream.eventRetentionDays.toDouble(),
    );
  }
}

/// A collapsible card that displays a camera stream's configuration:
/// roles, recording schedule, and retention with inline storage estimates.
class StreamCard extends StatelessWidget {
  final CameraStream stream;
  final StreamSettingsState settings;
  final StreamStorageEstimate? estimate;
  final List<ScheduleTemplate> templates;
  final bool expanded;
  final VoidCallback onToggleExpand;
  final ValueChanged<StreamSettingsState> onChanged;

  const StreamCard({
    super.key,
    required this.stream,
    required this.settings,
    this.estimate,
    required this.templates,
    required this.expanded,
    required this.onToggleExpand,
    required this.onChanged,
  });

  String _formatBytes(int bytes) {
    if (bytes <= 0) return '0 B';
    const units = ['B', 'KB', 'MB', 'GB', 'TB'];
    int i = 0;
    double val = bytes.toDouble();
    while (val >= 1024 && i < units.length - 1) {
      val /= 1024;
      i++;
    }
    return '${val.toStringAsFixed(i == 0 ? 0 : 1)} ${units[i]}';
  }

  String _summaryText() {
    final retDays = settings.retentionDays.round();
    final eventDays = settings.eventRetentionDays.round();
    final parts = <String>[];

    // Find template name.
    final tmpl = templates.where((t) => t.id == settings.templateId).firstOrNull;
    if (tmpl != null) {
      parts.add(tmpl.name);
    } else if (settings.templateId.isNotEmpty && settings.templateId != '__custom__') {
      parts.add('Custom');
    }

    if (retDays > 0 || eventDays > 0) {
      parts.add('${retDays}d/${eventDays}d');
    }

    return parts.join(' · ');
  }

  @override
  Widget build(BuildContext context) {
    final totalBytes = estimate?.totalBytes ?? 0;

    return Container(
      margin: const EdgeInsets.only(bottom: 8),
      decoration: BoxDecoration(
        color: NvrColors.bgSecondary,
        borderRadius: BorderRadius.circular(4),
        border: Border.all(
          color: expanded ? NvrColors.accent.withValues(alpha: 0.3) : NvrColors.border,
        ),
      ),
      child: Column(
        children: [
          // Header (always visible, tap to expand/collapse)
          GestureDetector(
            onTap: onToggleExpand,
            child: Container(
              padding: const EdgeInsets.symmetric(horizontal: 12, vertical: 10),
              decoration: expanded
                  ? const BoxDecoration(
                      border: Border(bottom: BorderSide(color: NvrColors.border)),
                    )
                  : null,
              child: Row(
                children: [
                  Icon(
                    expanded ? Icons.expand_more : Icons.chevron_right,
                    size: 16,
                    color: NvrColors.accent,
                  ),
                  const SizedBox(width: 6),
                  Expanded(
                    child: Row(
                      children: [
                        Text(
                          stream.name,
                          style: NvrTypography.monoData.copyWith(
                            color: NvrColors.textPrimary,
                            fontWeight: FontWeight.w600,
                          ),
                        ),
                        if (stream.resolutionLabel.isNotEmpty) ...[
                          const SizedBox(width: 8),
                          Text(
                            stream.resolutionLabel,
                            style: NvrTypography.monoLabel,
                          ),
                        ],
                      ],
                    ),
                  ),
                  if (!expanded && _summaryText().isNotEmpty) ...[
                    Text(
                      _summaryText(),
                      style: NvrTypography.monoLabel,
                    ),
                    const SizedBox(width: 8),
                  ],
                  Text(
                    totalBytes > 0 ? '~${_formatBytes(totalBytes)}' : '',
                    style: NvrTypography.monoData.copyWith(
                      color: NvrColors.accent,
                      fontWeight: FontWeight.w600,
                      fontSize: 11,
                    ),
                  ),
                ],
              ),
            ),
          ),

          // Expanded content
          if (expanded)
            Padding(
              padding: const EdgeInsets.all(12),
              child: Column(
                crossAxisAlignment: CrossAxisAlignment.start,
                children: [
                  _buildRoles(),
                  const SizedBox(height: 14),
                  _buildSchedule(),
                  const SizedBox(height: 14),
                  _buildRetention(),
                ],
              ),
            ),
        ],
      ),
    );
  }

  Widget _buildRoles() {
    return Column(
      crossAxisAlignment: CrossAxisAlignment.start,
      children: [
        Text('ROLES', style: NvrTypography.monoLabel),
        const SizedBox(height: 6),
        Wrap(
          spacing: 5,
          runSpacing: 5,
          children: ['live_view', 'recording', 'ai_detection', 'mobile'].map((role) {
            final active = settings.roles.contains(role);
            return GestureDetector(
              onTap: () {
                final newRoles = List<String>.from(settings.roles);
                if (active) {
                  newRoles.remove(role);
                } else {
                  newRoles.add(role);
                }
                onChanged(StreamSettingsState(
                  roles: newRoles,
                  templateId: settings.templateId,
                  retentionDays: settings.retentionDays,
                  eventRetentionDays: settings.eventRetentionDays,
                ));
              },
              child: Container(
                padding: const EdgeInsets.symmetric(horizontal: 7, vertical: 3),
                decoration: BoxDecoration(
                  color: active
                      ? NvrColors.accent.withValues(alpha: 0.15)
                      : Colors.transparent,
                  borderRadius: BorderRadius.circular(3),
                  border: Border.all(
                    color: active
                        ? NvrColors.accent.withValues(alpha: 0.3)
                        : NvrColors.border,
                  ),
                ),
                child: Text(
                  role.replaceAll('_', ' ').toUpperCase(),
                  style: TextStyle(
                    fontFamily: 'JetBrainsMono',
                    fontSize: 9,
                    color: active ? NvrColors.accent : NvrColors.textMuted,
                    letterSpacing: 0.5,
                  ),
                ),
              ),
            );
          }).toList(),
        ),
      ],
    );
  }

  Widget _buildSchedule() {
    final validValue = settings.templateId == '__custom__'
        ? '__custom__'
        : (templates.any((t) => t.id == settings.templateId) ? settings.templateId : '');

    return Column(
      crossAxisAlignment: CrossAxisAlignment.start,
      children: [
        Text('RECORDING SCHEDULE', style: NvrTypography.monoLabel),
        const SizedBox(height: 6),
        DropdownButtonFormField<String>(
          value: validValue,
          dropdownColor: NvrColors.bgTertiary,
          style: NvrTypography.monoData,
          isExpanded: true,
          decoration: InputDecoration(
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
              child: Text('None', style: NvrTypography.monoData),
            ),
            ...templates.map((t) => DropdownMenuItem(
              value: t.id,
              child: Text('${t.name} (${t.description})', style: NvrTypography.monoData),
            )),
            if (validValue == '__custom__')
              const DropdownMenuItem(
                value: '__custom__',
                child: Text('Custom', style: TextStyle(
                  fontFamily: 'JetBrainsMono',
                  fontSize: 12,
                  fontStyle: FontStyle.italic,
                  color: Color(0xFF737373),
                )),
              ),
          ],
          onChanged: (v) {
            if (v != null && v != '__custom__') {
              onChanged(StreamSettingsState(
                roles: settings.roles,
                templateId: v,
                retentionDays: settings.retentionDays,
                eventRetentionDays: settings.eventRetentionDays,
              ));
            }
          },
        ),
      ],
    );
  }

  Widget _buildRetention() {
    return Column(
      crossAxisAlignment: CrossAxisAlignment.start,
      children: [
        Text('RETENTION', style: NvrTypography.monoLabel),
        const SizedBox(height: 8),
        // No-Event slider
        AnalogSlider(
          label: 'NO-EVENT RECORDINGS',
          value: settings.retentionDays,
          min: 0,
          max: 90,
          onChanged: (v) => onChanged(StreamSettingsState(
            roles: settings.roles,
            templateId: settings.templateId,
            retentionDays: v,
            eventRetentionDays: settings.eventRetentionDays,
          )),
          valueFormatter: (v) => v.round() == 0 ? 'OFF' : '${v.round()} DAYS',
        ),
        if (estimate != null)
          Padding(
            padding: const EdgeInsets.only(top: 2, right: 4),
            child: Align(
              alignment: Alignment.centerRight,
              child: Text(
                '~${_formatBytes(estimate!.noEventBytes)}',
                style: NvrTypography.monoLabel.copyWith(
                  fontSize: 9,
                  color: NvrColors.accent,
                ),
              ),
            ),
          ),
        const SizedBox(height: 12),
        // Event slider
        AnalogSlider(
          label: 'EVENT RECORDINGS',
          value: settings.eventRetentionDays,
          min: 0,
          max: 730,
          onChanged: (v) => onChanged(StreamSettingsState(
            roles: settings.roles,
            templateId: settings.templateId,
            retentionDays: settings.retentionDays,
            eventRetentionDays: v,
          )),
          valueFormatter: (v) => v.round() == 0 ? 'OFF' : '${v.round()} DAYS',
        ),
        if (estimate != null)
          Padding(
            padding: const EdgeInsets.only(top: 2, right: 4),
            child: Align(
              alignment: Alignment.centerRight,
              child: Text.rich(
                TextSpan(
                  children: [
                    TextSpan(
                      text: '~${_formatBytes(estimate!.eventBytes)}',
                      style: NvrTypography.monoLabel.copyWith(
                        fontSize: 9,
                        color: NvrColors.accent,
                      ),
                    ),
                    if (estimate!.eventFrequency > 0)
                      TextSpan(
                        text: ' · ${estimate!.eventFrequency.toStringAsFixed(1)} events/day${estimate!.freqSource == "default" ? " (est)" : ""}',
                        style: NvrTypography.monoLabel.copyWith(
                          fontSize: 9,
                          color: NvrColors.textMuted,
                        ),
                      ),
                  ],
                ),
              ),
            ),
          ),
      ],
    );
  }
}
```

- [ ] **Step 2: Verify it compiles**

Run: `cd clients/flutter && flutter analyze 2>&1 | grep -E "error" | head -5`
Expected: No errors

- [ ] **Step 3: Commit**

```bash
git add clients/flutter/lib/widgets/stream_card.dart
git commit -m "feat(flutter): add collapsible stream card widget with storage estimates"
```

---

### Task 3: Camera Detail Screen Refactor

**Files:**

- Modify: `clients/flutter/lib/screens/cameras/camera_detail_screen.dart`

This is the main task — rebuilding the right column. The changes are:

1. Add stream settings state tracking (replaces separate retention/role/schedule state)
2. Add storage estimate fetching with debounce
3. Remove auto-saves from `_toggleRole` and `_assignSchedule`
4. Remove the separate AI save button
5. Rebuild `_buildRightColumn` with stream cards → AI → Advanced → single Save
6. Update the save button to save everything in one action

- [ ] **Step 1: Add imports for StreamCard**

At the top of `camera_detail_screen.dart`, add after the existing widget imports:

```dart
import '../../widgets/stream_card.dart';
```

- [ ] **Step 2: Add new state variables**

Replace the retention state variables:

```dart
// ── Retention ───────────────────────────────────────────────────────────
double _retentionDays = 30;
double _eventRetentionDays = 0;
```

With stream settings state + storage estimates:

```dart
// ── Per-stream settings (held locally until save) ────────────────────
Map<String, StreamSettingsState> _streamSettings = {};
Set<String> _expandedStreams = {};
Map<String, StreamStorageEstimate> _storageEstimates = {};
```

- [ ] **Step 3: Add storage estimate fetching**

Add a debounce timer field after the loading state fields (around line 67):

```dart
// ── Storage estimate debounce ──────────────────────────────────────────
Timer? _estimateTimer;
```

Add the `dart:async` import at the top if not already present.

Add a method to fetch storage estimates (after `_saveAdvanced`):

```dart
void _fetchStorageEstimates() {
  _estimateTimer?.cancel();
  _estimateTimer = Timer(const Duration(milliseconds: 300), () async {
    final api = ref.read(apiClientProvider);
    if (api == null || _camera == null) return;

    // Use the first stream's retention as representative for the estimate query.
    // The endpoint returns per-stream estimates based on bitrate.
    int retDays = 0;
    int eventDays = 0;
    for (final s in _streamSettings.values) {
      if (s.retentionDays > retDays) retDays = s.retentionDays.round();
      if (s.eventRetentionDays > eventDays) eventDays = s.eventRetentionDays.round();
    }

    try {
      final res = await api.get('/cameras/${widget.cameraId}/storage-estimate',
        queryParameters: {
          'retention_days': retDays,
          'event_retention_days': eventDays,
        },
      );
      final data = res.data as Map<String, dynamic>;
      final streams = (data['streams'] as List<dynamic>? ?? []);
      final estimates = <String, StreamStorageEstimate>{};
      for (final s in streams) {
        final est = StreamStorageEstimate.fromJson(s as Map<String, dynamic>);
        estimates[est.streamId] = est;
      }
      if (mounted) {
        setState(() => _storageEstimates = estimates);
      }
    } catch (_) {
      // Silently fail — estimates are optional.
    }
  });
}
```

- [ ] **Step 4: Initialize stream settings in \_fetchCamera**

In the `_fetchCamera` method, after the streams are loaded and `_streamTemplateMap` is built (around line 174), add initialization of `_streamSettings`:

Find the existing line:

```dart
        _retentionDays = camera.retentionDays.toDouble().clamp(0, 90);
        _eventRetentionDays = camera.eventRetentionDays.toDouble().clamp(0, 730);
```

Replace with:

```dart
        // Camera-level retention is kept for backward compat display only.
```

Then, after the recording rules are loaded (after the `_streamTemplateMap` is built), add:

```dart
        // Initialize per-stream settings state.
        final newSettings = <String, StreamSettingsState>{};
        for (final stream in _streams) {
          final tmplId = _streamTemplateMap[stream.id] ?? '';
          newSettings[stream.id] = StreamSettingsState.fromStream(stream, tmplId);
        }
        _streamSettings = newSettings;
        _fetchStorageEstimates();
```

- [ ] **Step 5: Remove auto-save from \_toggleRole**

Replace the entire `_toggleRole` method with a no-op — role toggling is now handled locally in the StreamCard widget via `onChanged`. The method can be removed entirely. Also remove `_assignSchedule` since schedule changes are also local now.

Actually, keep both methods but they are no longer called directly from UI. They'll be called during save instead. Let's leave them for now and change how save works in Step 7.

- [ ] **Step 6: Rebuild \_buildRightColumn**

Replace the entire `_buildRightColumn` method:

```dart
Widget _buildRightColumn(Camera camera) {
  return Column(
    crossAxisAlignment: CrossAxisAlignment.stretch,
    children: [
      // ── Streams section ──
      if (_streams.isNotEmpty) ...[
        Padding(
          padding: const EdgeInsets.only(bottom: 10),
          child: Text('STREAMS', style: NvrTypography.monoSection),
        ),
        for (final stream in _streams)
          StreamCard(
            stream: stream,
            settings: _streamSettings[stream.id] ?? StreamSettingsState.fromStream(stream, ''),
            estimate: _storageEstimates[stream.id],
            templates: _templates,
            expanded: _expandedStreams.contains(stream.id),
            onToggleExpand: () {
              setState(() {
                if (_expandedStreams.contains(stream.id)) {
                  _expandedStreams.remove(stream.id);
                } else {
                  _expandedStreams.add(stream.id);
                }
              });
            },
            onChanged: (newState) {
              setState(() {
                _streamSettings[stream.id] = newState;
              });
              _fetchStorageEstimates();
            },
          ),
      ],

      const SizedBox(height: 12),

      // ── AI Detection section ──
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
                value: _streams.any((s) => s.id == _aiStreamId) || _aiStreamId.isEmpty
                    ? _aiStreamId
                    : '',
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
                  const DropdownMenuItem(value: '', child: Text('Auto', style: NvrTypography.monoData)),
                  ..._streams.map((s) => DropdownMenuItem(
                    value: s.id,
                    child: Text(s.displayLabel, style: NvrTypography.monoData),
                  )),
                ],
                onChanged: (v) {
                  if (v != null) setState(() => _aiStreamId = v);
                },
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
            ],
          ],
        ),
      ),

      const SizedBox(height: 12),

      // ── Connection info ──
      _SectionCard(
        header: 'CONNECTION',
        child: Column(
          children: [
            _KvRow(label: 'Protocol', value: camera.rtspUrl.startsWith('rtsp') ? 'RTSP' : 'HTTP'),
            const SizedBox(height: 6),
            _KvRow(label: 'ONVIF', value: camera.onvifEndpoint.isEmpty ? 'Not configured' : 'Configured'),
          ],
        ),
      ),

      // ── Advanced sections ──
      if (_showAdvanced) ...[
        const SizedBox(height: 16),
        _buildAdvancedSections(camera),
      ],

      const SizedBox(height: 24),

      // ── Single save button ──
      HudButton(
        label: _savingGeneral || _savingAdvanced || _savingAi ? 'SAVING...' : 'SAVE CHANGES',
        onPressed: (_savingGeneral || _savingAdvanced || _savingAi)
            ? null
            : _saveAll,
      ),

      const SizedBox(height: 16),
    ],
  );
}
```

- [ ] **Step 7: Implement \_saveAll method**

Add a new `_saveAll` method that saves everything in sequence:

```dart
Future<void> _saveAll() async {
  final api = ref.read(apiClientProvider);
  if (api == null) return;

  setState(() {
    _savingGeneral = true;
    _savingAi = true;
    _savingAdvanced = true;
  });

  try {
    // 1. Save general settings (name, RTSP, ONVIF).
    await api.put('/cameras/${widget.cameraId}', data: {
      'name': _nameCtrl.text.trim(),
      'rtsp_url': _rtspCtrl.text.trim(),
      'onvif_endpoint': _onvifCtrl.text.trim(),
    });

    // 2. Save AI settings.
    await api.put('/cameras/${widget.cameraId}/ai', data: {
      'ai_enabled': _aiEnabled,
      'stream_id': _aiStreamId,
      'confidence': _confidence,
      'track_timeout': _trackTimeout.round(),
    });

    // 3. Save per-stream settings (roles, schedule, retention).
    for (final entry in _streamSettings.entries) {
      final streamId = entry.key;
      final state = entry.value;

      // Save roles.
      await api.put('/streams/$streamId/roles', data: {
        'roles': state.roles.join(','),
      });

      // Save schedule.
      final oldTemplateId = _streamTemplateMap[streamId] ?? '';
      if (state.templateId != oldTemplateId) {
        await api.put('/cameras/${widget.cameraId}/stream-schedule', data: {
          'stream_id': streamId,
          'template_id': state.templateId,
        });
      }

      // Save retention.
      await api.put('/streams/$streamId/retention', data: {
        'retention_days': state.retentionDays.round(),
        'event_retention_days': state.eventRetentionDays.round(),
      });
    }

    // 4. Refresh from server.
    await _fetchCamera();

    if (mounted) {
      ScaffoldMessenger.of(context).showSnackBar(
        const SnackBar(backgroundColor: NvrColors.success, content: Text('Saved')),
      );
    }
  } catch (e) {
    if (mounted) {
      ScaffoldMessenger.of(context).showSnackBar(
        SnackBar(backgroundColor: NvrColors.danger, content: Text('Error: $e')),
      );
    }
  } finally {
    if (mounted) {
      setState(() {
        _savingGeneral = false;
        _savingAi = false;
        _savingAdvanced = false;
      });
    }
  }
}
```

- [ ] **Step 8: Update retention stat tile**

In `_buildLeftColumn`, update the RETENTION stat tile to show a summary from stream settings:

```dart
_StatTile(
  label: 'RETENTION',
  value: _retentionSummary(),
  valueStyle: NvrTypography.monoDataLarge,
),
```

Add the helper method:

```dart
String _retentionSummary() {
  if (_streamSettings.isEmpty) return '--';
  final retentions = _streamSettings.values
      .map((s) => '${s.retentionDays.round()}d/${s.eventRetentionDays.round()}d')
      .toSet();
  if (retentions.length == 1) return retentions.first;
  return 'Mixed';
}
```

- [ ] **Step 9: Remove old \_saveGeneral, \_saveAi, \_saveAdvanced methods**

These are replaced by `_saveAll`. Remove:

- `_saveGeneral()` method
- `_saveAi()` method
- `_saveAdvanced()` method

Also remove `_buildStreamInfoCard` and `_buildScheduleDropdown` since they're replaced by `StreamCard`.

- [ ] **Step 10: Clean up dispose**

In `dispose()`, add:

```dart
_estimateTimer?.cancel();
```

- [ ] **Step 11: Verify it compiles and runs**

Run: `cd clients/flutter && flutter analyze 2>&1 | grep -E "error" | head -10`
Expected: No errors (warnings about unused variables are OK, fix them)

- [ ] **Step 12: Commit**

```bash
git add clients/flutter/lib/screens/cameras/camera_detail_screen.dart
git commit -m "feat(flutter): redesign camera detail with stream cards, single save, storage estimates"
```

---

### Task 4: Cleanup and Polish

**Files:**

- Modify: `clients/flutter/lib/screens/cameras/camera_detail_screen.dart`

- [ ] **Step 1: Remove dead code**

Remove any remaining references to the old patterns:

- Remove `_toggleRole` method (role toggling is now in StreamCard)
- Remove `_assignSchedule` method (schedule changes are in StreamCard)
- Remove the `_savingAdvanced` flag if unused after `_saveAll` refactor
- Remove unused `_retentionDays` and `_eventRetentionDays` state variables if any remain

- [ ] **Step 2: Verify the AI save button is gone**

Search the file for `SAVE AI SETTINGS` — it should not exist. The only save button should be the single `SAVE CHANGES` at the bottom.

- [ ] **Step 3: Verify no auto-saves remain**

Search for direct API calls outside of `_saveAll` and `_fetchCamera`:

- No `api.put` calls in `_toggleRole` (removed)
- No `api.put` calls in `_assignSchedule` (removed)
- No `api.put` calls in any `onChanged` callbacks

- [ ] **Step 4: Run flutter analyze**

Run: `cd clients/flutter && flutter analyze 2>&1 | grep -cE "error"`
Expected: 0

- [ ] **Step 5: Commit**

```bash
git add clients/flutter/lib/screens/cameras/camera_detail_screen.dart
git commit -m "refactor(flutter): remove dead code and auto-save patterns from camera detail"
```

---

## Summary

| Task | What it does                                                                       |
| ---- | ---------------------------------------------------------------------------------- |
| 1    | Add retention fields to CameraStream model                                         |
| 2    | Create StreamCard widget with collapsible roles + schedule + retention + estimates |
| 3    | Refactor camera detail screen: stream cards, single save, storage estimates        |
| 4    | Clean up dead code, verify no auto-saves remain                                    |
