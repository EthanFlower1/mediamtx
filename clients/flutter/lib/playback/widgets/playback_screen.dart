// KAI-302 — Top-level playback screen for the new timeline layer.
//
// This is the *new* screen shape called for by KAI-302. It combines the
// scrub bar, date picker, speed control, bookmark/export buttons, and a
// placeholder video area.
//
// Real video playback wiring is *not* in scope — the placeholder banner
// will be replaced by lead-onprem's libwebrtc/libavformat integration
// (KAI-334). This screen is additive; it does not touch the existing
// `lib/screens/playback/playback_screen.dart` which wraps the current
// RTSP/HLS pipeline.

import 'package:flutter/material.dart';
import 'package:flutter_riverpod/flutter_riverpod.dart';

import '../../models/camera.dart';
import '../playback_client.dart';
import '../playback_strings.dart';
import '../providers.dart';
import '../timeline_model.dart';
import 'bookmark_button.dart';
import 'clip_export_button.dart';
import 'scrub_bar.dart';
import 'speed_control.dart';
import 'timeline_date_picker.dart';

/// Pure placeholder that stands in for the real video view. The
/// `video_player` package is in pubspec but is not wired through here on
/// purpose — playback signaling is lead-onprem's territory (KAI-334).
class PlaybackVideoView extends StatelessWidget {
  final PlaybackStrings strings;
  const PlaybackVideoView({super.key, this.strings = PlaybackStrings.en});

  @override
  Widget build(BuildContext context) {
    return Container(
      color: Colors.black,
      alignment: Alignment.center,
      child: Text(
        strings.videoIntegrationPending,
        style: const TextStyle(color: Colors.white70),
      ),
    );
  }
}

class Kai302PlaybackScreen extends ConsumerStatefulWidget {
  final Camera camera;
  final PlaybackClient? clientOverride;
  final PlaybackStrings strings;

  const Kai302PlaybackScreen({
    super.key,
    required this.camera,
    this.clientOverride,
    this.strings = PlaybackStrings.en,
  });

  @override
  ConsumerState<Kai302PlaybackScreen> createState() =>
      _Kai302PlaybackScreenState();
}

class _Kai302PlaybackScreenState extends ConsumerState<Kai302PlaybackScreen> {
  late DateTimeRange _range;
  DateTime? _scrubAt;
  TimelineSpan? _span;

  @override
  void initState() {
    super.initState();
    final now = DateTime.now();
    final start = DateTime(now.year, now.month, now.day);
    _range = DateTimeRange(start: start, end: start.add(const Duration(days: 1)));
    _load();
  }

  PlaybackClient get _client =>
      widget.clientOverride ?? ref.read(playbackClientProvider);

  Future<void> _load() async {
    final span = await _client.loadSpan(
      cameraId: widget.camera.id,
      start: _range.start,
      end: _range.end,
    );
    if (!mounted) return;
    setState(() {
      _span = span;
      _scrubAt ??= span.start;
    });
  }

  String? get _currentSegmentId {
    final span = _span;
    final at = _scrubAt;
    if (span == null || at == null) return null;
    for (final s in span.segments) {
      if (!at.isBefore(s.startedAt) && !at.isAfter(s.endedAt)) return s.id;
    }
    return span.segments.isEmpty ? null : span.segments.first.id;
  }

  int get _atMsInSegment {
    final span = _span;
    final at = _scrubAt;
    if (span == null || at == null) return 0;
    for (final s in span.segments) {
      if (!at.isBefore(s.startedAt) && !at.isAfter(s.endedAt)) {
        return at.difference(s.startedAt).inMilliseconds;
      }
    }
    return 0;
  }

  @override
  Widget build(BuildContext context) {
    final speed = ref.watch(playbackSpeedProvider);
    final strings = widget.strings;
    final span = _span;
    final segId = _currentSegmentId;

    return Scaffold(
      appBar: AppBar(
        title: Text(strings.screenTitle),
        actions: [
          TimelineDatePicker(
            initialDate: _range.start,
            firstDate: DateTime(2020),
            lastDate: DateTime.now().add(const Duration(days: 1)),
            strings: strings,
            onRangeSelected: (r) {
              setState(() {
                _range = r;
                _span = null;
                _scrubAt = null;
              });
              _load();
            },
            child: const Padding(
              padding: EdgeInsets.symmetric(horizontal: 12),
              child: Icon(Icons.calendar_today),
            ),
          ),
        ],
      ),
      body: Column(
        children: [
          Expanded(child: PlaybackVideoView(strings: strings)),
          if (span == null)
            Padding(
              padding: const EdgeInsets.all(16),
              child: Text(strings.loading),
            )
          else if (span.segments.isEmpty)
            Padding(
              padding: const EdgeInsets.all(16),
              child: Text(strings.noRecordings),
            )
          else
            ScrubBar(
              span: span,
              onScrub: (t) => setState(() => _scrubAt = t),
            ),
          Padding(
            padding: const EdgeInsets.all(8),
            child: Row(
              mainAxisAlignment: MainAxisAlignment.spaceBetween,
              children: [
                SpeedControl(
                  selected: speed,
                  strings: strings,
                  onSpeedChanged: (s) =>
                      ref.read(playbackSpeedProvider.notifier).state = s,
                ),
                Row(children: [
                  if (segId != null) ...[
                    BookmarkButton(
                      client: _client,
                      segmentId: segId,
                      atMs: _atMsInSegment,
                      strings: strings,
                    ),
                    ClipExportButton(
                      client: _client,
                      segmentId: segId,
                      startMs: _atMsInSegment,
                      endMs: _atMsInSegment + 10000,
                      strings: strings,
                    ),
                  ],
                ]),
              ],
            ),
          ),
        ],
      ),
    );
  }
}
