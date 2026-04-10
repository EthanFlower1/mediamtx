import 'package:flutter/foundation.dart';
import 'package:flutter/material.dart';
import 'package:flutter_riverpod/flutter_riverpod.dart';
import 'package:flutter_secure_storage/flutter_secure_storage.dart';
import 'package:fvp/fvp.dart';
import 'package:media_kit/media_kit.dart';
import 'app.dart';
import 'auth/auth_providers.dart';
import 'auth/flutter_appauth_authorizer.dart';
import 'auth/flutter_secure_storage_token_store.dart';
import 'state/app_session.dart';
import 'state/secure_token_store.dart';

/// Production overrides for [ProviderScope].
///
/// * [secureTokenStoreProvider] → [FlutterSecureStorageTokenStore] on native
///   (Keychain on iOS, Keystore on Android, libsecret/DPAPI on desktop); the
///   in-memory fake on web until an encrypted IndexedDB adapter lands.
/// * [ssoAuthorizerProvider] → [FlutterAppAuthAuthorizer] unconditionally so
///   production builds never fall back to [FakeSsoAuthorizer].
List<Override> productionOverrides() {
  // KAI-298 security review: device-bound Keychain (no iCloud sync), encrypted SharedPreferences on Android — see PR #149 lead-security review.
  const secureStorage = FlutterSecureStorage(
    iOptions: IOSOptions(
      accessibility: KeychainAccessibility.first_unlock_this_device,
    ),
    aOptions: AndroidOptions(
      encryptedSharedPreferences: true,
    ),
  );
  return <Override>[
    secureTokenStoreProvider.overrideWithValue(
      kIsWeb
          ? InMemorySecureTokenStore()
          : FlutterSecureStorageTokenStore(secureStorage),
    ),
    ssoAuthorizerProvider.overrideWithValue(FlutterAppAuthAuthorizer()),
  ];
}

void main() {
  WidgetsFlutterBinding.ensureInitialized();
  MediaKit.ensureInitialized();
  registerWith(); // fvp: use libmdk as video_player backend
  runApp(ProviderScope(
    overrides: productionOverrides(),
    child: const NvrApp(),
  ));
}
