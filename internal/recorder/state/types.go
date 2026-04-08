package state

import "time"

// CameraConfig is the canonical Recorder-side view of an assigned camera.
//
// This struct is serialized to JSON and stored in assigned_cameras.config.
// Keep it stable: fields are only ever added, never renamed or removed,
// so that older Recorders can still deserialize rows written by newer
// Directories. Unknown JSON keys are tolerated on read.
//
// Secrets (RTSP password) are deliberately NOT part of this struct.
// They live in the dedicated rtsp_credentials ciphertext column and are
// only materialized onto AssignedCamera at read time by the Store.
type CameraConfig struct {
	// ID is the stable camera identifier assigned by the Directory.
	ID string `json:"id"`

	// Name is a human-readable label.
	Name string `json:"name"`

	// RTSPURL is the RTSP endpoint, without embedded credentials.
	RTSPURL string `json:"rtsp_url"`

	// RTSPUsername is stored in plaintext. The password lives in the
	// encrypted rtsp_credentials column.
	RTSPUsername string `json:"rtsp_username,omitempty"`

	// ONVIFEndpoint, ONVIFProfileToken describe the ONVIF device, if any.
	ONVIFEndpoint     string `json:"onvif_endpoint,omitempty"`
	ONVIFProfileToken string `json:"onvif_profile_token,omitempty"`

	// PTZCapable reports whether the camera exposes PTZ controls.
	PTZCapable bool `json:"ptz_capable,omitempty"`

	// RetentionDays is the local retention target in days. 0 means default.
	RetentionDays int `json:"retention_days,omitempty"`

	// Tags is a free-form label set applied by the Directory.
	Tags []string `json:"tags,omitempty"`

	// Extra holds any additional Directory-side fields the Recorder does
	// not yet understand. Preserved on round-trip.
	Extra map[string]any `json:"extra,omitempty"`
}

// AssignedCamera is one row of assigned_cameras materialized in Go.
type AssignedCamera struct {
	CameraID        string
	Config          CameraConfig
	ConfigVersion   int64
	RTSPPassword    string // plaintext, decrypted lazily at read time
	AssignedAt      time.Time
	UpdatedAt       time.Time
	LastStatePushAt *time.Time // nil if never pushed
}

// Segment is one row of segment_index materialized in Go.
type Segment struct {
	CameraID               string
	StartTS                time.Time
	EndTS                  time.Time
	Path                   string
	SizeBytes              int64
	UploadedToCloudArchive bool
}

// ReconcileDiff is the result of Store.ReconcileAssignments.
//
// Added     — camera_ids present in the snapshot but not in the cache
// Updated   — camera_ids present in both, where the incoming config differs
//
//	from what the cache currently holds (by ConfigVersion, or by
//	deep equality if versions are zero / equal)
//
// Removed   — camera_ids present in the cache but not in the snapshot
// Unchanged — camera_ids present in both and byte-identical
type ReconcileDiff struct {
	Added     []string
	Updated   []string
	Removed   []string
	Unchanged []string
}
