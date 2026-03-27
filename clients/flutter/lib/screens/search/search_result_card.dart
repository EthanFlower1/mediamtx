import 'package:flutter/material.dart';
import 'package:flutter_riverpod/flutter_riverpod.dart';
import '../../models/search_result.dart';
import '../../providers/auth_provider.dart';
import '../../theme/nvr_colors.dart';
import '../../theme/nvr_typography.dart';

class SearchResultCard extends ConsumerWidget {
  final SearchResult result;
  final String? thumbnailBaseUrl;
  final VoidCallback? onTap;

  const SearchResultCard({
    super.key,
    required this.result,
    this.thumbnailBaseUrl,
    this.onTap,
  });

  String _formatTimestamp(DateTime dt) {
    final now = DateTime.now();
    final today = DateTime(now.year, now.month, now.day);
    final yesterday = today.subtract(const Duration(days: 1));
    final dtDay = DateTime(dt.year, dt.month, dt.day);

    final h = dt.hour.toString().padLeft(2, '0');
    final m = dt.minute.toString().padLeft(2, '0');
    final s = dt.second.toString().padLeft(2, '0');
    final timeStr = '$h:$m:$s';

    if (dtDay == today) return 'TODAY · $timeStr';
    if (dtDay == yesterday) return 'YESTERDAY · $timeStr';

    final mo = dt.month.toString().padLeft(2, '0');
    final d = dt.day.toString().padLeft(2, '0');
    return '${dt.year}-$mo-$d · $timeStr';
  }

  @override
  Widget build(BuildContext context, WidgetRef ref) {
    final confidencePct = (result.confidence * 100).round();

    return GestureDetector(
      onTap: onTap,
      child: Container(
        decoration: BoxDecoration(
          color: NvrColors.bgSecondary,
          borderRadius: BorderRadius.circular(6),
          border: Border.all(color: NvrColors.border),
        ),
        clipBehavior: Clip.hardEdge,
        child: Column(
          crossAxisAlignment: CrossAxisAlignment.start,
          children: [
            // Thumbnail — 16:9 aspect ratio
            AspectRatio(
              aspectRatio: 16 / 9,
              child: Stack(
                fit: StackFit.expand,
                children: [
                  _VodThumbnail(
                    serverUrl: thumbnailBaseUrl,
                    cameraId: result.cameraId,
                    frameTime: result.frameTime,
                  ),
                  // Confidence badge — top right
                  Positioned(
                    top: 4,
                    right: 4,
                    child: Container(
                      padding: const EdgeInsets.symmetric(
                          horizontal: 4, vertical: 2),
                      decoration: BoxDecoration(
                        color: NvrColors.accent,
                        borderRadius: BorderRadius.circular(2),
                      ),
                      child: Text(
                        '$confidencePct%',
                        style: const TextStyle(
                          fontFamily: 'JetBrainsMono',
                          fontSize: 8,
                          fontWeight: FontWeight.w700,
                          color: NvrColors.bgPrimary,
                        ),
                      ),
                    ),
                  ),
                ],
              ),
            ),
            // Details row
            Padding(
              padding: const EdgeInsets.fromLTRB(10, 8, 10, 8),
              child: Column(
                crossAxisAlignment: CrossAxisAlignment.start,
                children: [
                  Row(
                    children: [
                      Expanded(
                        child: Text(
                          result.cameraName,
                          style: const TextStyle(
                            fontFamily: 'IBMPlexSans',
                            fontSize: 10,
                            fontWeight: FontWeight.w500,
                            color: NvrColors.textPrimary,
                          ),
                          overflow: TextOverflow.ellipsis,
                        ),
                      ),
                      const SizedBox(width: 6),
                      Text(
                        result.className.toUpperCase(),
                        style: NvrTypography.monoLabel.copyWith(
                          color: NvrColors.accent,
                        ),
                      ),
                    ],
                  ),
                  const SizedBox(height: 4),
                  Text(
                    _formatTimestamp(result.time),
                    style: NvrTypography.monoLabel.copyWith(
                      color: NvrColors.textMuted,
                    ),
                  ),
                ],
              ),
            ),
          ],
        ),
      ),
    );
  }
}

/// Fetches a thumbnail on demand from the VOD endpoint using the detection's
/// camera ID and frame time. Passes JWT token for auth.
class _VodThumbnail extends ConsumerWidget {
  final String? serverUrl;
  final String cameraId;
  final String frameTime;

  const _VodThumbnail({
    required this.serverUrl,
    required this.cameraId,
    required this.frameTime,
  });

  @override
  Widget build(BuildContext context, WidgetRef ref) {
    if (serverUrl == null || serverUrl!.isEmpty || frameTime.isEmpty) {
      return _placeholder();
    }

    final authService = ref.watch(authServiceProvider);

    return FutureBuilder<String?>(
      future: authService.getAccessToken(),
      builder: (context, snapshot) {
        final token = snapshot.data;
        if (token == null) return _placeholder();

        final url =
            '$serverUrl/api/nvr/vod/thumbnail?camera_id=$cameraId&time=$frameTime&token=$token';

        return Image.network(
          url,
          fit: BoxFit.cover,
          errorBuilder: (_, __, ___) => _placeholder(),
          loadingBuilder: (context, child, loadingProgress) {
            if (loadingProgress == null) return child;
            return _placeholder();
          },
        );
      },
    );
  }

  Widget _placeholder() {
    return Container(
      color: NvrColors.bgTertiary,
      child: const Center(
        child: Icon(Icons.videocam_off,
            color: NvrColors.textMuted, size: 24),
      ),
    );
  }
}
