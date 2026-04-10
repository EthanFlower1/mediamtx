// KAI-302 — BookmarkButton tests.
import 'package:flutter/material.dart';
import 'package:flutter_test/flutter_test.dart';
import 'package:nvr_client/playback/playback_client.dart';
import 'package:nvr_client/playback/playback_strings.dart';
import 'package:nvr_client/playback/widgets/bookmark_button.dart';

void main() {
  testWidgets('tapping save calls createBookmark and shows toast',
      (tester) async {
    final client = FakePlaybackClient();

    await tester.pumpWidget(MaterialApp(
      home: Scaffold(
        body: BookmarkButton(
          client: client,
          segmentId: 'seg-1',
          atMs: 1234,
        ),
      ),
    ));

    await tester.tap(find.byType(BookmarkButton));
    await tester.pumpAndSettle();

    // Dialog open — type a note.
    await tester.enterText(find.byType(TextField), 'incident');
    await tester.tap(find.text(PlaybackStrings.en.bookmarkDialogSave));
    await tester.pumpAndSettle();

    expect(client.lastCall, 'createBookmark');
    expect(client.lastArgs?['segmentId'], 'seg-1');
    expect(client.lastArgs?['atMs'], 1234);
    expect(client.lastArgs?['note'], 'incident');
    expect(find.text(PlaybackStrings.en.bookmarkCreatedToast), findsOneWidget);
  });

  testWidgets('failWith shows the failure toast', (tester) async {
    final client = FakePlaybackClient()..failWith = StateError('nope');

    await tester.pumpWidget(MaterialApp(
      home: Scaffold(
        body: BookmarkButton(
          client: client,
          segmentId: 'seg-1',
          atMs: 0,
        ),
      ),
    ));

    await tester.tap(find.byType(BookmarkButton));
    await tester.pumpAndSettle();
    await tester.tap(find.text(PlaybackStrings.en.bookmarkDialogSave));
    await tester.pumpAndSettle();

    expect(find.text(PlaybackStrings.en.bookmarkFailedToast), findsOneWidget);
  });

  testWidgets('cancel button closes dialog without calling client',
      (tester) async {
    final client = FakePlaybackClient();

    await tester.pumpWidget(MaterialApp(
      home: Scaffold(
        body: BookmarkButton(
          client: client,
          segmentId: 'seg-1',
          atMs: 0,
        ),
      ),
    ));

    await tester.tap(find.byType(BookmarkButton));
    await tester.pumpAndSettle();
    await tester.tap(find.text(PlaybackStrings.en.bookmarkDialogCancel));
    await tester.pumpAndSettle();

    expect(client.lastCall, isNull);
  });
}
