/// Localised UI strings for the push notification configuration feature.
///
/// Provides translations for the four initial locales: en, es, fr, de.
class PushBrandStringsL10n {
  const PushBrandStringsL10n._({
    required this.pushConfigured,
    required this.pushNotConfigured,
    required this.pushRegistering,
    required this.pushRegistered,
    required this.pushFailed,
  });

  final String pushConfigured;
  final String pushNotConfigured;
  final String pushRegistering;
  final String pushRegistered;
  final String pushFailed;

  /// Return the string set for [locale] (BCP-47 language tag).
  ///
  /// Falls back to English for unsupported locales.
  static PushBrandStringsL10n forLocale(String locale) {
    final tag = locale.split('-').first.split('_').first.toLowerCase();
    return _locales[tag] ?? _en;
  }

  // i18n base: en
  static const _en = PushBrandStringsL10n._(
    pushConfigured: 'Push notifications configured',
    pushNotConfigured: 'Push notifications not configured',
    pushRegistering: 'Registering for push notifications...',
    pushRegistered: 'Successfully registered for push notifications',
    pushFailed: 'Failed to register for push notifications',
  );

  static const _es = PushBrandStringsL10n._(
    pushConfigured: 'Notificaciones push configuradas',
    pushNotConfigured: 'Notificaciones push no configuradas',
    pushRegistering: 'Registrando notificaciones push...',
    pushRegistered: 'Registro exitoso de notificaciones push',
    pushFailed: 'Error al registrar notificaciones push',
  );

  static const _fr = PushBrandStringsL10n._(
    pushConfigured: 'Notifications push configurees',
    pushNotConfigured: 'Notifications push non configurees',
    pushRegistering: 'Enregistrement des notifications push...',
    pushRegistered: 'Notifications push enregistrees avec succes',
    pushFailed: "Echec de l'enregistrement des notifications push",
  );

  static const _de = PushBrandStringsL10n._(
    pushConfigured: 'Push-Benachrichtigungen konfiguriert',
    pushNotConfigured: 'Push-Benachrichtigungen nicht konfiguriert',
    pushRegistering: 'Push-Benachrichtigungen werden registriert...',
    pushRegistered: 'Push-Benachrichtigungen erfolgreich registriert',
    pushFailed: 'Registrierung der Push-Benachrichtigungen fehlgeschlagen',
  );

  static const _locales = <String, PushBrandStringsL10n>{
    'en': _en,
    'es': _es,
    'fr': _fr,
    'de': _de,
  };
}
