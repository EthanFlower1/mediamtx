import 'package:flutter/material.dart';
import '../../theme/nvr_colors.dart';

enum HudButtonStyle { primary, secondary, danger, tactical }

class HudButton extends StatelessWidget {
  const HudButton({
    super.key,
    required this.label,
    required this.onPressed,
    this.style = HudButtonStyle.primary,
    this.icon,
  });

  final String label;
  final VoidCallback? onPressed;
  final HudButtonStyle style;
  final IconData? icon;

  @override
  Widget build(BuildContext context) {
    final (bg, fg, border) = switch (style) {
      HudButtonStyle.primary => (NvrColors.of(context).accent, NvrColors.of(context).bgPrimary, Colors.transparent),
      HudButtonStyle.secondary => (NvrColors.of(context).bgTertiary, NvrColors.of(context).textPrimary, NvrColors.of(context).border),
      HudButtonStyle.danger => (NvrColors.of(context).danger.withOpacity(0.13), NvrColors.of(context).danger, NvrColors.of(context).danger.withOpacity(0.27)),
      HudButtonStyle.tactical => (NvrColors.of(context).bgTertiary, NvrColors.of(context).accent, NvrColors.of(context).accent.withOpacity(0.27)),
    };

    final textStyle = style == HudButtonStyle.tactical
        ? TextStyle(fontFamily: 'JetBrainsMono', fontSize: 10, letterSpacing: 1, fontWeight: FontWeight.w500, color: fg)
        : TextStyle(fontFamily: 'IBMPlexSans', fontSize: 12, fontWeight: FontWeight.w600, color: fg);

    return Material(
      color: bg,
      borderRadius: BorderRadius.circular(4),
      child: InkWell(
        onTap: onPressed,
        borderRadius: BorderRadius.circular(4),
        child: Container(
          padding: const EdgeInsets.symmetric(horizontal: 16, vertical: 8),
          decoration: BoxDecoration(
            border: Border.all(color: border),
            borderRadius: BorderRadius.circular(4),
          ),
          child: Row(
            mainAxisSize: MainAxisSize.min,
            children: [
              if (icon != null) ...[
                Icon(icon, size: 14, color: fg),
                const SizedBox(width: 6),
              ],
              Text(label, style: textStyle),
            ],
          ),
        ),
      ),
    );
  }
}
