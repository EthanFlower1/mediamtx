// KAI-303 — PushChannel abstraction + per-platform implementations.
//
// This file draws a seam the cloud-platform team (lead-cloud) relies on:
// everything above it is in-app Dart; everything below it is a plugin +
// native code. The interface is deliberately tiny so a fake can be swapped
// in during tests without pulling firebase_messaging into the Dart VM.

import 'dart:async';
import 'dart:io' show Platform;

import 'package:flutter/foundation.dart';

import 'notification_strings.dart';
import 'push_message.dart';

/// Abstract platform push channel. One implementation per target platform.
///
/// Contract:
///   * `start()` must be idempotent — calling it twice is a no-op.
///   * `stop()` must release any native listeners.
///   * `getDeviceToken()` may return `null` when the platform refuses to
///     issue one (e.g. user denied permission, firebase not initialised,
///     or the desktop stub).
///   * `incoming` is a broadcast stream — multiple listeners are allowed.
abstract class PushChannel {
  /// Returns the current device token, or `null` if one cannot be issued.
  ///
  /// Implementations should cache the token and only go to the platform on
  /// first call + on token refresh. [NotificationService] re-registers the
  /// device on every session switch regardless.
  Future<String?> getDeviceToken();

  /// Broadcast stream of incoming push messages. Emits nothing until
  /// [start] has been called.
  Stream<PushMessage> get incoming;

  /// Start listening for pushes. Idempotent.
  Future<void> start();

  /// Stop listening and release platform resources.
  Future<void> stop();

  /// Opaque platform tag used when the client registers with the backend.
  /// Matches the `platform` enum that lead-cloud will define on the
  /// `RegisterDevice` RPC.
  String get platformTag;
}

/// APNs push channel. On iOS this will forward the token and payloads
/// received from the iOS native side (APS + UNUserNotificationCenter).
///
/// In this PR we do NOT instantiate firebase_messaging at construction
/// time — the plugin imports are resolved lazily inside [start]. That way
/// tests can construct an [ApnsPushChannel] on a non-iOS host without
/// loading the platform plugin.
class ApnsPushChannel implements PushChannel {
  final _controller = StreamController<PushMessage>.broadcast();
  bool _started = false;
  String? _token;

  @override
  String get platformTag => 'ios_apns';

  @override
  Stream<PushMessage> get incoming => _controller.stream;

  @override
  Future<String?> getDeviceToken() async => _token;

  @override
  Future<void> start() async {
    if (_started) return;
    _started = true;
    // TODO(lead-cloud / follow-up): wire FirebaseMessaging.instance here.
    //   * FirebaseMessaging.instance.getAPNSToken() -> _token
    //   * FirebaseMessaging.onMessage.listen((m) => _controller.add(...))
    // Requires google-services init to be landed in a separate credential-
    // landing PR — see Info.plist comment.
    assert(() {
      debugPrint(
        'ApnsPushChannel.start(): native wiring deferred pending '
        'Firebase credential landing. See Info.plist comment for details.',
      );
      return true;
    }());
  }

  @override
  Future<void> stop() async {
    if (!_started) return;
    _started = false;
    await _controller.close();
  }

  /// Test-only: feed a message into the stream as if it arrived from APNs.
  @visibleForTesting
  void debugDeliver(PushMessage m) => _controller.add(m);

  /// Test-only: override the cached device token.
  @visibleForTesting
  void debugSetToken(String? t) => _token = t;
}

/// FCM push channel for Android.
class FcmPushChannel implements PushChannel {
  final _controller = StreamController<PushMessage>.broadcast();
  bool _started = false;
  String? _token;

  @override
  String get platformTag => 'android_fcm';

  @override
  Stream<PushMessage> get incoming => _controller.stream;

  @override
  Future<String?> getDeviceToken() async => _token;

  @override
  Future<void> start() async {
    if (_started) return;
    _started = true;
    // TODO(lead-cloud / follow-up): wire FirebaseMessaging.instance here.
    //   * FirebaseMessaging.instance.getToken() -> _token
    //   * FirebaseMessaging.onMessage.listen((m) => _controller.add(...))
    //   * FirebaseMessaging.onBackgroundMessage(_bg) for background.
    // Requires google-services.json to land — see AndroidManifest comment.
    assert(() {
      debugPrint(
        'FcmPushChannel.start(): native wiring deferred pending '
        'google-services.json landing.',
      );
      return true;
    }());
  }

  @override
  Future<void> stop() async {
    if (!_started) return;
    _started = false;
    await _controller.close();
  }

  @visibleForTesting
  void debugDeliver(PushMessage m) => _controller.add(m);

  @visibleForTesting
  void debugSetToken(String? t) => _token = t;
}

/// Web Push channel using the browser's `PushManager` + Service Worker.
class WebPushChannel implements PushChannel {
  final _controller = StreamController<PushMessage>.broadcast();
  bool _started = false;
  String? _token;

  @override
  String get platformTag => 'web_push';

  @override
  Stream<PushMessage> get incoming => _controller.stream;

  @override
  Future<String?> getDeviceToken() async => _token;

  @override
  Future<void> start() async {
    if (_started) return;
    _started = true;
    // TODO(lead-cloud / follow-up): register the Service Worker, call
    // `pushManager.subscribe({userVisibleOnly: true, applicationServerKey})`,
    // and bridge postMessage events from the SW into _controller.
    assert(() {
      debugPrint(
        'WebPushChannel.start(): Service Worker wiring deferred pending '
        'VAPID key landing.',
      );
      return true;
    }());
  }

  @override
  Future<void> stop() async {
    if (!_started) return;
    _started = false;
    await _controller.close();
  }

  @visibleForTesting
  void debugDeliver(PushMessage m) => _controller.add(m);

  @visibleForTesting
  void debugSetToken(String? t) => _token = t;
}

/// Desktop stub. macOS/Windows/Linux get a no-op channel for v1: no token,
/// no inbound messages, and a single debug warning on [start].
///
/// Real native integration (UNUserNotificationCenter on macOS, Windows
/// Notification Platform, libnotify/freedesktop on Linux) is tracked as a
/// follow-up. Callers should treat "no token" as "push disabled" — the
/// [NotificationService] falls back to in-app polling when this is the
/// active channel.
class DesktopPushChannel implements PushChannel {
  final _controller = StreamController<PushMessage>.broadcast();
  final NotificationStrings strings;
  bool _started = false;

  DesktopPushChannel({this.strings = NotificationStrings.en});

  @override
  String get platformTag => 'desktop_stub';

  @override
  Stream<PushMessage> get incoming => _controller.stream;

  @override
  Future<String?> getDeviceToken() async => null;

  @override
  Future<void> start() async {
    if (_started) return;
    _started = true;
    // Intentional runtime warning — desktop users should know push is inert.
    debugPrint('DesktopPushChannel: ${strings.desktopStubWarning}');
  }

  @override
  Future<void> stop() async {
    if (!_started) return;
    _started = false;
    await _controller.close();
  }
}

/// Factory returning the correct [PushChannel] for the current platform.
///
/// Test code should construct fakes directly rather than calling this.
PushChannel currentPlatformPushChannel({
  NotificationStrings strings = NotificationStrings.en,
}) {
  if (kIsWeb) return WebPushChannel();
  // `Platform.is*` throws on web, so guard behind kIsWeb above.
  if (Platform.isIOS) return ApnsPushChannel();
  if (Platform.isAndroid) return FcmPushChannel();
  return DesktopPushChannel(strings: strings);
}
