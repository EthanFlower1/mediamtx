import 'package:flutter/material.dart';
import 'package:flutter_riverpod/flutter_riverpod.dart';
import '../../providers/auth_provider.dart';
import '../../theme/nvr_colors.dart';

/// Loads a camera snapshot thumbnail with JWT auth.
/// Shows a placeholder on failure or while loading.
class CameraThumbnail extends ConsumerWidget {
  const CameraThumbnail({
    super.key,
    required this.serverUrl,
    required this.cameraId,
    this.width,
    this.height,
    this.borderRadius = 3.0,
  });

  final String serverUrl;
  final String cameraId;
  final double? width;
  final double? height;
  final double borderRadius;

  @override
  Widget build(BuildContext context, WidgetRef ref) {
    if (serverUrl.isEmpty) return _placeholder();

    final authService = ref.watch(authServiceProvider);

    return ClipRRect(
      borderRadius: BorderRadius.circular(borderRadius),
      child: SizedBox(
        width: width,
        height: height,
        child: FutureBuilder<String?>(
          future: authService.getAccessToken(),
          builder: (context, snapshot) {
            final token = snapshot.data;
            if (token == null) return _placeholder();

            final url =
                '$serverUrl/api/nvr/vod/thumbnail?camera_id=$cameraId&token=$token&t=${DateTime.now().millisecondsSinceEpoch ~/ 30000}';

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
        ),
      ),
    );
  }

  Widget _placeholder() {
    return Container(
      width: width,
      height: height,
      decoration: BoxDecoration(
        color: NvrColors.border,
        borderRadius: BorderRadius.circular(borderRadius),
      ),
      child: const Icon(Icons.videocam, size: 12, color: NvrColors.textMuted),
    );
  }
}
