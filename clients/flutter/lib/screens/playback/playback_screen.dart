import 'package:flutter/material.dart';
import 'package:flutter_riverpod/flutter_riverpod.dart';

import '../../models/camera.dart';
import '../../models/recording.dart';
import '../../providers/cameras_provider.dart';
import '../../providers/recordings_provider.dart';
import '../../providers/auth_provider.dart';
import '../../services/playback_service.dart';
import '../../theme/nvr_colors.dart';
import 'camera_player.dart';
import 'playback_controls.dart';
import 'timeline_widget.dart';

class PlaybackScreen extends ConsumerStatefulWidget {
  const PlaybackScreen({super.key});

  @override
  ConsumerState<PlaybackScreen> createState() => _PlaybackScreenState();
}

class _PlaybackScreenState extends ConsumerState<PlaybackScreen> {
  DateTime _selectedDate = DateTime.now();
  final Set<String> _selectedCameraIds = {};
  bool _playing = false;
  double _speed = 1.0;
  Duration _position = Duration.zero;

  String get _dateKey =>
      '${_selectedDate.year}-${_selectedDate.month.toString().padLeft(2, '0')}-${_selectedDate.day.toString().padLeft(2, '0')}';

  @override
  Widget build(BuildContext context) {
    final camerasAsync = ref.watch(camerasProvider);
    final isWide = MediaQuery.of(context).size.width >= 720;

    return Scaffold(
      backgroundColor: NvrColors.bgPrimary,
      appBar: AppBar(
        backgroundColor: NvrColors.bgSecondary,
        title: const Text('Playback', style: TextStyle(color: NvrColors.textPrimary)),
        actions: [
          _DatePickerButton(
            date: _selectedDate,
            onChanged: (d) => setState(() {
              _selectedDate = d;
              _position = Duration.zero;
            }),
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
        data: (cameras) {
          if (cameras.isEmpty) {
            return const Center(
              child: Text('No cameras configured.',
                  style: TextStyle(color: NvrColors.textMuted)),
            );
          }

          // Default select first camera
          if (_selectedCameraIds.isEmpty && cameras.isNotEmpty) {
            _selectedCameraIds.add(cameras.first.id);
          }

          final selected = cameras
              .where((c) => _selectedCameraIds.contains(c.id))
              .toList();

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
              Expanded(
                child: isWide
                    ? _WideLayout(
                        cameras: selected,
                        date: _selectedDate,
                        dateKey: _dateKey,
                        playing: _playing,
                        speed: _speed,
                        position: _position,
                        onSeek: (d) => setState(() => _position = d),
                      )
                    : _NarrowLayout(
                        cameras: selected,
                        date: _selectedDate,
                        dateKey: _dateKey,
                        playing: _playing,
                        speed: _speed,
                        position: _position,
                        onSeek: (d) => setState(() => _position = d),
                      ),
              ),
              PlaybackControls(
                playing: _playing,
                speed: _speed,
                onPlayPause: () => setState(() => _playing = !_playing),
                onBack10: () => setState(() {
                  final s = _position.inSeconds - 10;
                  _position = Duration(seconds: s < 0 ? 0 : s);
                }),
                onForward10: () => setState(() {
                  final s = _position.inSeconds + 10;
                  _position = Duration(seconds: s > 86400 ? 86400 : s);
                }),
                onSpeedChange: (s) => setState(() => _speed = s),
              ),
            ],
          );
        },
      ),
    );
  }
}

// ── Camera Chips ──────────────────────────────────────────────────────────────

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

// ── Date Picker Button ────────────────────────────────────────────────────────

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

// ── Video Grid Helpers ────────────────────────────────────────────────────────

class _VideoGrid extends ConsumerWidget {
  final List<Camera> cameras;
  final DateTime date;
  final String dateKey;
  final bool playing;
  final double speed;
  final Duration position;

  const _VideoGrid({
    required this.cameras,
    required this.date,
    required this.dateKey,
    required this.playing,
    required this.speed,
    required this.position,
  });

  @override
  Widget build(BuildContext context, WidgetRef ref) {
    final auth = ref.watch(authProvider);
    final serverUrl = auth.serverUrl ?? '';
    final svc = PlaybackService(serverUrl: serverUrl);
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
        final dayStart = DateTime(date.year, date.month, date.day);
        final startAt = dayStart.add(position);
        final url = svc.playbackUrl(cam.mediamtxPath, startAt);
        return CameraPlayer(
          key: ValueKey('${cam.id}-$dateKey-${position.inSeconds}'),
          url: url,
          cameraName: cam.name,
          playing: playing,
          playbackSpeed: speed,
        );
      },
    );
  }
}

// ── Layout variants ───────────────────────────────────────────────────────────

class _WideLayout extends ConsumerWidget {
  final List<Camera> cameras;
  final DateTime date;
  final String dateKey;
  final bool playing;
  final double speed;
  final Duration position;
  final ValueChanged<Duration> onSeek;

  const _WideLayout({
    required this.cameras,
    required this.date,
    required this.dateKey,
    required this.playing,
    required this.speed,
    required this.position,
    required this.onSeek,
  });

  @override
  Widget build(BuildContext context, WidgetRef ref) {
    final motionAsync = cameras.isNotEmpty
        ? ref.watch(motionEventsProvider(
            (cameraId: cameras.first.id, date: dateKey)))
        : const AsyncValue<List<MotionEvent>>.data([]);

    return Row(
      children: [
        // Video area
        Expanded(
          child: _VideoGrid(
            cameras: cameras,
            date: date,
            dateKey: dateKey,
            playing: playing,
            speed: speed,
            position: position,
          ),
        ),
        // Vertical timeline
        Container(
          width: 64,
          color: NvrColors.bgTertiary,
          child: motionAsync.when(
            loading: () => const SizedBox.shrink(),
            error: (_, __) => const SizedBox.shrink(),
            data: (events) => TimelineWidget(
              motionEvents: events,
              selectedDate: date,
              position: position,
              vertical: true,
              onSeek: onSeek,
            ),
          ),
        ),
      ],
    );
  }
}

class _NarrowLayout extends ConsumerWidget {
  final List<Camera> cameras;
  final DateTime date;
  final String dateKey;
  final bool playing;
  final double speed;
  final Duration position;
  final ValueChanged<Duration> onSeek;

  const _NarrowLayout({
    required this.cameras,
    required this.date,
    required this.dateKey,
    required this.playing,
    required this.speed,
    required this.position,
    required this.onSeek,
  });

  @override
  Widget build(BuildContext context, WidgetRef ref) {
    final motionAsync = cameras.isNotEmpty
        ? ref.watch(motionEventsProvider(
            (cameraId: cameras.first.id, date: dateKey)))
        : const AsyncValue<List<MotionEvent>>.data([]);

    return Column(
      children: [
        // Video area
        Expanded(
          child: _VideoGrid(
            cameras: cameras,
            date: date,
            dateKey: dateKey,
            playing: playing,
            speed: speed,
            position: position,
          ),
        ),
        // Horizontal timeline
        Container(
          height: 64,
          color: NvrColors.bgTertiary,
          child: motionAsync.when(
            loading: () => const SizedBox.shrink(),
            error: (_, __) => const SizedBox.shrink(),
            data: (events) => TimelineWidget(
              motionEvents: events,
              selectedDate: date,
              position: position,
              vertical: false,
              onSeek: onSeek,
            ),
          ),
        ),
      ],
    );
  }
}
