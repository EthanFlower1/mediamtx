// i18n base: en
//
// KAI-305 — English base strings for the white-label branding layer.
// These strings live here (instead of the app-wide localization map) so the
// branding layer is self-contained and can be copied by integrators who fork
// the client. A follow-up PR will fold them into the ARB pipeline once the
// router owner wires the branded provider into `main.dart`.
//
// Keep strings short, plain, and safe to surface to end users.
library;

/// English base strings for runtime branding errors and notices.
class BrandingStrings {
  const BrandingStrings._();

  /// Shown when a brand config hex color cannot be parsed.
  static const String invalidHexColor =
      'Invalid brand color value. Using built-in default.';

  /// Shown when the remote brand config cannot be fetched and we fall back
  /// to the built-in default.
  static const String fallbackToDefault =
      'Unable to load tenant branding. Using built-in default.';

  /// Shown when a cached brand config is stale but still served while a
  /// refresh is attempted in the background.
  static const String servingStaleBrand =
      'Showing last-known branding while refreshing.';

  /// Shown when the brand config references a logo URL that fails to load.
  static const String logoLoadFailed =
      'Brand logo could not be loaded.';
}
