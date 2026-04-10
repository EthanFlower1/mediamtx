// KAI-299 — SiteTreeView widget test.
//
// Builds a two-peer tree, expands a site, taps a camera and asserts the
// callback fires with the right camera. Uses CameraStrings.en.

import 'package:flutter/material.dart';
import 'package:flutter_test/flutter_test.dart';
import 'package:nvr_client/cameras/camera_status_notifier.dart';
import 'package:nvr_client/cameras/camera_strings.dart';
import 'package:nvr_client/cameras/site_tree.dart';
import 'package:nvr_client/cameras/widgets/site_tree_view.dart';
import 'package:nvr_client/models/camera.dart';
import 'package:nvr_client/state/app_session.dart';
import 'package:nvr_client/state/federation_peer.dart';

Camera _cam(String id, String name, {String site = ''}) => Camera(
      id: id,
      name: name,
      storagePath: site.isEmpty ? '' : 'site/$site',
    );

FederationPeer _peer(String id, String name) => FederationPeer(
      peerId: id,
      endpoint: 'https://$id.example',
      displayName: name,
      catalogVersion: 1,
      status: FederationPeerStatus.online,
      permissions: const PeerPermissionSnapshot(canListCameras: true),
    );

void main() {
  testWidgets('expands a site and taps a camera', (tester) async {
    final tree = SiteTree.fromCameras(
      homeCameras: [
        _cam('h1', 'Lobby Entrance', site: 'Lobby'),
        _cam('h2', 'Stockroom', site: 'Warehouse'),
      ],
      peerCameras: {
        'peer-b': [_cam('b1', 'Parking', site: 'Lot')],
      },
      peers: [_peer('peer-b', 'Branch')],
      homeLabel: CameraStrings.en.homeDirectoryLabel,
      unassignedSiteLabel: CameraStrings.en.unknownSiteLabel,
    );

    Camera? tapped;

    await tester.pumpWidget(
      MaterialApp(
        home: Scaffold(
          body: SiteTreeView(
            tree: tree,
            statuses: const <String, CameraStatus>{},
            session: AppSession.empty,
            strings: CameraStrings.en,
            onCameraTapped: (c) => tapped = c,
          ),
        ),
      ),
    );

    // Home peer should be visible.
    expect(find.text(CameraStrings.en.homeDirectoryLabel), findsOneWidget);

    // Peers and sites default to expanded, so camera names should be present.
    expect(find.text('Lobby Entrance'), findsOneWidget);
    expect(find.text('Stockroom'), findsOneWidget);

    await tester.tap(find.text('Lobby Entrance'));
    await tester.pumpAndSettle();
    expect(tapped?.id, 'h1');
  });

  testWidgets('empty tree shows empty message', (tester) async {
    await tester.pumpWidget(
      MaterialApp(
        home: Scaffold(
          body: SiteTreeView(
            tree: SiteTree.empty,
            statuses: const <String, CameraStatus>{},
            session: AppSession.empty,
            strings: CameraStrings.en,
          ),
        ),
      ),
    );
    expect(find.text(CameraStrings.en.emptyTreeMessage), findsOneWidget);
  });
}
