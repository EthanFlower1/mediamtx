// KAI-301 — Grid CPU/battery telemetry reporter.
//
// Emits periodic [GridTelemetryEvent]s describing the current grid workload.
// The reporter is intentionally tiny — it does not read real CPU counters
// (those aren't portable across our six targets) and it does not ship a
// metrics sink. Events are logged via [debugPrint] and exposed on a stream
// so the eventual OTEL/Prometheus wiring (coordinate with lead-sre on
// KAI-422) can subscribe without reshaping this layer.
//
// Battery: `battery_plus` is NOT yet in pubspec.yaml. When it lands, replace
// [_readBatteryPercent] with a real call; `batteryPercent` is nullable today
// so test + call sites don't need to change.

import 'dart:async';

import 'package:flutter/foundation.dart';

import 'render_mode.dart';

@immutable
class GridTelemetryEvent {
  final DateTime timestamp;
  final int cellCount;
  final RenderMode renderMode;
  final bool alwaysLiveOverride;
  final bool isOnLan;

  /// Percent [0,100]. Null when the host has no battery API available
  /// (desktop without a battery, or `battery_plus` not yet wired).
  final int? batteryPercent;

  /// Cheap "work-density" proxy — microseconds of wall clock consumed by the
  /// reporter's own tick handler. Not a real CPU counter; good enough as a
  /// trend signal until KAI-422 lands.
  final int tickBudgetMicros;

  const GridTelemetryEvent({
    required this.timestamp,
    required this.cellCount,
    required this.renderMode,
    required this.alwaysLiveOverride,
    required this.isOnLan,
    required this.tickBudgetMicros,
    this.batteryPercent,
  });

  @override
  String toString() => 'GridTelemetryEvent('
      'cells=$cellCount, '
      'mode=$renderMode, '
      'alwaysLive=$alwaysLiveOverride, '
      'onLan=$isOnLan, '
      'battery=$batteryPercent, '
      'tickMicros=$tickBudgetMicros'
      ')';
}

/// Optional hook to plug in a real battery reader. Returns a percent in
/// [0,100] or `null` when unknown. The default reader always returns `null`.
typedef BatteryPercentReader = Future<int?> Function();

/// Snapshot of the grid's current state, sampled by the reporter on each tick.
typedef GridStateSnapshot = GridTelemetrySnapshot Function();

@immutable
class GridTelemetrySnapshot {
  final int cellCount;
  final RenderMode renderMode;
  final bool alwaysLiveOverride;
  final bool isOnLan;

  const GridTelemetrySnapshot({
    required this.cellCount,
    required this.renderMode,
    required this.alwaysLiveOverride,
    required this.isOnLan,
  });
}

class GridTelemetryReporter {
  final Duration cadence;
  final BatteryPercentReader batteryReader;
  final GridStateSnapshot snapshotProvider;
  final void Function(GridTelemetryEvent)? onEvent;

  final StreamController<GridTelemetryEvent> _events =
      StreamController<GridTelemetryEvent>.broadcast();
  Timer? _timer;
  bool _disposed = false;

  GridTelemetryReporter({
    required this.snapshotProvider,
    this.cadence = const Duration(seconds: 5),
    BatteryPercentReader? batteryReader,
    this.onEvent,
  }) : batteryReader = batteryReader ?? _nullBattery;

  Stream<GridTelemetryEvent> get events => _events.stream;

  void start() {
    if (_disposed) {
      throw StateError('GridTelemetryReporter.start after dispose');
    }
    _timer?.cancel();
    _timer = Timer.periodic(cadence, (_) => _tick());
  }

  void stop() {
    _timer?.cancel();
    _timer = null;
  }

  /// Sample once immediately. Useful for tests and for the first-foreground
  /// event before the timer has fired.
  Future<void> sampleOnce() => _tick();

  Future<void> _tick() async {
    if (_disposed) return;
    final started = DateTime.now();
    final snap = snapshotProvider();
    final battery = await batteryReader();
    final elapsed = DateTime.now().difference(started).inMicroseconds;
    final event = GridTelemetryEvent(
      timestamp: started,
      cellCount: snap.cellCount,
      renderMode: snap.renderMode,
      alwaysLiveOverride: snap.alwaysLiveOverride,
      isOnLan: snap.isOnLan,
      batteryPercent: battery,
      tickBudgetMicros: elapsed,
    );
    if (_disposed) return;
    debugPrint('[grid-telemetry] $event');
    onEvent?.call(event);
    if (!_events.isClosed) _events.add(event);
  }

  void dispose() {
    if (_disposed) return;
    _disposed = true;
    _timer?.cancel();
    _events.close();
  }
}

Future<int?> _nullBattery() async => null;
