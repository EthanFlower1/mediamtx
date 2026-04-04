import 'dart:math' as math;

import 'package:flutter/gestures.dart';
import 'package:flutter/material.dart';
import 'package:flutter/scheduler.dart';

import '../../../models/bookmark.dart';
import '../../../models/recording.dart';
import '../../../providers/timeline_intensity_provider.dart';
import '../../../theme/nvr_animations.dart';
import '../../../theme/nvr_colors.dart';
import 'timeline_painter.dart';

/// Zoom presets for the timeline. Each defines a visible duration.
enum TimelineZoom {
  oneHour(Duration(hours: 1), '1H'),
  thirtyMinutes(Duration(minutes: 30), '30M'),
  tenMinutes(Duration(minutes: 10), '10M'),
  fiveMinutes(Duration(minutes: 5), '5M');

  final Duration visibleDuration;
  final String label;

  const TimelineZoom(this.visibleDuration, this.label);
}

/// A fixed-center playhead timeline widget.
///
/// The playhead is a vertical line fixed at the horizontal center of the
/// timeline. Content (recordings, events, bookmarks, time labels) scrolls
/// underneath it. Think tape deck: the tape moves, the head stays still.
class FixedPlayheadTimeline extends StatefulWidget {
  /// Current playback position as a Duration from midnight.
  final Duration currentPosition;

  /// Whether playback is currently active.
  final bool isPlaying;

  /// Playback speed multiplier (1.0 = normal).
  final double playbackSpeed;

  /// Recording segments for the selected day.
  final List<RecordingSegment> segments;

  /// Motion events for the selected day.
  final List<MotionEvent> events;

  /// User bookmarks for the selected day.
  final List<Bookmark> bookmarks;

  /// Pre-aggregated motion intensity buckets.
  final List<IntensityBucket> intensityBuckets;

  /// Duration of each intensity bucket.
  final Duration bucketDuration;

  /// The start of the selected day (midnight).
  final DateTime dayStart;

  /// Called whenever the time under the playhead changes.
  final ValueChanged<Duration> onPositionChanged;

  /// Called when the user grabs the playhead to begin dragging.
  final VoidCallback onDragStart;

  /// Called when the user releases the playhead after dragging.
  final VoidCallback onDragEnd;

  /// Optional external zoom level. When provided, the internal zoom bar is
  /// hidden and the timeline uses this zoom preset instead.
  final TimelineZoom? zoomLevel;

  /// Called when the zoom level changes (via pinch, scroll-wheel, or internal
  /// zoom bar). Only fires when [zoomLevel] is provided so the parent can
  /// keep its state in sync.
  final ValueChanged<TimelineZoom>? onZoomChanged;

  const FixedPlayheadTimeline({
    super.key,
    required this.currentPosition,
    required this.isPlaying,
    this.playbackSpeed = 1.0,
    required this.segments,
    required this.events,
    required this.bookmarks,
    required this.intensityBuckets,
    required this.bucketDuration,
    required this.dayStart,
    required this.onPositionChanged,
    required this.onDragStart,
    required this.onDragEnd,
    this.zoomLevel,
    this.onZoomChanged,
  });

  @override
  State<FixedPlayheadTimeline> createState() => _FixedPlayheadTimelineState();
}

class _FixedPlayheadTimelineState extends State<FixedPlayheadTimeline>
    with TickerProviderStateMixin {
  /// The scroll offset in pixels. Maps to time via _pixelsPerSecond.
  double _scrollOffset = 0;

  /// Current zoom level. Seeded from widget.zoomLevel when provided.
  TimelineZoom _zoomLevel = TimelineZoom.thirtyMinutes;

  /// Pixels per second at the current zoom level. Calculated from widget width.
  double _pixelsPerSecond = 0.22; // Default for 30min at ~400px

  /// Whether the user is dragging the playhead.
  bool _isDragging = false;

  /// Tracks the initial scale for pinch-zoom gestures.
  double _scaleStart = 1.0;

  /// The pixelsPerSecond value at the start of a pinch-zoom gesture.
  double _ppsAtScaleStart = 0.22;

  /// Ticker for auto-scroll during playback.
  Ticker? _playbackTicker;

  /// Last tick time for calculating delta during auto-scroll.
  Duration? _lastTickTime;

  /// Animation controller for tap-to-seek animation.
  AnimationController? _seekAnimController;
  Animation<double>? _seekAnimation;

  /// Whether we should ignore external position updates (during user interaction).
  bool _ignoreExternalUpdates = false;

  // Day boundaries in seconds.
  static const double _dayStartSeconds = 0;
  static const double _dayEndSeconds = 86400; // 24 * 3600

  // Timeline body height.
  static const double _timelineHeight = 80;

  @override
  void initState() {
    super.initState();
    if (widget.zoomLevel != null) {
      _zoomLevel = widget.zoomLevel!;
    }
    // Initialize scroll offset from the current position.
    _scrollOffset = widget.currentPosition.inMilliseconds / 1000.0 *
        _pixelsPerSecond;
    _startPlaybackTickerIfNeeded();
  }

  @override
  void didUpdateWidget(FixedPlayheadTimeline oldWidget) {
    super.didUpdateWidget(oldWidget);

    // Update scroll offset when position changes externally
    // (e.g., from PlaybackController seeking), but NOT during user interaction.
    if (!_ignoreExternalUpdates && !_isDragging) {
      final newOffset = widget.currentPosition.inMilliseconds / 1000.0 *
          _pixelsPerSecond;
      if ((newOffset - _scrollOffset).abs() > _pixelsPerSecond * 0.5) {
        _scrollOffset = newOffset;
      }
    }

    // Sync zoom level from external controller.
    if (widget.zoomLevel != null && widget.zoomLevel != _zoomLevel) {
      final centerTimeBefore = _centerTime;
      _zoomLevel = widget.zoomLevel!;
      // Recompute pps using a deferred callback (needs layout width).
      WidgetsBinding.instance.addPostFrameCallback((_) {
        if (!mounted) return;
        final box = context.findRenderObject() as RenderBox?;
        final width = box?.size.width ?? 400;
        _pixelsPerSecond = width / _zoomLevel.visibleDuration.inSeconds;
        _scrollOffset = centerTimeBefore.inMilliseconds / 1000.0 *
            _pixelsPerSecond;
        setState(() {});
      });
    }

    // Handle play state changes.
    if (widget.isPlaying != oldWidget.isPlaying ||
        widget.playbackSpeed != oldWidget.playbackSpeed) {
      _stopPlaybackTicker();
      _startPlaybackTickerIfNeeded();
    }
  }

  @override
  void dispose() {
    _stopPlaybackTicker();
    _seekAnimController?.dispose();
    super.dispose();
  }

  // ─── Playback Auto-Scroll ────────────────────────────────────────────

  void _startPlaybackTickerIfNeeded() {
    if (widget.isPlaying && !_isDragging) {
      _playbackTicker?.dispose();
      _lastTickTime = null;
      _playbackTicker = createTicker(_onPlaybackTick);
      _playbackTicker!.start();
    }
  }

  void _stopPlaybackTicker() {
    _playbackTicker?.stop();
    _playbackTicker?.dispose();
    _playbackTicker = null;
    _lastTickTime = null;
  }

  void _onPlaybackTick(Duration elapsed) {
    if (_isDragging) return;

    final lastTick = _lastTickTime;
    _lastTickTime = elapsed;

    if (lastTick == null) return;

    final deltaMs = (elapsed - lastTick).inMicroseconds / 1000000.0;
    final pixelDelta = _pixelsPerSecond * widget.playbackSpeed * deltaMs;

    setState(() {
      _scrollOffset = (_scrollOffset + pixelDelta).clamp(
        _dayStartSeconds * _pixelsPerSecond,
        _dayEndSeconds * _pixelsPerSecond,
      );
    });
  }

  // ─── Computed Values ─────────────────────────────────────────────────

  /// The time at the center of the playhead, as a Duration from midnight.
  Duration get _centerTime {
    final seconds = _scrollOffset / _pixelsPerSecond;
    final clampedMs =
        (seconds * 1000).round().clamp(0, _dayEndSeconds.toInt() * 1000);
    return Duration(milliseconds: clampedMs);
  }

  // ─── Position Callback ───────────────────────────────────────────────

  void _firePositionChanged() {
    widget.onPositionChanged(_centerTime);
  }

  // ─── Unified Scale/Pan Gesture Handling ────────────────────────────
  //
  // Flutter's GestureDetector cannot have both horizontal drag and scale
  // gestures simultaneously — they compete. The scale gesture subsumes
  // single-finger pan (focalPointDelta) and multi-finger pinch (scale).
  // We use onScale* for ALL drag/zoom interactions.

  void _onScaleStart(ScaleStartDetails details) {
    _scaleStart = 1.0;
    _ppsAtScaleStart = _pixelsPerSecond;

    // Treat any scale start as a drag start.
    _stopPlaybackTicker();
    _seekAnimController?.stop();
    setState(() {
      _isDragging = true;
      _ignoreExternalUpdates = true;
    });
    widget.onDragStart();
  }

  void _onScaleUpdate(ScaleUpdateDetails details, double widgetWidth) {
    // Single-finger pan: scroll the timeline.
    if (details.pointerCount < 2) {
      setState(() {
        // Moving finger right = scrolling timeline right = going back in time.
        _scrollOffset = (_scrollOffset - details.focalPointDelta.dx).clamp(
          _dayStartSeconds * _pixelsPerSecond,
          _dayEndSeconds * _pixelsPerSecond,
        );
      });
      _firePositionChanged();
      return;
    }

    // Multi-finger pinch: zoom while keeping center time fixed.
    final scale = details.scale;
    if (scale == _scaleStart) return;

    final centerTimeBeforeZoom = _centerTime;

    // Scale pixelsPerSecond. Clamp to reasonable bounds.
    // Min: show full 24h; Max: show ~1 min (very zoomed in).
    final minPps = widgetWidth / _dayEndSeconds;
    final maxPps = widgetWidth / 60.0;
    final newPps = (_ppsAtScaleStart * scale).clamp(minPps, maxPps);

    final oldZoom = _zoomLevel;
    final newZoom = _closestZoomPreset(widgetWidth);
    setState(() {
      _pixelsPerSecond = newPps;
      // Restore scroll offset to keep center time fixed.
      _scrollOffset =
          centerTimeBeforeZoom.inMilliseconds / 1000.0 * _pixelsPerSecond;
      // Update zoom level to closest preset.
      _zoomLevel = newZoom;
    });
    if (newZoom != oldZoom) {
      widget.onZoomChanged?.call(newZoom);
    }
  }

  void _onScaleEnd(ScaleEndDetails details) {
    setState(() {
      _isDragging = false;
      _ignoreExternalUpdates = false;
    });
    widget.onDragEnd();
    _startPlaybackTickerIfNeeded();
  }

  // ─── Tap-to-Seek Handling ────────────────────────────────────────────

  void _onTapUp(TapUpDetails details, double widgetWidth) {
    // Calculate the time at the tap position.
    final tapX = details.localPosition.dx;
    final centerSeconds = _scrollOffset / _pixelsPerSecond;
    final tapTimeSeconds =
        centerSeconds + (tapX - widgetWidth / 2) / _pixelsPerSecond;

    // Clamp to valid range.
    final targetSeconds = tapTimeSeconds.clamp(_dayStartSeconds, _dayEndSeconds);
    final targetOffset = targetSeconds * _pixelsPerSecond;

    // Animate to the target offset.
    _seekAnimController?.dispose();
    _seekAnimController = AnimationController(
      vsync: this,
      duration: NvrAnimations.seekDuration,
    );

    final startOffset = _scrollOffset;
    _seekAnimation = Tween<double>(
      begin: startOffset,
      end: targetOffset,
    ).animate(CurvedAnimation(
      parent: _seekAnimController!,
      curve: NvrAnimations.seekCurve,
    ));

    _seekAnimController!.addListener(() {
      setState(() {
        _scrollOffset = _seekAnimation!.value;
      });
      _firePositionChanged();
    });

    _seekAnimController!.forward();
  }

  /// Handle mouse scroll wheel for desktop zoom.
  void _onPointerSignal(PointerSignalEvent event, double widgetWidth) {
    if (event is PointerScrollEvent) {
      final centerTimeBeforeZoom = _centerTime;

      // Scroll up (negative dy) = zoom in, scroll down = zoom out.
      final zoomFactor = event.scrollDelta.dy > 0 ? 0.9 : 1.1;
      final minPps = widgetWidth / _dayEndSeconds;
      final maxPps = widgetWidth / 60.0;
      final newPps = (_pixelsPerSecond * zoomFactor).clamp(minPps, maxPps);

      final oldZoom = _zoomLevel;
      final newZoom = _closestZoomPreset(widgetWidth);
      setState(() {
        _pixelsPerSecond = newPps;
        _scrollOffset =
            centerTimeBeforeZoom.inMilliseconds / 1000.0 * _pixelsPerSecond;
        _zoomLevel = newZoom;
      });
      if (newZoom != oldZoom) {
        widget.onZoomChanged?.call(newZoom);
      }
    }
  }

  /// Find the TimelineZoom preset closest to the current pixelsPerSecond.
  TimelineZoom _closestZoomPreset(double widgetWidth) {
    final currentVisibleSeconds = widgetWidth / _pixelsPerSecond;
    TimelineZoom closest = TimelineZoom.values.first;
    double closestDiff = double.infinity;

    for (final z in TimelineZoom.values) {
      final diff = (z.visibleDuration.inSeconds - currentVisibleSeconds).abs();
      if (diff < closestDiff) {
        closestDiff = diff;
        closest = z;
      }
    }
    return closest;
  }

  // ─── Zoom Preset Buttons ─────────────────────────────────────────────

  void _setZoomPreset(TimelineZoom zoom, double widgetWidth) {
    final centerTimeBeforeZoom = _centerTime;
    final oldZoom = _zoomLevel;

    setState(() {
      _zoomLevel = zoom;
      _pixelsPerSecond = widgetWidth / zoom.visibleDuration.inSeconds;
      _scrollOffset =
          centerTimeBeforeZoom.inMilliseconds / 1000.0 * _pixelsPerSecond;
    });
    if (zoom != oldZoom) {
      widget.onZoomChanged?.call(zoom);
    }
  }

  // ─── Format ──────────────────────────────────────────────────────────

  String _formatDuration(Duration d) {
    final h = d.inHours.clamp(0, 23).toString().padLeft(2, '0');
    final m = (d.inMinutes % 60).toString().padLeft(2, '0');
    final s = (d.inSeconds % 60).toString().padLeft(2, '0');
    return '$h:$m:$s';
  }

  // ─── Build ───────────────────────────────────────────────────────────

  @override
  Widget build(BuildContext context) {
    return Column(
      mainAxisSize: MainAxisSize.min,
      children: [
        // Timeline body
        SizedBox(
          height: _timelineHeight,
          child: LayoutBuilder(
            builder: (context, constraints) {
              final width = constraints.maxWidth;

              // Recalculate pps based on actual width.
              // We do this in the build phase to keep things in sync.
              // Use addPostFrameCallback on first build for clean initialization.
              WidgetsBinding.instance.addPostFrameCallback((_) {
                if (mounted) {
                  final oldPps = _pixelsPerSecond;
                  final newPps =
                      width / _zoomLevel.visibleDuration.inSeconds;
                  if ((newPps - oldPps).abs() > 0.001) {
                    final centerTimeBefore = _centerTime;
                    _pixelsPerSecond = newPps;
                    _scrollOffset = centerTimeBefore.inMilliseconds / 1000.0 *
                        _pixelsPerSecond;
                  }
                }
              });

              // For the current frame, compute pps directly.
              final effectivePps =
                  width / _zoomLevel.visibleDuration.inSeconds;

              return Listener(
                onPointerSignal: (event) =>
                    _onPointerSignal(event, width),
                child: GestureDetector(
                  onTapUp: (details) => _onTapUp(details, width),
                  onScaleStart: _onScaleStart,
                  onScaleUpdate: (details) =>
                      _onScaleUpdate(details, width),
                  onScaleEnd: _onScaleEnd,
                  child: Container(
                    color: NvrColors.of(context).bgSecondary,
                    child: Stack(
                      children: [
                        // Scrolling timeline content
                        CustomPaint(
                          painter: TimelinePainter(
                            centerTime: _centerTime,
                            pixelsPerSecond: effectivePps,
                            segments: widget.segments,
                            events: widget.events,
                            bookmarks: widget.bookmarks,
                            intensityBuckets: widget.intensityBuckets,
                            bucketDuration: widget.bucketDuration,
                            dayStart: widget.dayStart,
                            colors: NvrColors.of(context),
                          ),
                          size: Size(width, _timelineHeight),
                        ),
                        // Fixed center playhead
                        Positioned(
                          left: width / 2 - 1,
                          top: 0,
                          bottom: 0,
                          child: _PlayheadWidget(
                            isDragging: _isDragging,
                            timeLabel: _formatDuration(_centerTime),
                          ),
                        ),
                      ],
                    ),
                  ),
                ),
              );
            },
          ),
        ),
        // Zoom preset buttons — hidden when zoom is externally controlled.
        if (widget.zoomLevel == null)
          _ZoomBar(
            currentZoom: _zoomLevel,
            onZoomSelected: (zoom) {
              // We need the width; use a post-frame callback context.
              final box = context.findRenderObject() as RenderBox?;
              final width = box?.size.width ?? 400;
              _setZoomPreset(zoom, width);
            },
          ),
      ],
    );
  }
}

// ─── Playhead Widget ─────────────────────────────────────────────────────────

class _PlayheadWidget extends StatelessWidget {
  final bool isDragging;
  final String timeLabel;

  const _PlayheadWidget({
    required this.isDragging,
    required this.timeLabel,
  });

  @override
  Widget build(BuildContext context) {
    final handleSize = isDragging ? 20.0 : 16.0;

    return SizedBox(
      width: math.max(handleSize, 60), // Wide enough for the time badge
      child: Stack(
        clipBehavior: Clip.none,
        alignment: Alignment.topCenter,
        children: [
          // Time readout badge above the handle
          Positioned(
            top: -18,
            child: Container(
              padding: const EdgeInsets.symmetric(horizontal: 4, vertical: 1),
              decoration: BoxDecoration(
                color: NvrColors.of(context).accent,
                borderRadius: BorderRadius.circular(3),
              ),
              child: Text(
                timeLabel,
                style: TextStyle(
                  fontFamily: 'JetBrainsMono',
                  fontSize: 10,
                  fontWeight: FontWeight.bold,
                  color: NvrColors.of(context).bgPrimary,
                ),
              ),
            ),
          ),
          // Circular handle at top
          Positioned(
            top: 0,
            child: AnimatedContainer(
              duration: NvrAnimations.microDuration,
              curve: NvrAnimations.microCurve,
              width: handleSize,
              height: handleSize,
              decoration: BoxDecoration(
                color: NvrColors.of(context).accent,
                shape: BoxShape.circle,
                border: Border.all(
                  color: NvrColors.of(context).bgPrimary,
                  width: 3,
                ),
                boxShadow: [
                  BoxShadow(
                    color: NvrColors.of(context).accent.withValues(alpha: isDragging ? 0.5 : 0.3),
                    blurRadius: isDragging ? 12 : 6,
                    spreadRadius: isDragging ? 2 : 0,
                  ),
                ],
              ),
            ),
          ),
          // Vertical line
          Positioned(
            top: handleSize / 2,
            bottom: 0,
            child: Container(
              width: 2,
              decoration: BoxDecoration(
                color: NvrColors.of(context).accent,
                boxShadow: [
                  BoxShadow(
                    color: NvrColors.of(context).accent.withValues(alpha: 0.4),
                    blurRadius: 4,
                  ),
                ],
              ),
            ),
          ),
        ],
      ),
    );
  }
}

// ─── Zoom Bar ────────────────────────────────────────────────────────────────

class _ZoomBar extends StatelessWidget {
  final TimelineZoom currentZoom;
  final ValueChanged<TimelineZoom> onZoomSelected;

  const _ZoomBar({
    required this.currentZoom,
    required this.onZoomSelected,
  });

  @override
  Widget build(BuildContext context) {
    return Container(
      height: 28,
      decoration: BoxDecoration(
        color: NvrColors.of(context).bgTertiary,
        border: Border(
          top: BorderSide(color: NvrColors.of(context).border, width: 0.5),
        ),
      ),
      child: Row(
        mainAxisAlignment: MainAxisAlignment.center,
        children: TimelineZoom.values.map((zoom) {
          final isActive = zoom == currentZoom;
          return GestureDetector(
            onTap: () => onZoomSelected(zoom),
            child: Container(
              margin: const EdgeInsets.symmetric(horizontal: 2),
              padding: const EdgeInsets.symmetric(horizontal: 10, vertical: 4),
              decoration: BoxDecoration(
                color: isActive
                    ? NvrColors.of(context).accent.withValues(alpha: 0.15)
                    : Colors.transparent,
                borderRadius: BorderRadius.circular(4),
                border: isActive
                    ? Border.all(
                        color: NvrColors.of(context).accent.withValues(alpha: 0.4),
                        width: 0.5,
                      )
                    : null,
              ),
              child: Text(
                zoom.label,
                style: TextStyle(
                  fontFamily: 'JetBrainsMono',
                  fontSize: 10,
                  fontWeight: isActive ? FontWeight.w700 : FontWeight.w500,
                  letterSpacing: 1.0,
                  color: isActive ? NvrColors.of(context).accent : NvrColors.of(context).textMuted,
                ),
              ),
            ),
          );
        }).toList(),
      ),
    );
  }
}
