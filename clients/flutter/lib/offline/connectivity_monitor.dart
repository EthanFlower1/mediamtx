import 'package:flutter_riverpod/flutter_riverpod.dart';

/// Represents the current network connectivity state.
enum ConnectivityState {
  /// Device has full network access.
  online,

  /// Device has no network access.
  offline,

  /// Device has limited or unreliable connectivity.
  degraded,
}

/// Monitors network connectivity and exposes the current [ConnectivityState].
///
/// Transitions are debounced: rapid changes within [_debounceDuration] of the
/// last accepted transition are silently ignored to avoid UI flicker.
class ConnectivityMonitor extends StateNotifier<ConnectivityState> {
  ConnectivityMonitor() : super(ConnectivityState.online);

  /// Minimum interval between accepted state transitions.
  static const _debounceDuration = Duration(seconds: 2);

  DateTime? _lastTransitionTime;

  /// Transition to [ConnectivityState.online].
  void setOnline() => _transition(ConnectivityState.online);

  /// Transition to [ConnectivityState.offline].
  void setOffline() => _transition(ConnectivityState.offline);

  /// Transition to [ConnectivityState.degraded].
  void setDegraded() => _transition(ConnectivityState.degraded);

  void _transition(ConnectivityState next) {
    if (next == state) return;

    final now = DateTime.now();
    if (_lastTransitionTime != null &&
        now.difference(_lastTransitionTime!) < _debounceDuration) {
      return;
    }

    _lastTransitionTime = now;
    state = next;
  }

  /// Visible for testing: override the internal clock for deterministic tests.
  void transitionWithTimestamp(ConnectivityState next, DateTime timestamp) {
    if (next == state) return;

    if (_lastTransitionTime != null &&
        timestamp.difference(_lastTransitionTime!) < _debounceDuration) {
      return;
    }

    _lastTransitionTime = timestamp;
    state = next;
  }
}

/// Riverpod provider for [ConnectivityMonitor].
final connectivityProvider =
    StateNotifierProvider<ConnectivityMonitor, ConnectivityState>(
  (ref) => ConnectivityMonitor(),
);
