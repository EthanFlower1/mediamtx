// KAI-299 — Permission filter (UI hint) tests.

import 'package:flutter_test/flutter_test.dart';
import 'package:nvr_client/cameras/permission_filter.dart';
import 'package:nvr_client/models/camera.dart';
import 'package:nvr_client/state/app_session.dart';

Camera _cam(String id) => Camera(id: id, name: id);

const _session = AppSession(userId: 'u', tenantRef: 't');

void main() {
  group('isThumbnailVisible', () {
    test('admin group with view.thumbnails sees thumbnail', () {
      final groups = UserGroups(
        groups: const {'admin'},
        permissionsByGroup: const {
          'admin': {'view.thumbnails', 'view.streams'},
        },
      );
      expect(
        isThumbnailVisible(_cam('c1'), _session, userGroupsOverride: groups),
        isTrue,
      );
    });

    test('viewer with only view.thumbnails sees thumbnail', () {
      final groups = UserGroups(
        groups: const {'viewer'},
        permissionsByGroup: const {
          'viewer': {'view.thumbnails'},
        },
      );
      expect(
        isThumbnailVisible(_cam('c1'), _session, userGroupsOverride: groups),
        isTrue,
      );
    });

    test('group without view.thumbnails does NOT see thumbnail', () {
      final groups = UserGroups(
        groups: const {'limited'},
        permissionsByGroup: const {
          'limited': {'view.streams'},
        },
      );
      expect(
        isThumbnailVisible(_cam('c1'), _session, userGroupsOverride: groups),
        isFalse,
      );
    });

    test('empty groups is permissive (fallback until KAI-298 groups land)', () {
      expect(
        isThumbnailVisible(_cam('c1'), _session,
            userGroupsOverride: UserGroups.empty),
        isTrue,
      );
    });
  });

  group('filterByPermission', () {
    test('returns cameras unchanged (blur-in-place policy)', () {
      final cams = [_cam('a'), _cam('b')];
      final out = filterByPermission(cams, _session);
      expect(out, hasLength(2));
    });
  });
}
