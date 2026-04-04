import 'package:flutter/material.dart';
import 'package:flutter_test/flutter_test.dart';
import 'package:nvr_client/theme/nvr_colors.dart';
import 'package:nvr_client/theme/nvr_theme.dart';
import 'package:nvr_client/widgets/hud/status_badge.dart';

void main() {
  Widget wrap(Widget child) {
    return MaterialApp(
      theme: NvrTheme.light(),
      darkTheme: NvrTheme.dark(),
      themeMode: ThemeMode.dark,
      home: Scaffold(body: child),
    );
  }

  /// Build a StatusBadge via a Builder to get access to BuildContext.
  Widget wrapFactory(StatusBadge Function(BuildContext) factory) {
    return wrap(Builder(builder: (context) => factory(context)));
  }

  group('StatusBadge factory constructors', () {
    testWidgets('online() renders "ONLINE" with green color', (tester) async {
      await tester.pumpWidget(wrapFactory(StatusBadge.online));
      expect(find.text('ONLINE'), findsOneWidget);

      final textWidget = tester.widget<Text>(find.text('ONLINE'));
      expect(textWidget.style?.color, NvrColors.dark.success);
    });

    testWidgets('offline() renders "OFFLINE" with red color', (tester) async {
      await tester.pumpWidget(wrapFactory(StatusBadge.offline));
      expect(find.text('OFFLINE'), findsOneWidget);

      final textWidget = tester.widget<Text>(find.text('OFFLINE'));
      expect(textWidget.style?.color, NvrColors.dark.danger);
    });

    testWidgets('degraded() renders "DEGRADED" with warning color', (tester) async {
      await tester.pumpWidget(wrapFactory(StatusBadge.degraded));
      expect(find.text('DEGRADED'), findsOneWidget);

      final textWidget = tester.widget<Text>(find.text('DEGRADED'));
      expect(textWidget.style?.color, NvrColors.dark.warning);
    });

    testWidgets('live() renders "LIVE" with green color', (tester) async {
      await tester.pumpWidget(wrapFactory(StatusBadge.live));
      expect(find.text('LIVE'), findsOneWidget);

      final textWidget = tester.widget<Text>(find.text('LIVE'));
      expect(textWidget.style?.color, NvrColors.dark.success);
    });

    testWidgets('recording() renders "REC" and has showDot false', (tester) async {
      await tester.pumpWidget(wrapFactory(StatusBadge.recording));
      expect(find.text('REC'), findsOneWidget);

      final statusBadge = tester.widget<StatusBadge>(find.byType(StatusBadge));
      expect(statusBadge.showDot, false);
    });

    testWidgets('custom StatusBadge renders the label passed in', (tester) async {
      await tester.pumpWidget(wrap(
        const StatusBadge(label: 'CUSTOM', color: Colors.purple),
      ));
      expect(find.text('CUSTOM'), findsOneWidget);

      final textWidget = tester.widget<Text>(find.text('CUSTOM'));
      expect(textWidget.style?.color, Colors.purple);
    });
  });

  group('StatusBadge dot indicator', () {
    testWidgets('shows dot when showDot is true', (tester) async {
      await tester.pumpWidget(wrapFactory(StatusBadge.online));
      // The Row inside the badge should have 3 children: dot, SizedBox, Text
      final row = tester.widget<Row>(find.byType(Row));
      expect(row.children.length, 3);
    });

    testWidgets('hides dot when showDot is false', (tester) async {
      await tester.pumpWidget(wrapFactory(StatusBadge.recording));
      // With showDot false, the Row should have 1 child: just the Text
      final row = tester.widget<Row>(find.byType(Row));
      expect(row.children.length, 1);
    });
  });
}
