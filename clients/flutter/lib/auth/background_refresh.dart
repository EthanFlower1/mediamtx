// KAI-298 — Platform-specific background token refresh.
//
// Platform matrix:
//
//   Android   — `workmanager` package. Schedules a periodic task every 15
//               minutes that the OS executes even when the app is in the
//               background. Doze mode can defer execution; the spec accepts
//               best-effort delivery. The foreground `Timer`-based refresh
//               (from RefreshScheduler / TokenRefresher) is the primary path;
//               WorkManager is the safety net for backgrounded apps.
//
//   iOS       — BGTaskScheduler via `background_fetch` plugin. Apple imposes
//               a ~15 minute minimum interval and throttles based on user
//               patterns. A BGAppRefreshTask is registered and rescheduled on
//               each execution. LIMITATION: iOS can silently skip executions
//               if the system is under memory/battery pressure; this is
//               unavoidable and documented below.
//
//   Desktop   — No true OS-level background task scheduling. A
//               `Timer.periodic` runs inside the app process; it pauses when
//               the OS suspends the process (unlikely on desktop). Acceptable
//               for desktop NVR use-cases where the machine is typically on.
//
//   Web       — No background execution. The browser may suspend the tab
//               (page visibility change). A `Timer.periodic` runs while the
//               tab is active. Users on web must keep the tab open or
//               re-authenticate on return. LIMITATION: documented.
//
// All platforms fall back to an in-process `Timer.periodic` when the platform
// channel is unavailable (e.g. integration tests running against a mock host).
//
// This file deliberately does NOT import `workmanager` or `background_fetch`
// directly — both are imported in the platform-specific initialisation code
// (`BackgroundRefreshAndroidPlugin` / `BackgroundRefreshIosPlugin`) so the
// test VM never loads those symbols. The seam is `PlatformBackgroundBinder`.

import 'dart:async';

import 'package:flutter/foundation.dart' show defaultTargetPlatform, kIsWeb, TargetPlatform;

import 'token_refresh.dart';
import 'token_store.dart';

// ---------------------------------------------------------------------------
// PlatformBackgroundBinder — pluggable seam
// ---------------------------------------------------------------------------

/// Contract for platform-specific background task registration.
///
/// Production implementations live in `BackgroundRefreshAndroidPlugin` and
/// `BackgroundRefreshIosPlugin`. Tests use `FakePlatformBackgroundBinder`.
abstract class PlatformBackgroundBinder {
  /// Register (or re-register) the periodic background refresh task with the
  /// OS scheduler. The OS calls [taskCallback] whenever it executes the task.
  ///
  /// [connectionIds] is the set of connection IDs whose tokens should be
  /// refreshed. The binder passes this to the task so it knows which
  /// connections to act on when woken.
  ///
  /// May be called multiple times (e.g. on login or when connections change).
  /// Implementations must be idempotent — duplicate registrations should
  /// replace the previous one, not stack.
  Future<void> register({
    required List<String> connectionIds,
  });

  /// Cancel the background task registration. Called on full sign-out.
  Future<void> cancel();
}

/// No-op binder used on Web, when the platform channel is unavailable,
/// and in tests. Falls back to the foreground timer only.
class NoOpPlatformBinder implements PlatformBackgroundBinder {
  const NoOpPlatformBinder();

  @override
  Future<void> register({required List<String> connectionIds}) async {}

  @override
  Future<void> cancel() async {}
}

// ---------------------------------------------------------------------------
// WorkManager binder (Android)
// ---------------------------------------------------------------------------
//
// PRODUCTION USE: In `main.dart` call:
//
//   await Workmanager().initialize(callbackDispatcher, isInDebugMode: false);
//   BackgroundRefreshController.instance.platformBinder =
//       WorkManagerBackgroundBinder();
//
// The `callbackDispatcher` is a top-level function (required by WorkManager's
// isolate architecture) defined in `background_tasks_android.dart`.
//
// We do NOT import `workmanager` here — that would pull the plugin into the
// test VM and throw `MissingPluginException`. The production code that
// instantiates `WorkManagerBackgroundBinder` is in a file that is only
// compiled into Android targets.

// Task name registered with WorkManager.
const String kWorkManagerTaskName = 'kai_token_refresh';

// Minimum fetch interval (WorkManager enforces a floor of 15 min on Android).
const Duration kWorkManagerInterval = Duration(minutes: 15);

// ---------------------------------------------------------------------------
// BGTaskScheduler binder (iOS)
// ---------------------------------------------------------------------------
//
// PRODUCTION USE: In `AppDelegate.swift` register the task ID:
//   BGTaskScheduler.shared.register(
//     forTaskWithIdentifier: "com.kaivue.tokenrefresh",
//     using: nil) { task in ... }
//
// Then in `main.dart`:
//   BackgroundRefreshController.instance.platformBinder =
//       BgTaskSchedulerBinder();
//
// LIMITATION: iOS throttles BGAppRefreshTask based on the device's learned
// usage patterns. The system may defer or skip executions. The foreground
// `Timer.periodic` in BackgroundRefreshController is the reliable path on iOS.

const String kBgTaskIdentifier = 'com.kaivue.tokenrefresh';

// ---------------------------------------------------------------------------
// BackgroundRefreshController — the main orchestrator
// ---------------------------------------------------------------------------

/// Periodic in-process refresh timer. Runs on all platforms as the primary
/// foreground refresh mechanism; the OS-level binders are layered on top.
const Duration kForegroundRefreshPollInterval = Duration(minutes: 14);

/// Manages background and foreground periodic token refresh for all active
/// connections.
///
/// Lifecycle:
///   1. `BackgroundRefreshController.instance.start(...)` on app startup
///      (called by `AuthLifecycle`).
///   2. `onConnectionsChanged(connectionIds)` whenever the list of known
///      connections changes (login, logout, forget).
///   3. `stop()` on full app sign-out / dispose.
///
/// The controller owns:
///   * A `Timer.periodic` that fires every ~14 minutes in the foreground.
///   * An optional `PlatformBackgroundBinder` for OS-level background tasks.
///
/// All refresh calls are routed through `TokenRefresher`, which owns the HTTP
/// client and the Completer-based debounce.
class BackgroundRefreshController {
  BackgroundRefreshController._();

  static final BackgroundRefreshController instance =
      BackgroundRefreshController._();

  TokenRefresher? _refresher;
  PlatformBackgroundBinder _binder = const NoOpPlatformBinder();
  Timer? _timer;
  List<String> _connectionIds = const [];

  // ---------- configuration ----------

  /// Override the platform binder. Called from `main.dart` after plugin init.
  set platformBinder(PlatformBackgroundBinder binder) {
    _binder = binder;
  }

  // ---------- lifecycle ----------

  /// Initialise the controller. Must be called once on app start before any
  /// refresh occurs.
  ///
  /// [connectionIds]: IDs of all known connections with persisted tokens.
  /// [endpointUrlFor]: maps a connection ID to its base endpoint URL.
  Future<void> start({
    required TokenStore tokenStore,
    required TokenRefresher refresher,
    required List<String> connectionIds,
    required String Function(String connectionId) endpointUrlFor,
  }) async {
    _refresher = refresher;
    _connectionIds = List.unmodifiable(connectionIds);

    // Register the OS-level binder (no-op on desktop/web).
    await _binder.register(connectionIds: _connectionIds);

    // Start the foreground timer.
    _startTimer(endpointUrlFor: endpointUrlFor);
  }

  /// Call whenever the set of active connection IDs changes.
  Future<void> onConnectionsChanged({
    required List<String> connectionIds,
    required String Function(String connectionId) endpointUrlFor,
  }) async {
    _connectionIds = List.unmodifiable(connectionIds);
    await _binder.register(connectionIds: _connectionIds);
    _stopTimer();
    _startTimer(endpointUrlFor: endpointUrlFor);
  }

  /// Stop all refresh activity. Call on full sign-out.
  Future<void> stop() async {
    _stopTimer();
    await _binder.cancel();
    _connectionIds = const [];
  }

  // ---------- internal ----------

  void _startTimer({
    required String Function(String connectionId) endpointUrlFor,
  }) {
    // Platform-specific interval notes:
    //   Desktop / Web: 14-minute timer is the primary mechanism.
    //   Android: WorkManager fires separately; the timer catches any gaps.
    //   iOS: BGTaskScheduler fires separately; the timer catches foreground
    //        periods between BGTaskScheduler executions.
    _timer = Timer.periodic(kForegroundRefreshPollInterval, (_) {
      _onTimerFired(endpointUrlFor: endpointUrlFor);
    });
  }

  void _stopTimer() {
    _timer?.cancel();
    _timer = null;
  }

  void _onTimerFired({
    required String Function(String connectionId) endpointUrlFor,
  }) {
    final refresher = _refresher;
    if (refresher == null) return;

    for (final id in _connectionIds) {
      // Fire-and-forget per connection. Errors are swallowed here because
      // the token's current state is checked inside `ensureFresh`; if it
      // fails with AuthInvalidException the next authenticated API call
      // will catch it and bounce the user to login. AuthLifecycle registers
      // a global error handler that surfaces AuthInvalidException to the UI;
      // transient network errors are retried on the next tick.
      _refreshSafe(
        refresher: refresher,
        connectionId: id,
        endpointUrl: endpointUrlFor(id),
      );
    }
  }

  /// Swallows all errors from a single ensureFresh call. The error surface
  /// for AuthInvalidException is through AuthLifecycle.events; transient
  /// network errors are retried on the next timer tick.
  Future<void> _refreshSafe({
    required TokenRefresher refresher,
    required String connectionId,
    required String endpointUrl,
  }) async {
    try {
      await refresher.ensureFresh(
        connectionId: connectionId,
        endpointUrl: endpointUrl,
      );
    } catch (_) {
      // Intentionally swallowed — see comment in _onTimerFired.
    }
  }

  // ---------- test helpers ----------

  /// Force an immediate poll cycle. Test-only; not called in production.
  void testTriggerPoll({required String Function(String) endpointUrlFor}) {
    _onTimerFired(endpointUrlFor: endpointUrlFor);
  }
}

// ---------------------------------------------------------------------------
// Platform-capability helpers
// ---------------------------------------------------------------------------

/// Whether this platform supports OS-level background task scheduling.
///
/// Desktop and Web do not — they rely on the foreground `Timer.periodic`.
/// Android and iOS have platform binders available.
bool get platformSupportsOsBackgroundRefresh {
  if (kIsWeb) return false;
  return defaultTargetPlatform == TargetPlatform.android ||
      defaultTargetPlatform == TargetPlatform.iOS;
}

/// Human-readable limitation note for each platform. Used in docs + CI parity
/// matrix.
String get backgroundRefreshLimitationNote {
  if (kIsWeb) {
    return 'Web: no background execution. Timer runs while the tab is active. '
        'Users must keep the tab open or re-authenticate on return.';
  }
  switch (defaultTargetPlatform) {
    case TargetPlatform.android:
      return 'Android: WorkManager periodic task, minimum 15-minute interval. '
          'Doze mode may defer execution. Best-effort delivery.';
    case TargetPlatform.iOS:
      return 'iOS: BGTaskScheduler BGAppRefreshTask. Apple may throttle or skip '
          'executions based on device usage patterns. Not guaranteed.';
    case TargetPlatform.macOS:
    case TargetPlatform.linux:
    case TargetPlatform.windows:
      return 'Desktop: in-process Timer.periodic only. Pauses when the process '
          'is suspended (rare on desktop). No OS-level background task.';
    case TargetPlatform.fuchsia:
      return 'Fuchsia: unsupported target; in-process timer only.';
  }
}
