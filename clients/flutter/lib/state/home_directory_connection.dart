// KAI-295 — HomeDirectoryConnection state type.
//
// Models the single active "home" the Flutter app is connected to. Per the
// Raikada connection model, the app speaks one protocol on the wire (OIDC + a
// local form), but the backend can be either the multi-tenant cloud or an
// on-prem Directory. Both surface the same shape — only the endpoint differs.
//
// This file is intentionally hand-written (no freezed/json_serializable code-
// gen) so it can be consumed without a `build_runner` step. Match the Riverpod
// patterns already used in `lib/providers/`.

/// Which kind of "home" the user has connected to.
///
/// `cloud` is the multi-tenant Raikada cloud (e.g. `cloud.yourbrand.com`).
/// `onPrem` is a self-hosted Directory (e.g. `https://nvr.acme.local`).
enum HomeConnectionKind {
  cloud,
  onPrem,
}

/// How the user discovered / picked this connection.
///
/// Used by the discovery flow (KAI-296) to remember provenance for analytics
/// and to drive UI hints (e.g. "found via mDNS — refresh?").
enum DiscoveryMethod {
  mdns,
  qrCode,
  manual,
}

String _kindToWire(HomeConnectionKind k) {
  switch (k) {
    case HomeConnectionKind.cloud:
      return 'cloud';
    case HomeConnectionKind.onPrem:
      return 'on_prem';
  }
}

HomeConnectionKind _kindFromWire(String s) {
  switch (s) {
    case 'cloud':
      return HomeConnectionKind.cloud;
    case 'on_prem':
      return HomeConnectionKind.onPrem;
    default:
      throw ArgumentError('Unknown HomeConnectionKind: $s');
  }
}

String _methodToWire(DiscoveryMethod m) {
  switch (m) {
    case DiscoveryMethod.mdns:
      return 'mdns';
    case DiscoveryMethod.qrCode:
      return 'qr_code';
    case DiscoveryMethod.manual:
      return 'manual';
  }
}

DiscoveryMethod _methodFromWire(String s) {
  switch (s) {
    case 'mdns':
      return DiscoveryMethod.mdns;
    case 'qr_code':
      return DiscoveryMethod.qrCode;
    case 'manual':
      return DiscoveryMethod.manual;
    default:
      throw ArgumentError('Unknown DiscoveryMethod: $s');
  }
}

/// A single home Directory the app can connect to.
///
/// Identity is a UUID minted by the client when the connection is first added,
/// not by the server — this lets the same physical Directory show up under
/// multiple identities if the user re-adds it after `forget`.
class HomeDirectoryConnection {
  /// Client-side UUID. Used as the secure-storage key prefix for tokens.
  final String id;

  /// Whether this is cloud or on-prem.
  final HomeConnectionKind kind;

  /// Base URL of the Directory or cloud, e.g. `https://nvr.acme.local`.
  final String endpointUrl;

  /// User-visible name. Defaults to the host but the user can rename.
  final String displayName;

  /// How the user added this connection.
  final DiscoveryMethod discoveryMethod;

  /// Catalog version last fetched from this connection. Used by the federation
  /// layer to decide whether to re-pull the camera/group catalog.
  final int? cachedCatalogVersion;

  /// Wall-clock time of the last successful sync. `null` if never synced.
  final DateTime? lastSyncAt;

  const HomeDirectoryConnection({
    required this.id,
    required this.kind,
    required this.endpointUrl,
    required this.displayName,
    required this.discoveryMethod,
    this.cachedCatalogVersion,
    this.lastSyncAt,
  });

  HomeDirectoryConnection copyWith({
    String? id,
    HomeConnectionKind? kind,
    String? endpointUrl,
    String? displayName,
    DiscoveryMethod? discoveryMethod,
    int? cachedCatalogVersion,
    DateTime? lastSyncAt,
  }) {
    return HomeDirectoryConnection(
      id: id ?? this.id,
      kind: kind ?? this.kind,
      endpointUrl: endpointUrl ?? this.endpointUrl,
      displayName: displayName ?? this.displayName,
      discoveryMethod: discoveryMethod ?? this.discoveryMethod,
      cachedCatalogVersion: cachedCatalogVersion ?? this.cachedCatalogVersion,
      lastSyncAt: lastSyncAt ?? this.lastSyncAt,
    );
  }

  Map<String, dynamic> toJson() => {
        'id': id,
        'kind': _kindToWire(kind),
        'endpoint_url': endpointUrl,
        'display_name': displayName,
        'discovery_method': _methodToWire(discoveryMethod),
        'cached_catalog_version': cachedCatalogVersion,
        'last_sync_at': lastSyncAt?.toUtc().toIso8601String(),
      };

  factory HomeDirectoryConnection.fromJson(Map<String, dynamic> json) {
    return HomeDirectoryConnection(
      id: json['id'] as String,
      kind: _kindFromWire(json['kind'] as String),
      endpointUrl: json['endpoint_url'] as String,
      displayName: json['display_name'] as String,
      discoveryMethod: _methodFromWire(json['discovery_method'] as String),
      cachedCatalogVersion: json['cached_catalog_version'] as int?,
      lastSyncAt: json['last_sync_at'] == null
          ? null
          : DateTime.parse(json['last_sync_at'] as String),
    );
  }

  @override
  bool operator ==(Object other) {
    if (identical(this, other)) return true;
    return other is HomeDirectoryConnection &&
        other.id == id &&
        other.kind == kind &&
        other.endpointUrl == endpointUrl &&
        other.displayName == displayName &&
        other.discoveryMethod == discoveryMethod &&
        other.cachedCatalogVersion == cachedCatalogVersion &&
        other.lastSyncAt == lastSyncAt;
  }

  @override
  int get hashCode => Object.hash(
        id,
        kind,
        endpointUrl,
        displayName,
        discoveryMethod,
        cachedCatalogVersion,
        lastSyncAt,
      );

  @override
  String toString() =>
      'HomeDirectoryConnection(id: $id, kind: $kind, endpoint: $endpointUrl)';
}
