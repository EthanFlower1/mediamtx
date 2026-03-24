import 'package:flutter/material.dart';
import 'package:media_kit/media_kit.dart';
import 'package:media_kit_video/media_kit_video.dart';

import '../../theme/nvr_colors.dart';

class CameraPlayer extends StatefulWidget {
  final String url;
  final String cameraName;
  final double playbackSpeed;
  final bool playing;
  final VoidCallback? onReady;

  const CameraPlayer({
    super.key,
    required this.url,
    required this.cameraName,
    this.playbackSpeed = 1.0,
    this.playing = true,
    this.onReady,
  });

  @override
  State<CameraPlayer> createState() => _CameraPlayerState();
}

class _CameraPlayerState extends State<CameraPlayer> {
  late final Player _player;
  late final VideoController _controller;
  bool _ready = false;

  @override
  void initState() {
    super.initState();
    _player = Player();
    _controller = VideoController(_player);
    _open();
  }

  Future<void> _open() async {
    await _player.open(Media(widget.url), play: widget.playing);
    await _player.setRate(widget.playbackSpeed);
    _player.stream.playing.listen((_) {
      if (!_ready) {
        setState(() => _ready = true);
        widget.onReady?.call();
      }
    });
  }

  @override
  void didUpdateWidget(CameraPlayer old) {
    super.didUpdateWidget(old);

    if (old.url != widget.url) {
      _player.open(Media(widget.url), play: widget.playing);
      _player.setRate(widget.playbackSpeed);
      return;
    }

    if (old.playing != widget.playing) {
      if (widget.playing) {
        _player.play();
      } else {
        _player.pause();
      }
    }

    if (old.playbackSpeed != widget.playbackSpeed) {
      _player.setRate(widget.playbackSpeed);
    }
  }

  @override
  void dispose() {
    _player.dispose();
    super.dispose();
  }

  @override
  Widget build(BuildContext context) {
    return Stack(
      fit: StackFit.expand,
      children: [
        ColoredBox(
          color: Colors.black,
          child: Video(controller: _controller),
        ),
        // Camera name label
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
              widget.cameraName,
              style: const TextStyle(
                color: NvrColors.textPrimary,
                fontSize: 12,
                fontWeight: FontWeight.w500,
              ),
            ),
          ),
        ),
        if (!_ready)
          const Center(
            child: CircularProgressIndicator(color: NvrColors.accent),
          ),
      ],
    );
  }
}
