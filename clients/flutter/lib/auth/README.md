# auth/ — KAI-297 Flutter login flow

Post-discovery login for the Kaivue end-user app. Speaks exactly one protocol
on the wire: **OIDC PKCE + a local username/password form**. SAML, LDAP, and
any other federation lives server-side (Zitadel) and is invisible from here.

## Files

| File | Role |
|---|---|
| `auth_types.dart` | `AvailableAuthMethods`, `SsoProviderDescriptor`, `LoginResult`, `UserClaims`, `SsoFlow`, `LoginError` / `LoginErrorKind`. |
| `auth_strings.dart` | All user-visible strings, behind `AuthStrings`. Override per-locale via `authStringsProvider`. |
| `login_service.dart` | `LoginService` — `beginLogin`, `loginLocal`, `beginSso`, `completeSso`, `refresh`. Uses an injected `http.Client` so tests use `MockClient`. |
| `sso_authorizer.dart` | `SsoAuthorizer` interface + `FakeSsoAuthorizer` for tests. PKCE verifier / state generators live here. |
| `flutter_appauth_authorizer.dart` | Production `SsoAuthorizer` backed by `flutter_appauth`. **Only file that imports the platform plugin.** |
| `flutter_secure_storage_token_store.dart` | Production `SecureTokenStore` adapter from KAI-295's interface to `FlutterSecureStorage`. |
| `refresh_scheduler.dart` | Dart-side background refresh logic (5 min lead). `BackgroundTaskBinding` is the seam where WorkManager / BGTaskScheduler will plug in. |
| `auth_providers.dart` | Riverpod wiring: `loginServiceProvider`, `ssoAuthorizerProvider`, `authMethodsProvider`, `loginStateProvider`. |
| `widgets/login_screen.dart` | Composes the form + SSO list + error banner. |
| `widgets/local_login_form.dart` | Email + password fields, inline validation. |
| `widgets/sso_button_list.dart` | One button per advertised SSO provider. |
| `widgets/login_error_banner.dart` | `LoginError` → localized message + recovery hint. |

## Custom URL scheme — `kaivue://auth/callback`

OIDC redirects come back through a custom URL scheme on iOS and Android. The
exact value is `kaivue://auth/callback` and lives in
`login_service.dart` as `kKaivueAuthRedirectUri`.

### iOS (`ios/Runner/Info.plist`)

Add inside `<dict>`:

```xml
<key>CFBundleURLTypes</key>
<array>
  <dict>
    <key>CFBundleTypeRole</key>
    <string>Editor</string>
    <key>CFBundleURLName</key>
    <string>com.kaivue.auth</string>
    <key>CFBundleURLSchemes</key>
    <array>
      <string>kaivue</string>
    </array>
  </dict>
</array>
```

For per-integrator white-label builds, override `kKaivueAuthRedirectUri` with
the integrator's scheme via the brand-config layer; the `Info.plist` template
should be templated by the white-label build pipeline (`white-label` agent).

### Android (`android/app/src/main/AndroidManifest.xml`)

Inside the `<activity android:name=".MainActivity">` block:

```xml
<intent-filter android:autoVerify="false">
  <action android:name="android.intent.action.VIEW" />
  <category android:name="android.intent.category.DEFAULT" />
  <category android:name="android.intent.category.BROWSABLE" />
  <data android:scheme="kaivue" android:host="auth" />
</intent-filter>
```

The `flutter_appauth` plugin also requires its own activity declaration —
see the plugin README. The same per-integrator override applies on Android via
`build.gradle`'s `manifestPlaceholders`.

## Test seams

Everything overrideable is a Riverpod `Provider`. Tests build a
`ProviderContainer(overrides: [...])` and never touch the platform plugins:

| Override | Purpose |
|---|---|
| `authHttpClientProvider.overrideWithValue(MockClient(handler))` | Mock the network. |
| `ssoAuthorizerProvider.overrideWithValue(FakeSsoAuthorizer())` | Drive happy / cancel SSO paths. |
| `secureTokenStoreProvider.overrideWithValue(InMemorySecureTokenStore())` | KAI-295's in-memory token fake. |
| `authStringsProvider.overrideWithValue(...)` | Test against alternate strings, no string-matching frozen literals. |

## Background refresh

`RefreshScheduler` is the Dart-side scheduler — it computes the next deadline
(`expiresAt - 5 min`, floored at 1 s) and arms a one-shot timer via
`BackgroundTaskBinding`. The default binding (`InMemoryBackgroundTaskBinding`)
is fine for foreground sessions and unit tests. A separate ticket lands the
WorkManager (Android) and BGTaskScheduler (iOS) bindings; until then the
background-execution path uses the in-memory binding while the app is alive.

## Multi-directory token isolation

Tokens are stored under `kai_session:<connectionId>:<field>` (see
`ConnectionScopedKeys` in `lib/state/secure_token_store.dart`). Switching the
active connection rebinds the in-memory `AppSession.accessToken`/`refreshToken`
from the new connection's scoped keys; the previous connection's secrets stay
walled off until the user explicitly forgets it. The chaos test in
`test/auth/multi_connection_isolation_test.dart` covers this.
