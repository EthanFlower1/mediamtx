import 'package:flutter/material.dart';
import 'package:flutter_riverpod/flutter_riverpod.dart';
import 'package:go_router/go_router.dart';

import '../../models/camera.dart';
import '../../models/recording.dart';
import '../../models/bookmark.dart';
import '../../providers/bookmarks_provider.dart';
import '../../providers/cameras_provider.dart';
import '../../providers/auth_provider.dart';
import '../../providers/recordings_provider.dart';
import '../../providers/timeline_intensity_provider.dart';
import '../../services/playback_service.dart';
import '../../theme/nvr_colors.dart';
import '../../theme/nvr_typography.dart';
import '../../utils/responsive.dart';
import '../../widgets/hud/segmented_control.dart';
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
  DateTime _selectedDate = DateTime.now();
  final Set<String> _selectedCameraIds = {};
  String _lastServerUrl = '';
  bool _appliedInitialBookmark = false;

  /// Grid layout: 1 = 1x1, 2 = 2x2.
  int _gridMode = 1;

  /// Timeline zoom preset index: {0: 24H, 1: 12H, 2: 4H, 3: 1H}.
  int _zoomIndex = 2;

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

  @override
  void deactivate() {
    // Pause playback when navigating away from this tab (GoRouter ShellRoute
    // keeps the widget alive but deactivates it).
    // Remove listener first to avoid setState during build phase.
    _controller?.removeListener(_onControllerChanged);
    _controller?.pause();
    _controller?.addListener(_onControllerChanged);
    super.deactivate();
  }

  @override
  void dispose() {
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
          colorScheme: const ColorScheme.dark(
            primary: NvrColors.accent,
            surface: NvrColors.bgSecondary,
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

    return Scaffold(
      backgroundColor: NvrColors.bgPrimary,
      body: camerasAsync.when(
        loading: () => const Center(
          child: CircularProgressIndicator(color: NvrColors.accent),
        ),
        error: (e, _) => Center(
          child: Text('Error: $e',
              style: const TextStyle(color: NvrColors.danger)),
        ),
        data: (cameras) => _buildBody(cameras, controller),
      ),
    );
  }

  Widget _buildBody(List<Camera> cameras, PlaybackController controller) {
    if (cameras.isEmpty) {
      return const Center(
        child: Text('No cameras configured.',
            style: TextStyle(color: NvrColors.textMuted)),
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
          gridMode: _gridMode,
          onGridChanged: (v) => setState(() => _gridMode = v),
          onExport: () => _showExportDialog(selected, controller),
        ),

        // ── Camera selector chips ─────────────────────────────────────
        _CameraChips(
          cameras: cameras,
          selectedIds: _selectedCameraIds,
          maxCameras: 4,
          onToggle: (id) => setState(() {
            if (_selectedCameraIds.contains(id)) {
              if (_selectedCameraIds.length > 1) {
                _selectedCameraIds.remove(id);
                // Auto-switch back to 1x1 when only one camera remains.
                if (_selectedCameraIds.length == 1) {
                  _gridMode = 1;
                }
              }
            } else {
              // Cap at 4 cameras for synchronized playback.
              if (_selectedCameraIds.length >= 4) return;
              _selectedCameraIds.add(id);
              // Auto-switch to 2x2 grid when multiple cameras are selected.
              if (_selectedCameraIds.length > 1) {
                _gridMode = 2;
              }
            }
          }),
        ),

        // ── Video area ────────────────────────────────────────────────
        Expanded(
          child: _VideoGrid(
            cameras: selected,
            controller: controller,
            columns: _gridMode == 1 ? 1 : 2,
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
  final int gridMode;
  final ValueChanged<int> onGridChanged;
  final VoidCallback onExport;

  const _TopBar({
    required this.date,
    required this.onDateTap,
    required this.gridMode,
    required this.onGridChanged,
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
      decoration: const BoxDecoration(
        color: NvrColors.bgSecondary,
        border: Border(bottom: BorderSide(color: NvrColors.border)),
      ),
      child: isPhone ? _buildMobileBar() : _buildDesktopBar(),
    );
  }

  Widget _buildDesktopBar() {
    return Row(
      children: [
        // Title
        const Text('Playback', style: NvrTypography.pageTitle),
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
        const SizedBox(width: 12),

        // Grid selector
        HudSegmentedControl<int>(
          segments: const {1: '1\u00D71', 2: '2\u00D72'},
          selected: gridMode,
          onChanged: onGridChanged,
        ),
      ],
    );
  }

  Widget _buildMobileBar() {
    return Row(
      children: [
        // Title
        const Text('Playback', style: NvrTypography.pageTitle),
        const SizedBox(width: 12),

        // Date picker
        _DateButton(date: date, onTap: onDateTap),

        const Spacer(),

        // Icon-only buttons on mobile
        GestureDetector(
          onTap: () {}, // TODO: wire export
          child: const Padding(
            padding: EdgeInsets.all(6),
            child: Icon(Icons.download, size: 18, color: NvrColors.textSecondary),
          ),
        ),
        GestureDetector(
          onTap: () {}, // TODO: wire add-bookmark
          child: const Padding(
            padding: EdgeInsets.all(6),
            child: Icon(Icons.bookmark, size: 18, color: NvrColors.accent),
          ),
        ),
        const SizedBox(width: 4),

        // Grid selector
        HudSegmentedControl<int>(
          segments: const {1: '1\u00D71', 2: '2\u00D72'},
          selected: gridMode,
          onChanged: onGridChanged,
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
          const Icon(Icons.calendar_today,
              size: 14, color: NvrColors.accent),
          const SizedBox(width: 6),
          Text(
            date,
            style: NvrTypography.monoTimestamp.copyWith(fontSize: 13),
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
          color: NvrColors.bgTertiary,
          border: Border.all(color: NvrColors.border),
          borderRadius: BorderRadius.circular(6),
        ),
        child: Row(
          mainAxisSize: MainAxisSize.min,
          children: [
            Icon(icon, size: 14, color: NvrColors.textSecondary),
            const SizedBox(width: 6),
            Text(label, style: NvrTypography.button.copyWith(fontSize: 11)),
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
          color: NvrColors.bgTertiary,
          border: Border.all(color: NvrColors.border),
          borderRadius: BorderRadius.circular(6),
        ),
        child: Row(
          mainAxisSize: MainAxisSize.min,
          children: [
            Icon(icon, size: 14, color: NvrColors.accent),
            const SizedBox(width: 6),
            Text(label, style: NvrTypography.button.copyWith(fontSize: 11)),
          ],
        ),
      ),
    );
  }
}

// ── Camera Chips ─────────────────────────────────────────────────────────────

class _CameraChips extends StatelessWidget {
  final List<Camera> cameras;
  final Set<String> selectedIds;
  final int maxCameras;
  final ValueChanged<String> onToggle;

  const _CameraChips({
    required this.cameras,
    required this.selectedIds,
    required this.onToggle,
    this.maxCameras = 4,
  });

  @override
  Widget build(BuildContext context) {
    final atCapacity = selectedIds.length >= maxCameras;
    return Container(
      color: NvrColors.bgSecondary,
      padding: const EdgeInsets.symmetric(horizontal: 12, vertical: 6),
      child: SizedBox(
        height: 32,
        child: Row(
          children: [
            // Camera count indicator
            if (selectedIds.length > 1)
              Padding(
                padding: const EdgeInsets.only(right: 8),
                child: Container(
                  padding:
                      const EdgeInsets.symmetric(horizontal: 6, vertical: 4),
                  decoration: BoxDecoration(
                    color: NvrColors.accent.withValues(alpha: 0.12),
                    borderRadius: BorderRadius.circular(4),
                  ),
                  child: Text(
                    '${selectedIds.length}/$maxCameras',
                    style: const TextStyle(
                      fontFamily: 'JetBrainsMono',
                      fontSize: 10,
                      fontWeight: FontWeight.w600,
                      color: NvrColors.accent,
                    ),
                  ),
                ),
              ),
            // Camera chips
            Expanded(
              child: ListView.separated(
                scrollDirection: Axis.horizontal,
                itemCount: cameras.length,
                separatorBuilder: (_, __) => const SizedBox(width: 8),
                itemBuilder: (_, i) {
                  final c = cameras[i];
                  final sel = selectedIds.contains(c.id);
                  // Dim unselected chips when at capacity.
                  final disabled = atCapacity && !sel;
                  return FilterChip(
                    label: Text(c.name,
                        style: TextStyle(
                          color: sel
                              ? Colors.white
                              : disabled
                                  ? NvrColors.textMuted
                                  : NvrColors.textSecondary,
                          fontSize: 11,
                        )),
                    selected: sel,
                    onSelected: disabled ? null : (_) => onToggle(c.id),
                    backgroundColor: NvrColors.bgTertiary,
                    selectedColor: NvrColors.accent,
                    disabledColor: NvrColors.bgTertiary,
                    checkmarkColor: Colors.white,
                    side: BorderSide(
                        color: sel
                            ? NvrColors.accent
                            : disabled
                                ? NvrColors.border.withValues(alpha: 0.5)
                                : NvrColors.border),
                    padding: const EdgeInsets.symmetric(horizontal: 4),
                    materialTapTargetSize: MaterialTapTargetSize.shrinkWrap,
                  );
                },
              ),
            ),
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
  final int columns;

  const _VideoGrid({
    required this.cameras,
    required this.controller,
    required this.columns,
  });

  @override
  Widget build(BuildContext context) {
    if (cameras.isEmpty) {
      return const Center(
        child: Text('Select a camera',
            style: TextStyle(color: NvrColors.textMuted)),
      );
    }

    // Single camera — fill the space.
    if (columns == 1 && cameras.length == 1) {
      return Padding(
        padding: const EdgeInsets.all(8),
        child: CameraPlayer(
          key: ValueKey('player-${cameras.first.id}'),
          cameraId: cameras.first.id,
          cameraName: cameras.first.name,
          controller: controller,
        ),
      );
    }

    // Multi-camera grid.
    final rows = (cameras.length / columns).ceil();

    return LayoutBuilder(
      builder: (context, constraints) {
        final totalGapH = (columns - 1) * 4.0;
        final totalGapV = (rows - 1) * 4.0;
        final cellWidth = (constraints.maxWidth - totalGapH - 16) / columns;
        final cellHeight = (constraints.maxHeight - totalGapV - 16) / rows;
        final h = (cellWidth / 16 * 9).clamp(0.0, cellHeight);
        final w = h * 16 / 9;

        return Center(
          child: Padding(
            padding: const EdgeInsets.all(8),
            child: Wrap(
              spacing: 4,
              runSpacing: 4,
              children: [
                for (final cam in cameras)
                  SizedBox(
                    width: w,
                    height: h,
                    child: CameraPlayer(
                      key: ValueKey('player-${cam.id}'),
                      cameraId: cam.id,
                      cameraName: cam.name,
                      controller: controller,
                    ),
                  ),
              ],
            ),
          ),
        );
      },
    );
  }
}
