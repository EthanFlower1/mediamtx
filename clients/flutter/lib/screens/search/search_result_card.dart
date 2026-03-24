import 'package:flutter/material.dart';
import '../../models/search_result.dart';
import '../../theme/nvr_colors.dart';

class SearchResultCard extends StatelessWidget {
  final SearchResult result;
  final String? thumbnailBaseUrl;
  final VoidCallback? onPlay;
  final VoidCallback? onSave;

  const SearchResultCard({
    super.key,
    required this.result,
    this.thumbnailBaseUrl,
    this.onPlay,
    this.onSave,
  });

  String _formatTime(DateTime dt) {
    final h = dt.hour.toString().padLeft(2, '0');
    final m = dt.minute.toString().padLeft(2, '0');
    final s = dt.second.toString().padLeft(2, '0');
    return '${dt.year}-${dt.month.toString().padLeft(2, '0')}-${dt.day.toString().padLeft(2, '0')} $h:$m:$s';
  }

  @override
  Widget build(BuildContext context) {
    final confidencePct = (result.confidence * 100).round();
    final similarityPct = (result.similarity * 100).round();

    return Card(
      color: NvrColors.bgSecondary,
      margin: const EdgeInsets.symmetric(horizontal: 12, vertical: 6),
      shape: RoundedRectangleBorder(
        borderRadius: BorderRadius.circular(8),
        side: const BorderSide(color: NvrColors.border),
      ),
      child: InkWell(
        onTap: onPlay,
        borderRadius: BorderRadius.circular(8),
        child: Padding(
          padding: const EdgeInsets.all(12),
          child: Row(
            children: [
              // Thumbnail or placeholder icon
              ClipRRect(
                borderRadius: BorderRadius.circular(6),
                child: SizedBox(
                  width: 80,
                  height: 56,
                  child: _ThumbnailWidget(
                    thumbnailPath: result.thumbnailPath,
                    baseUrl: thumbnailBaseUrl,
                  ),
                ),
              ),
              const SizedBox(width: 12),
              // Details
              Expanded(
                child: Column(
                  crossAxisAlignment: CrossAxisAlignment.start,
                  children: [
                    Row(
                      children: [
                        _ClassBadge(label: result.className),
                        const SizedBox(width: 8),
                        Text(
                          'Confidence: $confidencePct%',
                          style: const TextStyle(
                            color: NvrColors.textSecondary,
                            fontSize: 11,
                          ),
                        ),
                        const SizedBox(width: 8),
                        Text(
                          'Match: $similarityPct%',
                          style: const TextStyle(
                            color: NvrColors.accent,
                            fontSize: 11,
                          ),
                        ),
                      ],
                    ),
                    const SizedBox(height: 4),
                    Text(
                      result.cameraName,
                      style: const TextStyle(
                        color: NvrColors.textPrimary,
                        fontSize: 13,
                        fontWeight: FontWeight.w500,
                      ),
                    ),
                    const SizedBox(height: 2),
                    Text(
                      _formatTime(result.time),
                      style: const TextStyle(
                        color: NvrColors.textMuted,
                        fontSize: 11,
                      ),
                    ),
                  ],
                ),
              ),
              // Actions
              Column(
                children: [
                  IconButton(
                    tooltip: 'Play clip',
                    icon: const Icon(Icons.play_circle_outline,
                        color: NvrColors.accent),
                    onPressed: onPlay,
                  ),
                  IconButton(
                    tooltip: 'Save clip',
                    icon: const Icon(Icons.bookmark_border,
                        color: NvrColors.textSecondary),
                    onPressed: onSave,
                  ),
                ],
              ),
            ],
          ),
        ),
      ),
    );
  }
}

class _ThumbnailWidget extends StatelessWidget {
  final String? thumbnailPath;
  final String? baseUrl;

  const _ThumbnailWidget({this.thumbnailPath, this.baseUrl});

  @override
  Widget build(BuildContext context) {
    if (thumbnailPath != null && baseUrl != null) {
      final url = '$baseUrl$thumbnailPath';
      return Image.network(
        url,
        fit: BoxFit.cover,
        errorBuilder: (_, __, ___) => _placeholder(),
      );
    }
    return _placeholder();
  }

  Widget _placeholder() {
    return Container(
      color: NvrColors.bgTertiary,
      child: const Icon(Icons.image_not_supported,
          color: NvrColors.textMuted, size: 28),
    );
  }
}

class _ClassBadge extends StatelessWidget {
  final String label;

  const _ClassBadge({required this.label});

  Color _color() {
    switch (label.toLowerCase()) {
      case 'person':
        return NvrColors.accent;
      case 'vehicle':
      case 'car':
        return NvrColors.success;
      case 'animal':
        return NvrColors.warning;
      default:
        return NvrColors.textMuted;
    }
  }

  @override
  Widget build(BuildContext context) {
    return Container(
      padding: const EdgeInsets.symmetric(horizontal: 6, vertical: 2),
      decoration: BoxDecoration(
        color: _color().withValues(alpha: 0.15),
        borderRadius: BorderRadius.circular(4),
        border: Border.all(color: _color().withValues(alpha: 0.5)),
      ),
      child: Text(
        label,
        style: TextStyle(
          color: _color(),
          fontSize: 10,
          fontWeight: FontWeight.w600,
        ),
      ),
    );
  }
}
