// KAI-299 — Permission filter (UI hint only).
//
// IMPORTANT: This is a **UI hint**. The server is authoritative for every
// real data fetch (listing, thumbnails, streams). The purpose of this filter
// is to avoid flashing thumbnails the user can't keep, not to enforce
// security. Any actual thumbnail or stream request will still be gated by
// the Directory.
//
// The spec says we should check a `view.thumbnails` claim against the user's
// groups on AppSession. As of today AppSession does NOT yet carry a `groups`
// field (KAI-298 fix hasn't fully landed). So we code against a best-effort
// fallback: an external `UserGroups` shim which the caller computes however
// it likes (from a JWT claim, from a REST call, from a test fixture). When
// KAI-298's groups land this shim can be filled in trivially; nothing here
// needs to change.
//
// TODO(KAI-298): Once AppSession gains a `groups` field, swap the external
// shim for `session.groups` and delete `UserGroups.fromSession`.

import '../models/camera.dart';
import '../state/app_session.dart';

/// The permission string we check in this flow. Lifted to a constant so
/// tests and call sites don't drift.
const String kViewThumbnailsPermission = 'view.thumbnails';

/// Opaque, test-friendly wrapper over "what groups is the current user in
/// and what permissions does each group grant". Real-world code will build
/// this from the user's Zitadel claims; tests can construct it directly.
class UserGroups {
  /// Group names the user belongs to.
  final Set<String> groups;

  /// Map of group name → set of permissions that group grants.
  final Map<String, Set<String>> permissionsByGroup;

  const UserGroups({
    required this.groups,
    this.permissionsByGroup = const {},
  });

  static const UserGroups empty = UserGroups(groups: {});

  /// Build a best-effort [UserGroups] from an [AppSession]. Because the
  /// groups field doesn't exist yet this currently returns [empty] — it's a
  /// placeholder so call sites read naturally.
  factory UserGroups.fromSession(AppSession session) {
    // TODO(KAI-298): `session.groups` once the fix commit lands.
    return UserGroups.empty;
  }

  /// True if the user belongs to any group that grants [permission].
  bool hasPermission(String permission) {
    for (final g in groups) {
      final perms = permissionsByGroup[g];
      if (perms != null && perms.contains(permission)) return true;
    }
    return false;
  }
}

/// Permissive per-camera filter.
///
/// Returns the cameras untouched (the tree still shows them) — the caller
/// uses [isThumbnailVisible] to decide whether to blur the thumbnail and
/// show the lock icon. This matches the spec: "don't hide entirely; show a
/// lock icon".
///
/// [required] defaults to [kViewThumbnailsPermission] but is taken as a
/// parameter so the filter is reusable for future permission checks.
List<Camera> filterByPermission(
  List<Camera> cameras,
  AppSession session, {
  String required = kViewThumbnailsPermission,
  UserGroups? userGroupsOverride,
}) {
  // Spec says "show cameras but blur thumbnail" — so this list is
  // intentionally the same list. The companion [isThumbnailVisible] is what
  // gates the thumbnail widget.
  //
  // We accept the params even though we currently no-op on the list, because
  // we want the call site shape to match what we'll do once the server tells
  // us about camera-scoped hide rules.
  return cameras;
}

/// True if the user has permission to view the thumbnail for [camera].
///
/// Permissive fallback: if [UserGroups.fromSession] returns empty groups
/// (the KAI-298 groups field isn't wired yet), we return `true` — better to
/// show a thumbnail that the server will then replace with a "blocked"
/// placeholder than to blur every tile by default and confuse users.
bool isThumbnailVisible(
  Camera camera,
  AppSession session, {
  UserGroups? userGroupsOverride,
  String required = kViewThumbnailsPermission,
}) {
  final groups = userGroupsOverride ?? UserGroups.fromSession(session);
  if (groups.groups.isEmpty) {
    // TODO(KAI-298): tighten once AppSession.groups lands. For now we must
    // not block the UI on a groups list we do not have.
    return true;
  }
  return groups.hasPermission(required);
}
