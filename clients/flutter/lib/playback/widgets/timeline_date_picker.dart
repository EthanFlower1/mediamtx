// KAI-302 — Date picker wrapper.
//
// Thin adapter around Material `showDatePicker` that also captures a
// start + end time so the caller gets a full `DateTimeRange`. All
// user-visible strings route through `PlaybackStrings`.

import 'package:flutter/material.dart';

import '../playback_strings.dart';

class TimelineDatePicker extends StatelessWidget {
  final DateTime initialDate;
  final DateTime firstDate;
  final DateTime lastDate;
  final ValueChanged<DateTimeRange> onRangeSelected;
  final PlaybackStrings strings;
  final Widget child;

  const TimelineDatePicker({
    super.key,
    required this.initialDate,
    required this.firstDate,
    required this.lastDate,
    required this.onRangeSelected,
    required this.child,
    this.strings = PlaybackStrings.en,
  });

  Future<void> _open(BuildContext context) async {
    final picked = await showDatePicker(
      context: context,
      initialDate: initialDate,
      firstDate: firstDate,
      lastDate: lastDate,
      helpText: strings.datePickerTitle,
      cancelText: strings.datePickerCancel,
      confirmText: strings.datePickerConfirm,
    );
    if (picked == null) return;
    final start = DateTime(picked.year, picked.month, picked.day);
    final end = start.add(const Duration(days: 1));
    onRangeSelected(DateTimeRange(start: start, end: end));
  }

  @override
  Widget build(BuildContext context) {
    return GestureDetector(
      behavior: HitTestBehavior.opaque,
      onTap: () => _open(context),
      child: child,
    );
  }
}
