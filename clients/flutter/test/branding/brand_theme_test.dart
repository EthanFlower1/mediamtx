// KAI-305 — buildBrandTheme tests.
import 'package:flutter/material.dart';
import 'package:flutter_test/flutter_test.dart';
import 'package:nvr_client/branding/brand_config.dart';
import 'package:nvr_client/branding/brand_theme.dart';

void main() {
  group('buildBrandTheme', () {
    test('returns a Material3 theme', () {
      final theme = buildBrandTheme(BrandConfig.raikadaDefault(), dark: false);
      expect(theme.useMaterial3, isTrue);
    });

    test('light variant has Brightness.light', () {
      final theme = buildBrandTheme(BrandConfig.raikadaDefault(), dark: false);
      expect(theme.brightness, Brightness.light);
      expect(theme.colorScheme.brightness, Brightness.light);
    });

    test('dark variant has Brightness.dark', () {
      final theme = buildBrandTheme(BrandConfig.raikadaDefault(), dark: true);
      expect(theme.brightness, Brightness.dark);
      expect(theme.colorScheme.brightness, Brightness.dark);
    });

    test('light and dark variants differ', () {
      final light = buildBrandTheme(BrandConfig.raikadaDefault(), dark: false);
      final dark = buildBrandTheme(BrandConfig.raikadaDefault(), dark: true);
      expect(light.colorScheme.surface, isNot(dark.colorScheme.surface));
    });

    test('secondary color in ColorScheme matches BrandConfig', () {
      const cfg = BrandConfig(
        appName: 'Test',
        primaryColorHex: '#1F6FEB',
        secondaryColorHex: '#FF00FF',
        logoUrl: '',
        supportUrl: '',
        privacyUrl: '',
        termsUrl: '',
      );
      final theme = buildBrandTheme(cfg, dark: false);
      expect(theme.colorScheme.secondary.toARGB32(), 0xFFFF00FF);
    });

    test('different seed colors produce different color schemes', () {
      const cfgA = BrandConfig(
        appName: 'A',
        primaryColorHex: '#FF0000',
        secondaryColorHex: '#000000',
        logoUrl: '',
        supportUrl: '',
        privacyUrl: '',
        termsUrl: '',
      );
      const cfgB = BrandConfig(
        appName: 'B',
        primaryColorHex: '#00FF00',
        secondaryColorHex: '#000000',
        logoUrl: '',
        supportUrl: '',
        privacyUrl: '',
        termsUrl: '',
      );
      final themeA = buildBrandTheme(cfgA, dark: false);
      final themeB = buildBrandTheme(cfgB, dark: false);
      expect(
        themeA.colorScheme.primary.toARGB32(),
        isNot(themeB.colorScheme.primary.toARGB32()),
      );
    });

    test('scaffoldBackgroundColor matches colorScheme.surface', () {
      final theme = buildBrandTheme(BrandConfig.raikadaDefault(), dark: true);
      expect(theme.scaffoldBackgroundColor, theme.colorScheme.surface);
    });
  });
}
