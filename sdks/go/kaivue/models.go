package kaivue

import "time"

// CameraState tracks the lifecycle of a camera.
type CameraState string

const (
	CameraStateUnspecified  CameraState = "CAMERA_STATE_UNSPECIFIED"
	CameraStateProvisioning CameraState = "CAMERA_STATE_PROVISIONING"
	CameraStateOnline       CameraState = "CAMERA_STATE_ONLINE"
	CameraStateOffline      CameraState = "CAMERA_STATE_OFFLINE"
	CameraStateDisabled     CameraState = "CAMERA_STATE_DISABLED"
	CameraStateError        CameraState = "CAMERA_STATE_ERROR"
)

// RecordingMode controls when the recorder writes segments.
type RecordingMode string

const (
	RecordingModeUnspecified RecordingMode = "RECORDING_MODE_UNSPECIFIED"
	RecordingModeContinuous  RecordingMode = "RECORDING_MODE_CONTINUOUS"
	RecordingModeMotion      RecordingMode = "RECORDING_MODE_MOTION"
	RecordingModeSchedule    RecordingMode = "RECORDING_MODE_SCHEDULE"
	RecordingModeEvent       RecordingMode = "RECORDING_MODE_EVENT"
	RecordingModeOff         RecordingMode = "RECORDING_MODE_OFF"
)

// EventKind is the detection type.
type EventKind string

const (
	EventKindUnspecified  EventKind = "EVENT_KIND_UNSPECIFIED"
	EventKindMotion       EventKind = "EVENT_KIND_MOTION"
	EventKindPerson       EventKind = "EVENT_KIND_PERSON"
	EventKindVehicle      EventKind = "EVENT_KIND_VEHICLE"
	EventKindFace         EventKind = "EVENT_KIND_FACE"
	EventKindLicensePlate EventKind = "EVENT_KIND_LICENSE_PLATE"
	EventKindAudioAlarm   EventKind = "EVENT_KIND_AUDIO_ALARM"
	EventKindLineCrossing EventKind = "EVENT_KIND_LINE_CROSSING"
	EventKindLoitering    EventKind = "EVENT_KIND_LOITERING"
	EventKindTamper       EventKind = "EVENT_KIND_TAMPER"
	EventKindCustom       EventKind = "EVENT_KIND_CUSTOM"
)

// IntegrationKind is the type of integration.
type IntegrationKind string

const (
	IntegrationKindUnspecified IntegrationKind = "INTEGRATION_KIND_UNSPECIFIED"
	IntegrationKindWebhook    IntegrationKind = "INTEGRATION_KIND_WEBHOOK"
	IntegrationKindMQTT       IntegrationKind = "INTEGRATION_KIND_MQTT"
	IntegrationKindSyslog     IntegrationKind = "INTEGRATION_KIND_SYSLOG"
	IntegrationKindCustom     IntegrationKind = "INTEGRATION_KIND_CUSTOM"
)

// StreamProfile describes a single video stream profile.
type StreamProfile struct {
	Name        string `json:"name"`
	Codec       string `json:"codec"`
	Width       int    `json:"width,omitempty"`
	Height      int    `json:"height,omitempty"`
	BitrateKbps int    `json:"bitrate_kbps,omitempty"`
	Framerate   int    `json:"framerate,omitempty"`
}

// Camera represents a camera resource.
type Camera struct {
	ID               string          `json:"id"`
	Name             string          `json:"name"`
	Description      string          `json:"description,omitempty"`
	Manufacturer     string          `json:"manufacturer,omitempty"`
	Model            string          `json:"model,omitempty"`
	FirmwareVersion  string          `json:"firmware_version,omitempty"`
	MACAddress       string          `json:"mac_address,omitempty"`
	IPAddress        string          `json:"ip_address,omitempty"`
	State            CameraState     `json:"state"`
	RecordingMode    RecordingMode   `json:"recording_mode"`
	Profiles         []StreamProfile `json:"profiles,omitempty"`
	Labels           []string        `json:"labels,omitempty"`
	RecorderID       string          `json:"recorder_id,omitempty"`
	StateReportedAt  *time.Time      `json:"state_reported_at,omitempty"`
	CreatedAt        *time.Time      `json:"created_at,omitempty"`
	UpdatedAt        *time.Time      `json:"updated_at,omitempty"`
	AudioEnabled     bool            `json:"audio_enabled,omitempty"`
	MotionSensitivity int           `json:"motion_sensitivity,omitempty"`
}

// User represents a user resource.
type User struct {
	ID          string     `json:"id"`
	Username    string     `json:"username"`
	Email       string     `json:"email,omitempty"`
	DisplayName string     `json:"display_name,omitempty"`
	Groups      []string   `json:"groups,omitempty"`
	Disabled    bool       `json:"disabled,omitempty"`
	CreatedAt   *time.Time `json:"created_at,omitempty"`
	UpdatedAt   *time.Time `json:"updated_at,omitempty"`
	LastLoginAt *time.Time `json:"last_login_at,omitempty"`
}

// Recording represents a recorded media segment.
type Recording struct {
	ID          string     `json:"id"`
	CameraID    string     `json:"camera_id"`
	RecorderID  string     `json:"recorder_id,omitempty"`
	StartTime   *time.Time `json:"start_time,omitempty"`
	EndTime     *time.Time `json:"end_time,omitempty"`
	SizeBytes   int64      `json:"size_bytes,omitempty"`
	Codec       string     `json:"codec,omitempty"`
	HasAudio    bool       `json:"has_audio,omitempty"`
	IsEventClip bool       `json:"is_event_clip,omitempty"`
	StorageTier string     `json:"storage_tier,omitempty"`
}

// BoundingBox is a normalized 0..1 rectangle.
type BoundingBox struct {
	X      float32 `json:"x"`
	Y      float32 `json:"y"`
	Width  float32 `json:"width"`
	Height float32 `json:"height"`
}

// Event represents an AI/motion detection event.
type Event struct {
	ID           string            `json:"id"`
	CameraID     string            `json:"camera_id"`
	Kind         EventKind         `json:"kind"`
	KindLabel    string            `json:"kind_label,omitempty"`
	ObservedAt   *time.Time        `json:"observed_at,omitempty"`
	Confidence   float32           `json:"confidence,omitempty"`
	BBox         *BoundingBox      `json:"bbox,omitempty"`
	TrackID      string            `json:"track_id,omitempty"`
	ThumbnailURL string            `json:"thumbnail_url,omitempty"`
	Attributes   map[string]string `json:"attributes,omitempty"`
}

// ScheduleEntry describes one rule in a weekly schedule.
type ScheduleEntry struct {
	DayOfWeek   int           `json:"day_of_week"`
	StartMinute int           `json:"start_minute"`
	EndMinute   int           `json:"end_minute"`
	Mode        RecordingMode `json:"mode"`
}

// Schedule represents a recording schedule.
type Schedule struct {
	ID        string          `json:"id"`
	CameraID  string          `json:"camera_id"`
	Name      string          `json:"name,omitempty"`
	Timezone  string          `json:"timezone,omitempty"`
	Entries   []ScheduleEntry `json:"entries,omitempty"`
	CreatedAt *time.Time      `json:"created_at,omitempty"`
	UpdatedAt *time.Time      `json:"updated_at,omitempty"`
}

// RetentionPolicy defines how long recordings are kept.
type RetentionPolicy struct {
	ID                 string     `json:"id"`
	Name               string     `json:"name,omitempty"`
	Description        string     `json:"description,omitempty"`
	RetentionDays      int        `json:"retention_days,omitempty"`
	MaxBytes           int64      `json:"max_bytes,omitempty"`
	EventRetentionDays int        `json:"event_retention_days,omitempty"`
	CameraIDs          []string   `json:"camera_ids,omitempty"`
	CreatedAt          *time.Time `json:"created_at,omitempty"`
	UpdatedAt          *time.Time `json:"updated_at,omitempty"`
}

// Integration represents a webhook/MQTT/syslog integration.
type Integration struct {
	ID               string            `json:"id"`
	Name             string            `json:"name,omitempty"`
	Kind             IntegrationKind   `json:"kind"`
	Enabled          bool              `json:"enabled,omitempty"`
	Config           map[string]string `json:"config,omitempty"`
	SubscribedEvents []EventKind       `json:"subscribed_events,omitempty"`
	CameraIDs        []string          `json:"camera_ids,omitempty"`
	CreatedAt        *time.Time        `json:"created_at,omitempty"`
	UpdatedAt        *time.Time        `json:"updated_at,omitempty"`
}
