// KAI-301 — Snapshot refresh controller.
//
// Drives per-camera JPEG polling for grids that exceed the WebRTC cell cap.
// Each camera gets its own [Timer] with a random period in the configured
// jitter window, so sixteen cameras don't all hit the recorder on the same
// tick.
//
// Lifecycle:
//   * [start] — creates one periodic timer per camera + kicks off an
//     immediate fetch so the first frame shows up without waiting.
//   * [pauseForLifecycle] — called when the app goes to background.
//   * [resumeFromLifecycle] — called when the app returns to foreground.
//   * [dispose] — cancels everything.
//
// The controller does NOT drive HTTP itself; it delegates both the URL
// minting (via [StreamUrlMinter]) and the actual byte fetch (via an injected
// [SnapshotFetcher] closure). Tests swap in a synchronous fake fetcher.

import 'dart:async';
import 'dart:math' as math;

import 'package:flutter/foundation.dart';

import 'camera.dart';
import 'render_mode.dart' show kMinSnapshotJitterSeconds, kMaxSnapshotJitterSeconds;
import 'stream_url_minter.dart';

/// Single snapshot emission.
@immutable
class SnapshotFrame {
  final String cameraId;
  final Uint8List bytes;
  final DateTime fetchedAt;

  const SnapshotFrame({
    required this.cameraId,
    required this.bytes,
    required this.fetchedAt,
  });
}

/// Closure that resolves a [SnapshotTicket] to raw bytes. The controller is
/// agnostic about how — in production this is a Dio call with the session's
/// auth headers; in tests it's whatever the test provides.
typedef SnapshotFetcher = Future<Uint8List> Function(SnapshotTicket ticket);

/// Tick hook (for tests) — called every time a timer fires so tests can
/// observe fetches without waiting on wall-clock.
typedef SnapshotTickCallback = void Function(String cameraId);

class SnapshotRefreshController {
  final StreamUrlMinter minter;
  final SnapshotFetcher fetcher;

  /// Per-camera minimum period (inclusive). Default [kMinSnapshotJitterSeconds].
  final Duration minPeriod;

  /// Per-camera maximum period (exclusive). Default [kMaxSnapshotJitterSeconds].
  final Duration maxPeriod;

  /// Seeded RNG — injectable for deterministic tests.
  final math.Random _rng;

  final StreamController<SnapshotFrame> _frames =
      StreamController<SnapshotFrame>.broadcast();
  final Map<String, Timer> _timers = {};
  final Map<String, Duration> _periods = {};
  List<Camera> _cameras = const [];
  bool _paused = false;
  bool _disposed = false;

  /// Optional per-tick hook used by tests to assert a camera fetched.
  @visibleForTesting
  SnapshotTickCallback? onTick;

  SnapshotRefreshController({
    required this.minter,
    required this.fetcher,
    Duration? minPeriod,
    Duration? maxPeriod,
    math.Random? rng,
  })  : minPeriod = minPeriod ??
            Duration(milliseconds: (kMinSnapshotJitterSeconds * 1000).round()),
        maxPeriod = maxPeriod ??
            Duration(milliseconds: (kMaxSnapshotJitterSeconds * 1000).round()),
        _rng = rng ?? math.Random();

  Stream<SnapshotFrame> get frames => _frames.stream;

  @visibleForTesting
  Map<String, Duration> get currentPeriods => Map.unmodifiable(_periods);

  @visibleForTesting
  bool get isPaused => _paused;

  /// Begin polling [cameras]. Re-invoking replaces the active set.
  void start(List<Camera> cameras) {
    if (_disposed) {
      throw StateError('SnapshotRefreshController.start after dispose');
    }
    _cameras = List.unmodifiable(cameras);
    _cancelAll();
    if (_paused) return;
    for (final c in _cameras) {
      _scheduleCamera(c);
    }
  }

  /// Pause all timers without clearing the camera list. Safe to call multiple
  /// times; no-op after dispose.
  void pauseForLifecycle() {
    if (_disposed) return;
    _paused = true;
    _cancelAll();
  }

  /// Resume polling after a background trip. Re-arms timers and re-fetches
  /// once immediately so the UI updates without waiting a full period.
  void resumeFromLifecycle() {
    if (_disposed) return;
    if (!_paused) return;
    _paused = false;
    for (final c in _cameras) {
      _scheduleCamera(c);
    }
  }

  void dispose() {
    if (_disposed) return;
    _disposed = true;
    _cancelAll();
    _frames.close();
  }

  // ── Internals ───────────────────────────────────────────────────────────

  void _cancelAll() {
    for (final t in _timers.values) {
      t.cancel();
    }
    _timers.clear();
    _periods.clear();
  }

  Duration _pickPeriod() {
    final minMs = minPeriod.inMilliseconds;
    final maxMs = maxPeriod.inMilliseconds;
    assert(maxMs >= minMs, 'maxPeriod must be >= minPeriod');
    if (maxMs == minMs) return Duration(milliseconds: minMs);
    final ms = minMs + _rng.nextInt(maxMs - minMs + 1);
    return Duration(milliseconds: ms);
  }

  void _scheduleCamera(Camera camera) {
    final period = _pickPeriod();
    _periods[camera.id] = period;
    // Immediate first fetch so the tile isn't blank for up to `period`.
    _fetchOnce(camera);
    _timers[camera.id] = Timer.periodic(period, (_) {
      if (_paused || _disposed) return;
      _fetchOnce(camera);
    });
  }

  Future<void> _fetchOnce(Camera camera) async {
    onTick?.call(camera.id);
    try {
      final ticket = await minter.mintSnapshot(camera.id);
      if (_disposed) return;
      final bytes = await fetcher(ticket);
      if (_disposed) return;
      _frames.add(SnapshotFrame(
        cameraId: camera.id,
        bytes: bytes,
        fetchedAt: DateTime.now(),
      ));
    } catch (e, st) {
      // Swallow per-tick failures — the next tick will retry. Log via
      // debugPrint so tests can see what happened without failing.
      debugPrint('SnapshotRefreshController: fetch failed for '
          '${camera.id}: $e\n$st');
    }
  }
}
