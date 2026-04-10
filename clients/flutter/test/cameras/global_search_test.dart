// KAI-299 — Debounced global search + tree filter tests.

import 'package:flutter/material.dart';
import 'package:flutter_test/flutter_test.dart';
import 'package:nvr_client/cameras/camera_strings.dart';
import 'package:nvr_client/cameras/site_tree.dart';
import 'package:nvr_client/cameras/widgets/global_search_bar.dart';
import 'package:nvr_client/models/camera.dart';

Camera _cam(String id, String name, {String site = ''}) => Camera(
      id: id,
      name: name,
      storagePath: site.isEmpty ? '' : 'site/$site',
    );

void main() {
  group('SiteTree.search', () {
    final tree = SiteTree.fromCameras(
      homeCameras: [
        _cam('c1', 'Lobby Entrance', site: 'Lobby'),
        _cam('c2', 'Back Dock', site: 'Warehouse'),
      ],
      peerCameras: const {},
      peers: const [],
      homeLabel: 'Home',
      unassignedSiteLabel: 'Unassigned',
    );

    test('camera-name filter', () {
      expect(tree.search('dock').totalCameraCount, 1);
    });

    test('site-label filter', () {
      expect(tree.search('Lobby').totalCameraCount, 1);
    });

    test('no match yields empty', () {
      expect(tree.search('zzz').totalCameraCount, 0);
    });
  });

  group('GlobalSearchBar', () {
    testWidgets('debounces and emits trimmed query', (tester) async {
      final emissions = <String>[];
      await tester.pumpWidget(
        MaterialApp(
          home: Scaffold(
            body: GlobalSearchBar(
              strings: CameraStrings.en,
              debounce: const Duration(milliseconds: 50),
              onQuery: emissions.add,
            ),
          ),
        ),
      );

      await tester.enterText(find.byType(TextField), 'door');
      // Before debounce elapses — no emission yet.
      expect(emissions, isEmpty);
      await tester.pump(const Duration(milliseconds: 60));
      expect(emissions, ['door']);
    });
  });
}
