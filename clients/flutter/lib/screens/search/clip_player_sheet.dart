import 'package:flutter/material.dart';
import 'package:media_kit/media_kit.dart';
import 'package:media_kit_video/media_kit_video.dart';
import '../../theme/nvr_colors.dart';

class ClipPlayerSheet extends StatefulWidget {
  final String url;
  final String title;

  const ClipPlayerSheet({
    super.key,
    required this.url,
    required this.title,
  });

  /// Convenience method to show as a bottom sheet.
  static Future<void> show(
    BuildContext context, {
    required String url,
    required String title,
  }) {
    return showModalBottomSheet<void>(
      context: context,
      isScrollControlled: true,
      backgroundColor: NvrColors.bgSecondary,
      shape: const RoundedRectangleBorder(
        borderRadius: BorderRadius.vertical(top: Radius.circular(16)),
      ),
      builder: (_) => ClipPlayerSheet(url: url, title: title),
    );
  }

  @override
  State<ClipPlayerSheet> createState() => _ClipPlayerSheetState();
}

class _ClipPlayerSheetState extends State<ClipPlayerSheet> {
  late final Player _player;
  late final VideoController _controller;

  @override
  void initState() {
    super.initState();
    _player = Player();
    _controller = VideoController(_player);
    _player.open(Media(widget.url));
  }

  @override
  void dispose() {
    _player.dispose();
    super.dispose();
  }

  @override
  Widget build(BuildContext context) {
    return SafeArea(
      child: Column(
        mainAxisSize: MainAxisSize.min,
        children: [
          // Handle
          Padding(
            padding: const EdgeInsets.symmetric(vertical: 10),
            child: Container(
              width: 40,
              height: 4,
              decoration: BoxDecoration(
                color: NvrColors.bgTertiary,
                borderRadius: BorderRadius.circular(2),
              ),
            ),
          ),
          // Title bar
          Padding(
            padding: const EdgeInsets.fromLTRB(16, 0, 8, 8),
            child: Row(
              children: [
                Expanded(
                  child: Text(
                    widget.title,
                    style: const TextStyle(
                      color: NvrColors.textPrimary,
                      fontSize: 14,
                      fontWeight: FontWeight.w600,
                    ),
                    overflow: TextOverflow.ellipsis,
                  ),
                ),
                IconButton(
                  icon: const Icon(Icons.close, color: NvrColors.textSecondary),
                  onPressed: () => Navigator.of(context).pop(),
                ),
              ],
            ),
          ),
          const Divider(color: NvrColors.border, height: 1),
          // Video player  (16:9 ratio)
          AspectRatio(
            aspectRatio: 16 / 9,
            child: ColoredBox(
              color: Colors.black,
              child: Video(controller: _controller),
            ),
          ),
          const SizedBox(height: 12),
        ],
      ),
    );
  }
}
