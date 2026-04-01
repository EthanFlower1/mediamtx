import 'package:flutter_test/flutter_test.dart';
import 'package:nvr_client/providers/camera_panel_provider.dart';

void main() {
  group('CameraPanelState', () {
    test('default state has isOpen false, empty search, null group', () {
      const state = CameraPanelState();
      expect(state.isOpen, false);
      expect(state.searchQuery, '');
      expect(state.activeGroupId, isNull);
    });

    test('copyWith creates modified copy', () {
      const state = CameraPanelState();
      final modified = state.copyWith(isOpen: true, searchQuery: 'test');
      expect(modified.isOpen, true);
      expect(modified.searchQuery, 'test');
      expect(modified.activeGroupId, isNull);
    });

    test('copyWith clearGroupFilter clears activeGroupId', () {
      const state = CameraPanelState(activeGroupId: 'group1');
      final modified = state.copyWith(clearGroupFilter: true);
      expect(modified.activeGroupId, isNull);
    });
  });

  group('CameraPanelNotifier', () {
    late CameraPanelNotifier notifier;

    setUp(() {
      notifier = CameraPanelNotifier();
    });

    tearDown(() {
      notifier.dispose();
    });

    test('initial state: isOpen false, empty search, null group', () {
      expect(notifier.state.isOpen, false);
      expect(notifier.state.searchQuery, '');
      expect(notifier.state.activeGroupId, isNull);
    });

    test('toggle() flips isOpen', () {
      expect(notifier.state.isOpen, false);
      notifier.toggle();
      expect(notifier.state.isOpen, true);
      notifier.toggle();
      expect(notifier.state.isOpen, false);
    });

    test('open() sets isOpen to true', () {
      notifier.open();
      expect(notifier.state.isOpen, true);
      // calling open again keeps it true
      notifier.open();
      expect(notifier.state.isOpen, true);
    });

    test('close() sets isOpen to false', () {
      notifier.open();
      notifier.close();
      expect(notifier.state.isOpen, false);
    });

    test('setSearch updates query', () {
      notifier.setSearch('front door');
      expect(notifier.state.searchQuery, 'front door');
    });

    test('setSearch replaces previous query', () {
      notifier.setSearch('front door');
      notifier.setSearch('back yard');
      expect(notifier.state.searchQuery, 'back yard');
    });

    test('setGroupFilter sets filter', () {
      notifier.setGroupFilter('group_a');
      expect(notifier.state.activeGroupId, 'group_a');
    });

    test('setGroupFilter with same ID clears filter', () {
      notifier.setGroupFilter('group_a');
      expect(notifier.state.activeGroupId, 'group_a');
      notifier.setGroupFilter('group_a');
      expect(notifier.state.activeGroupId, isNull);
    });

    test('setGroupFilter with different ID replaces filter', () {
      notifier.setGroupFilter('group_a');
      notifier.setGroupFilter('group_b');
      expect(notifier.state.activeGroupId, 'group_b');
    });

    test('setGroupFilter with null when no group is active is a no-op', () {
      // Initially activeGroupId is null, calling setGroupFilter(null) means
      // null == null -> clearGroupFilter: true -> remains null
      notifier.setGroupFilter(null);
      expect(notifier.state.activeGroupId, isNull);
    });

    test('setGroupFilter with null when group is active keeps group (copyWith limitation)', () {
      notifier.setGroupFilter('group_a');
      // null != 'group_a' -> copyWith(activeGroupId: null)
      // but copyWith uses: activeGroupId ?? this.activeGroupId
      // so null ?? 'group_a' = 'group_a' — the filter is NOT cleared
      notifier.setGroupFilter(null);
      expect(notifier.state.activeGroupId, 'group_a');
    });

    test('operations are independent', () {
      notifier.open();
      notifier.setSearch('test');
      notifier.setGroupFilter('g1');

      expect(notifier.state.isOpen, true);
      expect(notifier.state.searchQuery, 'test');
      expect(notifier.state.activeGroupId, 'g1');

      notifier.close();
      expect(notifier.state.searchQuery, 'test'); // preserved
      expect(notifier.state.activeGroupId, 'g1'); // preserved
    });
  });
}
