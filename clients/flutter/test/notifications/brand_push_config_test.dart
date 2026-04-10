import 'package:flutter_test/flutter_test.dart';
import 'package:mediamtx/notifications/brand_push_config.dart';
import 'package:mediamtx/notifications/push_brand_resolver.dart';
import 'package:mediamtx/notifications/push_registration_service.dart';
import 'package:mediamtx/notifications/push_brand_strings.dart';

void main() {
  // ---------- BrandPushConfig ----------

  group('BrandPushConfig', () {
    test('fromJson / toJson round-trip preserves all fields', () {
      final original = BrandPushConfig(
        fcmProjectId: 'proj-1',
        fcmSenderId: '123',
        apnsTeamId: 'TEAM',
        apnsKeyId: 'KEY',
        apnsBundleId: 'com.brand.app',
        webVapidPublicKey: 'vapid-key',
      );

      final json = original.toJson();
      final restored = BrandPushConfig.fromJson(json);

      expect(restored, equals(original));
    });

    test('fromJson / toJson round-trip with null fields', () {
      final sparse = BrandPushConfig(fcmProjectId: 'proj-2');
      final json = sparse.toJson();
      final restored = BrandPushConfig.fromJson(json);

      expect(restored.fcmProjectId, 'proj-2');
      expect(restored.fcmSenderId, isNull);
      expect(restored.apnsTeamId, isNull);
    });

    test('defaultConfig has expected placeholder values', () {
      final d = BrandPushConfig.defaultConfig;
      expect(d.fcmProjectId, 'dev-project-placeholder');
      expect(d.fcmSenderId, '000000000000');
      expect(d.apnsTeamId, 'DEV_TEAM_ID');
      expect(d.apnsKeyId, 'DEV_KEY_ID');
      expect(d.apnsBundleId, 'com.example.dev');
      expect(d.webVapidPublicKey, 'DEV_VAPID_PUBLIC_KEY');
    });

    test('equality for identical configs', () {
      const a = BrandPushConfig(fcmProjectId: 'x');
      const b = BrandPushConfig(fcmProjectId: 'x');
      expect(a, equals(b));
      expect(a.hashCode, equals(b.hashCode));
    });
  });

  // ---------- PushBrandResolver ----------

  group('PushBrandResolver', () {
    const resolver = PushBrandResolver();

    test('resolves valid push section from brand config', () {
      final config = resolver.resolve({
        'push': {
          'fcmProjectId': 'brand-proj',
          'fcmSenderId': '999',
          'apnsBundleId': 'com.brand.live',
        },
      });

      expect(config.fcmProjectId, 'brand-proj');
      expect(config.fcmSenderId, '999');
      expect(config.apnsBundleId, 'com.brand.live');
      expect(config.apnsTeamId, isNull);
    });

    test('falls back to default on null brand config', () {
      expect(resolver.resolve(null), equals(BrandPushConfig.defaultConfig));
    });

    test('falls back to default on empty brand config', () {
      expect(resolver.resolve({}), equals(BrandPushConfig.defaultConfig));
    });

    test('falls back to default when push key is missing', () {
      expect(
        resolver.resolve({'theme': 'dark'}),
        equals(BrandPushConfig.defaultConfig),
      );
    });
  });

  // ---------- FakePushRegistrationService ----------

  group('FakePushRegistrationService', () {
    test('records register calls', () async {
      final fake = FakePushRegistrationService();
      const config = BrandPushConfig(fcmProjectId: 'test-proj');

      await fake.register(deviceToken: 'tok-1', config: config);

      expect(fake.calls, hasLength(1));
      expect(fake.calls.first.method, 'register');
      expect(fake.calls.first.deviceToken, 'tok-1');
      expect(fake.calls.first.config, equals(config));
    });

    test('records unregister calls', () async {
      final fake = FakePushRegistrationService();

      await fake.unregister(deviceToken: 'tok-2');

      expect(fake.calls, hasLength(1));
      expect(fake.calls.first.method, 'unregister');
      expect(fake.calls.first.deviceToken, 'tok-2');
      expect(fake.calls.first.config, isNull);
    });

    test('records mixed register and unregister calls in order', () async {
      final fake = FakePushRegistrationService();
      const cfg = BrandPushConfig.defaultConfig;

      await fake.register(deviceToken: 'a', config: cfg);
      await fake.unregister(deviceToken: 'b');
      await fake.register(deviceToken: 'c', config: cfg);

      expect(fake.calls.map((c) => c.method).toList(),
          ['register', 'unregister', 'register']);
      expect(fake.calls.map((c) => c.deviceToken).toList(), ['a', 'b', 'c']);
    });
  });

  // ---------- PushBrandStringsL10n ----------

  group('PushBrandStringsL10n', () {
    test('English locale returns English strings', () {
      final s = PushBrandStringsL10n.forLocale('en');
      expect(s.pushConfigured, 'Push notifications configured');
      expect(s.pushFailed, 'Failed to register for push notifications');
    });

    test('unsupported locale falls back to English', () {
      final s = PushBrandStringsL10n.forLocale('ja');
      expect(s.pushConfigured, 'Push notifications configured');
    });

    test('German locale returns German strings', () {
      final s = PushBrandStringsL10n.forLocale('de');
      expect(s.pushConfigured, 'Push-Benachrichtigungen konfiguriert');
    });

    test('locale with region tag resolves correctly', () {
      final s = PushBrandStringsL10n.forLocale('es-MX');
      expect(s.pushConfigured, 'Notificaciones push configuradas');
    });
  });
}
