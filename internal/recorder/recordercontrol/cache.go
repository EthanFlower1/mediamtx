package recordercontrol

import "context"

// CameraStore is the persistence seam between the reconcile loop and the
// local SQLite cache (KAI-250, internal/recorder/state/).
//
// Implementations MUST be safe for concurrent use from multiple goroutines.
// The canonical implementation wraps state.Store.
type CameraStore interface {
	// ReplaceAll atomically replaces the entire assigned-camera set with
	// the supplied list. Used when a Snapshot is received.
	ReplaceAll(ctx context.Context, cameras []Camera) error

	// Add upserts a single camera. Used for CameraAdded and CameraUpdated
	// events; idempotent on repeated apply.
	Add(ctx context.Context, c Camera) error

	// Update upserts a single camera with a new config. Same semantics as
	// Add; kept separate for metric / audit separation.
	Update(ctx context.Context, c Camera) error

	// Remove deletes a single camera from the cache.
	// A no-op (nil error) if the camera is not present.
	Remove(ctx context.Context, cameraID string) error

	// List returns all currently-cached cameras, sorted by ID.
	List(ctx context.Context) ([]Camera, error)
}
