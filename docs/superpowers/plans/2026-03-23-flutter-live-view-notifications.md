# Flutter NVR Client — Plan 2: Live View + Notifications

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Add live video streaming (WebRTC WHEP), camera grid, PTZ controls, WebSocket notifications with AI detection overlay, and notification bell to the Flutter NVR client.

**Architecture:** WHEP service handles WebRTC SDP offer/answer with MediaMTX. WebSocket service maintains persistent connection for notifications and detection_frame events. Camera grid uses adaptive layout. Analytics overlay uses CustomPainter over video. Backend broadcasts detection_frame events via existing WebSocket.

**Tech Stack:** flutter_webrtc, web_socket_channel, Riverpod, CustomPainter

**Spec:** `docs/superpowers/specs/2026-03-23-flutter-client-design.md` (Sections 4, 5, 10)

**Prerequisite:** Plan 1 (Foundation + Auth) must be complete. The app already has: project scaffold, theme, navigation shell, auth, API client, Camera model.

---

## File Structure

| File                                                           | Task | Purpose                            |
| -------------------------------------------------------------- | ---- | ---------------------------------- |
| `clients/flutter/pubspec.yaml`                                 | 1    | Add flutter_webrtc dependency      |
| `clients/flutter/lib/services/whep_service.dart`               | 1    | WebRTC WHEP handshake              |
| `clients/flutter/lib/services/websocket_service.dart`          | 2    | Persistent WebSocket connection    |
| `clients/flutter/lib/models/notification_event.dart`           | 2    | Notification event model           |
| `clients/flutter/lib/models/detection_frame.dart`              | 2    | Detection frame model              |
| `clients/flutter/lib/providers/notifications_provider.dart`    | 3    | Notification state + history       |
| `clients/flutter/lib/providers/detection_stream_provider.dart` | 3    | Detection frame stream per camera  |
| `clients/flutter/lib/screens/live_view/live_view_screen.dart`  | 4    | Adaptive camera grid               |
| `clients/flutter/lib/screens/live_view/camera_tile.dart`       | 4    | Single camera WebRTC tile          |
| `clients/flutter/lib/screens/live_view/fullscreen_view.dart`   | 5    | Fullscreen video + controls        |
| `clients/flutter/lib/screens/live_view/ptz_controls.dart`      | 5    | PTZ d-pad overlay                  |
| `clients/flutter/lib/screens/live_view/analytics_overlay.dart` | 6    | Bounding box CustomPainter         |
| `clients/flutter/lib/widgets/notification_bell.dart`           | 7    | App bar bell + badge + dropdown    |
| `clients/flutter/lib/widgets/notification_toast.dart`          | 7    | Snackbar helper for events         |
| `internal/nvr/ai/pipeline.go`                                  | 8    | Backend: broadcast detection_frame |
| `internal/nvr/api/events.go`                                   | 8    | Backend: Detections field on Event |

---

### Task 1: WHEP Service + flutter_webrtc Dependency

**Files:**

- Modify: `clients/flutter/pubspec.yaml`
- Create: `clients/flutter/lib/services/whep_service.dart`

- [ ] **Step 1: Add flutter_webrtc to pubspec.yaml**

Add under `dependencies:`:

```yaml
flutter_webrtc: ^0.12.0
```

Run:

```bash
cd clients/flutter && flutter pub get
```

- [ ] **Step 2: Create WHEP service**

```dart
// clients/flutter/lib/services/whep_service.dart
import 'dart:async';
import 'package:flutter_webrtc/flutter_webrtc.dart';
import 'package:dio/dio.dart';

enum WhepConnectionState { connecting, connected, failed, disposed }

/// Manages a single WebRTC WHEP connection to a camera stream.
class WhepConnection {
  final String serverUrl;
  final String mediamtxPath;
  final Dio _dio;

  RTCPeerConnection? _pc;
  RTCVideoRenderer? _renderer;
  WhepConnectionState _state = WhepConnectionState.connecting;
  int _retryCount = 0;
  Timer? _retryTimer;
  final _stateController = StreamController<WhepConnectionState>.broadcast();

  static const _maxRetries = 5;

  WhepConnection({
    required this.serverUrl,
    required this.mediamtxPath,
    Dio? dio,
  }) : _dio = dio ?? Dio();

  WhepConnectionState get state => _state;
  Stream<WhepConnectionState> get stateStream => _stateController.stream;
  RTCVideoRenderer? get renderer => _renderer;

  /// Connect to the WHEP endpoint and start receiving video.
  Future<void> connect() async {
    _setState(WhepConnectionState.connecting);

    try {
      _renderer = RTCVideoRenderer();
      await _renderer!.initialize();

      _pc = await createPeerConnection({
        'iceServers': [],
        'sdpSemantics': 'unified-plan',
      });

      // Add recvonly transceivers for video + audio
      await _pc!.addTransceiver(
        kind: RTCRtpMediaType.RTCRtpMediaTypeVideo,
        init: RTCRtpTransceiverInit(direction: TransceiverDirection.RecvOnly),
      );
      await _pc!.addTransceiver(
        kind: RTCRtpMediaType.RTCRtpMediaTypeAudio,
        init: RTCRtpTransceiverInit(direction: TransceiverDirection.RecvOnly),
      );

      // Create offer
      final offer = await _pc!.createOffer();
      await _pc!.setLocalDescription(offer);

      // Wait for ICE gathering to complete
      final sdp = await _waitForIceGathering();

      // POST offer to WHEP endpoint
      final whepUrl = '$serverUrl:8889/$mediamtxPath/whep';
      final response = await _dio.post(
        whepUrl,
        data: sdp,
        options: Options(
          headers: {'Content-Type': 'application/sdp'},
          responseType: ResponseType.plain,
        ),
      );

      // Set answer
      await _pc!.setRemoteDescription(
        RTCSessionDescription(response.data as String, 'answer'),
      );

      // Handle tracks
      _pc!.onTrack = (event) {
        if (event.streams.isNotEmpty) {
          _renderer!.srcObject = event.streams[0];
        }
      };

      // Monitor connection state
      _pc!.onConnectionState = (state) {
        if (state == RTCPeerConnectionState.RTCPeerConnectionStateConnected) {
          _retryCount = 0;
          _setState(WhepConnectionState.connected);
        } else if (state == RTCPeerConnectionState.RTCPeerConnectionStateFailed ||
            state == RTCPeerConnectionState.RTCPeerConnectionStateDisconnected) {
          _setState(WhepConnectionState.failed);
          _scheduleRetry();
        }
      };
    } catch (e) {
      _setState(WhepConnectionState.failed);
      _scheduleRetry();
    }
  }

  Future<String> _waitForIceGathering() async {
    final completer = Completer<String>();
    final desc = await _pc!.getLocalDescription();
    if (_pc!.iceGatheringState == RTCIceGatheringState.RTCIceGatheringStateComplete) {
      return desc?.sdp ?? '';
    }
    _pc!.onIceGatheringState = (state) async {
      if (state == RTCIceGatheringState.RTCIceGatheringStateComplete && !completer.isCompleted) {
        final d = await _pc!.getLocalDescription();
        completer.complete(d?.sdp ?? '');
      }
    };
    // Timeout after 5 seconds
    return completer.future.timeout(
      const Duration(seconds: 5),
      onTimeout: () async {
        final d = await _pc!.getLocalDescription();
        return d?.sdp ?? '';
      },
    );
  }

  void _scheduleRetry() {
    if (_retryCount >= _maxRetries || _state == WhepConnectionState.disposed) return;
    _retryCount++;
    final delay = Duration(seconds: (3 * (1 << (_retryCount - 1))).clamp(3, 30));
    _retryTimer?.cancel();
    _retryTimer = Timer(delay, () {
      if (_state != WhepConnectionState.disposed) {
        _cleanup();
        connect();
      }
    });
  }

  /// Manually retry the connection.
  void retry() {
    _retryCount = 0;
    _cleanup();
    connect();
  }

  void _setState(WhepConnectionState newState) {
    _state = newState;
    _stateController.add(newState);
  }

  void _cleanup() {
    _pc?.close();
    _pc = null;
  }

  /// Dispose all resources.
  Future<void> dispose() async {
    _setState(WhepConnectionState.disposed);
    _retryTimer?.cancel();
    _pc?.close();
    _pc = null;
    _renderer?.dispose();
    _renderer = null;
    await _stateController.close();
  }
}
```

- [ ] **Step 3: Verify**

```bash
cd clients/flutter && flutter analyze
```

- [ ] **Step 4: Commit**

```bash
git add clients/flutter/pubspec.yaml clients/flutter/pubspec.lock clients/flutter/lib/services/whep_service.dart
git commit -m "feat(flutter): add WHEP WebRTC streaming service"
```

---

### Task 2: WebSocket Service + Event Models

**Files:**

- Create: `clients/flutter/lib/services/websocket_service.dart`
- Create: `clients/flutter/lib/models/notification_event.dart`
- Create: `clients/flutter/lib/models/detection_frame.dart`

- [ ] **Step 1: Create notification event model**

```dart
// clients/flutter/lib/models/notification_event.dart

class NotificationEvent {
  final String type;     // "ai_detection", "motion", "camera_offline", etc.
  final String camera;
  final String message;
  final DateTime time;
  final String? zone;
  final String? className;
  final String? action;  // "entered", "loitering", "left"
  final int? trackId;
  final double? confidence;

  NotificationEvent({
    required this.type,
    required this.camera,
    required this.message,
    required this.time,
    this.zone,
    this.className,
    this.action,
    this.trackId,
    this.confidence,
  });

  factory NotificationEvent.fromJson(Map<String, dynamic> json) {
    return NotificationEvent(
      type: json['type'] as String? ?? '',
      camera: json['camera'] as String? ?? '',
      message: json['message'] as String? ?? '',
      time: DateTime.tryParse(json['time'] as String? ?? '') ?? DateTime.now(),
      zone: json['zone'] as String?,
      className: json['class'] as String?,
      action: json['action'] as String?,
      trackId: json['track_id'] as int?,
      confidence: (json['confidence'] as num?)?.toDouble(),
    );
  }

  bool get isDetectionFrame => type == 'detection_frame';
  bool get isAlert => type == 'ai_detection' || type == 'motion' ||
      type == 'camera_offline' || type == 'camera_online';
}
```

- [ ] **Step 2: Create detection frame model**

```dart
// clients/flutter/lib/models/detection_frame.dart

class DetectionBox {
  final String className;
  final double confidence;
  final int trackId;
  final double x, y, w, h; // normalized 0-1

  DetectionBox({
    required this.className,
    required this.confidence,
    required this.trackId,
    required this.x,
    required this.y,
    required this.w,
    required this.h,
  });

  factory DetectionBox.fromJson(Map<String, dynamic> json) {
    return DetectionBox(
      className: json['class'] as String? ?? '',
      confidence: (json['confidence'] as num?)?.toDouble() ?? 0,
      trackId: json['track_id'] as int? ?? 0,
      x: (json['x'] as num?)?.toDouble() ?? 0,
      y: (json['y'] as num?)?.toDouble() ?? 0,
      w: (json['w'] as num?)?.toDouble() ?? 0,
      h: (json['h'] as num?)?.toDouble() ?? 0,
    );
  }
}

class DetectionFrame {
  final String camera;
  final DateTime time;
  final List<DetectionBox> detections;

  DetectionFrame({
    required this.camera,
    required this.time,
    required this.detections,
  });

  factory DetectionFrame.fromJson(Map<String, dynamic> json) {
    final dets = (json['detections'] as List?)
            ?.map((d) => DetectionBox.fromJson(d as Map<String, dynamic>))
            .toList() ??
        [];
    return DetectionFrame(
      camera: json['camera'] as String? ?? '',
      time: DateTime.tryParse(json['time'] as String? ?? '') ?? DateTime.now(),
      detections: dets,
    );
  }
}
```

- [ ] **Step 3: Create WebSocket service**

```dart
// clients/flutter/lib/services/websocket_service.dart
import 'dart:async';
import 'dart:convert';
import 'package:web_socket_channel/web_socket_channel.dart';

import '../models/notification_event.dart';
import '../models/detection_frame.dart';

/// Persistent WebSocket connection to the NVR notification server.
class WebSocketService {
  final String serverUrl;
  WebSocketChannel? _channel;
  Timer? _reconnectTimer;
  int _reconnectDelay = 3;
  bool _disposed = false;

  final _eventController = StreamController<NotificationEvent>.broadcast();
  final _detectionController = StreamController<DetectionFrame>.broadcast();
  final _connectedController = StreamController<bool>.broadcast();
  bool _connected = false;

  WebSocketService({required this.serverUrl});

  Stream<NotificationEvent> get events => _eventController.stream;
  Stream<DetectionFrame> get detectionFrames => _detectionController.stream;
  Stream<bool> get connectionState => _connectedController.stream;
  bool get connected => _connected;

  /// Connect to the WebSocket server.
  void connect() {
    if (_disposed) return;

    // Derive WS port from API URL (API port + 1)
    final uri = Uri.parse(serverUrl);
    final wsPort = uri.port + 1;
    final scheme = uri.scheme == 'https' ? 'wss' : 'ws';
    final wsUrl = '$scheme://${uri.host}:$wsPort/ws';

    try {
      _channel = WebSocketChannel.connect(Uri.parse(wsUrl));
      _channel!.stream.listen(
        _onMessage,
        onDone: _onDisconnected,
        onError: (_) => _onDisconnected(),
      );
      _connected = true;
      _connectedController.add(true);
      _reconnectDelay = 3; // reset backoff
    } catch (_) {
      _onDisconnected();
    }
  }

  void _onMessage(dynamic data) {
    try {
      final json = jsonDecode(data as String) as Map<String, dynamic>;
      final type = json['type'] as String? ?? '';

      if (type == 'connected') return; // skip handshake message

      if (type == 'detection_frame') {
        _detectionController.add(DetectionFrame.fromJson(json));
      } else {
        _eventController.add(NotificationEvent.fromJson(json));
      }
    } catch (_) {
      // ignore malformed messages
    }
  }

  void _onDisconnected() {
    _connected = false;
    _connectedController.add(false);
    _channel = null;
    _scheduleReconnect();
  }

  void _scheduleReconnect() {
    if (_disposed) return;
    _reconnectTimer?.cancel();
    _reconnectTimer = Timer(Duration(seconds: _reconnectDelay), () {
      if (!_disposed) connect();
    });
    _reconnectDelay = (_reconnectDelay * 2).clamp(3, 30);
  }

  /// Dispose all resources.
  Future<void> dispose() async {
    _disposed = true;
    _reconnectTimer?.cancel();
    await _channel?.sink.close();
    await _eventController.close();
    await _detectionController.close();
    await _connectedController.close();
  }
}
```

- [ ] **Step 4: Verify**

```bash
cd clients/flutter && flutter analyze
```

- [ ] **Step 5: Commit**

```bash
git add clients/flutter/lib/services/websocket_service.dart clients/flutter/lib/models/
git commit -m "feat(flutter): add WebSocket service with notification and detection frame models"
```

---

### Task 3: Riverpod Providers — Notifications + Detection Stream

**Files:**

- Create: `clients/flutter/lib/providers/notifications_provider.dart`
- Create: `clients/flutter/lib/providers/detection_stream_provider.dart`

- [ ] **Step 1: Create notifications provider**

```dart
// clients/flutter/lib/providers/notifications_provider.dart
import 'dart:async';
import 'package:flutter_riverpod/flutter_riverpod.dart';

import '../models/notification_event.dart';
import '../services/websocket_service.dart';
import 'auth_provider.dart';

const _maxHistory = 100;

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
  WebSocketService? _ws;
  StreamSubscription? _eventSub;
  StreamSubscription? _connSub;

  NotificationsNotifier() : super(const NotificationState());

  void connect(String serverUrl) {
    _ws?.dispose();
    _ws = WebSocketService(serverUrl: serverUrl);
    _ws!.connect();

    _eventSub = _ws!.events.listen((event) {
      final newHistory = [event, ...state.history];
      if (newHistory.length > _maxHistory) newHistory.removeRange(_maxHistory, newHistory.length);
      state = state.copyWith(
        history: newHistory,
        unreadCount: state.unreadCount + 1,
      );
    });

    _connSub = _ws!.connectionState.listen((connected) {
      state = state.copyWith(wsConnected: connected);
    });
  }

  /// Access the WebSocket service (for detection frames).
  WebSocketService? get webSocket => _ws;

  void markAllRead() {
    state = state.copyWith(unreadCount: 0);
  }

  @override
  void dispose() {
    _eventSub?.cancel();
    _connSub?.cancel();
    _ws?.dispose();
    super.dispose();
  }
}

final notificationsProvider =
    StateNotifierProvider<NotificationsNotifier, NotificationState>((ref) {
  final notifier = NotificationsNotifier();
  final auth = ref.watch(authProvider);
  if (auth.status == AuthStatus.authenticated && auth.serverUrl != null) {
    notifier.connect(auth.serverUrl!);
  }
  return notifier;
});
```

- [ ] **Step 2: Create detection stream provider**

```dart
// clients/flutter/lib/providers/detection_stream_provider.dart
import 'dart:async';
import 'package:flutter_riverpod/flutter_riverpod.dart';

import '../models/detection_frame.dart';
import 'notifications_provider.dart';

/// Provides a stream of detection frames filtered by camera name.
/// Only emits when the overlay is actively listening for a specific camera.
final detectionStreamProvider =
    StreamProvider.family<DetectionFrame, String>((ref, cameraName) {
  final notifier = ref.watch(notificationsProvider.notifier);
  final ws = notifier.webSocket;
  if (ws == null) return const Stream.empty();

  return ws.detectionFrames.where((frame) => frame.camera == cameraName);
});
```

- [ ] **Step 3: Verify**

```bash
cd clients/flutter && flutter analyze
```

- [ ] **Step 4: Commit**

```bash
git add clients/flutter/lib/providers/
git commit -m "feat(flutter): add notification and detection stream Riverpod providers"
```

---

### Task 4: Live View Screen + Camera Tile

**Files:**

- Create: `clients/flutter/lib/screens/live_view/live_view_screen.dart`
- Create: `clients/flutter/lib/screens/live_view/camera_tile.dart`
- Modify: `clients/flutter/lib/router/app_router.dart`

- [ ] **Step 1: Create camera tile widget**

```dart
// clients/flutter/lib/screens/live_view/camera_tile.dart
import 'dart:async';
import 'package:flutter/material.dart';
import 'package:flutter_webrtc/flutter_webrtc.dart';

import '../../models/camera.dart';
import '../../services/whep_service.dart';
import '../../theme/nvr_colors.dart';

class CameraTile extends StatefulWidget {
  final Camera camera;
  final String serverUrl;
  final VoidCallback? onTap;

  const CameraTile({
    super.key,
    required this.camera,
    required this.serverUrl,
    this.onTap,
  });

  @override
  State<CameraTile> createState() => _CameraTileState();
}

class _CameraTileState extends State<CameraTile> {
  WhepConnection? _connection;
  WhepConnectionState _connState = WhepConnectionState.connecting;
  StreamSubscription? _stateSub;

  @override
  void initState() {
    super.initState();
    if (widget.camera.status == 'online' && widget.camera.mediamtxPath.isNotEmpty) {
      _startConnection();
    }
  }

  void _startConnection() {
    _connection = WhepConnection(
      serverUrl: widget.serverUrl,
      mediamtxPath: widget.camera.mediamtxPath,
    );
    _stateSub = _connection!.stateStream.listen((state) {
      if (mounted) setState(() => _connState = state);
    });
    _connection!.connect();
  }

  @override
  void dispose() {
    _stateSub?.cancel();
    _connection?.dispose();
    super.dispose();
  }

  @override
  Widget build(BuildContext context) {
    final isOffline = widget.camera.status != 'online';

    return GestureDetector(
      onTap: widget.onTap,
      child: Card(
        clipBehavior: Clip.antiAlias,
        child: Stack(
          fit: StackFit.expand,
          children: [
            // Video or black background
            if (_connection?.renderer != null && _connState == WhepConnectionState.connected)
              RTCVideoView(_connection!.renderer!, objectFit: RTCVideoViewObjectFit.RTCVideoViewObjectFitCover)
            else
              Container(color: Colors.black),

            // Camera name label
            Positioned(
              left: 8,
              bottom: 8,
              child: Container(
                padding: const EdgeInsets.symmetric(horizontal: 8, vertical: 4),
                decoration: BoxDecoration(
                  color: Colors.black54,
                  borderRadius: BorderRadius.circular(4),
                ),
                child: Text(
                  widget.camera.name,
                  style: const TextStyle(color: Colors.white, fontSize: 12, fontWeight: FontWeight.w600),
                ),
              ),
            ),

            // Offline badge
            if (isOffline)
              Positioned(
                top: 8,
                right: 8,
                child: Container(
                  padding: const EdgeInsets.symmetric(horizontal: 6, vertical: 2),
                  decoration: BoxDecoration(
                    color: NvrColors.danger.withAlpha(230),
                    borderRadius: BorderRadius.circular(4),
                  ),
                  child: const Text(
                    'OFFLINE',
                    style: TextStyle(color: Colors.white, fontSize: 10, fontWeight: FontWeight.bold),
                  ),
                ),
              ),

            // Connecting spinner
            if (!isOffline && _connState == WhepConnectionState.connecting)
              const Center(child: CircularProgressIndicator(strokeWidth: 2)),

            // Failed overlay with retry
            if (!isOffline && _connState == WhepConnectionState.failed)
              Center(
                child: Column(
                  mainAxisSize: MainAxisSize.min,
                  children: [
                    const Icon(Icons.error_outline, color: NvrColors.danger, size: 32),
                    const SizedBox(height: 8),
                    const Text('Connection failed', style: TextStyle(color: Colors.white70, fontSize: 12)),
                    const SizedBox(height: 8),
                    TextButton(
                      onPressed: () => _connection?.retry(),
                      child: const Text('Retry'),
                    ),
                  ],
                ),
              ),

            // AI enabled indicator
            if (widget.camera.aiEnabled)
              const Positioned(
                top: 8,
                left: 8,
                child: Icon(Icons.psychology, color: NvrColors.accent, size: 16),
              ),
          ],
        ),
      ),
    );
  }
}
```

- [ ] **Step 2: Create live view screen**

```dart
// clients/flutter/lib/screens/live_view/live_view_screen.dart
import 'package:flutter/material.dart';
import 'package:flutter_riverpod/flutter_riverpod.dart';

import '../../providers/cameras_provider.dart';
import '../../providers/auth_provider.dart';
import '../../models/camera.dart';
import '../../theme/nvr_colors.dart';
import 'camera_tile.dart';

class LiveViewScreen extends ConsumerWidget {
  const LiveViewScreen({super.key});

  @override
  Widget build(BuildContext context, WidgetRef ref) {
    final camerasAsync = ref.watch(camerasProvider);
    final auth = ref.watch(authProvider);
    final serverUrl = auth.serverUrl ?? '';

    return Scaffold(
      appBar: AppBar(
        title: const Text('Live View'),
        actions: [
          // Notification bell will be added in Task 7
          IconButton(
            icon: const Icon(Icons.refresh),
            onPressed: () => ref.invalidate(camerasProvider),
          ),
        ],
      ),
      body: camerasAsync.when(
        data: (cameras) {
          if (cameras.isEmpty) {
            return Center(
              child: Column(
                mainAxisSize: MainAxisSize.min,
                children: [
                  Icon(Icons.videocam_off, size: 48, color: NvrColors.textMuted),
                  const SizedBox(height: 16),
                  Text('No cameras configured', style: Theme.of(context).textTheme.titleMedium),
                  const SizedBox(height: 8),
                  Text('Add cameras in the Cameras tab', style: TextStyle(color: NvrColors.textSecondary)),
                ],
              ),
            );
          }
          return _CameraGrid(cameras: cameras, serverUrl: serverUrl);
        },
        loading: () => const Center(child: CircularProgressIndicator()),
        error: (e, _) => Center(child: Text('Error loading cameras: $e')),
      ),
    );
  }
}

class _CameraGrid extends StatelessWidget {
  final List<Camera> cameras;
  final String serverUrl;

  const _CameraGrid({required this.cameras, required this.serverUrl});

  @override
  Widget build(BuildContext context) {
    return LayoutBuilder(
      builder: (context, constraints) {
        final columns = _columnsForWidth(constraints.maxWidth);
        return GridView.builder(
          padding: const EdgeInsets.all(8),
          gridDelegate: SliverGridDelegateWithFixedCrossAxisCount(
            crossAxisCount: columns,
            crossAxisSpacing: 8,
            mainAxisSpacing: 8,
            childAspectRatio: 16 / 9,
          ),
          itemCount: cameras.length,
          itemBuilder: (context, index) {
            final camera = cameras[index];
            return CameraTile(
              camera: camera,
              serverUrl: serverUrl,
              onTap: () {
                // Navigate to fullscreen — Task 5
                Navigator.of(context).push(MaterialPageRoute(
                  builder: (_) => Scaffold(
                    appBar: AppBar(title: Text(camera.name)),
                    body: const Center(child: Text('Fullscreen — coming in Task 5')),
                  ),
                ));
              },
            );
          },
        );
      },
    );
  }

  int _columnsForWidth(double width) {
    if (width < 600) return 1;
    if (width < 900) return 2;
    if (width < 1200) return 3;
    return 4;
  }
}
```

- [ ] **Step 3: Update router to use LiveViewScreen**

In `clients/flutter/lib/router/app_router.dart`, replace the `/live` route:

Change:

```dart
GoRoute(path: '/live', builder: (_, __) => const HomePlaceholder(title: 'Live View')),
```

To:

```dart
GoRoute(path: '/live', builder: (_, __) => const LiveViewScreen()),
```

Add import: `import '../screens/live_view/live_view_screen.dart';`

- [ ] **Step 4: Verify**

```bash
cd clients/flutter && flutter analyze
```

- [ ] **Step 5: Commit**

```bash
git add clients/flutter/lib/screens/live_view/ clients/flutter/lib/router/app_router.dart
git commit -m "feat(flutter): add live view screen with adaptive camera grid"
```

---

### Task 5: Fullscreen View + PTZ Controls

**Files:**

- Create: `clients/flutter/lib/screens/live_view/fullscreen_view.dart`
- Create: `clients/flutter/lib/screens/live_view/ptz_controls.dart`
- Modify: `clients/flutter/lib/screens/live_view/live_view_screen.dart`

- [ ] **Step 1: Create PTZ controls widget**

```dart
// clients/flutter/lib/screens/live_view/ptz_controls.dart
import 'package:flutter/material.dart';
import '../../services/api_client.dart';
import '../../theme/nvr_colors.dart';

class PtzControls extends StatelessWidget {
  final ApiClient api;
  final String cameraId;

  const PtzControls({super.key, required this.api, required this.cameraId});

  Future<void> _sendPtz(String direction) async {
    try {
      await api.post('/cameras/$cameraId/ptz', data: {'action': direction});
    } catch (_) {
      // Best-effort PTZ command
    }
  }

  @override
  Widget build(BuildContext context) {
    return Container(
      decoration: BoxDecoration(
        color: Colors.black45,
        borderRadius: BorderRadius.circular(12),
      ),
      padding: const EdgeInsets.all(8),
      child: Column(
        mainAxisSize: MainAxisSize.min,
        children: [
          _PtzButton(icon: Icons.arrow_drop_up, onPressed: () => _sendPtz('up')),
          Row(
            mainAxisSize: MainAxisSize.min,
            children: [
              _PtzButton(icon: Icons.arrow_left, onPressed: () => _sendPtz('left')),
              const SizedBox(width: 32, height: 32),
              _PtzButton(icon: Icons.arrow_right, onPressed: () => _sendPtz('right')),
            ],
          ),
          _PtzButton(icon: Icons.arrow_drop_down, onPressed: () => _sendPtz('down')),
          const SizedBox(height: 8),
          Row(
            mainAxisSize: MainAxisSize.min,
            children: [
              _PtzButton(icon: Icons.zoom_in, onPressed: () => _sendPtz('zoom_in')),
              const SizedBox(width: 8),
              _PtzButton(icon: Icons.zoom_out, onPressed: () => _sendPtz('zoom_out')),
            ],
          ),
        ],
      ),
    );
  }
}

class _PtzButton extends StatelessWidget {
  final IconData icon;
  final VoidCallback onPressed;

  const _PtzButton({required this.icon, required this.onPressed});

  @override
  Widget build(BuildContext context) {
    return IconButton(
      onPressed: onPressed,
      icon: Icon(icon, color: Colors.white, size: 28),
      style: IconButton.styleFrom(
        backgroundColor: NvrColors.accent.withAlpha(100),
        shape: RoundedRectangleBorder(borderRadius: BorderRadius.circular(8)),
      ),
    );
  }
}
```

- [ ] **Step 2: Create fullscreen view**

```dart
// clients/flutter/lib/screens/live_view/fullscreen_view.dart
import 'dart:async';
import 'package:flutter/material.dart';
import 'package:flutter_riverpod/flutter_riverpod.dart';
import 'package:flutter_webrtc/flutter_webrtc.dart';

import '../../models/camera.dart';
import '../../services/whep_service.dart';
import '../../providers/auth_provider.dart';
import '../../theme/nvr_colors.dart';
import 'ptz_controls.dart';
import 'analytics_overlay.dart';

class FullscreenView extends ConsumerStatefulWidget {
  final Camera camera;

  const FullscreenView({super.key, required this.camera});

  @override
  ConsumerState<FullscreenView> createState() => _FullscreenViewState();
}

class _FullscreenViewState extends ConsumerState<FullscreenView> {
  WhepConnection? _connection;
  WhepConnectionState _connState = WhepConnectionState.connecting;
  StreamSubscription? _stateSub;
  bool _showControls = true;

  @override
  void initState() {
    super.initState();
    final auth = ref.read(authProvider);
    if (auth.serverUrl != null && widget.camera.mediamtxPath.isNotEmpty) {
      _connection = WhepConnection(
        serverUrl: auth.serverUrl!,
        mediamtxPath: widget.camera.mediamtxPath,
      );
      _stateSub = _connection!.stateStream.listen((state) {
        if (mounted) setState(() => _connState = state);
      });
      _connection!.connect();
    }
  }

  @override
  void dispose() {
    _stateSub?.cancel();
    _connection?.dispose();
    super.dispose();
  }

  @override
  Widget build(BuildContext context) {
    final api = ref.watch(apiClientProvider);

    return Scaffold(
      backgroundColor: Colors.black,
      appBar: _showControls
          ? AppBar(
              backgroundColor: Colors.black87,
              title: Text(widget.camera.name),
              actions: [
                IconButton(
                  icon: const Icon(Icons.camera_alt),
                  tooltip: 'Screenshot',
                  onPressed: () {
                    // Screenshot — future enhancement
                  },
                ),
              ],
            )
          : null,
      body: GestureDetector(
        onTap: () => setState(() => _showControls = !_showControls),
        child: Stack(
          fit: StackFit.expand,
          children: [
            // Video
            if (_connection?.renderer != null && _connState == WhepConnectionState.connected)
              RTCVideoView(
                _connection!.renderer!,
                objectFit: RTCVideoViewObjectFit.RTCVideoViewObjectFitContain,
              )
            else
              const Center(child: CircularProgressIndicator()),

            // Analytics overlay (bounding boxes from WebSocket)
            if (widget.camera.aiEnabled)
              AnalyticsOverlay(cameraName: widget.camera.name),

            // PTZ controls
            if (_showControls && widget.camera.ptzCapable && api != null)
              Positioned(
                right: 16,
                bottom: 80,
                child: PtzControls(api: api, cameraId: widget.camera.id),
              ),

            // Connection state
            if (_connState == WhepConnectionState.failed)
              Center(
                child: Column(
                  mainAxisSize: MainAxisSize.min,
                  children: [
                    const Icon(Icons.error_outline, color: NvrColors.danger, size: 48),
                    const SizedBox(height: 12),
                    ElevatedButton(
                      onPressed: () => _connection?.retry(),
                      child: const Text('Retry Connection'),
                    ),
                  ],
                ),
              ),
          ],
        ),
      ),
    );
  }
}
```

- [ ] **Step 3: Update live view to navigate to fullscreen**

In `live_view_screen.dart`, update the `onTap` in `CameraTile` to push `FullscreenView`:

Replace the `onTap` callback:

```dart
onTap: () {
  Navigator.of(context).push(MaterialPageRoute(
    builder: (_) => FullscreenView(camera: camera),
  ));
},
```

Add import: `import 'fullscreen_view.dart';`

- [ ] **Step 4: Verify**

```bash
cd clients/flutter && flutter analyze
```

- [ ] **Step 5: Commit**

```bash
git add clients/flutter/lib/screens/live_view/
git commit -m "feat(flutter): add fullscreen view with PTZ controls"
```

---

### Task 6: Analytics Overlay (CustomPainter)

**Files:**

- Create: `clients/flutter/lib/screens/live_view/analytics_overlay.dart`

- [ ] **Step 1: Create analytics overlay widget**

```dart
// clients/flutter/lib/screens/live_view/analytics_overlay.dart
import 'package:flutter/material.dart';
import 'package:flutter_riverpod/flutter_riverpod.dart';

import '../../models/detection_frame.dart';
import '../../providers/detection_stream_provider.dart';
import '../../theme/nvr_colors.dart';

class AnalyticsOverlay extends ConsumerWidget {
  final String cameraName;

  const AnalyticsOverlay({super.key, required this.cameraName});

  @override
  Widget build(BuildContext context, WidgetRef ref) {
    final detectionAsync = ref.watch(detectionStreamProvider(cameraName));

    return detectionAsync.when(
      data: (frame) => CustomPaint(
        painter: _DetectionPainter(frame.detections),
        size: Size.infinite,
      ),
      loading: () => const SizedBox.shrink(),
      error: (_, __) => const SizedBox.shrink(),
    );
  }
}

class _DetectionPainter extends CustomPainter {
  final List<DetectionBox> detections;

  _DetectionPainter(this.detections);

  @override
  void paint(Canvas canvas, Size size) {
    for (final det in detections) {
      final color = _colorForClass(det.className);
      final rect = Rect.fromLTWH(
        det.x * size.width,
        det.y * size.height,
        det.w * size.width,
        det.h * size.height,
      );

      // Bounding box
      final boxPaint = Paint()
        ..color = color
        ..style = PaintingStyle.stroke
        ..strokeWidth = 2;
      canvas.drawRect(rect, boxPaint);

      // Label background
      final label = '${_capitalize(det.className)} #${det.trackId} ${(det.confidence * 100).toInt()}%';
      final textSpan = TextSpan(
        text: label,
        style: const TextStyle(color: Colors.white, fontSize: 11, fontWeight: FontWeight.w600),
      );
      final textPainter = TextPainter(text: textSpan, textDirection: TextDirection.ltr);
      textPainter.layout();

      final labelRect = Rect.fromLTWH(
        rect.left,
        rect.top - textPainter.height - 4,
        textPainter.width + 8,
        textPainter.height + 4,
      );
      canvas.drawRect(labelRect, Paint()..color = color.withAlpha(200));
      textPainter.paint(canvas, Offset(labelRect.left + 4, labelRect.top + 2));
    }
  }

  Color _colorForClass(String className) {
    switch (className) {
      case 'person':
        return NvrColors.accent;   // blue
      case 'car':
      case 'truck':
      case 'bus':
      case 'motorcycle':
      case 'bicycle':
        return NvrColors.success;  // green
      case 'cat':
      case 'dog':
      case 'horse':
        return NvrColors.warning;  // amber
      default:
        return NvrColors.danger;   // red
    }
  }

  String _capitalize(String s) => s.isEmpty ? s : '${s[0].toUpperCase()}${s.substring(1)}';

  @override
  bool shouldRepaint(_DetectionPainter old) => detections != old.detections;
}
```

- [ ] **Step 2: Verify**

```bash
cd clients/flutter && flutter analyze
```

- [ ] **Step 3: Commit**

```bash
git add clients/flutter/lib/screens/live_view/analytics_overlay.dart
git commit -m "feat(flutter): add AI analytics overlay with bounding box CustomPainter"
```

---

### Task 7: Notification Bell + Toast

**Files:**

- Create: `clients/flutter/lib/widgets/notification_bell.dart`
- Create: `clients/flutter/lib/widgets/notification_toast.dart`
- Modify: `clients/flutter/lib/screens/live_view/live_view_screen.dart`

- [ ] **Step 1: Create notification toast helper**

```dart
// clients/flutter/lib/widgets/notification_toast.dart
import 'package:flutter/material.dart';
import '../models/notification_event.dart';
import '../theme/nvr_colors.dart';

void showNotificationSnackbar(BuildContext context, NotificationEvent event) {
  final color = _colorForEvent(event);
  final icon = _iconForEvent(event);

  ScaffoldMessenger.of(context).showSnackBar(
    SnackBar(
      behavior: SnackBarBehavior.floating,
      backgroundColor: NvrColors.bgSecondary,
      shape: RoundedRectangleBorder(borderRadius: BorderRadius.circular(8)),
      duration: event.type == 'camera_offline'
          ? const Duration(seconds: 30) // persistent for offline
          : const Duration(seconds: 5),
      content: Row(
        children: [
          Icon(icon, color: color, size: 18),
          const SizedBox(width: 8),
          Expanded(
            child: Column(
              mainAxisSize: MainAxisSize.min,
              crossAxisAlignment: CrossAxisAlignment.start,
              children: [
                Text(
                  _titleForEvent(event),
                  style: TextStyle(color: color, fontWeight: FontWeight.w600, fontSize: 13),
                ),
                Text(
                  event.message,
                  style: const TextStyle(color: NvrColors.textSecondary, fontSize: 12),
                  maxLines: 1,
                  overflow: TextOverflow.ellipsis,
                ),
              ],
            ),
          ),
        ],
      ),
    ),
  );
}

String _titleForEvent(NotificationEvent event) {
  if (event.action != null && event.className != null) {
    final cls = event.className![0].toUpperCase() + event.className!.substring(1);
    switch (event.action) {
      case 'entered': return '$cls Entered';
      case 'loitering': return '$cls Loitering';
      case 'left': return '$cls Left';
    }
  }
  switch (event.type) {
    case 'motion': return 'Motion Detected';
    case 'camera_offline': return 'Camera Offline';
    case 'camera_online': return 'Camera Online';
    default: return 'Notification';
  }
}

Color _colorForEvent(NotificationEvent event) {
  if (event.action == 'loitering') return NvrColors.danger;
  if (event.action == 'left') return NvrColors.accent;
  switch (event.type) {
    case 'camera_offline': return NvrColors.danger;
    case 'camera_online': return NvrColors.success;
    default: return NvrColors.warning;
  }
}

IconData _iconForEvent(NotificationEvent event) {
  switch (event.type) {
    case 'ai_detection': return Icons.psychology;
    case 'motion': return Icons.directions_run;
    case 'camera_offline': return Icons.videocam_off;
    case 'camera_online': return Icons.videocam;
    case 'recording_started': return Icons.fiber_manual_record;
    default: return Icons.notifications;
  }
}
```

- [ ] **Step 2: Create notification bell widget**

```dart
// clients/flutter/lib/widgets/notification_bell.dart
import 'package:flutter/material.dart';
import 'package:flutter_riverpod/flutter_riverpod.dart';
import 'package:intl/intl.dart' show DateFormat;

import '../providers/notifications_provider.dart';
import '../models/notification_event.dart';
import '../theme/nvr_colors.dart';

class NotificationBell extends ConsumerWidget {
  const NotificationBell({super.key});

  @override
  Widget build(BuildContext context, WidgetRef ref) {
    final notifState = ref.watch(notificationsProvider);

    return Stack(
      clipBehavior: Clip.none,
      children: [
        IconButton(
          icon: Icon(
            notifState.wsConnected ? Icons.notifications : Icons.notifications_off,
            color: notifState.wsConnected ? null : NvrColors.textMuted,
          ),
          onPressed: () => _showNotificationSheet(context, ref),
        ),
        if (notifState.unreadCount > 0)
          Positioned(
            right: 4,
            top: 4,
            child: Container(
              padding: const EdgeInsets.all(4),
              decoration: const BoxDecoration(color: NvrColors.danger, shape: BoxShape.circle),
              constraints: const BoxConstraints(minWidth: 16, minHeight: 16),
              child: Text(
                notifState.unreadCount > 99 ? '99+' : '${notifState.unreadCount}',
                style: const TextStyle(color: Colors.white, fontSize: 9, fontWeight: FontWeight.bold),
                textAlign: TextAlign.center,
              ),
            ),
          ),
      ],
    );
  }

  void _showNotificationSheet(BuildContext context, WidgetRef ref) {
    ref.read(notificationsProvider.notifier).markAllRead();

    showModalBottomSheet(
      context: context,
      backgroundColor: NvrColors.bgSecondary,
      shape: const RoundedRectangleBorder(
        borderRadius: BorderRadius.vertical(top: Radius.circular(16)),
      ),
      builder: (context) {
        return Consumer(
          builder: (context, ref, _) {
            final history = ref.watch(notificationsProvider).history;
            if (history.isEmpty) {
              return const SizedBox(
                height: 200,
                child: Center(child: Text('No notifications', style: TextStyle(color: NvrColors.textMuted))),
              );
            }
            return ListView.builder(
              shrinkWrap: true,
              itemCount: history.length.clamp(0, 50),
              itemBuilder: (_, i) => _NotificationItem(event: history[i]),
            );
          },
        );
      },
    );
  }
}

class _NotificationItem extends StatelessWidget {
  final NotificationEvent event;

  const _NotificationItem({required this.event});

  @override
  Widget build(BuildContext context) {
    return ListTile(
      dense: true,
      leading: Icon(
        _iconForType(event.type),
        color: _colorForType(event.type),
        size: 20,
      ),
      title: Text(event.message, style: const TextStyle(fontSize: 13, color: NvrColors.textPrimary)),
      subtitle: Text(
        '${event.camera} • ${_formatTime(event.time)}',
        style: const TextStyle(fontSize: 11, color: NvrColors.textMuted),
      ),
    );
  }

  IconData _iconForType(String type) {
    switch (type) {
      case 'ai_detection': return Icons.psychology;
      case 'motion': return Icons.directions_run;
      case 'camera_offline': return Icons.videocam_off;
      case 'camera_online': return Icons.videocam;
      default: return Icons.notifications;
    }
  }

  Color _colorForType(String type) {
    switch (type) {
      case 'camera_offline': return NvrColors.danger;
      case 'camera_online': return NvrColors.success;
      default: return NvrColors.warning;
    }
  }

  String _formatTime(DateTime time) {
    final now = DateTime.now();
    final diff = now.difference(time);
    if (diff.inMinutes < 1) return 'Just now';
    if (diff.inMinutes < 60) return '${diff.inMinutes}m ago';
    if (diff.inHours < 24) return '${diff.inHours}h ago';
    return '${time.month}/${time.day} ${time.hour}:${time.minute.toString().padLeft(2, '0')}';
  }
}
```

Note: The `intl` import for `DateFormat` may not be needed since we use manual formatting. Remove the import if it causes issues.

- [ ] **Step 3: Add notification bell to live view app bar**

In `live_view_screen.dart`, update the `AppBar` actions to include the bell:

```dart
actions: [
  const NotificationBell(),
  IconButton(
    icon: const Icon(Icons.refresh),
    onPressed: () => ref.invalidate(camerasProvider),
  ),
],
```

Add import: `import '../../widgets/notification_bell.dart';`

- [ ] **Step 4: Verify**

```bash
cd clients/flutter && flutter analyze
```

Remove `intl` import from notification_bell.dart if it causes an error (package not in pubspec).

- [ ] **Step 5: Commit**

```bash
git add clients/flutter/lib/widgets/ clients/flutter/lib/screens/live_view/live_view_screen.dart
git commit -m "feat(flutter): add notification bell with history and event snackbar toasts"
```

---

### Task 8: Backend — Broadcast detection_frame Events

**Files:**

- Modify: `internal/nvr/api/events.go`
- Modify: `internal/nvr/ai/pipeline.go`

- [ ] **Step 1: Add Detections field to Event struct**

In `internal/nvr/api/events.go`, add a `Detections` field to the `Event` struct:

```go
type DetectionData struct {
    Class      string  `json:"class"`
    Confidence float32 `json:"confidence"`
    TrackID    int     `json:"track_id"`
    X          float32 `json:"x"`
    Y          float32 `json:"y"`
    W          float32 `json:"w"`
    H          float32 `json:"h"`
}
```

Add to Event struct:

```go
Detections []DetectionData `json:"detections,omitempty"`
```

Add a new publish method:

```go
func (b *EventBroadcaster) PublishDetectionFrame(camera string, detections []DetectionData) {
    b.Publish(Event{
        Type:       "detection_frame",
        Camera:     camera,
        Detections: detections,
        Time:       time.Now().UTC().Format(time.RFC3339),
    })
}
```

- [ ] **Step 2: Broadcast detection_frame from pipeline**

In `internal/nvr/ai/pipeline.go`, in `ProcessFrame()`, after the tracking step and before storing detections, broadcast the frame:

```go
// Broadcast detection_frame for WebSocket clients (Flutter overlay)
if p.eventPub != nil && len(tracked) > 0 {
    dets := make([]api.DetectionData, 0, len(tracked))
    for _, td := range tracked {
        dets = append(dets, api.DetectionData{
            Class:      td.ClassName,
            Confidence: td.Confidence,
            TrackID:    td.TrackID,
            X:          td.X, Y: td.Y, W: td.W, H: td.H,
        })
    }
    p.eventPub.PublishDetectionFrame(p.cameraName, dets)
}
```

This requires updating the `EventPublisher` interface to include `PublishDetectionFrame`. Add to the interface:

```go
PublishDetectionFrame(camera string, detections []DetectionData)
```

Note: The `DetectionData` type is in the `api` package, so the pipeline needs to import it, or define a matching interface. The simplest approach: define the method signature so the pipeline passes raw data and the broadcaster formats it. Alternatively, define `DetectionData` in the `ai` package and have the `api` package reuse it.

**Practical approach:** Add `PublishDetectionFrame` to the `EventPublisher` interface in pipeline.go using a simple struct defined in the ai package, and have `EventBroadcaster` implement it.

- [ ] **Step 3: Verify backend builds**

```bash
go build ./internal/nvr/...
```

- [ ] **Step 4: Commit**

```bash
git add internal/nvr/api/events.go internal/nvr/ai/pipeline.go
git commit -m "feat(api): broadcast detection_frame WebSocket events for Flutter overlay"
```

---

### Task 9: Wire It All Together + Verify

**Files:** Various minor wiring

- [ ] **Step 1: Ensure WebSocket connects on authentication**

Verify that `notificationsProvider` in `notifications_provider.dart` properly watches `authProvider` and connects the WebSocket when authenticated. This was done in Task 3, but verify it works end-to-end.

- [ ] **Step 2: Add notification bell to the adaptive layout app bar**

The notification bell should appear on ALL screens, not just live view. Update `adaptive_layout.dart` to accept an `appBar` parameter, or move the bell to the `NvrApp` level. Simplest: add it to each screen's AppBar where relevant — live view already has it from Task 7.

For a unified approach, update `AdaptiveLayout` to optionally show the bell:

In `adaptive_layout.dart`, wrap the child in a `Scaffold` that has an `AppBar` with the bell:

```dart
// Add to the rail layout:
Expanded(
  child: Scaffold(
    appBar: AppBar(
      title: const Text('MediaMTX NVR'),
      actions: const [NotificationBell()],
    ),
    body: child,
  ),
),
```

Or alternatively, each screen manages its own AppBar (current approach from Task 4). The bell is on live view; other screens can add it when they're built in Plans 3-4.

- [ ] **Step 3: Final verification**

```bash
cd clients/flutter && flutter analyze
```

```bash
go build ./internal/nvr/...
go test ./internal/nvr/ai/ -v
```

All must pass with no errors.

- [ ] **Step 4: Commit any wiring fixes**

```bash
git add -A
git commit -m "feat(flutter): wire notifications and finalize live view integration"
```
