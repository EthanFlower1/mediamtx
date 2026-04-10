// KAI-305 — White-label runtime brand config model.
//
// `BrandConfig` is an immutable value object describing everything the
// Flutter client needs to re-skin itself for a given integrator tenant:
//   * display name
//   * primary + secondary accent colors (hex strings; parsed on demand)
//   * logo URL (signed URL served by KAI-353 brand asset storage)
//   * support / privacy / terms URLs for the in-app about & legal screens
//
// Color strings are stored in their original hex form (e.g. `#1F6FEB` or
// `1F6FEB`) so the wire format stays human-readable and trivially diffable.
// Callers get `Color` objects via `primaryColor` / `secondaryColor`, which
// fall back to the Kaivue default on parse failure so a malformed tenant
// config can never crash the app shell.
library;

import 'package:flutter/material.dart';

/// Thrown when a hex color string cannot be parsed into a [Color].
class BrandHexFormatException implements Exception {
  const BrandHexFormatException(this.input);

  final String input;

  @override
  String toString() => 'BrandHexFormatException: "$input" is not a valid '
      'hex color. Expected #RRGGBB, #AARRGGBB, RRGGBB, or AARRGGBB.';
}

/// Immutable runtime brand configuration for a single integrator tenant.
@immutable
class BrandConfig {
  const BrandConfig({
    required this.appName,
    required this.primaryColorHex,
    required this.secondaryColorHex,
    required this.logoUrl,
    required this.supportUrl,
    required this.privacyUrl,
    required this.termsUrl,
  });

  /// Built-in Kaivue default. Used when no tenant config is available, when
  /// the remote fetch fails, or as the fixture for unbranded builds.
  factory BrandConfig.kaivueDefault() => const BrandConfig(
        appName: 'Kaivue',
        primaryColorHex: '#1F6FEB',
        secondaryColorHex: '#6E7681',
        logoUrl: '',
        supportUrl: 'https://kaivue.com/support',
        privacyUrl: 'https://kaivue.com/privacy',
        termsUrl: 'https://kaivue.com/terms',
      );

  final String appName;
  final String primaryColorHex;
  final String secondaryColorHex;
  final String logoUrl;
  final String supportUrl;
  final String privacyUrl;
  final String termsUrl;

  /// Parses [primaryColorHex]. Falls back to the Kaivue default primary
  /// color on malformed input rather than throwing, so the app shell can
  /// never crash on a broken tenant config.
  Color get primaryColor => parseBrandHex(
        primaryColorHex,
        fallback: const Color(0xFF1F6FEB),
      );

  /// Parses [secondaryColorHex]. Falls back to the Kaivue default secondary.
  Color get secondaryColor => parseBrandHex(
        secondaryColorHex,
        fallback: const Color(0xFF6E7681),
      );

  BrandConfig copyWith({
    String? appName,
    String? primaryColorHex,
    String? secondaryColorHex,
    String? logoUrl,
    String? supportUrl,
    String? privacyUrl,
    String? termsUrl,
  }) {
    return BrandConfig(
      appName: appName ?? this.appName,
      primaryColorHex: primaryColorHex ?? this.primaryColorHex,
      secondaryColorHex: secondaryColorHex ?? this.secondaryColorHex,
      logoUrl: logoUrl ?? this.logoUrl,
      supportUrl: supportUrl ?? this.supportUrl,
      privacyUrl: privacyUrl ?? this.privacyUrl,
      termsUrl: termsUrl ?? this.termsUrl,
    );
  }

  @override
  bool operator ==(Object other) {
    if (identical(this, other)) return true;
    return other is BrandConfig &&
        other.appName == appName &&
        other.primaryColorHex == primaryColorHex &&
        other.secondaryColorHex == secondaryColorHex &&
        other.logoUrl == logoUrl &&
        other.supportUrl == supportUrl &&
        other.privacyUrl == privacyUrl &&
        other.termsUrl == termsUrl;
  }

  @override
  int get hashCode => Object.hash(
        appName,
        primaryColorHex,
        secondaryColorHex,
        logoUrl,
        supportUrl,
        privacyUrl,
        termsUrl,
      );

  @override
  String toString() => 'BrandConfig(appName: $appName, '
      'primary: $primaryColorHex, secondary: $secondaryColorHex)';
}

/// Parses a hex color string into a [Color].
///
/// Accepted forms (leading `#` optional, whitespace trimmed, case-insensitive):
///   * `RGB`       → expanded to `RRGGBB`, alpha 0xFF
///   * `RRGGBB`    → alpha 0xFF
///   * `AARRGGBB`  → full alpha+RGB
///
/// Throws [BrandHexFormatException] on malformed input when [fallback] is
/// null; otherwise returns [fallback].
Color parseBrandHex(String input, {Color? fallback}) {
  final trimmed = input.trim();
  if (trimmed.isEmpty) {
    if (fallback != null) return fallback;
    throw BrandHexFormatException(input);
  }

  var hex = trimmed.startsWith('#') ? trimmed.substring(1) : trimmed;

  // Expand short form #RGB -> #RRGGBB.
  if (hex.length == 3) {
    hex = hex.split('').map((c) => '$c$c').join();
  }

  if (hex.length != 6 && hex.length != 8) {
    if (fallback != null) return fallback;
    throw BrandHexFormatException(input);
  }

  if (hex.length == 6) {
    hex = 'FF$hex';
  }

  final value = int.tryParse(hex, radix: 16);
  if (value == null) {
    if (fallback != null) return fallback;
    throw BrandHexFormatException(input);
  }

  return Color(value);
}
