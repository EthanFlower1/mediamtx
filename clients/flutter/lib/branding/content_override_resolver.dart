/// Resolves runtime content overrides with locale fall-back logic.
///
/// Proto-first seam: the resolver sits between the UI layer and the
/// [ContentOverride] value object so swapping the data source (e.g. to
/// a gRPC BrandingService) requires zero UI changes.
library;

import 'content_override.dart';

/// Stateless resolver that applies fall-back rules to a [ContentOverride].
class ContentOverrideResolver {
  final ContentOverride _override;

  const ContentOverrideResolver(this._override);

  /// Convenience constructor for no-override scenarios.
  static const empty = ContentOverrideResolver(ContentOverride.empty);

  // ── String resolution ───────────────────────────────────────────────

  /// Resolve a localised string override.
  ///
  /// Look-up order:
  ///  1. `stringOverrides[locale][key]`
  ///  2. `stringOverrides['en'][key]`  (English fall-back)
  ///  3. [fallback]                     (compile-time default)
  String resolveString(
    String key,
    String locale, {
    required String fallback,
  }) {
    // Try exact locale first.
    final localeMap = _override.stringOverrides[locale];
    if (localeMap != null && localeMap.containsKey(key)) {
      return localeMap[key]!;
    }

    // Fall back to English.
    if (locale != 'en') {
      final enMap = _override.stringOverrides['en'];
      if (enMap != null && enMap.containsKey(key)) {
        return enMap[key]!;
      }
    }

    return fallback;
  }

  // ── Asset resolution ────────────────────────────────────────────────

  /// Resolve an asset URL by logical key.
  ///
  /// Recognised keys: `logo`, `splash`, `appIcon`.
  /// Returns `null` when no override is set for [assetKey].
  String? resolveAsset(String assetKey) {
    switch (assetKey) {
      case 'logo':
        return _override.logoUrl;
      case 'splash':
        return _override.splashUrl;
      case 'appIcon':
        return _override.appIconUrl;
      default:
        return null;
    }
  }

  // ── Legal URL resolution ────────────────────────────────────────────

  /// Resolve a legal/support URL by logical key.
  ///
  /// Recognised keys: `terms`, `privacy`, `supportEmail`.
  /// Returns `null` when no override is set for [key].
  String? resolveLegalUrl(String key) {
    switch (key) {
      case 'terms':
        return _override.termsUrl;
      case 'privacy':
        return _override.privacyUrl;
      case 'supportEmail':
        return _override.supportEmail;
      default:
        return null;
    }
  }
}
