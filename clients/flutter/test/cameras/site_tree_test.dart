// KAI-299 — SiteTree grouping + search + filter tests.

import 'package:flutter_test/flutter_test.dart';
import 'package:nvr_client/cameras/site_tree.dart';
import 'package:nvr_client/models/camera.dart';
import 'package:nvr_client/state/federation_peer.dart';

Camera _cam(String id, {String? name, String storagePath = ''}) => Camera(
      id: id,
      name: name ?? id,
      storagePath: storagePath,
    );

FederationPeer _peer(String id, {String? name}) => FederationPeer(
      peerId: id,
      endpoint: 'https://$id.example',
      displayName: name ?? id,
      catalogVersion: 1,
      status: FederationPeerStatus.online,
      permissions: const PeerPermissionSnapshot(canListCameras: true),
    );

void main() {
  group('SiteTree.fromCameras', () {
    test('single peer (home only) groups by site label prefix', () {
      final tree = SiteTree.fromCameras(
        homeCameras: [
          _cam('cam-1', storagePath: 'site/warehouse'),
          _cam('cam-2', storagePath: 'site/warehouse'),
          _cam('cam-3', storagePath: 'site/lobby'),
          _cam('cam-4'), // unassigned
        ],
        peerCameras: const {},
        peers: const [],
        homeLabel: 'Home',
        unassignedSiteLabel: 'Unassigned',
      );

      expect(tree.peers, hasLength(1));
      final home = tree.peers.first;
      expect(home.label, 'Home');
      expect(home.peerConnectionId, homePeerConnectionId);
      expect(home.children, hasLength(3));
      // lobby < warehouse alpha, unassigned last.
      expect(home.children[0].label, 'lobby');
      expect(home.children[1].label, 'warehouse');
      expect(home.children[2].label, 'Unassigned');
      expect(home.children[1].cameras, hasLength(2));
    });

    test('multi-peer tree: home first, federated sorted by display name', () {
      final tree = SiteTree.fromCameras(
        homeCameras: [_cam('h1', storagePath: 'site/main')],
        peerCameras: {
          'peer-zulu': [_cam('z1', storagePath: 'site/A')],
          'peer-alpha': [_cam('a1', storagePath: 'site/A')],
        },
        peers: [
          _peer('peer-zulu', name: 'Zulu Corp'),
          _peer('peer-alpha', name: 'Alpha Inc'),
        ],
        homeLabel: 'Home',
        unassignedSiteLabel: 'Unassigned',
      );

      expect(tree.peers.map((p) => p.label).toList(),
          ['Home', 'Alpha Inc', 'Zulu Corp']);
    });

    test('empty cameras produces an empty home branch', () {
      final tree = SiteTree.fromCameras(
        homeCameras: const [],
        peerCameras: const {},
        peers: const [],
        homeLabel: 'Home',
        unassignedSiteLabel: 'Unassigned',
      );
      expect(tree.peers, hasLength(1));
      expect(tree.totalCameraCount, 0);
    });

    test('flatten yields every camera once, with peer+site context', () {
      final tree = SiteTree.fromCameras(
        homeCameras: [
          _cam('h1', storagePath: 'site/main'),
          _cam('h2', storagePath: 'site/main'),
        ],
        peerCameras: {
          'peer-b': [_cam('b1', storagePath: 'site/annex')],
        },
        peers: [_peer('peer-b', name: 'Branch')],
        homeLabel: 'Home',
        unassignedSiteLabel: 'Unassigned',
      );

      final flat = tree.flatten();
      expect(flat, hasLength(3));
      expect(flat.map((e) => e.camera.id).toSet(), {'h1', 'h2', 'b1'});
      expect(flat.firstWhere((e) => e.camera.id == 'b1').peerNode.label,
          'Branch');
    });
  });

  group('SiteTree.search', () {
    late SiteTree tree;
    setUp(() {
      tree = SiteTree.fromCameras(
        homeCameras: [
          _cam('h-door', name: 'Front Door', storagePath: 'site/lobby'),
          _cam('h-stock', name: 'Stockroom', storagePath: 'site/warehouse'),
        ],
        peerCameras: {
          'peer-b': [_cam('b-park', name: 'Parking', storagePath: 'site/lot')],
        },
        peers: [_peer('peer-b', name: 'Branch Office')],
        homeLabel: 'Home',
        unassignedSiteLabel: 'Unassigned',
      );
    });

    test('empty query returns identical tree', () {
      expect(tree.search('').totalCameraCount, tree.totalCameraCount);
    });

    test('camera name match', () {
      final r = tree.search('door');
      expect(r.totalCameraCount, 1);
      expect(r.flatten().first.camera.name, 'Front Door');
    });

    test('site label match pulls in whole site', () {
      final r = tree.search('warehouse');
      expect(r.totalCameraCount, 1);
    });

    test('peer label match pulls in whole peer', () {
      final r = tree.search('branch');
      expect(r.totalCameraCount, 1);
      expect(r.flatten().first.camera.id, 'b-park');
    });
  });
}
