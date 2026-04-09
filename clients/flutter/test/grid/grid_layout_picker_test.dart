// KAI-301 — GridLayoutPicker widget tests.

import 'package:flutter/material.dart';
import 'package:flutter_test/flutter_test.dart';

import 'package:nvr_client/grid/grid_layout_picker.dart';
import 'package:nvr_client/grid/grid_strings.dart';

void main() {
  testWidgets('tapping 3x3 segment fires onLayoutChanged', (tester) async {
    GridLayout current = GridLayout.twoByTwo;
    bool alwaysLive = false;

    await tester.pumpWidget(MaterialApp(
      home: Scaffold(
        body: StatefulBuilder(builder: (context, setState) {
          return GridLayoutPicker(
            selected: current,
            alwaysLive: alwaysLive,
            onLayoutChanged: (l) => setState(() => current = l),
            onAlwaysLiveChanged: (v) => setState(() => alwaysLive = v),
            strings: GridStrings.en,
          );
        }),
      ),
    ));

    expect(current, GridLayout.twoByTwo);
    await tester.tap(find.text(GridStrings.en.layoutThreeByThree));
    await tester.pumpAndSettle();
    expect(current, GridLayout.threeByThree);

    // Always-live switch flips.
    await tester.tap(find.byType(Switch));
    await tester.pumpAndSettle();
    expect(alwaysLive, true);
  });

  test('GridLayoutExt.crossAxisCount + maxCells', () {
    expect(GridLayout.twoByTwo.crossAxisCount, 2);
    expect(GridLayout.twoByTwo.maxCells, 4);
    expect(GridLayout.threeByThree.crossAxisCount, 3);
    expect(GridLayout.threeByThree.maxCells, 9);
    expect(GridLayout.fourByFour.crossAxisCount, 4);
    expect(GridLayout.fourByFour.maxCells, 16);
    expect(GridLayout.auto.crossAxisCount, 2);
  });
}
