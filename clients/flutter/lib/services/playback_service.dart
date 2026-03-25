class PlaybackService {
  final String serverUrl;
  PlaybackService({required this.serverUrl});

  /// Builds the VoD URL pointing to MediaMTX's built-in playback endpoint.
  ///
  /// MediaMTX serves recordings at port 9996:
  ///   GET /get?path=CAMERA_PATH&start=RFC3339&duration=SECONDS
  ///
  /// The NVR JWT token (which includes "action": "playback") authenticates.
  String vodUrl({
    required String cameraPath,
    required DateTime start,
    int durationSecs = 7200,
    String? token,
  }) {
    final uri = Uri.parse(serverUrl);
    final startStr = _toRfc3339(start);
    final base = '${uri.scheme}://${uri.host}:9996/get';
    final params = <String, String>{
      'path': cameraPath,
      'start': startStr,
      'duration': durationSecs.toString(),
    };
    if (token != null && token.isNotEmpty) {
      params['jwt'] = token;
    }
    return Uri.parse(base).replace(queryParameters: params).toString();
  }

  static String _toRfc3339(DateTime d) {
    final offset = d.timeZoneOffset;
    final sign = offset.isNegative ? '-' : '+';
    final abs = offset.abs();
    final h = abs.inHours.toString().padLeft(2, '0');
    final m = (abs.inMinutes % 60).toString().padLeft(2, '0');
    return '${d.year}-${_p(d.month)}-${_p(d.day)}'
        'T${_p(d.hour)}:${_p(d.minute)}:${_p(d.second)}$sign$h:$m';
  }

  static String _p(int n) => n.toString().padLeft(2, '0');
}
