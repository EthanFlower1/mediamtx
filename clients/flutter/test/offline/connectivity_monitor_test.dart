import 'package:flutter_test/flutter_test.dart';
import 'package:mediamtx/offline/connectivity_monitor.dart';

void main() {
  late ConnectivityMonitor monitor;

  setUp(() {
    monitor = ConnectivityMonitor();
  });

  tearDown(() {
    monitor.dispose();
  });

  test('initial state is online', () {
    expect(monitor.debugState, ConnectivityState.online);
  });

  test('setOffline transitions to offline', () {
    monitor.setOffline();
    expect(monitor.debugState, ConnectivityState.offline);
  });

  test('setDegraded transitions to degraded', () {
    monitor.setDegraded();
    expect(monitor.debugState, ConnectivityState.degraded);
  });

  test('setOnline from offline transitions back to online', () {
    monitor.setOffline();
    // Use timestamp to bypass debounce
    final base = DateTime(2026, 1, 1, 0, 0, 0);
    monitor.transitionWithTimestamp(ConnectivityState.offline, base);
    monitor.transitionWithTimestamp(
      ConnectivityState.online,
      base.add(const Duration(seconds: 3)),
    );
    expect(monitor.debugState, ConnectivityState.online);
  });

  test('duplicate state is ignored', () {
    // Already online; calling setOnline should be a no-op.
    monitor.setOnline();
    expect(monitor.debugState, ConnectivityState.online);
  });

  test('debounce suppresses rapid transitions', () {
    final base = DateTime(2026, 1, 1, 0, 0, 0);

    // First transition accepted.
    monitor.transitionWithTimestamp(ConnectivityState.offline, base);
    expect(monitor.debugState, ConnectivityState.offline);

    // Second transition within 2 seconds is suppressed.
    monitor.transitionWithTimestamp(
      ConnectivityState.online,
      base.add(const Duration(milliseconds: 500)),
    );
    expect(monitor.debugState, ConnectivityState.offline);

    // After 2+ seconds the transition is accepted.
    monitor.transitionWithTimestamp(
      ConnectivityState.online,
      base.add(const Duration(seconds: 3)),
    );
    expect(monitor.debugState, ConnectivityState.online);
  });

  test('transition through all three states with debounce gaps', () {
    final base = DateTime(2026, 1, 1, 0, 0, 0);

    monitor.transitionWithTimestamp(ConnectivityState.degraded, base);
    expect(monitor.debugState, ConnectivityState.degraded);

    monitor.transitionWithTimestamp(
      ConnectivityState.offline,
      base.add(const Duration(seconds: 3)),
    );
    expect(monitor.debugState, ConnectivityState.offline);

    monitor.transitionWithTimestamp(
      ConnectivityState.online,
      base.add(const Duration(seconds: 6)),
    );
    expect(monitor.debugState, ConnectivityState.online);
  });
}
