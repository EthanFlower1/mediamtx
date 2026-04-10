// KAI-298 — Web-only warning banner for non-persistent token storage.
//
// When the Flutter app is built for web, `productionOverrides()` wires the
// secure token store to `InMemorySecureTokenStore` because the
// `flutter_secure_storage_web` IndexedDB adapter has not been integrated yet.
// Per the lead-security review on PR #149 (KAI-298) the user MUST be warned
// that the session will not persist across tab refresh.
//
// This file only exposes the banner widget — it does NOT mount itself. A
// follow-up PR (router owner) will wire it into the top-level app shell when
// one exists.

import 'package:flutter/foundation.dart' show kIsWeb;
import 'package:flutter/material.dart';
import 'package:flutter_riverpod/flutter_riverpod.dart';

import '../state/app_session.dart';
import '../state/secure_token_store.dart';
import 'auth_providers.dart';

/// Returns a [MaterialBanner] describing the non-persistent web session when
/// running on web AND the resolved secure token store is
/// [InMemorySecureTokenStore]. On any other platform, or when a persistent
/// store is wired, returns [SizedBox.shrink].
///
/// This helper is informational only — it does NOT block the user from using
/// the app.
Widget maybeWebTokenStorageWarning(
  BuildContext context,
  WidgetRef ref, {
  @visibleForTesting bool isWebOverride = kIsWeb,
}) {
  if (!isWebOverride) return const SizedBox.shrink();
  final store = ref.watch(secureTokenStoreProvider);
  if (store is! InMemorySecureTokenStore) return const SizedBox.shrink();

  final strings = ref.watch(authStringsProvider);
  return MaterialBanner(
    content: Text(strings.webSessionNotPersistentWarning),
    leading: const Icon(Icons.warning_amber_rounded),
    actions: [
      TextButton(
        onPressed: () {
          ScaffoldMessenger.of(context).hideCurrentMaterialBanner();
        },
        child: Text(strings.webSessionNotPersistentDismiss),
      ),
    ],
  );
}
