import 'package:flutter/material.dart';
import '../../theme/nvr_colors.dart';
import '../../theme/nvr_animations.dart';

class HudToggle extends StatelessWidget {
  const HudToggle({
    super.key,
    required this.value,
    required this.onChanged,
    this.label,
    this.showStateLabel = true,
  });

  final bool value;
  final ValueChanged<bool> onChanged;
  final String? label;
  final bool showStateLabel;

  @override
  Widget build(BuildContext context) {
    return Column(
      crossAxisAlignment: CrossAxisAlignment.start,
      mainAxisSize: MainAxisSize.min,
      children: [
        if (label != null)
          Padding(
            padding: const EdgeInsets.only(bottom: 6),
            child: Text(label!, style: TextStyle(
              fontFamily: 'JetBrainsMono', fontSize: 9,
              letterSpacing: 1, color: NvrColors.textMuted,
            )),
          ),
        GestureDetector(
          onTap: () => onChanged(!value),
          child: AnimatedContainer(
            duration: NvrAnimations.microDuration,
            curve: NvrAnimations.microCurve,
            width: 44, height: 22,
            decoration: BoxDecoration(
              color: NvrColors.bgTertiary,
              borderRadius: BorderRadius.circular(11),
              border: Border.all(
                color: value ? NvrColors.accent : NvrColors.border,
                width: 2,
              ),
              boxShadow: value ? [
                BoxShadow(color: NvrColors.accent.withOpacity(0.2), blurRadius: 8),
              ] : null,
            ),
            child: AnimatedAlign(
              duration: NvrAnimations.microDuration,
              curve: NvrAnimations.microCurve,
              alignment: value ? Alignment.centerRight : Alignment.centerLeft,
              child: Padding(
                padding: const EdgeInsets.all(2),
                child: Container(
                  width: 14, height: 14,
                  decoration: BoxDecoration(
                    shape: BoxShape.circle,
                    color: value ? NvrColors.accent : NvrColors.textMuted,
                    boxShadow: value ? [
                      BoxShadow(color: NvrColors.accent.withOpacity(0.4), blurRadius: 6),
                    ] : null,
                  ),
                ),
              ),
            ),
          ),
        ),
        if (showStateLabel)
          Padding(
            padding: const EdgeInsets.only(top: 4),
            child: Text(
              value ? 'ON' : 'OFF',
              style: TextStyle(
                fontFamily: 'JetBrainsMono', fontSize: 8,
                letterSpacing: 1,
                color: value ? NvrColors.accent : NvrColors.textMuted,
              ),
            ),
          ),
      ],
    );
  }
}
