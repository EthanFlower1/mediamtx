// KAI-297 — SSO authorizer seam.
//
// `flutter_appauth` is a platform plugin; importing it directly from the
// Dart test VM throws `MissingPluginException`. We insulate the login service
// from it by defining a thin interface here and keeping the production
// implementation in a separate file (`flutter_appauth_authorizer.dart`) that
// lives behind `kIsWeb == false` guards.
//
// Tests inject `FakeSsoAuthorizer` via the `loginServiceProvider` override —
// no platform plugin is ever touched.

import 'dart:math';

import 'auth_types.dart';

/// Result of an authorization call.
class SsoAuthorizationResult {
  /// Authorization code returned by the IdP. Null iff [cancelled] is true.
  final String? authorizationCode;
  final String state;
  final String codeVerifier;
  final bool cancelled;

  const SsoAuthorizationResult({
    required this.state,
    required this.codeVerifier,
    this.authorizationCode,
    this.cancelled = false,
  });

  const SsoAuthorizationResult.cancelled({
    required this.state,
    required this.codeVerifier,
  })  : authorizationCode = null,
        cancelled = true;
}

/// Pluggable interface. Production wires this to `flutter_appauth`; tests
/// inject a fake.
abstract class SsoAuthorizer {
  /// Launch the system browser / custom tab against [provider] and return the
  /// authorization result. The implementation is responsible for generating
  /// the PKCE verifier + state, supplying the redirect URI, and handling
  /// user cancellation.
  Future<SsoAuthorizationResult> authorize({
    required SsoProviderDescriptor provider,
    required String redirectUri,
  });
}

/// Generate a random PKCE-friendly string. RFC 7636 requires 43-128 chars from
/// `[A-Z][a-z][0-9]-._~`.
String generatePkceVerifier([Random? rng]) {
  final r = rng ?? Random.secure();
  const charset =
      'ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopqrstuvwxyz0123456789-._~';
  final buf = StringBuffer();
  for (var i = 0; i < 64; i++) {
    buf.write(charset[r.nextInt(charset.length)]);
  }
  return buf.toString();
}

/// Generate a random opaque `state` value round-tripped through the IdP.
String generateOpaqueState([Random? rng]) {
  final r = rng ?? Random.secure();
  const charset =
      'ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopqrstuvwxyz0123456789';
  final buf = StringBuffer();
  for (var i = 0; i < 32; i++) {
    buf.write(charset[r.nextInt(charset.length)]);
  }
  return buf.toString();
}

/// In-test fake. Scripted authorize results so tests can drive happy-path and
/// cancellation paths deterministically without touching platform channels.
class FakeSsoAuthorizer implements SsoAuthorizer {
  /// If set, returns a cancelled result. Otherwise returns a successful
  /// result with [scriptedCode] as the authorization code.
  bool shouldCancel = false;
  String scriptedCode = 'fake-auth-code';
  String scriptedState = 'fake-state';
  String scriptedVerifier = 'fake-verifier';

  /// Records every `authorize` call so tests can assert the right provider
  /// was launched.
  final List<SsoProviderDescriptor> calls = [];

  @override
  Future<SsoAuthorizationResult> authorize({
    required SsoProviderDescriptor provider,
    required String redirectUri,
  }) async {
    calls.add(provider);
    if (shouldCancel) {
      return SsoAuthorizationResult.cancelled(
        state: scriptedState,
        codeVerifier: scriptedVerifier,
      );
    }
    return SsoAuthorizationResult(
      authorizationCode: scriptedCode,
      state: scriptedState,
      codeVerifier: scriptedVerifier,
    );
  }
}
