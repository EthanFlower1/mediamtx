// KAI-301 — CameraGridView widget tests.
//
// Verifies that the grid picks the right RenderMode for its cell count +
// always-live flag, by counting the per-mode placeholder keys.

import 'package:flutter/material.dart';
import 'package:flutter_riverpod/flutter_riverpod.dart';
import 'package:flutter_test/flutter_test.dart';

import 'package:nvr_client/grid/camera.dart';
import 'package:nvr_client/grid/grid_layout_picker.dart';
import 'package:nvr_client/grid/grid_strings.dart';
import 'package:nvr_client/grid/grid_view.dart' as grid;
import 'package:nvr_client/grid/stream_url_minter.dart';

Camera _cam(String id) => Camera(
      id: id,
      directoryConnectionId: 'c',
      label: 'Camera $id',
      siteLabel: 'Site',
      snapshotUrl: 'https://fake/$id.jpg',
      mainStreamWebRtcEndpoint: 'https://fake/whep/$id',
      thumbnailUrl: 'https://fake/$id.thumb',
      isOnline: true,
    );

List<Camera> _cams(int n) =>
    List.generate(n, (i) => _cam('c$i'), growable: false);

Widget _host(Widget child) => ProviderScope(
      child: MaterialApp(
        home: Scaffold(body: SizedBox(width: 800, height: 600, child: child)),
      ),
    );

void main() {
  testWidgets('4 cameras off-LAN renders all WebRTC tiles', (tester) async {
    await tester.pumpWidget(_host(grid.CameraGridView(
      cameras: _cams(4),
      layout: GridLayout.twoByTwo,
      alwaysLiveOverride: false,
      isOnLan: false,
      minter: FakeStreamUrlMinter(),
      strings: GridStrings.en,
    )));
    await tester.pump();
    // The WebRTC tile placeholder carries this key regardless of ticket state.
    expect(find.byKey(const Key('webrtc-tile-connecting')), findsNWidgets(4));
  });

  testWidgets('9 cameras off-LAN renders all snapshot tiles', (tester) async {
    // Provide an override snapshot controller so the test can dispose it
    // without waiting for the widget to tear down pending timers.
    final minter = FakeStreamUrlMinter();
    await tester.pumpWidget(_host(grid.CameraGridView(
      cameras: _cams(9),
      layout: GridLayout.threeByThree,
      alwaysLiveOverride: false,
      isOnLan: false,
      minter: minter,
      strings: GridStrings.en,
    )));
    await tester.pump();
    // Snapshot mode → no WebRTC connecting labels.
    expect(find.byKey(const Key('webrtc-tile-connecting')), findsNothing);
    // Every snapshot tile shows its connecting placeholder until a frame
    // arrives — the default grid fetcher returns an empty buffer so the
    // placeholder sticks.
    expect(
      find.byKey(const Key('snapshot-tile-connecting-c0')),
      findsOneWidget,
    );
    // Tear down: pump an empty widget so the grid's State disposes its
    // SnapshotRefreshController + cancels periodic timers.
    await tester.pumpWidget(const SizedBox.shrink());
  });

  testWidgets('always-live override forces WebRTC with 9 cameras',
      (tester) async {
    await tester.pumpWidget(_host(grid.CameraGridView(
      cameras: _cams(9),
      layout: GridLayout.threeByThree,
      alwaysLiveOverride: true,
      isOnLan: false,
      minter: FakeStreamUrlMinter(),
      strings: GridStrings.en,
    )));
    await tester.pump();
    expect(find.byKey(const Key('webrtc-tile-connecting')), findsNWidgets(9));
  });

  testWidgets('empty camera list shows localized placeholder',
      (tester) async {
    await tester.pumpWidget(_host(grid.CameraGridView(
      cameras: const [],
      layout: GridLayout.twoByTwo,
      alwaysLiveOverride: false,
      isOnLan: false,
      minter: FakeStreamUrlMinter(),
      strings: GridStrings.en,
    )));
    await tester.pump();
    expect(find.byKey(const Key('grid-empty')), findsOneWidget);
    expect(find.text(GridStrings.en.noCameras), findsOneWidget);
  });
}
