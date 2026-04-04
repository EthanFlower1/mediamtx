import 'package:flutter/material.dart';
import '../../theme/nvr_colors.dart';

class HudSegmentedControl<T> extends StatelessWidget {
  const HudSegmentedControl({
    super.key,
    required this.segments,
    required this.selected,
    required this.onChanged,
  });

  final Map<T, String> segments;
  final T selected;
  final ValueChanged<T> onChanged;

  @override
  Widget build(BuildContext context) {
    final entries = segments.entries.toList();
    return Container(
      decoration: BoxDecoration(
        color: NvrColors.of(context).bgPrimary,
        border: Border.all(color: NvrColors.of(context).border),
        borderRadius: BorderRadius.circular(4),
      ),
      child: Row(
        mainAxisSize: MainAxisSize.min,
        children: [
          for (int i = 0; i < entries.length; i++) ...[
            if (i > 0) Container(width: 1, height: 24, color: NvrColors.of(context).border),
            GestureDetector(
              onTap: () => onChanged(entries[i].key),
              child: Container(
                padding: const EdgeInsets.symmetric(horizontal: 10, vertical: 5),
                color: entries[i].key == selected ? NvrColors.of(context).accent.withOpacity(0.13) : Colors.transparent,
                child: Text(
                  entries[i].value,
                  style: TextStyle(
                    fontFamily: 'JetBrainsMono',
                    fontSize: 9,
                    color: entries[i].key == selected ? NvrColors.of(context).accent : NvrColors.of(context).textMuted,
                  ),
                ),
              ),
            ),
          ],
        ],
      ),
    );
  }
}
