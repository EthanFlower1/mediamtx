import 'dart:async';
import 'dart:convert';
import 'package:web_socket_channel/web_socket_channel.dart';
import '../models/notification_event.dart';
import '../models/detection_frame.dart';

class WebSocketService {
  final String serverUrl;

  WebSocketChannel? _channel;
  StreamSubscription? _subscription;
  Timer? _reconnectTimer;
  bool _disposed = false;
  int _reconnectDelay = 3;

  final _eventsController =
      StreamController<NotificationEvent>.broadcast();
  final _detectionFramesController =
      StreamController<DetectionFrame>.broadcast();
  final _connectionStateController = StreamController<bool>.broadcast();

  Stream<NotificationEvent> get events => _eventsController.stream;
  Stream<DetectionFrame> get detectionFrames =>
      _detectionFramesController.stream;
  Stream<bool> get connectionState => _connectionStateController.stream;

  WebSocketService({required this.serverUrl});

  Uri _buildWsUri() {
    final parsed = Uri.parse(serverUrl);
    final isSecure =
        parsed.scheme == 'https' || parsed.scheme == 'wss';
    final wsScheme = isSecure ? 'wss' : 'ws';
    final apiPort = parsed.hasPort ? parsed.port : (isSecure ? 443 : 80);
    final wsPort = apiPort + 1;
    return Uri(
      scheme: wsScheme,
      host: parsed.host,
      port: wsPort,
      path: '/ws',
    );
  }

  void connect() {
    if (_disposed) return;
    _reconnectTimer?.cancel();
    _reconnectTimer = null;

    final wsUri = _buildWsUri();

    try {
      _channel = WebSocketChannel.connect(wsUri);

      // Wait for the connection handshake to complete before declaring success.
      _channel!.ready.then((_) {
        if (_disposed) return;
        _connectionStateController.add(true);
        _reconnectDelay = 3;
      }).catchError((_) {
        if (_disposed) return;
        _connectionStateController.add(false);
        _scheduleReconnect();
      });

      _subscription = _channel!.stream.listen(
        _onMessage,
        onError: (_) {
          _connectionStateController.add(false);
          _scheduleReconnect();
        },
        onDone: () {
          _connectionStateController.add(false);
          _scheduleReconnect();
        },
        cancelOnError: true,
      );
    } catch (_) {
      _connectionStateController.add(false);
      _scheduleReconnect();
    }
  }

  void _onMessage(dynamic raw) {
    if (_disposed) return;
    try {
      final json = jsonDecode(raw as String) as Map<String, dynamic>;
      final type = json['type'] as String?;

      if (type == 'connected') {
        return;
      } else if (type == 'detection_frame') {
        _detectionFramesController.add(DetectionFrame.fromJson(json));
      } else {
        _eventsController.add(NotificationEvent.fromJson(json));
      }
    } catch (_) {
      // Ignore malformed messages
    }
  }

  void _scheduleReconnect() {
    if (_disposed) return;
    _subscription?.cancel();
    _subscription = null;
    _channel = null;

    _reconnectTimer?.cancel();
    _reconnectTimer = Timer(Duration(seconds: _reconnectDelay), () {
      if (!_disposed) connect();
    });

    _reconnectDelay = (_reconnectDelay * 2).clamp(3, 30);
  }

  void dispose() {
    _disposed = true;
    _reconnectTimer?.cancel();
    _subscription?.cancel();
    _channel?.sink.close();
    _eventsController.close();
    _detectionFramesController.close();
    _connectionStateController.close();
  }
}
