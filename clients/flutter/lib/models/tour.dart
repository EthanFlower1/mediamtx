import 'package:freezed_annotation/freezed_annotation.dart';

part 'tour.freezed.dart';
part 'tour.g.dart';

@freezed
class Tour with _$Tour {
  const factory Tour({
    required String id,
    required String name,
    @JsonKey(name: 'camera_ids') @Default([]) List<String> cameraIds,
    @JsonKey(name: 'dwell_seconds') @Default(10) int dwellSeconds,
    @JsonKey(name: 'created_at') required String? createdAt,
    @JsonKey(name: 'updated_at') required String? updatedAt,
  }) = _Tour;

  factory Tour.fromJson(Map<String, dynamic> json) =>
      _$TourFromJson(json);
}
