// KAI-301 — Grid layout picker.
//
// Material SegmentedButton + Always-Live switch. Takes a [GridStrings] so the
// widget tests can inject an English instance directly without spinning up
// the MaterialApp localization stack.

import 'package:flutter/material.dart';

import 'grid_strings.dart';

/// Supported grid layouts.
///
/// `auto` is the default state — the screen picks a layout based on available
/// space. The picker UI itself never selects `auto`; it's a system-chosen
/// value that gets replaced the moment the user taps 2x2/3x3/4x4.
enum GridLayout {
  auto,
  twoByTwo,
  threeByThree,
  fourByFour,
}

extension GridLayoutExt on GridLayout {
  int get crossAxisCount {
    switch (this) {
      case GridLayout.twoByTwo:
        return 2;
      case GridLayout.threeByThree:
        return 3;
      case GridLayout.fourByFour:
        return 4;
      case GridLayout.auto:
        return 2; // auto defaults to 2x2 until the chooser picks otherwise
    }
  }

  int get maxCells => crossAxisCount * crossAxisCount;
}

class GridLayoutPicker extends StatelessWidget {
  final GridLayout selected;
  final bool alwaysLive;
  final ValueChanged<GridLayout> onLayoutChanged;
  final ValueChanged<bool> onAlwaysLiveChanged;
  final GridStrings strings;

  const GridLayoutPicker({
    super.key,
    required this.selected,
    required this.alwaysLive,
    required this.onLayoutChanged,
    required this.onAlwaysLiveChanged,
    required this.strings,
  });

  @override
  Widget build(BuildContext context) {
    // The SegmentedButton can't represent `auto`, so we mirror it as 2x2 in
    // the UI state. Any tap sends a concrete layout back to the caller.
    final uiSelected =
        selected == GridLayout.auto ? GridLayout.twoByTwo : selected;

    return Row(
      mainAxisSize: MainAxisSize.min,
      children: [
        SegmentedButton<GridLayout>(
          segments: <ButtonSegment<GridLayout>>[
            ButtonSegment<GridLayout>(
              value: GridLayout.twoByTwo,
              label: Text(strings.layoutTwoByTwo),
            ),
            ButtonSegment<GridLayout>(
              value: GridLayout.threeByThree,
              label: Text(strings.layoutThreeByThree),
            ),
            ButtonSegment<GridLayout>(
              value: GridLayout.fourByFour,
              label: Text(strings.layoutFourByFour),
            ),
          ],
          selected: {uiSelected},
          onSelectionChanged: (Set<GridLayout> set) {
            if (set.isEmpty) return;
            onLayoutChanged(set.first);
          },
        ),
        const SizedBox(width: 16),
        Tooltip(
          message: strings.alwaysLiveTooltip,
          child: Row(
            mainAxisSize: MainAxisSize.min,
            children: [
              Text(strings.alwaysLiveLabel),
              const SizedBox(width: 4),
              Switch(
                value: alwaysLive,
                onChanged: onAlwaysLiveChanged,
              ),
            ],
          ),
        ),
      ],
    );
  }
}
