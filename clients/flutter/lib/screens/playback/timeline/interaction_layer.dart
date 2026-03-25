import 'package:flutter/material.dart';
import '../../../models/recording.dart';
import 'timeline_viewport.dart';

class InteractionLayer extends StatefulWidget {
  final TimelineViewport viewport;
  final Duration position;
  final List<MotionEvent> events;
  final DateTime dayStart;
  final ValueChanged<Duration> onSeek;
  final void Function(MotionEvent event, Offset position) onEventLongPress;
  final ValueChanged<double> onZoom;
  final ValueChanged<double> onPan;
  final ValueChanged<bool> onDragStateChanged;
  final ValueChanged<double?> onDragXChanged;

  const InteractionLayer({
    super.key,
    required this.viewport,
    required this.position,
    required this.events,
    required this.dayStart,
    required this.onSeek,
    required this.onEventLongPress,
    required this.onZoom,
    required this.onPan,
    required this.onDragStateChanged,
    required this.onDragXChanged,
  });

  @override
  State<InteractionLayer> createState() => _InteractionLayerState();
}

class _InteractionLayerState extends State<InteractionLayer> {
  bool _draggingPlayhead = false;

  bool _isNearPlayhead(double px) {
    final playheadX = widget.viewport.timeToPixel(widget.position);
    return (px - playheadX).abs() < 40;
  }

  MotionEvent? _findNearestEvent(double px) {
    MotionEvent? nearest;
    double nearestPxDist = 30;

    for (final event in widget.events) {
      final eventDur = event.startTime.difference(widget.dayStart);
      final pxDist = (widget.viewport.timeToPixel(eventDur) - px).abs();
      if (pxDist < nearestPxDist) {
        nearest = event;
        nearestPxDist = pxDist;
      }
    }
    return nearest;
  }

  @override
  Widget build(BuildContext context) {
    return GestureDetector(
      behavior: HitTestBehavior.opaque,
      onTapUp: (details) {
        final time = widget.viewport.pixelToTime(details.localPosition.dx);
        widget.onSeek(time);
      },
      onLongPressStart: (details) {
        final event = _findNearestEvent(details.localPosition.dx);
        if (event != null) {
          widget.onEventLongPress(event, details.globalPosition);
        }
      },
      onHorizontalDragStart: (details) {
        if (_isNearPlayhead(details.localPosition.dx)) {
          _draggingPlayhead = true;
          widget.onDragStateChanged(true);
          widget.onDragXChanged(details.localPosition.dx);
        }
      },
      onHorizontalDragUpdate: (details) {
        if (_draggingPlayhead) {
          final x = details.localPosition.dx.clamp(0.0, widget.viewport.widthPx);
          widget.onDragXChanged(x);
        } else {
          widget.onPan(details.delta.dx);
        }
      },
      onHorizontalDragEnd: (details) {
        if (_draggingPlayhead) {
          _draggingPlayhead = false;
          widget.onDragStateChanged(false);
          final x = details.localPosition.dx.clamp(0.0, widget.viewport.widthPx);
          final time = widget.viewport.pixelToTime(x);
          widget.onDragXChanged(null);
          widget.onSeek(time);
        }
      },
      onScaleStart: (_) {},
      onScaleUpdate: (details) {
        if (details.pointerCount >= 2) {
          widget.onZoom(details.scale);
        }
      },
      child: const SizedBox.expand(),
    );
  }
}
