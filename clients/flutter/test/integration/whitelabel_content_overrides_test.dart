// Integration test: White-label + Content Overrides.
//
// Verifies that applying a brand configuration through ContentOverride +
// ContentOverrideResolver uses the correct locale fallback chain:
//   exact locale -> English -> compile-time default
// Also tests asset resolution and legal URL resolution in the context of a
// branded deployment.

import 'package:flutter_test/flutter_test.dart';

import 'package:nvr_client/branding/content_override.dart';
import 'package:nvr_client/branding/content_override_resolver.dart';

void main() {
  group('White-label + Content Overrides integration', () {
    late ContentOverride brandConfig;
    late ContentOverrideResolver resolver;

    setUp(() {
      brandConfig = const ContentOverride(
        stringOverrides: {
          'en': {
            'app_title': 'Acme NVR',
            'login_button': 'Sign In',
            'welcome_message': 'Welcome to Acme Security',
          },
          'es': {
            'app_title': 'Acme NVR (ES)',
            'login_button': 'Iniciar sesion',
          },
          'fr': {
            'app_title': 'Acme NVR (FR)',
          },
        },
        logoUrl: 'https://brand.acme.com/logo.png',
        splashUrl: 'https://brand.acme.com/splash.png',
        appIconUrl: 'https://brand.acme.com/icon.png',
        termsUrl: 'https://acme.com/terms',
        privacyUrl: 'https://acme.com/privacy',
        supportEmail: 'support@acme.com',
      );
      resolver = ContentOverrideResolver(brandConfig);
    });

    test('exact locale match returns locale-specific string', () {
      final title = resolver.resolveString(
        'app_title',
        'es',
        fallback: 'Default App',
      );
      expect(title, 'Acme NVR (ES)');
    });

    test('fallback to English when locale key is missing', () {
      // French has app_title but not welcome_message -> falls back to en
      final welcome = resolver.resolveString(
        'welcome_message',
        'fr',
        fallback: 'Welcome',
      );
      expect(welcome, 'Welcome to Acme Security');
    });

    test('fallback to compile-time default when neither locale nor en has key',
        () {
      // No locale has 'nonexistent_key'
      final result = resolver.resolveString(
        'nonexistent_key',
        'de',
        fallback: 'Compile-time default',
      );
      expect(result, 'Compile-time default');
    });

    test('English locale does not fall back further', () {
      final button = resolver.resolveString(
        'login_button',
        'en',
        fallback: 'Log In',
      );
      expect(button, 'Sign In');
    });

    test('unknown locale without en fallback uses compile-time default', () {
      // 'ja' locale does not exist, and key exists in en
      final title = resolver.resolveString(
        'app_title',
        'ja',
        fallback: 'Default Title',
      );
      // en has it, so it falls back to en
      expect(title, 'Acme NVR');
    });

    test('asset resolution returns correct URLs', () {
      expect(resolver.resolveAsset('logo'), 'https://brand.acme.com/logo.png');
      expect(
          resolver.resolveAsset('splash'), 'https://brand.acme.com/splash.png');
      expect(
          resolver.resolveAsset('appIcon'), 'https://brand.acme.com/icon.png');
    });

    test('asset resolution returns null for unknown keys', () {
      expect(resolver.resolveAsset('banner'), isNull);
      expect(resolver.resolveAsset(''), isNull);
    });

    test('legal URL resolution returns correct URLs', () {
      expect(resolver.resolveLegalUrl('terms'), 'https://acme.com/terms');
      expect(resolver.resolveLegalUrl('privacy'), 'https://acme.com/privacy');
      expect(resolver.resolveLegalUrl('supportEmail'), 'support@acme.com');
    });

    test('legal URL resolution returns null for unknown keys', () {
      expect(resolver.resolveLegalUrl('eula'), isNull);
    });

    test('empty resolver returns fallbacks for everything', () {
      final emptyResolver = ContentOverrideResolver.empty;

      expect(
        emptyResolver.resolveString('app_title', 'en', fallback: 'Default'),
        'Default',
      );
      expect(emptyResolver.resolveAsset('logo'), isNull);
      expect(emptyResolver.resolveLegalUrl('terms'), isNull);
    });

    test('ContentOverride serialization round-trip preserves overrides', () {
      final json = brandConfig.toJson();
      final deserialized = ContentOverride.fromJson(json);

      // Verify string overrides survived
      final resolver2 = ContentOverrideResolver(deserialized);
      expect(
        resolver2.resolveString('app_title', 'es', fallback: 'X'),
        'Acme NVR (ES)',
      );
      expect(
        resolver2.resolveString('welcome_message', 'fr', fallback: 'X'),
        'Welcome to Acme Security',
      );
      expect(resolver2.resolveAsset('logo'), 'https://brand.acme.com/logo.png');
      expect(resolver2.resolveLegalUrl('supportEmail'), 'support@acme.com');
    });

    test('multiple brand configs can coexist for different tenants', () {
      final tenantA = ContentOverride(
        stringOverrides: const {
          'en': {'app_title': 'Tenant A NVR'},
        },
        logoUrl: 'https://a.com/logo.png',
      );
      final tenantB = ContentOverride(
        stringOverrides: const {
          'en': {'app_title': 'Tenant B Security'},
        },
        logoUrl: 'https://b.com/logo.png',
      );

      final resolverA = ContentOverrideResolver(tenantA);
      final resolverB = ContentOverrideResolver(tenantB);

      expect(
        resolverA.resolveString('app_title', 'en', fallback: 'X'),
        'Tenant A NVR',
      );
      expect(
        resolverB.resolveString('app_title', 'en', fallback: 'X'),
        'Tenant B Security',
      );
      expect(resolverA.resolveAsset('logo'), 'https://a.com/logo.png');
      expect(resolverB.resolveAsset('logo'), 'https://b.com/logo.png');
    });
  });
}
