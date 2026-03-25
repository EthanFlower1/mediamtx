import 'package:flutter/material.dart';
import '../../../models/bookmark.dart';
import '../../../models/recording.dart';
import '../../../providers/timeline_intensity_provider.dart';
import '../../../theme/nvr_colors.dart';
import '../event_detail_popup.dart';
import 'bookmark_layer.dart';
import 'event_layer.dart';
import 'grid_layer.dart';
import 'intensity_layer.dart';
import 'interaction_layer.dart';
import 'mini_overview_bar.dart';
import 'playhead_layer.dart';
import 'recording_layer.dart';
import 'timeline_viewport.dart';

class ComposableTimeline extends StatefulWidget {
  final List<RecordingSegment> segments;
  final List<MotionEvent> events;
  final List<IntensityBucket> intensityBuckets;
  final int intensityBucketSeconds;
  final List<Bookmark> bookmarks;
  final DateTime selectedDate;
  final Duration position;
  final ValueChanged<Duration> onSeek;
  final bool isLoading;

  const ComposableTimeline({
    super.key,
    required this.segments,
    required this.events,
    required this.selectedDate,
    required this.position,
    required this.onSeek,
    this.intensityBuckets = const [],
    this.intensityBucketSeconds = 60,
    this.bookmarks = const [],
    this.isLoading = false,
  });

  @override
  State<ComposableTimeline> createState() => _ComposableTimelineState();
}

class _ComposableTimelineState extends State<ComposableTimeline> {
  Duration _visibleStart = Duration.zero;
  Duration _visibleEnd = const Duration(hours: 24);
  double _lastScale = 1.0;
  bool _draggingPlayhead = false;
  double? _dragX;
  bool _didAutoZoom = false;

  DateTime get _dayStart => DateTime(
      widget.selectedDate.year, widget.selectedDate.month, widget.selectedDate.day);

  @override
  void didUpdateWidget(ComposableTimeline oldWidget) {
    super.didUpdateWidget(oldWidget);
    // Reset auto-zoom when the date changes.
    if (widget.selectedDate != oldWidget.selectedDate) {
      _didAutoZoom = false;
    }
    // Auto-zoom to first recording when segments first arrive.
    if (!_didAutoZoom && widget.segments.isNotEmpty) {
      _didAutoZoom = true;
      final firstStart = widget.segments.first.startTime.difference(_dayStart);
      final lastEnd = widget.segments.last.endTime.difference(_dayStart);
      // Show from 5 minutes before first segment to 5 minutes after last,
      // with a minimum window of 1 hour.
      const pad = Duration(minutes: 5);
      const minWindow = Duration(hours: 1);
      var start = firstStart - pad;
      var end = lastEnd + pad;
      if (end - start < minWindow) {
        final center = start + (end - start) ~/ 2;
        start = center - minWindow ~/ 2;
        end = center + minWindow ~/ 2;
      }
      if (start < Duration.zero) start = Duration.zero;
      if (end > const Duration(hours: 24)) end = const Duration(hours: 24);
      setState(() {
        _visibleStart = start;
        _visibleEnd = end;
      });
    }
  }

  void _handleZoom(double scale) {
    setState(() {
      final center = _visibleStart +
          Duration(
              milliseconds:
                  ((_visibleEnd - _visibleStart).inMilliseconds / 2).round());
      final factor = scale / _lastScale;
      _lastScale = scale;

      final vp = TimelineViewport(
        visibleStart: _visibleStart,
        visibleEnd: _visibleEnd,
        widthPx: 1,
      ).zoom(factor, center);

      _visibleStart = vp.visibleStart;
      _visibleEnd = vp.visibleEnd;
    });
  }

  void _handlePan(double deltaPx) {
    setState(() {
      final vp = TimelineViewport(
        visibleStart: _visibleStart,
        visibleEnd: _visibleEnd,
        widthPx: context.size?.width ?? 800,
      ).pan(deltaPx);

      _visibleStart = vp.visibleStart;
      _visibleEnd = vp.visibleEnd;
    });
  }

  void _handleViewportJump(Duration centerTime) {
    setState(() {
      final halfVisible =
          Duration(milliseconds: (_visibleEnd - _visibleStart).inMilliseconds ~/ 2);
      var newStart = centerTime - halfVisible;
      var newEnd = centerTime + halfVisible;

      if (newStart.isNegative) {
        newStart = Duration.zero;
        newEnd = _visibleEnd - _visibleStart;
      }
      if (newEnd > const Duration(hours: 24)) {
        newEnd = const Duration(hours: 24);
        newStart = newEnd - (_visibleEnd - _visibleStart);
      }

      _visibleStart = newStart;
      _visibleEnd = newEnd;
    });
  }

  void _handleEventLongPress(MotionEvent event, Offset globalPosition) {
    showEventDetailPopup(
      context: context,
      event: event,
      position: globalPosition,
      onPlayFromHere: () {
        final dur = event.startTime.difference(_dayStart);
        widget.onSeek(dur);
      },
    );
  }

  @override
  Widget build(BuildContext context) {
    return Column(
      children: [
        // Mini overview bar
        LayoutBuilder(
          builder: (context, constraints) {
            final mainVp = TimelineViewport(
              visibleStart: _visibleStart,
              visibleEnd: _visibleEnd,
              widthPx: constraints.maxWidth,
            );
            return MiniOverviewBar(
              mainViewport: mainVp,
              segments: widget.segments,
              events: widget.events,
              dayStart: _dayStart,
              position: widget.position,
              onViewportJump: _handleViewportJump,
            );
          },
        ),
        // Main timeline
        Expanded(
          child: LayoutBuilder(
            builder: (context, constraints) {
              final vp = TimelineViewport(
                visibleStart: _visibleStart,
                visibleEnd: _visibleEnd,
                widthPx: constraints.maxWidth,
              );

              return Container(
                color: NvrColors.bgSecondary,
                child: Stack(
                  children: [
                    // Grid
                    Positioned.fill(
                      child: CustomPaint(
                        painter: GridLayer(viewport: vp),
                      ),
                    ),
                    // Motion intensity heatmap (behind recordings)
                    if (!widget.isLoading && widget.intensityBuckets.isNotEmpty)
                      Positioned.fill(
                        child: CustomPaint(
                          painter: IntensityLayer(
                            viewport: vp,
                            buckets: widget.intensityBuckets,
                            bucketSeconds: widget.intensityBucketSeconds,
                            dayStart: _dayStart,
                          ),
                        ),
                      ),
                    // Recording segments
                    if (!widget.isLoading)
                      Positioned.fill(
                        child: CustomPaint(
                          painter: RecordingLayer(
                            viewport: vp,
                            segments: widget.segments,
                            dayStart: _dayStart,
                          ),
                        ),
                      ),
                    // Events
                    if (!widget.isLoading)
                      Positioned.fill(
                        child: CustomPaint(
                          painter: EventLayer(
                            viewport: vp,
                            events: widget.events,
                            dayStart: _dayStart,
                          ),
                        ),
                      ),
                    // Bookmarks
                    if (!widget.isLoading && widget.bookmarks.isNotEmpty)
                      Positioned.fill(
                        child: CustomPaint(
                          painter: BookmarkLayer(
                            viewport: vp,
                            bookmarks: widget.bookmarks,
                            dayStart: _dayStart,
                          ),
                        ),
                      ),
                    // Loading shimmer
                    if (widget.isLoading)
                      const Positioned.fill(
                        child: _ShimmerOverlay(),
                      ),
                    // Playhead (visual only)
                    Positioned.fill(
                      child: CustomPaint(
                        painter: PlayheadLayer(
                          viewport: vp,
                          position: widget.position,
                          isDragging: _draggingPlayhead,
                          dragX: _dragX,
                        ),
                      ),
                    ),
                    // Interaction (ALL gestures)
                    Positioned.fill(
                      child: InteractionLayer(
                        viewport: vp,
                        position: widget.position,
                        events: widget.events,
                        dayStart: _dayStart,
                        onSeek: widget.onSeek,
                        onEventLongPress: _handleEventLongPress,
                        onZoom: _handleZoom,
                        onPan: _handlePan,
                        onDragStateChanged: (dragging) =>
                            setState(() => _draggingPlayhead = dragging),
                        onDragXChanged: (x) =>
                            setState(() => _dragX = x),
                      ),
                    ),
                    // Time label at playhead
                    Positioned(
                      left: vp.timeToPixel(widget.position) + 8,
                      top: 4,
                      child: Container(
                        padding: const EdgeInsets.symmetric(
                            horizontal: 6, vertical: 2),
                        decoration: BoxDecoration(
                          color: NvrColors.bgTertiary,
                          borderRadius: BorderRadius.circular(4),
                        ),
                        child: Text(
                          _formatPosition(widget.position),
                          style: const TextStyle(
                            color: NvrColors.textPrimary,
                            fontSize: 11,
                            fontWeight: FontWeight.w500,
                          ),
                        ),
                      ),
                    ),
                  ],
                ),
              );
            },
          ),
        ),
      ],
    );
  }

  String _formatPosition(Duration d) {
    final h = d.inHours.toString().padLeft(2, '0');
    final m = (d.inMinutes % 60).toString().padLeft(2, '0');
    final s = (d.inSeconds % 60).toString().padLeft(2, '0');
    return '$h:$m:$s';
  }
}

class _ShimmerOverlay extends StatefulWidget {
  const _ShimmerOverlay();

  @override
  State<_ShimmerOverlay> createState() => _ShimmerOverlayState();
}

class _ShimmerOverlayState extends State<_ShimmerOverlay>
    with SingleTickerProviderStateMixin {
  late final AnimationController _controller;

  @override
  void initState() {
    super.initState();
    _controller = AnimationController(
      vsync: this,
      duration: const Duration(milliseconds: 1500),
    )..repeat();
  }

  @override
  void dispose() {
    _controller.dispose();
    super.dispose();
  }

  @override
  Widget build(BuildContext context) {
    return AnimatedBuilder(
      animation: _controller,
      builder: (context, _) {
        return CustomPaint(
          painter: _ShimmerPainter(_controller.value),
        );
      },
    );
  }
}

class _ShimmerPainter extends CustomPainter {
  final double progress;

  _ShimmerPainter(this.progress);

  @override
  void paint(Canvas canvas, Size size) {
    final shimmerX = (progress * (size.width + 200)) - 100;
    final gradient = LinearGradient(
      colors: [
        Colors.transparent,
        NvrColors.accent.withValues(alpha: 0.08),
        Colors.transparent,
      ],
      stops: const [0.0, 0.5, 1.0],
    );

    final rect = Rect.fromLTWH(shimmerX - 100, 0, 200, size.height);
    canvas.drawRect(
      rect,
      Paint()..shader = gradient.createShader(rect),
    );
  }

  @override
  bool shouldRepaint(_ShimmerPainter old) => old.progress != progress;
}
