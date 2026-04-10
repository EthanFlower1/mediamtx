import 'package:flutter/material.dart';
import 'package:flutter_test/flutter_test.dart';

import 'package:mediamtx/accessibility/a11y_semantics.dart';

void main() {
  group('A11ySemantics', () {
    testWidgets('withLabel creates Semantics node with label', (tester) async {
      await tester.pumpWidget(
        MaterialApp(
          home: A11ySemantics.withLabel(
            const Text('Hello'),
            'greeting text',
          ),
        ),
      );

      final semantics = tester.getSemantics(find.text('Hello'));
      expect(semantics.label, 'greeting text');
    });

    testWidgets('button has button flag set to true', (tester) async {
      await tester.pumpWidget(
        MaterialApp(
          home: A11ySemantics.button(
            child: const Text('Press'),
            label: 'press button',
          ),
        ),
      );

      final semantics = tester.getSemantics(find.text('Press'));
      expect(semantics.hasFlag(SemanticsFlag.isButton), isTrue);
    });

    testWidgets('image has image flag set to true', (tester) async {
      await tester.pumpWidget(
        MaterialApp(
          home: A11ySemantics.image(
            const Icon(Icons.camera),
            'camera icon',
          ),
        ),
      );

      final semantics = tester.getSemantics(find.byIcon(Icons.camera));
      expect(semantics.hasFlag(SemanticsFlag.isImage), isTrue);
      expect(semantics.label, 'camera icon');
    });

    testWidgets('excludeDecorative wraps child in ExcludeSemantics',
        (tester) async {
      await tester.pumpWidget(
        MaterialApp(
          home: A11ySemantics.excludeDecorative(
            const Text('decorative'),
          ),
        ),
      );

      // The ExcludeSemantics widget should be present in the tree.
      expect(find.byType(ExcludeSemantics), findsOneWidget);
    });

    testWidgets('liveRegion has liveRegion flag', (tester) async {
      await tester.pumpWidget(
        MaterialApp(
          home: A11ySemantics.liveRegion(
            const Text('status'),
            'connection status',
          ),
        ),
      );

      final semantics = tester.getSemantics(find.text('status'));
      expect(semantics.hasFlag(SemanticsFlag.isLiveRegion), isTrue);
    });
  });
}
