// KAI-305 — BrandConfig + hex parser tests.
import 'package:flutter/material.dart';
import 'package:flutter_test/flutter_test.dart';
import 'package:nvr_client/branding/brand_config.dart';

void main() {
  group('parseBrandHex', () {
    test('parses #RRGGBB with leading hash', () {
      final color = parseBrandHex('#1F6FEB');
      expect(color.toARGB32(), 0xFF1F6FEB);
    });

    test('parses RRGGBB without leading hash', () {
      final color = parseBrandHex('1F6FEB');
      expect(color.toARGB32(), 0xFF1F6FEB);
    });

    test('parses #AARRGGBB with explicit alpha', () {
      final color = parseBrandHex('#801F6FEB');
      expect(color.toARGB32(), 0x801F6FEB);
    });

    test('expands short #RGB form', () {
      final color = parseBrandHex('#0AF');
      expect(color.toARGB32(), 0xFF00AAFF);
    });

    test('is case-insensitive', () {
      final lower = parseBrandHex('#1f6feb');
      final upper = parseBrandHex('#1F6FEB');
      expect(lower.toARGB32(), upper.toARGB32());
    });

    test('trims whitespace', () {
      final color = parseBrandHex('  #1F6FEB  ');
      expect(color.toARGB32(), 0xFF1F6FEB);
    });

    test('returns fallback on empty string', () {
      final color = parseBrandHex('', fallback: const Color(0xFF000000));
      expect(color.toARGB32(), 0xFF000000);
    });

    test('returns fallback on garbage input', () {
      final color =
          parseBrandHex('not-a-color', fallback: const Color(0xFF123456));
      expect(color.toARGB32(), 0xFF123456);
    });

    test('returns fallback on wrong length', () {
      final color =
          parseBrandHex('#12345', fallback: const Color(0xFF123456));
      expect(color.toARGB32(), 0xFF123456);
    });

    test('returns fallback on non-hex chars in a correct-length string', () {
      final color =
          parseBrandHex('#GGGGGG', fallback: const Color(0xFF123456));
      expect(color.toARGB32(), 0xFF123456);
    });

    test('throws BrandHexFormatException on bad input with no fallback', () {
      expect(
        () => parseBrandHex('nope'),
        throwsA(isA<BrandHexFormatException>()),
      );
    });

    test('throws BrandHexFormatException on empty input with no fallback', () {
      expect(
        () => parseBrandHex('  '),
        throwsA(isA<BrandHexFormatException>()),
      );
    });
  });

  group('BrandConfig', () {
    test('kaivueDefault has sensible values', () {
      final def = BrandConfig.kaivueDefault();
      expect(def.appName, 'Kaivue');
      expect(def.primaryColorHex, '#1F6FEB');
      expect(def.primaryColor.toARGB32(), 0xFF1F6FEB);
      expect(def.secondaryColor.toARGB32(), 0xFF6E7681);
    });

    test('primaryColor falls back to default on malformed hex', () {
      const cfg = BrandConfig(
        appName: 'Broken',
        primaryColorHex: 'not-hex',
        secondaryColorHex: 'also-not-hex',
        logoUrl: '',
        supportUrl: '',
        privacyUrl: '',
        termsUrl: '',
      );
      // Should not throw.
      expect(cfg.primaryColor.toARGB32(), 0xFF1F6FEB);
      expect(cfg.secondaryColor.toARGB32(), 0xFF6E7681);
    });

    test('equality is value-based', () {
      final a = BrandConfig.kaivueDefault();
      final b = BrandConfig.kaivueDefault();
      expect(a, equals(b));
      expect(a.hashCode, equals(b.hashCode));
    });

    test('copyWith overrides only the given fields', () {
      final base = BrandConfig.kaivueDefault();
      final next = base.copyWith(appName: 'Acme Security');
      expect(next.appName, 'Acme Security');
      expect(next.primaryColorHex, base.primaryColorHex);
    });
  });
}
