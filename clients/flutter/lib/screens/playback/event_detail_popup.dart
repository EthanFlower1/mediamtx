import 'package:flutter/material.dart';
import '../../models/recording.dart';
import '../../theme/nvr_colors.dart';
import 'timeline/event_layer.dart';

void showEventDetailPopup({
  required BuildContext context,
  required MotionEvent event,
  required Offset position,
  required VoidCallback onPlayFromHere,
}) {
  final overlay = Overlay.of(context);
  late OverlayEntry entry;

  entry = OverlayEntry(
    builder: (context) => _EventDetailOverlay(
      event: event,
      position: position,
      onPlayFromHere: () {
        entry.remove();
        onPlayFromHere();
      },
      onDismiss: () => entry.remove(),
    ),
  );

  overlay.insert(entry);
}

class _EventDetailOverlay extends StatelessWidget {
  final MotionEvent event;
  final Offset position;
  final VoidCallback onPlayFromHere;
  final VoidCallback onDismiss;

  const _EventDetailOverlay({
    required this.event,
    required this.position,
    required this.onPlayFromHere,
    required this.onDismiss,
  });

  String _formatTime(DateTime? t) {
    if (t == null) return '--:--:--';
    return '${t.hour.toString().padLeft(2, '0')}:'
        '${t.minute.toString().padLeft(2, '0')}:'
        '${t.second.toString().padLeft(2, '0')}';
  }

  String _formatDuration(DateTime start, DateTime? end) {
    if (end == null) return 'ongoing';
    final d = end.difference(start);
    if (d.inMinutes > 0) return '${d.inMinutes}m ${d.inSeconds % 60}s';
    return '${d.inSeconds}s';
  }

  String _classLabel(String? objectClass, String? eventType) {
    if (objectClass != null && objectClass.isNotEmpty) {
      return '${objectClass[0].toUpperCase()}${objectClass.substring(1)} detected';
    }
    if (eventType != null) return eventType;
    return 'Event';
  }

  @override
  Widget build(BuildContext context) {
    final color = EventLayer.colorForClass(event.objectClass);

    return Stack(
      children: [
        Positioned.fill(
          child: GestureDetector(
            onTap: onDismiss,
            child: const ColoredBox(color: Colors.transparent),
          ),
        ),
        Positioned(
          left: (position.dx - 140).clamp(8.0, MediaQuery.of(context).size.width - 288),
          top: (position.dy - 200).clamp(8.0, MediaQuery.of(context).size.height - 220),
          child: Material(
            color: Colors.transparent,
            child: Container(
              width: 280,
              padding: const EdgeInsets.all(12),
              decoration: BoxDecoration(
                color: NvrColors.bgSecondary,
                borderRadius: BorderRadius.circular(8),
                border: Border.all(color: NvrColors.border),
                boxShadow: [
                  BoxShadow(
                    color: Colors.black.withValues(alpha: 0.4),
                    blurRadius: 12,
                    offset: const Offset(0, 4),
                  ),
                ],
              ),
              child: Column(
                mainAxisSize: MainAxisSize.min,
                crossAxisAlignment: CrossAxisAlignment.start,
                children: [
                  if (event.thumbnailPath != null)
                    ClipRRect(
                      borderRadius: BorderRadius.circular(4),
                      child: Image.network(
                        event.thumbnailPath!,
                        width: 256,
                        height: 80,
                        fit: BoxFit.cover,
                        errorBuilder: (_, __, ___) => Container(
                          width: 256,
                          height: 80,
                          color: NvrColors.bgTertiary,
                          child: const Icon(Icons.image_not_supported,
                              color: NvrColors.textMuted),
                        ),
                      ),
                    )
                  else
                    Container(
                      width: 256,
                      height: 80,
                      decoration: BoxDecoration(
                        color: NvrColors.bgTertiary,
                        borderRadius: BorderRadius.circular(4),
                      ),
                      child: const Icon(Icons.videocam,
                          color: NvrColors.textMuted, size: 32),
                    ),
                  const SizedBox(height: 8),
                  Row(
                    children: [
                      Container(
                        width: 8,
                        height: 8,
                        decoration: BoxDecoration(
                          color: color,
                          shape: BoxShape.circle,
                        ),
                      ),
                      const SizedBox(width: 8),
                      Text(
                        _classLabel(event.objectClass, event.eventType),
                        style: const TextStyle(
                          color: NvrColors.textPrimary,
                          fontSize: 14,
                          fontWeight: FontWeight.w600,
                        ),
                      ),
                    ],
                  ),
                  const SizedBox(height: 4),
                  if (event.confidence != null)
                    Text(
                      '${event.objectClass ?? 'unknown'} (${(event.confidence! * 100).toStringAsFixed(0)}%)',
                      style: const TextStyle(
                          color: NvrColors.textSecondary, fontSize: 12),
                    ),
                  const SizedBox(height: 4),
                  Text(
                    '${_formatTime(event.startTime)} – ${_formatTime(event.endTime)}  (${_formatDuration(event.startTime, event.endTime)})',
                    style: const TextStyle(
                        color: NvrColors.textSecondary, fontSize: 12),
                  ),
                  const SizedBox(height: 8),
                  SizedBox(
                    width: double.infinity,
                    child: ElevatedButton.icon(
                      onPressed: onPlayFromHere,
                      icon: const Icon(Icons.play_arrow, size: 16),
                      label: const Text('Play from here'),
                      style: ElevatedButton.styleFrom(
                        backgroundColor: NvrColors.accent,
                        foregroundColor: Colors.white,
                        padding: const EdgeInsets.symmetric(vertical: 8),
                        textStyle: const TextStyle(fontSize: 12),
                      ),
                    ),
                  ),
                ],
              ),
            ),
          ),
        ),
      ],
    );
  }
}
