import 'package:flutter/material.dart';
import 'package:flutter/services.dart';
import 'package:flutter_riverpod/flutter_riverpod.dart';
import 'package:go_router/go_router.dart';

import '../../models/camera.dart';
import '../../models/recording.dart';
import '../../models/bookmark.dart';
import '../../providers/bookmarks_provider.dart';
import '../../providers/cameras_provider.dart';
import '../../providers/auth_provider.dart';
import '../../providers/recordings_provider.dart';
import '../../providers/playback_session_provider.dart';
import '../../providers/timeline_intensity_provider.dart';
import '../../services/playback_service.dart';
import '../../theme/nvr_colors.dart';
import '../../theme/nvr_typography.dart';
import '../../utils/responsive.dart';

import 'camera_player.dart';
import 'controls/transport_bar.dart';
import 'export_clip_dialog.dart';
import 'playback_controller.dart';
import 'timeline/fixed_playhead_timeline.dart';
import 'timeline/mini_overview_bar.dart';
import 'timeline/timeline_viewport.dart';

class PlaybackScreen extends ConsumerStatefulWidget {
  final String? initialCameraId;
  final DateTime? initialTimestamp;

  const PlaybackScreen({
    super.key,
    this.initialCameraId,
    this.initialTimestamp,
  });

  /// Navigate to playback at a specific bookmark's camera and timestamp.
  static void navigateToBookmark(
    BuildContext context, {
    required String cameraId,
    required DateTime timestamp,
  }) {
    final uri = Uri(
      path: '/playback',
      queryParameters: {
        'cameraId': cameraId,
        'timestamp': timestamp.toUtc().toIso8601String(),
      },
    );
    GoRouter.of(context).go(uri.toString());
  }

  @override
  ConsumerState<PlaybackScreen> createState() => _PlaybackScreenState();
}

class _PlaybackScreenState extends ConsumerState<PlaybackScreen> {
  PlaybackController? _controller;
  late DateTime _selectedDate;
  late final Set<String> _selectedCameraIds;
  String _lastServerUrl = '';
  bool _appliedInitialBookmark = false;
  bool _restoredSession = false;

  final FocusNode _keyFocusNode = FocusNode();

  /// Timeline zoom preset index: {0: 24H, 1: 12H, 2: 4H, 3: 1H}.
  late int _zoomIndex;

  @override
  void initState() {
    super.initState();
    final session = ref.read(playbackSessionProvider);
    _sessionNotifier = ref.read(playbackSessionProvider.notifier);
    _selectedDate = session.selectedDate;
    _selectedCameraIds = {...session.selectedCameraIds};
    _zoomIndex = session.zoomIndex;
  }

  String get _dateKey =>
      '${_selectedDate.year}-${_selectedDate.month.toString().padLeft(2, '0')}-${_selectedDate.day.toString().padLeft(2, '0')}';

  PlaybackController _getController(String serverUrl) {
    if (_controller == null || _lastServerUrl != serverUrl) {
      _controller?.removeListener(_onControllerChanged);
      _controller?.dispose();
      _lastServerUrl = serverUrl;
      final authService = ref.read(authServiceProvider);
      _controller = PlaybackController(
        playbackService: PlaybackService(serverUrl: serverUrl),
        getAccessToken: () => authService.getAccessToken(),
      );
      _controller!.addListener(_onControllerChanged);
    }
    return _controller!;
  }

  void _onControllerChanged() {
    if (mounted) {
      setState(() {
        _selectedDate = _controller!.selectedDate;
      });
    }
  }

  // Cache the notifier so we can save in dispose (where ref is unavailable).
  PlaybackSessionNotifier? _sessionNotifier;

  void _saveSession() {
    final cameraIds = {..._selectedCameraIds};
    final date = _selectedDate;
    final position = _controller?.position ?? Duration.zero;
    final zoom = _zoomIndex;
    final notifier = _sessionNotifier;
    if (notifier == null) return;
    // Defer to avoid modifying provider during widget tree teardown.
    Future(() {
      notifier.save(
        cameraIds: cameraIds,
        date: date,
        position: position,
        zoomIndex: zoom,
      );
    });
  }

  // ── Keyboard shortcuts ─────────────────────────────────────────────
  /// Speed presets (mirrored from transport_bar.dart).
  static const _speedPresets = [0.25, 0.5, 1.0, 2.0, 4.0, 8.0, 16.0];

  void _onKeyEvent(KeyEvent event) {
    if (event is! KeyDownEvent && event is! KeyRepeatEvent) return;
    final ctrl = _controller;
    if (ctrl == null) return;

    switch (event.logicalKey) {
      case LogicalKeyboardKey.space:
        ctrl.togglePlayPause();
      case LogicalKeyboardKey.equal: // + or =
      case LogicalKeyboardKey.add:
      case LogicalKeyboardKey.numpadAdd:
        _changeSpeed(ctrl, 1);
      case LogicalKeyboardKey.minus:
      case LogicalKeyboardKey.numpadSubtract:
        _changeSpeed(ctrl, -1);
      case LogicalKeyboardKey.arrowLeft:
        ctrl.stepFrame(-1);
      case LogicalKeyboardKey.arrowRight:
        ctrl.stepFrame(1);
      case LogicalKeyboardKey.keyJ:
        ctrl.skipToPreviousEvent();
      case LogicalKeyboardKey.keyL:
        ctrl.skipToNextEvent();
      default:
        return;
    }
  }

  void _changeSpeed(PlaybackController ctrl, int direction) {
    final currentIndex = _speedPresets.indexWhere(
        (s) => (s - ctrl.speed).abs() < 0.001);
    final idx = currentIndex < 0 ? 2 : currentIndex;
    final newIdx = (idx + direction).clamp(0, _speedPresets.length - 1);
    ctrl.setSpeed(_speedPresets[newIdx]);
  }

  @override
  void deactivate() {
    // Save session and pause when navigating away.
    _saveSession();
    _controller?.removeListener(_onControllerChanged);
    _controller?.pause();
    _controller?.addListener(_onControllerChanged);
    super.deactivate();
  }

  @override
  void dispose() {
    _keyFocusNode.dispose();
    _controller?.removeListener(_onControllerChanged);
    _controller?.dispose();
    super.dispose();
  }

  // ── Date helpers ──────────────────────────────────────────────────

  String get _formattedDate =>
      '${_selectedDate.year}-${_selectedDate.month.toString().padLeft(2, '0')}-${_selectedDate.day.toString().padLeft(2, '0')}';

  Future<void> _pickDate(PlaybackController controller) async {
    final picked = await showDatePicker(
      context: context,
      initialDate: _selectedDate,
      firstDate: DateTime(2020),
      lastDate: DateTime.now(),
      builder: (context, child) => Theme(
        data: Theme.of(context).copyWith(
          colorScheme: ColorScheme.dark(
            primary: NvrColors.of(context).accent,
            surface: NvrColors.of(context).bgSecondary,
          ),
        ),
        child: child!,
      ),
    );
    if (picked != null) {
      setState(() => _selectedDate = picked);
      controller.setSelectedDate(picked);
    }
  }

  // ── Zoom mapping helpers ──────────────────────────────────────────

  static const _zoomPresets = TimelineZoom.values; // [1H, 30M, 10M, 5M]

  TimelineZoom get _currentZoom => _zoomPresets[_zoomIndex];

  void _onZoomChanged(int index) {
    setState(() => _zoomIndex = index.clamp(0, _zoomPresets.length - 1));
  }

  void _onTimelineZoomChanged(TimelineZoom zoom) {
    final idx = _zoomPresets.indexOf(zoom);
    if (idx >= 0) {
      setState(() => _zoomIndex = idx);
    }
  }

  void _showExportDialog(List<Camera> selected, PlaybackController controller) {
    if (selected.isEmpty) return;
    final camera = selected.first;
    final dayStart = DateTime(
        _selectedDate.year, _selectedDate.month, _selectedDate.day);

    showDialog(
      context: context,
      builder: (_) => ExportClipDialog(
        cameraId: camera.id,
        cameraName: camera.name,
        dayStart: dayStart,
        currentPosition: controller.position,
      ),
    );
  }

  // ── Build ─────────────────────────────────────────────────────────

  @override
  Widget build(BuildContext context) {
    final camerasAsync = ref.watch(camerasProvider);
    final auth = ref.watch(authProvider);
    final serverUrl = auth.serverUrl ?? '';
    final controller = _getController(serverUrl);

    return KeyboardListener(
      focusNode: _keyFocusNode,
      autofocus: true,
      onKeyEvent: _onKeyEvent,
      child: Scaffold(
        backgroundColor: NvrColors.of(context).bgPrimary,
        body: camerasAsync.when(
          loading: () => Center(
            child: CircularProgressIndicator(color: NvrColors.of(context).accent),
          ),
          error: (e, _) => Center(
            child: Text('Error: $e',
                style: TextStyle(color: NvrColors.of(context).danger)),
          ),
          data: (cameras) => _buildBody(cameras, controller),
        ),
      ),
    );
  }

  Widget _buildBody(List<Camera> cameras, PlaybackController controller) {
    if (cameras.isEmpty) {
      return Center(
        child: Text('No cameras configured.',
            style: TextStyle(color: NvrColors.of(context).textMuted)),
      );
    }

    // Apply initial bookmark navigation (camera + timestamp) once.
    if (!_appliedInitialBookmark && widget.initialCameraId != null) {
      _appliedInitialBookmark = true;
      final cameraExists =
          cameras.any((c) => c.id == widget.initialCameraId);
      if (cameraExists) {
        _selectedCameraIds.clear();
        _selectedCameraIds.add(widget.initialCameraId!);
        if (widget.initialTimestamp != null) {
          final ts = widget.initialTimestamp!;
          _selectedDate = DateTime(ts.year, ts.month, ts.day);
          controller.setSelectedDate(_selectedDate);
          // Seek to the time-of-day offset.
          final dayStart =
              DateTime(ts.year, ts.month, ts.day);
          final offset = ts.difference(dayStart);
          controller.seek(offset);
        }
      }
    }

    if (_selectedCameraIds.isEmpty && cameras.isNotEmpty) {
      _selectedCameraIds.add(cameras.first.id);
    }

    // Restore saved position once after segments load.
    if (!_restoredSession) {
      _restoredSession = true;
      final session = ref.read(playbackSessionProvider);
      if (session.position > Duration.zero && widget.initialCameraId == null) {
        final pos = session.position;
        WidgetsBinding.instance.addPostFrameCallback((_) {
          if (mounted && pos > Duration.zero) {
            controller.seek(pos);
          }
        });
      }
    }

    final pathMap = {for (final c in cameras) c.id: c.mediamtxPath};
    controller.setCameraPaths(pathMap);
    controller.setSelectedCameraIds(_selectedCameraIds.toList());

    final selected =
        cameras.where((c) => _selectedCameraIds.contains(c.id)).toList();

    // Fetch and merge recordings/events for all selected cameras.
    final allSegments = <RecordingSegment>[];
    final allEvents = <MotionEvent>[];
    final allIntensityBuckets = <IntensityBucket>[];
    final allBookmarks = <Bookmark>[];
    const intensityBucketSeconds = 60;

    for (final cam in selected) {
      final key = (cameraId: cam.id, date: _dateKey);
      final segAsync = ref.watch(recordingSegmentsProvider(key));
      final evtAsync = ref.watch(motionEventsProvider(key));
      final intensityKey = (
        cameraId: cam.id,
        date: _dateKey,
        bucketSeconds: intensityBucketSeconds,
      );
      final intensityAsync = ref.watch(intensityProvider(intensityKey));
      final bookmarksAsync = ref.watch(bookmarksProvider(key));

      allSegments.addAll(segAsync.valueOrNull ?? []);
      allEvents.addAll(evtAsync.valueOrNull ?? []);
      allIntensityBuckets.addAll(intensityAsync.valueOrNull ?? []);
      allBookmarks.addAll(bookmarksAsync.valueOrNull ?? []);
    }

    allSegments.sort((a, b) => a.startTime.compareTo(b.startTime));
    allEvents.sort((a, b) => a.startTime.compareTo(b.startTime));
    allBookmarks.sort((a, b) => a.timestamp.compareTo(b.timestamp));

    controller.setSegments(allSegments);
    controller.setEvents(allEvents);
    controller.setBookmarks(allBookmarks);

    final dayStart = DateTime(
        _selectedDate.year, _selectedDate.month, _selectedDate.day);

    // Compute a TimelineViewport for the MiniOverviewBar based on current
    // zoom and position so it can show the visible window indicator.
    final visibleDuration = _currentZoom.visibleDuration;
    final halfVisible = Duration(
        milliseconds: visibleDuration.inMilliseconds ~/ 2);
    final visStart = controller.position - halfVisible;
    final visEnd = controller.position + halfVisible;

    return Column(
      children: [
        // ── Top bar ───────────────────────────────────────────────────
        _TopBar(
          date: _formattedDate,
          onDateTap: () => _pickDate(controller),
          onExport: () => _showExportDialog(selected, controller),
        ),

        // ── Video area ────────────────────────────────────────────────
        Expanded(
          child: DragTarget<String>(
            onWillAcceptWithDetails: (details) =>
                !_selectedCameraIds.contains(details.data),
            onAcceptWithDetails: (details) {
              setState(() => _selectedCameraIds.add(details.data));
            },
            builder: (context, candidateData, rejectedData) {
              return _VideoGrid(
                cameras: selected,
                controller: controller,
                onRemoveCamera: (camId) {
                  if (_selectedCameraIds.length > 1) {
                    setState(() => _selectedCameraIds.remove(camId));
                  }
                },
              );
            },
          ),
        ),

        // ── Transport bar ─────────────────────────────────────────────
        TransportBar(
          isPlaying: controller.isPlaying,
          currentTime: controller.position,
          speed: controller.speed,
          zoomLevel: _zoomIndex,
          onPlayPause: controller.togglePlayPause,
          onStepBack: () => controller.stepFrame(-1),
          onStepForward: () => controller.stepFrame(1),
          onSkipPrevious: controller.skipToPreviousEvent,
          onSkipNext: controller.skipToNextEvent,
          onSpeedChanged: (s) => controller.setSpeed(s),
          onZoomChanged: _onZoomChanged,
        ),

        // ── Mini overview bar ─────────────────────────────────────────
        Padding(
          padding: const EdgeInsets.symmetric(horizontal: 12, vertical: 4),
          child: MiniOverviewBar(
            mainViewport: TimelineViewport(
              visibleStart: visStart < Duration.zero ? Duration.zero : visStart,
              visibleEnd: visEnd > const Duration(hours: 24)
                  ? const Duration(hours: 24)
                  : visEnd,
              widthPx: 400, // Approximate; will be recalculated internally.
            ),
            segments: allSegments,
            events: allEvents,
            dayStart: dayStart,
            position: controller.position,
            onViewportJump: (d) => controller.seek(d),
          ),
        ),

        // ── Fixed playhead timeline ───────────────────────────────────
        FixedPlayheadTimeline(
          currentPosition: controller.position,
          isPlaying: controller.isPlaying,
          playbackSpeed: controller.speed,
          segments: allSegments,
          events: allEvents,
          bookmarks: allBookmarks,
          intensityBuckets: allIntensityBuckets,
          bucketDuration: const Duration(seconds: intensityBucketSeconds),
          dayStart: dayStart,
          onPositionChanged: (d) => controller.seek(d),
          onDragStart: () => controller.pause(),
          onDragEnd: () => controller.play(),
          zoomLevel: _currentZoom,
          onZoomChanged: _onTimelineZoomChanged,
        ),

        // ── Bottom padding ────────────────────────────────────────────
        const SizedBox(height: 8),
      ],
    );
  }
}

// ── Top Bar ──────────────────────────────────────────────────────────────────

class _TopBar extends StatelessWidget {
  final String date;
  final VoidCallback onDateTap;
  final VoidCallback onExport;

  const _TopBar({
    required this.date,
    required this.onDateTap,
    required this.onExport,
  });

  @override
  Widget build(BuildContext context) {
    final isPhone = Responsive.isPhone(context);

    return Container(
      padding: EdgeInsets.only(
        top: MediaQuery.of(context).padding.top + 8,
        left: isPhone ? 12 : 16,
        right: isPhone ? 12 : 16,
        bottom: 8,
      ),
      decoration: BoxDecoration(
        color: NvrColors.of(context).bgSecondary,
        border: Border(bottom: BorderSide(color: NvrColors.of(context).border)),
      ),
      child: isPhone ? _buildMobileBar(context) : _buildDesktopBar(context),
    );
  }

  Widget _buildDesktopBar(BuildContext context) {
    return Row(
      children: [
        // Title
        Text('Playback', style: NvrTypography.of(context).pageTitle),
        const SizedBox(width: 20),

        // Date picker
        _DateButton(date: date, onTap: onDateTap),

        const Spacer(),

        // Export button
        _SecondaryButton(
          icon: Icons.download,
          label: 'Export',
          onTap: () {}, // TODO: wire export
        ),
        const SizedBox(width: 8),

        // Bookmark button
        _AccentButton(
          icon: Icons.bookmark,
          label: 'Bookmark',
          onTap: () {}, // TODO: wire add-bookmark
        ),
      ],
    );
  }

  Widget _buildMobileBar(BuildContext context) {
    return Row(
      children: [
        // Title
        Text('Playback', style: NvrTypography.of(context).pageTitle),
        const SizedBox(width: 12),

        // Date picker
        _DateButton(date: date, onTap: onDateTap),

        const Spacer(),

        // Icon-only buttons on mobile
        GestureDetector(
          onTap: () {}, // TODO: wire export
          child: Padding(
            padding: const EdgeInsets.all(6),
            child: Icon(Icons.download, size: 18, color: NvrColors.of(context).textSecondary),
          ),
        ),
        GestureDetector(
          onTap: () {}, // TODO: wire add-bookmark
          child: Padding(
            padding: const EdgeInsets.all(6),
            child: Icon(Icons.bookmark, size: 18, color: NvrColors.of(context).accent),
          ),
        ),
      ],
    );
  }
}

class _DateButton extends StatelessWidget {
  final String date;
  final VoidCallback onTap;
  const _DateButton({required this.date, required this.onTap});

  @override
  Widget build(BuildContext context) {
    return GestureDetector(
      onTap: onTap,
      child: Row(
        mainAxisSize: MainAxisSize.min,
        children: [
          Icon(Icons.calendar_today,
              size: 14, color: NvrColors.of(context).accent),
          const SizedBox(width: 6),
          Text(
            date,
            style: NvrTypography.of(context).monoTimestamp.copyWith(fontSize: 13),
          ),
        ],
      ),
    );
  }
}

class _SecondaryButton extends StatelessWidget {
  final IconData icon;
  final String label;
  final VoidCallback onTap;

  const _SecondaryButton({
    required this.icon,
    required this.label,
    required this.onTap,
  });

  @override
  Widget build(BuildContext context) {
    return GestureDetector(
      onTap: onTap,
      child: Container(
        padding: const EdgeInsets.symmetric(horizontal: 10, vertical: 6),
        decoration: BoxDecoration(
          color: NvrColors.of(context).bgTertiary,
          border: Border.all(color: NvrColors.of(context).border),
          borderRadius: BorderRadius.circular(6),
        ),
        child: Row(
          mainAxisSize: MainAxisSize.min,
          children: [
            Icon(icon, size: 14, color: NvrColors.of(context).textSecondary),
            const SizedBox(width: 6),
            Text(label, style: NvrTypography.of(context).button.copyWith(fontSize: 11)),
          ],
        ),
      ),
    );
  }
}

class _AccentButton extends StatelessWidget {
  final IconData icon;
  final String label;
  final VoidCallback onTap;

  const _AccentButton({
    required this.icon,
    required this.label,
    required this.onTap,
  });

  @override
  Widget build(BuildContext context) {
    return GestureDetector(
      onTap: onTap,
      child: Container(
        padding: const EdgeInsets.symmetric(horizontal: 10, vertical: 6),
        decoration: BoxDecoration(
          color: NvrColors.of(context).bgTertiary,
          border: Border.all(color: NvrColors.of(context).border),
          borderRadius: BorderRadius.circular(6),
        ),
        child: Row(
          mainAxisSize: MainAxisSize.min,
          children: [
            Icon(icon, size: 14, color: NvrColors.of(context).accent),
            const SizedBox(width: 6),
            Text(label, style: NvrTypography.of(context).button.copyWith(fontSize: 11)),
          ],
        ),
      ),
    );
  }
}

// ── Video Grid ───────────────────────────────────────────────────────────────

class _VideoGrid extends StatelessWidget {
  final List<Camera> cameras;
  final PlaybackController controller;
  final void Function(String cameraId) onRemoveCamera;

  const _VideoGrid({
    required this.cameras,
    required this.controller,
    required this.onRemoveCamera,
  });

  void _showTileMenu(BuildContext context, Offset globalPosition, Camera cam) {
    final overlay = Overlay.of(context).context.findRenderObject() as RenderBox;
    showMenu<String>(
      context: context,
      position: RelativeRect.fromRect(
        globalPosition & const Size(1, 1),
        Offset.zero & overlay.size,
      ),
      color: NvrColors.of(context).bgSecondary,
      shape: RoundedRectangleBorder(
        borderRadius: BorderRadius.circular(6),
        side: BorderSide(color: NvrColors.of(context).border),
      ),
      items: [
        PopupMenuItem(
          value: 'remove',
          height: 32,
          child: Row(
            children: [
              Icon(Icons.close, size: 14, color: NvrColors.of(context).danger),
              const SizedBox(width: 8),
              Text('Remove', style: TextStyle(fontSize: 12, color: NvrColors.of(context).danger)),
            ],
          ),
        ),
      ],
    ).then((value) {
      if (value == 'remove') onRemoveCamera(cam.id);
    });
  }

  @override
  Widget build(BuildContext context) {
    if (cameras.isEmpty) {
      return Center(
        child: Text('Drag a camera here to start',
            style: TextStyle(color: NvrColors.of(context).textMuted)),
      );
    }

    const gap = 4.0;
    const pad = 8.0;
    final n = cameras.length;

    return LayoutBuilder(
      builder: (context, constraints) {
        final availW = constraints.maxWidth - pad * 2;
        final availH = constraints.maxHeight - pad * 2;

        // Find the column count that maximizes tile size at 16:9.
        int bestCols = 1;
        double bestTileW = 0;
        for (int cols = 1; cols <= n; cols++) {
          final numRows = (n / cols).ceil();
          final tileW = (availW - gap * (cols - 1)) / cols;
          final tileH = tileW * 9 / 16;
          final totalH = tileH * numRows + gap * (numRows - 1);
          if (totalH <= availH && tileW > bestTileW) {
            bestTileW = tileW;
            bestCols = cols;
          }
        }
        for (int cols = 1; cols <= n; cols++) {
          final numRows = (n / cols).ceil();
          final tileH = (availH - gap * (numRows - 1)) / numRows;
          final tileW = tileH * 16 / 9;
          final totalW = tileW * cols + gap * (cols - 1);
          if (totalW <= availW && tileW > bestTileW) {
            bestTileW = tileW;
            bestCols = cols;
          }
        }

        final numRows = (n / bestCols).ceil();
        final tileW = bestTileW;
        final tileH = tileW * 9 / 16;

        int idx = 0;
        final rowWidgets = <Widget>[];
        for (int r = 0; r < numRows; r++) {
          final tilesInRow = (r < numRows - 1) ? bestCols : n - idx;
          final rowChildren = <Widget>[];
          for (int c = 0; c < tilesInRow; c++) {
            if (c > 0) rowChildren.add(const SizedBox(width: gap));
            final cam = cameras[idx];
            rowChildren.add(
              SizedBox(
                width: tileW,
                height: tileH,
                child: GestureDetector(
                  onSecondaryTapDown: (details) =>
                      _showTileMenu(context, details.globalPosition, cam),
                  onLongPressStart: (details) =>
                      _showTileMenu(context, details.globalPosition, cam),
                  child: CameraPlayer(
                    key: ValueKey('player-${cam.id}'),
                    cameraId: cam.id,
                    cameraName: cam.name,
                    controller: controller,
                  ),
                ),
              ),
            );
            idx++;
          }
          if (r > 0) rowWidgets.add(const SizedBox(height: gap));
          rowWidgets.add(Row(
            mainAxisAlignment: MainAxisAlignment.center,
            children: rowChildren,
          ));
        }

        return Center(
          child: Padding(
            padding: const EdgeInsets.all(pad),
            child: Column(
              mainAxisAlignment: MainAxisAlignment.center,
              children: rowWidgets,
            ),
          ),
        );
      },
    );
  }
}
