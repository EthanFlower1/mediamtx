// KAI-295 — FederationPeer serialization + status transition tests.

import 'package:flutter_test/flutter_test.dart';
import 'package:nvr_client/state/federation_peer.dart';

FederationPeer _peer({
  FederationPeerStatus status = FederationPeerStatus.online,
  int catalogVersion = 1,
}) {
  return FederationPeer(
    peerId: 'peer-1',
    endpoint: 'https://branch1.example',
    displayName: 'Branch 1',
    catalogVersion: catalogVersion,
    status: status,
    permissions: const PeerPermissionSnapshot(
      canListCameras: true,
      canViewLive: true,
      canViewPlayback: false,
      canExport: false,
      extraClaims: {'beta_feature': true},
    ),
    lastSyncAt: DateTime.utc(2026, 4, 7, 9, 30),
  );
}

void main() {
  group('FederationPeer', () {
    test('JSON roundtrip preserves all fields including extra claims', () {
      final original = _peer();
      final json = original.toJson();

      expect(json['status'], equals('online'));
      final perms = json['permissions'] as Map<String, dynamic>;
      expect(perms['extra_claims'], equals({'beta_feature': true}));

      final roundTripped = FederationPeer.fromJson(json);
      expect(roundTripped, equals(original));
    });

    test('status transition: online -> stale -> offline -> online', () {
      var p = _peer(status: FederationPeerStatus.online);

      // online -> stale
      p = p.copyWith(status: FederationPeerStatus.stale);
      expect(p.status, FederationPeerStatus.stale);

      // stale -> offline
      p = p.copyWith(status: FederationPeerStatus.offline);
      expect(p.status, FederationPeerStatus.offline);

      // offline -> online (via successful sync, bumps catalogVersion + time)
      final syncTime = DateTime.utc(2026, 4, 7, 10, 0, 0);
      p = p.copyWith(
        status: FederationPeerStatus.online,
        catalogVersion: p.catalogVersion + 1,
        lastSyncAt: syncTime,
      );
      expect(p.status, FederationPeerStatus.online);
      expect(p.catalogVersion, equals(2));
      expect(p.lastSyncAt, equals(syncTime));
    });

    test('PeerPermissionSnapshot serialises and deserialises defaults', () {
      const snap = PeerPermissionSnapshot();
      final r = PeerPermissionSnapshot.fromJson(snap.toJson());
      expect(r.canListCameras, isFalse);
      expect(r.canViewLive, isFalse);
      expect(r.canViewPlayback, isFalse);
      expect(r.canExport, isFalse);
      expect(r.extraClaims, isEmpty);
    });
  });
}
