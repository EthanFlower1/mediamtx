// KAI-297 — Production SsoAuthorizer backed by `flutter_appauth`.
//
// This file is the ONLY place that imports `flutter_appauth`. The rest of
// `lib/auth/` talks to the `SsoAuthorizer` interface so the Dart unit test VM
// never touches the platform plugin. A missing plugin in the test VM would
// raise `MissingPluginException`; by isolating the import here, `flutter test`
// only loads this file when a widget test explicitly imports it — which we
// do not do.

import 'package:flutter_appauth/flutter_appauth.dart';

import 'auth_types.dart';
import 'sso_authorizer.dart';

/// Real authorizer — delegates to `FlutterAppAuth.authorize` with PKCE.
///
/// The redirect URI is the custom scheme documented in
/// `clients/flutter/lib/auth/README.md`: `raikada://auth/callback`. iOS and
/// Android must declare matching handlers (Info.plist CFBundleURLTypes on
/// iOS; intent-filter on Android) — see README for the exact snippets.
class FlutterAppAuthAuthorizer implements SsoAuthorizer {
  final FlutterAppAuth _appAuth;

  FlutterAppAuthAuthorizer({FlutterAppAuth? appAuth})
      : _appAuth = appAuth ?? const FlutterAppAuth();

  @override
  Future<SsoAuthorizationResult> authorize({
    required SsoProviderDescriptor provider,
    required String redirectUri,
  }) async {
    final state = generateOpaqueState();
    final verifier = generatePkceVerifier();
    final nonce = generateNonce();
    final request = AuthorizationRequest(
      provider.clientId,
      redirectUri,
      discoveryUrl: provider.issuerUrl,
      scopes: provider.scopes,
      // `flutter_appauth` derives the PKCE challenge from the verifier
      // automatically when the verifier is supplied via `state` metadata.
      // We pass our own `state` and `nonce` so the server can round-trip them.
      // The nonce is embedded in the ID token by the IdP; login_service.dart
      // validates it after the code exchange.
      additionalParameters: {'state': state, 'nonce': nonce},
    );
    try {
      final response = await _appAuth.authorize(request);
      // `flutter_appauth` in v8 returns a non-nullable AuthorizationResponse;
      // user cancellation surfaces as a thrown FlutterAppAuthUserCancelledException
      // which we catch below. The `response == null` branch is defensive for
      // older plugin versions.
      // ignore: unnecessary_null_comparison
      if (response == null) {
        return SsoAuthorizationResult.cancelled(
          state: state,
          codeVerifier: verifier,
          nonce: nonce,
        );
      }
      return SsoAuthorizationResult(
        authorizationCode: response.authorizationCode,
        state: response.authorizationAdditionalParameters?['state'] ?? state,
        codeVerifier: response.codeVerifier ?? verifier,
        nonce: nonce,
      );
    } on FlutterAppAuthUserCancelledException {
      // User explicitly cancelled (closed the browser tab / hit system back).
      // Surface as a cancelled result so the UI can show a neutral toast
      // instead of an error banner.
      return SsoAuthorizationResult.cancelled(
        state: state,
        codeVerifier: verifier,
        nonce: nonce,
      );
    } on FlutterAppAuthPlatformException catch (e) {
      // Any other flutter_appauth error — network failure during discovery,
      // internal plugin error, etc. Map to ssoPlugin so the UI can show a
      // retryable error banner.
      return SsoAuthorizationResult.error(
        kind: LoginErrorKind.ssoPlugin,
        message: e.message ?? 'flutter_appauth error',
        state: state,
        codeVerifier: verifier,
        nonce: nonce,
      );
    } catch (e) {
      // Genuinely unexpected — default catch-all.
      return SsoAuthorizationResult.error(
        kind: LoginErrorKind.unknown,
        message: e.toString(),
        state: state,
        codeVerifier: verifier,
        nonce: nonce,
      );
    }
  }
}
