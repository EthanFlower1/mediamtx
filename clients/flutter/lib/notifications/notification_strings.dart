// KAI-303 — Localizable strings for the push notifications flow.
//
// Follows the same pattern as AuthStrings (KAI-297) and DiscoveryStrings:
// a single class exposing every user-visible string, with an English default.
// When flutter_intl lands, swap the body for `AppLocalizations.of(ctx)` and
// leave call sites alone.
//
// Tests override individual fields rather than string-matching literals.

/// User-visible strings used by the push notifications flow.
class NotificationStrings {
  const NotificationStrings({
    required this.permissionPromptTitle,
    required this.permissionPromptBody,
    required this.permissionGrantedBanner,
    required this.permissionDeniedBanner,
    required this.permissionProvisionalBanner,
    required this.permissionUnknownBanner,
    required this.permissionOpenSettings,
    required this.fallbackNotificationTitle,
    required this.fallbackNotificationBody,
    required this.eventKindMotion,
    required this.eventKindFace,
    required this.eventKindLpr,
    required this.eventKindManual,
    required this.eventKindSystem,
    required this.subscribeCameraLabel,
    required this.unsubscribeCameraLabel,
    required this.errorRegisterFailed,
    required this.errorSubscribeFailed,
    required this.errorPlatformUnsupported,
    required this.desktopStubWarning,
    required this.firebaseNotInitialisedWarning,
    required this.crossTenantPushBanner,
  });

  final String permissionPromptTitle;
  final String permissionPromptBody;
  final String permissionGrantedBanner;
  final String permissionDeniedBanner;
  final String permissionProvisionalBanner;
  final String permissionUnknownBanner;
  final String permissionOpenSettings;

  /// Title used when a payload omits a human-readable title. Kept generic so
  /// no PII leaks into the notification shade.
  final String fallbackNotificationTitle;

  /// Body used when a payload omits a human-readable body. Kept generic so
  /// no PII leaks into the notification shade — the metadata-only payload
  /// contract forbids putting camera names or face identities in the body.
  final String fallbackNotificationBody;

  final String eventKindMotion;
  final String eventKindFace;
  final String eventKindLpr;
  final String eventKindManual;
  final String eventKindSystem;

  final String subscribeCameraLabel;
  final String unsubscribeCameraLabel;

  final String errorRegisterFailed;
  final String errorSubscribeFailed;
  final String errorPlatformUnsupported;

  final String desktopStubWarning;
  final String firebaseNotInitialisedWarning;

  /// KAI-303: shown as a user-visible banner when a push message's
  /// tenantId does not match the active AppSession. i18n base: en.
  final String crossTenantPushBanner;

  /// KAI-303: returns the localized notification title for a given event
  /// kind string (as returned by Directory /api/v1/events/<id>).
  ///
  /// i18n base: en. The sweep agent should move these into the project's
  /// .arb file when the localisation pipeline lands.
  String titleForKind(String kind) {
    switch (kind) {
      case 'motion':
        return eventKindMotion;
      case 'face':
        return eventKindFace;
      case 'lpr':
        return eventKindLpr;
      case 'manual':
        return eventKindManual;
      case 'system':
        return eventKindSystem;
      default:
        return fallbackNotificationTitle;
    }
  }

  /// Default English strings. Override via a Riverpod provider in tests or
  /// when the localisation layer lands.
  static const NotificationStrings en = NotificationStrings(
    permissionPromptTitle: 'Enable notifications',
    permissionPromptBody:
        'Get alerted when your cameras detect motion, faces, or license plates.',
    permissionGrantedBanner: 'Notifications are on.',
    permissionDeniedBanner:
        'Notifications are off. You can enable them in Settings.',
    permissionProvisionalBanner:
        'Notifications will arrive quietly until you confirm them.',
    permissionUnknownBanner:
        'Notification permission could not be determined on this device.',
    permissionOpenSettings: 'Open settings',
    fallbackNotificationTitle: 'New event',
    fallbackNotificationBody: 'Tap to open the event.',
    eventKindMotion: 'Motion detected',
    eventKindFace: 'Face detected',
    eventKindLpr: 'License plate detected',
    eventKindManual: 'Manual alert',
    eventKindSystem: 'System notice',
    subscribeCameraLabel: 'Notify me for this camera',
    unsubscribeCameraLabel: 'Stop notifying me for this camera',
    errorRegisterFailed: 'Could not register this device for notifications.',
    errorSubscribeFailed: 'Could not update your notification preferences.',
    errorPlatformUnsupported:
        'Push notifications are not supported on this platform yet.',
    desktopStubWarning:
        'Desktop push notifications are a stub in this build. Real native '
        'integration is tracked as a follow-up.',
    firebaseNotInitialisedWarning:
        'Firebase is not initialised — push notifications will be inert. '
        'Land the google-services configuration to enable delivery.',
    crossTenantPushBanner:
        'A notification arrived for a different account and was ignored.',
  );
}
