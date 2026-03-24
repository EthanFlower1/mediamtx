import 'dart:async';

import 'package:flutter/material.dart';
import 'package:flutter_webrtc/flutter_webrtc.dart';

import '../../models/camera.dart';
import '../../services/whep_service.dart';
import '../../theme/nvr_colors.dart';

class CameraTile extends StatefulWidget {
  final Camera camera;
  final String serverUrl;
  final VoidCallback? onTap;

  const CameraTile({
    super.key,
    required this.camera,
    required this.serverUrl,
    this.onTap,
  });

  @override
  State<CameraTile> createState() => _CameraTileState();
}

class _CameraTileState extends State<CameraTile> {
  WhepConnection? _connection;
  WhepConnectionState _connState = WhepConnectionState.connecting;
  StreamSubscription<WhepConnectionState>? _stateSub;

  @override
  void initState() {
    super.initState();
    _initConnection();
  }

  void _initConnection() {
    final camera = widget.camera;
    final isOnline = camera.status == 'connected' || camera.status == 'online';
    if (!isOnline || camera.mediamtxPath.isEmpty) return;

    _connection = WhepConnection(
      serverUrl: widget.serverUrl,
      mediamtxPath: camera.mediamtxPath,
    );

    _stateSub = _connection!.stateStream.listen((state) {
      if (mounted) setState(() => _connState = state);
    });

    _connection!.connect();
  }

  Future<void> _retry() async {
    await _connection?.retry();
  }

  @override
  void dispose() {
    _stateSub?.cancel();
    _connection?.dispose();
    super.dispose();
  }

  @override
  Widget build(BuildContext context) {
    final camera = widget.camera;
    final isOnline = camera.status == 'connected' || camera.status == 'online';

    return GestureDetector(
      onTap: widget.onTap,
      child: ClipRRect(
        borderRadius: BorderRadius.circular(8),
        child: Stack(
          fit: StackFit.expand,
          children: [
            // Video / state layer
            _buildVideoLayer(isOnline),

            // Camera name - bottom left
            Positioned(
              left: 8,
              bottom: 8,
              right: 40,
              child: Container(
                padding: const EdgeInsets.symmetric(horizontal: 6, vertical: 3),
                decoration: BoxDecoration(
                  color: Colors.black54,
                  borderRadius: BorderRadius.circular(4),
                ),
                child: Text(
                  camera.name,
                  maxLines: 1,
                  overflow: TextOverflow.ellipsis,
                  style: const TextStyle(
                    color: NvrColors.textPrimary,
                    fontSize: 11,
                    fontWeight: FontWeight.w500,
                  ),
                ),
              ),
            ),

            // OFFLINE badge - top right
            if (!isOnline)
              Positioned(
                top: 8,
                right: 8,
                child: Container(
                  padding: const EdgeInsets.symmetric(horizontal: 6, vertical: 2),
                  decoration: BoxDecoration(
                    color: NvrColors.danger,
                    borderRadius: BorderRadius.circular(4),
                  ),
                  child: const Text(
                    'OFFLINE',
                    style: TextStyle(
                      color: Colors.white,
                      fontSize: 10,
                      fontWeight: FontWeight.bold,
                    ),
                  ),
                ),
              ),

            // AI icon - top left
            if (camera.aiEnabled)
              Positioned(
                top: 8,
                left: 8,
                child: Container(
                  padding: const EdgeInsets.all(4),
                  decoration: BoxDecoration(
                    color: NvrColors.accent.withValues(alpha: 0.85),
                    borderRadius: BorderRadius.circular(4),
                  ),
                  child: const Icon(
                    Icons.auto_awesome,
                    color: Colors.white,
                    size: 14,
                  ),
                ),
              ),
          ],
        ),
      ),
    );
  }

  Widget _buildVideoLayer(bool isOnline) {
    if (!isOnline) {
      return Container(
        color: NvrColors.bgSecondary,
        child: const Center(
          child: Icon(Icons.videocam_off, color: NvrColors.textMuted, size: 36),
        ),
      );
    }

    if (_connection == null) {
      return Container(
        color: NvrColors.bgSecondary,
        child: const Center(
          child: Icon(Icons.videocam_off, color: NvrColors.textMuted, size: 36),
        ),
      );
    }

    switch (_connState) {
      case WhepConnectionState.connecting:
        return Container(
          color: NvrColors.bgSecondary,
          child: const Center(
            child: CircularProgressIndicator(
              strokeWidth: 2,
              color: NvrColors.accent,
            ),
          ),
        );

      case WhepConnectionState.connected:
        final renderer = _connection!.renderer;
        if (renderer == null) {
          return Container(
            color: NvrColors.bgSecondary,
            child: const Center(
              child: CircularProgressIndicator(
                strokeWidth: 2,
                color: NvrColors.accent,
              ),
            ),
          );
        }
        return RTCVideoView(
          renderer,
          objectFit: RTCVideoViewObjectFit.RTCVideoViewObjectFitCover,
        );

      case WhepConnectionState.failed:
        return Container(
          color: NvrColors.bgSecondary,
          child: Center(
            child: Column(
              mainAxisSize: MainAxisSize.min,
              children: [
                const Icon(Icons.error_outline, color: NvrColors.danger, size: 28),
                const SizedBox(height: 8),
                const Text(
                  'Connection failed',
                  style: TextStyle(color: NvrColors.textSecondary, fontSize: 11),
                ),
                const SizedBox(height: 8),
                TextButton(
                  style: TextButton.styleFrom(
                    padding: const EdgeInsets.symmetric(horizontal: 12, vertical: 4),
                    minimumSize: Size.zero,
                    tapTargetSize: MaterialTapTargetSize.shrinkWrap,
                  ),
                  onPressed: _retry,
                  child: const Text(
                    'Retry',
                    style: TextStyle(color: NvrColors.accent, fontSize: 11),
                  ),
                ),
              ],
            ),
          ),
        );

      case WhepConnectionState.disposed:
        return Container(color: NvrColors.bgSecondary);
    }
  }
}
