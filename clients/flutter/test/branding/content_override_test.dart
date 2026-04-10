import 'dart:convert';

import 'package:flutter_test/flutter_test.dart';
import 'package:mediamtx_nvr/branding/content_override.dart';
import 'package:mediamtx_nvr/branding/content_override_resolver.dart';
import 'package:mediamtx_nvr/branding/content_override_strings.dart';

void main() {
  // ── ContentOverride ─────────────────────────────────────────────────

  group('ContentOverride', () {
    test('fromJson/toJson round-trip preserves all fields', () {
      final original = ContentOverride(
        stringOverrides: {
          'en': {'app_title': 'Acme NVR', 'greeting': 'Hello'},
          'es': {'app_title': 'Acme NVR (ES)'},
        },
        logoUrl: 'https://cdn.example.com/logo.png',
        splashUrl: 'https://cdn.example.com/splash.png',
        appIconUrl: 'https://cdn.example.com/icon.png',
        termsUrl: 'https://example.com/terms',
        privacyUrl: 'https://example.com/privacy',
        supportEmail: 'help@example.com',
      );

      final jsonMap = original.toJson();
      final restored = ContentOverride.fromJson(jsonMap);

      expect(restored.stringOverrides, original.stringOverrides);
      expect(restored.logoUrl, original.logoUrl);
      expect(restored.splashUrl, original.splashUrl);
      expect(restored.appIconUrl, original.appIconUrl);
      expect(restored.termsUrl, original.termsUrl);
      expect(restored.privacyUrl, original.privacyUrl);
      expect(restored.supportEmail, original.supportEmail);
    });

    test('fromJsonString round-trip through raw JSON', () {
      final original = ContentOverride(
        stringOverrides: {
          'en': {'title': 'Test'},
        },
        logoUrl: 'logo.png',
      );

      final jsonString = json.encode(original.toJson());
      final restored = ContentOverride.fromJsonString(jsonString);

      expect(restored.stringOverrides, original.stringOverrides);
      expect(restored.logoUrl, 'logo.png');
    });

    test('empty constant has all nulls and empty maps', () {
      const co = ContentOverride.empty;

      expect(co.stringOverrides, isEmpty);
      expect(co.logoUrl, isNull);
      expect(co.splashUrl, isNull);
      expect(co.appIconUrl, isNull);
      expect(co.termsUrl, isNull);
      expect(co.privacyUrl, isNull);
      expect(co.supportEmail, isNull);
    });

    test('fromJson handles missing stringOverrides gracefully', () {
      final co = ContentOverride.fromJson({
        'logoUrl': 'logo.png',
      });

      expect(co.stringOverrides, isEmpty);
      expect(co.logoUrl, 'logo.png');
    });

    test('fromJson handles null stringOverrides gracefully', () {
      final co = ContentOverride.fromJson({
        'stringOverrides': null,
      });

      expect(co.stringOverrides, isEmpty);
    });

    test('equality works for identical overrides', () {
      final a = ContentOverride(
        stringOverrides: {
          'en': {'k': 'v'},
        },
        logoUrl: 'x',
      );
      final b = ContentOverride(
        stringOverrides: {
          'en': {'k': 'v'},
        },
        logoUrl: 'x',
      );

      expect(a, equals(b));
      expect(a.hashCode, b.hashCode);
    });
  });

  // ── ContentOverrideResolver ─────────────────────────────────────────

  group('ContentOverrideResolver', () {
    final override = ContentOverride(
      stringOverrides: {
        'en': {'app_title': 'Acme NVR', 'greeting': 'Hello'},
        'es': {'app_title': 'Acme NVR (ES)'},
      },
      logoUrl: 'https://cdn.example.com/logo.png',
      splashUrl: 'https://cdn.example.com/splash.png',
      appIconUrl: 'https://cdn.example.com/icon.png',
      termsUrl: 'https://example.com/terms',
      privacyUrl: 'https://example.com/privacy',
      supportEmail: 'help@example.com',
    );

    final resolver = ContentOverrideResolver(override);

    test('returns override when present for exact locale', () {
      expect(
        resolver.resolveString('app_title', 'es', fallback: 'Default'),
        'Acme NVR (ES)',
      );
    });

    test('falls back to English when locale key missing', () {
      // 'es' has no 'greeting' key, so should fall back to 'en'.
      expect(
        resolver.resolveString('greeting', 'es', fallback: 'Default'),
        'Hello',
      );
    });

    test('falls back to English when locale not found at all', () {
      expect(
        resolver.resolveString('app_title', 'ja', fallback: 'Default'),
        'Acme NVR',
      );
    });

    test('returns fallback when no override exists', () {
      expect(
        resolver.resolveString('nonexistent_key', 'en', fallback: 'Fallback'),
        'Fallback',
      );
    });

    test('returns fallback for unknown locale and unknown key', () {
      expect(
        resolver.resolveString('nope', 'zz', fallback: 'FB'),
        'FB',
      );
    });

    // ── Asset resolution ──────────────────────────────────────────────

    test('resolves logo asset', () {
      expect(resolver.resolveAsset('logo'), 'https://cdn.example.com/logo.png');
    });

    test('resolves splash asset', () {
      expect(
          resolver.resolveAsset('splash'), 'https://cdn.example.com/splash.png');
    });

    test('resolves appIcon asset', () {
      expect(
          resolver.resolveAsset('appIcon'), 'https://cdn.example.com/icon.png');
    });

    test('returns null for unknown asset key', () {
      expect(resolver.resolveAsset('banner'), isNull);
    });

    // ── Legal URL resolution ──────────────────────────────────────────

    test('resolves terms URL', () {
      expect(resolver.resolveLegalUrl('terms'), 'https://example.com/terms');
    });

    test('resolves privacy URL', () {
      expect(resolver.resolveLegalUrl('privacy'), 'https://example.com/privacy');
    });

    test('resolves support email', () {
      expect(resolver.resolveLegalUrl('supportEmail'), 'help@example.com');
    });

    test('returns null for unknown legal key', () {
      expect(resolver.resolveLegalUrl('refund'), isNull);
    });

    // ── Empty resolver ────────────────────────────────────────────────

    test('empty resolver always returns fallback / null', () {
      const r = ContentOverrideResolver.empty;
      expect(r.resolveString('x', 'en', fallback: 'fb'), 'fb');
      expect(r.resolveAsset('logo'), isNull);
      expect(r.resolveLegalUrl('terms'), isNull);
    });
  });

  // ── ContentOverrideStringsL10n ──────────────────────────────────────

  group('ContentOverrideStringsL10n', () {
    test('English strings are non-empty', () {
      final s = ContentOverrideStringsL10n.forLocale('en');
      expect(s.contentLoading, isNotEmpty);
      expect(s.contentLoadFailed, isNotEmpty);
      expect(s.usingDefaults, isNotEmpty);
      expect(s.brandCustomized, isNotEmpty);
    });

    test('forLocale falls back to English for unknown locale', () {
      final s = ContentOverrideStringsL10n.forLocale('zh');
      final en = ContentOverrideStringsL10n.forLocale('en');
      expect(s.contentLoading, en.contentLoading);
    });

    test('Spanish locale returns Spanish strings', () {
      final s = ContentOverrideStringsL10n.forLocale('es');
      expect(s.contentLoading, contains('Cargando'));
    });

    test('all four locales return distinct contentLoading', () {
      final locales = ['en', 'es', 'fr', 'de'];
      final values =
          locales.map((l) => ContentOverrideStringsL10n.forLocale(l).contentLoading).toSet();
      expect(values.length, 4);
    });
  });
}
