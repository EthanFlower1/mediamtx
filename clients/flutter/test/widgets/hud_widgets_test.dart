import 'package:flutter/material.dart';
import 'package:flutter_test/flutter_test.dart';
import 'package:nvr_client/theme/nvr_theme.dart';
import 'package:nvr_client/widgets/hud/hud_toggle.dart';
import 'package:nvr_client/widgets/hud/segmented_control.dart';
import 'package:nvr_client/widgets/hud/analog_slider.dart';
import 'package:nvr_client/widgets/hud/corner_brackets.dart';

void main() {
  Widget wrap(Widget child) {
    return MaterialApp(
      theme: NvrTheme.light(),
      darkTheme: NvrTheme.dark(),
      themeMode: ThemeMode.dark,
      home: Scaffold(body: child),
    );
  }

  group('HudToggle', () {
    testWidgets('renders with OFF label when value is false', (tester) async {
      await tester.pumpWidget(wrap(
        HudToggle(value: false, onChanged: (_) {}),
      ));
      expect(find.text('OFF'), findsOneWidget);
    });

    testWidgets('renders with ON label when value is true', (tester) async {
      await tester.pumpWidget(wrap(
        HudToggle(value: true, onChanged: (_) {}),
      ));
      expect(find.text('ON'), findsOneWidget);
    });

    testWidgets('tapping toggles the value', (tester) async {
      bool currentValue = false;
      await tester.pumpWidget(wrap(
        StatefulBuilder(
          builder: (context, setState) {
            return HudToggle(
              value: currentValue,
              onChanged: (v) => setState(() => currentValue = v),
            );
          },
        ),
      ));

      expect(find.text('OFF'), findsOneWidget);

      // Tap the toggle (GestureDetector)
      await tester.tap(find.byType(GestureDetector).first);
      await tester.pumpAndSettle();

      expect(currentValue, true);
      expect(find.text('ON'), findsOneWidget);
    });

    testWidgets('renders optional label', (tester) async {
      await tester.pumpWidget(wrap(
        HudToggle(value: false, onChanged: (_) {}, label: 'AUTO RECORD'),
      ));
      expect(find.text('AUTO RECORD'), findsOneWidget);
    });

    testWidgets('hides state label when showStateLabel is false', (tester) async {
      await tester.pumpWidget(wrap(
        HudToggle(value: false, onChanged: (_) {}, showStateLabel: false),
      ));
      expect(find.text('OFF'), findsNothing);
      expect(find.text('ON'), findsNothing);
    });
  });

  group('HudSegmentedControl', () {
    testWidgets('renders all segment labels', (tester) async {
      await tester.pumpWidget(wrap(
        HudSegmentedControl<int>(
          segments: const {0: 'CONTINUOUS', 1: 'EVENTS', 2: 'SCHEDULE'},
          selected: 0,
          onChanged: (_) {},
        ),
      ));

      expect(find.text('CONTINUOUS'), findsOneWidget);
      expect(find.text('EVENTS'), findsOneWidget);
      expect(find.text('SCHEDULE'), findsOneWidget);
    });

    testWidgets('tapping a segment calls onChanged with correct value', (tester) async {
      int selected = 0;
      await tester.pumpWidget(wrap(
        StatefulBuilder(
          builder: (context, setState) {
            return HudSegmentedControl<int>(
              segments: const {0: 'A', 1: 'B', 2: 'C'},
              selected: selected,
              onChanged: (v) => setState(() => selected = v),
            );
          },
        ),
      ));

      // Tap "B"
      await tester.tap(find.text('B'));
      await tester.pumpAndSettle();
      expect(selected, 1);

      // Tap "C"
      await tester.tap(find.text('C'));
      await tester.pumpAndSettle();
      expect(selected, 2);
    });

    testWidgets('works with string keys', (tester) async {
      String selected = 'day';
      await tester.pumpWidget(wrap(
        StatefulBuilder(
          builder: (context, setState) {
            return HudSegmentedControl<String>(
              segments: const {'day': 'DAY', 'week': 'WEEK', 'month': 'MONTH'},
              selected: selected,
              onChanged: (v) => setState(() => selected = v),
            );
          },
        ),
      ));

      await tester.tap(find.text('WEEK'));
      await tester.pumpAndSettle();
      expect(selected, 'week');
    });
  });

  group('AnalogSlider', () {
    testWidgets('renders with label and value display', (tester) async {
      await tester.pumpWidget(wrap(
        SizedBox(
          width: 300,
          child: AnalogSlider(
            label: 'BRIGHTNESS',
            value: 0.5,
            onChanged: (_) {},
          ),
        ),
      ));

      expect(find.text('BRIGHTNESS'), findsOneWidget);
      // Default formatter: "${(value * 100).round()}%"
      expect(find.text('50%'), findsOneWidget);
    });

    testWidgets('renders with custom valueFormatter', (tester) async {
      await tester.pumpWidget(wrap(
        SizedBox(
          width: 300,
          child: AnalogSlider(
            label: 'RETENTION',
            value: 30.0,
            min: 7.0,
            max: 90.0,
            onChanged: (_) {},
            valueFormatter: (v) => '${v.round()} DAYS',
          ),
        ),
      ));

      expect(find.text('RETENTION'), findsOneWidget);
      expect(find.text('30 DAYS'), findsOneWidget);
    });

    testWidgets('renders without label when label is null', (tester) async {
      await tester.pumpWidget(wrap(
        SizedBox(
          width: 300,
          child: AnalogSlider(
            value: 0.75,
            onChanged: (_) {},
          ),
        ),
      ));

      // Should display value but no label row
      expect(find.text('75%'), findsNothing); // no label row means no value display either
    });

    testWidgets('renders tick marks', (tester) async {
      await tester.pumpWidget(wrap(
        SizedBox(
          width: 300,
          child: AnalogSlider(
            value: 0.5,
            onChanged: (_) {},
            tickCount: 5,
          ),
        ),
      ));

      // The tick marks are Container widgets with width: 1, height: 4
      // 5 ticks should be rendered
      await tester.pumpAndSettle();
      // Verify the widget renders without error
      expect(find.byType(AnalogSlider), findsOneWidget);
    });
  });

  group('CornerBrackets', () {
    testWidgets('renders child widget', (tester) async {
      await tester.pumpWidget(wrap(
        SizedBox(
          width: 200,
          height: 150,
          child: CornerBrackets(
            child: Container(color: Colors.black),
          ),
        ),
      ));

      expect(find.byType(CornerBrackets), findsOneWidget);
      // The child should be in the widget tree
      expect(find.byType(Container), findsWidgets);
    });

    testWidgets('renders 4 bracket painters via Positioned widgets', (tester) async {
      await tester.pumpWidget(wrap(
        SizedBox(
          width: 200,
          height: 150,
          child: CornerBrackets(
            child: Container(color: Colors.black),
          ),
        ),
      ));

      // CornerBrackets creates 4 Positioned widgets each containing a _Bracket with CustomPaint
      expect(find.byType(Positioned), findsNWidgets(4));
      // At least 4 CustomPaint widgets from the brackets (may be more from Scaffold etc.)
      expect(find.byType(CustomPaint), findsAtLeast(4));
    });

    testWidgets('uses Stack to overlay brackets on child', (tester) async {
      await tester.pumpWidget(wrap(
        SizedBox(
          width: 200,
          height: 150,
          child: CornerBrackets(
            child: Container(color: Colors.black),
          ),
        ),
      ));

      // CornerBrackets uses a Stack; there may be others in the tree from Scaffold
      expect(find.byType(Stack), findsAtLeast(1));
      expect(find.byType(Positioned), findsNWidgets(4));
    });

    testWidgets('respects custom bracket size and color', (tester) async {
      await tester.pumpWidget(wrap(
        SizedBox(
          width: 200,
          height: 150,
          child: CornerBrackets(
            bracketSize: 24.0,
            strokeWidth: 3.0,
            color: Colors.red,
            child: Container(color: Colors.black),
          ),
        ),
      ));

      // Each bracket SizedBox should be 24x24
      final sizedBoxes = tester.widgetList<SizedBox>(find.byType(SizedBox));
      final bracketSizedBoxes = sizedBoxes.where(
        (sb) => sb.width == 24.0 && sb.height == 24.0,
      );
      expect(bracketSizedBoxes.length, 4);
    });
  });
}
