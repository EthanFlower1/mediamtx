// KAI-297 — Login-flow localizable strings.
//
// Per seam #8 ("no hardcoded strings") the Flutter app does not yet have
// flutter_intl / ARB wiring, so every user-visible string produced by the
// login flow is routed through this single class. When flutter_intl lands,
// swap the body for `AppLocalizations.of(ctx)` and leave call sites alone.
//
// Mirrors the structure of `DiscoveryStrings` so both flows migrate the same
// way. Tests override individual fields rather than string-matching a frozen
// literal.

/// User-visible strings used by the login flow.
class AuthStrings {
  const AuthStrings({
    required this.loginScreenTitle,
    required this.loginScreenSubtitle,
    required this.localFormHeader,
    required this.localFormEmailLabel,
    required this.localFormEmailHint,
    required this.localFormPasswordLabel,
    required this.localFormPasswordHint,
    required this.localFormSubmit,
    required this.localFormEmailEmpty,
    required this.localFormPasswordEmpty,
    required this.ssoHeader,
    required this.ssoContinueWith,
    required this.ssoCancelled,
    required this.errorWrongCredentials,
    required this.errorNetwork,
    required this.errorUnknownProvider,
    required this.errorRefreshExpired,
    required this.errorServer,
    required this.errorMalformed,
    required this.recoveryCheckPassword,
    required this.recoveryCheckNetwork,
    required this.recoverySignInAgain,
    required this.recoveryRetry,
    required this.webSessionNotPersistentWarning,
    required this.webSessionNotPersistentDismiss,
  });

  final String loginScreenTitle;
  final String loginScreenSubtitle;

  final String localFormHeader;
  final String localFormEmailLabel;
  final String localFormEmailHint;
  final String localFormPasswordLabel;
  final String localFormPasswordHint;
  final String localFormSubmit;
  final String localFormEmailEmpty;
  final String localFormPasswordEmpty;

  final String ssoHeader;
  final String ssoContinueWith;
  final String ssoCancelled;

  final String errorWrongCredentials;
  final String errorNetwork;
  final String errorUnknownProvider;
  final String errorRefreshExpired;
  final String errorServer;
  final String errorMalformed;

  final String recoveryCheckPassword;
  final String recoveryCheckNetwork;
  final String recoverySignInAgain;
  final String recoveryRetry;

  /// KAI-298 security review: shown on web when the secure token store falls
  /// back to an in-memory implementation. Users must be told the session will
  /// not survive a tab refresh.
  final String webSessionNotPersistentWarning;
  final String webSessionNotPersistentDismiss;

  /// Default English strings. Swap via a Riverpod override in tests.
  static const AuthStrings en = AuthStrings(
    loginScreenTitle: 'Sign in',
    loginScreenSubtitle: 'Continue to your directory',
    localFormHeader: 'Sign in with email',
    localFormEmailLabel: 'Email',
    localFormEmailHint: 'you@example.com',
    localFormPasswordLabel: 'Password',
    localFormPasswordHint: 'Enter your password',
    localFormSubmit: 'Sign in',
    localFormEmailEmpty: 'Email is required',
    localFormPasswordEmpty: 'Password is required',
    ssoHeader: 'Or use single sign-on',
    ssoContinueWith: 'Continue with ',
    ssoCancelled: 'Sign-in was cancelled.',
    errorWrongCredentials: 'The email or password is incorrect.',
    errorNetwork: 'Could not reach the server. Check your connection.',
    errorUnknownProvider: 'That identity provider is not available.',
    errorRefreshExpired: 'Your session expired. Please sign in again.',
    errorServer: 'The server returned an error. Try again in a moment.',
    errorMalformed: 'The server returned an unexpected response.',
    recoveryCheckPassword: 'Double-check your email and password.',
    recoveryCheckNetwork: 'Make sure you have internet access.',
    recoverySignInAgain: 'Tap sign in to try again.',
    recoveryRetry: 'Tap retry to try again.',
    webSessionNotPersistentWarning:
        'Your session will not persist if you close or refresh this tab. '
            'Sign in again on return.',
    webSessionNotPersistentDismiss: 'Dismiss for session',
  );
}
