// KAI-295 — AppSession invariants + token isolation tests.
//
// IMPORTANT: this file uses `"REPLACE_ME_*"` placeholders instead of real
// tokens. Never paste a real JWT into a test fixture.

import 'dart:convert';

import 'package:flutter_test/flutter_test.dart';
import 'package:nvr_client/state/app_session.dart';
import 'package:nvr_client/state/federation_peer.dart';
import 'package:nvr_client/state/home_directory_connection.dart';
import 'package:nvr_client/state/secure_token_store.dart';

HomeDirectoryConnection _conn(String id, {String? name}) {
  return HomeDirectoryConnection(
    id: id,
    kind: HomeConnectionKind.onPrem,
    endpointUrl: 'https://$id.example',
    displayName: name ?? id,
    discoveryMethod: DiscoveryMethod.manual,
  );
}

void main() {
  group('AppSession serialization', () {
    test('JSON roundtrip preserves connection + peers but NOT tokens', () {
      final session = AppSession(
        userId: 'u-1',
        tenantRef: 'tenant-acme',
        accessToken: 'REPLACE_ME_ACCESS',
        refreshToken: 'REPLACE_ME_REFRESH',
        activeConnection: _conn('home-1'),
        knownPeers: [
          FederationPeer(
            peerId: 'peer-A',
            endpoint: 'https://a.example',
            displayName: 'A',
            catalogVersion: 3,
            status: FederationPeerStatus.online,
            permissions: const PeerPermissionSnapshot(canListCameras: true),
          ),
        ],
      );

      final encoded = session.encode();

      // Tokens MUST NOT appear in the persisted JSON blob.
      expect(encoded.contains('REPLACE_ME_ACCESS'), isFalse);
      expect(encoded.contains('REPLACE_ME_REFRESH'), isFalse);

      final decoded = AppSession.decode(encoded);
      expect(decoded.userId, 'u-1');
      expect(decoded.tenantRef, 'tenant-acme');
      expect(decoded.activeConnection, equals(session.activeConnection));
      expect(decoded.knownPeers, equals(session.knownPeers));
      expect(decoded.accessToken, isNull);
      expect(decoded.refreshToken, isNull);
    });
  });

  group('AppSessionNotifier invariants', () {
    test('exactly one active home connection at any time', () async {
      final store = InMemorySecureTokenStore();
      final notifier = AppSessionNotifier(store);

      expect(notifier.state.activeConnection, isNull);

      await notifier.activateConnection(
        connection: _conn('home-1'),
        userId: 'u-1',
        tenantRef: 't-1',
      );
      expect(notifier.state.activeConnection?.id, equals('home-1'));

      // Switching replaces, never accumulates.
      await notifier.switchConnection(target: _conn('home-2'));
      expect(notifier.state.activeConnection?.id, equals('home-2'));
      // Peers from the previous home are dropped on switch.
      expect(notifier.state.knownPeers, isEmpty);
    });

    test('switching connections does NOT leak tokens between them', () async {
      final store = InMemorySecureTokenStore();
      final notifier = AppSessionNotifier(store);

      // Activate home-1 and store its tokens.
      await notifier.activateConnection(
        connection: _conn('home-1'),
        userId: 'u-1',
        tenantRef: 't-1',
      );
      await notifier.setTokens(
        accessToken: 'REPLACE_ME_ACCESS_1',
        refreshToken: 'REPLACE_ME_REFRESH_1',
      );
      expect(notifier.state.accessToken, 'REPLACE_ME_ACCESS_1');

      // Switch to home-2 — has no tokens yet, so the in-memory tokens MUST
      // be cleared (otherwise we'd present home-1's token to home-2).
      await notifier.switchConnection(target: _conn('home-2'));
      expect(notifier.state.activeConnection?.id, 'home-2');
      expect(notifier.state.accessToken, isNull,
          reason: 'home-2 has no tokens; home-1 token must not leak');
      expect(notifier.state.refreshToken, isNull);

      // Give home-2 its own distinct tokens.
      await notifier.setTokens(
        accessToken: 'REPLACE_ME_ACCESS_2',
        refreshToken: 'REPLACE_ME_REFRESH_2',
      );
      expect(notifier.state.accessToken, 'REPLACE_ME_ACCESS_2');

      // Switch back to home-1 — should restore home-1's tokens, NOT home-2's.
      await notifier.switchConnection(target: _conn('home-1'));
      expect(notifier.state.accessToken, 'REPLACE_ME_ACCESS_1');
      expect(notifier.state.refreshToken, 'REPLACE_ME_REFRESH_1');

      // Sanity: secure storage holds both, under distinct keys.
      final keys = store.keysForTest.toList();
      expect(
        keys.any((k) => k.contains('home-1') && k.contains('access_token')),
        isTrue,
      );
      expect(
        keys.any((k) => k.contains('home-2') && k.contains('access_token')),
        isTrue,
      );
    });

    test('logout wipes only the active connection tokens', () async {
      final store = InMemorySecureTokenStore();
      final notifier = AppSessionNotifier(store);

      await notifier.activateConnection(
        connection: _conn('home-1'),
        userId: 'u-1',
        tenantRef: 't-1',
      );
      await notifier.setTokens(
        accessToken: 'REPLACE_ME_A1',
        refreshToken: 'REPLACE_ME_R1',
      );

      await notifier.switchConnection(target: _conn('home-2'));
      await notifier.setTokens(
        accessToken: 'REPLACE_ME_A2',
        refreshToken: 'REPLACE_ME_R2',
      );

      // Log out of home-2 only.
      await notifier.logout();
      expect(notifier.state.accessToken, isNull);

      // home-1's tokens should still be in secure storage.
      final after = store.keysForTest.toList();
      expect(after.any((k) => k.contains('home-1')), isTrue);
      expect(after.any((k) => k.contains('home-2')), isFalse);
    });

    test('forgetConnection clears that connection only', () async {
      final store = InMemorySecureTokenStore();
      final notifier = AppSessionNotifier(store);

      await notifier.activateConnection(
        connection: _conn('home-1'),
        userId: 'u-1',
        tenantRef: 't-1',
      );
      await notifier.setTokens(
        accessToken: 'REPLACE_ME_A1',
        refreshToken: 'REPLACE_ME_R1',
      );
      await notifier.switchConnection(target: _conn('home-2'));
      await notifier.setTokens(
        accessToken: 'REPLACE_ME_A2',
        refreshToken: 'REPLACE_ME_R2',
      );

      await notifier.forgetConnection('home-1');
      final remaining = store.keysForTest.toList();
      expect(remaining.any((k) => k.contains('home-1')), isFalse);
      expect(remaining.any((k) => k.contains('home-2')), isTrue);
    });

    test('setTokens without active connection throws', () async {
      final notifier = AppSessionNotifier(InMemorySecureTokenStore());
      expect(
        () => notifier.setTokens(
          accessToken: 'REPLACE_ME',
          refreshToken: 'REPLACE_ME',
        ),
        throwsStateError,
      );
    });

    test('upsertPeer adds then updates a cached peer in place', () async {
      final notifier = AppSessionNotifier(InMemorySecureTokenStore());
      await notifier.activateConnection(
        connection: _conn('home-1'),
        userId: 'u-1',
        tenantRef: 't-1',
      );

      final p1 = FederationPeer(
        peerId: 'peer-A',
        endpoint: 'https://a.example',
        displayName: 'A',
        catalogVersion: 1,
        status: FederationPeerStatus.online,
        permissions: const PeerPermissionSnapshot(canListCameras: true),
      );
      notifier.upsertPeer(p1);
      expect(notifier.state.knownPeers, hasLength(1));

      final p1Updated = p1.copyWith(
        catalogVersion: 2,
        status: FederationPeerStatus.stale,
      );
      notifier.upsertPeer(p1Updated);
      expect(notifier.state.knownPeers, hasLength(1));
      expect(notifier.state.knownPeers.first.status,
          FederationPeerStatus.stale);
      expect(notifier.state.knownPeers.first.catalogVersion, 2);
    });
  });

  group('parseGroupsFromJwt (KAI-147 crossover)', () {
    // Build a JWT with a given payload. Signature is a dummy — parser does
    // NOT validate it (server remains authoritative).
    String _makeJwt(Map<String, dynamic> payload) {
      String _b64url(String input) {
        final bytes = utf8.encode(input);
        var encoded = base64Url.encode(bytes);
        // Strip padding; parser re-pads on its own.
        return encoded.replaceAll('=', '');
      }

      final header = _b64url(jsonEncode({'alg': 'none', 'typ': 'JWT'}));
      final body = _b64url(jsonEncode(payload));
      return '$header.$body.sig';
    }

    test('extracts groups list from a synthetic JWT', () {
      final jwt = _makeJwt({
        'sub': 'u-1',
        'groups': ['admin', 'viewer'],
      });
      expect(parseGroupsFromJwt(jwt), equals(['admin', 'viewer']));
    });

    test('returns empty list when groups claim is empty', () {
      final jwt = _makeJwt({'sub': 'u-1', 'groups': <String>[]});
      expect(parseGroupsFromJwt(jwt), isEmpty);
    });

    test('returns empty list when groups claim is missing', () {
      final jwt = _makeJwt({'sub': 'u-1'});
      expect(parseGroupsFromJwt(jwt), isEmpty);
    });

    test('accepts groups as a space-separated string', () {
      final jwt = _makeJwt({'sub': 'u-1', 'groups': 'admin viewer auditor'});
      expect(parseGroupsFromJwt(jwt), equals(['admin', 'viewer', 'auditor']));
    });

    test('returns empty list on malformed JWT without throwing', () {
      expect(parseGroupsFromJwt('not-a-jwt'), isEmpty);
      expect(parseGroupsFromJwt(''), isEmpty);
      expect(parseGroupsFromJwt('a.b'), isEmpty);
    });

    test('setTokens parses groups onto AppSession', () async {
      final store = InMemorySecureTokenStore();
      final notifier = AppSessionNotifier(store);
      await notifier.activateConnection(
        connection: _conn('home-1'),
        userId: 'u-1',
        tenantRef: 'tenant-1',
      );

      final jwt = _makeJwt({
        'sub': 'u-1',
        'groups': ['admin', 'viewer'],
      });
      await notifier.setTokens(accessToken: jwt, refreshToken: 'r-1');

      expect(notifier.state.groups, equals(['admin', 'viewer']));
      expect(notifier.state.accessToken, jwt);
    });
  });
}
