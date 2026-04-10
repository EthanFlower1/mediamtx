// KAI-305 — BrandConfig loader abstraction.
//
// The loader is deliberately split from any concrete HTTP wiring. The
// server-side brand asset storage (KAI-353, lead-sre) is still in flight
// and the final proto/HTTP shape is not yet frozen, so we:
//
//   1. Define an abstract [BrandConfigLoader].
//   2. Ship an [InMemoryBrandConfigLoader] for tests and unbranded builds.
//   3. Ship a [RemoteBrandConfigLoader] that depends on an abstract
//      [BrandAssetClient] — NOT on `dio`, `http`, or any URL shape.
//
// Once KAI-353 ships its proto definition (see PR body: `service
// cloud.branding.v1.BrandAssets.GetBrandConfig`), a follow-up PR will land
// a concrete `ConnectBrandAssetClient` in a separate file without touching
// any of this module.
library;

import 'brand_config.dart';

/// Abstract entrypoint for loading a tenant's [BrandConfig].
abstract class BrandConfigLoader {
  /// Returns the [BrandConfig] for [tenantId].
  ///
  /// Implementations should return a sensible default (typically
  /// [BrandConfig.kaivueDefault]) rather than throwing on network failure,
  /// so the app shell can always render something.
  Future<BrandConfig> load(String tenantId);
}

/// In-memory loader used by tests, fixtures, and unbranded dev builds.
///
/// Deterministic: `load()` always returns the exact instance passed to the
/// constructor, regardless of `tenantId`.
class InMemoryBrandConfigLoader implements BrandConfigLoader {
  const InMemoryBrandConfigLoader(this.fixture);

  final BrandConfig fixture;

  @override
  Future<BrandConfig> load(String tenantId) async => fixture;
}

/// Abstract client that knows how to fetch a raw [BrandConfig] from
/// wherever brand assets live (KAI-353 cloud bucket, on-prem mirror, etc.).
///
/// The HTTP / gRPC / Connect-Go shape is intentionally NOT defined here.
/// That mapping lives in a follow-up PR once the server side lands.
abstract class BrandAssetClient {
  Future<BrandConfig> fetch(String tenantId);
}

/// Remote loader with simple in-memory caching + staleness.
///
/// Behavior:
///   * First call per tenant → delegates to [client] and caches the result.
///   * Subsequent calls within [freshness] → returns cache (no fetch).
///   * Calls after [freshness] → tries to refresh; on failure, returns the
///     stale cached value (best-effort).
///   * Calls with no cache and a failing client → returns the Kaivue
///     default rather than throwing.
class RemoteBrandConfigLoader implements BrandConfigLoader {
  RemoteBrandConfigLoader({
    required this.client,
    this.freshness = const Duration(minutes: 5),
    DateTime Function()? clock,
  }) : _clock = clock ?? DateTime.now;

  final BrandAssetClient client;
  final Duration freshness;
  final DateTime Function() _clock;

  final Map<String, _CacheEntry> _cache = {};

  @override
  Future<BrandConfig> load(String tenantId) async {
    final now = _clock();
    final cached = _cache[tenantId];

    if (cached != null && now.difference(cached.fetchedAt) < freshness) {
      return cached.config;
    }

    try {
      final fresh = await client.fetch(tenantId);
      _cache[tenantId] = _CacheEntry(fresh, now);
      return fresh;
    } catch (_) {
      if (cached != null) {
        // Serve stale rather than blanking the UI.
        return cached.config;
      }
      return BrandConfig.kaivueDefault();
    }
  }

  /// Test hook: drop the cache for a tenant (or all tenants).
  void invalidate([String? tenantId]) {
    if (tenantId == null) {
      _cache.clear();
    } else {
      _cache.remove(tenantId);
    }
  }
}

class _CacheEntry {
  const _CacheEntry(this.config, this.fetchedAt);
  final BrandConfig config;
  final DateTime fetchedAt;
}
