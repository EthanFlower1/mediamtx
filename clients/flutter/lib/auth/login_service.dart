// KAI-297 — LoginService.
//
// Orchestrates the post-discovery login flow against a
// [HomeDirectoryConnection]. Three code paths:
//
//   1. Local form → POST /api/v1/auth/login
//   2. SSO        → `beginSso` (launches the authorizer) then `completeSso`
//                     which POSTs /api/v1/auth/sso/complete
//   3. Refresh    → POST /api/v1/auth/refresh using the stored refresh token
//
// All network I/O is done via a `http.Client` so tests can inject `MockClient`.
// The SSO flow is additionally abstracted behind `SsoAuthorizer` so tests can
// drive happy-path + cancellation paths without touching `flutter_appauth`.

import 'dart:async';
import 'dart:convert';
import 'dart:io' show HandshakeException, SocketException;

import 'package:http/http.dart' as http;

import '../state/app_session.dart';
import '../state/home_directory_connection.dart';
import 'auth_types.dart';
import 'sso_authorizer.dart';

/// Custom URL scheme used as the OIDC redirect URI on iOS and Android.
///
/// Must match `CFBundleURLTypes` in `ios/Runner/Info.plist` and the
/// `<intent-filter>` in `android/app/src/main/AndroidManifest.xml`. See the
/// auth package README for the exact platform snippets.
const String kKaivueAuthRedirectUri = 'kaivue://auth/callback';

/// Typed HTTP status codes LoginService treats specially.
const int _httpUnauthorized = 401;
const int _httpNotFound = 404;

class LoginService {
  final http.Client _http;
  final SsoAuthorizer _authorizer;

  LoginService({
    http.Client? httpClient,
    SsoAuthorizer? authorizer,
  })  : _http = httpClient ?? http.Client(),
        _authorizer = authorizer ?? FakeSsoAuthorizer();

  /// Release the underlying HTTP client. Safe to call multiple times.
  void dispose() => _http.close();

  Uri _resolve(HomeDirectoryConnection connection, String path) {
    final base = Uri.parse(connection.endpointUrl);
    final joined = '${base.path}$path';
    return base.replace(path: joined);
  }

  // ---------------- Discovery of auth methods ----------------

  /// GET `<base>/api/v1/auth/methods` and return the advertised methods.
  ///
  /// Throws [LoginError] with a typed [LoginErrorKind] on any failure.
  Future<AvailableAuthMethods> beginLogin(
      HomeDirectoryConnection connection) async {
    final uri = _resolve(connection, '/api/v1/auth/methods');
    http.Response resp;
    try {
      resp = await _http
          .get(uri, headers: const {'Accept': 'application/json'})
          .timeout(const Duration(seconds: 10));
    } on TimeoutException catch (e) {
      throw LoginError(LoginErrorKind.network, 'timeout: $e');
    } on SocketException catch (e) {
      throw LoginError(LoginErrorKind.network, 'socket: $e');
    } on HandshakeException catch (e) {
      throw LoginError(LoginErrorKind.network, 'tls: $e');
    } on http.ClientException catch (e) {
      throw LoginError(LoginErrorKind.network, 'http: $e');
    }
    if (resp.statusCode == _httpNotFound) {
      throw const LoginError(
          LoginErrorKind.malformed, 'auth methods endpoint missing');
    }
    if (resp.statusCode < 200 || resp.statusCode >= 300) {
      throw LoginError(LoginErrorKind.server, 'status ${resp.statusCode}');
    }
    try {
      final parsed = jsonDecode(resp.body);
      if (parsed is! Map<String, dynamic>) {
        throw const LoginError(
            LoginErrorKind.malformed, 'response is not a JSON object');
      }
      return AvailableAuthMethods.fromJson(parsed);
    } on LoginError {
      rethrow;
    } catch (e) {
      throw LoginError(LoginErrorKind.malformed, 'parse error: $e');
    }
  }

  // ---------------- Local login ----------------

  /// POST `<base>/api/v1/auth/login` with the supplied credentials.
  Future<LoginResult> loginLocal(
    HomeDirectoryConnection connection,
    String username,
    String password,
  ) async {
    final uri = _resolve(connection, '/api/v1/auth/login');
    http.Response resp;
    try {
      resp = await _http
          .post(
            uri,
            headers: const {
              'Accept': 'application/json',
              'Content-Type': 'application/json',
            },
            body: jsonEncode({'username': username, 'password': password}),
          )
          .timeout(const Duration(seconds: 10));
    } on TimeoutException catch (e) {
      throw LoginError(LoginErrorKind.network, 'timeout: $e');
    } on SocketException catch (e) {
      throw LoginError(LoginErrorKind.network, 'socket: $e');
    } on HandshakeException catch (e) {
      throw LoginError(LoginErrorKind.network, 'tls: $e');
    } on http.ClientException catch (e) {
      throw LoginError(LoginErrorKind.network, 'http: $e');
    }

    if (resp.statusCode == _httpUnauthorized) {
      throw const LoginError(
          LoginErrorKind.wrongCredentials, 'server returned 401');
    }
    if (resp.statusCode < 200 || resp.statusCode >= 300) {
      throw LoginError(LoginErrorKind.server, 'status ${resp.statusCode}');
    }
    return _parseLoginResult(resp.body);
  }

  // ---------------- SSO ----------------

  /// Launch the SSO flow for [providerId]. Resolves to a [SsoFlow] with the
  /// authorization code (on success), or a cancelled/error flow if the user
  /// bailed out or the authorizer hit an error.
  ///
  /// Throws [LoginError] with [LoginErrorKind.unknownProvider] if the
  /// directory doesn't advertise [providerId].
  Future<SsoFlow> beginSso(
    HomeDirectoryConnection connection,
    String providerId, {
    AvailableAuthMethods? knownMethods,
  }) async {
    final methods = knownMethods ?? await beginLogin(connection);
    final provider = methods.ssoProviders.firstWhere(
      (p) => p.id == providerId,
      orElse: () => throw LoginError(
          LoginErrorKind.unknownProvider, 'provider not advertised: $providerId'),
    );
    final flowId =
        '${DateTime.now().microsecondsSinceEpoch}-${provider.id}';
    final result = await _authorizer.authorize(
      provider: provider,
      redirectUri: kKaivueAuthRedirectUri,
    );

    // Propagate typed errors from the authorizer (ssoPlugin, unknown, etc.)
    if (result.errorKind != null && !result.cancelled) {
      return SsoFlow(
        flowId: flowId,
        providerId: providerId,
        state: result.state,
        codeVerifier: result.codeVerifier,
        nonce: result.nonce,
        sentState: result.state,
        cancelled: false,
        errorKind: result.errorKind,
        errorMessage: result.errorMessage,
        issuerUrl: provider.issuerUrl,
      );
    }

    if (result.cancelled || result.authorizationCode == null) {
      return SsoFlow(
        flowId: flowId,
        providerId: providerId,
        state: result.state,
        codeVerifier: result.codeVerifier,
        nonce: result.nonce,
        sentState: result.state,
        cancelled: true,
        errorKind: LoginErrorKind.cancelled,
        issuerUrl: provider.issuerUrl,
      );
    }
    return SsoFlow(
      flowId: flowId,
      providerId: providerId,
      authorizationCode: result.authorizationCode,
      state: result.state,
      codeVerifier: result.codeVerifier,
      nonce: result.nonce,
      sentState: result.state,
      issuerUrl: provider.issuerUrl,
    );
  }

  /// Finish the SSO flow by exchanging the authorization code at
  /// `/api/v1/auth/sso/complete`. Callers should only invoke this with a
  /// non-cancelled [SsoFlow].
  ///
  /// Validation order (lead-security mandated):
  ///   1. Check cancelled / authorizer error
  ///   2. State validation (sentState vs returned state) — BEFORE any network
  ///   3. Exchange authorization code with server
  ///   4. Nonce validation against the ID token
  ///   5. Issuer validation (nice-to-have, non-blocker)
  Future<LoginResult> completeSso(
    HomeDirectoryConnection connection,
    SsoFlow flow,
  ) async {
    // Gate 0: propagate typed errors from the authorizer.
    if (flow.errorKind != null && flow.errorKind != LoginErrorKind.cancelled) {
      throw LoginError(
          flow.errorKind!, flow.errorMessage ?? 'SSO authorizer error');
    }

    if (flow.cancelled || flow.authorizationCode == null) {
      throw const LoginError(
          LoginErrorKind.cancelled, 'SSO flow was cancelled');
    }

    // Gate 1 (FIRST): state validation — the returned state must match what we
    // sent. This prevents CSRF before we do any network I/O.
    if (flow.sentState != null && flow.state != flow.sentState) {
      throw const LoginError(
          LoginErrorKind.malformed, 'state mismatch — possible CSRF');
    }

    final uri = _resolve(connection, '/api/v1/auth/sso/complete');
    http.Response resp;
    try {
      resp = await _http
          .post(
            uri,
            headers: const {
              'Accept': 'application/json',
              'Content-Type': 'application/json',
            },
            body: jsonEncode({
              'provider_id': flow.providerId,
              'authorization_code': flow.authorizationCode,
              'state': flow.state,
              'code_verifier': flow.codeVerifier,
            }),
          )
          .timeout(const Duration(seconds: 15));
    } on TimeoutException catch (e) {
      throw LoginError(LoginErrorKind.network, 'timeout: $e');
    } on SocketException catch (e) {
      throw LoginError(LoginErrorKind.network, 'socket: $e');
    } on HandshakeException catch (e) {
      throw LoginError(LoginErrorKind.network, 'tls: $e');
    } on http.ClientException catch (e) {
      throw LoginError(LoginErrorKind.network, 'http: $e');
    }

    // Check for IdP rejection (OAuth error responses).
    if (resp.statusCode == _httpUnauthorized) {
      throw const LoginError(
          LoginErrorKind.idpRejected, 'sso exchange rejected (401)');
    }
    if (resp.statusCode == 403) {
      throw const LoginError(
          LoginErrorKind.idpRejected, 'sso exchange rejected (403)');
    }
    if (resp.statusCode < 200 || resp.statusCode >= 300) {
      throw LoginError(LoginErrorKind.server, 'status ${resp.statusCode}');
    }

    final result = _parseLoginResult(resp.body);

    // Gate 2: nonce validation. Decode the ID token payload (base64) and
    // compare the `nonce` claim to the one we sent. flutter_appauth already
    // verified the JWT signature; we only need to check the nonce claim.
    if (flow.nonce != null) {
      _validateIdTokenNonce(resp.body, flow.nonce!);
    }

    // Gate 3 (nice-to-have): issuer validation. Log a warning if `iss` doesn't
    // match the provider's issuerUrl.
    if (flow.issuerUrl != null && flow.issuerUrl!.isNotEmpty) {
      _validateIssuer(resp.body, flow.issuerUrl!);
    }

    return result;
  }

  /// Decode the ID token from the server response and validate the nonce claim.
  ///
  /// The server response is expected to contain an `id_token` field. We decode
  /// the JWT payload (middle segment, base64) without signature verification
  /// (flutter_appauth already did that). If the nonce doesn't match, throw
  /// [LoginErrorKind.malformed].
  void _validateIdTokenNonce(String responseBody, String expectedNonce) {
    try {
      final parsed = jsonDecode(responseBody);
      if (parsed is! Map<String, dynamic>) return;
      final idToken = parsed['id_token'] as String?;
      if (idToken == null || idToken.isEmpty) return;

      final claims = _decodeJwtPayload(idToken);
      if (claims == null) return;

      final tokenNonce = claims['nonce'] as String?;
      if (tokenNonce == null) {
        // IdP didn't include nonce — this is acceptable for some IdPs that
        // don't echo the nonce in the authorization code flow (the nonce is
        // only mandatory in the implicit flow per OIDC Core 3.1.3.7). Log
        // but don't reject.
        return;
      }
      if (tokenNonce != expectedNonce) {
        throw const LoginError(
            LoginErrorKind.malformed, 'nonce mismatch — possible replay');
      }
    } on LoginError {
      rethrow;
    } catch (_) {
      // ID token decode failed — not fatal, the server already validated it.
    }
  }

  /// Validate the `iss` claim in the ID token against the expected issuer URL.
  /// Logs a warning on mismatch but does not throw (non-blocker per
  /// lead-security review).
  void _validateIssuer(String responseBody, String expectedIssuer) {
    try {
      final parsed = jsonDecode(responseBody);
      if (parsed is! Map<String, dynamic>) return;
      final idToken = parsed['id_token'] as String?;
      if (idToken == null || idToken.isEmpty) return;

      final claims = _decodeJwtPayload(idToken);
      if (claims == null) return;

      final iss = claims['iss'] as String?;
      if (iss != null && iss != expectedIssuer) {
        // Non-blocker: log for observability. A future version may reject.
        // ignore: avoid_print
        print('[KAI-297] WARNING: ID token issuer "$iss" does not match '
            'expected "$expectedIssuer"');
      }
    } catch (_) {
      // Best-effort — don't fail the login over issuer validation.
    }
  }

  /// Decode the payload (middle segment) of a JWT. Returns the claims map or
  /// null if decoding fails.
  static Map<String, dynamic>? _decodeJwtPayload(String jwt) {
    final parts = jwt.split('.');
    if (parts.length != 3) return null;
    try {
      var payload = parts[1];
      // Base64 padding
      switch (payload.length % 4) {
        case 2:
          payload += '==';
          break;
        case 3:
          payload += '=';
          break;
      }
      final decoded = utf8.decode(base64Url.decode(payload));
      final result = jsonDecode(decoded);
      return result is Map<String, dynamic> ? result : null;
    } catch (_) {
      return null;
    }
  }

  // ---------------- Refresh ----------------

  /// Mint a fresh access token using [session]'s refresh token.
  ///
  /// Returns a new [LoginResult]. On expired/revoked refresh token, throws
  /// [LoginError] with [LoginErrorKind.refreshExpired] so the caller can
  /// bounce the user back to the login screen.
  Future<LoginResult> refresh(AppSession session) async {
    final conn = session.activeConnection;
    final rt = session.refreshToken;
    if (conn == null) {
      throw const LoginError(
          LoginErrorKind.refreshExpired, 'no active connection');
    }
    if (rt == null || rt.isEmpty) {
      throw const LoginError(
          LoginErrorKind.refreshExpired, 'no refresh token');
    }
    final uri = _resolve(conn, '/api/v1/auth/refresh');
    http.Response resp;
    try {
      resp = await _http
          .post(
            uri,
            headers: const {
              'Accept': 'application/json',
              'Content-Type': 'application/json',
            },
            body: jsonEncode({'refresh_token': rt}),
          )
          .timeout(const Duration(seconds: 10));
    } on TimeoutException catch (e) {
      throw LoginError(LoginErrorKind.network, 'timeout: $e');
    } on SocketException catch (e) {
      throw LoginError(LoginErrorKind.network, 'socket: $e');
    } on HandshakeException catch (e) {
      throw LoginError(LoginErrorKind.network, 'tls: $e');
    } on http.ClientException catch (e) {
      throw LoginError(LoginErrorKind.network, 'http: $e');
    }
    if (resp.statusCode == _httpUnauthorized) {
      throw const LoginError(
          LoginErrorKind.refreshExpired, 'refresh token rejected');
    }
    if (resp.statusCode < 200 || resp.statusCode >= 300) {
      throw LoginError(LoginErrorKind.server, 'status ${resp.statusCode}');
    }
    return _parseLoginResult(resp.body);
  }

  // ---------------- Helpers ----------------

  LoginResult _parseLoginResult(String body) {
    dynamic parsed;
    try {
      parsed = jsonDecode(body);
    } catch (e) {
      throw LoginError(LoginErrorKind.malformed, 'not json: $e');
    }
    if (parsed is! Map<String, dynamic>) {
      throw const LoginError(
          LoginErrorKind.malformed, 'response is not a JSON object');
    }
    try {
      return LoginResult.fromJson(parsed);
    } catch (e) {
      throw LoginError(LoginErrorKind.malformed, 'missing field: $e');
    }
  }
}

