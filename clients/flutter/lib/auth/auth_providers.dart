// KAI-297 — Riverpod wiring for the login flow.
//
// Hand-written providers in the same style as `discovery_providers.dart`. The
// goal is to expose every seam as an overridable Provider so tests build a
// `ProviderContainer(overrides: [...])` and never reach into platform plugins.
//
// Provider list:
//   * [authStringsProvider]   — i18n strings (swap in tests)
//   * [ssoAuthorizerProvider] — SSO authorizer; defaults to FakeSsoAuthorizer
//                               so the test VM never loads flutter_appauth
//   * [authHttpClientProvider]— shared `http.Client` for the login service
//   * [loginServiceProvider]  — singleton [LoginService]
//   * [authMethodsProvider]   — `FutureProvider.family` keyed on connection ID;
//                               fetches /api/v1/auth/methods
//   * [loginStateProvider]    — `StateNotifierProvider` that tracks the in-
//                               flight login attempt for the active connection
//
// The state notifier deliberately does NOT itself touch `appSessionProvider`.
// On a successful login it surfaces a `LoginPhase.success(LoginResult)` to the
// UI, and the UI hands the result + connection to `AppSessionNotifier`. This
// keeps the auth layer free of cyclic provider dependencies and matches the
// pattern KAI-296 used for `DiscoveryResultsController`.

import 'package:flutter_riverpod/flutter_riverpod.dart';
import 'package:http/http.dart' as http;

import '../state/home_directory_connection.dart';
import 'auth_strings.dart';
import 'auth_types.dart';
import 'login_service.dart';
import 'sso_authorizer.dart';

/// User-visible strings. Override in widget tests with
/// `authStringsProvider.overrideWithValue(testStrings)`.
final authStringsProvider = Provider<AuthStrings>((ref) => AuthStrings.en);

/// SSO authorizer seam. Defaults to a fake so unit / widget tests never pull
/// `flutter_appauth` into the Dart test VM. Production wires this to a
/// `FlutterAppAuthAuthorizer` via override in `main.dart`.
final ssoAuthorizerProvider = Provider<SsoAuthorizer>(
  (ref) => FakeSsoAuthorizer(),
);

/// Shared HTTP client used by the login service. Disposed when no listener
/// remains so we don't leak sockets across hot-restarts.
final authHttpClientProvider = Provider<http.Client>((ref) {
  final client = http.Client();
  ref.onDispose(client.close);
  return client;
});

/// Singleton login service. Reads the HTTP client + authorizer from their
/// respective providers so tests can swap either independently.
final loginServiceProvider = Provider<LoginService>((ref) {
  return LoginService(
    httpClient: ref.watch(authHttpClientProvider),
    authorizer: ref.watch(ssoAuthorizerProvider),
  );
});

/// Fetches `/api/v1/auth/methods` for a given connection. Family-scoped so
/// switching directories triggers a re-fetch instead of leaking results.
///
/// Returns `AvailableAuthMethods`; throws `LoginError` on failure (Riverpod
/// surfaces it as `AsyncError` so the UI can show a `LoginErrorBanner`).
final authMethodsProvider = FutureProvider.autoDispose
    .family<AvailableAuthMethods, HomeDirectoryConnection>((ref, conn) async {
  final svc = ref.watch(loginServiceProvider);
  return svc.beginLogin(conn);
});

// -------------------- LoginPhase + LoginStateNotifier --------------------

/// Phases the in-flight login attempt can be in. Mirrors how
/// `DiscoveryResultsState` carries the union via discrete fields rather than
/// a sealed class — easier to consume in widgets without pattern matching.
enum LoginPhase {
  idle,
  submitting,
  awaitingSso,
  success,
  cancelled,
  error,
}

/// Snapshot of the current login attempt for the active connection.
class LoginState {
  final LoginPhase phase;
  final LoginResult? result;
  final LoginError? error;

  /// In-flight SSO state, used to thread the authorization code from
  /// `beginSso` to `completeSso` without exposing it to widgets.
  final SsoFlow? pendingSso;

  const LoginState({
    required this.phase,
    this.result,
    this.error,
    this.pendingSso,
  });

  static const idle = LoginState(phase: LoginPhase.idle);

  bool get isSubmitting =>
      phase == LoginPhase.submitting || phase == LoginPhase.awaitingSso;
}

/// Notifier that owns the in-flight login attempt for the active connection.
///
/// The UI calls [submitLocal] / [submitSso]. On success the state transitions
/// to `LoginPhase.success` carrying a `LoginResult` and the surrounding app
/// (`AppSessionNotifier`) is responsible for persisting tokens via the
/// SecureTokenStore. We deliberately keep this notifier free of any reference
/// to `appSessionProvider` so the auth and session layers stay decoupled.
class LoginStateNotifier extends StateNotifier<LoginState> {
  final LoginService _service;

  LoginStateNotifier(this._service) : super(LoginState.idle);

  void reset() {
    state = LoginState.idle;
  }

  /// Run a local username/password login. Errors are caught and surfaced via
  /// state — the notifier never throws.
  Future<void> submitLocal({
    required HomeDirectoryConnection connection,
    required String username,
    required String password,
  }) async {
    state = const LoginState(phase: LoginPhase.submitting);
    try {
      final result = await _service.loginLocal(connection, username, password);
      state = LoginState(phase: LoginPhase.success, result: result);
    } on LoginError catch (e) {
      state = LoginState(phase: LoginPhase.error, error: e);
    } catch (e) {
      state = LoginState(
        phase: LoginPhase.error,
        error: LoginError(LoginErrorKind.malformed, 'unexpected: $e'),
      );
    }
  }

  /// Run the full SSO flow: launch the authorizer, then exchange the code at
  /// the directory's `/sso/complete` endpoint. Cancellation surfaces as
  /// `LoginPhase.cancelled` so the UI can show a non-error toast.
  Future<void> submitSso({
    required HomeDirectoryConnection connection,
    required String providerId,
    AvailableAuthMethods? knownMethods,
  }) async {
    state = const LoginState(phase: LoginPhase.awaitingSso);
    SsoFlow flow;
    try {
      flow = await _service.beginSso(
        connection,
        providerId,
        knownMethods: knownMethods,
      );
    } on LoginError catch (e) {
      state = LoginState(phase: LoginPhase.error, error: e);
      return;
    }
    if (flow.cancelled) {
      state = LoginState(phase: LoginPhase.cancelled, pendingSso: flow);
      return;
    }
    state = LoginState(phase: LoginPhase.submitting, pendingSso: flow);
    try {
      final result = await _service.completeSso(connection, flow);
      state = LoginState(phase: LoginPhase.success, result: result);
    } on LoginError catch (e) {
      state = LoginState(phase: LoginPhase.error, error: e, pendingSso: flow);
    }
  }
}

/// State notifier for the in-flight login attempt. Auto-disposed so switching
/// directories doesn't leak the previous attempt's state.
final loginStateProvider =
    StateNotifierProvider.autoDispose<LoginStateNotifier, LoginState>((ref) {
  final svc = ref.watch(loginServiceProvider);
  return LoginStateNotifier(svc);
});
