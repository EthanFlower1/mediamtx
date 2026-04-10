// KAI-299 — Camera row in the federated site tree.
//
// Shows name + online indicator + optional blurred thumbnail + trailing
// site badge. All user-facing strings come from [CameraStrings].

import 'dart:ui';

import 'package:flutter/material.dart';

import '../../models/camera.dart';
import '../camera_status_notifier.dart';
import '../camera_strings.dart';

class CameraRow extends StatelessWidget {
  final Camera camera;
  final String siteLabel;
  final CameraOnlineState onlineState;
  final bool thumbnailVisible;
  final String? thumbnailUrl;
  final VoidCallback? onTap;
  final CameraStrings strings;

  const CameraRow({
    super.key,
    required this.camera,
    required this.siteLabel,
    required this.onlineState,
    required this.thumbnailVisible,
    required this.strings,
    this.thumbnailUrl,
    this.onTap,
  });

  @override
  Widget build(BuildContext context) {
    return ListTile(
      onTap: onTap,
      leading: _buildThumbnail(context),
      title: Text(camera.name),
      subtitle: Text(_statusLabel()),
      trailing: _buildSiteBadge(context),
    );
  }

  String _statusLabel() {
    switch (onlineState) {
      case CameraOnlineState.online:
        return strings.statusOnline;
      case CameraOnlineState.offline:
        return strings.statusOffline;
      case CameraOnlineState.unknown:
        return strings.statusUnknown;
    }
  }

  Color _statusColor() {
    switch (onlineState) {
      case CameraOnlineState.online:
        return Colors.green;
      case CameraOnlineState.offline:
        return Colors.red;
      case CameraOnlineState.unknown:
        return Colors.grey;
    }
  }

  Widget _buildThumbnail(BuildContext context) {
    final indicator = Positioned(
      right: 0,
      bottom: 0,
      child: Container(
        width: 10,
        height: 10,
        decoration: BoxDecoration(
          color: _statusColor(),
          shape: BoxShape.circle,
          border: Border.all(color: Colors.white, width: 1.5),
        ),
      ),
    );

    Widget body;
    if (thumbnailUrl == null || thumbnailUrl!.isEmpty) {
      body = Container(
        width: 48,
        height: 48,
        color: Theme.of(context).colorScheme.surfaceContainerHighest,
        child: const Icon(Icons.videocam_outlined),
      );
    } else {
      body = Image.network(
        thumbnailUrl!,
        width: 48,
        height: 48,
        fit: BoxFit.cover,
        errorBuilder: (_, __, ___) => Container(
          width: 48,
          height: 48,
          color: Theme.of(context).colorScheme.surfaceContainerHighest,
          child: const Icon(Icons.broken_image_outlined),
        ),
      );
    }

    if (!thumbnailVisible) {
      body = Tooltip(
        message: strings.thumbnailLockedTooltip,
        child: Stack(
          alignment: Alignment.center,
          children: [
            ImageFiltered(
              imageFilter: ImageFilter.blur(sigmaX: 6, sigmaY: 6),
              child: body,
            ),
            const Icon(Icons.lock_outline, size: 20),
          ],
        ),
      );
    }

    return SizedBox(
      width: 48,
      height: 48,
      child: Stack(
        clipBehavior: Clip.none,
        children: [body, indicator],
      ),
    );
  }

  Widget _buildSiteBadge(BuildContext context) {
    return Container(
      padding: const EdgeInsets.symmetric(horizontal: 8, vertical: 4),
      decoration: BoxDecoration(
        color: Theme.of(context).colorScheme.secondaryContainer,
        borderRadius: BorderRadius.circular(12),
      ),
      child: Text(
        siteLabel,
        style: Theme.of(context).textTheme.labelSmall,
      ),
    );
  }
}
