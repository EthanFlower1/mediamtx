// KAI-296 — Discovery-flow localizable strings.
//
// The Flutter app does not yet have flutter_intl / ARB wiring. Per seam #8
// ("no hardcoded strings"), all user-visible text produced by the discovery
// flow is routed through this single class so that (a) it can be swapped for a
// real `AppLocalizations` implementation later without touching call sites,
// and (b) tests can override individual strings without string-matching a
// frozen literal.
//
// When flutter_intl lands, the intended migration is:
//   1. Add the same keys to `app_en.arb` / `app_xx.arb`.
//   2. Make `DiscoveryStrings` a thin wrapper around `AppLocalizations.of(ctx)`.
//   3. Keep the call sites exactly as-is.

/// User-visible strings used by the discovery flow.
///
/// Instances are injected via [DiscoveryStrings.en] (default) or supplied by a
/// test. Every widget and error message MUST read from here — never inline a
/// literal.
class DiscoveryStrings {
  const DiscoveryStrings({
    required this.pickerTitle,
    required this.pickerMethodManual,
    required this.pickerMethodMdns,
    required this.pickerMethodQr,
    required this.manualUrlLabel,
    required this.manualUrlHint,
    required this.manualUrlInvalid,
    required this.manualUrlEmpty,
    required this.manualConnectButton,
    required this.mdnsSearching,
    required this.mdnsEmpty,
    required this.mdnsUnsupported,
    required this.qrPermissionDenied,
    required this.qrInstructions,
    required this.qrPayloadInvalid,
    required this.errorUnreachable,
    required this.errorTlsMismatch,
    required this.errorNotRaikada,
    required this.errorVersionMismatch,
    required this.errorTimeout,
    required this.errorMalformedResponse,
  });

  final String pickerTitle;
  final String pickerMethodManual;
  final String pickerMethodMdns;
  final String pickerMethodQr;

  final String manualUrlLabel;
  final String manualUrlHint;
  final String manualUrlInvalid;
  final String manualUrlEmpty;
  final String manualConnectButton;

  final String mdnsSearching;
  final String mdnsEmpty;
  final String mdnsUnsupported;

  final String qrPermissionDenied;
  final String qrInstructions;
  final String qrPayloadInvalid;

  final String errorUnreachable;
  final String errorTlsMismatch;
  final String errorNotRaikada;
  final String errorVersionMismatch;
  final String errorTimeout;
  final String errorMalformedResponse;

  /// Default English strings. Swap via a Riverpod override in tests.
  static const DiscoveryStrings en = DiscoveryStrings(
    pickerTitle: 'Connect to a Raikada directory',
    pickerMethodManual: 'Enter server address',
    pickerMethodMdns: 'Find on local network',
    pickerMethodQr: 'Scan invite QR code',
    manualUrlLabel: 'Server URL',
    manualUrlHint: 'https://nvr.example.local',
    manualUrlInvalid: 'Enter a valid https:// or http:// URL',
    manualUrlEmpty: 'URL is required',
    manualConnectButton: 'Connect',
    mdnsSearching: 'Searching for directories on this network...',
    mdnsEmpty: 'No Raikada directories found on this network',
    mdnsUnsupported:
        'mDNS browsing is not available on this platform. Try entering the server URL manually.',
    qrPermissionDenied:
        'Camera permission is required to scan invite codes.',
    qrInstructions: 'Point your camera at the invite QR code.',
    qrPayloadInvalid: 'This QR code is not a valid Raikada invite.',
    errorUnreachable: 'Could not reach the server. Check the URL and network.',
    errorTlsMismatch: 'TLS certificate does not match the expected fingerprint.',
    errorNotRaikada: 'This address is not a Raikada directory.',
    errorVersionMismatch:
        'This directory runs an unsupported version. Update the app or the server.',
    errorTimeout: 'The server did not respond in time.',
    errorMalformedResponse: 'The server returned an unexpected response.',
  );
}
