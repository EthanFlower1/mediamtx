import 'brand_push_config.dart';

/// Abstract interface for push notification registration.
///
/// Concrete implementations will talk to FCM / APNs / a custom backend.
/// During early development only [FakePushRegistrationService] is provided.
abstract class PushRegistrationService {
  /// Register a device for push notifications using the given brand config.
  Future<void> register({
    required String deviceToken,
    required BrandPushConfig config,
  });

  /// Unregister a device from push notifications.
  Future<void> unregister({required String deviceToken});
}

/// A recording fake suitable for unit and widget tests.
///
/// Every call to [register] / [unregister] is appended to [calls] so tests
/// can assert on the interaction history.
class FakePushRegistrationService implements PushRegistrationService {
  /// Ordered log of method invocations.
  final List<PushServiceCall> calls = [];

  @override
  Future<void> register({
    required String deviceToken,
    required BrandPushConfig config,
  }) async {
    calls.add(PushServiceCall.register(deviceToken: deviceToken, config: config));
  }

  @override
  Future<void> unregister({required String deviceToken}) async {
    calls.add(PushServiceCall.unregister(deviceToken: deviceToken));
  }
}

/// Represents a single recorded call against [FakePushRegistrationService].
class PushServiceCall {
  const PushServiceCall._({
    required this.method,
    required this.deviceToken,
    this.config,
  });

  factory PushServiceCall.register({
    required String deviceToken,
    required BrandPushConfig config,
  }) =>
      PushServiceCall._(
        method: 'register',
        deviceToken: deviceToken,
        config: config,
      );

  factory PushServiceCall.unregister({required String deviceToken}) =>
      PushServiceCall._(
        method: 'unregister',
        deviceToken: deviceToken,
      );

  final String method;
  final String deviceToken;
  final BrandPushConfig? config;

  @override
  String toString() => 'PushServiceCall($method, token=$deviceToken)';
}
