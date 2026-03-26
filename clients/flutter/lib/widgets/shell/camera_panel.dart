import 'package:flutter/material.dart';
import 'package:flutter_riverpod/flutter_riverpod.dart';
import '../../theme/nvr_colors.dart';
import '../../theme/nvr_typography.dart';
import '../../providers/camera_panel_provider.dart';
import '../../providers/cameras_provider.dart';
import '../hud/status_badge.dart';

class CameraPanel extends ConsumerWidget {
  const CameraPanel({super.key});

  @override
  Widget build(BuildContext context, WidgetRef ref) {
    final panelState = ref.watch(cameraPanelProvider);
    final camerasAsync = ref.watch(camerasProvider);

    return Container(
      width: 230,
      color: const Color(0xFF0e0e0e),
      child: Column(
        children: [
          // Header
          Container(
            padding: const EdgeInsets.fromLTRB(16, 14, 16, 10),
            decoration: const BoxDecoration(
              border: Border(bottom: BorderSide(color: NvrColors.border)),
            ),
            child: Row(
              children: [
                Text('CAMERAS', style: NvrTypography.monoSection),
                const Spacer(),
                GestureDetector(
                  onTap: () => ref.read(cameraPanelProvider.notifier).close(),
                  child: const Icon(Icons.close, size: 16, color: NvrColors.textMuted),
                ),
              ],
            ),
          ),
          // Search
          Padding(
            padding: const EdgeInsets.all(10),
            child: TextField(
              onChanged: (q) => ref.read(cameraPanelProvider.notifier).setSearch(q),
              style: const TextStyle(fontSize: 12, color: NvrColors.textPrimary),
              decoration: InputDecoration(
                hintText: 'Search cameras...',
                prefixIcon: const Icon(Icons.search, size: 16, color: NvrColors.textMuted),
                isDense: true,
                contentPadding: const EdgeInsets.symmetric(horizontal: 10, vertical: 8),
                filled: true,
                fillColor: NvrColors.bgTertiary,
                border: OutlineInputBorder(
                  borderRadius: BorderRadius.circular(6),
                  borderSide: const BorderSide(color: NvrColors.border),
                ),
              ),
            ),
          ),
          // Camera list
          Expanded(
            child: camerasAsync.when(
              data: (cameras) {
                final filtered = panelState.searchQuery.isEmpty
                    ? cameras
                    : cameras.where((c) => c.name.toLowerCase().contains(panelState.searchQuery.toLowerCase())).toList();

                if (filtered.isEmpty) {
                  return Center(child: Text('No cameras found', style: NvrTypography.body));
                }

                return ListView.builder(
                  padding: const EdgeInsets.symmetric(horizontal: 10),
                  itemCount: filtered.length,
                  itemBuilder: (context, index) {
                    final cam = filtered[index];
                    final isOnline = cam.status == 'online';
                    return Padding(
                      padding: const EdgeInsets.only(bottom: 3),
                      child: Container(
                        padding: const EdgeInsets.symmetric(horizontal: 8, vertical: 6),
                        decoration: BoxDecoration(
                          borderRadius: BorderRadius.circular(6),
                          color: NvrColors.bgTertiary,
                          border: Border.all(color: NvrColors.border),
                        ),
                        child: Row(
                          children: [
                            Container(
                              width: 6, height: 6,
                              decoration: BoxDecoration(
                                shape: BoxShape.circle,
                                color: isOnline ? NvrColors.success : NvrColors.danger,
                                boxShadow: isOnline ? [
                                  BoxShadow(color: NvrColors.success.withOpacity(0.5), blurRadius: 4),
                                ] : null,
                              ),
                            ),
                            const SizedBox(width: 8),
                            // Thumbnail placeholder
                            Container(
                              width: 44, height: 26,
                              decoration: BoxDecoration(
                                color: NvrColors.border,
                                borderRadius: BorderRadius.circular(3),
                              ),
                            ),
                            const SizedBox(width: 8),
                            Expanded(
                              child: Column(
                                crossAxisAlignment: CrossAxisAlignment.start,
                                children: [
                                  Text(cam.name, style: const TextStyle(
                                    fontSize: 11, fontWeight: FontWeight.w500,
                                    color: NvrColors.textPrimary,
                                  ), overflow: TextOverflow.ellipsis),
                                  Text(cam.id.substring(0, 8).toUpperCase(), style: TextStyle(
                                    fontFamily: 'JetBrainsMono', fontSize: 8,
                                    color: NvrColors.textMuted,
                                  )),
                                ],
                              ),
                            ),
                            const Icon(Icons.drag_handle, size: 14, color: NvrColors.border),
                          ],
                        ),
                      ),
                    );
                  },
                );
              },
              loading: () => const Center(child: CircularProgressIndicator(color: NvrColors.accent)),
              error: (e, _) => Center(child: Text('Error loading cameras', style: NvrTypography.alert)),
            ),
          ),
        ],
      ),
    );
  }
}
