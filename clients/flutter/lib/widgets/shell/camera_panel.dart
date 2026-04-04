import 'package:flutter/material.dart';
import 'package:flutter_riverpod/flutter_riverpod.dart';
import '../../theme/nvr_colors.dart';
import '../../theme/nvr_typography.dart';
import '../../providers/auth_provider.dart';
import '../../providers/camera_panel_provider.dart';
import '../../providers/cameras_provider.dart';
import '../../providers/groups_provider.dart';
import '../hud/camera_thumbnail.dart';
import 'camera_panel_groups.dart';
import 'camera_panel_tours.dart';

class CameraPanel extends ConsumerWidget {
  const CameraPanel({super.key});

  @override
  Widget build(BuildContext context, WidgetRef ref) {
    final panelState = ref.watch(cameraPanelProvider);
    final camerasAsync = ref.watch(camerasProvider);
    final groupsAsync = ref.watch(groupsProvider);
    final serverUrl = ref.watch(authProvider).serverUrl ?? '';

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
            decoration: BoxDecoration(
              border: Border(bottom: BorderSide(color: NvrColors.of(context).border)),
            ),
            child: Row(
              children: [
                Text('CAMERAS', style: NvrTypography.of(context).monoSection),
                const Spacer(),
                GestureDetector(
                  onTap: () => ref.read(cameraPanelProvider.notifier).close(),
                  child: Icon(Icons.close,
                      size: 16, color: NvrColors.of(context).textMuted),
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
              style: TextStyle(
                  fontSize: 12, color: NvrColors.of(context).textPrimary),
              decoration: InputDecoration(
                hintText: 'Search cameras...',
                prefixIcon: Icon(Icons.search,
                    size: 16, color: NvrColors.of(context).textMuted),
                isDense: true,
                contentPadding:
                    const EdgeInsets.symmetric(horizontal: 10, vertical: 8),
                filled: true,
                fillColor: NvrColors.of(context).bgTertiary,
                border: OutlineInputBorder(
                  borderRadius: BorderRadius.circular(6),
                  borderSide: BorderSide(color: NvrColors.of(context).border),
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
                      color: NvrColors.of(context).accent.withOpacity(0.08),
                      borderRadius: BorderRadius.circular(4),
                      border: Border.all(
                          color: NvrColors.of(context).accent.withOpacity(0.3)),
                    ),
                    child: Row(
                      mainAxisSize: MainAxisSize.min,
                      children: [
                        Icon(Icons.folder_outlined,
                            size: 11, color: NvrColors.of(context).accent),
                        const SizedBox(width: 4),
                        Text(
                          activeGroupName.toUpperCase(),
                          style: NvrTypography.of(context).monoLabel
                              .copyWith(color: NvrColors.of(context).accent),
                        ),
                      ],
                    ),
                  ),
                  const SizedBox(width: 6),
                  GestureDetector(
                    onTap: () => ref
                        .read(cameraPanelProvider.notifier)
                        .setGroupFilter(panelState.activeGroupId),
                    child: Icon(Icons.close,
                        size: 13, color: NvrColors.of(context).textMuted),
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
                          style: NvrTypography.of(context).body));
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
                                color: NvrColors.of(context).bgSecondary,
                                border: Border.all(color: NvrColors.of(context).accent),
                                borderRadius: BorderRadius.circular(6),
                                boxShadow: [
                                  BoxShadow(
                                    color: NvrColors.of(context).accent.withOpacity(0.2),
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
                                          ? NvrColors.of(context).success
                                          : NvrColors.of(context).danger,
                                    ),
                                  ),
                                  const SizedBox(width: 8),
                                  Flexible(
                                    child: Text(
                                      cam.name,
                                      style: TextStyle(
                                        fontSize: 11,
                                        fontWeight: FontWeight.w500,
                                        color: NvrColors.of(context).textPrimary,
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
                              color: NvrColors.of(context).bgTertiary,
                              border: Border.all(color: NvrColors.of(context).border),
                            ),
                            child: Row(
                              children: [
                                const SizedBox(width: 6 + 8 + 44 + 8),
                                Expanded(
                                  child: Text(cam.name,
                                      style: TextStyle(
                                          fontSize: 11,
                                          color: NvrColors.of(context).textMuted)),
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
                            color: NvrColors.of(context).bgTertiary,
                            border: Border.all(color: NvrColors.of(context).border),
                          ),
                          child: Row(
                            children: [
                              Container(
                                width: 6,
                                height: 6,
                                decoration: BoxDecoration(
                                  shape: BoxShape.circle,
                                  color: isOnline
                                      ? NvrColors.of(context).success
                                      : NvrColors.of(context).danger,
                                  boxShadow: isOnline
                                      ? [
                                          BoxShadow(
                                            color: NvrColors.of(context).success
                                                .withOpacity(0.5),
                                            blurRadius: 4,
                                          )
                                        ]
                                      : null,
                                ),
                              ),
                              const SizedBox(width: 8),
                              // Camera thumbnail
                              CameraThumbnail(
                                serverUrl: serverUrl,
                                cameraId: cam.id,
                                width: 44,
                                height: 26,
                              ),
                              const SizedBox(width: 8),
                              Expanded(
                                child: Column(
                                  crossAxisAlignment:
                                      CrossAxisAlignment.start,
                                  children: [
                                    Text(
                                      cam.name,
                                      style: TextStyle(
                                        fontSize: 11,
                                        fontWeight: FontWeight.w500,
                                        color: NvrColors.of(context).textPrimary,
                                      ),
                                      overflow: TextOverflow.ellipsis,
                                    ),
                                    Text(
                                      cam.id.substring(0, 8).toUpperCase(),
                                      style: TextStyle(
                                        fontFamily: 'JetBrainsMono',
                                        fontSize: 8,
                                        color: NvrColors.of(context).textMuted,
                                      ),
                                    ),
                                  ],
                                ),
                              ),
                              Icon(Icons.drag_handle,
                                  size: 14, color: NvrColors.of(context).border),
                            ],
                          ),
                        ),
                      ),
                    );
                  },
                );
              },
              loading: () => Center(
                  child: CircularProgressIndicator(
                      color: NvrColors.of(context).accent)),
              error: (e, _) => Center(
                  child: Text('Error loading cameras',
                      style: NvrTypography.of(context).alert)),
            ),
          ),

          // ── Tours section ────────────────────────────────────────────────
          const CameraPanelTours(),
        ],
      ),
    );
  }
}
