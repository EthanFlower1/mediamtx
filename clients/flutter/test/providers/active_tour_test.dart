import 'package:fake_async/fake_async.dart';
import 'package:flutter_test/flutter_test.dart';
import 'package:nvr_client/models/tour.dart';
import 'package:nvr_client/providers/tours_provider.dart';

void main() {
  Tour makeTour({
    String id = 'tour1',
    String name = 'Test Tour',
    List<String> cameraIds = const ['cam1', 'cam2', 'cam3'],
    int dwellSeconds = 5,
  }) {
    return Tour(
      id: id,
      name: name,
      cameraIds: cameraIds,
      dwellSeconds: dwellSeconds,
      createdAt: null,
      updatedAt: null,
    );
  }

  group('ActiveTourState', () {
    test('default state is not active', () {
      const state = ActiveTourState();
      expect(state.isActive, false);
      expect(state.tour, isNull);
      expect(state.currentCameraIndex, 0);
      expect(state.isPaused, false);
      expect(state.currentCameraId, isNull);
    });

    test('isActive is true when tour is set', () {
      final tour = makeTour();
      final state = ActiveTourState(tour: tour);
      expect(state.isActive, true);
    });

    test('currentCameraId returns correct camera', () {
      final tour = makeTour();
      final state = ActiveTourState(tour: tour, currentCameraIndex: 1);
      expect(state.currentCameraId, 'cam2');
    });

    test('currentCameraId wraps with modulo', () {
      final tour = makeTour(); // 3 cameras
      final state = ActiveTourState(tour: tour, currentCameraIndex: 5);
      // 5 % 3 = 2 -> 'cam3'
      expect(state.currentCameraId, 'cam3');
    });
  });

  group('ActiveTourNotifier', () {
    late ActiveTourNotifier notifier;

    setUp(() {
      notifier = ActiveTourNotifier();
    });

    tearDown(() {
      notifier.dispose();
    });

    test('initial state is not active', () {
      expect(notifier.state.isActive, false);
      expect(notifier.state.tour, isNull);
    });

    test('start() sets active with currentCameraIndex 0', () {
      final tour = makeTour();
      notifier.start(tour);
      expect(notifier.state.isActive, true);
      expect(notifier.state.tour?.id, 'tour1');
      expect(notifier.state.currentCameraIndex, 0);
      expect(notifier.state.isPaused, false);
    });

    test('stop() clears state', () {
      notifier.start(makeTour());
      notifier.stop();
      expect(notifier.state.isActive, false);
      expect(notifier.state.tour, isNull);
      expect(notifier.state.currentCameraIndex, 0);
    });

    test('pause() sets isPaused to true', () {
      notifier.start(makeTour());
      notifier.pause();
      expect(notifier.state.isPaused, true);
      expect(notifier.state.isActive, true); // still active
    });

    test('resume() sets isPaused to false', () {
      notifier.start(makeTour());
      notifier.pause();
      notifier.resume();
      expect(notifier.state.isPaused, false);
    });

    test('resume() is a no-op when no tour is active', () {
      notifier.resume();
      expect(notifier.state.isActive, false);
    });

    test('timer advances currentCameraIndex after dwell period', () {
      fakeAsync((async) {
        final tour = makeTour(dwellSeconds: 5);
        notifier.start(tour);

        expect(notifier.state.currentCameraIndex, 0);

        // Advance past one dwell period
        async.elapse(const Duration(seconds: 5));
        expect(notifier.state.currentCameraIndex, 1);

        // Advance past another dwell period
        async.elapse(const Duration(seconds: 5));
        expect(notifier.state.currentCameraIndex, 2);

        // Wraps around
        async.elapse(const Duration(seconds: 5));
        expect(notifier.state.currentCameraIndex, 0); // 3 % 3 = 0

        notifier.stop();
      });
    });

    test('timer does not advance when paused', () {
      fakeAsync((async) {
        final tour = makeTour(dwellSeconds: 5);
        notifier.start(tour);

        async.elapse(const Duration(seconds: 5));
        expect(notifier.state.currentCameraIndex, 1);

        notifier.pause();

        // Timer fires but should not advance because isPaused
        async.elapse(const Duration(seconds: 10));
        expect(notifier.state.currentCameraIndex, 1); // unchanged
        expect(notifier.state.isPaused, true);

        notifier.stop();
      });
    });

    test('starting a new tour stops the old one', () {
      fakeAsync((async) {
        final tour1 = makeTour(id: 'tour1', dwellSeconds: 5);
        final tour2 = makeTour(
          id: 'tour2',
          cameraIds: ['a', 'b'],
          dwellSeconds: 10,
        );

        notifier.start(tour1);
        async.elapse(const Duration(seconds: 5));
        expect(notifier.state.currentCameraIndex, 1);

        // Start a new tour — should reset
        notifier.start(tour2);
        expect(notifier.state.tour?.id, 'tour2');
        expect(notifier.state.currentCameraIndex, 0);

        // Old timer should be canceled; new timer uses 10s dwell
        async.elapse(const Duration(seconds: 5));
        expect(notifier.state.currentCameraIndex, 0); // not yet

        async.elapse(const Duration(seconds: 5));
        expect(notifier.state.currentCameraIndex, 1);

        notifier.stop();
      });
    });

    test('stop() cancels the timer', () {
      fakeAsync((async) {
        notifier.start(makeTour(dwellSeconds: 5));

        async.elapse(const Duration(seconds: 5));
        expect(notifier.state.currentCameraIndex, 1);

        notifier.stop();

        // Timer should be canceled — further elapsed time should not throw
        async.elapse(const Duration(seconds: 20));
        expect(notifier.state.isActive, false);
        expect(notifier.state.currentCameraIndex, 0);
      });
    });
  });
}
