// KAI-299 — Expandable site tree widget.
//
// Renders the two-level peer → site grouping with Material ExpansionTiles.
// All strings come from [CameraStrings]. Camera rows are rendered via
// [CameraRow].

import 'package:flutter/material.dart';

import '../../models/camera.dart';
import '../../state/app_session.dart';
import '../camera_status_notifier.dart';
import '../camera_strings.dart';
import '../permission_filter.dart';
import '../site_tree.dart';
import 'camera_row.dart';

typedef CameraTapCallback = void Function(Camera camera);

class SiteTreeView extends StatelessWidget {
  final SiteTree tree;
  final Map<String, CameraStatus> statuses;
  final AppSession session;
  final CameraStrings strings;
  final CameraTapCallback? onCameraTapped;
  final String Function(Camera)? thumbnailUrlBuilder;
  final UserGroups? userGroupsOverride;

  const SiteTreeView({
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
  Widget build(BuildContext context) {
    if (tree.peers.isEmpty || tree.totalCameraCount == 0) {
      return Center(
        child: Padding(
          padding: const EdgeInsets.all(24),
          child: Text(strings.emptyTreeMessage),
        ),
      );
    }

    return ListView(
      children: [
        for (final peer in tree.peers)
          if (peer.totalCameraCount > 0) _buildPeer(context, peer),
      ],
    );
  }

  Widget _buildPeer(BuildContext context, SiteNode peer) {
    return ExpansionTile(
      key: PageStorageKey('peer-${peer.id}'),
      initiallyExpanded: peer.isExpanded,
      title: Text(peer.label),
      subtitle: Text(_cameraCountLabel(peer.totalCameraCount)),
      children: [
        for (final site in peer.children) _buildSite(context, site),
      ],
    );
  }

  Widget _buildSite(BuildContext context, SiteNode site) {
    return Padding(
      padding: const EdgeInsets.only(left: 16),
      child: ExpansionTile(
        key: PageStorageKey('site-${site.id}'),
        initiallyExpanded: site.isExpanded,
        title: Text(site.label),
        subtitle: Text(_cameraCountLabel(site.cameras.length)),
        children: [
          for (final cam in site.cameras) _buildRow(context, site, cam),
        ],
      ),
    );
  }

  Widget _buildRow(BuildContext context, SiteNode site, Camera cam) {
    final status = statuses[cam.id]?.state ?? CameraOnlineState.unknown;
    final thumbVisible = isThumbnailVisible(
      cam,
      session,
      userGroupsOverride: userGroupsOverride,
    );
    final thumbUrl = thumbnailUrlBuilder?.call(cam);
    return CameraRow(
      camera: cam,
      siteLabel: site.label,
      onlineState: status,
      thumbnailVisible: thumbVisible,
      thumbnailUrl: thumbUrl,
      strings: strings,
      onTap: onCameraTapped == null ? null : () => onCameraTapped!(cam),
    );
  }

  String _cameraCountLabel(int count) {
    if (count == 1) return strings.cameraCountSingular;
    return strings.cameraCountPlural.replaceAll('{count}', count.toString());
  }
}
