import 'package:flutter/material.dart';
import 'package:flutter_riverpod/flutter_riverpod.dart';
import '../../theme/nvr_colors.dart';
import '../../providers/tours_provider.dart';

class TourActivePill extends ConsumerWidget {
  const TourActivePill({super.key});

  @override
  Widget build(BuildContext context, WidgetRef ref) {
    final tourState = ref.watch(activeTourProvider);
    if (!tourState.isActive) return const SizedBox.shrink();

    return Positioned(
      top: 12, right: 12,
      child: Container(
        padding: const EdgeInsets.symmetric(horizontal: 12, vertical: 8),
        decoration: BoxDecoration(
          color: NvrColors.bgSecondary,
          border: Border.all(color: NvrColors.accent.withOpacity(0.5)),
          borderRadius: BorderRadius.circular(20),
          boxShadow: [BoxShadow(color: NvrColors.accent.withOpacity(0.15), blurRadius: 12)],
        ),
        child: Row(
          mainAxisSize: MainAxisSize.min,
          children: [
            Icon(Icons.refresh, size: 14, color: NvrColors.accent),
            const SizedBox(width: 6),
            Text(
              tourState.tour!.name,
              style: const TextStyle(
                fontFamily: 'JetBrainsMono', fontSize: 10,
                color: NvrColors.textPrimary, letterSpacing: 0.5,
              ),
            ),
            if (tourState.isPaused) ...[
              const SizedBox(width: 6),
              Text('PAUSED', style: TextStyle(
                fontFamily: 'JetBrainsMono', fontSize: 8,
                color: NvrColors.warning, letterSpacing: 1,
              )),
            ],
            const SizedBox(width: 8),
            GestureDetector(
              onTap: () => ref.read(activeTourProvider.notifier).stop(),
              child: Container(
                padding: const EdgeInsets.all(3),
                decoration: BoxDecoration(
                  shape: BoxShape.circle,
                  color: NvrColors.danger.withOpacity(0.13),
                  border: Border.all(color: NvrColors.danger.withOpacity(0.27)),
                ),
                child: const Icon(Icons.stop, size: 10, color: NvrColors.danger),
              ),
            ),
          ],
        ),
      ),
    );
  }
}
