// KAI-305 — Loader tests. Covers InMemoryBrandConfigLoader determinism and
// RemoteBrandConfigLoader behavior against a fake BrandAssetClient.
import 'package:flutter_test/flutter_test.dart';
import 'package:nvr_client/branding/brand_config.dart';
import 'package:nvr_client/branding/brand_config_loader.dart';

class _FakeBrandAssetClient implements BrandAssetClient {
  _FakeBrandAssetClient(this.result);

  BrandConfig result;
  int calls = 0;
  bool throwNext = false;

  @override
  Future<BrandConfig> fetch(String tenantId) async {
    calls++;
    if (throwNext) {
      throwNext = false;
      throw Exception('simulated network failure');
    }
    return result;
  }
}

void main() {
  group('InMemoryBrandConfigLoader', () {
    test('returns the fixture regardless of tenantId', () async {
      final fixture = BrandConfig.kaivueDefault().copyWith(appName: 'Fixture');
      final loader = InMemoryBrandConfigLoader(fixture);
      expect(await loader.load('tenant-a'), equals(fixture));
      expect(await loader.load('tenant-b'), equals(fixture));
    });

    test('is deterministic across repeated calls', () async {
      final loader =
          InMemoryBrandConfigLoader(BrandConfig.kaivueDefault());
      final first = await loader.load('t');
      final second = await loader.load('t');
      expect(identical(first, second), isTrue);
    });
  });

  group('RemoteBrandConfigLoader', () {
    test('delegates first call to the BrandAssetClient', () async {
      final branded = BrandConfig.kaivueDefault().copyWith(appName: 'Acme');
      final fake = _FakeBrandAssetClient(branded);
      final loader = RemoteBrandConfigLoader(client: fake);

      final result = await loader.load('tenant-1');
      expect(result, equals(branded));
      expect(fake.calls, 1);
    });

    test('serves from cache within the freshness window', () async {
      final fake = _FakeBrandAssetClient(BrandConfig.kaivueDefault());
      final loader = RemoteBrandConfigLoader(
        client: fake,
        freshness: const Duration(minutes: 5),
      );

      await loader.load('t');
      await loader.load('t');
      await loader.load('t');
      expect(fake.calls, 1);
    });

    test('re-fetches after the freshness window expires', () async {
      var fakeNow = DateTime(2026, 1, 1, 12, 0, 0);
      final fake = _FakeBrandAssetClient(BrandConfig.kaivueDefault());
      final loader = RemoteBrandConfigLoader(
        client: fake,
        freshness: const Duration(minutes: 5),
        clock: () => fakeNow,
      );

      await loader.load('t');
      fakeNow = fakeNow.add(const Duration(minutes: 10));
      await loader.load('t');
      expect(fake.calls, 2);
    });

    test('serves stale cache when refresh fails', () async {
      var fakeNow = DateTime(2026, 1, 1, 12, 0, 0);
      final first = BrandConfig.kaivueDefault().copyWith(appName: 'v1');
      final fake = _FakeBrandAssetClient(first);
      final loader = RemoteBrandConfigLoader(
        client: fake,
        freshness: const Duration(minutes: 5),
        clock: () => fakeNow,
      );

      final v1 = await loader.load('t');
      expect(v1.appName, 'v1');

      fakeNow = fakeNow.add(const Duration(minutes: 10));
      fake.throwNext = true;

      final stale = await loader.load('t');
      expect(stale.appName, 'v1',
          reason: 'should serve stale cache when client throws');
      expect(fake.calls, 2);
    });

    test('returns Kaivue default when no cache and client throws', () async {
      final fake = _FakeBrandAssetClient(BrandConfig.kaivueDefault());
      fake.throwNext = true;
      final loader = RemoteBrandConfigLoader(client: fake);

      final result = await loader.load('t');
      expect(result, equals(BrandConfig.kaivueDefault()));
    });

    test('invalidate() forces a re-fetch on next load', () async {
      final fake = _FakeBrandAssetClient(BrandConfig.kaivueDefault());
      final loader = RemoteBrandConfigLoader(client: fake);

      await loader.load('t');
      loader.invalidate('t');
      await loader.load('t');
      expect(fake.calls, 2);
    });

    test('caches per-tenant independently', () async {
      final fake = _FakeBrandAssetClient(BrandConfig.kaivueDefault());
      final loader = RemoteBrandConfigLoader(client: fake);

      await loader.load('tenant-a');
      await loader.load('tenant-b');
      await loader.load('tenant-a');
      await loader.load('tenant-b');
      expect(fake.calls, 2);
    });
  });
}
