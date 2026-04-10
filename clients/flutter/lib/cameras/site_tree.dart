// KAI-299 — Federated camera tree data model.
//
// Pure-data layer. No Flutter imports. Deterministic grouping so tests can
// assert exact shapes.
//
// Grouping contract (two levels deep):
//
//     Root
//     ├── Peer "Home"           (home directory connection)
//     │   ├── Site "Warehouse"
//     │   │   ├── Camera ...
//     │   │   └── Camera ...
//     │   └── Site "Lobby"
//     │       └── Camera ...
//     └── Peer "Acme Corp"       (federated peer)
//         └── Site "Main Office"
//             └── Camera ...
//
// NOTE on Camera.siteLabel:
// KAI-301's [Camera] model (see lib/models/camera.dart) does NOT currently
// carry a `siteLabel` field. Rather than mutate that shared model we take a
// `siteLabelResolver` callback so callers can derive the site label however
// they like (e.g. from a server-side tag, from the camera name prefix, or
// from a future Camera.siteLabel field). This keeps KAI-299 decoupled from
// the KAI-301 schema and the eventual list-cameras proto ask.

import '../models/camera.dart';
import '../state/federation_peer.dart';

/// Callback that extracts a site label from a [Camera]. Return an empty
/// string for "unassigned" — the tree will group those under a single
/// "Unassigned site" node.
typedef SiteLabelResolver = String Function(Camera camera);

/// Default resolver: looks for a `storagePath` prefix of the form `site/foo`,
/// otherwise returns the empty string (unassigned).
String defaultSiteLabelResolver(Camera camera) {
  final path = camera.storagePath;
  if (path.isEmpty) return '';
  final parts = path.split('/');
  if (parts.length >= 2 && parts.first == 'site') return parts[1];
  return '';
}

/// Sentinel peer connection ID used for the home directory branch of the
/// tree. Federated peers use their [FederationPeer.peerId] directly.
const String homePeerConnectionId = '__home__';

/// A node in the [SiteTree]. Nodes come in three shapes:
///
///   - Peer node: children = site nodes, cameras = empty.
///   - Site node: children = empty, cameras = list of cameras.
///   - (Camera rows are leaves rendered from Site nodes' [cameras] list —
///      they are not themselves [SiteNode]s, to keep the model flat.)
class SiteNode {
  final String id;
  final String label;
  final String? siteLabel;
  final String peerConnectionId;
  final List<SiteNode> children;
  final List<Camera> cameras;
  final bool isExpanded;

  const SiteNode({
    required this.id,
    required this.label,
    required this.peerConnectionId,
    this.siteLabel,
    this.children = const [],
    this.cameras = const [],
    this.isExpanded = true,
  });

  SiteNode copyWith({
    String? id,
    String? label,
    String? siteLabel,
    String? peerConnectionId,
    List<SiteNode>? children,
    List<Camera>? cameras,
    bool? isExpanded,
  }) {
    return SiteNode(
      id: id ?? this.id,
      label: label ?? this.label,
      siteLabel: siteLabel ?? this.siteLabel,
      peerConnectionId: peerConnectionId ?? this.peerConnectionId,
      children: children ?? this.children,
      cameras: cameras ?? this.cameras,
      isExpanded: isExpanded ?? this.isExpanded,
    );
  }

  /// Total cameras under this node (recursive).
  int get totalCameraCount {
    var n = cameras.length;
    for (final c in children) {
      n += c.totalCameraCount;
    }
    return n;
  }
}

/// A federated camera tree. Two levels: peer → site → cameras.
///
/// Build via [SiteTree.fromCameras]. The resulting tree is immutable; to
/// filter or search, use [search] / [filterCameras] which return new trees.
class SiteTree {
  /// Peer-level children. Order is deterministic: home first, then
  /// federated peers sorted by displayName (case-insensitive).
  final List<SiteNode> peers;

  const SiteTree({required this.peers});

  static const SiteTree empty = SiteTree(peers: []);

  /// Build a tree from a flat list of home cameras plus per-peer cameras.
  ///
  /// [homeCameras] — cameras from the active home directory connection.
  /// [peerCameras] — cameras grouped by [FederationPeer.peerId]. Any peer in
  ///                 [peers] with no entry in this map gets an empty branch.
  /// [peers]       — known federation peers (from AppSession.knownPeers).
  /// [homeLabel]   — display label for the home branch.
  /// [siteLabelResolver] — extractor for per-camera site labels.
  /// [unassignedSiteLabel] — label used when the resolver returns empty.
  factory SiteTree.fromCameras({
    required List<Camera> homeCameras,
    required Map<String, List<Camera>> peerCameras,
    required List<FederationPeer> peers,
    required String homeLabel,
    required String unassignedSiteLabel,
    SiteLabelResolver siteLabelResolver = defaultSiteLabelResolver,
  }) {
    final peerNodes = <SiteNode>[];

    // Home branch — always first.
    peerNodes.add(_buildPeerNode(
      connectionId: homePeerConnectionId,
      label: homeLabel,
      cameras: homeCameras,
      siteLabelResolver: siteLabelResolver,
      unassignedSiteLabel: unassignedSiteLabel,
    ));

    // Federated peers — sorted by displayName.
    final sortedPeers = [...peers]..sort(
        (a, b) => a.displayName.toLowerCase().compareTo(
              b.displayName.toLowerCase(),
            ),
      );
    for (final peer in sortedPeers) {
      peerNodes.add(_buildPeerNode(
        connectionId: peer.peerId,
        label: peer.displayName,
        cameras: peerCameras[peer.peerId] ?? const [],
        siteLabelResolver: siteLabelResolver,
        unassignedSiteLabel: unassignedSiteLabel,
      ));
    }

    return SiteTree(peers: List.unmodifiable(peerNodes));
  }

  static SiteNode _buildPeerNode({
    required String connectionId,
    required String label,
    required List<Camera> cameras,
    required SiteLabelResolver siteLabelResolver,
    required String unassignedSiteLabel,
  }) {
    // Group by site label.
    final grouped = <String, List<Camera>>{};
    for (final cam in cameras) {
      final raw = siteLabelResolver(cam);
      final key = raw.isEmpty ? '' : raw;
      (grouped[key] ??= <Camera>[]).add(cam);
    }

    // Build site nodes, sorted. Empty-key bucket becomes "Unassigned site"
    // and sorts last.
    final siteKeys = grouped.keys.toList()
      ..sort((a, b) {
        if (a.isEmpty && b.isEmpty) return 0;
        if (a.isEmpty) return 1;
        if (b.isEmpty) return -1;
        return a.toLowerCase().compareTo(b.toLowerCase());
      });
    final siteNodes = <SiteNode>[];
    for (final key in siteKeys) {
      final resolvedLabel = key.isEmpty ? unassignedSiteLabel : key;
      final camerasForSite = [...grouped[key]!]..sort(
          (a, b) => a.name.toLowerCase().compareTo(b.name.toLowerCase()),
        );
      siteNodes.add(SiteNode(
        id: '$connectionId::$key',
        label: resolvedLabel,
        siteLabel: key.isEmpty ? null : key,
        peerConnectionId: connectionId,
        cameras: List.unmodifiable(camerasForSite),
      ));
    }

    return SiteNode(
      id: connectionId,
      label: label,
      peerConnectionId: connectionId,
      children: List.unmodifiable(siteNodes),
    );
  }

  /// Flatten to a list of (peerNode, siteNode, camera) tuples — handy for
  /// search and for count assertions in tests.
  List<FlatCameraEntry> flatten() {
    final out = <FlatCameraEntry>[];
    for (final peer in peers) {
      for (final site in peer.children) {
        for (final cam in site.cameras) {
          out.add(FlatCameraEntry(
            peerNode: peer,
            siteNode: site,
            camera: cam,
          ));
        }
      }
    }
    return out;
  }

  /// Total number of cameras in the tree.
  int get totalCameraCount {
    var n = 0;
    for (final peer in peers) {
      n += peer.totalCameraCount;
    }
    return n;
  }

  /// Return a new [SiteTree] that only contains entries whose
  /// peer label / site label / camera name contains [query] (case-insensitive).
  /// Empty query returns the original tree unchanged.
  SiteTree search(String query) {
    final q = query.trim().toLowerCase();
    if (q.isEmpty) return this;

    final filteredPeers = <SiteNode>[];
    for (final peer in peers) {
      final peerMatches = peer.label.toLowerCase().contains(q);

      final filteredSites = <SiteNode>[];
      for (final site in peer.children) {
        final siteMatches = site.label.toLowerCase().contains(q);

        List<Camera> filteredCams;
        if (peerMatches || siteMatches) {
          // Whole branch matches: keep all cameras under it.
          filteredCams = site.cameras;
        } else {
          filteredCams = site.cameras
              .where((c) => c.name.toLowerCase().contains(q))
              .toList(growable: false);
        }

        if (filteredCams.isNotEmpty) {
          filteredSites.add(site.copyWith(cameras: filteredCams));
        }
      }

      if (filteredSites.isNotEmpty) {
        filteredPeers.add(peer.copyWith(children: filteredSites));
      }
    }

    return SiteTree(peers: List.unmodifiable(filteredPeers));
  }

  /// Apply an arbitrary camera filter (e.g. permission filter) and rebuild
  /// a shallow copy of the tree. Empty sites and empty peers are pruned.
  SiteTree filterCameras(bool Function(Camera) predicate) {
    final filteredPeers = <SiteNode>[];
    for (final peer in peers) {
      final filteredSites = <SiteNode>[];
      for (final site in peer.children) {
        final cams =
            site.cameras.where(predicate).toList(growable: false);
        if (cams.isNotEmpty) {
          filteredSites.add(site.copyWith(cameras: cams));
        }
      }
      if (filteredSites.isNotEmpty) {
        filteredPeers.add(peer.copyWith(children: filteredSites));
      }
    }
    return SiteTree(peers: List.unmodifiable(filteredPeers));
  }
}

/// A single (peer, site, camera) entry produced by [SiteTree.flatten].
class FlatCameraEntry {
  final SiteNode peerNode;
  final SiteNode siteNode;
  final Camera camera;

  const FlatCameraEntry({
    required this.peerNode,
    required this.siteNode,
    required this.camera,
  });
}
