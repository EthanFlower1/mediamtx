package recordercontrol

import "time"

// Camera is the Recorder-local view of an assigned camera.
// It is a superset of state.AssignedCamera fields that are relevant to
// the capture manager — the store wires the two together.
type Camera struct {
	ID            string
	TenantID      string
	RecorderID    string
	Name          string
	CredentialRef string
	ConfigJSON    string
	ConfigVersion int64
}

// wireEvent is the JSON shape sent by the Directory server over the
// NDJSON long-poll body. Must stay in sync with
// internal/cloud/recordercontrol.wireEvent.
//
// When Connect-Go (KAI-310) lands this hand-rolled type is replaced by
// the generated proto; the JSON field names are intentionally identical
// to the proto snake_case names to make the migration mechanical.
type wireEvent struct {
	Kind      string           `json:"kind"`
	Version   int64            `json:"version"`
	EmittedAt string           `json:"emitted_at"`
	Snapshot  *wireSnapshot    `json:"snapshot,omitempty"`
	Added     *wireCameraAdded `json:"camera_added,omitempty"`
	Updated   *wireCameraUpdated `json:"camera_updated,omitempty"`
	Removed   *wireCameraRemoved `json:"camera_removed,omitempty"`
}

type wireSnapshot struct {
	Cameras []wireCamera `json:"cameras"`
}

type wireCamera struct {
	ID            string `json:"id"`
	TenantID      string `json:"tenant_id"`
	RecorderID    string `json:"recorder_id"`
	Name          string `json:"name"`
	CredentialRef string `json:"credential_ref"`
	ConfigJSON    string `json:"config_json"`
	ConfigVersion int64  `json:"config_version"`
}

type wireCameraAdded struct {
	Camera wireCamera `json:"camera"`
}

type wireCameraUpdated struct {
	Camera wireCamera `json:"camera"`
}

type wireCameraRemoved struct {
	CameraID        string `json:"camera_id"`
	PurgeRecordings bool   `json:"purge_recordings"`
	Reason          string `json:"reason,omitempty"`
}

// eventKind constants mirror the server-side assignmentEventKind consts.
const (
	kindSnapshot      = "snapshot"
	kindCameraAdded   = "camera_added"
	kindCameraUpdated = "camera_updated"
	kindCameraRemoved = "camera_removed"
	kindHeartbeat     = "heartbeat"
)

func wireCameraToCamera(wc wireCamera) Camera {
	return Camera{
		ID:            wc.ID,
		TenantID:      wc.TenantID,
		RecorderID:    wc.RecorderID,
		Name:          wc.Name,
		CredentialRef: wc.CredentialRef,
		ConfigJSON:    wc.ConfigJSON,
		ConfigVersion: wc.ConfigVersion,
	}
}

// backoffState tracks reconnect back-off state.
type backoffState struct {
	base    time.Duration
	max     time.Duration
	current time.Duration
	// nowFn is injectable for tests; defaults to time.Now.
	nowFn func() time.Time
}

func newBackoff(base, max time.Duration) backoffState {
	return backoffState{base: base, max: max, current: base}
}

func (b *backoffState) next() time.Duration {
	d := b.current
	b.current *= 2
	if b.current > b.max {
		b.current = b.max
	}
	return d
}

func (b *backoffState) reset() {
	b.current = b.base
}
