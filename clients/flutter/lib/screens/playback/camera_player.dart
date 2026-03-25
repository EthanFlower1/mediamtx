import 'package:flutter/material.dart';
import 'package:video_player/video_player.dart';

import '../../theme/nvr_colors.dart';

class CameraPlayer extends StatelessWidget {
  final VideoPlayerController videoController;
  final String cameraName;

  const CameraPlayer({
    super.key,
    required this.videoController,
    required this.cameraName,
  });

  @override
  Widget build(BuildContext context) {
    return Stack(
      fit: StackFit.expand,
      children: [
        ColoredBox(
          color: Colors.black,
          child: videoController.value.isInitialized
              ? FittedBox(
                  fit: BoxFit.contain,
                  child: SizedBox(
                    width: videoController.value.size.width,
                    height: videoController.value.size.height,
                    child: VideoPlayer(videoController),
                  ),
                )
              : const Center(
                  child: CircularProgressIndicator(color: NvrColors.accent),
                ),
        ),
        Positioned(
          top: 8,
          left: 8,
          child: Container(
            padding: const EdgeInsets.symmetric(horizontal: 8, vertical: 4),
            decoration: BoxDecoration(
              color: Colors.black54,
              borderRadius: BorderRadius.circular(4),
            ),
            child: Text(
              cameraName,
              style: const TextStyle(
                color: NvrColors.textPrimary,
                fontSize: 12,
                fontWeight: FontWeight.w500,
              ),
            ),
          ),
        ),
      ],
    );
  }
}
