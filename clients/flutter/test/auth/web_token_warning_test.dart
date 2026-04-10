// KAI-298 — Tests for maybeWebTokenStorageWarning.
//
// The helper must:
//   * Return SizedBox.shrink on non-web platforms.
//   * Build a MaterialBanner on web when the resolved secure token store is
//     InMemorySecureTokenStore.

import 'package:flutter/material.dart';
import 'package:flutter_riverpod/flutter_riverpod.dart';
import 'package:flutter_test/flutter_test.dart';

import 'package:nvr_client/auth/auth_providers.dart';
import 'package:nvr_client/auth/auth_strings.dart';
import 'package:nvr_client/auth/web_token_warning.dart';
import 'package:nvr_client/state/app_session.dart';
import 'package:nvr_client/state/secure_token_store.dart';

Widget _host({
  required bool isWebOverride,
  required SecureTokenStore store,
}) {
  return ProviderScope(
    overrides: [
      secureTokenStoreProvider.overrideWithValue(store),
      authStringsProvider.overrideWithValue(AuthStrings.en),
    ],
    child: MaterialApp(
      home: Consumer(
        builder: (context, ref, _) {
          return Scaffold(
            body: maybeWebTokenStorageWarning(
              context,
              ref,
              isWebOverride: isWebOverride,
            ),
          );
        },
      ),
    ),
  );
}

void main() {
  testWidgets(
      'maybeWebTokenStorageWarning returns SizedBox.shrink on non-web',
      (tester) async {
    await tester.pumpWidget(_host(
      isWebOverride: false,
      store: InMemorySecureTokenStore(),
    ));
    // No banner should be present.
    expect(find.byType(MaterialBanner), findsNothing);
    expect(find.byType(SizedBox), findsWidgets);
  });

  testWidgets(
      'maybeWebTokenStorageWarning returns SizedBox.shrink when store is persistent',
      (tester) async {
    // Even on the web path, if the store is not in-memory, nothing renders.
    await tester.pumpWidget(_host(
      isWebOverride: true,
      store: _FakePersistentStore(),
    ));
    expect(find.byType(MaterialBanner), findsNothing);
  });

  testWidgets(
      'maybeWebTokenStorageWarning builds a MaterialBanner on web with in-memory store',
      (tester) async {
    await tester.pumpWidget(_host(
      isWebOverride: true,
      store: InMemorySecureTokenStore(),
    ));
    expect(find.byType(MaterialBanner), findsOneWidget);
    expect(
      find.text(AuthStrings.en.webSessionNotPersistentWarning),
      findsOneWidget,
    );
    expect(
      find.text(AuthStrings.en.webSessionNotPersistentDismiss),
      findsOneWidget,
    );
  });
}

class _FakePersistentStore implements SecureTokenStore {
  @override
  Future<String?> read(String key) async => null;
  @override
  Future<void> write(String key, String value) async {}
  @override
  Future<void> delete(String key) async {}
  @override
  Future<void> deleteByPrefix(String prefix) async {}
}
