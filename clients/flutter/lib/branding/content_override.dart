/// Runtime content/string/asset overrides for white-label branding.
///
/// Proto-first seam: this model mirrors the shape of the server-side
/// BrandingConfig proto so a future gRPC/REST fetch drops straight in.
library;

import 'dart:convert';

/// Immutable value object that holds every override a white-label
/// deployment can specify at runtime.
class ContentOverride {
  /// Locale -> key -> translated value.
  ///
  /// Example: `{'en': {'app_title': 'Acme NVR'}, 'es': {'app_title': 'Acme NVR (ES)'}}`
  final Map<String, Map<String, String>> stringOverrides;

  // ── Asset URLs ──────────────────────────────────────────────────────
  /// URL or asset path for the main logo.
  final String? logoUrl;

  /// URL or asset path for the splash screen image.
  final String? splashUrl;

  /// URL or asset path for the app icon (launcher icon at runtime).
  final String? appIconUrl;

  // ── Legal / support URLs ────────────────────────────────────────────
  /// Terms of service URL.
  final String? termsUrl;

  /// Privacy policy URL.
  final String? privacyUrl;

  /// Support email address.
  final String? supportEmail;

  const ContentOverride({
    this.stringOverrides = const {},
    this.logoUrl,
    this.splashUrl,
    this.appIconUrl,
    this.termsUrl,
    this.privacyUrl,
    this.supportEmail,
  });

  /// Canonical empty instance -- no overrides applied.
  static const empty = ContentOverride();

  // ── Serialisation (proto-first seam) ────────────────────────────────

  /// Deserialise from a JSON map (e.g. fetched from the branding API).
  factory ContentOverride.fromJson(Map<String, dynamic> json) {
    return ContentOverride(
      stringOverrides: _parseStringOverrides(json['stringOverrides']),
      logoUrl: json['logoUrl'] as String?,
      splashUrl: json['splashUrl'] as String?,
      appIconUrl: json['appIconUrl'] as String?,
      termsUrl: json['termsUrl'] as String?,
      privacyUrl: json['privacyUrl'] as String?,
      supportEmail: json['supportEmail'] as String?,
    );
  }

  /// Deserialise from a raw JSON string.
  factory ContentOverride.fromJsonString(String source) {
    return ContentOverride.fromJson(
      json.decode(source) as Map<String, dynamic>,
    );
  }

  /// Serialise to a JSON-compatible map.
  Map<String, dynamic> toJson() {
    return {
      'stringOverrides': stringOverrides.map(
        (locale, entries) => MapEntry(locale, Map<String, String>.from(entries)),
      ),
      if (logoUrl != null) 'logoUrl': logoUrl,
      if (splashUrl != null) 'splashUrl': splashUrl,
      if (appIconUrl != null) 'appIconUrl': appIconUrl,
      if (termsUrl != null) 'termsUrl': termsUrl,
      if (privacyUrl != null) 'privacyUrl': privacyUrl,
      if (supportEmail != null) 'supportEmail': supportEmail,
    };
  }

  // ── Helpers ─────────────────────────────────────────────────────────

  static Map<String, Map<String, String>> _parseStringOverrides(
    dynamic raw,
  ) {
    if (raw == null) return {};
    if (raw is! Map) return {};
    final outer = raw as Map<String, dynamic>;
    return outer.map((locale, inner) {
      if (inner is! Map) return MapEntry(locale, <String, String>{});
      return MapEntry(
        locale,
        (inner as Map<String, dynamic>).map(
          (k, v) => MapEntry(k, v.toString()),
        ),
      );
    });
  }

  @override
  bool operator ==(Object other) =>
      identical(this, other) ||
      other is ContentOverride &&
          _mapsEqual(stringOverrides, other.stringOverrides) &&
          logoUrl == other.logoUrl &&
          splashUrl == other.splashUrl &&
          appIconUrl == other.appIconUrl &&
          termsUrl == other.termsUrl &&
          privacyUrl == other.privacyUrl &&
          supportEmail == other.supportEmail;

  @override
  int get hashCode => Object.hash(
        stringOverrides.length,
        logoUrl,
        splashUrl,
        appIconUrl,
        termsUrl,
        privacyUrl,
        supportEmail,
      );

  static bool _mapsEqual(
    Map<String, Map<String, String>> a,
    Map<String, Map<String, String>> b,
  ) {
    if (a.length != b.length) return false;
    for (final key in a.keys) {
      if (!b.containsKey(key)) return false;
      final aInner = a[key]!;
      final bInner = b[key]!;
      if (aInner.length != bInner.length) return false;
      for (final k in aInner.keys) {
        if (aInner[k] != bInner[k]) return false;
      }
    }
    return true;
  }
}
