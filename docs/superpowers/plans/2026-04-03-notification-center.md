# KAI-66: Notification Center Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Add a fully functional notification center to the Flutter NVR client with type icons, tap-to-navigate, persistent read state, and mobile access.

**Architecture:** Enhance the existing `NotificationEvent` model with icon/navigation helpers, update `AlertsPanel` to use type-specific icons and navigate on tap, add `shared_preferences` persistence to `NotificationsNotifier`, and add a notification bell to the mobile layout via an app bar in `NavigationShell`.

**Tech Stack:** Flutter, Riverpod, go_router, shared_preferences, WebSocket

---

## File Structure

| File | Action | Responsibility |
|------|--------|----------------|
| `clients/flutter/lib/models/notification_event.dart` | Modify | Add icon getter, navigation route getter |
| `clients/flutter/lib/widgets/alerts_panel.dart` | Modify | Type icons, tap-to-navigate, close panel on tap |
| `clients/flutter/lib/providers/notifications_provider.dart` | Modify | Persist read IDs via shared_preferences |
| `clients/flutter/lib/widgets/shell/navigation_shell.dart` | Modify | Add mobile app bar with notification bell |

---

### Task 1: Add icon and navigation helpers to NotificationEvent model

**Files:**
- Modify: `clients/flutter/lib/models/notification_event.dart`

- [ ] **Step 1: Add a unique `id` field and type icon getter**

The model needs a stable ID for persistence and an icon getter for display. Add these to `notification_event.dart`:

```dart
import 'package:flutter/material.dart';

class NotificationEvent {
  final String id;
  final String type;
  final String camera;
  final String message;
  final DateTime time;
  final String? zone;
  final String? className;
  final String? action;
  final String? trackId;
  final double? confidence;
  final bool isRead;

  const NotificationEvent({
    required this.id,
    required this.type,
    required this.camera,
    required this.message,
    required this.time,
    this.zone,
    this.className,
    this.action,
    this.trackId,
    this.confidence,
    this.isRead = false,
  });

  factory NotificationEvent.fromJson(Map<String, dynamic> json) {
    return NotificationEvent(
      id: json['id'] as String? ??
          '${json['type']}_${json['camera']}_${json['time'] ?? DateTime.now().toIso8601String()}',
      type: json['type'] as String? ?? '',
      camera: json['camera'] as String? ?? '',
      message: json['message'] as String? ?? '',
      time: json['time'] != null
          ? DateTime.parse(json['time'] as String)
          : DateTime.now(),
      zone: json['zone'] as String?,
      className: json['class'] as String?,
      action: json['action'] as String?,
      trackId: json['trackId'] as String?,
      confidence: (json['confidence'] as num?)?.toDouble(),
      isRead: json['isRead'] as bool? ?? false,
    );
  }

  NotificationEvent copyWith({bool? isRead}) {
    return NotificationEvent(
      id: id,
      type: type,
      camera: camera,
      message: message,
      time: time,
      zone: zone,
      className: className,
      action: action,
      trackId: trackId,
      confidence: confidence,
      isRead: isRead ?? this.isRead,
    );
  }

  bool get isDetectionFrame => type == 'detection_frame';

  bool get isAlert => type == 'alert';

  /// Returns a Material icon appropriate for this notification type.
  IconData get typeIcon {
    switch (type) {
      case 'motion':
        return Icons.directions_run;
      case 'camera_offline':
        return Icons.videocam_off;
      case 'camera_online':
        return Icons.videocam;
      case 'alert':
        return Icons.warning_amber;
      case 'detection_frame':
        return Icons.center_focus_strong;
      case 'recording_started':
        return Icons.fiber_manual_record;
      case 'recording_stopped':
        return Icons.stop_circle_outlined;
      default:
        return Icons.notifications_outlined;
    }
  }

  /// Returns the go_router path this notification should navigate to, or null
  /// if no specific destination applies.
  String? get navigationRoute {
    switch (type) {
      case 'camera_offline':
      case 'camera_online':
        return '/devices/$camera';
      case 'motion':
      case 'detection_frame':
      case 'alert':
        final ts = time.toIso8601String();
        return '/playback?cameraId=$camera&timestamp=$ts';
      default:
        return null;
    }
  }
}
```

- [ ] **Step 2: Verify no compile errors**

Run: `cd clients/flutter && flutter analyze lib/models/notification_event.dart`
Expected: No issues found

- [ ] **Step 3: Commit**

```bash
git add clients/flutter/lib/models/notification_event.dart
git commit -m "feat(notifications): add id, type icon, and navigation route to NotificationEvent"
```

---

### Task 2: Update AlertsPanel with type icons and tap-to-navigate

**Files:**
- Modify: `clients/flutter/lib/widgets/alerts_panel.dart`

- [ ] **Step 1: Add go_router import and update _NotificationItem**

In `alerts_panel.dart`, add the `go_router` import at the top, then replace the `_NotificationItem` class to use type icons and navigate on tap. Also update the `_AlertsPanelContent` to pass context/ref for navigation and panel closing.

Replace the existing `import` block (add `go_router`):

```dart
import 'package:flutter/material.dart';
import 'package:flutter_riverpod/flutter_riverpod.dart';
import 'package:go_router/go_router.dart';
import '../theme/nvr_colors.dart';
import '../theme/nvr_typography.dart';
import '../providers/notifications_provider.dart';
import '../models/notification_event.dart';
import 'hud/hud_button.dart';
```

Replace the `_AlertsPanelContent` build method's `itemBuilder` to pass navigation callback:

```dart
itemBuilder: (context, index) {
  final event = history[index];
  return _NotificationItem(
    event: event,
    onTap: () {
      notifier.markRead(index);
      final route = event.navigationRoute;
      if (route != null) {
        // Close the panel first
        onClose();
        context.go(route);
      }
    },
  );
},
```

Replace the `_NotificationItem` widget's status dot with a type icon. Change the `_dotColor` getter to `_iconColor` and replace the dot `Container` with an `Icon`:

```dart
class _NotificationItem extends StatelessWidget {
  const _NotificationItem({required this.event, required this.onTap});

  final NotificationEvent event;
  final VoidCallback onTap;

  Color get _iconColor {
    switch (event.type) {
      case 'motion':
        return NvrColors.accent;
      case 'camera_offline':
        return NvrColors.danger;
      case 'camera_online':
        return NvrColors.success;
      case 'alert':
        return NvrColors.warning;
      default:
        return NvrColors.textSecondary;
    }
  }

  String _timeAgo(DateTime time) {
    final diff = DateTime.now().difference(time);
    if (diff.inSeconds < 60) return '${diff.inSeconds}s ago';
    if (diff.inMinutes < 60) return '${diff.inMinutes}m ago';
    if (diff.inHours < 24) return '${diff.inHours}h ago';
    return '${diff.inDays}d ago';
  }

  @override
  Widget build(BuildContext context) {
    final bg = event.isRead ? NvrColors.bgSecondary : NvrColors.bgTertiary;
    final hasRoute = event.navigationRoute != null;

    return Material(
      color: bg,
      child: InkWell(
        onTap: onTap,
        splashColor: NvrColors.accent.withOpacity(0.08),
        highlightColor: NvrColors.accent.withOpacity(0.04),
        child: Padding(
          padding: const EdgeInsets.symmetric(horizontal: 12, vertical: 10),
          child: Row(
            crossAxisAlignment: CrossAxisAlignment.start,
            children: [
              // Type icon
              Padding(
                padding: const EdgeInsets.only(top: 1),
                child: Icon(
                  event.typeIcon,
                  size: 16,
                  color: _iconColor,
                ),
              ),
              const SizedBox(width: 10),
              // Message + meta
              Expanded(
                child: Column(
                  crossAxisAlignment: CrossAxisAlignment.start,
                  children: [
                    Text(
                      event.message,
                      style: NvrTypography.body.copyWith(
                        color: event.isRead
                            ? NvrColors.textSecondary
                            : NvrColors.textPrimary,
                      ),
                    ),
                    const SizedBox(height: 3),
                    Text(
                      '${event.camera} \u00b7 ${_timeAgo(event.time)}',
                      style: NvrTypography.monoLabel,
                    ),
                  ],
                ),
              ),
              // Navigate arrow for actionable notifications
              if (hasRoute && !event.isRead)
                Padding(
                  padding: const EdgeInsets.only(top: 2, left: 8),
                  child: Icon(
                    Icons.chevron_right,
                    size: 14,
                    color: NvrColors.textMuted,
                  ),
                )
              else if (!event.isRead)
                Padding(
                  padding: const EdgeInsets.only(top: 4, left: 8),
                  child: Container(
                    width: 5,
                    height: 5,
                    decoration: const BoxDecoration(
                      color: NvrColors.accent,
                      shape: BoxShape.circle,
                    ),
                  ),
                ),
            ],
          ),
        ),
      ),
    );
  }
}
```

- [ ] **Step 2: Verify no compile errors**

Run: `cd clients/flutter && flutter analyze lib/widgets/alerts_panel.dart`
Expected: No issues found

- [ ] **Step 3: Commit**

```bash
git add clients/flutter/lib/widgets/alerts_panel.dart
git commit -m "feat(notifications): add type icons and tap-to-navigate in alerts panel"
```

---

### Task 3: Add shared_preferences persistence for read state

**Files:**
- Modify: `clients/flutter/lib/providers/notifications_provider.dart`

- [ ] **Step 1: Add persistence to NotificationsNotifier**

The notifier needs to load/save read notification IDs to `shared_preferences`. Since events arrive via WebSocket and have no server-side persistence of read state, we store the set of read event IDs locally.

Replace the entire file:

```dart
import 'dart:async';
import 'package:flutter_riverpod/flutter_riverpod.dart';
import 'package:shared_preferences/shared_preferences.dart';
import '../models/notification_event.dart';
import '../services/websocket_service.dart';
import 'auth_provider.dart';

class NotificationState {
  final List<NotificationEvent> history;
  final int unreadCount;
  final bool wsConnected;

  const NotificationState({
    this.history = const [],
    this.unreadCount = 0,
    this.wsConnected = false,
  });

  NotificationState copyWith({
    List<NotificationEvent>? history,
    int? unreadCount,
    bool? wsConnected,
  }) {
    return NotificationState(
      history: history ?? this.history,
      unreadCount: unreadCount ?? this.unreadCount,
      wsConnected: wsConnected ?? this.wsConnected,
    );
  }
}

class NotificationsNotifier extends StateNotifier<NotificationState> {
  static const _readIdsKey = 'nvr_read_notification_ids';
  static const _maxReadIds = 200;

  WebSocketService? _webSocket;
  StreamSubscription<NotificationEvent>? _eventsSub;
  StreamSubscription<bool>? _connectionSub;
  Set<String> _readIds = {};

  NotificationsNotifier() : super(const NotificationState()) {
    _loadReadIds();
  }

  WebSocketService? get webSocket => _webSocket;

  Future<void> _loadReadIds() async {
    final prefs = await SharedPreferences.getInstance();
    final ids = prefs.getStringList(_readIdsKey);
    if (ids != null) {
      _readIds = ids.toSet();
    }
  }

  Future<void> _saveReadIds() async {
    final prefs = await SharedPreferences.getInstance();
    // Cap stored IDs to prevent unbounded growth
    final ids = _readIds.toList();
    if (ids.length > _maxReadIds) {
      _readIds = ids.sublist(ids.length - _maxReadIds).toSet();
    }
    await prefs.setStringList(_readIdsKey, _readIds.toList());
  }

  void connect(String serverUrl) {
    _cleanup();

    _webSocket = WebSocketService(serverUrl: serverUrl);

    _connectionSub = _webSocket!.connectionState.listen((connected) {
      if (mounted) {
        state = state.copyWith(wsConnected: connected);
      }
    });

    _eventsSub = _webSocket!.events.listen((event) {
      if (mounted) {
        // Apply persisted read state to incoming events
        final isRead = _readIds.contains(event.id);
        final markedEvent = isRead ? event.copyWith(isRead: true) : event;

        final updated = [markedEvent, ...state.history];
        final capped =
            updated.length > 100 ? updated.sublist(0, 100) : updated;
        final unread = capped.where((e) => !e.isRead).length;
        state = state.copyWith(
          history: capped,
          unreadCount: unread,
        );
      }
    });

    _webSocket!.connect();
  }

  void markAllRead() {
    final updated =
        state.history.map((e) => e.copyWith(isRead: true)).toList();
    for (final e in updated) {
      _readIds.add(e.id);
    }
    state = NotificationState(
      history: updated,
      unreadCount: 0,
      wsConnected: state.wsConnected,
    );
    _saveReadIds();
  }

  void markRead(int index) {
    if (index < 0 || index >= state.history.length) return;
    final event = state.history[index];
    if (event.isRead) return;
    final updated = List<NotificationEvent>.from(state.history);
    updated[index] = event.copyWith(isRead: true);
    _readIds.add(event.id);
    state = NotificationState(
      history: updated,
      unreadCount: (state.unreadCount - 1).clamp(0, state.unreadCount),
      wsConnected: state.wsConnected,
    );
    _saveReadIds();
  }

  void _cleanup() {
    _eventsSub?.cancel();
    _connectionSub?.cancel();
    _webSocket?.dispose();
    _webSocket = null;
    _eventsSub = null;
    _connectionSub = null;
  }

  @override
  void dispose() {
    _cleanup();
    super.dispose();
  }
}

final notificationsProvider =
    StateNotifierProvider<NotificationsNotifier, NotificationState>((ref) {
  final notifier = NotificationsNotifier();

  ref.listen<AuthState>(authProvider, (previous, next) {
    if (next.status == AuthStatus.authenticated &&
        next.serverUrl != null) {
      notifier.connect(next.serverUrl!);
    }
  }, fireImmediately: true);

  return notifier;
});
```

- [ ] **Step 2: Verify no compile errors**

Run: `cd clients/flutter && flutter analyze lib/providers/notifications_provider.dart`
Expected: No issues found

- [ ] **Step 3: Commit**

```bash
git add clients/flutter/lib/providers/notifications_provider.dart
git commit -m "feat(notifications): persist read state via shared_preferences"
```

---

### Task 4: Add notification bell to mobile layout

**Files:**
- Modify: `clients/flutter/lib/widgets/shell/navigation_shell.dart`

- [ ] **Step 1: Add mobile app bar with notification bell**

In `navigation_shell.dart`, wrap the mobile Scaffold's body with an AppBar that includes a notification bell with unread badge. Add the notifications provider import.

Add import at the top of the file:

```dart
import '../../providers/notifications_provider.dart';
```

Replace the mobile `Scaffold` block (the `if (width < 600)` return) with:

```dart
return Scaffold(
  appBar: AppBar(
    backgroundColor: NvrColors.bgSecondary,
    elevation: 0,
    toolbarHeight: 44,
    titleSpacing: 12,
    title: Transform.rotate(
      angle: 0.785398,
      child: Container(
        width: 14, height: 14,
        decoration: BoxDecoration(
          border: Border.all(color: NvrColors.accent, width: 2),
        ),
      ),
    ),
    centerTitle: false,
    actions: [
      Padding(
        padding: const EdgeInsets.only(right: 8),
        child: _MobileNotificationBell(
          unreadCount: ref.watch(
            notificationsProvider.select((s) => s.unreadCount),
          ),
          onTap: () => _onAlertsTap(context, ref),
        ),
      ),
    ],
  ),
  body: Stack(
    children: [
      child,
      const TourActivePill(),
    ],
  ),
  bottomNavigationBar: MobileBottomNav(
    selectedIndex: mobileIndex,
    onDestinationSelected: (i) {
      if (i == 4) {
        onDestinationSelected(6);
      } else {
        onDestinationSelected(i);
      }
    },
  ),
);
```

Then add the `_MobileNotificationBell` widget at the bottom of the file:

```dart
class _MobileNotificationBell extends StatelessWidget {
  const _MobileNotificationBell({
    required this.unreadCount,
    required this.onTap,
  });

  final int unreadCount;
  final VoidCallback onTap;

  @override
  Widget build(BuildContext context) {
    return GestureDetector(
      onTap: onTap,
      child: SizedBox(
        width: 40,
        height: 40,
        child: Stack(
          alignment: Alignment.center,
          children: [
            Icon(
              Icons.notifications_outlined,
              size: 22,
              color: NvrColors.textSecondary,
            ),
            if (unreadCount > 0)
              Positioned(
                right: 4,
                top: 6,
                child: Container(
                  padding: const EdgeInsets.all(3),
                  decoration: BoxDecoration(
                    color: NvrColors.danger,
                    shape: BoxShape.circle,
                    border: Border.all(
                      color: NvrColors.bgSecondary,
                      width: 1.5,
                    ),
                  ),
                  child: Text(
                    unreadCount > 9 ? '9+' : '$unreadCount',
                    style: const TextStyle(
                      fontSize: 7,
                      fontWeight: FontWeight.bold,
                      color: Colors.white,
                    ),
                  ),
                ),
              ),
          ],
        ),
      ),
    );
  }
}
```

- [ ] **Step 2: Verify no compile errors**

Run: `cd clients/flutter && flutter analyze lib/widgets/shell/navigation_shell.dart`
Expected: No issues found

- [ ] **Step 3: Commit**

```bash
git add clients/flutter/lib/widgets/shell/navigation_shell.dart
git commit -m "feat(notifications): add notification bell to mobile app bar"
```

---

### Task 5: Full project analysis and final commit

- [ ] **Step 1: Run full flutter analyze**

Run: `cd clients/flutter && flutter analyze`
Expected: No issues found (or only pre-existing warnings)

- [ ] **Step 2: Final commit with all changes**

If any fixups were needed from the analyze step, commit them:

```bash
git add -A clients/flutter/lib/
git commit -m "fix(notifications): address analyzer warnings"
```

- [ ] **Step 3: Push and create PR**

```bash
git push -u origin feat/kai-66-notification-center
gh pr create --title "feat: notification center (KAI-66)" --body "..."
```
