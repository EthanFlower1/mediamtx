// KAI-302 — Riverpod providers for the playback timeline layer.
//
// These are intentionally minimal — the existing `lib/providers/*` files
// handle the live HTTP shim wiring. KAI-302's providers exist so widgets
// in `lib/playback/widgets/` can consume a typed PlaybackClient + span
// without touching the legacy providers and without churning test setup.

import 'package:flutter/material.dart';
import 'package:flutter_riverpod/flutter_riverpod.dart';

import 'playback_client.dart';
import 'timeline_model.dart';

/// The PlaybackClient used by the KAI-302 timeline layer. Override this
/// at the root of a test or a real run (with HttpPlaybackClient) to wire
/// transport.
final playbackClientProvider = Provider<PlaybackClient>((ref) {
  return FakePlaybackClient();
});

/// Selected playback speed. One of {1.0, 2.0, 4.0, 8.0}.
final playbackSpeedProvider = StateProvider<double>((_) => 1.0);

/// Load a timeline span for `(cameraId, range)`. AutoDispose so ranges
/// that leave the viewport are garbage-collected.
final timelineSpanProvider = FutureProvider.autoDispose
    .family<TimelineSpan, TimelineSpanKey>((ref, key) async {
  final client = ref.watch(playbackClientProvider);
  return client.loadSpan(
    cameraId: key.cameraId,
    start: key.range.start,
    end: key.range.end,
  );
});

/// Stable family key for `timelineSpanProvider`.
@immutable
class TimelineSpanKey {
  final String cameraId;
  final DateTimeRange range;
  const TimelineSpanKey({required this.cameraId, required this.range});

  @override
  bool operator ==(Object other) =>
      identical(this, other) ||
      (other is TimelineSpanKey &&
          other.cameraId == cameraId &&
          other.range.start == range.start &&
          other.range.end == range.end);

  @override
  int get hashCode =>
      Object.hash(cameraId, range.start, range.end);
}
