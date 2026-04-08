// KAI-295 — FederationPeer state type.
//
// A FederationPeer is the cached snapshot the app keeps about a Directory that
// is federated with the user's currently active home connection. Peers are not
// directly logged in to — the home Directory brokers access — but the client
// caches enough metadata to render a unified catalog and to surface staleness.

/// Liveness of a federation peer as observed by the home Directory.
///
/// Transitions:
///   online  -> stale    after `cachedCatalogVersion` not refreshed > X min
///   stale   -> offline  after no contact for > Y min
///   offline -> online   on a successful sync
enum FederationPeerStatus {
  online,
  stale,
  offline,
}

String _statusToWire(FederationPeerStatus s) {
  switch (s) {
    case FederationPeerStatus.online:
      return 'online';
    case FederationPeerStatus.stale:
      return 'stale';
    case FederationPeerStatus.offline:
      return 'offline';
  }
}

FederationPeerStatus _statusFromWire(String s) {
  switch (s) {
    case 'online':
      return FederationPeerStatus.online;
    case 'stale':
      return FederationPeerStatus.stale;
    case 'offline':
      return FederationPeerStatus.offline;
    default:
      throw ArgumentError('Unknown FederationPeerStatus: $s');
  }
}

/// A flat permission snapshot for a single peer.
///
/// Kept intentionally minimal so the client doesn't get coupled to the
/// server-side RBAC engine. The home Directory issues this; the client just
/// gates UI on it.
class PeerPermissionSnapshot {
  /// True if the user can list the cameras on this peer.
  final bool canListCameras;

  /// True if the user can request a live stream.
  final bool canViewLive;

  /// True if the user can scrub recorded playback.
  final bool canViewPlayback;

  /// True if the user can export clips.
  final bool canExport;

  /// Free-form additional claims (forward-compat, e.g. for new permissions).
  final Map<String, dynamic> extraClaims;

  const PeerPermissionSnapshot({
    this.canListCameras = false,
    this.canViewLive = false,
    this.canViewPlayback = false,
    this.canExport = false,
    this.extraClaims = const {},
  });

  Map<String, dynamic> toJson() => {
        'can_list_cameras': canListCameras,
        'can_view_live': canViewLive,
        'can_view_playback': canViewPlayback,
        'can_export': canExport,
        'extra_claims': extraClaims,
      };

  factory PeerPermissionSnapshot.fromJson(Map<String, dynamic> json) {
    return PeerPermissionSnapshot(
      canListCameras: (json['can_list_cameras'] as bool?) ?? false,
      canViewLive: (json['can_view_live'] as bool?) ?? false,
      canViewPlayback: (json['can_view_playback'] as bool?) ?? false,
      canExport: (json['can_export'] as bool?) ?? false,
      extraClaims: Map<String, dynamic>.from(
        (json['extra_claims'] as Map?) ?? const {},
      ),
    );
  }

  @override
  bool operator ==(Object other) =>
      identical(this, other) ||
      (other is PeerPermissionSnapshot &&
          other.canListCameras == canListCameras &&
          other.canViewLive == canViewLive &&
          other.canViewPlayback == canViewPlayback &&
          other.canExport == canExport);

  @override
  int get hashCode =>
      Object.hash(canListCameras, canViewLive, canViewPlayback, canExport);
}

/// A federated Directory the home connection knows about.
class FederationPeer {
  /// Server-issued ID — comes from the home Directory's federation registry.
  final String peerId;

  /// Base URL of the peer Directory. May be unreachable from this client.
  final String endpoint;

  /// User-visible name (set on the home Directory side).
  final String displayName;

  /// Catalog version last seen for this peer.
  final int catalogVersion;

  /// Last successful sync time. `null` if never synced.
  final DateTime? lastSyncAt;

  /// Liveness as observed by the home Directory.
  final FederationPeerStatus status;

  /// Per-peer permissions for the current user.
  final PeerPermissionSnapshot permissions;

  const FederationPeer({
    required this.peerId,
    required this.endpoint,
    required this.displayName,
    required this.catalogVersion,
    required this.status,
    required this.permissions,
    this.lastSyncAt,
  });

  FederationPeer copyWith({
    String? peerId,
    String? endpoint,
    String? displayName,
    int? catalogVersion,
    DateTime? lastSyncAt,
    FederationPeerStatus? status,
    PeerPermissionSnapshot? permissions,
  }) {
    return FederationPeer(
      peerId: peerId ?? this.peerId,
      endpoint: endpoint ?? this.endpoint,
      displayName: displayName ?? this.displayName,
      catalogVersion: catalogVersion ?? this.catalogVersion,
      lastSyncAt: lastSyncAt ?? this.lastSyncAt,
      status: status ?? this.status,
      permissions: permissions ?? this.permissions,
    );
  }

  Map<String, dynamic> toJson() => {
        'peer_id': peerId,
        'endpoint': endpoint,
        'display_name': displayName,
        'catalog_version': catalogVersion,
        'last_sync_at': lastSyncAt?.toUtc().toIso8601String(),
        'status': _statusToWire(status),
        'permissions': permissions.toJson(),
      };

  factory FederationPeer.fromJson(Map<String, dynamic> json) {
    return FederationPeer(
      peerId: json['peer_id'] as String,
      endpoint: json['endpoint'] as String,
      displayName: json['display_name'] as String,
      catalogVersion: (json['catalog_version'] as num).toInt(),
      lastSyncAt: json['last_sync_at'] == null
          ? null
          : DateTime.parse(json['last_sync_at'] as String),
      status: _statusFromWire(json['status'] as String),
      permissions: PeerPermissionSnapshot.fromJson(
        Map<String, dynamic>.from(json['permissions'] as Map),
      ),
    );
  }

  @override
  bool operator ==(Object other) =>
      identical(this, other) ||
      (other is FederationPeer &&
          other.peerId == peerId &&
          other.endpoint == endpoint &&
          other.displayName == displayName &&
          other.catalogVersion == catalogVersion &&
          other.lastSyncAt == lastSyncAt &&
          other.status == status &&
          other.permissions == permissions);

  @override
  int get hashCode => Object.hash(
        peerId,
        endpoint,
        displayName,
        catalogVersion,
        lastSyncAt,
        status,
        permissions,
      );

  @override
  String toString() =>
      'FederationPeer(id: $peerId, status: $status, version: $catalogVersion)';
}
