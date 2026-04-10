// KAI-297 — Typed shapes for the login flow.
//
// These types are the contract between `LoginService`, the Riverpod providers,
// and the UI widgets. Everything is hand-written (no freezed codegen) to match
// the pattern already established by KAI-295 / KAI-296.

/// A single OIDC provider advertised by a directory.
///
/// The directory decides which providers are enabled and returns the metadata
/// the client needs to render a branded SSO button. `iconUrl` and `brandColor`
/// are optional — the UI falls back to a neutral default when absent.
class SsoProviderDescriptor {
  /// Stable identifier the server recognises, e.g. `google`, `zitadel`,
  /// `azure-ad`. Passed back verbatim to `/api/v1/auth/sso/complete`.
  final String id;

  /// User-visible name, e.g. "Google Workspace".
  final String displayName;

  /// Optional icon URL. Widgets treat a null/empty value as "no icon".
  final String? iconUrl;

  /// Optional 24-bit brand color as `#RRGGBB` or `#AARRGGBB`. Null means the
  /// UI should use its default accent.
  final String? brandColor;

  /// OIDC discovery document URL for this provider. `flutter_appauth` uses
  /// this to fetch the authorization + token endpoints.
  final String issuerUrl;

  /// Client ID registered with the provider. Server controls this — it can
  /// differ per directory, so we never hardcode it on the client.
  final String clientId;

  /// OAuth scopes, e.g. `['openid', 'profile', 'email']`.
  final List<String> scopes;

  const SsoProviderDescriptor({
    required this.id,
    required this.displayName,
    required this.issuerUrl,
    required this.clientId,
    required this.scopes,
    this.iconUrl,
    this.brandColor,
  });

  factory SsoProviderDescriptor.fromJson(Map<String, dynamic> json) {
    final rawScopes = json['scopes'];
    final scopes = rawScopes is List
        ? rawScopes.whereType<String>().toList(growable: false)
        : const <String>[];
    return SsoProviderDescriptor(
      id: json['id'] as String,
      displayName: json['display_name'] as String,
      issuerUrl: json['issuer_url'] as String? ?? '',
      clientId: json['client_id'] as String? ?? '',
      scopes: scopes,
      iconUrl: json['icon_url'] as String?,
      brandColor: json['brand_color'] as String?,
    );
  }

  @override
  bool operator ==(Object other) =>
      identical(this, other) ||
      other is SsoProviderDescriptor &&
          other.id == id &&
          other.displayName == displayName &&
          other.iconUrl == iconUrl &&
          other.brandColor == brandColor &&
          other.issuerUrl == issuerUrl &&
          other.clientId == clientId;

  @override
  int get hashCode =>
      Object.hash(id, displayName, iconUrl, brandColor, issuerUrl, clientId);
}

/// What auth methods a directory advertises. Returned from
/// `/api/v1/auth/methods`.
class AvailableAuthMethods {
  /// Whether the server accepts a local (username/password) form login.
  final bool localEnabled;

  /// OIDC providers the server is willing to federate with. Order is
  /// preserved — the server decides the display order.
  final List<SsoProviderDescriptor> ssoProviders;

  const AvailableAuthMethods({
    required this.localEnabled,
    required this.ssoProviders,
  });

  bool get hasSso => ssoProviders.isNotEmpty;

  factory AvailableAuthMethods.fromJson(Map<String, dynamic> json) {
    final rawProviders = json['sso_providers'];
    final providers = rawProviders is List
        ? rawProviders
            .whereType<Map>()
            .map((e) => SsoProviderDescriptor.fromJson(
                Map<String, dynamic>.from(e)))
            .toList(growable: false)
        : const <SsoProviderDescriptor>[];
    return AvailableAuthMethods(
      localEnabled: json['local_enabled'] as bool? ?? false,
      ssoProviders: providers,
    );
  }
}

/// Minimal user claims carried on a [LoginResult]. Keep this tight — the full
/// profile lives on the server; the client only needs enough to render a name
/// + avatar while the session is active.
class UserClaims {
  final String userId;
  final String? email;
  final String? displayName;
  final String? avatarUrl;
  final String tenantRef;

  const UserClaims({
    required this.userId,
    required this.tenantRef,
    this.email,
    this.displayName,
    this.avatarUrl,
  });

  factory UserClaims.fromJson(Map<String, dynamic> json) {
    return UserClaims(
      userId: json['user_id'] as String,
      tenantRef: json['tenant_ref'] as String? ?? '',
      email: json['email'] as String?,
      displayName: json['display_name'] as String?,
      avatarUrl: json['avatar_url'] as String?,
    );
  }
}

/// Result of a successful login (local or SSO) or of a refresh.
class LoginResult {
  final String accessToken;
  final String refreshToken;

  /// Absolute expiry of [accessToken]. Callers use this to schedule the next
  /// background refresh.
  final DateTime expiresAt;

  final UserClaims user;

  /// True iff the server wants the client to complete an MFA challenge before
  /// the session is usable. Full MFA flow lands in v1.x; for now this is a
  /// stubbed pass-through so call sites know the shape.
  final bool requiresMfa;

  const LoginResult({
    required this.accessToken,
    required this.refreshToken,
    required this.expiresAt,
    required this.user,
    this.requiresMfa = false,
  });

  factory LoginResult.fromJson(Map<String, dynamic> json) {
    final expiresInSec = json['expires_in'];
    final expiresAtStr = json['expires_at'];
    DateTime expiresAt;
    if (expiresAtStr is String) {
      expiresAt = DateTime.parse(expiresAtStr).toUtc();
    } else if (expiresInSec is num) {
      expiresAt = DateTime.now()
          .toUtc()
          .add(Duration(seconds: expiresInSec.toInt()));
    } else {
      // Conservative default — 15 minutes. The refresh scheduler will bounce
      // us out long before that if the server actually issued a shorter TTL.
      expiresAt = DateTime.now().toUtc().add(const Duration(minutes: 15));
    }
    return LoginResult(
      accessToken: json['access_token'] as String,
      refreshToken: json['refresh_token'] as String,
      expiresAt: expiresAt,
      user: UserClaims.fromJson(
          Map<String, dynamic>.from(json['user'] as Map)),
      requiresMfa: json['requires_mfa'] as bool? ?? false,
    );
  }
}

/// In-flight SSO authorization state. Returned from `beginSso` so the UI can
/// surface a "cancelled" state separately from a hard error, and so the
/// completion step can thread the authorization code back into the server.
class SsoFlow {
  /// Stable flow ID minted by the client. Used only for correlation in logs.
  final String flowId;
  final String providerId;

  /// Authorization code returned by the IdP after the user consents. Null if
  /// the user cancelled the flow.
  final String? authorizationCode;

  /// `state` parameter round-tripped back from the IdP. The server re-verifies
  /// this on `/sso/complete`.
  final String? state;

  /// PKCE verifier kept around so the server can validate the code exchange.
  final String? codeVerifier;

  /// Cryptographic nonce sent in the authorization request. Validated against
  /// the `nonce` claim in the ID token by [LoginService].
  final String? nonce;

  /// The `state` value originally sent in the authorization request. Used for
  /// state-first validation before any other processing.
  final String? sentState;

  /// True iff the user explicitly cancelled before granting consent. Distinct
  /// from a network/provider error.
  final bool cancelled;

  /// Non-null when the SSO authorizer surfaced a typed error.
  final LoginErrorKind? errorKind;
  final String? errorMessage;

  /// Issuer URL of the provider, for ID token issuer validation.
  final String? issuerUrl;

  const SsoFlow({
    required this.flowId,
    required this.providerId,
    this.authorizationCode,
    this.state,
    this.codeVerifier,
    this.nonce,
    this.sentState,
    this.cancelled = false,
    this.errorKind,
    this.errorMessage,
    this.issuerUrl,
  });

  SsoFlow cancel() => SsoFlow(
        flowId: flowId,
        providerId: providerId,
        cancelled: true,
      );
}

/// Typed login error surface. Exactly one kind is produced per failed call.
///
/// The five SSO-specific variants (`cancelled`, `idpRejected`, `ssoPlugin`,
/// `malformed`, `unknown`) were approved by lead-security in the KAI-297
/// review. The remaining variants cover non-SSO paths (local login, refresh,
/// server errors).
enum LoginErrorKind {
  // ---- SSO 5-variant enum (lead-security approved) ----

  /// User explicitly cancelled the SSO flow (closed the browser / hit back).
  cancelled,

  /// OAuth error response from the IdP (error/error_description in response,
  /// e.g. `invalid_grant`, `access_denied`).
  idpRejected,

  /// Any `FlutterAppAuthException` that isn't user-initiated — network error
  /// during discovery, internal plugin error, etc.
  ssoPlugin,

  /// Our parsing / validation failed: state mismatch, nonce mismatch, bad
  /// claims, missing required fields.
  malformed,

  /// Genuinely unexpected error — the default catch-all.
  unknown,

  // ---- Non-SSO variants (unchanged from prior code) ----

  /// Server rejected the credentials (401 on local login).
  wrongCredentials,

  /// Network I/O failed — timeout, DNS, TLS, etc.
  network,

  /// The caller asked for an SSO provider the directory doesn't advertise.
  unknownProvider,

  /// Refresh token is expired or revoked. UI should bounce to login.
  refreshExpired,

  /// Non-2xx response that's neither 401 nor 404 — treat as transient.
  server,
}

class LoginError implements Exception {
  final LoginErrorKind kind;
  final String debugMessage;

  const LoginError(this.kind, this.debugMessage);

  @override
  String toString() => 'LoginError($kind): $debugMessage';
}
