import 'package:flutter/material.dart';
import 'package:flutter_riverpod/flutter_riverpod.dart';
import '../../theme/nvr_colors.dart';
import '../../theme/nvr_typography.dart';
import '../../providers/camera_panel_provider.dart';
import '../../providers/cameras_provider.dart';
import '../../providers/groups_provider.dart';
import 'camera_panel_groups.dart';
import 'camera_panel_tours.dart';

class CameraPanel extends ConsumerWidget {
  const CameraPanel({super.key});

  @override
  Widget build(BuildContext context, WidgetRef ref) {
    final panelState = ref.watch(cameraPanelProvider);
    final camerasAsync = ref.watch(camerasProvider);
    final groupsAsync = ref.watch(groupsProvider);

    // Determine active group name for the filter badge
    final activeGroupName = panelState.activeGroupId == null
        ? null
        : groupsAsync.valueOrNull
            ?.where((g) => g.id == panelState.activeGroupId)
            .map((g) => g.name)
            .firstOrNull;

    return Container(
      width: 230,
      color: const Color(0xFF0e0e0e),
      child: Column(
        children: [
          // ── Header ──────────────────────────────────────────────────────
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
                  child: const Icon(Icons.close,
                      size: 16, color: NvrColors.textMuted),
                ),
              ],
            ),
          ),

          // ── Search bar ──────────────────────────────────────────────────
          Padding(
            padding: const EdgeInsets.all(10),
            child: TextField(
              onChanged: (q) =>
                  ref.read(cameraPanelProvider.notifier).setSearch(q),
              style: const TextStyle(
                  fontSize: 12, color: NvrColors.textPrimary),
              decoration: InputDecoration(
                hintText: 'Search cameras...',
                prefixIcon: const Icon(Icons.search,
                    size: 16, color: NvrColors.textMuted),
                isDense: true,
                contentPadding:
                    const EdgeInsets.symmetric(horizontal: 10, vertical: 8),
                filled: true,
                fillColor: NvrColors.bgTertiary,
                border: OutlineInputBorder(
                  borderRadius: BorderRadius.circular(6),
                  borderSide: const BorderSide(color: NvrColors.border),
                ),
              ),
            ),
          ),

          // ── Active group filter badge ────────────────────────────────────
          if (activeGroupName != null)
            Padding(
              padding:
                  const EdgeInsets.symmetric(horizontal: 10, vertical: 4),
              child: Row(
                children: [
                  Container(
                    padding: const EdgeInsets.symmetric(
                        horizontal: 8, vertical: 3),
                    decoration: BoxDecoration(
                      color: NvrColors.accent.withOpacity(0.08),
                      borderRadius: BorderRadius.circular(4),
                      border: Border.all(
                          color: NvrColors.accent.withOpacity(0.3)),
                    ),
                    child: Row(
                      mainAxisSize: MainAxisSize.min,
                      children: [
                        const Icon(Icons.folder_outlined,
                            size: 11, color: NvrColors.accent),
                        const SizedBox(width: 4),
                        Text(
                          activeGroupName.toUpperCase(),
                          style: NvrTypography.monoLabel
                              .copyWith(color: NvrColors.accent),
                        ),
                      ],
                    ),
                  ),
                  const SizedBox(width: 6),
                  GestureDetector(
                    onTap: () => ref
                        .read(cameraPanelProvider.notifier)
                        .setGroupFilter(panelState.activeGroupId),
                    child: const Icon(Icons.close,
                        size: 13, color: NvrColors.textMuted),
                  ),
                ],
              ),
            ),

          // ── Groups section ──────────────────────────────────────────────
          const CameraPanelGroups(),

          // ── Camera list ─────────────────────────────────────────────────
          Expanded(
            child: camerasAsync.when(
              data: (cameras) {
                // Apply search filter
                var filtered = panelState.searchQuery.isEmpty
                    ? cameras
                    : cameras
                        .where((c) => c.name
                            .toLowerCase()
                            .contains(panelState.searchQuery.toLowerCase()))
                        .toList();

                // Apply group filter
                if (panelState.activeGroupId != null) {
                  final group = groupsAsync.valueOrNull?.where(
                      (g) => g.id == panelState.activeGroupId).firstOrNull;
                  if (group != null) {
                    filtered = filtered
                        .where((c) => group.cameraIds.contains(c.id))
                        .toList();
                  }
                }

                if (filtered.isEmpty) {
                  return Center(
                      child: Text('No cameras found',
                          style: NvrTypography.body));
                }

                return ListView.builder(
                  padding: const EdgeInsets.symmetric(horizontal: 10),
                  itemCount: filtered.length,
                  itemBuilder: (context, index) {
                    final cam = filtered[index];
                    final isOnline = cam.status == 'online';
                    return Padding(
                      padding: const EdgeInsets.only(bottom: 3),
                      child: LongPressDraggable<String>(
                        data: cam.id,
                        feedback: Material(
                          color: Colors.transparent,
                          child: Opacity(
                            opacity: 0.85,
                            child: Container(
                              width: 180,
                              padding: const EdgeInsets.symmetric(
                                  horizontal: 10, vertical: 8),
                              decoration: BoxDecoration(
                                color: NvrColors.bgSecondary,
                                border: Border.all(color: NvrColors.accent),
                                borderRadius: BorderRadius.circular(6),
                                boxShadow: [
                                  BoxShadow(
                                    color: NvrColors.accent.withOpacity(0.2),
                                    blurRadius: 12,
                                  ),
                                ],
                              ),
                              child: Row(
                                mainAxisSize: MainAxisSize.min,
                                children: [
                                  Container(
                                    width: 6,
                                    height: 6,
                                    decoration: BoxDecoration(
                                      shape: BoxShape.circle,
                                      color: isOnline
                                          ? NvrColors.success
                                          : NvrColors.danger,
                                    ),
                                  ),
                                  const SizedBox(width: 8),
                                  Flexible(
                                    child: Text(
                                      cam.name,
                                      style: const TextStyle(
                                        fontSize: 11,
                                        fontWeight: FontWeight.w500,
                                        color: NvrColors.textPrimary,
                                        decoration: TextDecoration.none,
                                      ),
                                      overflow: TextOverflow.ellipsis,
                                    ),
                                  ),
                                ],
                              ),
                            ),
                          ),
                        ),
                        childWhenDragging: Opacity(
                          opacity: 0.3,
                          child: Container(
                            padding: const EdgeInsets.symmetric(
                                horizontal: 8, vertical: 6),
                            decoration: BoxDecoration(
                              borderRadius: BorderRadius.circular(6),
                              color: NvrColors.bgTertiary,
                              border: Border.all(color: NvrColors.border),
                            ),
                            child: Row(
                              children: [
                                const SizedBox(width: 6 + 8 + 44 + 8),
                                Expanded(
                                  child: Text(cam.name,
                                      style: const TextStyle(
                                          fontSize: 11,
                                          color: NvrColors.textMuted)),
                                ),
                              ],
                            ),
                          ),
                        ),
                        child: Container(
                          padding: const EdgeInsets.symmetric(
                              horizontal: 8, vertical: 6),
                          decoration: BoxDecoration(
                            borderRadius: BorderRadius.circular(6),
                            color: NvrColors.bgTertiary,
                            border: Border.all(color: NvrColors.border),
                          ),
                          child: Row(
                            children: [
                              Container(
                                width: 6,
                                height: 6,
                                decoration: BoxDecoration(
                                  shape: BoxShape.circle,
                                  color: isOnline
                                      ? NvrColors.success
                                      : NvrColors.danger,
                                  boxShadow: isOnline
                                      ? [
                                          BoxShadow(
                                            color: NvrColors.success
                                                .withOpacity(0.5),
                                            blurRadius: 4,
                                          )
                                        ]
                                      : null,
                                ),
                              ),
                              const SizedBox(width: 8),
                              // Thumbnail placeholder
                              Container(
                                width: 44,
                                height: 26,
                                decoration: BoxDecoration(
                                  color: NvrColors.border,
                                  borderRadius: BorderRadius.circular(3),
                                ),
                              ),
                              const SizedBox(width: 8),
                              Expanded(
                                child: Column(
                                  crossAxisAlignment:
                                      CrossAxisAlignment.start,
                                  children: [
                                    Text(
                                      cam.name,
                                      style: const TextStyle(
                                        fontSize: 11,
                                        fontWeight: FontWeight.w500,
                                        color: NvrColors.textPrimary,
                                      ),
                                      overflow: TextOverflow.ellipsis,
                                    ),
                                    Text(
                                      cam.id.substring(0, 8).toUpperCase(),
                                      style: const TextStyle(
                                        fontFamily: 'JetBrainsMono',
                                        fontSize: 8,
                                        color: NvrColors.textMuted,
                                      ),
                                    ),
                                  ],
                                ),
                              ),
                              const Icon(Icons.drag_handle,
                                  size: 14, color: NvrColors.border),
                            ],
                          ),
                        ),
                      ),
                    );
                  },
                );
              },
              loading: () => const Center(
                  child: CircularProgressIndicator(
                      color: NvrColors.accent)),
              error: (e, _) => Center(
                  child: Text('Error loading cameras',
                      style: NvrTypography.alert)),
            ),
          ),

          // ── Tours section ────────────────────────────────────────────────
          const CameraPanelTours(),
        ],
      ),
    );
  }
}
