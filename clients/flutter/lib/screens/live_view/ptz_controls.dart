import 'package:flutter/material.dart';

import '../../services/api_client.dart';
import '../../theme/nvr_colors.dart';

class PtzControls extends StatelessWidget {
  final ApiClient apiClient;
  final String cameraId;

  const PtzControls({
    super.key,
    required this.apiClient,
    required this.cameraId,
  });

  Future<void> _sendPtz(String action) async {
    try {
      await apiClient.post(
        '/cameras/$cameraId/ptz',
        data: {'action': action},
      );
    } catch (_) {
      // PTZ failures are fire-and-forget; ignore silently
    }
  }

  @override
  Widget build(BuildContext context) {
    return Container(
      padding: const EdgeInsets.all(12),
      decoration: BoxDecoration(
        color: Colors.black.withValues(alpha: 0.6),
        borderRadius: BorderRadius.circular(12),
      ),
      child: Column(
        mainAxisSize: MainAxisSize.min,
        children: [
          // Up
          _PtzButton(
            icon: Icons.keyboard_arrow_up,
            tooltip: 'Up',
            onPressed: () => _sendPtz('up'),
          ),
          const SizedBox(height: 4),
          // Left / Stop / Right row
          Row(
            mainAxisSize: MainAxisSize.min,
            children: [
              _PtzButton(
                icon: Icons.keyboard_arrow_left,
                tooltip: 'Left',
                onPressed: () => _sendPtz('left'),
              ),
              const SizedBox(width: 4),
              _PtzButton(
                icon: Icons.stop,
                tooltip: 'Stop',
                onPressed: () => _sendPtz('stop'),
                small: true,
              ),
              const SizedBox(width: 4),
              _PtzButton(
                icon: Icons.keyboard_arrow_right,
                tooltip: 'Right',
                onPressed: () => _sendPtz('right'),
              ),
            ],
          ),
          const SizedBox(height: 4),
          // Down
          _PtzButton(
            icon: Icons.keyboard_arrow_down,
            tooltip: 'Down',
            onPressed: () => _sendPtz('down'),
          ),
          const SizedBox(height: 8),
          // Zoom row
          Row(
            mainAxisSize: MainAxisSize.min,
            children: [
              _PtzButton(
                icon: Icons.zoom_in,
                tooltip: 'Zoom In',
                onPressed: () => _sendPtz('zoom_in'),
              ),
              const SizedBox(width: 8),
              _PtzButton(
                icon: Icons.zoom_out,
                tooltip: 'Zoom Out',
                onPressed: () => _sendPtz('zoom_out'),
              ),
            ],
          ),
        ],
      ),
    );
  }
}

class _PtzButton extends StatelessWidget {
  final IconData icon;
  final String tooltip;
  final VoidCallback onPressed;
  final bool small;

  const _PtzButton({
    required this.icon,
    required this.tooltip,
    required this.onPressed,
    this.small = false,
  });

  @override
  Widget build(BuildContext context) {
    final size = small ? 36.0 : 44.0;
    final iconSize = small ? 18.0 : 22.0;
    return Tooltip(
      message: tooltip,
      child: Material(
        color: NvrColors.bgTertiary.withValues(alpha: 0.85),
        borderRadius: BorderRadius.circular(8),
        child: InkWell(
          onTap: onPressed,
          borderRadius: BorderRadius.circular(8),
          child: SizedBox(
            width: size,
            height: size,
            child: Icon(icon, color: NvrColors.textPrimary, size: iconSize),
          ),
        ),
      ),
    );
  }
}
