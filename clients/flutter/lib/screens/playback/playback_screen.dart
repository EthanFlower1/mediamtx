import 'package:flutter/material.dart';
import 'package:flutter_riverpod/flutter_riverpod.dart';

import '../../models/camera.dart';
import '../../models/recording.dart';
import '../../providers/cameras_provider.dart';
import '../../providers/auth_provider.dart';
import '../../providers/recordings_provider.dart';
import '../../providers/timeline_intensity_provider.dart';
import '../../services/playback_service.dart';
import '../../theme/nvr_colors.dart';
import 'camera_player.dart';
import 'controls/jog_slider.dart';
import 'controls/transport_controls.dart';
import 'playback_controller.dart';
import 'timeline/composable_timeline.dart';

class PlaybackScreen extends ConsumerStatefulWidget {
  const PlaybackScreen({super.key});

  @override
  ConsumerState<PlaybackScreen> createState() => _PlaybackScreenState();
}

class _PlaybackScreenState extends ConsumerState<PlaybackScreen> {
  PlaybackController? _controller;
  DateTime _selectedDate = DateTime.now();
  final Set<String> _selectedCameraIds = {};
  String _lastServerUrl = '';

  static const _speeds = [0.5, 1.0, 1.5, 2.0, 4.0, 8.0];
  double _playbackSpeed = 1.0;

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
  void dispose() {
    _controller?.removeListener(_onControllerChanged);
    _controller?.dispose();
    super.dispose();
  }

  @override
  Widget build(BuildContext context) {
    final camerasAsync = ref.watch(camerasProvider);
    final auth = ref.watch(authProvider);
    final serverUrl = auth.serverUrl ?? '';
    final controller = _getController(serverUrl);

    return Scaffold(
      backgroundColor: NvrColors.bgPrimary,
      appBar: AppBar(
        backgroundColor: NvrColors.bgSecondary,
        title: const Text('Playback',
            style: TextStyle(color: NvrColors.textPrimary)),
        actions: [
          IconButton(
            icon: const Icon(Icons.chevron_left, color: NvrColors.textSecondary),
            onPressed: () {
              final prev = DateTime(
                controller.selectedDate.year,
                controller.selectedDate.month,
                controller.selectedDate.day - 1,
              );
              controller.setSelectedDate(prev);
            },
          ),
          _DatePickerButton(
            date: _selectedDate,
            onChanged: (d) {
              setState(() => _selectedDate = d);
              controller.setSelectedDate(d);
            },
          ),
          IconButton(
            icon: const Icon(Icons.chevron_right, color: NvrColors.textSecondary),
            onPressed: () {
              final next = DateTime(
                controller.selectedDate.year,
                controller.selectedDate.month,
                controller.selectedDate.day + 1,
              );
              controller.setSelectedDate(next);
            },
          ),
        ],
      ),
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

    if (_selectedCameraIds.isEmpty && cameras.isNotEmpty) {
      _selectedCameraIds.add(cameras.first.id);
    }

    final pathMap = {for (final c in cameras) c.id: c.mediamtxPath};
    controller.setCameraPaths(pathMap);
    controller.setSelectedCameraIds(_selectedCameraIds.toList());

    final selected =
        cameras.where((c) => _selectedCameraIds.contains(c.id)).toList();

    // Fetch and merge recordings/events for all selected cameras
    final allSegments = <RecordingSegment>[];
    final allEvents = <MotionEvent>[];
    final allIntensityBuckets = <IntensityBucket>[];
    const intensityBucketSeconds = 60;
    bool isLoading = false;

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

      if (segAsync.isLoading || evtAsync.isLoading) isLoading = true;
      allSegments.addAll(segAsync.valueOrNull ?? []);
      allEvents.addAll(evtAsync.valueOrNull ?? []);
      allIntensityBuckets.addAll(intensityAsync.valueOrNull ?? []);
    }

    allSegments.sort((a, b) => a.startTime.compareTo(b.startTime));
    allEvents.sort((a, b) => a.startTime.compareTo(b.startTime));

    controller.setSegments(allSegments);
    controller.setEvents(allEvents);

    return Column(
      children: [
        // Camera selector chips
        _CameraChips(
          cameras: cameras,
          selectedIds: _selectedCameraIds,
          onToggle: (id) => setState(() {
            if (_selectedCameraIds.contains(id)) {
              if (_selectedCameraIds.length > 1) {
                _selectedCameraIds.remove(id);
              }
            } else {
              _selectedCameraIds.add(id);
            }
          }),
        ),
        // Video grid
        Expanded(
          flex: 3,
          child: _VideoGrid(
            cameras: selected,
            controller: controller,
          ),
        ),
        // Timeline
        SizedBox(
          height: 120,
          child: ComposableTimeline(
            segments: allSegments,
            events: allEvents,
            intensityBuckets: allIntensityBuckets,
            intensityBucketSeconds: intensityBucketSeconds,
            selectedDate: _selectedDate,
            position: controller.position,
            onSeek: (d) => controller.seek(d),
            isLoading: isLoading,
          ),
        ),
        // Controls
        Container(
          color: NvrColors.bgSecondary,
          padding: const EdgeInsets.symmetric(vertical: 8, horizontal: 12),
          child: Column(
            children: [
              TransportControls(
                isPlaying: controller.isPlaying,
                onPlayPause: controller.togglePlayPause,
                onStepForward: () => controller.stepFrame(1),
                onStepBackward: () => controller.stepFrame(-1),
                onNextEvent: controller.skipToNextEvent,
                onPreviousEvent: controller.skipToPreviousEvent,
                onNextGap: controller.skipToNextGap,
                onPreviousGap: controller.skipToPreviousGap,
              ),
              const SizedBox(height: 4),
              Row(
                children: [
                  Expanded(
                    child: JogSlider(
                      currentPosition: controller.position,
                      onSeek: (target) => controller.seek(target),
                    ),
                  ),
                  const SizedBox(width: 12),
                  DropdownButton<double>(
                    value: _playbackSpeed,
                    dropdownColor: NvrColors.bgSecondary,
                    style: const TextStyle(
                        color: NvrColors.textPrimary, fontSize: 13),
                    underline: const SizedBox.shrink(),
                    items: _speeds
                        .map((s) => DropdownMenuItem(
                              value: s,
                              child: Text('${s}x'),
                            ))
                        .toList(),
                    onChanged: (s) {
                      if (s != null) {
                        setState(() => _playbackSpeed = s);
                        controller.setSpeed(s);
                      }
                    },
                  ),
                ],
              ),
            ],
          ),
        ),
      ],
    );
  }
}

// -- Camera Chips -------------------------------------------------------------

class _CameraChips extends StatelessWidget {
  final List<Camera> cameras;
  final Set<String> selectedIds;
  final ValueChanged<String> onToggle;

  const _CameraChips({
    required this.cameras,
    required this.selectedIds,
    required this.onToggle,
  });

  @override
  Widget build(BuildContext context) {
    return Container(
      color: NvrColors.bgSecondary,
      padding: const EdgeInsets.symmetric(horizontal: 12, vertical: 8),
      child: SizedBox(
        height: 36,
        child: ListView.separated(
          scrollDirection: Axis.horizontal,
          itemCount: cameras.length,
          separatorBuilder: (_, __) => const SizedBox(width: 8),
          itemBuilder: (_, i) {
            final c = cameras[i];
            final selected = selectedIds.contains(c.id);
            return FilterChip(
              label: Text(c.name,
                  style: TextStyle(
                    color: selected ? Colors.white : NvrColors.textSecondary,
                    fontSize: 12,
                  )),
              selected: selected,
              onSelected: (_) => onToggle(c.id),
              backgroundColor: NvrColors.bgTertiary,
              selectedColor: NvrColors.accent,
              checkmarkColor: Colors.white,
              side: BorderSide(
                  color: selected ? NvrColors.accent : NvrColors.border),
              padding: const EdgeInsets.symmetric(horizontal: 4),
            );
          },
        ),
      ),
    );
  }
}

// -- Date Picker Button -------------------------------------------------------

class _DatePickerButton extends StatelessWidget {
  final DateTime date;
  final ValueChanged<DateTime> onChanged;

  const _DatePickerButton({required this.date, required this.onChanged});

  @override
  Widget build(BuildContext context) {
    final label =
        '${date.year}-${date.month.toString().padLeft(2, '0')}-${date.day.toString().padLeft(2, '0')}';
    return TextButton.icon(
      onPressed: () async {
        final picked = await showDatePicker(
          context: context,
          initialDate: date,
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
        if (picked != null) onChanged(picked);
      },
      icon: const Icon(Icons.calendar_today,
          size: 16, color: NvrColors.textSecondary),
      label: Text(label,
          style: const TextStyle(color: NvrColors.textSecondary, fontSize: 13)),
    );
  }
}

// -- Video Grid ---------------------------------------------------------------

class _VideoGrid extends StatelessWidget {
  final List<Camera> cameras;
  final PlaybackController controller;

  const _VideoGrid({
    required this.cameras,
    required this.controller,
  });

  @override
  Widget build(BuildContext context) {
    final cols = cameras.length > 1 ? 2 : 1;

    return GridView.builder(
      physics: const NeverScrollableScrollPhysics(),
      gridDelegate: SliverGridDelegateWithFixedCrossAxisCount(
        crossAxisCount: cols,
        childAspectRatio: 16 / 9,
        crossAxisSpacing: 4,
        mainAxisSpacing: 4,
      ),
      itemCount: cameras.length,
      itemBuilder: (_, i) {
        final cam = cameras[i];
        final vc = controller.videoControllers[cam.id];
        if (vc == null) {
          return const ColoredBox(
            color: Colors.black,
            child: Center(
              child: CircularProgressIndicator(color: NvrColors.accent),
            ),
          );
        }
        return CameraPlayer(
          key: ValueKey('player-${cam.id}'),
          videoController: vc,
          cameraName: cam.name,
        );
      },
    );
  }
}
