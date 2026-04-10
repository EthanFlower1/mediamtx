// Integration test: Multi-feature state orchestration.
//
// End-to-end flow across 6 service layers:
//   login -> load cameras -> apply offline mode -> queue actions ->
//   go online -> verify actions drain
//
// This test exercises the interaction between AppSession, CameraDirectoryClient,
// ConnectivityMonitor, ActionQueue, and EventsClient using only the existing
// Fake* implementations (no mockito).

import 'package:flutter_test/flutter_test.dart';

import 'package:nvr_client/cameras/camera_directory_client.dart';
import 'package:nvr_client/events/events_client.dart';
import 'package:nvr_client/events/events_model.dart';
import 'package:nvr_client/models/camera.dart';
import 'package:nvr_client/offline/action_queue.dart';
import 'package:nvr_client/offline/connectivity_monitor.dart';
import 'package:nvr_client/playback/playback_client.dart';
import 'package:nvr_client/state/app_session.dart';
import 'package:nvr_client/state/home_directory_connection.dart';
import 'package:nvr_client/state/secure_token_store.dart';

void main() {
  group('Multi-feature state integration', () {
    late InMemorySecureTokenStore tokenStore;
    late AppSessionNotifier sessionNotifier;
    late FakeCameraDirectoryClient cameraClient;
    late ConnectivityMonitor connectivity;
    late ActionQueue actionQueue;
    late FakePlaybackClient playbackClient;

    final connection = HomeDirectoryConnection(
      id: 'conn-1',
      kind: HomeConnectionKind.onPrem,
      endpointUrl: 'https://nvr.acme.local',
      displayName: 'Acme HQ',
      discoveryMethod: DiscoveryMethod.mdns,
    );

    final now = DateTime.now().toUtc();

    setUp(() {
      tokenStore = InMemorySecureTokenStore();
      sessionNotifier = AppSessionNotifier(tokenStore);
      cameraClient = FakeCameraDirectoryClient();
      connectivity = ConnectivityMonitor();
      actionQueue = ActionQueue();
      playbackClient = FakePlaybackClient();
    });

    tearDown(() async {
      connectivity.dispose();
      await cameraClient.dispose();
    });

    test(
        'full lifecycle: login, cameras, offline, queue actions, online, drain',
        () async {
      // ---- Step 1: Login ----
      await sessionNotifier.activateConnection(
        connection: connection,
        userId: 'user-1',
        tenantRef: 'tenant-acme',
      );
      await sessionNotifier.setTokens(
        accessToken: 'eyJhbGciOiJSUzI1NiJ9.eyJzdWIiOiJ1c2VyLTEifQ.sig',
        refreshToken: 'refresh-tok',
      );
      expect(sessionNotifier.state.isAuthenticated, isTrue);

      // ---- Step 2: Load cameras ----
      cameraClient.setCameras(connection.id, [
        const Camera(id: 'cam-1', name: 'Lobby'),
        const Camera(id: 'cam-2', name: 'Parking'),
      ]);

      final cameras = await cameraClient.listCameras(
        sessionNotifier.state.activeConnection!,
      );
      expect(cameras, hasLength(2));

      // ---- Step 3: Go offline ----
      final t0 = DateTime.utc(2026, 4, 8, 12, 0);
      connectivity.transitionWithTimestamp(ConnectivityState.offline, t0);
      expect(connectivity.state, ConnectivityState.offline);

      // ---- Step 4: Queue actions while offline ----
      actionQueue.enqueue(QueuedAction(
        id: 'action-1',
        actionType: ActionType.apiCall,
        payload: {'method': 'POST', 'path': '/api/v1/bookmarks', 'body': {}},
        createdAt: now,
      ));
      actionQueue.enqueue(QueuedAction(
        id: 'action-2',
        actionType: ActionType.stateUpdate,
        payload: {'camera_id': 'cam-1', 'name': 'Lobby (Renamed)'},
        createdAt: now.add(const Duration(seconds: 1)),
      ));
      actionQueue.enqueue(QueuedAction(
        id: 'action-3',
        actionType: ActionType.notification,
        payload: {'title': 'Bookmark saved', 'body': 'Will sync when online'},
        createdAt: now.add(const Duration(seconds: 2)),
      ));

      expect(actionQueue.state, hasLength(3));
      expect(actionQueue.peek()!.id, 'action-1');

      // ---- Step 5: Go online ----
      final t1 = t0.add(const Duration(seconds: 3));
      connectivity.transitionWithTimestamp(ConnectivityState.online, t1);
      expect(connectivity.state, ConnectivityState.online);

      // ---- Step 6: Drain the queue ----
      final drained = actionQueue.drain();
      expect(drained, hasLength(3));
      expect(drained[0].actionType, ActionType.apiCall);
      expect(drained[1].actionType, ActionType.stateUpdate);
      expect(drained[2].actionType, ActionType.notification);

      // Queue is now empty
      expect(actionQueue.state, isEmpty);
      expect(actionQueue.peek(), isNull);
      expect(actionQueue.dequeue(), isNull);
    });

    test('action retry count increments correctly', () {
      final action = QueuedAction(
        id: 'action-retry',
        actionType: ActionType.apiCall,
        payload: {'path': '/api/v1/bookmarks'},
        createdAt: now,
      );

      expect(action.retryCount, 0);

      final retry1 = action.incrementRetry();
      expect(retry1.retryCount, 1);
      expect(retry1.id, action.id);

      final retry2 = retry1.incrementRetry();
      expect(retry2.retryCount, 2);

      final retry3 = retry2.incrementRetry();
      expect(retry3.retryCount, 3);
      expect(retry3.retryCount, ActionQueue.maxRetries);
    });

    test('dequeue processes actions in FIFO order', () {
      actionQueue.enqueue(QueuedAction(
        id: 'first',
        actionType: ActionType.apiCall,
        payload: {},
        createdAt: now,
      ));
      actionQueue.enqueue(QueuedAction(
        id: 'second',
        actionType: ActionType.stateUpdate,
        payload: {},
        createdAt: now.add(const Duration(seconds: 1)),
      ));

      final first = actionQueue.dequeue();
      expect(first!.id, 'first');

      final second = actionQueue.dequeue();
      expect(second!.id, 'second');

      expect(actionQueue.dequeue(), isNull);
    });

    test('removeById removes specific action from queue', () {
      actionQueue.enqueue(QueuedAction(
        id: 'keep',
        actionType: ActionType.apiCall,
        payload: {},
        createdAt: now,
      ));
      actionQueue.enqueue(QueuedAction(
        id: 'remove-me',
        actionType: ActionType.notification,
        payload: {},
        createdAt: now,
      ));
      actionQueue.enqueue(QueuedAction(
        id: 'also-keep',
        actionType: ActionType.stateUpdate,
        payload: {},
        createdAt: now,
      ));

      actionQueue.removeById('remove-me');
      expect(actionQueue.state, hasLength(2));
      expect(actionQueue.state.map((a) => a.id).toList(), ['keep', 'also-keep']);
    });

    test('events + playback work alongside offline queue', () async {
      // Login
      await sessionNotifier.activateConnection(
        connection: connection,
        userId: 'user-1',
        tenantRef: 'tenant-acme',
      );
      await sessionNotifier.setTokens(
        accessToken: 'eyJhbGciOiJSUzI1NiJ9.eyJzdWIiOiJ1c2VyLTEifQ.sig',
        refreshToken: 'refresh-tok',
      );

      // Events client works
      final eventsClient = FakeEventsClient([
        EventSummary(
          id: 'evt-1',
          timestamp: now,
          cameraId: 'cam-1',
          cameraName: 'Lobby',
          kind: 'motion',
          severity: EventSeverity.info,
          tenantId: 'tenant-acme',
        ),
      ]);

      final page = await eventsClient.list(
        tenantId: 'tenant-acme',
        filter: const EventFilter(),
      );
      expect(page.items, hasLength(1));

      // Playback client works
      final bookmark = await playbackClient.createBookmark(
        segmentId: 'seg-1',
        atMs: 5000,
        note: 'Important moment',
      );
      expect(bookmark, isNotEmpty);
      expect(playbackClient.lastCall, 'createBookmark');
      expect(playbackClient.bookmarkCounter, 1);

      // All while offline queue has pending items
      actionQueue.enqueue(QueuedAction(
        id: 'bookmark-sync',
        actionType: ActionType.apiCall,
        payload: {'bookmark_id': bookmark, 'segment_id': 'seg-1'},
        createdAt: now,
      ));
      expect(actionQueue.state, hasLength(1));
    });

    test('JWT groups claim is parsed on token set', () async {
      await sessionNotifier.activateConnection(
        connection: connection,
        userId: 'user-1',
        tenantRef: 'tenant-acme',
      );

      // Create a JWT with groups claim
      // Header: {"alg":"RS256"}
      // Payload: {"sub":"user-1","groups":["admin","operator"]}
      // Base64url encode the payload:
      //   eyJzdWIiOiJ1c2VyLTEiLCJncm91cHMiOlsiYWRtaW4iLCJvcGVyYXRvciJdfQ
      const jwt =
          'eyJhbGciOiJSUzI1NiJ9.eyJzdWIiOiJ1c2VyLTEiLCJncm91cHMiOlsiYWRtaW4iLCJvcGVyYXRvciJdfQ.sig';

      await sessionNotifier.setTokens(
        accessToken: jwt,
        refreshToken: 'refresh-tok',
      );

      expect(sessionNotifier.state.groups, contains('admin'));
      expect(sessionNotifier.state.groups, contains('operator'));
      expect(sessionNotifier.state.groups, hasLength(2));
    });

    test('logout clears tokens and groups but preserves queue', () async {
      await sessionNotifier.activateConnection(
        connection: connection,
        userId: 'user-1',
        tenantRef: 'tenant-acme',
      );
      await sessionNotifier.setTokens(
        accessToken: 'eyJhbGciOiJSUzI1NiJ9.eyJzdWIiOiJ1c2VyLTEifQ.sig',
        refreshToken: 'refresh-tok',
      );

      actionQueue.enqueue(QueuedAction(
        id: 'pending',
        actionType: ActionType.apiCall,
        payload: {},
        createdAt: now,
      ));

      await sessionNotifier.logout();

      expect(sessionNotifier.state.accessToken, isNull);
      expect(sessionNotifier.state.refreshToken, isNull);
      expect(sessionNotifier.state.isAuthenticated, isFalse);
      expect(sessionNotifier.state.groups, isEmpty);

      // Action queue is independent of session -- still has items
      expect(actionQueue.state, hasLength(1));
    });
  });
}
