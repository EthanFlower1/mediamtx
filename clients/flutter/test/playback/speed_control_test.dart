// KAI-302 — SpeedControl widget tests.
import 'package:flutter/material.dart';
import 'package:flutter_test/flutter_test.dart';
import 'package:nvr_client/playback/widgets/speed_control.dart';

void main() {
  testWidgets('renders all four speed segments', (tester) async {
    await tester.pumpWidget(MaterialApp(
      home: Scaffold(
        body: SpeedControl(selected: 1.0, onSpeedChanged: (_) {}),
      ),
    ));

    expect(find.text('1x'), findsOneWidget);
    expect(find.text('2x'), findsOneWidget);
    expect(find.text('4x'), findsOneWidget);
    expect(find.text('8x'), findsOneWidget);
  });

  testWidgets('tapping a segment emits onSpeedChanged', (tester) async {
    double? got;
    await tester.pumpWidget(MaterialApp(
      home: Scaffold(
        body: SpeedControl(selected: 1.0, onSpeedChanged: (s) => got = s),
      ),
    ));

    await tester.tap(find.text('4x'));
    await tester.pump();

    expect(got, 4.0);
  });
}
