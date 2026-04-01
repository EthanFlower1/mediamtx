import 'package:freezed_annotation/freezed_annotation.dart';

part 'camera_group.freezed.dart';
part 'camera_group.g.dart';

@freezed
class CameraGroup with _$CameraGroup {
  const factory CameraGroup({
    required String id,
    required String name,
    @JsonKey(name: 'camera_ids') @Default([]) List<String> cameraIds,
    @JsonKey(name: 'created_at') required String? createdAt,
    @JsonKey(name: 'updated_at') required String? updatedAt,
  }) = _CameraGroup;

  factory CameraGroup.fromJson(Map<String, dynamic> json) =>
      _$CameraGroupFromJson(json);
}
