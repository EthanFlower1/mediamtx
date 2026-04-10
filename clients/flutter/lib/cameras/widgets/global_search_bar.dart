// KAI-299 — Debounced search bar for the federated camera tree.
//
// Stateful so we can own the debounce timer. Emits via [onQuery] after a
// brief idle period so typing doesn't thrash the in-memory tree search.

import 'dart:async';

import 'package:flutter/material.dart';

import '../camera_strings.dart';

class GlobalSearchBar extends StatefulWidget {
  final ValueChanged<String> onQuery;
  final CameraStrings strings;
  final Duration debounce;
  final String initialQuery;

  const GlobalSearchBar({
    super.key,
    required this.onQuery,
    required this.strings,
    this.debounce = const Duration(milliseconds: 180),
    this.initialQuery = '',
  });

  @override
  State<GlobalSearchBar> createState() => _GlobalSearchBarState();
}

class _GlobalSearchBarState extends State<GlobalSearchBar> {
  late final TextEditingController _controller;
  Timer? _timer;

  @override
  void initState() {
    super.initState();
    _controller = TextEditingController(text: widget.initialQuery);
  }

  @override
  void dispose() {
    _timer?.cancel();
    _controller.dispose();
    super.dispose();
  }

  void _onChanged(String value) {
    _timer?.cancel();
    _timer = Timer(widget.debounce, () {
      if (mounted) widget.onQuery(value);
    });
  }

  void _clear() {
    _controller.clear();
    _timer?.cancel();
    widget.onQuery('');
  }

  @override
  Widget build(BuildContext context) {
    return Padding(
      padding: const EdgeInsets.all(8),
      child: TextField(
        controller: _controller,
        onChanged: _onChanged,
        decoration: InputDecoration(
          hintText: widget.strings.searchHint,
          prefixIcon: const Icon(Icons.search),
          suffixIcon: _controller.text.isEmpty
              ? null
              : IconButton(
                  tooltip: widget.strings.searchClearTooltip,
                  icon: const Icon(Icons.clear),
                  onPressed: _clear,
                ),
          border: const OutlineInputBorder(),
          isDense: true,
        ),
      ),
    );
  }
}
