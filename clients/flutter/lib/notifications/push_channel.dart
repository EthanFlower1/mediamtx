// KAI-303 — PushChannel abstraction + per-platform implementations.
//
// This file draws a seam the cloud-platform team (lead-cloud) relies on:
// everything above it is in-app Dart; everything below it is a plugin +
// native code. The interface is deliberately tiny so a fake can be
// swapped in during tests without pulling firebase_messaging into the
// Dart VM.
//
// Hard contract (cto + lead-security gate on PR #165):
//
//   Every platform channel's decode path MUST route the raw native
//   payload map through [PushMessage.fromRemote], which enforces the
//   metadata-only contract (event_id + tenant_id + priority). On a
//   [PushPayloadViolation] the channel LOGS + DROPS the message — never
//   crashes the app. Use [_deliverFromRemote] so every channel shares
//   the same behavior.

import 'dart:async';
import 'dart:io' show Platform;

import 'package:flutter/foundation.dart';

import 'notification_strings.dart';
import 'push_message.dart';

/// Abstract platform push channel. One implementation per target
/// platform.
abstract class PushChannel {
  Future<String?> getDeviceToken();
  Stream<PushMessage> get incoming;
  Future<void> start();
  Future<void> stop();

  /// Opaque platform tag used when the client registers with the
  /// backend.
  String get platformTag;
}

/// Shared decode helper. Every platform channel routes raw native
/// payloads through this function so the metadata-only contract is
/// enforced in exactly one place.
///
/// On success, [onMessage] is called with the decoded [PushMessage].
/// On [PushPayloadViolation], the error is logged via `debugPrint` and
/// the message is dropped. The app never crashes on a bad payload —
/// that is deliberate: a single misbehaving dispatcher must not be able
/// to take down a production device.
void decodeRemoteAndForward(
  Map<String, dynamic> rawNativePayload,
  void Function(PushMessage) onMessage, {
  String channelTag = 'PushChannel',
}) {
  try {
    final msg = PushMessage.fromRemote(rawNativePayload);
    onMessage(msg);
  } on PushPayloadViolation catch (e) {
    debugPrint('$channelTag: dropping push with payload violation: $e');
  } catch (e) {
    debugPrint('$channelTag: dropping push with decode error: $e');
  }
}

/// APNs push channel. On iOS this will forward the token and payloads
/// received from the iOS native side (APS + UNUserNotificationCenter).
///
/// APNs payload contract (KAI-303): the server MUST send
///   `mutable-content: 1` + `content-available: 1` for silent wake and
///   MUST NOT include an `alert` dict. The custom-data block carries
///   ONLY `event_id`, `tenant_id`, `priority`. Info.plist has the full
///   comment — see `ios/Runner/Info.plist`.
///
/// TODO(KAI-303-followup): iOS Notification Service Extension for
/// optional privacy-preserving content fetch. Default OFF; no NSE
/// target is built in this PR. Tracked as a follow-up.
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
    //   * FirebaseMessaging.onMessage.listen((rm) =>
    //       decodeRemoteAndForward(rm.data, _controller.add,
    //         channelTag: 'ApnsPushChannel'));
    assert(() {
      debugPrint(
        'ApnsPushChannel.start(): native wiring deferred pending '
        'Firebase credential landing. See Info.plist comment.',
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

  /// Test-only: feed a raw remote payload map through the decode path.
  @visibleForTesting
  void debugDeliverFromRemote(Map<String, dynamic> raw) {
    decodeRemoteAndForward(raw, _controller.add, channelTag: 'ApnsPushChannel');
  }

  /// Test-only: feed an already-decoded PushMessage into the stream.
  @visibleForTesting
  void debugDeliver(PushMessage m) => _controller.add(m);

  @visibleForTesting
  void debugSetToken(String? t) => _token = t;
}

/// FCM push channel for Android.
///
/// FCM payload contract (KAI-303): data-only, NO `notification` block.
/// This means FCM will NOT auto-display the notification when the app
/// is killed — the foreground handler / FCM background isolate must
/// format and display the notification client-side using i18n'd
/// strings. This is the HIPAA-correct behavior: a `notification` block
/// would cause FCM to auto-render the title/body on the lock screen
/// WITHOUT going through our client-side EventDetailsLoader, leaking
/// PII. See AndroidManifest.xml for the full comment block.
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
    //   * FirebaseMessaging.onMessage.listen((rm) =>
    //       decodeRemoteAndForward(rm.data, _controller.add,
    //         channelTag: 'FcmPushChannel'));
    //   * FirebaseMessaging.onBackgroundMessage(_bg) — the isolate must
    //     call decodeRemoteAndForward + render the visible notification
    //     itself using NotificationStrings.titleForKind(details.kind)
    //     AFTER fetching EventDetails from Directory.
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
  void debugDeliverFromRemote(Map<String, dynamic> raw) {
    decodeRemoteAndForward(raw, _controller.add, channelTag: 'FcmPushChannel');
  }

  @visibleForTesting
  void debugDeliver(PushMessage m) => _controller.add(m);

  @visibleForTesting
  void debugSetToken(String? t) => _token = t;
}

/// Web Push channel using the browser's `PushManager` + Service Worker.
///
/// Web Push payload contract (KAI-303): same as FCM/APNs — only
/// `event_id` + `tenant_id` + `priority`. The Service Worker's `push`
/// handler must call the Directory API on interaction, not on receipt,
/// to avoid leaking details to browser push infrastructure.
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
    // and bridge postMessage events from the SW into _controller via
    // decodeRemoteAndForward(rawPayloadMap, _controller.add, ...).
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
  void debugDeliverFromRemote(Map<String, dynamic> raw) {
    decodeRemoteAndForward(raw, _controller.add, channelTag: 'WebPushChannel');
  }

  @visibleForTesting
  void debugDeliver(PushMessage m) => _controller.add(m);

  @visibleForTesting
  void debugSetToken(String? t) => _token = t;
}

/// Desktop stub. macOS/Windows/Linux get a no-op channel for v1.
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
    debugPrint('DesktopPushChannel: ${strings.desktopStubWarning}');
  }

  @override
  Future<void> stop() async {
    if (!_started) return;
    _started = false;
    await _controller.close();
  }

  @visibleForTesting
  void debugDeliverFromRemote(Map<String, dynamic> raw) {
    decodeRemoteAndForward(raw, _controller.add,
        channelTag: 'DesktopPushChannel');
  }
}

/// Factory returning the correct [PushChannel] for the current platform.
PushChannel currentPlatformPushChannel({
  NotificationStrings strings = NotificationStrings.en,
}) {
  if (kIsWeb) return WebPushChannel();
  if (Platform.isIOS) return ApnsPushChannel();
  if (Platform.isAndroid) return FcmPushChannel();
  return DesktopPushChannel(strings: strings);
}
