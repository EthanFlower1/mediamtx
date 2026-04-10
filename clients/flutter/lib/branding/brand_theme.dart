// KAI-305 — Pure function that builds a Material 3 [ThemeData] from a
// [BrandConfig]. Kept intentionally small and *pure* (no side effects, no
// globals, no reads from providers) so it is trivially testable and safe
// to call during widget rebuilds.
//
// We deliberately do NOT touch or depend on `lib/theme/nvr_theme.dart` —
// the existing Tactical HUD theme is the unbranded default; this is a
// parallel layer so integrator rebrands can't accidentally regress the
// core app look when the branding provider is not overridden.
library;

import 'package:flutter/material.dart';

import 'brand_config.dart';

/// Builds a Material 3 [ThemeData] seeded from the brand's primary color.
///
/// The [dark] flag selects light or dark brightness. The returned theme is
/// intentionally minimal — it seeds a [ColorScheme] and wires font family
/// defaults. Widget-level theme extensions (card shapes, button radii, etc.)
/// are left to the host app so rebrands remain additive.
ThemeData buildBrandTheme(BrandConfig config, {required bool dark}) {
  final brightness = dark ? Brightness.dark : Brightness.light;

  final colorScheme = ColorScheme.fromSeed(
    seedColor: config.primaryColor,
    brightness: brightness,
  ).copyWith(
    secondary: config.secondaryColor,
  );

  return ThemeData(
    useMaterial3: true,
    brightness: brightness,
    colorScheme: colorScheme,
    scaffoldBackgroundColor: colorScheme.surface,
    appBarTheme: AppBarTheme(
      backgroundColor: colorScheme.surface,
      foregroundColor: colorScheme.onSurface,
      elevation: 0,
      scrolledUnderElevation: 0,
    ),
    elevatedButtonTheme: ElevatedButtonThemeData(
      style: ElevatedButton.styleFrom(
        backgroundColor: colorScheme.primary,
        foregroundColor: colorScheme.onPrimary,
        minimumSize: const Size(0, 44),
        shape: RoundedRectangleBorder(
          borderRadius: BorderRadius.circular(6),
        ),
      ),
    ),
  );
}
