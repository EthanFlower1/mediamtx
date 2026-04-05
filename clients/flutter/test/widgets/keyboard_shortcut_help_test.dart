import 'package:flutter/material.dart';
import 'package:flutter_test/flutter_test.dart';
import 'package:nvr_client/theme/nvr_theme.dart';
import 'package:nvr_client/widgets/keyboard_shortcut_help.dart';

void main() {
  Widget wrap(Widget child) {
    return MaterialApp(
      theme: NvrTheme.light(),
      darkTheme: NvrTheme.dark(),
      themeMode: ThemeMode.dark,
      home: Scaffold(body: child),
    );
  }

  group('KeyboardShortcutHelpOverlay', () {
    testWidgets('builds without errors', (tester) async {
      await tester.pumpWidget(wrap(
        KeyboardShortcutHelpOverlay(onClose: () {}),
      ));

      expect(find.byType(KeyboardShortcutHelpOverlay), findsOneWidget);
    });

    testWidgets('displays KEYBOARD SHORTCUTS header', (tester) async {
      await tester.pumpWidget(wrap(
        KeyboardShortcutHelpOverlay(onClose: () {}),
      ));

      expect(find.text('KEYBOARD SHORTCUTS'), findsOneWidget);
    });

    testWidgets('displays all shortcut group titles', (tester) async {
      await tester.pumpWidget(wrap(
        KeyboardShortcutHelpOverlay(onClose: () {}),
      ));

      for (final group in allShortcuts) {
        expect(find.text(group.title), findsOneWidget);
      }
    });

    testWidgets('displays shortcut descriptions', (tester) async {
      await tester.pumpWidget(wrap(
        KeyboardShortcutHelpOverlay(onClose: () {}),
      ));

      // Check a few well-known shortcuts.
      expect(find.text('Toggle shortcut help'), findsOneWidget);
      expect(find.text('Play / pause'), findsOneWidget);
    });

    testWidgets('displays close instruction in footer', (tester) async {
      await tester.pumpWidget(wrap(
        KeyboardShortcutHelpOverlay(onClose: () {}),
      ));

      expect(find.text(' or click outside to close'), findsOneWidget);
    });

    testWidgets('tapping outside calls onClose', (tester) async {
      bool closed = false;
      await tester.pumpWidget(wrap(
        KeyboardShortcutHelpOverlay(onClose: () => closed = true),
      ));

      // Tap the outer GestureDetector (the backdrop).
      // The backdrop is the outermost container with Colors.black54.
      await tester.tapAt(const Offset(10, 10));
      await tester.pump();

      expect(closed, isTrue);
    });

    testWidgets('tapping inside the card does not call onClose', (tester) async {
      bool closed = false;
      await tester.pumpWidget(wrap(
        KeyboardShortcutHelpOverlay(onClose: () => closed = true),
      ));

      // Tap the center of the screen (where the card is).
      await tester.tap(find.text('KEYBOARD SHORTCUTS'));
      await tester.pump();

      expect(closed, isFalse);
    });
  });

  group('allShortcuts data', () {
    test('contains at least 4 groups', () {
      expect(allShortcuts.length, greaterThanOrEqualTo(4));
    });

    test('each group has a non-empty title and at least one entry', () {
      for (final group in allShortcuts) {
        expect(group.title, isNotEmpty);
        expect(group.entries, isNotEmpty);
      }
    });

    test('each entry has non-empty keys and description', () {
      for (final group in allShortcuts) {
        for (final entry in group.entries) {
          expect(entry.keys, isNotEmpty);
          expect(entry.description, isNotEmpty);
        }
      }
    });
  });
}
