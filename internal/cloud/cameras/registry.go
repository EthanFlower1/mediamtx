package cameras

import (
	"context"
	"time"
)

// -----------------------------------------------------------------------
// CameraRegistry interface
// -----------------------------------------------------------------------

// CameraRegistry is the read/write interface for tenant-scoped camera records.
// All methods require a non-empty tenantID; implementations MUST include
// tenantID in every SQL predicate. Violating this constraint is the canonical
// multi-tenant isolation bug (KAI-235 chaos test guards against it).
type CameraRegistry interface {
	// List returns all cameras for the tenant, ordered by created_at ASC.
	List(ctx context.Context, tenantID string) ([]Camera, error)

	// Get returns the camera with id scoped to tenantID.
	// Returns ErrNotFound if no row exists in that tenant.
	Get(ctx context.Context, tenantID, id string) (Camera, error)

	// Create inserts a new camera record. The caller must supply a UUID for id.
	// RTSPCredentialsEncrypted must be a cryptostore v1 blob or nil.
	Create(ctx context.Context, cam Camera) error

	// Update replaces mutable fields on an existing camera. tenantID is
	// derived from cam.TenantID and used in the WHERE predicate — callers
	// cannot update cameras across tenant boundaries.
	Update(ctx context.Context, cam Camera) error

	// Delete removes the camera. Returns ErrNotFound if the camera does not
	// exist in the tenant. ON DELETE CASCADE from the cameras table propagates
	// to camera_segment_index rows.
	Delete(ctx context.Context, tenantID, id string) error

	// ListByRecorder returns cameras assigned to a specific recorder within
	// the tenant. This is the seam resolved by TODO(KAI-249) in
	// internal/cloud/recordercontrol/server.go.
	ListByRecorder(ctx context.Context, tenantID, recorderID string) ([]Camera, error)

	// GetCamera satisfies the streams.CameraRegistry interface resolved by
	// TODO(KAI-249) in internal/cloud/streams/service.go.
	GetCamera(ctx context.Context, tenantID, cameraID string) (Camera, error)
}

// -----------------------------------------------------------------------
// RecorderRegistry interface
// -----------------------------------------------------------------------

// RecorderRegistry is the read/write interface for tenant-scoped recorder records.
type RecorderRegistry interface {
	// List returns all recorders for the tenant, ordered by created_at ASC.
	List(ctx context.Context, tenantID string) ([]Recorder, error)

	// Get returns the recorder scoped to tenantID.
	Get(ctx context.Context, tenantID, id string) (Recorder, error)

	// Create inserts a new recorder record.
	Create(ctx context.Context, rec Recorder) error

	// UpdateStatus updates only the operational status + last_checkin_at fields.
	// Used by the recordercontrol handler's RecorderStore seam.
	UpdateStatus(ctx context.Context, tenantID, id, status string) error

	// GetTenantID returns the tenantID that owns the recorder. Used by
	// recordercontrol.Handler to verify mTLS identity without loading the full row.
	GetTenantID(ctx context.Context, recorderID string) (string, error)

	// Delete removes the recorder. CASCADE on the cameras table sets
	// assigned_recorder_id = NULL for cameras that were assigned to it.
	Delete(ctx context.Context, tenantID, id string) error
}

// -----------------------------------------------------------------------
// SegmentIndex interface
// -----------------------------------------------------------------------

// SegmentIndex is the write/query interface for camera_segment_index.
type SegmentIndex interface {
	// Append inserts a new segment record. Idempotent: duplicate primary key
	// (camera_id, start_ts) is silently ignored via ON CONFLICT DO NOTHING.
	Append(ctx context.Context, seg Segment) error

	// QueryByTimeRange returns segments for a camera where
	// start_ts >= from AND end_ts <= to, ordered by start_ts ASC.
	// tenantID is included in the predicate for defense-in-depth cross-tenant
	// isolation even though camera_id is globally unique.
	QueryByTimeRange(ctx context.Context, tenantID, cameraID string, from, to time.Time) ([]Segment, error)
}
