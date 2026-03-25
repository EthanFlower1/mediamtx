import 'package:flutter/material.dart';
import '../../../models/bookmark.dart';
import 'timeline_viewport.dart';

class BookmarkLayer extends CustomPainter {
  final TimelineViewport viewport;
  final List<Bookmark> bookmarks;
  final DateTime dayStart;

  BookmarkLayer({
    required this.viewport,
    required this.bookmarks,
    required this.dayStart,
  });

  @override
  void paint(Canvas canvas, Size size) {
    final fillPaint = Paint()
      ..color = Colors.amber
      ..style = PaintingStyle.fill;

    final linePaint = Paint()
      ..color = Colors.amber.withValues(alpha: 0.4)
      ..strokeWidth = 1;

    for (final bookmark in bookmarks) {
      final dur = bookmark.timestamp.difference(dayStart);
      final x = viewport.timeToPixel(dur);
      if (x < 0 || x > size.width) continue;

      // Draw a thin vertical line.
      canvas.drawLine(
        Offset(x, 0),
        Offset(x, size.height - 10),
        linePaint,
      );

      // Draw a small triangle marker at the bottom.
      final path = Path()
        ..moveTo(x, size.height)
        ..lineTo(x - 5, size.height - 10)
        ..lineTo(x + 5, size.height - 10)
        ..close();
      canvas.drawPath(path, fillPaint);
    }
  }

  @override
  bool shouldRepaint(covariant BookmarkLayer oldDelegate) =>
      bookmarks != oldDelegate.bookmarks ||
      viewport.visibleStart != oldDelegate.viewport.visibleStart ||
      viewport.visibleEnd != oldDelegate.viewport.visibleEnd;
}
