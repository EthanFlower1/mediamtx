import 'package:flutter/material.dart';
import '../../theme/nvr_colors.dart';

class StatusBadge extends StatelessWidget {
  const StatusBadge({
    super.key,
    required this.label,
    required this.color,
    this.showDot = true,
  });

  final String label;
  final Color color;
  final bool showDot;

  static StatusBadge online(BuildContext context) =>
      StatusBadge(label: 'ONLINE', color: NvrColors.of(context).success);
  static StatusBadge offline(BuildContext context) =>
      StatusBadge(label: 'OFFLINE', color: NvrColors.of(context).danger);
  static StatusBadge degraded(BuildContext context) =>
      StatusBadge(label: 'DEGRADED', color: NvrColors.of(context).warning);
  static StatusBadge live(BuildContext context) =>
      StatusBadge(label: 'LIVE', color: NvrColors.of(context).success);
  static StatusBadge recording(BuildContext context) =>
      StatusBadge(label: 'REC', color: NvrColors.of(context).danger, showDot: false);
  static StatusBadge motion(BuildContext context) =>
      StatusBadge(label: 'MOTION', color: NvrColors.of(context).accent);

  @override
  Widget build(BuildContext context) {
    return Container(
      padding: const EdgeInsets.symmetric(horizontal: 8, vertical: 3),
      decoration: BoxDecoration(
        color: color.withOpacity(0.07),
        border: Border.all(color: color.withOpacity(0.27)),
        borderRadius: BorderRadius.circular(4),
      ),
      child: Row(
        mainAxisSize: MainAxisSize.min,
        children: [
          if (showDot) ...[
            Container(
              width: 6, height: 6,
              decoration: BoxDecoration(
                shape: BoxShape.circle,
                color: color,
                boxShadow: [BoxShadow(color: color.withOpacity(0.5), blurRadius: 6)],
              ),
            ),
            const SizedBox(width: 5),
          ],
          Text(
            label,
            style: TextStyle(
              fontFamily: 'JetBrainsMono',
              fontSize: 9,
              fontWeight: FontWeight.w500,
              letterSpacing: 0.5,
              color: color,
            ),
          ),
        ],
      ),
    );
  }
}
