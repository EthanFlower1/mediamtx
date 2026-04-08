// KAI-296 — Riverpod wiring for discovery.
//
// Three providers, hand-written in the same style as `lib/providers/*.dart`
// (no riverpod_annotation codegen):
//
//   * [manualDiscoveryProvider] — singleton [ManualDiscovery] instance.
//   * [mdnsDiscoveryProvider]   — singleton [MdnsDiscovery] instance.
//   * [qrDiscoveryProvider]     — singleton [QrDiscovery] instance.
//   * [discoverProbeProvider]   — short-lived [DiscoverProbe]; auto-disposed.
//   * [mdnsBrowseProvider]      — `StreamProvider` that surfaces candidates.
//   * [discoveryResultsProvider]— state holder the picker UI reads from.
//   * [discoveryStringsProvider]— user-visible text (swap in tests).
//
// Everything runtime-overrideable is exposed as a plain `Provider` so tests
// and DI can `ProviderContainer(overrides: [...])` without touching globals.

import 'dart:math';

import 'package:flutter_riverpod/flutter_riverpod.dart';

import '../state/home_directory_connection.dart';
import 'discover_probe.dart';
import 'discovery.dart';
import 'discovery_strings.dart';

/// User-visible strings. Override in a test with:
/// `discoveryStringsProvider.overrideWithValue(testStrings)`.
final discoveryStringsProvider = Provider<DiscoveryStrings>(
  (ref) => DiscoveryStrings.en,
);

final manualDiscoveryProvider = Provider<ManualDiscovery>(
  (ref) => ManualDiscovery(),
);

final mdnsDiscoveryProvider = Provider<MdnsDiscovery>(
  (ref) => MdnsDiscovery(),
);

final qrDiscoveryProvider = Provider<QrDiscovery>(
  (ref) => QrDiscovery(),
);

/// Short-lived probe. Auto-disposed when nobody's listening so its underlying
/// [http.Client] is closed.
final discoverProbeProvider = Provider.autoDispose<DiscoverProbe>((ref) {
  final probe = DiscoverProbe();
  ref.onDispose(probe.dispose);
  return probe;
});

/// Live list of mDNS candidates. Subscribe from the mDNS browse screen;
/// unsubscribe by disposing the consuming widget. Web/unsupported platforms
/// produce an empty stream (see [MdnsDiscovery.isSupported]).
final mdnsBrowseProvider = StreamProvider.autoDispose<List<DiscoveryCandidate>>(
  (ref) async* {
    final mdns = ref.watch(mdnsDiscoveryProvider);
    if (!mdns.isSupported) {
      yield const [];
      return;
    }
    final seen = <String, DiscoveryCandidate>{};
    await for (final candidate in mdns.discover()) {
      seen[candidate.rawUrl] = candidate;
      yield seen.values.toList(growable: false);
    }
  },
);

/// Snapshot of whatever the discovery flow has produced so far. The picker UI
/// reads from here; the manual / mDNS / QR widgets write to it via
/// [DiscoveryResultsController].
class DiscoveryResultsState {
  final DiscoveryCandidate? selectedCandidate;
  final DiscoverResult? probeResult;
  final DiscoverProbeError? probeError;
  final bool isProbing;

  const DiscoveryResultsState({
    this.selectedCandidate,
    this.probeResult,
    this.probeError,
    this.isProbing = false,
  });

  DiscoveryResultsState copyWith({
    DiscoveryCandidate? selectedCandidate,
    DiscoverResult? probeResult,
    DiscoverProbeError? probeError,
    bool? isProbing,
  }) {
    return DiscoveryResultsState(
      selectedCandidate: selectedCandidate ?? this.selectedCandidate,
      probeResult: probeResult,
      probeError: probeError,
      isProbing: isProbing ?? this.isProbing,
    );
  }

  static const initial = DiscoveryResultsState();
}

/// Controller that runs the probe and constructs a
/// [HomeDirectoryConnection] on success.
///
/// UUIDs are minted via the package-default [uuid_pkg.Uuid] generator. Tests
/// can pass a deterministic `idGenerator` via the constructor.
class DiscoveryResultsController extends StateNotifier<DiscoveryResultsState> {
  final DiscoverProbe _probe;
  final String Function() _idGenerator;

  DiscoveryResultsController({
    required DiscoverProbe probe,
    String Function()? idGenerator,
  })  : _probe = probe,
        _idGenerator = idGenerator ?? _defaultId,
        super(DiscoveryResultsState.initial);

  /// Default ID generator. Produces a version-4-style random hex string
  /// suitable for [HomeDirectoryConnection.id]. Not cryptographically perfect
  /// — the only correctness requirement is uniqueness within the local device
  /// — but `Random.secure` is used so it's more than good enough.
  static String _defaultId() {
    final rng = Random.secure();
    String hex(int n) =>
        List.generate(n, (_) => rng.nextInt(16).toRadixString(16)).join();
    return '${hex(8)}-${hex(4)}-4${hex(3)}-${hex(4)}-${hex(12)}';
  }

  /// Probe [candidate]. Updates state as it goes; never throws.
  Future<void> probeCandidate(DiscoveryCandidate candidate) async {
    state = state.copyWith(
      selectedCandidate: candidate,
      isProbing: true,
      probeError: null,
      probeResult: null,
    );
    final Uri base;
    try {
      base = normalizeBaseUrl(candidate.rawUrl);
    } on FormatException catch (e) {
      state = state.copyWith(
        isProbing: false,
        probeError: DiscoverProbeError(
          DiscoverProbeErrorKind.malformedResponse,
          'bad URL: $e',
        ),
      );
      return;
    }

    try {
      // Pick LAN timeout for .local / mDNS candidates, cloud timeout otherwise.
      final timeout = candidate.method == DiscoveryMethod.mdns ||
              base.host.endsWith('.local')
          ? kLanProbeTimeout
          : kCloudProbeTimeout;
      final result = await _probe.probe(base, timeout: timeout);
      state = state.copyWith(
        isProbing: false,
        probeResult: result,
      );
    } on DiscoverProbeError catch (e) {
      state = state.copyWith(isProbing: false, probeError: e);
    }
  }

  /// Construct a [HomeDirectoryConnection] from the current state's probe
  /// result and selected candidate. Returns `null` if the probe has not yet
  /// succeeded.
  HomeDirectoryConnection? buildConnection() {
    final candidate = state.selectedCandidate;
    final result = state.probeResult;
    if (candidate == null || result == null) return null;

    return HomeDirectoryConnection(
      id: _idGenerator(),
      kind: result.deployment == DiscoverDeployment.cloud
          ? HomeConnectionKind.cloud
          : HomeConnectionKind.onPrem,
      endpointUrl: result.baseUrl,
      displayName: result.serverName,
      discoveryMethod: candidate.method,
    );
  }

  /// Reset to the initial state, e.g. when the user cancels the flow.
  void reset() {
    state = DiscoveryResultsState.initial;
  }
}

final discoveryResultsProvider = StateNotifierProvider.autoDispose<
    DiscoveryResultsController, DiscoveryResultsState>((ref) {
  final probe = ref.watch(discoverProbeProvider);
  return DiscoveryResultsController(probe: probe);
});
