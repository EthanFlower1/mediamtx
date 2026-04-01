// GENERATED CODE - DO NOT MODIFY BY HAND

part of 'camera_group.dart';

// **************************************************************************
// JsonSerializableGenerator
// **************************************************************************

_$CameraGroupImpl _$$CameraGroupImplFromJson(Map<String, dynamic> json) =>
    _$CameraGroupImpl(
      id: json['id'] as String,
      name: json['name'] as String,
      cameraIds: (json['camera_ids'] as List<dynamic>?)
              ?.map((e) => e as String)
              .toList() ??
          const [],
      createdAt: json['created_at'] as String?,
      updatedAt: json['updated_at'] as String?,
    );

Map<String, dynamic> _$$CameraGroupImplToJson(_$CameraGroupImpl instance) =>
    <String, dynamic>{
      'id': instance.id,
      'name': instance.name,
      'camera_ids': instance.cameraIds,
      'created_at': instance.createdAt,
      'updated_at': instance.updatedAt,
    };
