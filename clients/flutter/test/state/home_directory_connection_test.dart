// KAI-295 — HomeDirectoryConnection serialization tests.

import 'package:flutter_test/flutter_test.dart';
import 'package:nvr_client/state/home_directory_connection.dart';

void main() {
  group('HomeDirectoryConnection', () {
    test('JSON roundtrip preserves all fields (cloud)', () {
      final original = HomeDirectoryConnection(
        id: 'b3d9f2c1-1111-4444-8888-aaaaaaaaaaaa',
        kind: HomeConnectionKind.cloud,
        endpointUrl: 'https://cloud.example.com',
        displayName: 'Acme Cloud',
        discoveryMethod: DiscoveryMethod.manual,
        cachedCatalogVersion: 17,
        lastSyncAt: DateTime.utc(2026, 4, 7, 12, 0, 0),
      );

      final roundTripped =
          HomeDirectoryConnection.fromJson(original.toJson());

      expect(roundTripped, equals(original));
    });

    test('JSON roundtrip preserves all fields (on-prem, mDNS)', () {
      final original = HomeDirectoryConnection(
        id: 'on-prem-uuid-1',
        kind: HomeConnectionKind.onPrem,
        endpointUrl: 'https://nvr.acme.local',
        displayName: 'HQ NVR',
        discoveryMethod: DiscoveryMethod.mdns,
        cachedCatalogVersion: null,
        lastSyncAt: null,
      );

      final json = original.toJson();
      expect(json['kind'], equals('on_prem'));
      expect(json['discovery_method'], equals('mdns'));
      expect(json['cached_catalog_version'], isNull);
      expect(json['last_sync_at'], isNull);

      final roundTripped = HomeDirectoryConnection.fromJson(json);
      expect(roundTripped, equals(original));
    });

    test('copyWith updates only specified fields', () {
      final base = HomeDirectoryConnection(
        id: 'id',
        kind: HomeConnectionKind.cloud,
        endpointUrl: 'https://a.example',
        displayName: 'A',
        discoveryMethod: DiscoveryMethod.qrCode,
      );
      final updated = base.copyWith(displayName: 'B', cachedCatalogVersion: 9);
      expect(updated.displayName, 'B');
      expect(updated.cachedCatalogVersion, 9);
      expect(updated.id, base.id);
      expect(updated.endpointUrl, base.endpointUrl);
    });
  });
}
