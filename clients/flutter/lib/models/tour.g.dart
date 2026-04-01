// GENERATED CODE - DO NOT MODIFY BY HAND

part of 'tour.dart';

// **************************************************************************
// JsonSerializableGenerator
// **************************************************************************

_$TourImpl _$$TourImplFromJson(Map<String, dynamic> json) => _$TourImpl(
      id: json['id'] as String,
      name: json['name'] as String,
      cameraIds: (json['camera_ids'] as List<dynamic>?)
              ?.map((e) => e as String)
              .toList() ??
          const [],
      dwellSeconds: (json['dwell_seconds'] as num?)?.toInt() ?? 10,
      createdAt: json['created_at'] as String?,
      updatedAt: json['updated_at'] as String?,
    );

Map<String, dynamic> _$$TourImplToJson(_$TourImpl instance) =>
    <String, dynamic>{
      'id': instance.id,
      'name': instance.name,
      'camera_ids': instance.cameraIds,
      'dwell_seconds': instance.dwellSeconds,
      'created_at': instance.createdAt,
      'updated_at': instance.updatedAt,
    };
