// KAI-299 — Top-level federated camera tree screen.
//
// Combines the search bar + site tree view. Stateful so it can own the
// current search query. The tree, statuses, session, and strings are all
// injected to keep this widget testable without pumping Riverpod providers.

import 'package:flutter/material.dart';

import '../../models/camera.dart';
import '../../state/app_session.dart';
import '../camera_status_notifier.dart';
import '../camera_strings.dart';
import '../permission_filter.dart';
import '../site_tree.dart';
import 'global_search_bar.dart';
import 'site_tree_view.dart';

class CameraTreeScreen extends StatefulWidget {
  final SiteTree tree;
  final Map<String, CameraStatus> statuses;
  final AppSession session;
  final CameraStrings strings;
  final void Function(Camera)? onCameraTapped;
  final String Function(Camera)? thumbnailUrlBuilder;
  final UserGroups? userGroupsOverride;

  const CameraTreeScreen({
    super.key,
    required this.tree,
    required this.statuses,
    required this.session,
    required this.strings,
    this.onCameraTapped,
    this.thumbnailUrlBuilder,
    this.userGroupsOverride,
  });

  @override
  State<CameraTreeScreen> createState() => _CameraTreeScreenState();
}

class _CameraTreeScreenState extends State<CameraTreeScreen> {
  String _query = '';

  @override
  Widget build(BuildContext context) {
    final filtered = widget.tree.search(_query);
    final isSearching = _query.trim().isNotEmpty;
    return Scaffold(
      appBar: AppBar(title: Text(widget.strings.treeScreenTitle)),
      body: Column(
        children: [
          GlobalSearchBar(
            strings: widget.strings,
            onQuery: (q) => setState(() => _query = q),
          ),
          Expanded(
            child: filtered.totalCameraCount == 0 && isSearching
                ? Center(child: Text(widget.strings.emptySearchMessage))
                : SiteTreeView(
                    tree: filtered,
                    statuses: widget.statuses,
                    session: widget.session,
                    strings: widget.strings,
                    onCameraTapped: widget.onCameraTapped,
                    thumbnailUrlBuilder: widget.thumbnailUrlBuilder,
                    userGroupsOverride: widget.userGroupsOverride,
                  ),
          ),
        ],
      ),
    );
  }
}
