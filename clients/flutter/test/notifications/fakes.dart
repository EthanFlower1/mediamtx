// KAI-303 — Shared fakes for the notifications test suite.

import 'dart:async';

import 'package:nvr_client/notifications/push_channel.dart';
import 'package:nvr_client/notifications/push_message.dart';
import 'package:nvr_client/notifications/push_subscription_client.dart';

class FakePushChannel implements PushChannel {
  final _controller = StreamController<PushMessage>.broadcast();
  String? token;
  bool started = false;
  bool stopped = false;

  FakePushChannel({this.token = 'fake-device-token'});

  @override
  String get platformTag => 'fake_test';

  @override
  Stream<PushMessage> get incoming => _controller.stream;

  @override
  Future<String?> getDeviceToken() async => token;

  @override
  Future<void> start() async {
    started = true;
  }

  @override
  Future<void> stop() async {
    stopped = true;
    await _controller.close();
  }

  void deliver(PushMessage m) => _controller.add(m);
}

class FakePushSubscriptionClient implements PushSubscriptionClient {
  final Map<String, List<PushSubscription>> _subs = {};
  int _idCounter = 0;
  int registerCalls = 0;
  String? lastRegisteredDirectory;
  String? lastRegisteredToken;
  String? lastRegisteredPlatform;

  @override
  Future<String> registerDevice({
    required String deviceToken,
    required String platform,
    required String directoryConnectionId,
  }) async {
    registerCalls++;
    lastRegisteredDirectory = directoryConnectionId;
    lastRegisteredToken = deviceToken;
    lastRegisteredPlatform = platform;
    final id = 'sub-${_idCounter++}';
    _subs[id] = [];
    return id;
  }

  @override
  Future<void> subscribe({
    required String subscriptionId,
    required String cameraId,
    required Set<PushMessageKind> eventKinds,
  }) async {
    final list = _subs[subscriptionId] ?? (throw StateError('unknown sub'));
    list.removeWhere((s) => s.cameraId == cameraId);
    list.add(PushSubscription(
      id: '$subscriptionId:$cameraId',
      cameraId: cameraId,
      eventKinds: Set.of(eventKinds),
    ));
  }

  @override
  Future<void> unsubscribe({
    required String subscriptionId,
    required String cameraId,
  }) async {
    final list = _subs[subscriptionId] ?? (throw StateError('unknown sub'));
    list.removeWhere((s) => s.cameraId == cameraId);
  }

  @override
  Future<List<PushSubscription>> listSubscriptions(String subscriptionId) async {
    return List.unmodifiable(_subs[subscriptionId] ?? const []);
  }

  @override
  Future<void> deleteDevice(String subscriptionId) async {
    _subs.remove(subscriptionId);
  }
}
