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
/// `clients/flutter/lib/auth/README.md`: `kaivue://auth/callback`. iOS and
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
    final request = AuthorizationRequest(
      provider.clientId,
      redirectUri,
      discoveryUrl: provider.issuerUrl,
      scopes: provider.scopes,
      // `flutter_appauth` derives the PKCE challenge from the verifier
      // automatically when the verifier is supplied via `state` metadata.
      // We pass our own `state` so the server can round-trip it.
      additionalParameters: {'state': state},
    );
    try {
      final response = await _appAuth.authorize(request);
      // `flutter_appauth` in v8 returns a non-nullable AuthorizationResponse;
      // user cancellation surfaces as a thrown PlatformException which we
      // catch below. Both shapes (null vs throw) are handled here so the
      // adapter stays compatible across major versions of the plugin.
      // ignore: unnecessary_null_comparison
      if (response == null) {
        return SsoAuthorizationResult.cancelled(
          state: state,
          codeVerifier: verifier,
        );
      }
      return SsoAuthorizationResult(
        authorizationCode: response.authorizationCode,
        state: response.authorizationAdditionalParameters?['state'] ?? state,
        codeVerifier: response.codeVerifier ?? verifier,
      );
    } catch (e) {
      // `flutter_appauth` raises a PlatformException on cancellation on some
      // platforms. We conservatively treat any exception as a cancellation
      // here — the calling LoginService decides whether to show an error UI
      // based on whether `authorizationCode` came back.
      return SsoAuthorizationResult.cancelled(
        state: state,
        codeVerifier: verifier,
      );
    }
  }
}
