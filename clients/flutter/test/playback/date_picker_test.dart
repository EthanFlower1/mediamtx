// KAI-302 — TimelineDatePicker widget tests.
import 'package:flutter/material.dart';
import 'package:flutter_test/flutter_test.dart';
import 'package:nvr_client/playback/widgets/timeline_date_picker.dart';

void main() {
  testWidgets('tapping the child opens the date picker and emits a range',
      (tester) async {
    DateTimeRange? got;
    final initial = DateTime(2026, 4, 8);
    await tester.pumpWidget(MaterialApp(
      home: Scaffold(
        body: TimelineDatePicker(
          initialDate: initial,
          firstDate: DateTime(2025, 1, 1),
          lastDate: DateTime(2026, 12, 31),
          onRangeSelected: (r) => got = r,
          child: const Text('pick'),
        ),
      ),
    ));

    await tester.tap(find.text('pick'));
    await tester.pumpAndSettle();

    // Confirm the initial selection via default OK label.
    final okFinder = find.text('OK');
    expect(okFinder, findsOneWidget);
    await tester.tap(okFinder);
    await tester.pumpAndSettle();

    expect(got, isNotNull);
    expect(got!.start, initial);
    expect(got!.end, initial.add(const Duration(days: 1)));
  });
}
