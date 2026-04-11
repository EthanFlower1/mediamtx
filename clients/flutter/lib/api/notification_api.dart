import '../services/api_client.dart';

/// Data model for a single notification from the API.
class NotificationItem {
  final String id;
  final String type;
  final String severity;
  final String camera;
  final String message;
  final DateTime createdAt;
  final String? readAt;
  final bool archived;

  const NotificationItem({
    required this.id,
    required this.type,
    required this.severity,
    required this.camera,
    required this.message,
    required this.createdAt,
    this.readAt,
    this.archived = false,
  });

  bool get isRead => readAt != null && readAt!.isNotEmpty;

  factory NotificationItem.fromJson(Map<String, dynamic> json) {
    return NotificationItem(
      id: json['id'] as String? ?? '',
      type: json['type'] as String? ?? '',
      severity: json['severity'] as String? ?? 'info',
      camera: json['camera'] as String? ?? '',
      message: json['message'] as String? ?? '',
      createdAt: json['created_at'] != null
          ? DateTime.parse(json['created_at'] as String)
          : DateTime.now(),
      readAt: json['read_at'] as String?,
      archived: json['archived'] as bool? ?? false,
    );
  }

  NotificationItem copyWith({String? readAt, bool? archived, bool clearReadAt = false}) {
    return NotificationItem(
      id: id,
      type: type,
      severity: severity,
      camera: camera,
      message: message,
      createdAt: createdAt,
      readAt: clearReadAt ? null : (readAt ?? this.readAt),
      archived: archived ?? this.archived,
    );
  }
}

/// Paginated response from the notifications list endpoint.
class NotificationPage {
  final List<NotificationItem> notifications;
  final int total;
  final int limit;
  final int offset;

  const NotificationPage({
    required this.notifications,
    required this.total,
    required this.limit,
    required this.offset,
  });

  factory NotificationPage.fromJson(Map<String, dynamic> json) {
    final list = (json['notifications'] as List<dynamic>?)
            ?.map((e) => NotificationItem.fromJson(e as Map<String, dynamic>))
            .toList() ??
        [];
    return NotificationPage(
      notifications: list,
      total: json['total'] as int? ?? 0,
      limit: json['limit'] as int? ?? 30,
      offset: json['offset'] as int? ?? 0,
    );
  }
}

/// Filter parameters for notification queries.
class NotificationFilter {
  final String camera;
  final String type;
  final String severity;
  final String read; // '' | 'true' | 'false'
  final bool archived;
  final String query;

  const NotificationFilter({
    this.camera = '',
    this.type = '',
    this.severity = '',
    this.read = '',
    this.archived = false,
    this.query = '',
  });

  NotificationFilter copyWith({
    String? camera,
    String? type,
    String? severity,
    String? read,
    bool? archived,
    String? query,
  }) {
    return NotificationFilter(
      camera: camera ?? this.camera,
      type: type ?? this.type,
      severity: severity ?? this.severity,
      read: read ?? this.read,
      archived: archived ?? this.archived,
      query: query ?? this.query,
    );
  }

  bool get hasActiveFilters =>
      camera.isNotEmpty ||
      type.isNotEmpty ||
      severity.isNotEmpty ||
      read.isNotEmpty ||
      query.isNotEmpty;
}

/// API client for notification center endpoints.
class NotificationApi {
  final ApiClient _client;

  NotificationApi(this._client);

  Future<NotificationPage> list({
    required NotificationFilter filter,
    int limit = 30,
    int offset = 0,
  }) async {
    final params = <String, dynamic>{
      'limit': limit,
      'offset': offset,
    };
    if (filter.archived) params['archived'] = 'true';
    if (filter.camera.isNotEmpty) params['camera'] = filter.camera;
    if (filter.type.isNotEmpty) params['type'] = filter.type;
    if (filter.severity.isNotEmpty) params['severity'] = filter.severity;
    if (filter.read.isNotEmpty) params['read'] = filter.read;
    if (filter.query.isNotEmpty) params['q'] = filter.query;

    final res = await _client.get<Map<String, dynamic>>(
      '/notifications',
      queryParameters: params,
    );
    return NotificationPage.fromJson(res.data ?? {});
  }

  Future<int> unreadCount() async {
    final res = await _client.get<Map<String, dynamic>>('/notifications/unread-count');
    return (res.data?['count'] as int?) ?? 0;
  }

  Future<void> markRead(List<String> ids) async {
    await _client.post('/notifications/mark-read', data: {'ids': ids});
  }

  Future<void> markUnread(List<String> ids) async {
    await _client.post('/notifications/mark-unread', data: {'ids': ids});
  }

  Future<void> markAllRead() async {
    await _client.post('/notifications/mark-all-read');
  }

  Future<void> archive(List<String> ids) async {
    await _client.post('/notifications/archive', data: {'ids': ids});
  }

  Future<void> restore(List<String> ids) async {
    await _client.post('/notifications/restore', data: {'ids': ids});
  }

  Future<void> deleteNotifications(List<String> ids) async {
    // The DELETE endpoint expects a JSON body; ApiClient.delete doesn't
    // support a body, so we use the underlying Dio instance directly.
    await _client.dio.delete('/notifications', data: {'ids': ids});
  }
}
