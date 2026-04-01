import 'package:flutter_test/flutter_test.dart';
import 'package:shared_preferences/shared_preferences.dart';
import 'package:nvr_client/providers/grid_layout_provider.dart';

void main() {
  group('GridLayout model', () {
    test('totalSlots returns gridSize * gridSize', () {
      const layout = GridLayout(gridSize: 3);
      expect(layout.totalSlots, 9);

      const layout2 = GridLayout(gridSize: 4);
      expect(layout2.totalSlots, 16);

      const layout1 = GridLayout(gridSize: 1);
      expect(layout1.totalSlots, 1);
    });

    test('toJson produces correct structure', () {
      const layout = GridLayout(
        gridSize: 2,
        slots: {0: 'cam1', 3: 'cam2'},
      );
      final json = layout.toJson();
      expect(json['gridSize'], 2);
      expect(json['slots'], {'0': 'cam1', '3': 'cam2'});
    });

    test('fromJson parses correctly', () {
      final layout = GridLayout.fromJson({
        'gridSize': 3,
        'slots': {'0': 'camA', '5': 'camB'},
      });
      expect(layout.gridSize, 3);
      expect(layout.slots, {0: 'camA', 5: 'camB'});
    });

    test('toJson/fromJson round-trips correctly', () {
      const original = GridLayout(
        gridSize: 4,
        slots: {0: 'cam1', 3: 'cam2', 15: 'cam3'},
      );
      final json = original.toJson();
      final restored = GridLayout.fromJson(json);
      expect(restored.gridSize, original.gridSize);
      expect(restored.slots, original.slots);
    });

    test('fromJson defaults to gridSize 4 when missing', () {
      final layout = GridLayout.fromJson({});
      expect(layout.gridSize, 4);
      expect(layout.slots, isEmpty);
    });

    test('copyWith creates modified copy', () {
      const original = GridLayout(gridSize: 2, slots: {0: 'cam1'});
      final modified = original.copyWith(gridSize: 3);
      expect(modified.gridSize, 3);
      expect(modified.slots, {0: 'cam1'}); // slots preserved

      final modified2 = original.copyWith(slots: {1: 'cam2'});
      expect(modified2.gridSize, 2); // gridSize preserved
      expect(modified2.slots, {1: 'cam2'});
    });
  });

  group('GridLayoutNotifier', () {
    late GridLayoutNotifier notifier;

    setUp(() {
      SharedPreferences.setMockInitialValues({});
      notifier = GridLayoutNotifier('test_user');
    });

    tearDown(() {
      notifier.dispose();
    });

    test('initial state has gridSize 2', () {
      expect(notifier.state.gridSize, 2);
      expect(notifier.state.slots, isEmpty);
    });

    test('setGridSize updates grid size', () {
      notifier.setGridSize(3);
      expect(notifier.state.gridSize, 3);
    });

    test('setGridSize prunes slots beyond new size', () {
      notifier.setGridSize(4);
      notifier.assignCamera(0, 'cam1');
      notifier.assignCamera(15, 'cam2'); // slot 15 (last slot in 4x4)
      notifier.assignCamera(3, 'cam3');

      // Reduce to 2x2 (4 slots: 0,1,2,3)
      notifier.setGridSize(2);
      expect(notifier.state.gridSize, 2);
      expect(notifier.state.slots.containsKey(0), true);
      expect(notifier.state.slots.containsKey(3), true);
      expect(notifier.state.slots.containsKey(15), false); // pruned
    });

    test('assignCamera adds camera to slot', () {
      notifier.assignCamera(0, 'cam1');
      expect(notifier.state.slots[0], 'cam1');
    });

    test('assignCamera removes camera from old slot', () {
      notifier.assignCamera(0, 'cam1');
      notifier.assignCamera(1, 'cam1'); // move cam1 to slot 1
      expect(notifier.state.slots.containsKey(0), false);
      expect(notifier.state.slots[1], 'cam1');
    });

    test('assignCamera replaces existing camera in target slot', () {
      notifier.assignCamera(0, 'cam1');
      notifier.assignCamera(0, 'cam2');
      expect(notifier.state.slots[0], 'cam2');
    });

    test('removeCamera removes from slot', () {
      notifier.assignCamera(0, 'cam1');
      notifier.assignCamera(1, 'cam2');
      notifier.removeCamera(0);
      expect(notifier.state.slots.containsKey(0), false);
      expect(notifier.state.slots[1], 'cam2'); // other slot intact
    });

    test('removeCamera on empty slot is a no-op', () {
      notifier.assignCamera(0, 'cam1');
      notifier.removeCamera(5); // slot 5 is empty
      expect(notifier.state.slots[0], 'cam1');
    });

    test('swapSlots swaps two occupied slots', () {
      notifier.assignCamera(0, 'cam1');
      notifier.assignCamera(1, 'cam2');
      notifier.swapSlots(0, 1);
      expect(notifier.state.slots[0], 'cam2');
      expect(notifier.state.slots[1], 'cam1');
    });

    test('swapSlots with one empty slot moves camera', () {
      notifier.assignCamera(0, 'cam1');
      notifier.swapSlots(0, 3);
      expect(notifier.state.slots.containsKey(0), false);
      expect(notifier.state.slots[3], 'cam1');
    });

    test('fillFromGroup fills slots from camera list', () {
      notifier.setGridSize(2); // 4 slots
      notifier.fillFromGroup(['camA', 'camB', 'camC']);
      expect(notifier.state.slots[0], 'camA');
      expect(notifier.state.slots[1], 'camB');
      expect(notifier.state.slots[2], 'camC');
      expect(notifier.state.slots.containsKey(3), false);
    });

    test('fillFromGroup truncates to totalSlots', () {
      notifier.setGridSize(2); // 4 slots
      notifier.fillFromGroup(['c1', 'c2', 'c3', 'c4', 'c5', 'c6']);
      expect(notifier.state.slots.length, 4);
      expect(notifier.state.slots[3], 'c4');
    });

    test('fillFromGroup replaces previous slots', () {
      notifier.assignCamera(0, 'old_cam');
      notifier.fillFromGroup(['new1', 'new2']);
      expect(notifier.state.slots[0], 'new1');
      expect(notifier.state.slots[1], 'new2');
    });
  });
}
