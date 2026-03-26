import 'package:flutter/material.dart';
import '../../theme/nvr_colors.dart';

/// Loads a camera snapshot thumbnail. Shows a placeholder on failure or while loading.
class CameraThumbnail extends StatelessWidget {
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
  Widget build(BuildContext context) {
    if (serverUrl.isEmpty) return _placeholder();

    final url =
        '$serverUrl/api/nvr/vod/thumbnail?camera_id=$cameraId&t=${DateTime.now().millisecondsSinceEpoch ~/ 30000}';

    return ClipRRect(
      borderRadius: BorderRadius.circular(borderRadius),
      child: SizedBox(
        width: width,
        height: height,
        child: Image.network(
          url,
          fit: BoxFit.cover,
          headers: const {},
          errorBuilder: (_, __, ___) => _placeholder(),
          loadingBuilder: (context, child, loadingProgress) {
            if (loadingProgress == null) return child;
            return _placeholder();
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
