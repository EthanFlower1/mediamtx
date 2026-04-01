// coverage:ignore-file
// GENERATED CODE - DO NOT MODIFY BY HAND
// ignore_for_file: type=lint
// ignore_for_file: unused_element, deprecated_member_use, deprecated_member_use_from_same_package, use_function_type_syntax_for_parameters, unnecessary_const, avoid_init_to_null, invalid_override_different_default_values_named, prefer_expression_function_bodies, annotate_overrides, invalid_annotation_target, unnecessary_question_mark

part of 'camera_group.dart';

// **************************************************************************
// FreezedGenerator
// **************************************************************************

T _$identity<T>(T value) => value;

final _privateConstructorUsedError = UnsupportedError(
    'It seems like you constructed your class using `MyClass._()`. This constructor is only meant to be used by freezed and you are not supposed to need it nor use it.\nPlease check the documentation here for more information: https://github.com/rrousselGit/freezed#adding-getters-and-methods-to-our-models');

CameraGroup _$CameraGroupFromJson(Map<String, dynamic> json) {
  return _CameraGroup.fromJson(json);
}

/// @nodoc
mixin _$CameraGroup {
  String get id => throw _privateConstructorUsedError;
  String get name => throw _privateConstructorUsedError;
  @JsonKey(name: 'camera_ids')
  List<String> get cameraIds => throw _privateConstructorUsedError;
  @JsonKey(name: 'created_at')
  String? get createdAt => throw _privateConstructorUsedError;
  @JsonKey(name: 'updated_at')
  String? get updatedAt => throw _privateConstructorUsedError;

  /// Serializes this CameraGroup to a JSON map.
  Map<String, dynamic> toJson() => throw _privateConstructorUsedError;

  /// Create a copy of CameraGroup
  /// with the given fields replaced by the non-null parameter values.
  @JsonKey(includeFromJson: false, includeToJson: false)
  $CameraGroupCopyWith<CameraGroup> get copyWith =>
      throw _privateConstructorUsedError;
}

/// @nodoc
abstract class $CameraGroupCopyWith<$Res> {
  factory $CameraGroupCopyWith(
          CameraGroup value, $Res Function(CameraGroup) then) =
      _$CameraGroupCopyWithImpl<$Res, CameraGroup>;
  @useResult
  $Res call(
      {String id,
      String name,
      @JsonKey(name: 'camera_ids') List<String> cameraIds,
      @JsonKey(name: 'created_at') String? createdAt,
      @JsonKey(name: 'updated_at') String? updatedAt});
}

/// @nodoc
class _$CameraGroupCopyWithImpl<$Res, $Val extends CameraGroup>
    implements $CameraGroupCopyWith<$Res> {
  _$CameraGroupCopyWithImpl(this._value, this._then);

  // ignore: unused_field
  final $Val _value;
  // ignore: unused_field
  final $Res Function($Val) _then;

  /// Create a copy of CameraGroup
  /// with the given fields replaced by the non-null parameter values.
  @pragma('vm:prefer-inline')
  @override
  $Res call({
    Object? id = null,
    Object? name = null,
    Object? cameraIds = null,
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
      cameraIds: null == cameraIds
          ? _value.cameraIds
          : cameraIds // ignore: cast_nullable_to_non_nullable
              as List<String>,
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
abstract class _$$CameraGroupImplCopyWith<$Res>
    implements $CameraGroupCopyWith<$Res> {
  factory _$$CameraGroupImplCopyWith(
          _$CameraGroupImpl value, $Res Function(_$CameraGroupImpl) then) =
      __$$CameraGroupImplCopyWithImpl<$Res>;
  @override
  @useResult
  $Res call(
      {String id,
      String name,
      @JsonKey(name: 'camera_ids') List<String> cameraIds,
      @JsonKey(name: 'created_at') String? createdAt,
      @JsonKey(name: 'updated_at') String? updatedAt});
}

/// @nodoc
class __$$CameraGroupImplCopyWithImpl<$Res>
    extends _$CameraGroupCopyWithImpl<$Res, _$CameraGroupImpl>
    implements _$$CameraGroupImplCopyWith<$Res> {
  __$$CameraGroupImplCopyWithImpl(
      _$CameraGroupImpl _value, $Res Function(_$CameraGroupImpl) _then)
      : super(_value, _then);

  /// Create a copy of CameraGroup
  /// with the given fields replaced by the non-null parameter values.
  @pragma('vm:prefer-inline')
  @override
  $Res call({
    Object? id = null,
    Object? name = null,
    Object? cameraIds = null,
    Object? createdAt = freezed,
    Object? updatedAt = freezed,
  }) {
    return _then(_$CameraGroupImpl(
      id: null == id
          ? _value.id
          : id // ignore: cast_nullable_to_non_nullable
              as String,
      name: null == name
          ? _value.name
          : name // ignore: cast_nullable_to_non_nullable
              as String,
      cameraIds: null == cameraIds
          ? _value._cameraIds
          : cameraIds // ignore: cast_nullable_to_non_nullable
              as List<String>,
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
class _$CameraGroupImpl implements _CameraGroup {
  const _$CameraGroupImpl(
      {required this.id,
      required this.name,
      @JsonKey(name: 'camera_ids') final List<String> cameraIds = const [],
      @JsonKey(name: 'created_at') required this.createdAt,
      @JsonKey(name: 'updated_at') required this.updatedAt})
      : _cameraIds = cameraIds;

  factory _$CameraGroupImpl.fromJson(Map<String, dynamic> json) =>
      _$$CameraGroupImplFromJson(json);

  @override
  final String id;
  @override
  final String name;
  final List<String> _cameraIds;
  @override
  @JsonKey(name: 'camera_ids')
  List<String> get cameraIds {
    if (_cameraIds is EqualUnmodifiableListView) return _cameraIds;
    // ignore: implicit_dynamic_type
    return EqualUnmodifiableListView(_cameraIds);
  }

  @override
  @JsonKey(name: 'created_at')
  final String? createdAt;
  @override
  @JsonKey(name: 'updated_at')
  final String? updatedAt;

  @override
  String toString() {
    return 'CameraGroup(id: $id, name: $name, cameraIds: $cameraIds, createdAt: $createdAt, updatedAt: $updatedAt)';
  }

  @override
  bool operator ==(Object other) {
    return identical(this, other) ||
        (other.runtimeType == runtimeType &&
            other is _$CameraGroupImpl &&
            (identical(other.id, id) || other.id == id) &&
            (identical(other.name, name) || other.name == name) &&
            const DeepCollectionEquality()
                .equals(other._cameraIds, _cameraIds) &&
            (identical(other.createdAt, createdAt) ||
                other.createdAt == createdAt) &&
            (identical(other.updatedAt, updatedAt) ||
                other.updatedAt == updatedAt));
  }

  @JsonKey(includeFromJson: false, includeToJson: false)
  @override
  int get hashCode => Object.hash(runtimeType, id, name,
      const DeepCollectionEquality().hash(_cameraIds), createdAt, updatedAt);

  /// Create a copy of CameraGroup
  /// with the given fields replaced by the non-null parameter values.
  @JsonKey(includeFromJson: false, includeToJson: false)
  @override
  @pragma('vm:prefer-inline')
  _$$CameraGroupImplCopyWith<_$CameraGroupImpl> get copyWith =>
      __$$CameraGroupImplCopyWithImpl<_$CameraGroupImpl>(this, _$identity);

  @override
  Map<String, dynamic> toJson() {
    return _$$CameraGroupImplToJson(
      this,
    );
  }
}

abstract class _CameraGroup implements CameraGroup {
  const factory _CameraGroup(
          {required final String id,
          required final String name,
          @JsonKey(name: 'camera_ids') final List<String> cameraIds,
          @JsonKey(name: 'created_at') required final String? createdAt,
          @JsonKey(name: 'updated_at') required final String? updatedAt}) =
      _$CameraGroupImpl;

  factory _CameraGroup.fromJson(Map<String, dynamic> json) =
      _$CameraGroupImpl.fromJson;

  @override
  String get id;
  @override
  String get name;
  @override
  @JsonKey(name: 'camera_ids')
  List<String> get cameraIds;
  @override
  @JsonKey(name: 'created_at')
  String? get createdAt;
  @override
  @JsonKey(name: 'updated_at')
  String? get updatedAt;

  /// Create a copy of CameraGroup
  /// with the given fields replaced by the non-null parameter values.
  @override
  @JsonKey(includeFromJson: false, includeToJson: false)
  _$$CameraGroupImplCopyWith<_$CameraGroupImpl> get copyWith =>
      throw _privateConstructorUsedError;
}
