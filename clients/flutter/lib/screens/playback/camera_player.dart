import 'package:flutter/material.dart';
import 'package:media_kit_video/media_kit_video.dart';

import '../../theme/nvr_colors.dart';

class CameraPlayer extends StatelessWidget {
  final VideoController videoController;
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
          child: Video(controller: videoController),
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
