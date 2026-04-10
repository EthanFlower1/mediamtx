// KAI-302 — ClipExportButton tests.
import 'package:flutter/material.dart';
import 'package:flutter_test/flutter_test.dart';
import 'package:nvr_client/playback/playback_client.dart';
import 'package:nvr_client/playback/playback_strings.dart';
import 'package:nvr_client/playback/widgets/clip_export_button.dart';

void main() {
  testWidgets('export flow calls exportClip with the selected range',
      (tester) async {
    final client = FakePlaybackClient();

    await tester.pumpWidget(MaterialApp(
      home: Scaffold(
        body: ClipExportButton(
          client: client,
          segmentId: 'seg-42',
          startMs: 2000,
          endMs: 9000,
        ),
      ),
    ));

    await tester.tap(find.byType(ClipExportButton));
    await tester.pumpAndSettle();

    await tester.enterText(find.byType(TextField), 'suspect');
    await tester.tap(find.text(PlaybackStrings.en.clipExportDialogConfirm));
    await tester.pumpAndSettle();

    expect(client.lastCall, 'exportClip');
    expect(client.lastArgs?['segmentId'], 'seg-42');
    expect(client.lastArgs?['startMs'], 2000);
    expect(client.lastArgs?['endMs'], 9000);
    expect(client.lastArgs?['note'], 'suspect');
    expect(find.text(PlaybackStrings.en.clipExportQueuedToast), findsOneWidget);
  });

  testWidgets('failure shows the failure toast', (tester) async {
    final client = FakePlaybackClient()..failWith = StateError('fail');

    await tester.pumpWidget(MaterialApp(
      home: Scaffold(
        body: ClipExportButton(
          client: client,
          segmentId: 'seg-1',
          startMs: 0,
          endMs: 1000,
        ),
      ),
    ));

    await tester.tap(find.byType(ClipExportButton));
    await tester.pumpAndSettle();
    await tester.tap(find.text(PlaybackStrings.en.clipExportDialogConfirm));
    await tester.pumpAndSettle();

    expect(find.text(PlaybackStrings.en.clipExportFailedToast), findsOneWidget);
  });
}
