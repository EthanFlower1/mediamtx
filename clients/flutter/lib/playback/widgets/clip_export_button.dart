// KAI-302 — Clip export button.
//
// Opens a note dialog with the pre-selected range, then queues the export
// via `PlaybackClient.exportClip`. Shows a toast on success / failure.

import 'package:flutter/material.dart';

import '../playback_client.dart';
import '../playback_strings.dart';

class ClipExportButton extends StatelessWidget {
  final PlaybackClient client;
  final String segmentId;
  final int startMs;
  final int endMs;
  final PlaybackStrings strings;
  final ValueChanged<ClipId>? onQueued;

  const ClipExportButton({
    super.key,
    required this.client,
    required this.segmentId,
    required this.startMs,
    required this.endMs,
    this.strings = PlaybackStrings.en,
    this.onQueued,
  });

  Future<void> _press(BuildContext context) async {
    final controller = TextEditingController();
    final note = await showDialog<String>(
      context: context,
      builder: (ctx) => AlertDialog(
        title: Text(strings.clipExportDialogTitle),
        content: Column(
          mainAxisSize: MainAxisSize.min,
          children: [
            Text('${strings.clipExportDialogStart}: ${startMs}ms'),
            Text('${strings.clipExportDialogEnd}: ${endMs}ms'),
            TextField(
              controller: controller,
              decoration: InputDecoration(
                  hintText: strings.clipExportDialogNoteHint),
            ),
          ],
        ),
        actions: [
          TextButton(
            onPressed: () => Navigator.of(ctx).pop(),
            child: Text(strings.clipExportDialogCancel),
          ),
          TextButton(
            onPressed: () => Navigator.of(ctx).pop(controller.text),
            child: Text(strings.clipExportDialogConfirm),
          ),
        ],
      ),
    );
    if (note == null) return;

    final messenger = ScaffoldMessenger.of(context);
    try {
      final id = await client.exportClip(
        segmentId: segmentId,
        startMs: startMs,
        endMs: endMs,
        note: note.isEmpty ? null : note,
      );
      onQueued?.call(id);
      messenger.showSnackBar(
        SnackBar(content: Text(strings.clipExportQueuedToast)),
      );
    } catch (_) {
      messenger.showSnackBar(
        SnackBar(content: Text(strings.clipExportFailedToast)),
      );
    }
  }

  @override
  Widget build(BuildContext context) {
    return TextButton.icon(
      onPressed: () => _press(context),
      icon: const Icon(Icons.movie_creation_outlined),
      label: Text(strings.clipExport),
    );
  }
}
