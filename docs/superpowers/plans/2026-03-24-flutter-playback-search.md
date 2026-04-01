# Flutter NVR Client — Plan 3: Playback + Search

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Add HLS recording playback with timeline, multi-camera synchronized playback, AI semantic clip search, and saved clips management to the Flutter NVR client.

**Architecture:** `media_kit` handles HLS video playback for recordings. A shared `PlaybackController` synchronizes multiple cameras. The timeline widget uses CustomPainter for recording segments and motion event markers. Search calls the `/search` API and displays results as cards with inline playback.

**Tech Stack:** media_kit (HLS), Riverpod, CustomPainter, dio

**Spec:** `docs/superpowers/specs/2026-03-23-flutter-client-design.md` (Sections 6, 7)

**Prerequisite:** Plans 1 + 2 must be complete. The app has: auth, navigation, camera list, WebSocket notifications.

---

## File Structure

| File                                                          | Task | Purpose                                          |
| ------------------------------------------------------------- | ---- | ------------------------------------------------ |
| `clients/flutter/pubspec.yaml`                                | 1    | Add media_kit dependencies                       |
| `clients/flutter/lib/services/playback_service.dart`          | 1    | HLS URL builder + local RFC3339 formatter        |
| `clients/flutter/lib/models/recording.dart`                   | 2    | Recording segment + motion event models          |
| `clients/flutter/lib/models/search_result.dart`               | 2    | Search result model                              |
| `clients/flutter/lib/models/saved_clip.dart`                  | 2    | Saved clip model                                 |
| `clients/flutter/lib/providers/recordings_provider.dart`      | 3    | Recording segments, motion events, timeline data |
| `clients/flutter/lib/providers/search_provider.dart`          | 3    | Search state + results                           |
| `clients/flutter/lib/screens/playback/playback_screen.dart`   | 4    | Multi-camera playback page                       |
| `clients/flutter/lib/screens/playback/playback_controls.dart` | 4    | Play/pause/speed/seek bar                        |
| `clients/flutter/lib/screens/playback/timeline_widget.dart`   | 5    | Vertical timeline with events                    |
| `clients/flutter/lib/screens/playback/camera_player.dart`     | 4    | Single HLS camera player tile                    |
| `clients/flutter/lib/screens/search/clip_search_screen.dart`  | 6    | Search UI + results                              |
| `clients/flutter/lib/screens/search/search_result_card.dart`  | 6    | Result card with thumbnail + actions             |
| `clients/flutter/lib/screens/search/clip_player_sheet.dart`   | 6    | Bottom sheet inline clip player                  |

---

### Task 1: media_kit Dependency + Playback Service

**Files:**

- Modify: `clients/flutter/pubspec.yaml`
- Create: `clients/flutter/lib/services/playback_service.dart`

- [ ] **Step 1: Add media_kit to pubspec.yaml**

Add under `dependencies:`:

```yaml
media_kit: ^1.1.0
media_kit_video: ^1.2.0
media_kit_libs_video: ^1.0.0
```

Run:

```bash
cd clients/flutter && flutter pub get
```

Note: `media_kit_libs_video` bundles the native video libraries. On macOS it requires no extra setup. On Linux, `libmpv-dev` may be needed. On iOS/Android it works out of the box.

- [ ] **Step 2: Create playback service**

```dart
// clients/flutter/lib/services/playback_service.dart

/// Builds HLS playback URLs for MediaMTX recording server.
class PlaybackService {
  final String serverUrl;

  PlaybackService({required this.serverUrl});

  /// Build HLS playback URL for a camera's recording.
  /// [path] is the camera's mediamtx_path (e.g., "nvr/ad410")
  /// [start] is the playback start time
  /// [durationSecs] is how many seconds of recording to serve
  String playbackUrl(String path, DateTime start, {double durationSecs = 86400}) {
    final uri = Uri.parse(serverUrl);
    final startIso = toLocalRfc3339(start);
    return '${uri.scheme}://${uri.host}:9996/get?path=${Uri.encodeComponent(path)}&start=${Uri.encodeComponent(startIso)}&duration=$durationSecs';
  }

  /// Format DateTime as RFC3339 with local timezone offset.
  /// MediaMTX playback server matches against local-time file paths,
  /// so we must send local time, not UTC.
  static String toLocalRfc3339(DateTime d) {
    final offset = d.timeZoneOffset;
    final sign = offset.isNegative ? '-' : '+';
    final absOffset = offset.abs();
    final offH = absOffset.inHours.toString().padLeft(2, '0');
    final offM = (absOffset.inMinutes % 60).toString().padLeft(2, '0');
    return '${d.year}-${_p(d.month)}-${_p(d.day)}T${_p(d.hour)}:${_p(d.minute)}:${_p(d.second)}$sign$offH:$offM';
  }

  static String _p(int n) => n.toString().padLeft(2, '0');
}
```

- [ ] **Step 3: Verify**

```bash
cd clients/flutter && flutter analyze
```

- [ ] **Step 4: Commit**

```bash
git add clients/flutter/pubspec.yaml clients/flutter/pubspec.lock clients/flutter/lib/services/playback_service.dart
git commit -m "feat(flutter): add media_kit dependency and playback service"
```

---

### Task 2: Data Models — Recording, SearchResult, SavedClip

**Files:**

- Create: `clients/flutter/lib/models/recording.dart`
- Create: `clients/flutter/lib/models/search_result.dart`
- Create: `clients/flutter/lib/models/saved_clip.dart`

- [ ] **Step 1: Create recording models**

```dart
// clients/flutter/lib/models/recording.dart

/// A recording segment returned by GET /recordings
class RecordingSegment {
  final String start;

  RecordingSegment({required this.start});

  factory RecordingSegment.fromJson(Map<String, dynamic> json) {
    return RecordingSegment(start: json['start'] as String? ?? '');
  }

  DateTime? get startTime => DateTime.tryParse(start);
}

/// Motion event from GET /cameras/:id/motion-events
class MotionEvent {
  final int? id;
  final String cameraId;
  final String startedAt;
  final String? endedAt;
  final String? thumbnailPath;
  final String? eventType;
  final String? objectClass;
  final double? confidence;

  MotionEvent({
    this.id,
    required this.cameraId,
    required this.startedAt,
    this.endedAt,
    this.thumbnailPath,
    this.eventType,
    this.objectClass,
    this.confidence,
  });

  factory MotionEvent.fromJson(Map<String, dynamic> json) {
    return MotionEvent(
      id: json['id'] as int?,
      cameraId: json['camera_id'] as String? ?? '',
      startedAt: json['started_at'] as String? ?? '',
      endedAt: json['ended_at'] as String?,
      thumbnailPath: json['thumbnail_path'] as String?,
      eventType: json['event_type'] as String?,
      objectClass: json['object_class'] as String?,
      confidence: (json['confidence'] as num?)?.toDouble(),
    );
  }

  DateTime? get startTime => DateTime.tryParse(startedAt);
  DateTime? get endTime => endedAt != null ? DateTime.tryParse(endedAt!) : null;
}
```

- [ ] **Step 2: Create search result model**

```dart
// clients/flutter/lib/models/search_result.dart

class SearchResult {
  final int detectionId;
  final int eventId;
  final String cameraId;
  final String cameraName;
  final String className;
  final double confidence;
  final double similarity;
  final String frameTime;
  final String? thumbnailPath;

  SearchResult({
    required this.detectionId,
    required this.eventId,
    required this.cameraId,
    required this.cameraName,
    required this.className,
    required this.confidence,
    required this.similarity,
    required this.frameTime,
    this.thumbnailPath,
  });

  factory SearchResult.fromJson(Map<String, dynamic> json) {
    return SearchResult(
      detectionId: json['detection_id'] as int? ?? 0,
      eventId: json['event_id'] as int? ?? 0,
      cameraId: json['camera_id'] as String? ?? '',
      cameraName: json['camera_name'] as String? ?? '',
      className: json['class'] as String? ?? '',
      confidence: (json['confidence'] as num?)?.toDouble() ?? 0,
      similarity: (json['similarity'] as num?)?.toDouble() ?? 0,
      frameTime: json['frame_time'] as String? ?? '',
      thumbnailPath: json['thumbnail_path'] as String?,
    );
  }

  DateTime? get time => DateTime.tryParse(frameTime);
}
```

- [ ] **Step 3: Create saved clip model**

```dart
// clients/flutter/lib/models/saved_clip.dart

class SavedClip {
  final String id;
  final String cameraId;
  final String name;
  final String startTime;
  final String endTime;
  final String tags;
  final String notes;
  final String createdAt;

  SavedClip({
    required this.id,
    required this.cameraId,
    required this.name,
    required this.startTime,
    required this.endTime,
    this.tags = '',
    this.notes = '',
    this.createdAt = '',
  });

  factory SavedClip.fromJson(Map<String, dynamic> json) {
    return SavedClip(
      id: json['id'] as String? ?? '',
      cameraId: json['camera_id'] as String? ?? '',
      name: json['name'] as String? ?? '',
      startTime: json['start_time'] as String? ?? '',
      endTime: json['end_time'] as String? ?? '',
      tags: json['tags'] as String? ?? '',
      notes: json['notes'] as String? ?? '',
      createdAt: json['created_at'] as String? ?? '',
    );
  }
}
```

- [ ] **Step 4: Verify + commit**

```bash
cd clients/flutter && flutter analyze
git add clients/flutter/lib/models/
git commit -m "feat(flutter): add recording, search result, and saved clip models"
```

---

### Task 3: Riverpod Providers — Recordings + Search

**Files:**

- Create: `clients/flutter/lib/providers/recordings_provider.dart`
- Create: `clients/flutter/lib/providers/search_provider.dart`

- [ ] **Step 1: Create recordings provider**

```dart
// clients/flutter/lib/providers/recordings_provider.dart
import 'package:flutter_riverpod/flutter_riverpod.dart';
import '../models/recording.dart';
import '../services/api_client.dart';
import 'auth_provider.dart';

/// Fetch recording segments for a camera on a date.
/// Returns list of segment start times.
final recordingSegmentsProvider =
    FutureProvider.family<List<RecordingSegment>, ({String cameraId, String date})>((ref, params) async {
  final api = ref.watch(apiClientProvider);
  if (api == null) return [];

  final startDate = '${params.date}T00:00:00Z';
  final endDate = '${params.date}T23:59:59Z';
  final res = await api.get('/recordings', queryParameters: {
    'camera_id': params.cameraId,
    'start': startDate,
    'end': endDate,
  });

  final data = res.data;
  if (data is List) {
    return data.map((e) {
      final segments = (e['segments'] as List?)?.map((s) => RecordingSegment.fromJson(s as Map<String, dynamic>)).toList() ?? [];
      return segments;
    }).expand((s) => s).toList();
  }
  return [];
});

/// Fetch motion events for a camera on a date.
final motionEventsProvider =
    FutureProvider.family<List<MotionEvent>, ({String cameraId, String date})>((ref, params) async {
  final api = ref.watch(apiClientProvider);
  if (api == null) return [];

  final res = await api.get('/cameras/${params.cameraId}/motion-events', queryParameters: {
    'start': '${params.date}T00:00:00Z',
    'end': '${params.date}T23:59:59Z',
  });

  final data = res.data;
  if (data is List) {
    return data.map((e) => MotionEvent.fromJson(e as Map<String, dynamic>)).toList();
  }
  return [];
});
```

- [ ] **Step 2: Create search provider**

```dart
// clients/flutter/lib/providers/search_provider.dart
import 'package:flutter_riverpod/flutter_riverpod.dart';
import '../models/search_result.dart';
import '../models/saved_clip.dart';
import 'auth_provider.dart';

class SearchState {
  final String query;
  final List<SearchResult> results;
  final bool searching;
  final bool searched;
  final String? error;

  const SearchState({
    this.query = '',
    this.results = const [],
    this.searching = false,
    this.searched = false,
    this.error,
  });

  SearchState copyWith({
    String? query,
    List<SearchResult>? results,
    bool? searching,
    bool? searched,
    String? error,
  }) {
    return SearchState(
      query: query ?? this.query,
      results: results ?? this.results,
      searching: searching ?? this.searching,
      searched: searched ?? this.searched,
      error: error,
    );
  }
}

class SearchNotifier extends StateNotifier<SearchState> {
  final Ref _ref;

  SearchNotifier(this._ref) : super(const SearchState());

  Future<void> search(String query) async {
    if (query.trim().isEmpty) return;
    state = state.copyWith(query: query, searching: true, searched: true, error: null);

    final api = _ref.read(apiClientProvider);
    if (api == null) {
      state = state.copyWith(searching: false, error: 'Not authenticated');
      return;
    }

    try {
      final res = await api.get('/search', queryParameters: {'q': query.trim(), 'limit': '20'});
      final data = res.data as Map<String, dynamic>;
      final results = ((data['results'] as List?) ?? [])
          .map((r) => SearchResult.fromJson(r as Map<String, dynamic>))
          .toList();
      state = state.copyWith(results: results, searching: false);
    } catch (e) {
      state = state.copyWith(searching: false, error: 'Search failed: $e');
    }
  }

  void clear() {
    state = const SearchState();
  }
}

final searchProvider = StateNotifierProvider<SearchNotifier, SearchState>((ref) {
  return SearchNotifier(ref);
});

/// Fetch saved clips.
final savedClipsProvider = FutureProvider<List<SavedClip>>((ref) async {
  final api = ref.watch(apiClientProvider);
  if (api == null) return [];
  final res = await api.get('/saved-clips');
  final data = res.data;
  if (data is List) {
    return data.map((e) => SavedClip.fromJson(e as Map<String, dynamic>)).toList();
  }
  return [];
});
```

- [ ] **Step 3: Verify + commit**

```bash
cd clients/flutter && flutter analyze
git add clients/flutter/lib/providers/
git commit -m "feat(flutter): add recordings and search Riverpod providers"
```

---

### Task 4: Playback Screen + Camera Player + Controls

**Files:**

- Create: `clients/flutter/lib/screens/playback/playback_screen.dart`
- Create: `clients/flutter/lib/screens/playback/camera_player.dart`
- Create: `clients/flutter/lib/screens/playback/playback_controls.dart`
- Modify: `clients/flutter/lib/router/app_router.dart`

- [ ] **Step 1: Create camera player (single HLS video)**

```dart
// clients/flutter/lib/screens/playback/camera_player.dart
import 'package:flutter/material.dart';
import 'package:media_kit/media_kit.dart';
import 'package:media_kit_video/media_kit_video.dart';
import '../../theme/nvr_colors.dart';

class CameraPlayer extends StatefulWidget {
  final String url;
  final String cameraName;
  final double playbackSpeed;
  final bool playing;
  final VoidCallback? onReady;

  const CameraPlayer({
    super.key,
    required this.url,
    required this.cameraName,
    this.playbackSpeed = 1.0,
    this.playing = true,
    this.onReady,
  });

  @override
  State<CameraPlayer> createState() => CameraPlayerState();
}

class CameraPlayerState extends State<CameraPlayer> {
  late final Player _player;
  late final VideoController _controller;
  bool _ready = false;

  @override
  void initState() {
    super.initState();
    _player = Player();
    _controller = VideoController(_player);
    _player.stream.playing.listen((playing) {
      if (playing && !_ready) {
        _ready = true;
        widget.onReady?.call();
      }
    });
    _openUrl(widget.url);
  }

  Future<void> _openUrl(String url) async {
    await _player.open(Media(url));
    _player.setRate(widget.playbackSpeed);
    if (!widget.playing) {
      _player.pause();
    }
  }

  @override
  void didUpdateWidget(CameraPlayer old) {
    super.didUpdateWidget(old);
    if (widget.url != old.url) {
      _ready = false;
      _openUrl(widget.url);
    }
    if (widget.playbackSpeed != old.playbackSpeed) {
      _player.setRate(widget.playbackSpeed);
    }
    if (widget.playing != old.playing) {
      widget.playing ? _player.play() : _player.pause();
    }
  }

  void seekTo(Duration position) => _player.seek(position);

  @override
  void dispose() {
    _player.dispose();
    super.dispose();
  }

  @override
  Widget build(BuildContext context) {
    return Card(
      clipBehavior: Clip.antiAlias,
      child: Stack(
        fit: StackFit.expand,
        children: [
          Video(controller: _controller, fill: Colors.black),
          Positioned(
            left: 8,
            bottom: 8,
            child: Container(
              padding: const EdgeInsets.symmetric(horizontal: 8, vertical: 3),
              decoration: BoxDecoration(color: Colors.black54, borderRadius: BorderRadius.circular(4)),
              child: Text(widget.cameraName, style: const TextStyle(color: Colors.white, fontSize: 11)),
            ),
          ),
        ],
      ),
    );
  }
}
```

- [ ] **Step 2: Create playback controls**

```dart
// clients/flutter/lib/screens/playback/playback_controls.dart
import 'package:flutter/material.dart';
import '../../theme/nvr_colors.dart';

class PlaybackControls extends StatelessWidget {
  final bool playing;
  final double speed;
  final VoidCallback onPlayPause;
  final ValueChanged<double> onSpeedChange;
  final VoidCallback onSeekBack;
  final VoidCallback onSeekForward;

  const PlaybackControls({
    super.key,
    required this.playing,
    required this.speed,
    required this.onPlayPause,
    required this.onSpeedChange,
    required this.onSeekBack,
    required this.onSeekForward,
  });

  static const _speeds = [0.5, 1.0, 2.0, 4.0, 8.0];

  @override
  Widget build(BuildContext context) {
    return Container(
      padding: const EdgeInsets.symmetric(horizontal: 16, vertical: 8),
      decoration: BoxDecoration(
        color: NvrColors.bgSecondary,
        border: Border(top: BorderSide(color: NvrColors.border)),
      ),
      child: Row(
        mainAxisAlignment: MainAxisAlignment.center,
        children: [
          IconButton(
            icon: const Icon(Icons.replay_10),
            onPressed: onSeekBack,
            tooltip: 'Back 10s',
          ),
          IconButton(
            icon: Icon(playing ? Icons.pause : Icons.play_arrow, size: 32),
            onPressed: onPlayPause,
            color: NvrColors.accent,
          ),
          IconButton(
            icon: const Icon(Icons.forward_10),
            onPressed: onSeekForward,
            tooltip: 'Forward 10s',
          ),
          const SizedBox(width: 16),
          DropdownButton<double>(
            value: speed,
            dropdownColor: NvrColors.bgSecondary,
            underline: const SizedBox.shrink(),
            style: const TextStyle(color: NvrColors.textPrimary, fontSize: 13),
            items: _speeds.map((s) => DropdownMenuItem(
              value: s,
              child: Text('${s}x'),
            )).toList(),
            onChanged: (v) { if (v != null) onSpeedChange(v); },
          ),
        ],
      ),
    );
  }
}
```

- [ ] **Step 3: Create playback screen**

```dart
// clients/flutter/lib/screens/playback/playback_screen.dart
import 'package:flutter/material.dart';
import 'package:flutter_riverpod/flutter_riverpod.dart';

import '../../providers/cameras_provider.dart';
import '../../providers/auth_provider.dart';
import '../../models/camera.dart';
import '../../services/playback_service.dart';
import '../../theme/nvr_colors.dart';
import '../../widgets/notification_bell.dart';
import 'camera_player.dart';
import 'playback_controls.dart';
import 'timeline_widget.dart';

class PlaybackScreen extends ConsumerStatefulWidget {
  const PlaybackScreen({super.key});

  @override
  ConsumerState<PlaybackScreen> createState() => _PlaybackScreenState();
}

class _PlaybackScreenState extends ConsumerState<PlaybackScreen> {
  final List<Camera> _selectedCameras = [];
  DateTime _playbackTime = DateTime.now().subtract(const Duration(hours: 1));
  String _selectedDate = '';
  bool _playing = true;
  double _speed = 1.0;
  final Map<String, GlobalKey<CameraPlayerState>> _playerKeys = {};

  @override
  void initState() {
    super.initState();
    final now = DateTime.now();
    _selectedDate = '${now.year}-${now.month.toString().padLeft(2, '0')}-${now.day.toString().padLeft(2, '0')}';
  }

  PlaybackService? _getPlaybackService() {
    final auth = ref.read(authProvider);
    if (auth.serverUrl == null) return null;
    return PlaybackService(serverUrl: auth.serverUrl!);
  }

  void _handleSeek(DateTime time) {
    setState(() {
      _playbackTime = time;
    });
  }

  void _seekRelative(int seconds) {
    _handleSeek(_playbackTime.add(Duration(seconds: seconds)));
  }

  @override
  Widget build(BuildContext context) {
    final camerasAsync = ref.watch(camerasProvider);
    final playback = _getPlaybackService();

    return Scaffold(
      appBar: AppBar(
        title: const Text('Playback'),
        actions: const [NotificationBell()],
      ),
      body: camerasAsync.when(
        data: (cameras) {
          if (_selectedCameras.isEmpty && cameras.isNotEmpty) {
            _selectedCameras.add(cameras.first);
          }

          return Column(
            children: [
              // Camera selector chips
              SizedBox(
                height: 48,
                child: ListView(
                  scrollDirection: Axis.horizontal,
                  padding: const EdgeInsets.symmetric(horizontal: 8),
                  children: cameras.map((cam) {
                    final selected = _selectedCameras.any((c) => c.id == cam.id);
                    return Padding(
                      padding: const EdgeInsets.symmetric(horizontal: 4, vertical: 8),
                      child: FilterChip(
                        label: Text(cam.name),
                        selected: selected,
                        onSelected: (v) {
                          setState(() {
                            if (v) {
                              _selectedCameras.add(cam);
                            } else {
                              _selectedCameras.removeWhere((c) => c.id == cam.id);
                            }
                          });
                        },
                      ),
                    );
                  }).toList(),
                ),
              ),
              // Video area + timeline
              Expanded(
                child: LayoutBuilder(
                  builder: (context, constraints) {
                    final isWide = constraints.maxWidth > 800;
                    if (isWide) {
                      // Desktop: videos left, timeline right
                      return Row(
                        children: [
                          Expanded(flex: 3, child: _buildVideoGrid(playback)),
                          SizedBox(
                            width: 200,
                            child: TimelineWidget(
                              date: _selectedDate,
                              cameraIds: _selectedCameras.map((c) => c.id).toList(),
                              playbackTime: _playbackTime,
                              onSeek: _handleSeek,
                            ),
                          ),
                        ],
                      );
                    }
                    // Mobile: videos top, timeline below
                    return Column(
                      children: [
                        Expanded(flex: 2, child: _buildVideoGrid(playback)),
                        SizedBox(
                          height: 200,
                          child: TimelineWidget(
                            date: _selectedDate,
                            cameraIds: _selectedCameras.map((c) => c.id).toList(),
                            playbackTime: _playbackTime,
                            onSeek: _handleSeek,
                            horizontal: true,
                          ),
                        ),
                      ],
                    );
                  },
                ),
              ),
              // Controls
              PlaybackControls(
                playing: _playing,
                speed: _speed,
                onPlayPause: () => setState(() => _playing = !_playing),
                onSpeedChange: (s) => setState(() => _speed = s),
                onSeekBack: () => _seekRelative(-10),
                onSeekForward: () => _seekRelative(10),
              ),
            ],
          );
        },
        loading: () => const Center(child: CircularProgressIndicator()),
        error: (e, _) => Center(child: Text('Error: $e')),
      ),
    );
  }

  Widget _buildVideoGrid(PlaybackService? playback) {
    if (_selectedCameras.isEmpty || playback == null) {
      return const Center(child: Text('Select cameras to playback', style: TextStyle(color: NvrColors.textMuted)));
    }

    final columns = _selectedCameras.length <= 1 ? 1 : 2;
    return GridView.builder(
      padding: const EdgeInsets.all(4),
      gridDelegate: SliverGridDelegateWithFixedCrossAxisCount(
        crossAxisCount: columns,
        crossAxisSpacing: 4,
        mainAxisSpacing: 4,
        childAspectRatio: 16 / 9,
      ),
      itemCount: _selectedCameras.length,
      itemBuilder: (_, i) {
        final cam = _selectedCameras[i];
        final key = _playerKeys.putIfAbsent(cam.id, () => GlobalKey<CameraPlayerState>());
        final url = playback.playbackUrl(cam.mediamtxPath, _playbackTime);
        return CameraPlayer(
          key: key,
          url: url,
          cameraName: cam.name,
          playbackSpeed: _speed,
          playing: _playing,
        );
      },
    );
  }
}
```

- [ ] **Step 4: Update router**

In `clients/flutter/lib/router/app_router.dart`, replace the `/playback` route:

```dart
GoRoute(path: '/playback', builder: (_, __) => const PlaybackScreen()),
```

Add import: `import '../screens/playback/playback_screen.dart';`

- [ ] **Step 5: Verify + commit**

```bash
cd clients/flutter && flutter analyze
git add clients/flutter/lib/screens/playback/ clients/flutter/lib/router/app_router.dart
git commit -m "feat(flutter): add playback screen with multi-camera HLS and speed controls"
```

---

### Task 5: Timeline Widget

**Files:**

- Create: `clients/flutter/lib/screens/playback/timeline_widget.dart`

- [ ] **Step 1: Create vertical/horizontal timeline**

```dart
// clients/flutter/lib/screens/playback/timeline_widget.dart
import 'package:flutter/material.dart';
import 'package:flutter_riverpod/flutter_riverpod.dart';

import '../../providers/recordings_provider.dart';
import '../../models/recording.dart';
import '../../theme/nvr_colors.dart';

class TimelineWidget extends ConsumerWidget {
  final String date;
  final List<String> cameraIds;
  final DateTime playbackTime;
  final ValueChanged<DateTime> onSeek;
  final bool horizontal;

  const TimelineWidget({
    super.key,
    required this.date,
    required this.cameraIds,
    required this.playbackTime,
    required this.onSeek,
    this.horizontal = false,
  });

  @override
  Widget build(BuildContext context, WidgetRef ref) {
    // Load motion events for first camera
    final eventsAsync = cameraIds.isNotEmpty
        ? ref.watch(motionEventsProvider((cameraId: cameraIds.first, date: date)))
        : const AsyncValue<List<MotionEvent>>.data([]);

    return eventsAsync.when(
      data: (events) => _TimelineCanvas(
        date: date,
        events: events,
        playbackTime: playbackTime,
        onSeek: onSeek,
        horizontal: horizontal,
      ),
      loading: () => const Center(child: CircularProgressIndicator(strokeWidth: 2)),
      error: (_, __) => const Center(child: Text('Timeline error', style: TextStyle(color: NvrColors.textMuted, fontSize: 12))),
    );
  }
}

class _TimelineCanvas extends StatelessWidget {
  final String date;
  final List<MotionEvent> events;
  final DateTime playbackTime;
  final ValueChanged<DateTime> onSeek;
  final bool horizontal;

  const _TimelineCanvas({
    required this.date,
    required this.events,
    required this.playbackTime,
    required this.onSeek,
    required this.horizontal,
  });

  @override
  Widget build(BuildContext context) {
    return GestureDetector(
      onTapDown: (details) {
        final box = context.findRenderObject() as RenderBox;
        final size = box.size;
        final fraction = horizontal
            ? details.localPosition.dx / size.width
            : details.localPosition.dy / size.height;
        final dayStart = DateTime.parse('${date}T00:00:00');
        final ms = (fraction * 24 * 60 * 60 * 1000).toInt();
        onSeek(dayStart.add(Duration(milliseconds: ms)));
      },
      child: CustomPaint(
        painter: _TimelinePainter(
          date: date,
          events: events,
          playbackTime: playbackTime,
          horizontal: horizontal,
        ),
        size: Size.infinite,
      ),
    );
  }
}

class _TimelinePainter extends CustomPainter {
  final String date;
  final List<MotionEvent> events;
  final DateTime playbackTime;
  final bool horizontal;

  _TimelinePainter({
    required this.date,
    required this.events,
    required this.playbackTime,
    required this.horizontal,
  });

  @override
  void paint(Canvas canvas, Size size) {
    final dayStart = DateTime.parse('${date}T00:00:00');
    final dayMs = 24 * 60 * 60 * 1000.0;

    // Background
    canvas.drawRect(Rect.fromLTWH(0, 0, size.width, size.height),
        Paint()..color = NvrColors.bgPrimary);

    // Hour lines
    final hourPaint = Paint()..color = NvrColors.border..strokeWidth = 0.5;
    final textStyle = const TextStyle(color: NvrColors.textMuted, fontSize: 9);
    for (int h = 0; h < 24; h++) {
      final frac = h / 24.0;
      if (horizontal) {
        final x = frac * size.width;
        canvas.drawLine(Offset(x, 0), Offset(x, size.height), hourPaint);
      } else {
        final y = frac * size.height;
        canvas.drawLine(Offset(0, y), Offset(size.width, y), hourPaint);
        // Hour label
        final tp = TextPainter(text: TextSpan(text: '${h.toString().padLeft(2, '0')}:00', style: textStyle), textDirection: TextDirection.ltr);
        tp.layout();
        tp.paint(canvas, Offset(2, y + 2));
      }
    }

    // Motion events
    final eventPaint = Paint()..color = NvrColors.warning.withAlpha(100);
    for (final event in events) {
      final start = event.startTime;
      if (start == null) continue;
      final startFrac = start.difference(dayStart).inMilliseconds / dayMs;
      final endFrac = event.endTime != null
          ? event.endTime!.difference(dayStart).inMilliseconds / dayMs
          : startFrac + 0.002; // thin line if no end

      if (horizontal) {
        canvas.drawRect(
          Rect.fromLTRB(startFrac * size.width, 0, endFrac * size.width, size.height),
          eventPaint,
        );
      } else {
        canvas.drawRect(
          Rect.fromLTRB(size.width * 0.3, startFrac * size.height, size.width, endFrac * size.height),
          eventPaint,
        );
      }
    }

    // Playback position marker
    final posFrac = playbackTime.difference(dayStart).inMilliseconds / dayMs;
    if (posFrac >= 0 && posFrac <= 1) {
      final markerPaint = Paint()..color = NvrColors.accent..strokeWidth = 2;
      if (horizontal) {
        final x = posFrac * size.width;
        canvas.drawLine(Offset(x, 0), Offset(x, size.height), markerPaint);
      } else {
        final y = posFrac * size.height;
        canvas.drawLine(Offset(0, y), Offset(size.width, y), markerPaint);
        // Triangle marker
        final path = Path()
          ..moveTo(0, y - 4)
          ..lineTo(8, y)
          ..lineTo(0, y + 4)
          ..close();
        canvas.drawPath(path, Paint()..color = NvrColors.accent);
      }
    }
  }

  @override
  bool shouldRepaint(_TimelinePainter old) =>
      playbackTime != old.playbackTime || events.length != old.events.length;
}
```

- [ ] **Step 2: Verify + commit**

```bash
cd clients/flutter && flutter analyze
git add clients/flutter/lib/screens/playback/timeline_widget.dart
git commit -m "feat(flutter): add vertical/horizontal timeline with motion event markers"
```

---

### Task 6: Clip Search Screen

**Files:**

- Create: `clients/flutter/lib/screens/search/clip_search_screen.dart`
- Create: `clients/flutter/lib/screens/search/search_result_card.dart`
- Create: `clients/flutter/lib/screens/search/clip_player_sheet.dart`
- Modify: `clients/flutter/lib/router/app_router.dart`

- [ ] **Step 1: Create search result card**

```dart
// clients/flutter/lib/screens/search/search_result_card.dart
import 'package:flutter/material.dart';
import '../../models/search_result.dart';
import '../../theme/nvr_colors.dart';

class SearchResultCard extends StatelessWidget {
  final SearchResult result;
  final String serverUrl;
  final VoidCallback onPlay;
  final VoidCallback? onSave;

  const SearchResultCard({
    super.key,
    required this.result,
    required this.serverUrl,
    required this.onPlay,
    this.onSave,
  });

  @override
  Widget build(BuildContext context) {
    final time = result.time;
    final timeStr = time != null
        ? '${time.month}/${time.day} ${time.hour}:${time.minute.toString().padLeft(2, '0')}'
        : 'Unknown time';

    return Card(
      child: InkWell(
        onTap: onPlay,
        borderRadius: BorderRadius.circular(12),
        child: Padding(
          padding: const EdgeInsets.all(12),
          child: Row(
            children: [
              // Thumbnail or icon
              Container(
                width: 64,
                height: 48,
                decoration: BoxDecoration(
                  color: NvrColors.bgTertiary,
                  borderRadius: BorderRadius.circular(6),
                ),
                child: result.thumbnailPath != null
                    ? ClipRRect(
                        borderRadius: BorderRadius.circular(6),
                        child: Image.network(
                          '$serverUrl/thumbnails/${result.thumbnailPath!.split('/').last}',
                          fit: BoxFit.cover,
                          errorBuilder: (_, __, ___) => Icon(_iconForClass(result.className), color: NvrColors.textMuted),
                        ),
                      )
                    : Icon(_iconForClass(result.className), color: NvrColors.textMuted),
              ),
              const SizedBox(width: 12),
              // Info
              Expanded(
                child: Column(
                  crossAxisAlignment: CrossAxisAlignment.start,
                  children: [
                    Row(
                      children: [
                        _ClassBadge(className: result.className),
                        const SizedBox(width: 8),
                        Text('${(result.confidence * 100).toInt()}%',
                            style: const TextStyle(color: NvrColors.textMuted, fontSize: 11)),
                      ],
                    ),
                    const SizedBox(height: 4),
                    Text(result.cameraName, style: const TextStyle(fontSize: 13, fontWeight: FontWeight.w500)),
                    Text(timeStr, style: const TextStyle(color: NvrColors.textMuted, fontSize: 11)),
                  ],
                ),
              ),
              // Actions
              if (onSave != null)
                IconButton(icon: const Icon(Icons.bookmark_border, size: 20), onPressed: onSave),
              const Icon(Icons.play_circle_outline, color: NvrColors.accent),
            ],
          ),
        ),
      ),
    );
  }

  IconData _iconForClass(String cls) {
    switch (cls) {
      case 'person': return Icons.person;
      case 'car': case 'truck': case 'bus': return Icons.directions_car;
      case 'cat': case 'dog': return Icons.pets;
      default: return Icons.visibility;
    }
  }
}

class _ClassBadge extends StatelessWidget {
  final String className;
  const _ClassBadge({required this.className});

  @override
  Widget build(BuildContext context) {
    return Container(
      padding: const EdgeInsets.symmetric(horizontal: 6, vertical: 2),
      decoration: BoxDecoration(
        color: NvrColors.accent.withAlpha(40),
        borderRadius: BorderRadius.circular(4),
      ),
      child: Text(
        className[0].toUpperCase() + className.substring(1),
        style: const TextStyle(color: NvrColors.accent, fontSize: 11, fontWeight: FontWeight.w600),
      ),
    );
  }
}
```

- [ ] **Step 2: Create clip player bottom sheet**

```dart
// clients/flutter/lib/screens/search/clip_player_sheet.dart
import 'package:flutter/material.dart';
import 'package:media_kit/media_kit.dart';
import 'package:media_kit_video/media_kit_video.dart';
import '../../theme/nvr_colors.dart';

class ClipPlayerSheet extends StatefulWidget {
  final String url;
  final String title;

  const ClipPlayerSheet({super.key, required this.url, required this.title});

  @override
  State<ClipPlayerSheet> createState() => _ClipPlayerSheetState();
}

class _ClipPlayerSheetState extends State<ClipPlayerSheet> {
  late final Player _player;
  late final VideoController _controller;

  @override
  void initState() {
    super.initState();
    _player = Player();
    _controller = VideoController(_player);
    _player.open(Media(widget.url));
  }

  @override
  void dispose() {
    _player.dispose();
    super.dispose();
  }

  @override
  Widget build(BuildContext context) {
    return Container(
      height: MediaQuery.sizeOf(context).height * 0.5,
      decoration: const BoxDecoration(
        color: NvrColors.bgSecondary,
        borderRadius: BorderRadius.vertical(top: Radius.circular(16)),
      ),
      child: Column(
        children: [
          // Header
          Padding(
            padding: const EdgeInsets.all(12),
            child: Row(
              children: [
                Expanded(child: Text(widget.title, style: const TextStyle(fontWeight: FontWeight.w600))),
                IconButton(
                  icon: const Icon(Icons.close),
                  onPressed: () => Navigator.pop(context),
                ),
              ],
            ),
          ),
          // Video
          Expanded(child: Video(controller: _controller, fill: Colors.black)),
        ],
      ),
    );
  }
}
```

- [ ] **Step 3: Create clip search screen**

```dart
// clients/flutter/lib/screens/search/clip_search_screen.dart
import 'package:flutter/material.dart';
import 'package:flutter_riverpod/flutter_riverpod.dart';

import '../../providers/search_provider.dart';
import '../../providers/auth_provider.dart';
import '../../providers/cameras_provider.dart';
import '../../services/playback_service.dart';
import '../../theme/nvr_colors.dart';
import '../../widgets/notification_bell.dart';
import 'search_result_card.dart';
import 'clip_player_sheet.dart';

class ClipSearchScreen extends ConsumerStatefulWidget {
  const ClipSearchScreen({super.key});

  @override
  ConsumerState<ClipSearchScreen> createState() => _ClipSearchScreenState();
}

class _ClipSearchScreenState extends ConsumerState<ClipSearchScreen> {
  final _queryController = TextEditingController();

  void _search() {
    ref.read(searchProvider.notifier).search(_queryController.text);
  }

  void _playResult(result) {
    final auth = ref.read(authProvider);
    if (auth.serverUrl == null) return;

    final cameras = ref.read(camerasProvider).valueOrNull ?? [];
    final camera = cameras.where((c) => c.id == result.cameraId).firstOrNull;
    if (camera == null) return;

    final playback = PlaybackService(serverUrl: auth.serverUrl!);
    final time = result.time ?? DateTime.now();
    final url = playback.playbackUrl(camera.mediamtxPath, time.subtract(const Duration(seconds: 5)), durationSecs: 30);

    showModalBottomSheet(
      context: context,
      isScrollControlled: true,
      backgroundColor: Colors.transparent,
      builder: (_) => ClipPlayerSheet(url: url, title: '${result.cameraName} — ${result.className}'),
    );
  }

  @override
  Widget build(BuildContext context) {
    final searchState = ref.watch(searchProvider);
    final auth = ref.watch(authProvider);

    return Scaffold(
      appBar: AppBar(
        title: const Text('Search'),
        actions: const [NotificationBell()],
      ),
      body: Column(
        children: [
          // Search bar
          Padding(
            padding: const EdgeInsets.all(12),
            child: Row(
              children: [
                Expanded(
                  child: TextField(
                    controller: _queryController,
                    decoration: const InputDecoration(
                      hintText: 'Search: "person", "car at night"...',
                      prefixIcon: Icon(Icons.search),
                    ),
                    onSubmitted: (_) => _search(),
                  ),
                ),
                const SizedBox(width: 8),
                ElevatedButton(
                  onPressed: searchState.searching ? null : _search,
                  child: searchState.searching
                      ? const SizedBox(width: 16, height: 16, child: CircularProgressIndicator(strokeWidth: 2))
                      : const Text('Search'),
                ),
              ],
            ),
          ),
          // Hint text
          if (!searchState.searched)
            const Padding(
              padding: EdgeInsets.symmetric(horizontal: 16),
              child: Text(
                'AI-powered search across all camera footage. Try "person", "car", or descriptive phrases.',
                style: TextStyle(color: NvrColors.textMuted, fontSize: 12),
              ),
            ),
          // Results
          if (searchState.error != null)
            Padding(
              padding: const EdgeInsets.all(16),
              child: Text(searchState.error!, style: const TextStyle(color: NvrColors.danger)),
            ),
          if (searchState.searched && searchState.results.isEmpty && !searchState.searching)
            const Padding(
              padding: EdgeInsets.all(32),
              child: Text('No results found', style: TextStyle(color: NvrColors.textMuted)),
            ),
          Expanded(
            child: ListView.builder(
              padding: const EdgeInsets.symmetric(horizontal: 8),
              itemCount: searchState.results.length,
              itemBuilder: (_, i) {
                final result = searchState.results[i];
                return SearchResultCard(
                  result: result,
                  serverUrl: auth.serverUrl ?? '',
                  onPlay: () => _playResult(result),
                );
              },
            ),
          ),
        ],
      ),
    );
  }
}
```

- [ ] **Step 4: Update router**

In `app_router.dart`, replace the `/search` route:

```dart
GoRoute(path: '/search', builder: (_, __) => const ClipSearchScreen()),
```

Add import: `import '../screens/search/clip_search_screen.dart';`

- [ ] **Step 5: Verify + commit**

```bash
cd clients/flutter && flutter analyze
git add clients/flutter/lib/screens/search/ clients/flutter/lib/router/app_router.dart
git commit -m "feat(flutter): add AI clip search with result cards and inline playback"
```

---

### Task 7: media_kit Initialization + Final Verification

**Files:**

- Modify: `clients/flutter/lib/main.dart`

- [ ] **Step 1: Initialize media_kit in main.dart**

`media_kit` requires initialization before use. Update `main.dart`:

```dart
import 'package:flutter/material.dart';
import 'package:flutter_riverpod/flutter_riverpod.dart';
import 'package:media_kit/media_kit.dart';
import 'app.dart';

void main() {
  WidgetsFlutterBinding.ensureInitialized();
  MediaKit.ensureInitialized();
  runApp(const ProviderScope(child: NvrApp()));
}
```

- [ ] **Step 2: Final verification**

```bash
cd clients/flutter && flutter analyze
```

- [ ] **Step 3: Commit**

```bash
git add clients/flutter/lib/main.dart
git commit -m "feat(flutter): initialize media_kit and finalize playback + search integration"
```
