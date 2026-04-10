/// Localised UI strings for content-override related screens.
///
/// Four locales shipped inline (en, es, fr, de).
/// Proto-first seam: these strings themselves can be overridden at
/// runtime via [ContentOverrideResolver.resolveString].
// i18n base: en
library;

/// Lightweight l10n container for content-override UI strings.
class ContentOverrideStringsL10n {
  final String contentLoading;
  final String contentLoadFailed;
  final String usingDefaults;
  final String brandCustomized;

  const ContentOverrideStringsL10n._({
    required this.contentLoading,
    required this.contentLoadFailed,
    required this.usingDefaults,
    required this.brandCustomized,
  });

  // ── Built-in locale tables ──────────────────────────────────────────

  static const _en = ContentOverrideStringsL10n._(
    contentLoading: 'Loading brand content...',
    contentLoadFailed: 'Failed to load brand content.',
    usingDefaults: 'Using default branding.',
    brandCustomized: 'Brand content applied.',
  );

  static const _es = ContentOverrideStringsL10n._(
    contentLoading: 'Cargando contenido de marca...',
    contentLoadFailed: 'Error al cargar contenido de marca.',
    usingDefaults: 'Usando marca predeterminada.',
    brandCustomized: 'Contenido de marca aplicado.',
  );

  static const _fr = ContentOverrideStringsL10n._(
    contentLoading: 'Chargement du contenu de marque...',
    contentLoadFailed: 'Echec du chargement du contenu de marque.',
    usingDefaults: 'Utilisation de la marque par defaut.',
    brandCustomized: 'Contenu de marque applique.',
  );

  static const _de = ContentOverrideStringsL10n._(
    contentLoading: 'Markeninhalte werden geladen...',
    contentLoadFailed: 'Markeninhalte konnten nicht geladen werden.',
    usingDefaults: 'Standard-Branding wird verwendet.',
    brandCustomized: 'Markeninhalte angewendet.',
  );

  /// Look up the string table for [locale].
  ///
  /// Falls back to English for unknown locales.
  // i18n base: en
  static ContentOverrideStringsL10n forLocale(String locale) {
    switch (locale) {
      case 'en':
        return _en;
      case 'es':
        return _es;
      case 'fr':
        return _fr;
      case 'de':
        return _de;
      default:
        return _en;
    }
  }
}
