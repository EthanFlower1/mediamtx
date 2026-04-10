// KAI-302 — Bookmark button + note dialog.
//
// Opens a dialog, then invokes `PlaybackClient.createBookmark` and shows
// a success / failure toast via ScaffoldMessenger.

import 'package:flutter/material.dart';

import '../playback_client.dart';
import '../playback_strings.dart';

class BookmarkButton extends StatelessWidget {
  final PlaybackClient client;
  final String segmentId;
  final int atMs;
  final PlaybackStrings strings;
  final ValueChanged<BookmarkId>? onCreated;

  const BookmarkButton({
    super.key,
    required this.client,
    required this.segmentId,
    required this.atMs,
    this.strings = PlaybackStrings.en,
    this.onCreated,
  });

  Future<void> _press(BuildContext context) async {
    final controller = TextEditingController();
    final note = await showDialog<String>(
      context: context,
      builder: (ctx) => AlertDialog(
        title: Text(strings.bookmarkDialogTitle),
        content: TextField(
          controller: controller,
          decoration:
              InputDecoration(hintText: strings.bookmarkDialogNoteHint),
        ),
        actions: [
          TextButton(
            onPressed: () => Navigator.of(ctx).pop(),
            child: Text(strings.bookmarkDialogCancel),
          ),
          TextButton(
            onPressed: () => Navigator.of(ctx).pop(controller.text),
            child: Text(strings.bookmarkDialogSave),
          ),
        ],
      ),
    );
    if (note == null) return;

    final messenger = ScaffoldMessenger.of(context);
    try {
      final id = await client.createBookmark(
        segmentId: segmentId,
        atMs: atMs,
        note: note.isEmpty ? null : note,
      );
      onCreated?.call(id);
      messenger.showSnackBar(
        SnackBar(content: Text(strings.bookmarkCreatedToast)),
      );
    } catch (_) {
      messenger.showSnackBar(
        SnackBar(content: Text(strings.bookmarkFailedToast)),
      );
    }
  }

  @override
  Widget build(BuildContext context) {
    return TextButton.icon(
      onPressed: () => _press(context),
      icon: const Icon(Icons.bookmark_add),
      label: Text(strings.bookmarkAdd),
    );
  }
}
