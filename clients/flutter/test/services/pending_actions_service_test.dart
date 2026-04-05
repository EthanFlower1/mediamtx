import 'package:flutter_test/flutter_test.dart';
import 'package:shared_preferences/shared_preferences.dart';
import 'package:nvr_client/services/pending_actions_service.dart';

void main() {
  late PendingActionsService service;

  setUp(() {
    SharedPreferences.setMockInitialValues({});
    service = PendingActionsService();
  });

  group('PendingAction model', () {
    test('toJson and fromJson round-trip', () {
      final now = DateTime.utc(2026, 4, 3, 12, 0, 0);
      final action = PendingAction(
        type: 'create_bookmark',
        payload: {'camera_id': 'cam-1', 'label': 'Test'},
        createdAt: now,
      );

      final json = action.toJson();
      final restored = PendingAction.fromJson(json);

      expect(restored.type, 'create_bookmark');
      expect(restored.payload['camera_id'], 'cam-1');
      expect(restored.payload['label'], 'Test');
      expect(restored.createdAt, now);
    });

    test('createdAt defaults to now when not provided', () {
      final before = DateTime.now();
      final action = PendingAction(
        type: 'toggle_favorite',
        payload: {'camera_id': 'cam-1'},
      );
      final after = DateTime.now();

      expect(action.createdAt.isAfter(before.subtract(const Duration(seconds: 1))), isTrue);
      expect(action.createdAt.isBefore(after.add(const Duration(seconds: 1))), isTrue);
    });
  });

  group('PendingActionsService', () {
    test('getAll returns empty list when no actions queued', () async {
      final actions = await service.getAll();
      expect(actions, isEmpty);
    });

    test('pendingCount returns 0 when no actions queued', () async {
      final count = await service.pendingCount;
      expect(count, 0);
    });

    test('enqueue adds action and increments count', () async {
      final action = PendingAction(
        type: 'create_bookmark',
        payload: {'camera_id': 'cam-1'},
      );

      await service.enqueue(action);

      final count = await service.pendingCount;
      expect(count, 1);

      final actions = await service.getAll();
      expect(actions.length, 1);
      expect(actions[0].type, 'create_bookmark');
      expect(actions[0].payload['camera_id'], 'cam-1');
    });

    test('enqueue preserves order of multiple actions', () async {
      await service.enqueue(PendingAction(
        type: 'create_bookmark',
        payload: {'id': '1'},
      ));
      await service.enqueue(PendingAction(
        type: 'delete_bookmark',
        payload: {'id': '2'},
      ));
      await service.enqueue(PendingAction(
        type: 'toggle_favorite',
        payload: {'id': '3'},
      ));

      final actions = await service.getAll();
      expect(actions.length, 3);
      expect(actions[0].type, 'create_bookmark');
      expect(actions[1].type, 'delete_bookmark');
      expect(actions[2].type, 'toggle_favorite');
    });

    test('clearAll removes all actions', () async {
      await service.enqueue(PendingAction(
        type: 'create_bookmark',
        payload: {'id': '1'},
      ));
      await service.enqueue(PendingAction(
        type: 'delete_bookmark',
        payload: {'id': '2'},
      ));

      await service.clearAll();

      final count = await service.pendingCount;
      expect(count, 0);
      expect(await service.getAll(), isEmpty);
    });

    test('removeAt removes action at specific index', () async {
      await service.enqueue(PendingAction(
        type: 'first',
        payload: {},
      ));
      await service.enqueue(PendingAction(
        type: 'second',
        payload: {},
      ));
      await service.enqueue(PendingAction(
        type: 'third',
        payload: {},
      ));

      await service.removeAt(1); // Remove 'second'

      final actions = await service.getAll();
      expect(actions.length, 2);
      expect(actions[0].type, 'first');
      expect(actions[1].type, 'third');
    });

    test('removeAt with out-of-bounds index does nothing', () async {
      await service.enqueue(PendingAction(
        type: 'only',
        payload: {},
      ));

      await service.removeAt(5); // Out of bounds
      await service.removeAt(-1); // Negative index

      final count = await service.pendingCount;
      expect(count, 1);
    });
  });
}
