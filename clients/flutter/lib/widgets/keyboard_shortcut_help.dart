import 'dart:ui';

import 'package:flutter/material.dart';
import 'package:flutter/services.dart';

import '../theme/nvr_colors.dart';
import '../theme/nvr_typography.dart';

/// Data class representing a single keyboard shortcut entry.
class ShortcutEntry {
  final String keys;
  final String description;

  const ShortcutEntry({required this.keys, required this.description});
}

/// Groups of keyboard shortcuts by context.
class ShortcutGroup {
  final String title;
  final List<ShortcutEntry> entries;

  const ShortcutGroup({required this.title, required this.entries});
}

/// All keyboard shortcuts defined in the application.
const List<ShortcutGroup> allShortcuts = [
  ShortcutGroup(
    title: 'GLOBAL',
    entries: [
      ShortcutEntry(keys: '?', description: 'Toggle shortcut help'),
      ShortcutEntry(keys: 'Esc', description: 'Close overlay / exit fullscreen'),
    ],
  ),
  ShortcutGroup(
    title: 'LIVE VIEW',
    entries: [
      ShortcutEntry(keys: '1-9', description: 'Select grid position'),
    ],
  ),
  ShortcutGroup(
    title: 'LIVE VIEW — FULLSCREEN (PTZ)',
    entries: [
      ShortcutEntry(keys: '\u2191 / \u2193 / \u2190 / \u2192', description: 'Pan / tilt camera'),
      ShortcutEntry(keys: '+ / -', description: 'Zoom in / out'),
      ShortcutEntry(keys: 'Home', description: 'Stop PTZ movement'),
    ],
  ),
  ShortcutGroup(
    title: 'PLAYBACK',
    entries: [
      ShortcutEntry(keys: 'Space', description: 'Play / pause'),
      ShortcutEntry(keys: '+ / =', description: 'Increase playback speed'),
      ShortcutEntry(keys: '- / _', description: 'Decrease playback speed'),
      ShortcutEntry(keys: '\u2190 / \u2192', description: 'Step frame back / forward'),
      ShortcutEntry(keys: 'J / L', description: 'Skip to previous / next event'),
    ],
  ),
];

/// An overlay that displays all keyboard shortcuts.
///
/// Show it by calling [showKeyboardShortcutHelp] or toggling the
/// [KeyboardShortcutHelpOverlay] visibility.
class KeyboardShortcutHelpOverlay extends StatelessWidget {
  final VoidCallback onClose;

  const KeyboardShortcutHelpOverlay({super.key, required this.onClose});

  @override
  Widget build(BuildContext context) {
    final colors = NvrColors.of(context);
    final typo = NvrTypography.of(context);
    return GestureDetector(
      onTap: onClose,
      child: Container(
        color: Colors.black54,
        child: Center(
          child: GestureDetector(
            onTap: () {}, // absorb taps on the card itself
            child: ClipRRect(
              borderRadius: BorderRadius.circular(12),
              child: BackdropFilter(
                filter: ImageFilter.blur(sigmaX: 12, sigmaY: 12),
                child: Container(
                  width: 480,
                  constraints: BoxConstraints(
                    maxHeight: MediaQuery.of(context).size.height * 0.8,
                  ),
                  decoration: BoxDecoration(
                    color: colors.bgSecondary.withValues(alpha: 0.92),
                    borderRadius: BorderRadius.circular(12),
                    border: Border.all(color: colors.border),
                  ),
                  child: Column(
                    mainAxisSize: MainAxisSize.min,
                    children: [
                      // ── Header ───────────────────────────────────
                      Container(
                        padding: const EdgeInsets.symmetric(
                            horizontal: 20, vertical: 14),
                        decoration: BoxDecoration(
                          border: Border(
                            bottom: BorderSide(color: colors.border),
                          ),
                        ),
                        child: Row(
                          children: [
                            Icon(Icons.keyboard,
                                size: 16, color: colors.accent),
                            const SizedBox(width: 8),
                            Text('KEYBOARD SHORTCUTS',
                                style: typo.monoSection
                                    .copyWith(fontSize: 11)),
                            const Spacer(),
                            const _KeyCap(label: '?'),
                            const SizedBox(width: 6),
                            Text('to toggle',
                                style: typo.monoControl),
                          ],
                        ),
                      ),

                      // ── Shortcut groups ──────────────────────────
                      Flexible(
                        child: SingleChildScrollView(
                          padding: const EdgeInsets.symmetric(
                              horizontal: 20, vertical: 12),
                          child: Column(
                            crossAxisAlignment: CrossAxisAlignment.start,
                            children: [
                              for (int i = 0;
                                  i < allShortcuts.length;
                                  i++) ...[
                                if (i > 0) const SizedBox(height: 16),
                                _ShortcutGroupSection(
                                    group: allShortcuts[i]),
                              ],
                            ],
                          ),
                        ),
                      ),

                      // ── Footer ───────────────────────────────────
                      Container(
                        padding: const EdgeInsets.symmetric(
                            horizontal: 20, vertical: 10),
                        decoration: BoxDecoration(
                          border: Border(
                            top: BorderSide(color: colors.border),
                          ),
                        ),
                        child: Row(
                          mainAxisAlignment: MainAxisAlignment.end,
                          children: [
                            Text('Press ',
                                style: typo.monoControl),
                            const _KeyCap(label: 'Esc'),
                            Text(' or click outside to close',
                                style: typo.monoControl),
                          ],
                        ),
                      ),
                    ],
                  ),
                ),
              ),
            ),
          ),
        ),
      ),
    );
  }
}

class _ShortcutGroupSection extends StatelessWidget {
  final ShortcutGroup group;
  const _ShortcutGroupSection({required this.group});

  @override
  Widget build(BuildContext context) {
    final typo = NvrTypography.of(context);
    return Column(
      crossAxisAlignment: CrossAxisAlignment.start,
      children: [
        Text(group.title,
            style: typo.monoSection.copyWith(fontSize: 9)),
        const SizedBox(height: 8),
        for (final entry in group.entries)
          Padding(
            padding: const EdgeInsets.only(bottom: 6),
            child: Row(
              children: [
                SizedBox(
                  width: 140,
                  child: Wrap(
                    spacing: 4,
                    runSpacing: 4,
                    children: [
                      for (final part in entry.keys.split(' / '))
                        _KeyCap(label: part.trim()),
                    ],
                  ),
                ),
                const SizedBox(width: 12),
                Expanded(
                  child: Text(entry.description,
                      style: typo.body.copyWith(fontSize: 12)),
                ),
              ],
            ),
          ),
      ],
    );
  }
}

/// A styled key-cap widget showing a keyboard key.
class _KeyCap extends StatelessWidget {
  final String label;
  const _KeyCap({required this.label});

  @override
  Widget build(BuildContext context) {
    final colors = NvrColors.of(context);
    return Container(
      padding: const EdgeInsets.symmetric(horizontal: 6, vertical: 2),
      decoration: BoxDecoration(
        color: colors.bgTertiary,
        borderRadius: BorderRadius.circular(4),
        border: Border.all(color: colors.border),
        boxShadow: const [
          BoxShadow(
            color: Colors.black26,
            offset: Offset(0, 1),
            blurRadius: 0,
          ),
        ],
      ),
      child: Text(
        label,
        style: TextStyle(
          fontFamily: 'JetBrainsMono',
          fontSize: 10,
          fontWeight: FontWeight.w500,
          color: colors.textPrimary,
        ),
      ),
    );
  }
}

/// Mixin that provides keyboard shortcut help overlay toggle via the `?` key.
///
/// Add this mixin to any StatefulWidget state and call
/// [buildShortcutHelpOverlay] in your widget's Stack.
mixin KeyboardShortcutHelpMixin<T extends StatefulWidget> on State<T> {
  bool _shortcutHelpVisible = false;

  bool get shortcutHelpVisible => _shortcutHelpVisible;

  void toggleShortcutHelp() {
    setState(() => _shortcutHelpVisible = !_shortcutHelpVisible);
  }

  void hideShortcutHelp() {
    if (_shortcutHelpVisible) {
      setState(() => _shortcutHelpVisible = false);
    }
  }

  /// Returns the help overlay widget if visible, or null.
  Widget? buildShortcutHelpOverlay() {
    if (!_shortcutHelpVisible) return null;
    return KeyboardShortcutHelpOverlay(onClose: hideShortcutHelp);
  }

  /// Handles the `?` key and `Escape` for the help overlay.
  /// Returns true if the event was consumed.
  bool handleShortcutHelpKey(KeyEvent event) {
    if (event is! KeyDownEvent) return false;

    if (event.logicalKey == LogicalKeyboardKey.escape && _shortcutHelpVisible) {
      hideShortcutHelp();
      return true;
    }

    // `?` is Shift + `/` on US keyboards, or the `?` key directly.
    if (event.character == '?') {
      toggleShortcutHelp();
      return true;
    }

    return false;
  }
}
