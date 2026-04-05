import 'package:flutter_test/flutter_test.dart';
import 'package:nvr_client/models/user.dart';

void main() {
  group('User', () {
    test('fromJson parses all fields', () {
      final json = {
        'id': 'user-1',
        'username': 'admin',
        'role': 'admin',
        'camera_permissions': 'cam-1,cam-2',
      };

      final user = User.fromJson(json);

      expect(user.id, 'user-1');
      expect(user.username, 'admin');
      expect(user.role, 'admin');
      expect(user.cameraPermissions, 'cam-1,cam-2');
    });

    test('fromJson uses default camera_permissions', () {
      final json = {
        'id': 'user-2',
        'username': 'viewer',
        'role': 'viewer',
      };

      final user = User.fromJson(json);

      expect(user.cameraPermissions, '*');
    });

    test('copyWith overrides specific fields', () {
      final user = User.fromJson({
        'id': 'user-1',
        'username': 'admin',
        'role': 'admin',
      });

      final updated = user.copyWith(role: 'viewer');

      expect(updated.id, 'user-1');
      expect(updated.username, 'admin');
      expect(updated.role, 'viewer');
    });
  });
}
