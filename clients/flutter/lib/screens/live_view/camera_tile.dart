import 'dart:async';

import 'package:flutter/material.dart';
import 'package:flutter_webrtc/flutter_webrtc.dart';

import '../../models/camera.dart';
import '../../services/whep_service.dart';
import '../../theme/nvr_colors.dart';
import '../../theme/nvr_typography.dart';
import '../../widgets/hud/corner_brackets.dart';
import '../../widgets/hud/status_badge.dart';
import 'analytics_overlay.dart';

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

  // Drives the HH:MM:SS timestamp in the bottom-right overlay.
  late Timer _clockTimer;
  DateTime _now = DateTime.now();

  // Stream override — when non-null, overrides the default liveViewPath.
  String? _overridePath;

  @override
  void initState() {
    super.initState();
    _initConnection();
    _clockTimer = Timer.periodic(const Duration(seconds: 1), (_) {
      if (mounted) setState(() => _now = DateTime.now());
    });
  }

  @override
  void didUpdateWidget(CameraTile oldWidget) {
    super.didUpdateWidget(oldWidget);
    final oldOnline = oldWidget.camera.status != 'disconnected';
    final newOnline = widget.camera.status != 'disconnected';
    final pathChanged =
        oldWidget.camera.mediamtxPath != widget.camera.mediamtxPath ||
        oldWidget.camera.liveViewPath != widget.camera.liveViewPath;
    final serverChanged = oldWidget.serverUrl != widget.serverUrl;

    // Reconnect when camera comes online, path changes, or server changes.
    if ((!oldOnline && newOnline) || pathChanged || serverChanged) {
      _disposeConnection();
      _connState = WhepConnectionState.connecting;
      _initConnection();
    }
  }

  String get _activePath {
    if (_overridePath != null) return _overridePath!;
    final camera = widget.camera;
    return camera.liveViewPath.isNotEmpty
        ? camera.liveViewPath
        : camera.mediamtxPath;
  }

  void _initConnection() {
    final camera = widget.camera;
    final isOnline = camera.status != 'disconnected';
    final livePath = _activePath;
    if (!isOnline || livePath.isEmpty) return;

    _connection = WhepConnection(
      serverUrl: widget.serverUrl,
      mediamtxPath: livePath,
    );

    _stateSub = _connection!.stateStream.listen((state) {
      if (mounted) setState(() => _connState = state);
    });

    _connection!.connect();
  }

  Future<void> _retry() async {
    await _connection?.retry();
  }

  void _switchStream(String path) {
    if (path == _activePath) return;
    setState(() => _overridePath = path);
    _disposeConnection();
    _connState = WhepConnectionState.connecting;
    _initConnection();
  }

  void _disposeConnection() {
    _stateSub?.cancel();
    _stateSub = null;
    _connection?.dispose();
    _connection = null;
  }

  @override
  void dispose() {
    _clockTimer.cancel();
    _disposeConnection();
    super.dispose();
  }

  static String _hhmmss(DateTime dt) {
    String p(int v) => v.toString().padLeft(2, '0');
    return '${p(dt.hour)}:${p(dt.minute)}:${p(dt.second)}';
  }

  Widget _buildTile(bool isOnline, String timestampStr) {
    final camera = widget.camera;
    return GestureDetector(
      onTap: widget.onTap,
      child: Container(
        decoration: BoxDecoration(
          color: NvrColors.bgSecondary,
          border: Border.all(color: NvrColors.border, width: 1),
          borderRadius: BorderRadius.circular(6),
        ),
        child: ClipRRect(
          borderRadius: BorderRadius.circular(6),
          child: Stack(
            fit: StackFit.expand,
            children: [
              // Video / state layer fills the container.
              _buildVideoLayer(isOnline),

              // Corner-brackets HUD overlay.
              CornerBrackets(child: const SizedBox.expand()),

              // AI analytics overlay (preserved, rebuilt in next task).
              if (camera.aiEnabled &&
                  _connState == WhepConnectionState.connected)
                AnalyticsOverlay(
                  cameraName: camera.name,
                  cameraId: camera.id,
                ),

              // Top-left: LIVE / OFFLINE status badge.
              Positioned(
                top: 8,
                left: 8,
                child: isOnline
                    ? StatusBadge.live()
                    : StatusBadge.offline(),
              ),

              // Top-right: stream picker + REC badge.
              Positioned(
                top: 8,
                right: 8,
                child: Row(
                  mainAxisSize: MainAxisSize.min,
                  children: [
                    if (camera.streamPaths.length > 1)
                      _StreamPickerButton(
                        streamPaths: camera.streamPaths,
                        activePath: _activePath,
                        onSelected: _switchStream,
                      ),
                    if (camera.streamPaths.length > 1 && camera.aiEnabled)
                      const SizedBox(width: 4),
                    if (camera.aiEnabled) StatusBadge.recording(),
                  ],
                ),
              ),

              // Bottom-left: camera name.
              Positioned(
                left: 8,
                bottom: 8,
                right: 80,
                child: Text(
                  camera.name,
                  maxLines: 1,
                  overflow: TextOverflow.ellipsis,
                  style: const TextStyle(
                    fontFamily: 'IBMPlexSans',
                    fontSize: 10,
                    fontWeight: FontWeight.w500,
                    color: NvrColors.textPrimary,
                  ),
                ),
              ),

              // Bottom-right: HH:MM:SS timestamp.
              Positioned(
                right: 8,
                bottom: 8,
                child: Text(
                  timestampStr,
                  style: const TextStyle(
                    fontFamily: 'JetBrainsMono',
                    fontSize: 8,
                    color: NvrColors.textMuted,
                  ),
                ),
              ),
            ],
          ),
        ),
      ),
    );
  }

  @override
  Widget build(BuildContext context) {
    final camera = widget.camera;
    final isOnline = camera.status != 'disconnected';
    final timestampStr = _hhmmss(_now);

    return LongPressDraggable<String>(
      data: camera.id,
      feedback: Material(
        color: Colors.transparent,
        child: Opacity(
          opacity: 0.8,
          child: SizedBox(
            width: 160,
            height: 90,
            child: Container(
              decoration: BoxDecoration(
                color: NvrColors.bgSecondary,
                border: Border.all(color: NvrColors.accent),
                borderRadius: BorderRadius.circular(6),
              ),
              child: Center(
                child: Text(camera.name, style: NvrTypography.cameraName),
              ),
            ),
          ),
        ),
      ),
      childWhenDragging: Container(
        decoration: BoxDecoration(
          color: NvrColors.bgPrimary,
          border: Border.all(color: NvrColors.border),
          borderRadius: BorderRadius.circular(6),
        ),
      ),
      child: _buildTile(isOnline, timestampStr),
    );
  }

  Widget _buildVideoLayer(bool isOnline) {
    if (!isOnline) {
      return Container(
        color: NvrColors.bgSecondary,
        child: Center(
          child: Column(
            mainAxisSize: MainAxisSize.min,
            children: [
              const Icon(
                Icons.videocam_off,
                color: NvrColors.textMuted,
                size: 32,
              ),
              const SizedBox(height: 6),
              Text(
                widget.camera.name,
                maxLines: 1,
                overflow: TextOverflow.ellipsis,
                style: const TextStyle(
                  fontFamily: 'IBMPlexSans',
                  fontSize: 10,
                  fontWeight: FontWeight.w500,
                  color: NvrColors.textMuted,
                ),
              ),
            ],
          ),
        ),
      );
    }

    if (_connection == null) {
      return Container(
        color: NvrColors.bgSecondary,
        child: const Center(
          child: Icon(Icons.videocam_off, color: NvrColors.textMuted, size: 32),
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
                const Icon(
                  Icons.error_outline,
                  color: NvrColors.danger,
                  size: 28,
                ),
                const SizedBox(height: 8),
                const Text(
                  'Connection failed',
                  style: TextStyle(
                    color: NvrColors.textSecondary,
                    fontSize: 11,
                  ),
                ),
                const SizedBox(height: 8),
                TextButton(
                  style: TextButton.styleFrom(
                    padding: const EdgeInsets.symmetric(
                        horizontal: 12, vertical: 4),
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

class _StreamPickerButton extends StatelessWidget {
  final List<StreamPath> streamPaths;
  final String activePath;
  final ValueChanged<String> onSelected;

  const _StreamPickerButton({
    required this.streamPaths,
    required this.activePath,
    required this.onSelected,
  });

  String _label(StreamPath sp) {
    if (sp.resolution.isNotEmpty) return '${sp.name} (${sp.resolution})';
    return sp.name;
  }

  @override
  Widget build(BuildContext context) {
    return GestureDetector(
      onTap: () async {
        final box = context.findRenderObject() as RenderBox?;
        if (box == null) return;
        final offset = box.localToGlobal(Offset.zero);
        final picked = await showMenu<String>(
          context: context,
          position: RelativeRect.fromLTRB(
            offset.dx,
            offset.dy + box.size.height + 4,
            offset.dx + box.size.width,
            0,
          ),
          color: NvrColors.bgSecondary,
          shape: RoundedRectangleBorder(
            borderRadius: BorderRadius.circular(4),
            side: const BorderSide(color: NvrColors.border),
          ),
          items: streamPaths.map((sp) {
            final isActive = sp.path == activePath;
            return PopupMenuItem<String>(
              value: sp.path,
              height: 32,
              child: Text(
                _label(sp),
                style: TextStyle(
                  fontFamily: 'JetBrainsMono',
                  fontSize: 10,
                  color: isActive ? NvrColors.accent : NvrColors.textMuted,
                ),
              ),
            );
          }).toList(),
        );
        if (picked != null) onSelected(picked);
      },
      child: Container(
        padding: const EdgeInsets.symmetric(horizontal: 5, vertical: 2),
        decoration: BoxDecoration(
          color: NvrColors.bgPrimary.withValues(alpha: 0.7),
          borderRadius: BorderRadius.circular(3),
          border: Border.all(color: NvrColors.border),
        ),
        child: const Row(
          mainAxisSize: MainAxisSize.min,
          children: [
            Icon(Icons.videocam, color: NvrColors.textMuted, size: 10),
            SizedBox(width: 2),
            Icon(Icons.keyboard_arrow_down, color: NvrColors.textMuted, size: 10),
          ],
        ),
      ),
    );
  }
}
