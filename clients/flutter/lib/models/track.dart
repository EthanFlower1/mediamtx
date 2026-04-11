/// Cross-camera person tracking model (KAI-482 -- beta).

class Sighting {
  final int id;
  final int trackId;
  final String cameraId;
  final String cameraName;
  final String timestamp;
  final String endTime;
  final double confidence;
  final String thumbnail;

  const Sighting({
    required this.id,
    required this.trackId,
    required this.cameraId,
    required this.cameraName,
    required this.timestamp,
    this.endTime = '',
    required this.confidence,
    this.thumbnail = '',
  });

  factory Sighting.fromJson(Map<String, dynamic> json) {
    return Sighting(
      id: json['id'] as int? ?? 0,
      trackId: json['track_id'] as int? ?? 0,
      cameraId: json['camera_id']?.toString() ?? '',
      cameraName: json['camera_name']?.toString() ?? '',
      timestamp: json['timestamp']?.toString() ?? '',
      endTime: json['end_time']?.toString() ?? '',
      confidence: _toDouble(json['confidence']),
      thumbnail: json['thumbnail']?.toString() ?? '',
    );
  }

  DateTime get time =>
      DateTime.tryParse(timestamp) ?? DateTime.fromMillisecondsSinceEpoch(0);

  static double _toDouble(dynamic value) {
    if (value == null) return 0.0;
    if (value is double) return value;
    if (value is int) return value.toDouble();
    if (value is String) return double.tryParse(value) ?? 0.0;
    return 0.0;
  }
}

class Track {
  final int id;
  final String label;
  final String status;
  final String createdAt;
  final String updatedAt;
  final int detectionId;
  final List<Sighting> sightings;
  final int cameraCount;

  const Track({
    required this.id,
    required this.label,
    required this.status,
    required this.createdAt,
    required this.updatedAt,
    required this.detectionId,
    required this.sightings,
    required this.cameraCount,
  });

  factory Track.fromJson(Map<String, dynamic> json) {
    final sightingsList = (json['sightings'] as List<dynamic>?)
            ?.map((e) => Sighting.fromJson(e as Map<String, dynamic>))
            .toList() ??
        [];
    return Track(
      id: json['id'] as int? ?? 0,
      label: json['label']?.toString() ?? '',
      status: json['status']?.toString() ?? 'active',
      createdAt: json['created_at']?.toString() ?? '',
      updatedAt: json['updated_at']?.toString() ?? '',
      detectionId: json['detection_id'] as int? ?? 0,
      sightings: sightingsList,
      cameraCount: json['camera_count'] as int? ?? 0,
    );
  }

  /// Groups sightings by camera ID.
  Map<String, List<Sighting>> get sightingsByCamera {
    final map = <String, List<Sighting>>{};
    for (final s in sightings) {
      map.putIfAbsent(s.cameraId, () => []).add(s);
    }
    return map;
  }

  /// Returns camera transitions (consecutive sightings on different cameras).
  List<({Sighting from, Sighting to})> get transitions {
    if (sightings.length < 2) return [];
    final sorted = List<Sighting>.from(sightings)
      ..sort((a, b) => a.timestamp.compareTo(b.timestamp));
    final result = <({Sighting from, Sighting to})>[];
    for (var i = 1; i < sorted.length; i++) {
      if (sorted[i].cameraId != sorted[i - 1].cameraId) {
        result.add((from: sorted[i - 1], to: sorted[i]));
      }
    }
    return result;
  }
}
