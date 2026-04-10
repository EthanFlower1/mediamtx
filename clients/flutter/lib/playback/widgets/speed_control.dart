// KAI-302 — Playback speed control.
//
// SegmentedButton with the four speeds mandated by the ticket:
// 1x / 2x / 4x / 8x. String labels route through PlaybackStrings so i18n
// is ready when flutter_intl lands.

import 'package:flutter/material.dart';

import '../playback_strings.dart';

class SpeedControl extends StatelessWidget {
  static const List<double> speeds = [1.0, 2.0, 4.0, 8.0];

  final double selected;
  final ValueChanged<double> onSpeedChanged;
  final PlaybackStrings strings;

  const SpeedControl({
    super.key,
    required this.selected,
    required this.onSpeedChanged,
    this.strings = PlaybackStrings.en,
  });

  String _label(double s) {
    if (s == 1.0) return strings.speed1x;
    if (s == 2.0) return strings.speed2x;
    if (s == 4.0) return strings.speed4x;
    if (s == 8.0) return strings.speed8x;
    return '${s}x';
  }

  @override
  Widget build(BuildContext context) {
    return SegmentedButton<double>(
      segments: [
        for (final s in speeds)
          ButtonSegment<double>(value: s, label: Text(_label(s))),
      ],
      selected: {speeds.contains(selected) ? selected : 1.0},
      showSelectedIcon: false,
      onSelectionChanged: (set) {
        if (set.isNotEmpty) onSpeedChanged(set.first);
      },
    );
  }
}
