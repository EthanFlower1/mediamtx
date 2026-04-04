import 'dart:async';

import 'package:flutter/material.dart';
import 'package:flutter_webrtc/flutter_webrtc.dart';
import 'package:media_kit_video/media_kit_video.dart';

import '../../models/camera.dart';
import '../../services/rtsp_player_service.dart';
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
  final VoidCallback? onDoubleTap;

  const CameraTile({
    super.key,
    required this.camera,
    required this.serverUrl,
    this.onTap,
    this.onDoubleTap,
  });

  @override
  State<CameraTile> createState() => _CameraTileState();
}

class _CameraTileState extends State<CameraTile> {
  // WHEP (WebRTC) connection for H.264 streams.
  WhepConnection? _whepConnection;
  StreamSubscription<WhepConnectionState>? _whepStateSub;

  // RTSP connection for H.265 streams.
  RtspConnection? _rtspConnection;
  StreamSubscription<RtspConnectionState>? _rtspStateSub;

  // Unified connection state.
  bool _isConnecting = true;
  bool _isConnected = false;
  bool _isFailed = false;
  bool _useRtsp = false;

  // Drives the HH:MM:SS timestamp in the bottom-right overlay.
  late Timer _clockTimer;
  DateTime _now = DateTime.now();

  // Stream override — when non-null, overrides the default liveViewPath.
  String? _overridePath;
  String? _overrideCodec;

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
      _setConnState(connecting: true);
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

  String get _activeCodec {
    if (_overrideCodec != null) return _overrideCodec!;
    return widget.camera.liveViewCodec;
  }

  bool get _isH265 {
    final codec = _activeCodec.toUpperCase();
    return codec == 'H265' || codec == 'HEVC';
  }

  void _setConnState({bool connecting = false, bool connected = false, bool failed = false}) {
    if (mounted) {
      setState(() {
        _isConnecting = connecting;
        _isConnected = connected;
        _isFailed = failed;
      });
    }
  }

  void _initConnection() {
    final camera = widget.camera;
    final isOnline = camera.status != 'disconnected';
    final livePath = _activePath;
    if (!isOnline || livePath.isEmpty) return;

    _useRtsp = _isH265;

    if (_useRtsp) {
      _rtspConnection = RtspConnection(
        serverUrl: widget.serverUrl,
        mediamtxPath: livePath,
      );
      _rtspStateSub = _rtspConnection!.stateStream.listen((state) {
        if (!mounted) return;
        switch (state) {
          case RtspConnectionState.connecting:
            _setConnState(connecting: true);
          case RtspConnectionState.connected:
            _setConnState(connected: true);
          case RtspConnectionState.failed:
            _setConnState(failed: true);
          case RtspConnectionState.disposed:
            break;
        }
      });
      _rtspConnection!.connect();
    } else {
      _whepConnection = WhepConnection(
        serverUrl: widget.serverUrl,
        mediamtxPath: livePath,
      );
      _whepStateSub = _whepConnection!.stateStream.listen((state) {
        if (!mounted) return;
        switch (state) {
          case WhepConnectionState.connecting:
            _setConnState(connecting: true);
          case WhepConnectionState.connected:
            _setConnState(connected: true);
          case WhepConnectionState.failed:
            _setConnState(failed: true);
          case WhepConnectionState.disposed:
            break;
        }
      });
      _whepConnection!.connect();
    }
  }

  Future<void> _retry() async {
    if (_useRtsp) {
      await _rtspConnection?.retry();
    } else {
      await _whepConnection?.retry();
    }
  }

  void _switchStream(String path) {
    if (path == _activePath) return;
    // Find the codec for the selected stream.
    String? codec;
    for (final sp in widget.camera.streamPaths) {
      if (sp.path == path) {
        codec = sp.videoCodec;
        break;
      }
    }
    setState(() {
      _overridePath = path;
      _overrideCodec = codec;
    });
    _disposeConnection();
    _setConnState(connecting: true);
    _initConnection();
  }

  void _disposeConnection() {
    _whepStateSub?.cancel();
    _whepStateSub = null;
    _whepConnection?.dispose();
    _whepConnection = null;

    _rtspStateSub?.cancel();
    _rtspStateSub = null;
    _rtspConnection?.dispose();
    _rtspConnection = null;
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
      onDoubleTap: widget.onDoubleTap,
      child: Container(
        decoration: BoxDecoration(
          color: NvrColors.of(context).bgSecondary,
          border: Border.all(color: NvrColors.of(context).border, width: 1),
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
              const CornerBrackets(child: SizedBox.expand()),

              // AI analytics overlay (preserved, rebuilt in next task).
              if (camera.aiEnabled && _isConnected)
                AnalyticsOverlay(
                  cameraName: camera.name,
                  cameraId: camera.id,
                ),

              // Top-left: LIVE / OFFLINE status badge.
              Positioned(
                top: 8,
                left: 8,
                child: isOnline
                    ? StatusBadge.live(context)
                    : StatusBadge.offline(context),
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
                    if (camera.aiEnabled) StatusBadge.recording(context),
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
                  style: TextStyle(
                    fontFamily: 'IBMPlexSans',
                    fontSize: 10,
                    fontWeight: FontWeight.w500,
                    color: NvrColors.of(context).textPrimary,
                  ),
                ),
              ),

              // Bottom-right: HH:MM:SS timestamp.
              Positioned(
                right: 8,
                bottom: 8,
                child: Text(
                  timestampStr,
                  style: TextStyle(
                    fontFamily: 'JetBrainsMono',
                    fontSize: 8,
                    color: NvrColors.of(context).textMuted,
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
                color: NvrColors.of(context).bgSecondary,
                border: Border.all(color: NvrColors.of(context).accent),
                borderRadius: BorderRadius.circular(6),
              ),
              child: Center(
                child: Text(camera.name, style: NvrTypography.of(context).cameraName),
              ),
            ),
          ),
        ),
      ),
      childWhenDragging: Container(
        decoration: BoxDecoration(
          color: NvrColors.of(context).bgPrimary,
          border: Border.all(color: NvrColors.of(context).border),
          borderRadius: BorderRadius.circular(6),
        ),
      ),
      child: _buildTile(isOnline, timestampStr),
    );
  }

  Widget _buildVideoLayer(bool isOnline) {
    if (!isOnline) {
      return Container(
        color: NvrColors.of(context).bgSecondary,
        child: Center(
          child: Column(
            mainAxisSize: MainAxisSize.min,
            children: [
              Icon(
                Icons.videocam_off,
                color: NvrColors.of(context).textMuted,
                size: 32,
              ),
              const SizedBox(height: 6),
              Text(
                widget.camera.name,
                maxLines: 1,
                overflow: TextOverflow.ellipsis,
                style: TextStyle(
                  fontFamily: 'IBMPlexSans',
                  fontSize: 10,
                  fontWeight: FontWeight.w500,
                  color: NvrColors.of(context).textMuted,
                ),
              ),
            ],
          ),
        ),
      );
    }

    final hasConnection = _useRtsp ? _rtspConnection != null : _whepConnection != null;
    if (!hasConnection) {
      return Container(
        color: NvrColors.of(context).bgSecondary,
        child: Center(
          child: Icon(Icons.videocam_off, color: NvrColors.of(context).textMuted, size: 32),
        ),
      );
    }

    if (_isConnecting) {
      return Container(
        color: NvrColors.of(context).bgSecondary,
        child: Center(
          child: CircularProgressIndicator(
            strokeWidth: 2,
            color: NvrColors.of(context).accent,
          ),
        ),
      );
    }

    if (_isConnected) {
      if (_useRtsp) {
        final vc = _rtspConnection?.videoController;
        if (vc == null) {
          return Container(
            color: NvrColors.of(context).bgSecondary,
            child: Center(
              child: CircularProgressIndicator(
                strokeWidth: 2,
                color: NvrColors.of(context).accent,
              ),
            ),
          );
        }
        return Video(
          controller: vc,
          fill: NvrColors.of(context).bgSecondary,
        );
      } else {
        final renderer = _whepConnection!.renderer;
        if (renderer == null) {
          return Container(
            color: NvrColors.of(context).bgSecondary,
            child: Center(
              child: CircularProgressIndicator(
                strokeWidth: 2,
                color: NvrColors.of(context).accent,
              ),
            ),
          );
        }
        return RTCVideoView(
          renderer,
          objectFit: RTCVideoViewObjectFit.RTCVideoViewObjectFitCover,
        );
      }
    }

    if (_isFailed) {
      return Container(
        color: NvrColors.of(context).bgSecondary,
        child: Center(
          child: Column(
            mainAxisSize: MainAxisSize.min,
            children: [
              Icon(
                Icons.error_outline,
                color: NvrColors.of(context).danger,
                size: 28,
              ),
              const SizedBox(height: 8),
              Text(
                'Connection failed',
                style: TextStyle(
                  color: NvrColors.of(context).textSecondary,
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
                child: Text(
                  'Retry',
                  style: TextStyle(color: NvrColors.of(context).accent, fontSize: 11),
                ),
              ),
            ],
          ),
        ),
      );
    }

    return Container(color: NvrColors.of(context).bgSecondary);
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
          color: NvrColors.of(context).bgSecondary,
          shape: RoundedRectangleBorder(
            borderRadius: BorderRadius.circular(4),
            side: BorderSide(color: NvrColors.of(context).border),
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
                  color: isActive ? NvrColors.of(context).accent : NvrColors.of(context).textMuted,
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
          color: NvrColors.of(context).bgPrimary.withValues(alpha: 0.7),
          borderRadius: BorderRadius.circular(3),
          border: Border.all(color: NvrColors.of(context).border),
        ),
        child: Row(
          mainAxisSize: MainAxisSize.min,
          children: [
            Icon(Icons.videocam, color: NvrColors.of(context).textMuted, size: 10),
            SizedBox(width: 2),
            Icon(Icons.keyboard_arrow_down, color: NvrColors.of(context).textMuted, size: 10),
          ],
        ),
      ),
    );
  }
}
