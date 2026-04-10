// KAI-303 — NotificationService: wires PushChannel + PushSubscriptionClient
// + EventDetailsLoader + AppSession into a single object the UI layer
// watches via Riverpod.
//
// Hard contract (cto + lead-security gate on PR #165):
//
//   Incoming PushMessages carry ONLY (event_id, tenant_id, priority) and
//   are emitted to `notificationStreamProvider` UNCHANGED. The UI layer
//   decides when to fetch full details.
//
//   When the user taps a notification, the UI layer calls
//   [NotificationService.resolveForTap]. That method:
//     1. verifies the push's tenantId matches the active AppSession's
//        tenantRef (throws [CrossTenantPushViolation] on mismatch), and
//     2. delegates to [EventDetailsLoader.loadEvent] — the SINGLE
//        authorized path from a PushMessage to user-visible data.
//
//   The foreground handler formats a visible notification using the
//   fetched EventDetails (never the raw push), with i18n'd title/body
//   from [NotificationStrings.titleForKind].
//
// FCM trade-off (documented per brief): FCM payloads are data-only, so
// when the Android app is killed the FCM background isolate is the only
// thing that can render the visible notification client-side. The
// system's default auto-display (the `notification` block) is OFF by
// design — that block would leak PII to the lock screen. HIPAA-correct
// behavior. See AndroidManifest.xml comment block.

import 'dart:async';

import 'package:flutter/foundation.dart';
import 'package:flutter_riverpod/flutter_riverpod.dart';

import '../state/app_session.dart';
import 'event_details_loader.dart';
import 'notification_strings.dart';
import 'push_channel.dart';
import 'push_event_kind.dart';
import 'push_message.dart';
import 'push_subscription_client.dart';

/// Minimal local stub for the `SessionSwitchedEvent` that KAI-304 will add
/// to AppSessionNotifier. When KAI-304 lands, delete this stub and import
/// the real event from `state/app_session.dart`.
class SessionSwitchedEvent {
  final String? fromConnectionId;
  final String toConnectionId;

  const SessionSwitchedEvent({
    required this.fromConnectionId,
    required this.toConnectionId,
  });
}

/// State held by [NotificationService]. Exposed as a Riverpod-watchable
/// snapshot so UI can show the registered-device banner / permission
/// banner.
class NotificationServiceState {
  final bool started;
  final String? deviceToken;
  final String? subscriptionId;
  final String? lastError;

  const NotificationServiceState({
    this.started = false,
    this.deviceToken,
    this.subscriptionId,
    this.lastError,
  });

  NotificationServiceState copyWith({
    bool? started,
    String? deviceToken,
    String? subscriptionId,
    String? lastError,
    bool clearError = false,
  }) {
    return NotificationServiceState(
      started: started ?? this.started,
      deviceToken: deviceToken ?? this.deviceToken,
      subscriptionId: subscriptionId ?? this.subscriptionId,
      lastError: clearError ? null : (lastError ?? this.lastError),
    );
  }
}

/// A resolved foreground notification, ready to be displayed to the user.
/// Built from the fetched [EventDetails] via i18n'd [NotificationStrings].
class ForegroundNotification {
  final String title;
  final String body;
  final String? thumbnailUrl;
  final EventDetails event;

  const ForegroundNotification({
    required this.title,
    required this.body,
    required this.event,
    this.thumbnailUrl,
  });
}

/// The service itself. Constructed with concrete PushChannel +
/// PushSubscriptionClient + EventDetailsLoader + AppSession reader so
/// tests can inject fakes.
class NotificationService extends StateNotifier<NotificationServiceState> {
  final PushChannel channel;
  final PushSubscriptionClient subscriptionClient;
  final EventDetailsLoader eventDetailsLoader;
  final NotificationStrings strings;

  /// Reads the CURRENT AppSession on every call — the cross-tenant guard
  /// must not capture a stale session.
  final AppSession Function() readAppSession;

  /// Callback invoked when Firebase is not initialised.
  final void Function(String warning)? firebaseNotInitialisedWarner;

  /// Whether Firebase (or equivalent platform init) has completed.
  final bool firebaseInitialised;

  StreamSubscription<PushMessage>? _incomingSub;
  final _outController = StreamController<PushMessage>.broadcast();

  NotificationService({
    required this.channel,
    required this.subscriptionClient,
    required this.eventDetailsLoader,
    required this.readAppSession,
    this.strings = NotificationStrings.en,
    this.firebaseInitialised = false,
    this.firebaseNotInitialisedWarner,
  }) : super(const NotificationServiceState());

  /// Broadcast stream of incoming OPAQUE push messages (event_id +
  /// tenant_id + priority only).
  Stream<PushMessage> get incoming => _outController.stream;

  /// Start the service for the given active connection + device token
  /// binding. Idempotent across repeat calls for the same connection.
  Future<void> start({required String directoryConnectionId}) async {
    if (!firebaseInitialised) {
      (firebaseNotInitialisedWarner ?? debugPrint)(
        strings.firebaseNotInitialisedWarning,
      );
    }

    await channel.start();
    _incomingSub ??= channel.incoming.listen(_onIncoming);

    try {
      final token = await channel.getDeviceToken();
      if (token == null || token.isEmpty) {
        state = state.copyWith(
          started: true,
          lastError: strings.errorRegisterFailed,
        );
        return;
      }
      final subId = await subscriptionClient.registerDevice(
        deviceToken: token,
        platform: channel.platformTag,
        directoryConnectionId: directoryConnectionId,
      );
      state = state.copyWith(
        started: true,
        deviceToken: token,
        subscriptionId: subId,
        clearError: true,
      );
    } catch (e) {
      state = state.copyWith(
        started: true,
        lastError: strings.errorRegisterFailed,
      );
    }
  }

  /// Re-register against the new active connection when the session
  /// switches.
  Future<void> onSessionSwitched(SessionSwitchedEvent event) async {
    state = state.copyWith(subscriptionId: null, clearError: true);
    await start(directoryConnectionId: event.toConnectionId);
  }

  /// Subscribe to events of the given [eventKinds] on [cameraId].
  Future<void> subscribeCamera({
    required String cameraId,
    required Set<PushEventKind> eventKinds,
  }) async {
    final subId = state.subscriptionId;
    if (subId == null) {
      throw StateError('NotificationService.subscribeCamera before register');
    }
    await subscriptionClient.subscribe(
      subscriptionId: subId,
      cameraId: cameraId,
      eventKinds: eventKinds,
    );
  }

  Future<void> unsubscribeCamera({required String cameraId}) async {
    final subId = state.subscriptionId;
    if (subId == null) {
      throw StateError('NotificationService.unsubscribeCamera before register');
    }
    await subscriptionClient.unsubscribe(
      subscriptionId: subId,
      cameraId: cameraId,
    );
  }

  Future<List<PushSubscription>> listSubscriptions() async {
    final subId = state.subscriptionId;
    if (subId == null) return const [];
    return subscriptionClient.listSubscriptions(subId);
  }

  /// THE single authorized path from a PushMessage to user-visible event
  /// data. Called on notification tap.
  ///
  /// Throws [CrossTenantPushViolation] if the push message's tenantId
  /// does not match the current AppSession's tenantRef — preventing a
  /// stale push from fetching event details against a different tenant's
  /// Directory.
  Future<EventDetails> resolveForTap(PushMessage msg) async {
    final session = readAppSession();
    if (session.tenantRef.isEmpty) {
      throw CrossTenantPushViolation(
        'No active tenant in AppSession — refusing to resolve push '
        'eventId=${msg.eventId}',
      );
    }
    if (session.tenantRef != msg.tenantId) {
      throw CrossTenantPushViolation(
        'Push tenantId=${msg.tenantId} does not match active '
        'AppSession.tenantRef=${session.tenantRef}. Refusing cross-tenant '
        'fetch for eventId=${msg.eventId}.',
      );
    }
    return eventDetailsLoader.loadEvent(msg.eventId);
  }

  /// Build a ForegroundNotification for a tapped push, using i18n'd
  /// strings. This is the foreground handler's rendering step — called
  /// after [resolveForTap] returns successfully. The formatted title/body
  /// NEVER come from the raw push payload, only from the fetched
  /// [EventDetails] (which in turn came from an authenticated Directory
  /// call).
  ForegroundNotification formatForegroundNotification(EventDetails details) {
    final title = strings.titleForKind(details.kind);
    final body = details.cameraLabel.isEmpty
        ? strings.fallbackNotificationBody
        : details.cameraLabel;
    return ForegroundNotification(
      title: title,
      body: body,
      thumbnailUrl: details.thumbnailUrl,
      event: details,
    );
  }

  void _onIncoming(PushMessage m) {
    // Metadata-only contract is enforced at the factory boundary
    // (PushMessage.fromRemote). By the time a PushMessage reaches here,
    // it is already guaranteed to be opaque — we simply fan it out.
    _outController.add(m);
  }

  @override
  void dispose() {
    _incomingSub?.cancel();
    _outController.close();
    super.dispose();
  }
}

// ------------------------- Riverpod providers -------------------------

/// Override in production with `currentPlatformPushChannel()`.
final pushChannelProvider = Provider<PushChannel>((ref) {
  return DesktopPushChannel();
});

final pushSubscriptionClientProvider = Provider<PushSubscriptionClient>((ref) {
  throw UnimplementedError(
    'pushSubscriptionClientProvider must be overridden by the host app. '
    'The real implementation is a lead-cloud deliverable — see '
    'cloud.push.v1.Dispatcher proto ask in PR #165.',
  );
});

final eventDetailsLoaderProvider = Provider<EventDetailsLoader>((ref) {
  throw UnimplementedError(
    'eventDetailsLoaderProvider must be overridden by the host app. '
    'Production wires HttpEventDetailsLoader(session, http.Client()). '
    'The canonical proto is GetEvent(event_id) in cloud.directory.v1.Events '
    '(see PR #165, waiting on lead-cloud).',
  );
});

final notificationStringsProvider = Provider<NotificationStrings>((ref) {
  return NotificationStrings.en;
});

final notificationServiceProvider =
    StateNotifierProvider<NotificationService, NotificationServiceState>(
  (ref) {
    final service = NotificationService(
      channel: ref.watch(pushChannelProvider),
      subscriptionClient: ref.watch(pushSubscriptionClientProvider),
      eventDetailsLoader: ref.watch(eventDetailsLoaderProvider),
      readAppSession: () => ref.read(appSessionProvider),
      strings: ref.watch(notificationStringsProvider),
    );
    // Auto-start whenever the active connection exists.
    ref.listen<AppSession>(appSessionProvider, (previous, next) {
      final prevId = previous?.activeConnection?.id;
      final nextId = next.activeConnection?.id;
      if (nextId == null) return;
      if (prevId == nextId) return;
      // ignore: discarded_futures
      service.onSessionSwitched(
        SessionSwitchedEvent(fromConnectionId: prevId, toConnectionId: nextId),
      );
    });
    return service;
  },
);

/// UI-facing stream of incoming opaque push messages.
final notificationStreamProvider = StreamProvider<PushMessage>((ref) {
  final notifier = ref.watch(notificationServiceProvider.notifier);
  return notifier.incoming;
});
