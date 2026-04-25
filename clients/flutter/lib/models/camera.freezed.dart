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
  @JsonKey(name: 'ai_stream_id')
  String get aiStreamId => throw _privateConstructorUsedError;
  @JsonKey(name: 'ai_confidence')
  double get aiConfidence => throw _privateConstructorUsedError;
  @JsonKey(name: 'ai_track_timeout')
  int get aiTrackTimeout => throw _privateConstructorUsedError;
  @JsonKey(name: 'sub_stream_url')
  String get subStreamUrl =>
      throw _privateConstructorUsedError; // Proto capability flags per lead-cloud feedback (replaces stream URLs on proto)
  @JsonKey(name: 'has_sub_stream')
  bool get hasSubStream => throw _privateConstructorUsedError;
  @JsonKey(name: 'has_main_stream')
  bool get hasMainStream => throw _privateConstructorUsedError;
  @JsonKey(name: 'retention_days')
  int get retentionDays => throw _privateConstructorUsedError;
  @JsonKey(name: 'event_retention_days')
  int get eventRetentionDays => throw _privateConstructorUsedError;
  @JsonKey(name: 'detection_retention_days')
  int get detectionRetentionDays => throw _privateConstructorUsedError;
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
  @JsonKey(name: 'storage_path')
  String get storagePath => throw _privateConstructorUsedError;
  @JsonKey(name: 'storage_status')
  String get storageStatus => throw _privateConstructorUsedError;
  @JsonKey(name: 'live_view_path')
  String get liveViewPath => throw _privateConstructorUsedError;
  @JsonKey(name: 'live_view_codec')
  String get liveViewCodec => throw _privateConstructorUsedError;
  @JsonKey(name: 'stream_paths')
  List<StreamPath> get streamPaths =>
      throw _privateConstructorUsedError; // Recorder/Directory routing fields — populated by the Directory API so
// the client knows where to send data-plane requests (live view, playback,
// export) without an extra lookup hop.
  @JsonKey(name: 'recorder_id')
  String? get recorderId => throw _privateConstructorUsedError;
  @JsonKey(name: 'recorder_endpoint')
  String? get recorderEndpoint => throw _privateConstructorUsedError;
  @JsonKey(name: 'directory_id')
  String? get directoryId => throw _privateConstructorUsedError;

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
      @JsonKey(name: 'ai_stream_id') String aiStreamId,
      @JsonKey(name: 'ai_confidence') double aiConfidence,
      @JsonKey(name: 'ai_track_timeout') int aiTrackTimeout,
      @JsonKey(name: 'sub_stream_url') String subStreamUrl,
      @JsonKey(name: 'has_sub_stream') bool hasSubStream,
      @JsonKey(name: 'has_main_stream') bool hasMainStream,
      @JsonKey(name: 'retention_days') int retentionDays,
      @JsonKey(name: 'event_retention_days') int eventRetentionDays,
      @JsonKey(name: 'detection_retention_days') int detectionRetentionDays,
      @JsonKey(name: 'motion_timeout_seconds') int motionTimeoutSeconds,
      @JsonKey(name: 'snapshot_uri') String snapshotUri,
      @JsonKey(name: 'supports_events') bool supportsEvents,
      @JsonKey(name: 'supports_analytics') bool supportsAnalytics,
      @JsonKey(name: 'supports_relay') bool supportsRelay,
      @JsonKey(name: 'created_at') String? createdAt,
      @JsonKey(name: 'updated_at') String? updatedAt,
      @JsonKey(name: 'storage_path') String storagePath,
      @JsonKey(name: 'storage_status') String storageStatus,
      @JsonKey(name: 'live_view_path') String liveViewPath,
      @JsonKey(name: 'live_view_codec') String liveViewCodec,
      @JsonKey(name: 'stream_paths') List<StreamPath> streamPaths,
      @JsonKey(name: 'recorder_id') String? recorderId,
      @JsonKey(name: 'recorder_endpoint') String? recorderEndpoint,
      @JsonKey(name: 'directory_id') String? directoryId});
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
    Object? aiStreamId = null,
    Object? aiConfidence = null,
    Object? aiTrackTimeout = null,
    Object? subStreamUrl = null,
    Object? hasSubStream = null,
    Object? hasMainStream = null,
    Object? retentionDays = null,
    Object? eventRetentionDays = null,
    Object? detectionRetentionDays = null,
    Object? motionTimeoutSeconds = null,
    Object? snapshotUri = null,
    Object? supportsEvents = null,
    Object? supportsAnalytics = null,
    Object? supportsRelay = null,
    Object? createdAt = freezed,
    Object? updatedAt = freezed,
    Object? storagePath = null,
    Object? storageStatus = null,
    Object? liveViewPath = null,
    Object? liveViewCodec = null,
    Object? streamPaths = null,
    Object? recorderId = freezed,
    Object? recorderEndpoint = freezed,
    Object? directoryId = freezed,
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
      aiStreamId: null == aiStreamId
          ? _value.aiStreamId
          : aiStreamId // ignore: cast_nullable_to_non_nullable
              as String,
      aiConfidence: null == aiConfidence
          ? _value.aiConfidence
          : aiConfidence // ignore: cast_nullable_to_non_nullable
              as double,
      aiTrackTimeout: null == aiTrackTimeout
          ? _value.aiTrackTimeout
          : aiTrackTimeout // ignore: cast_nullable_to_non_nullable
              as int,
      subStreamUrl: null == subStreamUrl
          ? _value.subStreamUrl
          : subStreamUrl // ignore: cast_nullable_to_non_nullable
              as String,
      hasSubStream: null == hasSubStream
          ? _value.hasSubStream
          : hasSubStream // ignore: cast_nullable_to_non_nullable
              as bool,
      hasMainStream: null == hasMainStream
          ? _value.hasMainStream
          : hasMainStream // ignore: cast_nullable_to_non_nullable
              as bool,
      retentionDays: null == retentionDays
          ? _value.retentionDays
          : retentionDays // ignore: cast_nullable_to_non_nullable
              as int,
      eventRetentionDays: null == eventRetentionDays
          ? _value.eventRetentionDays
          : eventRetentionDays // ignore: cast_nullable_to_non_nullable
              as int,
      detectionRetentionDays: null == detectionRetentionDays
          ? _value.detectionRetentionDays
          : detectionRetentionDays // ignore: cast_nullable_to_non_nullable
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
      storagePath: null == storagePath
          ? _value.storagePath
          : storagePath // ignore: cast_nullable_to_non_nullable
              as String,
      storageStatus: null == storageStatus
          ? _value.storageStatus
          : storageStatus // ignore: cast_nullable_to_non_nullable
              as String,
      liveViewPath: null == liveViewPath
          ? _value.liveViewPath
          : liveViewPath // ignore: cast_nullable_to_non_nullable
              as String,
      liveViewCodec: null == liveViewCodec
          ? _value.liveViewCodec
          : liveViewCodec // ignore: cast_nullable_to_non_nullable
              as String,
      streamPaths: null == streamPaths
          ? _value.streamPaths
          : streamPaths // ignore: cast_nullable_to_non_nullable
              as List<StreamPath>,
      recorderId: freezed == recorderId
          ? _value.recorderId
          : recorderId // ignore: cast_nullable_to_non_nullable
              as String?,
      recorderEndpoint: freezed == recorderEndpoint
          ? _value.recorderEndpoint
          : recorderEndpoint // ignore: cast_nullable_to_non_nullable
              as String?,
      directoryId: freezed == directoryId
          ? _value.directoryId
          : directoryId // ignore: cast_nullable_to_non_nullable
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
      @JsonKey(name: 'ai_stream_id') String aiStreamId,
      @JsonKey(name: 'ai_confidence') double aiConfidence,
      @JsonKey(name: 'ai_track_timeout') int aiTrackTimeout,
      @JsonKey(name: 'sub_stream_url') String subStreamUrl,
      @JsonKey(name: 'has_sub_stream') bool hasSubStream,
      @JsonKey(name: 'has_main_stream') bool hasMainStream,
      @JsonKey(name: 'retention_days') int retentionDays,
      @JsonKey(name: 'event_retention_days') int eventRetentionDays,
      @JsonKey(name: 'detection_retention_days') int detectionRetentionDays,
      @JsonKey(name: 'motion_timeout_seconds') int motionTimeoutSeconds,
      @JsonKey(name: 'snapshot_uri') String snapshotUri,
      @JsonKey(name: 'supports_events') bool supportsEvents,
      @JsonKey(name: 'supports_analytics') bool supportsAnalytics,
      @JsonKey(name: 'supports_relay') bool supportsRelay,
      @JsonKey(name: 'created_at') String? createdAt,
      @JsonKey(name: 'updated_at') String? updatedAt,
      @JsonKey(name: 'storage_path') String storagePath,
      @JsonKey(name: 'storage_status') String storageStatus,
      @JsonKey(name: 'live_view_path') String liveViewPath,
      @JsonKey(name: 'live_view_codec') String liveViewCodec,
      @JsonKey(name: 'stream_paths') List<StreamPath> streamPaths,
      @JsonKey(name: 'recorder_id') String? recorderId,
      @JsonKey(name: 'recorder_endpoint') String? recorderEndpoint,
      @JsonKey(name: 'directory_id') String? directoryId});
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
    Object? aiStreamId = null,
    Object? aiConfidence = null,
    Object? aiTrackTimeout = null,
    Object? subStreamUrl = null,
    Object? hasSubStream = null,
    Object? hasMainStream = null,
    Object? retentionDays = null,
    Object? eventRetentionDays = null,
    Object? detectionRetentionDays = null,
    Object? motionTimeoutSeconds = null,
    Object? snapshotUri = null,
    Object? supportsEvents = null,
    Object? supportsAnalytics = null,
    Object? supportsRelay = null,
    Object? createdAt = freezed,
    Object? updatedAt = freezed,
    Object? storagePath = null,
    Object? storageStatus = null,
    Object? liveViewPath = null,
    Object? liveViewCodec = null,
    Object? streamPaths = null,
    Object? recorderId = freezed,
    Object? recorderEndpoint = freezed,
    Object? directoryId = freezed,
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
      aiStreamId: null == aiStreamId
          ? _value.aiStreamId
          : aiStreamId // ignore: cast_nullable_to_non_nullable
              as String,
      aiConfidence: null == aiConfidence
          ? _value.aiConfidence
          : aiConfidence // ignore: cast_nullable_to_non_nullable
              as double,
      aiTrackTimeout: null == aiTrackTimeout
          ? _value.aiTrackTimeout
          : aiTrackTimeout // ignore: cast_nullable_to_non_nullable
              as int,
      subStreamUrl: null == subStreamUrl
          ? _value.subStreamUrl
          : subStreamUrl // ignore: cast_nullable_to_non_nullable
              as String,
      hasSubStream: null == hasSubStream
          ? _value.hasSubStream
          : hasSubStream // ignore: cast_nullable_to_non_nullable
              as bool,
      hasMainStream: null == hasMainStream
          ? _value.hasMainStream
          : hasMainStream // ignore: cast_nullable_to_non_nullable
              as bool,
      retentionDays: null == retentionDays
          ? _value.retentionDays
          : retentionDays // ignore: cast_nullable_to_non_nullable
              as int,
      eventRetentionDays: null == eventRetentionDays
          ? _value.eventRetentionDays
          : eventRetentionDays // ignore: cast_nullable_to_non_nullable
              as int,
      detectionRetentionDays: null == detectionRetentionDays
          ? _value.detectionRetentionDays
          : detectionRetentionDays // ignore: cast_nullable_to_non_nullable
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
      storagePath: null == storagePath
          ? _value.storagePath
          : storagePath // ignore: cast_nullable_to_non_nullable
              as String,
      storageStatus: null == storageStatus
          ? _value.storageStatus
          : storageStatus // ignore: cast_nullable_to_non_nullable
              as String,
      liveViewPath: null == liveViewPath
          ? _value.liveViewPath
          : liveViewPath // ignore: cast_nullable_to_non_nullable
              as String,
      liveViewCodec: null == liveViewCodec
          ? _value.liveViewCodec
          : liveViewCodec // ignore: cast_nullable_to_non_nullable
              as String,
      streamPaths: null == streamPaths
          ? _value._streamPaths
          : streamPaths // ignore: cast_nullable_to_non_nullable
              as List<StreamPath>,
      recorderId: freezed == recorderId
          ? _value.recorderId
          : recorderId // ignore: cast_nullable_to_non_nullable
              as String?,
      recorderEndpoint: freezed == recorderEndpoint
          ? _value.recorderEndpoint
          : recorderEndpoint // ignore: cast_nullable_to_non_nullable
              as String?,
      directoryId: freezed == directoryId
          ? _value.directoryId
          : directoryId // ignore: cast_nullable_to_non_nullable
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
      @JsonKey(name: 'ai_stream_id') this.aiStreamId = '',
      @JsonKey(name: 'ai_confidence') this.aiConfidence = 0.5,
      @JsonKey(name: 'ai_track_timeout') this.aiTrackTimeout = 5,
      @JsonKey(name: 'sub_stream_url') this.subStreamUrl = '',
      @JsonKey(name: 'has_sub_stream') this.hasSubStream = false,
      @JsonKey(name: 'has_main_stream') this.hasMainStream = true,
      @JsonKey(name: 'retention_days') this.retentionDays = 30,
      @JsonKey(name: 'event_retention_days') this.eventRetentionDays = 0,
      @JsonKey(name: 'detection_retention_days')
      this.detectionRetentionDays = 0,
      @JsonKey(name: 'motion_timeout_seconds') this.motionTimeoutSeconds = 8,
      @JsonKey(name: 'snapshot_uri') this.snapshotUri = '',
      @JsonKey(name: 'supports_events') this.supportsEvents = false,
      @JsonKey(name: 'supports_analytics') this.supportsAnalytics = false,
      @JsonKey(name: 'supports_relay') this.supportsRelay = false,
      @JsonKey(name: 'created_at') this.createdAt,
      @JsonKey(name: 'updated_at') this.updatedAt,
      @JsonKey(name: 'storage_path') this.storagePath = '',
      @JsonKey(name: 'storage_status') this.storageStatus = 'default',
      @JsonKey(name: 'live_view_path') this.liveViewPath = '',
      @JsonKey(name: 'live_view_codec') this.liveViewCodec = '',
      @JsonKey(name: 'stream_paths')
      final List<StreamPath> streamPaths = const [],
      @JsonKey(name: 'recorder_id') this.recorderId,
      @JsonKey(name: 'recorder_endpoint') this.recorderEndpoint,
      @JsonKey(name: 'directory_id') this.directoryId})
      : _streamPaths = streamPaths;

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
  @JsonKey(name: 'ai_stream_id')
  final String aiStreamId;
  @override
  @JsonKey(name: 'ai_confidence')
  final double aiConfidence;
  @override
  @JsonKey(name: 'ai_track_timeout')
  final int aiTrackTimeout;
  @override
  @JsonKey(name: 'sub_stream_url')
  final String subStreamUrl;
// Proto capability flags per lead-cloud feedback (replaces stream URLs on proto)
  @override
  @JsonKey(name: 'has_sub_stream')
  final bool hasSubStream;
  @override
  @JsonKey(name: 'has_main_stream')
  final bool hasMainStream;
  @override
  @JsonKey(name: 'retention_days')
  final int retentionDays;
  @override
  @JsonKey(name: 'event_retention_days')
  final int eventRetentionDays;
  @override
  @JsonKey(name: 'detection_retention_days')
  final int detectionRetentionDays;
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
  @JsonKey(name: 'storage_path')
  final String storagePath;
  @override
  @JsonKey(name: 'storage_status')
  final String storageStatus;
  @override
  @JsonKey(name: 'live_view_path')
  final String liveViewPath;
  @override
  @JsonKey(name: 'live_view_codec')
  final String liveViewCodec;
  final List<StreamPath> _streamPaths;
  @override
  @JsonKey(name: 'stream_paths')
  List<StreamPath> get streamPaths {
    if (_streamPaths is EqualUnmodifiableListView) return _streamPaths;
    // ignore: implicit_dynamic_type
    return EqualUnmodifiableListView(_streamPaths);
  }

// Recorder/Directory routing fields — populated by the Directory API so
// the client knows where to send data-plane requests (live view, playback,
// export) without an extra lookup hop.
  @override
  @JsonKey(name: 'recorder_id')
  final String? recorderId;
  @override
  @JsonKey(name: 'recorder_endpoint')
  final String? recorderEndpoint;
  @override
  @JsonKey(name: 'directory_id')
  final String? directoryId;

  @override
  String toString() {
    return 'Camera(id: $id, name: $name, rtspUrl: $rtspUrl, onvifEndpoint: $onvifEndpoint, mediamtxPath: $mediamtxPath, status: $status, ptzCapable: $ptzCapable, aiEnabled: $aiEnabled, aiStreamId: $aiStreamId, aiConfidence: $aiConfidence, aiTrackTimeout: $aiTrackTimeout, subStreamUrl: $subStreamUrl, hasSubStream: $hasSubStream, hasMainStream: $hasMainStream, retentionDays: $retentionDays, eventRetentionDays: $eventRetentionDays, detectionRetentionDays: $detectionRetentionDays, motionTimeoutSeconds: $motionTimeoutSeconds, snapshotUri: $snapshotUri, supportsEvents: $supportsEvents, supportsAnalytics: $supportsAnalytics, supportsRelay: $supportsRelay, createdAt: $createdAt, updatedAt: $updatedAt, storagePath: $storagePath, storageStatus: $storageStatus, liveViewPath: $liveViewPath, liveViewCodec: $liveViewCodec, streamPaths: $streamPaths, recorderId: $recorderId, recorderEndpoint: $recorderEndpoint, directoryId: $directoryId)';
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
            (identical(other.aiStreamId, aiStreamId) ||
                other.aiStreamId == aiStreamId) &&
            (identical(other.aiConfidence, aiConfidence) ||
                other.aiConfidence == aiConfidence) &&
            (identical(other.aiTrackTimeout, aiTrackTimeout) ||
                other.aiTrackTimeout == aiTrackTimeout) &&
            (identical(other.subStreamUrl, subStreamUrl) ||
                other.subStreamUrl == subStreamUrl) &&
            (identical(other.hasSubStream, hasSubStream) ||
                other.hasSubStream == hasSubStream) &&
            (identical(other.hasMainStream, hasMainStream) ||
                other.hasMainStream == hasMainStream) &&
            (identical(other.retentionDays, retentionDays) ||
                other.retentionDays == retentionDays) &&
            (identical(other.eventRetentionDays, eventRetentionDays) ||
                other.eventRetentionDays == eventRetentionDays) &&
            (identical(other.detectionRetentionDays, detectionRetentionDays) ||
                other.detectionRetentionDays == detectionRetentionDays) &&
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
                other.updatedAt == updatedAt) &&
            (identical(other.storagePath, storagePath) ||
                other.storagePath == storagePath) &&
            (identical(other.storageStatus, storageStatus) ||
                other.storageStatus == storageStatus) &&
            (identical(other.liveViewPath, liveViewPath) ||
                other.liveViewPath == liveViewPath) &&
            (identical(other.liveViewCodec, liveViewCodec) ||
                other.liveViewCodec == liveViewCodec) &&
            const DeepCollectionEquality()
                .equals(other._streamPaths, _streamPaths) &&
            (identical(other.recorderId, recorderId) ||
                other.recorderId == recorderId) &&
            (identical(other.recorderEndpoint, recorderEndpoint) ||
                other.recorderEndpoint == recorderEndpoint) &&
            (identical(other.directoryId, directoryId) ||
                other.directoryId == directoryId));
  }

  @JsonKey(includeFromJson: false, includeToJson: false)
  @override
  int get hashCode => Object.hashAll([
        runtimeType,
        id,
        name,
        rtspUrl,
        onvifEndpoint,
        mediamtxPath,
        status,
        ptzCapable,
        aiEnabled,
        aiStreamId,
        aiConfidence,
        aiTrackTimeout,
        subStreamUrl,
        hasSubStream,
        hasMainStream,
        retentionDays,
        eventRetentionDays,
        detectionRetentionDays,
        motionTimeoutSeconds,
        snapshotUri,
        supportsEvents,
        supportsAnalytics,
        supportsRelay,
        createdAt,
        updatedAt,
        storagePath,
        storageStatus,
        liveViewPath,
        liveViewCodec,
        const DeepCollectionEquality().hash(_streamPaths),
        recorderId,
        recorderEndpoint,
        directoryId
      ]);

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
      @JsonKey(name: 'ai_stream_id') final String aiStreamId,
      @JsonKey(name: 'ai_confidence') final double aiConfidence,
      @JsonKey(name: 'ai_track_timeout') final int aiTrackTimeout,
      @JsonKey(name: 'sub_stream_url') final String subStreamUrl,
      @JsonKey(name: 'has_sub_stream') final bool hasSubStream,
      @JsonKey(name: 'has_main_stream') final bool hasMainStream,
      @JsonKey(name: 'retention_days') final int retentionDays,
      @JsonKey(name: 'event_retention_days') final int eventRetentionDays,
      @JsonKey(name: 'detection_retention_days')
      final int detectionRetentionDays,
      @JsonKey(name: 'motion_timeout_seconds') final int motionTimeoutSeconds,
      @JsonKey(name: 'snapshot_uri') final String snapshotUri,
      @JsonKey(name: 'supports_events') final bool supportsEvents,
      @JsonKey(name: 'supports_analytics') final bool supportsAnalytics,
      @JsonKey(name: 'supports_relay') final bool supportsRelay,
      @JsonKey(name: 'created_at') final String? createdAt,
      @JsonKey(name: 'updated_at') final String? updatedAt,
      @JsonKey(name: 'storage_path') final String storagePath,
      @JsonKey(name: 'storage_status') final String storageStatus,
      @JsonKey(name: 'live_view_path') final String liveViewPath,
      @JsonKey(name: 'live_view_codec') final String liveViewCodec,
      @JsonKey(name: 'stream_paths') final List<StreamPath> streamPaths,
      @JsonKey(name: 'recorder_id') final String? recorderId,
      @JsonKey(name: 'recorder_endpoint') final String? recorderEndpoint,
      @JsonKey(name: 'directory_id') final String? directoryId}) = _$CameraImpl;

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
  @JsonKey(name: 'ai_stream_id')
  String get aiStreamId;
  @override
  @JsonKey(name: 'ai_confidence')
  double get aiConfidence;
  @override
  @JsonKey(name: 'ai_track_timeout')
  int get aiTrackTimeout;
  @override
  @JsonKey(name: 'sub_stream_url')
  String
      get subStreamUrl; // Proto capability flags per lead-cloud feedback (replaces stream URLs on proto)
  @override
  @JsonKey(name: 'has_sub_stream')
  bool get hasSubStream;
  @override
  @JsonKey(name: 'has_main_stream')
  bool get hasMainStream;
  @override
  @JsonKey(name: 'retention_days')
  int get retentionDays;
  @override
  @JsonKey(name: 'event_retention_days')
  int get eventRetentionDays;
  @override
  @JsonKey(name: 'detection_retention_days')
  int get detectionRetentionDays;
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
  @override
  @JsonKey(name: 'storage_path')
  String get storagePath;
  @override
  @JsonKey(name: 'storage_status')
  String get storageStatus;
  @override
  @JsonKey(name: 'live_view_path')
  String get liveViewPath;
  @override
  @JsonKey(name: 'live_view_codec')
  String get liveViewCodec;
  @override
  @JsonKey(name: 'stream_paths')
  List<StreamPath>
      get streamPaths; // Recorder/Directory routing fields — populated by the Directory API so
// the client knows where to send data-plane requests (live view, playback,
// export) without an extra lookup hop.
  @override
  @JsonKey(name: 'recorder_id')
  String? get recorderId;
  @override
  @JsonKey(name: 'recorder_endpoint')
  String? get recorderEndpoint;
  @override
  @JsonKey(name: 'directory_id')
  String? get directoryId;

  /// Create a copy of Camera
  /// with the given fields replaced by the non-null parameter values.
  @override
  @JsonKey(includeFromJson: false, includeToJson: false)
  _$$CameraImplCopyWith<_$CameraImpl> get copyWith =>
      throw _privateConstructorUsedError;
}

StreamPath _$StreamPathFromJson(Map<String, dynamic> json) {
  return _StreamPath.fromJson(json);
}

/// @nodoc
mixin _$StreamPath {
  String get name => throw _privateConstructorUsedError;
  String get path => throw _privateConstructorUsedError;
  String get resolution => throw _privateConstructorUsedError;
  @JsonKey(name: 'video_codec')
  String get videoCodec => throw _privateConstructorUsedError;

  /// Serializes this StreamPath to a JSON map.
  Map<String, dynamic> toJson() => throw _privateConstructorUsedError;

  /// Create a copy of StreamPath
  /// with the given fields replaced by the non-null parameter values.
  @JsonKey(includeFromJson: false, includeToJson: false)
  $StreamPathCopyWith<StreamPath> get copyWith =>
      throw _privateConstructorUsedError;
}

/// @nodoc
abstract class $StreamPathCopyWith<$Res> {
  factory $StreamPathCopyWith(
          StreamPath value, $Res Function(StreamPath) then) =
      _$StreamPathCopyWithImpl<$Res, StreamPath>;
  @useResult
  $Res call(
      {String name,
      String path,
      String resolution,
      @JsonKey(name: 'video_codec') String videoCodec});
}

/// @nodoc
class _$StreamPathCopyWithImpl<$Res, $Val extends StreamPath>
    implements $StreamPathCopyWith<$Res> {
  _$StreamPathCopyWithImpl(this._value, this._then);

  // ignore: unused_field
  final $Val _value;
  // ignore: unused_field
  final $Res Function($Val) _then;

  /// Create a copy of StreamPath
  /// with the given fields replaced by the non-null parameter values.
  @pragma('vm:prefer-inline')
  @override
  $Res call({
    Object? name = null,
    Object? path = null,
    Object? resolution = null,
    Object? videoCodec = null,
  }) {
    return _then(_value.copyWith(
      name: null == name
          ? _value.name
          : name // ignore: cast_nullable_to_non_nullable
              as String,
      path: null == path
          ? _value.path
          : path // ignore: cast_nullable_to_non_nullable
              as String,
      resolution: null == resolution
          ? _value.resolution
          : resolution // ignore: cast_nullable_to_non_nullable
              as String,
      videoCodec: null == videoCodec
          ? _value.videoCodec
          : videoCodec // ignore: cast_nullable_to_non_nullable
              as String,
    ) as $Val);
  }
}

/// @nodoc
abstract class _$$StreamPathImplCopyWith<$Res>
    implements $StreamPathCopyWith<$Res> {
  factory _$$StreamPathImplCopyWith(
          _$StreamPathImpl value, $Res Function(_$StreamPathImpl) then) =
      __$$StreamPathImplCopyWithImpl<$Res>;
  @override
  @useResult
  $Res call(
      {String name,
      String path,
      String resolution,
      @JsonKey(name: 'video_codec') String videoCodec});
}

/// @nodoc
class __$$StreamPathImplCopyWithImpl<$Res>
    extends _$StreamPathCopyWithImpl<$Res, _$StreamPathImpl>
    implements _$$StreamPathImplCopyWith<$Res> {
  __$$StreamPathImplCopyWithImpl(
      _$StreamPathImpl _value, $Res Function(_$StreamPathImpl) _then)
      : super(_value, _then);

  /// Create a copy of StreamPath
  /// with the given fields replaced by the non-null parameter values.
  @pragma('vm:prefer-inline')
  @override
  $Res call({
    Object? name = null,
    Object? path = null,
    Object? resolution = null,
    Object? videoCodec = null,
  }) {
    return _then(_$StreamPathImpl(
      name: null == name
          ? _value.name
          : name // ignore: cast_nullable_to_non_nullable
              as String,
      path: null == path
          ? _value.path
          : path // ignore: cast_nullable_to_non_nullable
              as String,
      resolution: null == resolution
          ? _value.resolution
          : resolution // ignore: cast_nullable_to_non_nullable
              as String,
      videoCodec: null == videoCodec
          ? _value.videoCodec
          : videoCodec // ignore: cast_nullable_to_non_nullable
              as String,
    ));
  }
}

/// @nodoc
@JsonSerializable()
class _$StreamPathImpl implements _StreamPath {
  const _$StreamPathImpl(
      {this.name = '',
      this.path = '',
      this.resolution = '',
      @JsonKey(name: 'video_codec') this.videoCodec = ''});

  factory _$StreamPathImpl.fromJson(Map<String, dynamic> json) =>
      _$$StreamPathImplFromJson(json);

  @override
  @JsonKey()
  final String name;
  @override
  @JsonKey()
  final String path;
  @override
  @JsonKey()
  final String resolution;
  @override
  @JsonKey(name: 'video_codec')
  final String videoCodec;

  @override
  String toString() {
    return 'StreamPath(name: $name, path: $path, resolution: $resolution, videoCodec: $videoCodec)';
  }

  @override
  bool operator ==(Object other) {
    return identical(this, other) ||
        (other.runtimeType == runtimeType &&
            other is _$StreamPathImpl &&
            (identical(other.name, name) || other.name == name) &&
            (identical(other.path, path) || other.path == path) &&
            (identical(other.resolution, resolution) ||
                other.resolution == resolution) &&
            (identical(other.videoCodec, videoCodec) ||
                other.videoCodec == videoCodec));
  }

  @JsonKey(includeFromJson: false, includeToJson: false)
  @override
  int get hashCode =>
      Object.hash(runtimeType, name, path, resolution, videoCodec);

  /// Create a copy of StreamPath
  /// with the given fields replaced by the non-null parameter values.
  @JsonKey(includeFromJson: false, includeToJson: false)
  @override
  @pragma('vm:prefer-inline')
  _$$StreamPathImplCopyWith<_$StreamPathImpl> get copyWith =>
      __$$StreamPathImplCopyWithImpl<_$StreamPathImpl>(this, _$identity);

  @override
  Map<String, dynamic> toJson() {
    return _$$StreamPathImplToJson(
      this,
    );
  }
}

abstract class _StreamPath implements StreamPath {
  const factory _StreamPath(
          {final String name,
          final String path,
          final String resolution,
          @JsonKey(name: 'video_codec') final String videoCodec}) =
      _$StreamPathImpl;

  factory _StreamPath.fromJson(Map<String, dynamic> json) =
      _$StreamPathImpl.fromJson;

  @override
  String get name;
  @override
  String get path;
  @override
  String get resolution;
  @override
  @JsonKey(name: 'video_codec')
  String get videoCodec;

  /// Create a copy of StreamPath
  /// with the given fields replaced by the non-null parameter values.
  @override
  @JsonKey(includeFromJson: false, includeToJson: false)
  _$$StreamPathImplCopyWith<_$StreamPathImpl> get copyWith =>
      throw _privateConstructorUsedError;
}
