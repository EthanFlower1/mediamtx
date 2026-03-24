// coverage:ignore-file
// GENERATED CODE - DO NOT MODIFY BY HAND
// ignore_for_file: type=lint
// ignore_for_file: unused_element, deprecated_member_use, deprecated_member_use_from_same_package, use_function_type_syntax_for_parameters, unnecessary_const, avoid_init_to_null, invalid_override_different_default_values_named, prefer_expression_function_bodies, annotate_overrides, invalid_annotation_target, unnecessary_question_mark

part of 'camera.dart';

// **************************************************************************
// FreezedGenerator
// **************************************************************************

T _$identity<T>(T value) => value;

final _privateConstructorUsedError = UnsupportedError(
    'It seems like you constructed your class using `MyClass._()`. This constructor is only meant to be used by freezed and you are not supposed to need it nor use it.\nPlease check the documentation here for more information: https://github.com/rrousselGit/freezed#adding-getters-and-methods-to-our-models');

Camera _$CameraFromJson(Map<String, dynamic> json) {
  return _Camera.fromJson(json);
}

/// @nodoc
mixin _$Camera {
  String get id => throw _privateConstructorUsedError;
  String get name => throw _privateConstructorUsedError;
  @JsonKey(name: 'rtsp_url')
  String get rtspUrl => throw _privateConstructorUsedError;
  @JsonKey(name: 'onvif_endpoint')
  String get onvifEndpoint => throw _privateConstructorUsedError;
  @JsonKey(name: 'mediamtx_path')
  String get mediamtxPath => throw _privateConstructorUsedError;
  String get status => throw _privateConstructorUsedError;
  @JsonKey(name: 'ptz_capable')
  bool get ptzCapable => throw _privateConstructorUsedError;
  @JsonKey(name: 'ai_enabled')
  bool get aiEnabled => throw _privateConstructorUsedError;
  @JsonKey(name: 'sub_stream_url')
  String get subStreamUrl => throw _privateConstructorUsedError;
  @JsonKey(name: 'retention_days')
  int get retentionDays => throw _privateConstructorUsedError;
  @JsonKey(name: 'motion_timeout_seconds')
  int get motionTimeoutSeconds => throw _privateConstructorUsedError;
  @JsonKey(name: 'snapshot_uri')
  String get snapshotUri => throw _privateConstructorUsedError;
  @JsonKey(name: 'supports_events')
  bool get supportsEvents => throw _privateConstructorUsedError;
  @JsonKey(name: 'supports_analytics')
  bool get supportsAnalytics => throw _privateConstructorUsedError;
  @JsonKey(name: 'supports_relay')
  bool get supportsRelay => throw _privateConstructorUsedError;
  @JsonKey(name: 'created_at')
  String? get createdAt => throw _privateConstructorUsedError;
  @JsonKey(name: 'updated_at')
  String? get updatedAt => throw _privateConstructorUsedError;

  /// Serializes this Camera to a JSON map.
  Map<String, dynamic> toJson() => throw _privateConstructorUsedError;

  /// Create a copy of Camera
  /// with the given fields replaced by the non-null parameter values.
  @JsonKey(includeFromJson: false, includeToJson: false)
  $CameraCopyWith<Camera> get copyWith => throw _privateConstructorUsedError;
}

/// @nodoc
abstract class $CameraCopyWith<$Res> {
  factory $CameraCopyWith(Camera value, $Res Function(Camera) then) =
      _$CameraCopyWithImpl<$Res, Camera>;
  @useResult
  $Res call(
      {String id,
      String name,
      @JsonKey(name: 'rtsp_url') String rtspUrl,
      @JsonKey(name: 'onvif_endpoint') String onvifEndpoint,
      @JsonKey(name: 'mediamtx_path') String mediamtxPath,
      String status,
      @JsonKey(name: 'ptz_capable') bool ptzCapable,
      @JsonKey(name: 'ai_enabled') bool aiEnabled,
      @JsonKey(name: 'sub_stream_url') String subStreamUrl,
      @JsonKey(name: 'retention_days') int retentionDays,
      @JsonKey(name: 'motion_timeout_seconds') int motionTimeoutSeconds,
      @JsonKey(name: 'snapshot_uri') String snapshotUri,
      @JsonKey(name: 'supports_events') bool supportsEvents,
      @JsonKey(name: 'supports_analytics') bool supportsAnalytics,
      @JsonKey(name: 'supports_relay') bool supportsRelay,
      @JsonKey(name: 'created_at') String? createdAt,
      @JsonKey(name: 'updated_at') String? updatedAt});
}

/// @nodoc
class _$CameraCopyWithImpl<$Res, $Val extends Camera>
    implements $CameraCopyWith<$Res> {
  _$CameraCopyWithImpl(this._value, this._then);

  // ignore: unused_field
  final $Val _value;
  // ignore: unused_field
  final $Res Function($Val) _then;

  /// Create a copy of Camera
  /// with the given fields replaced by the non-null parameter values.
  @pragma('vm:prefer-inline')
  @override
  $Res call({
    Object? id = null,
    Object? name = null,
    Object? rtspUrl = null,
    Object? onvifEndpoint = null,
    Object? mediamtxPath = null,
    Object? status = null,
    Object? ptzCapable = null,
    Object? aiEnabled = null,
    Object? subStreamUrl = null,
    Object? retentionDays = null,
    Object? motionTimeoutSeconds = null,
    Object? snapshotUri = null,
    Object? supportsEvents = null,
    Object? supportsAnalytics = null,
    Object? supportsRelay = null,
    Object? createdAt = freezed,
    Object? updatedAt = freezed,
  }) {
    return _then(_value.copyWith(
      id: null == id
          ? _value.id
          : id // ignore: cast_nullable_to_non_nullable
              as String,
      name: null == name
          ? _value.name
          : name // ignore: cast_nullable_to_non_nullable
              as String,
      rtspUrl: null == rtspUrl
          ? _value.rtspUrl
          : rtspUrl // ignore: cast_nullable_to_non_nullable
              as String,
      onvifEndpoint: null == onvifEndpoint
          ? _value.onvifEndpoint
          : onvifEndpoint // ignore: cast_nullable_to_non_nullable
              as String,
      mediamtxPath: null == mediamtxPath
          ? _value.mediamtxPath
          : mediamtxPath // ignore: cast_nullable_to_non_nullable
              as String,
      status: null == status
          ? _value.status
          : status // ignore: cast_nullable_to_non_nullable
              as String,
      ptzCapable: null == ptzCapable
          ? _value.ptzCapable
          : ptzCapable // ignore: cast_nullable_to_non_nullable
              as bool,
      aiEnabled: null == aiEnabled
          ? _value.aiEnabled
          : aiEnabled // ignore: cast_nullable_to_non_nullable
              as bool,
      subStreamUrl: null == subStreamUrl
          ? _value.subStreamUrl
          : subStreamUrl // ignore: cast_nullable_to_non_nullable
              as String,
      retentionDays: null == retentionDays
          ? _value.retentionDays
          : retentionDays // ignore: cast_nullable_to_non_nullable
              as int,
      motionTimeoutSeconds: null == motionTimeoutSeconds
          ? _value.motionTimeoutSeconds
          : motionTimeoutSeconds // ignore: cast_nullable_to_non_nullable
              as int,
      snapshotUri: null == snapshotUri
          ? _value.snapshotUri
          : snapshotUri // ignore: cast_nullable_to_non_nullable
              as String,
      supportsEvents: null == supportsEvents
          ? _value.supportsEvents
          : supportsEvents // ignore: cast_nullable_to_non_nullable
              as bool,
      supportsAnalytics: null == supportsAnalytics
          ? _value.supportsAnalytics
          : supportsAnalytics // ignore: cast_nullable_to_non_nullable
              as bool,
      supportsRelay: null == supportsRelay
          ? _value.supportsRelay
          : supportsRelay // ignore: cast_nullable_to_non_nullable
              as bool,
      createdAt: freezed == createdAt
          ? _value.createdAt
          : createdAt // ignore: cast_nullable_to_non_nullable
              as String?,
      updatedAt: freezed == updatedAt
          ? _value.updatedAt
          : updatedAt // ignore: cast_nullable_to_non_nullable
              as String?,
    ) as $Val);
  }
}

/// @nodoc
abstract class _$$CameraImplCopyWith<$Res> implements $CameraCopyWith<$Res> {
  factory _$$CameraImplCopyWith(
          _$CameraImpl value, $Res Function(_$CameraImpl) then) =
      __$$CameraImplCopyWithImpl<$Res>;
  @override
  @useResult
  $Res call(
      {String id,
      String name,
      @JsonKey(name: 'rtsp_url') String rtspUrl,
      @JsonKey(name: 'onvif_endpoint') String onvifEndpoint,
      @JsonKey(name: 'mediamtx_path') String mediamtxPath,
      String status,
      @JsonKey(name: 'ptz_capable') bool ptzCapable,
      @JsonKey(name: 'ai_enabled') bool aiEnabled,
      @JsonKey(name: 'sub_stream_url') String subStreamUrl,
      @JsonKey(name: 'retention_days') int retentionDays,
      @JsonKey(name: 'motion_timeout_seconds') int motionTimeoutSeconds,
      @JsonKey(name: 'snapshot_uri') String snapshotUri,
      @JsonKey(name: 'supports_events') bool supportsEvents,
      @JsonKey(name: 'supports_analytics') bool supportsAnalytics,
      @JsonKey(name: 'supports_relay') bool supportsRelay,
      @JsonKey(name: 'created_at') String? createdAt,
      @JsonKey(name: 'updated_at') String? updatedAt});
}

/// @nodoc
class __$$CameraImplCopyWithImpl<$Res>
    extends _$CameraCopyWithImpl<$Res, _$CameraImpl>
    implements _$$CameraImplCopyWith<$Res> {
  __$$CameraImplCopyWithImpl(
      _$CameraImpl _value, $Res Function(_$CameraImpl) _then)
      : super(_value, _then);

  /// Create a copy of Camera
  /// with the given fields replaced by the non-null parameter values.
  @pragma('vm:prefer-inline')
  @override
  $Res call({
    Object? id = null,
    Object? name = null,
    Object? rtspUrl = null,
    Object? onvifEndpoint = null,
    Object? mediamtxPath = null,
    Object? status = null,
    Object? ptzCapable = null,
    Object? aiEnabled = null,
    Object? subStreamUrl = null,
    Object? retentionDays = null,
    Object? motionTimeoutSeconds = null,
    Object? snapshotUri = null,
    Object? supportsEvents = null,
    Object? supportsAnalytics = null,
    Object? supportsRelay = null,
    Object? createdAt = freezed,
    Object? updatedAt = freezed,
  }) {
    return _then(_$CameraImpl(
      id: null == id
          ? _value.id
          : id // ignore: cast_nullable_to_non_nullable
              as String,
      name: null == name
          ? _value.name
          : name // ignore: cast_nullable_to_non_nullable
              as String,
      rtspUrl: null == rtspUrl
          ? _value.rtspUrl
          : rtspUrl // ignore: cast_nullable_to_non_nullable
              as String,
      onvifEndpoint: null == onvifEndpoint
          ? _value.onvifEndpoint
          : onvifEndpoint // ignore: cast_nullable_to_non_nullable
              as String,
      mediamtxPath: null == mediamtxPath
          ? _value.mediamtxPath
          : mediamtxPath // ignore: cast_nullable_to_non_nullable
              as String,
      status: null == status
          ? _value.status
          : status // ignore: cast_nullable_to_non_nullable
              as String,
      ptzCapable: null == ptzCapable
          ? _value.ptzCapable
          : ptzCapable // ignore: cast_nullable_to_non_nullable
              as bool,
      aiEnabled: null == aiEnabled
          ? _value.aiEnabled
          : aiEnabled // ignore: cast_nullable_to_non_nullable
              as bool,
      subStreamUrl: null == subStreamUrl
          ? _value.subStreamUrl
          : subStreamUrl // ignore: cast_nullable_to_non_nullable
              as String,
      retentionDays: null == retentionDays
          ? _value.retentionDays
          : retentionDays // ignore: cast_nullable_to_non_nullable
              as int,
      motionTimeoutSeconds: null == motionTimeoutSeconds
          ? _value.motionTimeoutSeconds
          : motionTimeoutSeconds // ignore: cast_nullable_to_non_nullable
              as int,
      snapshotUri: null == snapshotUri
          ? _value.snapshotUri
          : snapshotUri // ignore: cast_nullable_to_non_nullable
              as String,
      supportsEvents: null == supportsEvents
          ? _value.supportsEvents
          : supportsEvents // ignore: cast_nullable_to_non_nullable
              as bool,
      supportsAnalytics: null == supportsAnalytics
          ? _value.supportsAnalytics
          : supportsAnalytics // ignore: cast_nullable_to_non_nullable
              as bool,
      supportsRelay: null == supportsRelay
          ? _value.supportsRelay
          : supportsRelay // ignore: cast_nullable_to_non_nullable
              as bool,
      createdAt: freezed == createdAt
          ? _value.createdAt
          : createdAt // ignore: cast_nullable_to_non_nullable
              as String?,
      updatedAt: freezed == updatedAt
          ? _value.updatedAt
          : updatedAt // ignore: cast_nullable_to_non_nullable
              as String?,
    ));
  }
}

/// @nodoc
@JsonSerializable()
class _$CameraImpl implements _Camera {
  const _$CameraImpl(
      {required this.id,
      required this.name,
      @JsonKey(name: 'rtsp_url') this.rtspUrl = '',
      @JsonKey(name: 'onvif_endpoint') this.onvifEndpoint = '',
      @JsonKey(name: 'mediamtx_path') this.mediamtxPath = '',
      this.status = 'disconnected',
      @JsonKey(name: 'ptz_capable') this.ptzCapable = false,
      @JsonKey(name: 'ai_enabled') this.aiEnabled = false,
      @JsonKey(name: 'sub_stream_url') this.subStreamUrl = '',
      @JsonKey(name: 'retention_days') this.retentionDays = 30,
      @JsonKey(name: 'motion_timeout_seconds') this.motionTimeoutSeconds = 8,
      @JsonKey(name: 'snapshot_uri') this.snapshotUri = '',
      @JsonKey(name: 'supports_events') this.supportsEvents = false,
      @JsonKey(name: 'supports_analytics') this.supportsAnalytics = false,
      @JsonKey(name: 'supports_relay') this.supportsRelay = false,
      @JsonKey(name: 'created_at') this.createdAt,
      @JsonKey(name: 'updated_at') this.updatedAt});

  factory _$CameraImpl.fromJson(Map<String, dynamic> json) =>
      _$$CameraImplFromJson(json);

  @override
  final String id;
  @override
  final String name;
  @override
  @JsonKey(name: 'rtsp_url')
  final String rtspUrl;
  @override
  @JsonKey(name: 'onvif_endpoint')
  final String onvifEndpoint;
  @override
  @JsonKey(name: 'mediamtx_path')
  final String mediamtxPath;
  @override
  @JsonKey()
  final String status;
  @override
  @JsonKey(name: 'ptz_capable')
  final bool ptzCapable;
  @override
  @JsonKey(name: 'ai_enabled')
  final bool aiEnabled;
  @override
  @JsonKey(name: 'sub_stream_url')
  final String subStreamUrl;
  @override
  @JsonKey(name: 'retention_days')
  final int retentionDays;
  @override
  @JsonKey(name: 'motion_timeout_seconds')
  final int motionTimeoutSeconds;
  @override
  @JsonKey(name: 'snapshot_uri')
  final String snapshotUri;
  @override
  @JsonKey(name: 'supports_events')
  final bool supportsEvents;
  @override
  @JsonKey(name: 'supports_analytics')
  final bool supportsAnalytics;
  @override
  @JsonKey(name: 'supports_relay')
  final bool supportsRelay;
  @override
  @JsonKey(name: 'created_at')
  final String? createdAt;
  @override
  @JsonKey(name: 'updated_at')
  final String? updatedAt;

  @override
  String toString() {
    return 'Camera(id: $id, name: $name, rtspUrl: $rtspUrl, onvifEndpoint: $onvifEndpoint, mediamtxPath: $mediamtxPath, status: $status, ptzCapable: $ptzCapable, aiEnabled: $aiEnabled, subStreamUrl: $subStreamUrl, retentionDays: $retentionDays, motionTimeoutSeconds: $motionTimeoutSeconds, snapshotUri: $snapshotUri, supportsEvents: $supportsEvents, supportsAnalytics: $supportsAnalytics, supportsRelay: $supportsRelay, createdAt: $createdAt, updatedAt: $updatedAt)';
  }

  @override
  bool operator ==(Object other) {
    return identical(this, other) ||
        (other.runtimeType == runtimeType &&
            other is _$CameraImpl &&
            (identical(other.id, id) || other.id == id) &&
            (identical(other.name, name) || other.name == name) &&
            (identical(other.rtspUrl, rtspUrl) || other.rtspUrl == rtspUrl) &&
            (identical(other.onvifEndpoint, onvifEndpoint) ||
                other.onvifEndpoint == onvifEndpoint) &&
            (identical(other.mediamtxPath, mediamtxPath) ||
                other.mediamtxPath == mediamtxPath) &&
            (identical(other.status, status) || other.status == status) &&
            (identical(other.ptzCapable, ptzCapable) ||
                other.ptzCapable == ptzCapable) &&
            (identical(other.aiEnabled, aiEnabled) ||
                other.aiEnabled == aiEnabled) &&
            (identical(other.subStreamUrl, subStreamUrl) ||
                other.subStreamUrl == subStreamUrl) &&
            (identical(other.retentionDays, retentionDays) ||
                other.retentionDays == retentionDays) &&
            (identical(other.motionTimeoutSeconds, motionTimeoutSeconds) ||
                other.motionTimeoutSeconds == motionTimeoutSeconds) &&
            (identical(other.snapshotUri, snapshotUri) ||
                other.snapshotUri == snapshotUri) &&
            (identical(other.supportsEvents, supportsEvents) ||
                other.supportsEvents == supportsEvents) &&
            (identical(other.supportsAnalytics, supportsAnalytics) ||
                other.supportsAnalytics == supportsAnalytics) &&
            (identical(other.supportsRelay, supportsRelay) ||
                other.supportsRelay == supportsRelay) &&
            (identical(other.createdAt, createdAt) ||
                other.createdAt == createdAt) &&
            (identical(other.updatedAt, updatedAt) ||
                other.updatedAt == updatedAt));
  }

  @JsonKey(includeFromJson: false, includeToJson: false)
  @override
  int get hashCode => Object.hash(
      runtimeType,
      id,
      name,
      rtspUrl,
      onvifEndpoint,
      mediamtxPath,
      status,
      ptzCapable,
      aiEnabled,
      subStreamUrl,
      retentionDays,
      motionTimeoutSeconds,
      snapshotUri,
      supportsEvents,
      supportsAnalytics,
      supportsRelay,
      createdAt,
      updatedAt);

  /// Create a copy of Camera
  /// with the given fields replaced by the non-null parameter values.
  @JsonKey(includeFromJson: false, includeToJson: false)
  @override
  @pragma('vm:prefer-inline')
  _$$CameraImplCopyWith<_$CameraImpl> get copyWith =>
      __$$CameraImplCopyWithImpl<_$CameraImpl>(this, _$identity);

  @override
  Map<String, dynamic> toJson() {
    return _$$CameraImplToJson(
      this,
    );
  }
}

abstract class _Camera implements Camera {
  const factory _Camera(
      {required final String id,
      required final String name,
      @JsonKey(name: 'rtsp_url') final String rtspUrl,
      @JsonKey(name: 'onvif_endpoint') final String onvifEndpoint,
      @JsonKey(name: 'mediamtx_path') final String mediamtxPath,
      final String status,
      @JsonKey(name: 'ptz_capable') final bool ptzCapable,
      @JsonKey(name: 'ai_enabled') final bool aiEnabled,
      @JsonKey(name: 'sub_stream_url') final String subStreamUrl,
      @JsonKey(name: 'retention_days') final int retentionDays,
      @JsonKey(name: 'motion_timeout_seconds') final int motionTimeoutSeconds,
      @JsonKey(name: 'snapshot_uri') final String snapshotUri,
      @JsonKey(name: 'supports_events') final bool supportsEvents,
      @JsonKey(name: 'supports_analytics') final bool supportsAnalytics,
      @JsonKey(name: 'supports_relay') final bool supportsRelay,
      @JsonKey(name: 'created_at') final String? createdAt,
      @JsonKey(name: 'updated_at') final String? updatedAt}) = _$CameraImpl;

  factory _Camera.fromJson(Map<String, dynamic> json) = _$CameraImpl.fromJson;

  @override
  String get id;
  @override
  String get name;
  @override
  @JsonKey(name: 'rtsp_url')
  String get rtspUrl;
  @override
  @JsonKey(name: 'onvif_endpoint')
  String get onvifEndpoint;
  @override
  @JsonKey(name: 'mediamtx_path')
  String get mediamtxPath;
  @override
  String get status;
  @override
  @JsonKey(name: 'ptz_capable')
  bool get ptzCapable;
  @override
  @JsonKey(name: 'ai_enabled')
  bool get aiEnabled;
  @override
  @JsonKey(name: 'sub_stream_url')
  String get subStreamUrl;
  @override
  @JsonKey(name: 'retention_days')
  int get retentionDays;
  @override
  @JsonKey(name: 'motion_timeout_seconds')
  int get motionTimeoutSeconds;
  @override
  @JsonKey(name: 'snapshot_uri')
  String get snapshotUri;
  @override
  @JsonKey(name: 'supports_events')
  bool get supportsEvents;
  @override
  @JsonKey(name: 'supports_analytics')
  bool get supportsAnalytics;
  @override
  @JsonKey(name: 'supports_relay')
  bool get supportsRelay;
  @override
  @JsonKey(name: 'created_at')
  String? get createdAt;
  @override
  @JsonKey(name: 'updated_at')
  String? get updatedAt;

  /// Create a copy of Camera
  /// with the given fields replaced by the non-null parameter values.
  @override
  @JsonKey(includeFromJson: false, includeToJson: false)
  _$$CameraImplCopyWith<_$CameraImpl> get copyWith =>
      throw _privateConstructorUsedError;
}
