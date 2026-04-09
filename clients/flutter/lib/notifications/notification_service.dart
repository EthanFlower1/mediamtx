// KAI-303 — NotificationService: wires PushChannel + PushSubscriptionClient
// + AppSession into a single object the UI layer watches via Riverpod.
//
// Responsibilities:
//   1. On session activation (and on SessionSwitchedEvent — see the
//      minimal stub below), fetch the device token from the active
//      [PushChannel] and register it against the active directory via
//      [PushSubscriptionClient.registerDevice].
//   2. Forward incoming [PushMessage]s to a broadcast stream exposed via
//      [notificationStreamProvider] for the UI layer.
//   3. Enforce the metadata-only payload contract at receive time — any
//      incoming PushMessage that somehow carries binary is dropped with a
//      visible debugPrint rather than crashing the app.
//
// Deep-link note: the service does NOT perform navigation. Tap handling
// calls `routeForPushMessage` from `deep_link.dart` and hands the resulting
// route name to whatever navigator the UI layer is using.
//
// Thumbnail note: if a PushMessage carries a `thumbnailUrl`, the UI layer
// is expected to fetch that URL via an authenticated API call WHEN THE
// USER TAPS THE NOTIFICATION — never from the push payload itself. The
// backend dispatcher must therefore sign the thumbnail URL such that it's
// only usable by a logged-in client. This contract is the metadata-only
// half of the security design.

import 'dart:async';

import 'package:flutter/foundation.dart';
import 'package:flutter_riverpod/flutter_riverpod.dart';

import '../state/app_session.dart';
import 'notification_strings.dart';
import 'push_channel.dart';
import 'push_message.dart';
import 'push_subscription_client.dart';

/// Minimal local stub for the `SessionSwitchedEvent` that KAI-304 will add
/// to AppSessionNotifier. KAI-304 is not yet in `main` at the time of this
/// PR, so we define the event shape here and wire to it once it lands.
///
/// When KAI-304 lands, delete this stub and import the real event from
/// `state/app_session.dart`.
class SessionSwitchedEvent {
  /// Previous active connection id, or `null` if there was none.
  final String? fromConnectionId;

  /// New active connection id, never null.
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

/// The service itself. Constructed with concrete PushChannel +
/// PushSubscriptionClient implementations so tests can inject fakes.
class NotificationService extends StateNotifier<NotificationServiceState> {
  final PushChannel channel;
  final PushSubscriptionClient subscriptionClient;
  final NotificationStrings strings;

  /// Callback invoked when Firebase is not initialised. Defaults to
  /// debugPrint the localised warning; tests override to assert it fired.
  final void Function(String warning)? firebaseNotInitialisedWarner;

  /// Whether Firebase (or equivalent platform init) has completed. In this
  /// scaffold it defaults to `false` because actual init is deferred to a
  /// credential-landing follow-up. Set to `true` once that lands.
  final bool firebaseInitialised;

  StreamSubscription<PushMessage>? _incomingSub;
  final _outController = StreamController<PushMessage>.broadcast();

  NotificationService({
    required this.channel,
    required this.subscriptionClient,
    this.strings = NotificationStrings.en,
    this.firebaseInitialised = false,
    this.firebaseNotInitialisedWarner,
  }) : super(const NotificationServiceState());

  /// Broadcast stream of incoming push messages.
  Stream<PushMessage> get incoming => _outController.stream;

  /// Start the service for the given active connection + device token
  /// binding. Idempotent across repeat calls for the same connection.
  Future<void> start({required String directoryConnectionId}) async {
    if (!firebaseInitialised) {
      (firebaseNotInitialisedWarner ?? debugPrint)(
        strings.firebaseNotInitialisedWarning,
      );
      // Continue anyway — a stub channel will simply yield no token.
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
  /// switches. This is the hook the (future) SessionSwitchedEvent will
  /// trigger.
  Future<void> onSessionSwitched(SessionSwitchedEvent event) async {
    // Drop any prior subscription id — it was bound to the old connection.
    state = state.copyWith(subscriptionId: null, clearError: true);
    await start(directoryConnectionId: event.toConnectionId);
  }

  /// Subscribe to events of the given [eventKinds] on [cameraId] for the
  /// currently registered device. Throws [StateError] if the service has
  /// not registered a subscription yet.
  Future<void> subscribeCamera({
    required String cameraId,
    required Set<PushMessageKind> eventKinds,
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

  /// Unsubscribe [cameraId] for the currently registered device.
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

  void _onIncoming(PushMessage m) {
    // Metadata-only re-check — PushMessage's constructor already rejects
    // data: URIs, but we defensively re-validate here so a corrupt native
    // bridge can't smuggle binary through.
    final url = m.thumbnailUrl;
    if (url != null && !url.startsWith('https://')) {
      debugPrint(
        'NotificationService: dropping message with non-https thumbnailUrl '
        '(metadata-only contract violation).',
      );
      return;
    }
    _outController.add(m);
  }

  @override
  void dispose() {
    _incomingSub?.cancel();
    _outController.close();
    // Do not stop `channel` here — the channel is owned by the Riverpod
    // provider and may be reused across notifier rebuilds.
    super.dispose();
  }
}

// ------------------------- Riverpod providers -------------------------

/// Override these in app startup (main.dart / productionOverrides) to wire
/// the real PushChannel + PushSubscriptionClient implementations. Tests
/// override with fakes.
final pushChannelProvider = Provider<PushChannel>((ref) {
  // Defaulting to a Desktop stub keeps tests hermetic until a real platform
  // factory is wired. Production should override this with
  // `currentPlatformPushChannel()`.
  return DesktopPushChannel();
});

final pushSubscriptionClientProvider = Provider<PushSubscriptionClient>((ref) {
  throw UnimplementedError(
    'pushSubscriptionClientProvider must be overridden by the host app. '
    'The real implementation is a lead-cloud deliverable — see '
    'pkg/api/v1/notifications.proto (TBD).',
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
      strings: ref.watch(notificationStringsProvider),
    );
    // Auto-start whenever the active connection exists. This is the hook
    // point where the (future) SessionSwitchedEvent will fire a re-register.
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

/// UI-facing stream of incoming push messages.
final notificationStreamProvider = StreamProvider<PushMessage>((ref) {
  final notifier = ref.watch(notificationServiceProvider.notifier);
  return notifier.incoming;
});
