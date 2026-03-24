class TimelineViewport {
  final Duration visibleStart;
  final Duration visibleEnd;
  final double widthPx;

  static const Duration dayDuration = Duration(hours: 24);

  const TimelineViewport({
    required this.visibleStart,
    required this.visibleEnd,
    required this.widthPx,
  });

  Duration get visibleDuration => visibleEnd - visibleStart;

  double get zoomLevel =>
      dayDuration.inMilliseconds / visibleDuration.inMilliseconds;

  double timeToPixel(Duration time) {
    final offset = (time - visibleStart).inMilliseconds;
    final span = visibleDuration.inMilliseconds;
    if (span == 0) return 0;
    return (offset / span) * widthPx;
  }

  Duration pixelToTime(double px) {
    final frac = px / widthPx;
    final ms = visibleStart.inMilliseconds +
        (frac * visibleDuration.inMilliseconds).round();
    return Duration(milliseconds: ms.clamp(0, dayDuration.inMilliseconds));
  }

  Duration get gridInterval {
    final z = zoomLevel;
    if (z < 2) return const Duration(hours: 3);
    if (z < 6) return const Duration(hours: 1);
    if (z < 12) return const Duration(minutes: 30);
    if (z < 24) return const Duration(minutes: 15);
    if (z < 48) return const Duration(minutes: 5);
    return const Duration(minutes: 1);
  }

  TimelineViewport zoom(double factor, Duration focalTime) {
    final newDurationMs =
        (visibleDuration.inMilliseconds / factor).round();
    final clampedDuration = newDurationMs.clamp(
      const Duration(minutes: 24).inMilliseconds,
      dayDuration.inMilliseconds,
    );

    final focalFrac = (focalTime - visibleStart).inMilliseconds /
        visibleDuration.inMilliseconds;
    final newStartMs =
        focalTime.inMilliseconds - (focalFrac * clampedDuration).round();
    final newStart = Duration(
        milliseconds: newStartMs.clamp(0, dayDuration.inMilliseconds - clampedDuration));
    final newEnd = Duration(
        milliseconds: (newStart.inMilliseconds + clampedDuration)
            .clamp(0, dayDuration.inMilliseconds));

    return TimelineViewport(
      visibleStart: newStart,
      visibleEnd: newEnd,
      widthPx: widthPx,
    );
  }

  TimelineViewport pan(double deltaPx) {
    final deltaMs =
        (deltaPx / widthPx * visibleDuration.inMilliseconds).round();
    var newStartMs = visibleStart.inMilliseconds - deltaMs;
    newStartMs = newStartMs.clamp(
        0, dayDuration.inMilliseconds - visibleDuration.inMilliseconds);
    return TimelineViewport(
      visibleStart: Duration(milliseconds: newStartMs),
      visibleEnd:
          Duration(milliseconds: newStartMs + visibleDuration.inMilliseconds),
      widthPx: widthPx,
    );
  }
}
