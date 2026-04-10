// KAI-305 — Riverpod wiring for the white-label branding layer.
//
// This file exposes providers but deliberately does NOT wire them into
// the app shell in `main.dart`. Router ownership is held by another
// engineer; a follow-up PR will flip the app's top-level `theme:` /
// `darkTheme:` to read from [brandThemeLightProvider] / [brandThemeDarkProvider].
//
// Tests and DI should override [brandConfigLoaderProvider] in a
// `ProviderScope` to supply an [InMemoryBrandConfigLoader]. The default
// factory returns a loader backed by [BrandConfig.kaivueDefault] so any
// unbranded build keeps working without a network dependency.
library;

import 'package:flutter/material.dart';
import 'package:flutter_riverpod/flutter_riverpod.dart';

import 'brand_config.dart';
import 'brand_config_loader.dart';
import 'brand_theme.dart';

/// Identifies the current tenant whose branding should be loaded.
///
/// Host apps override this in `ProviderScope` once the session is known
/// (e.g. post-login, post-discovery). Default is `'default'` which the
/// in-memory loader ignores.
final brandTenantIdProvider = Provider<String>((ref) => 'default');

/// The loader used to resolve a [BrandConfig] for the current tenant.
///
/// Defaults to an [InMemoryBrandConfigLoader] seeded with the Kaivue
/// default so unbranded builds and tests Just Work. Production apps
/// override this with a [RemoteBrandConfigLoader] wired to the eventual
/// KAI-353 asset client.
final brandConfigLoaderProvider = Provider<BrandConfigLoader>((ref) {
  return InMemoryBrandConfigLoader(BrandConfig.kaivueDefault());
});

/// The resolved [BrandConfig] for the active tenant.
///
/// This is the primary integration seam: the eventual `main.dart` wiring
/// reads this and feeds it into `MaterialApp(theme: ...)`.
final brandConfigProvider = FutureProvider<BrandConfig>((ref) async {
  final loader = ref.watch(brandConfigLoaderProvider);
  final tenantId = ref.watch(brandTenantIdProvider);
  return loader.load(tenantId);
});

/// Synchronous snapshot of the current brand config.
///
/// While the async load is in-flight or has failed, this returns
/// [BrandConfig.kaivueDefault] so callers can use it unconditionally in
/// build methods without juggling AsyncValue states.
final currentBrandConfigProvider = Provider<BrandConfig>((ref) {
  final async = ref.watch(brandConfigProvider);
  return async.maybeWhen(
    data: (config) => config,
    orElse: BrandConfig.kaivueDefault,
  );
});

/// Light-mode themed [ThemeData] for the current brand.
final brandThemeLightProvider = Provider<ThemeData>((ref) {
  final config = ref.watch(currentBrandConfigProvider);
  return buildBrandTheme(config, dark: false);
});

/// Dark-mode themed [ThemeData] for the current brand.
final brandThemeDarkProvider = Provider<ThemeData>((ref) {
  final config = ref.watch(currentBrandConfigProvider);
  return buildBrandTheme(config, dark: true);
});
