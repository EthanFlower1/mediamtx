// KAI-303 — Cross-platform notification permission request.
//
// The surface is a single function — `requestNotificationPermission()` —
// that returns a [PermissionState]. Per-platform behaviour:
//
//   * iOS       — provisional APNs pattern (UNAuthorizationOptions.provisional).
//   * Android   — POST_NOTIFICATIONS runtime permission on API 33+; auto-granted
//                 on <= API 32.
//   * Web       — Notification.requestPermission().
//   * Desktop   — returns [PermissionState.granted] as a stub; real native
//                 integration tracked as a follow-up.
//
// The actual platform calls are deferred behind a [NotificationPermissionHost]
// so tests can inject a fake host without pulling the firebase_messaging or
// flutter_local_notifications plugins into the Dart VM.

import 'dart:async';
import 'dart:io' show Platform;

import 'package:flutter/foundation.dart';

/// Outcome of a permission request.
enum PermissionState {
  /// User granted (or system auto-granted) full notification permission.
  granted,

  /// User denied permission.
  denied,

  /// iOS-specific: the app may deliver notifications quietly until the user
  /// manually promotes the app to "prominent" in Settings.
  provisional,

  /// Platform does not expose a permission state (or the call failed).
  unknown,
}

/// Host abstraction — all platform-touching calls go through this so tests
/// can inject a fake.
abstract class NotificationPermissionHost {
  Future<PermissionState> requestIos();
  Future<PermissionState> requestAndroid();
  Future<PermissionState> requestWeb();
}

/// Default host — safe stubs. Real implementations wire to
/// firebase_messaging / flutter_local_notifications / the browser API and
/// will be added in the same follow-up PR that lands credentials. This
/// scaffold just returns `unknown` so nothing silently claims "granted"
/// when the native wiring isn't done.
class _DefaultHost implements NotificationPermissionHost {
  const _DefaultHost();

  @override
  Future<PermissionState> requestIos() async => PermissionState.unknown;

  @override
  Future<PermissionState> requestAndroid() async => PermissionState.unknown;

  @override
  Future<PermissionState> requestWeb() async => PermissionState.unknown;
}

/// Request notification permission for the current platform.
///
/// [host] exists solely for tests; production callers should omit it.
Future<PermissionState> requestNotificationPermission({
  NotificationPermissionHost host = const _DefaultHost(),
}) async {
  if (kIsWeb) return host.requestWeb();
  try {
    if (Platform.isIOS) return await host.requestIos();
    if (Platform.isAndroid) return await host.requestAndroid();
  } catch (_) {
    return PermissionState.unknown;
  }
  // macOS / Windows / Linux desktop — stub for v1.
  return PermissionState.granted;
}
