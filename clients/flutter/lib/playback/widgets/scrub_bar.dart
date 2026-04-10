// KAI-302 — Scrub bar widget.
//
// Wraps `TimelinePainter` + gestures. Pan/drag emits `onScrub(DateTime)`.
// Pinch (touch) or scroll-wheel (desktop/web) adjusts the zoom level.
// Double-tap seeks immediately.

import 'package:flutter/gestures.dart';
import 'package:flutter/material.dart';

import '../timeline_model.dart';
import '../timeline_painter.dart';

class ScrubBar extends StatefulWidget {
  final TimelineSpan span;
  final ValueChanged<DateTime> onScrub;
  final ValueChanged<double>? onZoomChanged;
  final double initialZoom;
  final double height;

  const ScrubBar({
    super.key,
    required this.span,
    required this.onScrub,
    this.onZoomChanged,
    this.initialZoom = 1.0,
    this.height = 56.0,
  });

  @override
  State<ScrubBar> createState() => _ScrubBarState();
}

class _ScrubBarState extends State<ScrubBar> {
  late double _zoom = widget.initialZoom;
  double _scaleStartZoom = 1.0;

  DateTime _dateFromDx(double dx, double width) {
    final frac = (dx / width).clamp(0.0, 1.0);
    final ms = (widget.span.duration.inMilliseconds * frac).round();
    return widget.span.start.add(Duration(milliseconds: ms));
  }

  void _setZoom(double next) {
    final clamped = next.clamp(0.25, 16.0);
    if (clamped == _zoom) return;
    setState(() => _zoom = clamped);
    widget.onZoomChanged?.call(clamped);
  }

  @override
  Widget build(BuildContext context) {
    return LayoutBuilder(
      builder: (context, constraints) {
        final width = constraints.maxWidth;
        return Listener(
          onPointerSignal: (event) {
            if (event is PointerScrollEvent) {
              _setZoom(_zoom * (event.scrollDelta.dy < 0 ? 1.1 : 1 / 1.1));
            }
          },
          child: GestureDetector(
            behavior: HitTestBehavior.opaque,
            onTapDown: (d) {
              widget.onScrub(_dateFromDx(d.localPosition.dx, width));
            },
            onScaleStart: (_) {
              _scaleStartZoom = _zoom;
            },
            onScaleUpdate: (d) {
              if (d.pointerCount >= 2 && d.scale != 1.0) {
                _setZoom(_scaleStartZoom * d.scale);
              } else {
                // Single-pointer pan → scrub.
                widget.onScrub(
                    _dateFromDx(d.localFocalPoint.dx, width));
              }
            },
            child: SizedBox(
              width: width,
              height: widget.height,
              child: CustomPaint(
                painter: TimelinePainter(span: widget.span, zoom: _zoom),
                size: Size(width, widget.height),
              ),
            ),
          ),
        );
      },
    );
  }
}
