/** Camera lifecycle state. */
export enum CameraState {
  Unspecified = "CAMERA_STATE_UNSPECIFIED",
  Provisioning = "CAMERA_STATE_PROVISIONING",
  Online = "CAMERA_STATE_ONLINE",
  Offline = "CAMERA_STATE_OFFLINE",
  Disabled = "CAMERA_STATE_DISABLED",
  Error = "CAMERA_STATE_ERROR",
}

/** Recording mode. */
export enum RecordingMode {
  Unspecified = "RECORDING_MODE_UNSPECIFIED",
  Continuous = "RECORDING_MODE_CONTINUOUS",
  Motion = "RECORDING_MODE_MOTION",
  Schedule = "RECORDING_MODE_SCHEDULE",
  Event = "RECORDING_MODE_EVENT",
  Off = "RECORDING_MODE_OFF",
}

/** AI/motion event kind. */
export enum EventKind {
  Unspecified = "EVENT_KIND_UNSPECIFIED",
  Motion = "EVENT_KIND_MOTION",
  Person = "EVENT_KIND_PERSON",
  Vehicle = "EVENT_KIND_VEHICLE",
  Face = "EVENT_KIND_FACE",
  LicensePlate = "EVENT_KIND_LICENSE_PLATE",
  AudioAlarm = "EVENT_KIND_AUDIO_ALARM",
  LineCrossing = "EVENT_KIND_LINE_CROSSING",
  Loitering = "EVENT_KIND_LOITERING",
  Tamper = "EVENT_KIND_TAMPER",
  Custom = "EVENT_KIND_CUSTOM",
}

/** Integration type. */
export enum IntegrationKind {
  Unspecified = "INTEGRATION_KIND_UNSPECIFIED",
  Webhook = "INTEGRATION_KIND_WEBHOOK",
  MQTT = "INTEGRATION_KIND_MQTT",
  Syslog = "INTEGRATION_KIND_SYSLOG",
  Custom = "INTEGRATION_KIND_CUSTOM",
}

export interface StreamProfile {
  name: string;
  codec: string;
  width?: number;
  height?: number;
  bitrate_kbps?: number;
  framerate?: number;
}

export interface Camera {
  id: string;
  name: string;
  description?: string;
  manufacturer?: string;
  model?: string;
  firmware_version?: string;
  mac_address?: string;
  ip_address?: string;
  state: CameraState;
  recording_mode: RecordingMode;
  profiles?: StreamProfile[];
  labels?: string[];
  recorder_id?: string;
  state_reported_at?: string;
  created_at?: string;
  updated_at?: string;
  audio_enabled?: boolean;
  motion_sensitivity?: number;
}

export interface User {
  id: string;
  username: string;
  email?: string;
  display_name?: string;
  groups?: string[];
  disabled?: boolean;
  created_at?: string;
  updated_at?: string;
  last_login_at?: string;
}

export interface Recording {
  id: string;
  camera_id: string;
  recorder_id?: string;
  start_time?: string;
  end_time?: string;
  size_bytes?: number;
  codec?: string;
  has_audio?: boolean;
  is_event_clip?: boolean;
  storage_tier?: string;
}

export interface BoundingBox {
  x: number;
  y: number;
  width: number;
  height: number;
}

export interface Event {
  id: string;
  camera_id: string;
  kind: EventKind;
  kind_label?: string;
  observed_at?: string;
  confidence?: number;
  bbox?: BoundingBox;
  track_id?: string;
  thumbnail_url?: string;
  attributes?: Record<string, string>;
}

export interface ScheduleEntry {
  day_of_week: number;
  start_minute: number;
  end_minute: number;
  mode: RecordingMode;
}

export interface Schedule {
  id: string;
  camera_id: string;
  name?: string;
  timezone?: string;
  entries?: ScheduleEntry[];
  created_at?: string;
  updated_at?: string;
}

export interface RetentionPolicy {
  id: string;
  name?: string;
  description?: string;
  retention_days?: number;
  max_bytes?: number;
  event_retention_days?: number;
  camera_ids?: string[];
  created_at?: string;
  updated_at?: string;
}

export interface Integration {
  id: string;
  name?: string;
  kind: IntegrationKind;
  enabled?: boolean;
  config?: Record<string, string>;
  subscribed_events?: EventKind[];
  camera_ids?: string[];
  created_at?: string;
  updated_at?: string;
}

/** Generic paginated response wrapper. */
export interface PaginatedResponse<T> {
  data: T[];
  next_cursor?: string;
  total_count?: number;
}
