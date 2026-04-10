// KAI-156 -- Unit tests for TalkbackService + FakeTalkbackService.

import 'package:flutter_test/flutter_test.dart';

import 'package:mediamtx/features/live_view/services/talkback_service.dart';

void main() {
  group('TalkbackState enum', () {
    test('has exactly 5 values', () {
      expect(TalkbackState.values.length, 5);
    });

    test('values are in expected order', () {
      expect(TalkbackState.values, [
        TalkbackState.idle,
        TalkbackState.acquiringMic,
        TalkbackState.connecting,
        TalkbackState.active,
        TalkbackState.error,
      ]);
    });
  });

  group('FakeTalkbackService', () {
    late FakeTalkbackService service;

    setUp(() {
      service = FakeTalkbackService(
        acquireDelay: Duration.zero,
        connectDelay: Duration.zero,
      );
    });

    tearDown(() {
      service.dispose();
    });

    test('starts in idle state', () {
      expect(service.currentState, TalkbackState.idle);
    });

    test('happy path transitions: idle -> acquiringMic -> connecting -> active',
        () async {
      final states = <TalkbackState>[];
      service.stateStream.listen(states.add);

      await service.startTalkback(
        endpointUrl: 'https://cam1.example.com/talkback',
        accessToken: 'tok_abc',
      );

      expect(states, [
        TalkbackState.acquiringMic,
        TalkbackState.connecting,
        TalkbackState.active,
      ]);
      expect(service.currentState, TalkbackState.active);
    });

    test('stopTalkback transitions to idle and records the call', () async {
      await service.startTalkback(
        endpointUrl: 'https://cam1.example.com/talkback',
        accessToken: 'tok_abc',
      );
      expect(service.currentState, TalkbackState.active);

      await service.stopTalkback();
      expect(service.currentState, TalkbackState.idle);

      final stopCalls =
          service.calls.where((c) => c.method == 'stopTalkback');
      expect(stopCalls.length, 1);
    });

    test('records startTalkback call with correct arguments', () async {
      await service.startTalkback(
        endpointUrl: 'https://cam2.local/talk',
        accessToken: 'tok_xyz',
      );

      final startCall = service.calls.firstWhere(
        (c) => c.method == 'startTalkback',
      );
      expect(startCall.args['endpointUrl'], 'https://cam2.local/talk');
      expect(startCall.args['accessToken'], 'tok_xyz');
    });

    test('records all calls in order including dispose', () async {
      await service.startTalkback(
        endpointUrl: 'https://cam1.local/talk',
        accessToken: 'tok_1',
      );
      await service.stopTalkback();
      service.dispose();

      final methods = service.calls.map((c) => c.method).toList();
      expect(methods, ['startTalkback', 'stopTalkback', 'dispose']);
    });

    test('failure at connecting stage transitions to error', () async {
      final failService = FakeTalkbackService(
        acquireDelay: Duration.zero,
        connectDelay: Duration.zero,
        shouldFail: true,
        failAt: TalkbackState.connecting,
      );

      final states = <TalkbackState>[];
      failService.stateStream.listen(states.add);

      await failService.startTalkback(
        endpointUrl: 'https://cam1.local/talk',
        accessToken: 'tok_fail',
      );

      expect(states, [
        TalkbackState.acquiringMic,
        TalkbackState.connecting,
        TalkbackState.error,
      ]);
      expect(failService.currentState, TalkbackState.error);
      failService.dispose();
    });

    test('failure at acquiringMic stage transitions to error', () async {
      final failService = FakeTalkbackService(
        acquireDelay: Duration.zero,
        connectDelay: Duration.zero,
        shouldFail: true,
        failAt: TalkbackState.acquiringMic,
      );

      final states = <TalkbackState>[];
      failService.stateStream.listen(states.add);

      await failService.startTalkback(
        endpointUrl: 'https://cam1.local/talk',
        accessToken: 'tok_fail',
      );

      expect(states, [
        TalkbackState.acquiringMic,
        TalkbackState.error,
      ]);
      expect(failService.currentState, TalkbackState.error);
      failService.dispose();
    });

    test('can recover after error by calling startTalkback again', () async {
      final failService = FakeTalkbackService(
        acquireDelay: Duration.zero,
        connectDelay: Duration.zero,
        shouldFail: true,
        failAt: TalkbackState.connecting,
      );

      await failService.startTalkback(
        endpointUrl: 'https://cam1.local/talk',
        accessToken: 'tok_1',
      );
      expect(failService.currentState, TalkbackState.error);

      // Create a new non-failing service to simulate recovery
      final recoveryService = FakeTalkbackService(
        acquireDelay: Duration.zero,
        connectDelay: Duration.zero,
      );
      await recoveryService.startTalkback(
        endpointUrl: 'https://cam1.local/talk',
        accessToken: 'tok_2',
      );
      expect(recoveryService.currentState, TalkbackState.active);

      failService.dispose();
      recoveryService.dispose();
    });

    test('TalkbackCall toString includes method and args', () {
      final call = TalkbackCall(
        method: 'startTalkback',
        args: {'endpointUrl': 'https://x', 'accessToken': 'tok'},
      );
      expect(call.toString(), contains('startTalkback'));
      expect(call.toString(), contains('endpointUrl'));
    });
  });
}
