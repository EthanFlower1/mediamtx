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

  factory StatusBadge.online() => const StatusBadge(label: 'ONLINE', color: NvrColors.success);
  factory StatusBadge.offline() => const StatusBadge(label: 'OFFLINE', color: NvrColors.danger);
  factory StatusBadge.degraded() => const StatusBadge(label: 'DEGRADED', color: NvrColors.warning);
  factory StatusBadge.live() => const StatusBadge(label: 'LIVE', color: NvrColors.success);
  factory StatusBadge.recording() => const StatusBadge(label: 'REC', color: NvrColors.danger, showDot: false);
  factory StatusBadge.motion() => const StatusBadge(label: 'MOTION', color: NvrColors.accent);

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
