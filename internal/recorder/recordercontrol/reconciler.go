package recordercontrol

import (
	"context"
	"log/slog"
)

// CaptureManager is the control plane for capture loops managed by the
// Recorder. Implementations are expected to be idempotent: calling
// EnsureRunning on an already-running camera is a no-op, and calling Stop
// on an already-stopped camera is a no-op.
//
// The recording-never-stops invariant (seam #6) applies: EnsureRunning
// failures are logged and metered but MUST NOT cause Stop to be called on
// any other camera.
type CaptureManager interface {
	// EnsureRunning starts the capture loop for camera c if it is not
	// already running, or restarts it if its config_version has changed.
	// Must not block indefinitely; any long setup should be async.
	EnsureRunning(camera Camera) error

	// Stop tears down the capture loop for the given camera. Called only
	// when the camera is removed or the Recorder is shutting down.
	Stop(cameraID string) error

	// RunningCameras returns the IDs of all currently-running capture loops.
	RunningCameras() []string
}

// reconcileResult codes used for the reconcile_runs_total metric.
const (
	reconcileResultOK      = "ok"
	reconcileResultPartial = "partial"
	reconcileResultError   = "error"
)

// reconciler applies a diff between the authoritative camera list and the
// running capture loops, implementing the recording-never-stops invariant.
type reconciler struct {
	cap    CaptureManager
	store  CameraStore
	log    *slog.Logger
	metrics *clientMetrics
}

// reconcileSnapshot applies the full authoritative camera list:
//  1. Persist it atomically to the store.
//  2. Diff it against running loops.
//  3. Start missing, stop extra, restart on version change.
//
// If starting a camera fails the error is logged and the loop continues —
// no in-progress captures are interrupted.
func (r *reconciler) reconcileSnapshot(ctx context.Context, cameras []Camera) string {
	if err := r.store.ReplaceAll(ctx, cameras); err != nil {
		r.log.ErrorContext(ctx, "recordercontrol: snapshot store.ReplaceAll failed",
			slog.String("error", err.Error()))
		return reconcileResultError
	}
	return r.applyDiff(ctx, cameras)
}

// reconcileAdd upserts a camera into the store and ensures its capture loop
// is running.
func (r *reconciler) reconcileAdd(ctx context.Context, cam Camera) {
	if err := r.store.Add(ctx, cam); err != nil {
		r.log.WarnContext(ctx, "recordercontrol: store.Add failed",
			slog.String("camera_id", cam.ID),
			slog.String("error", err.Error()))
		// fall through — still try to ensure capture is running
	}
	if err := r.cap.EnsureRunning(cam); err != nil {
		r.log.WarnContext(ctx, "recordercontrol: EnsureRunning failed on add",
			slog.String("camera_id", cam.ID),
			slog.String("error", err.Error()))
		// recording-never-stops: do NOT stop other captures
	}
}

// reconcileUpdate updates the store and restarts the capture loop for the
// camera if the config has changed.
func (r *reconciler) reconcileUpdate(ctx context.Context, cam Camera) {
	if err := r.store.Update(ctx, cam); err != nil {
		r.log.WarnContext(ctx, "recordercontrol: store.Update failed",
			slog.String("camera_id", cam.ID),
			slog.String("error", err.Error()))
	}
	if err := r.cap.EnsureRunning(cam); err != nil {
		r.log.WarnContext(ctx, "recordercontrol: EnsureRunning failed on update",
			slog.String("camera_id", cam.ID),
			slog.String("error", err.Error()))
	}
}

// reconcileRemove removes a camera from the store and stops its capture loop.
func (r *reconciler) reconcileRemove(ctx context.Context, cameraID string) {
	if err := r.store.Remove(ctx, cameraID); err != nil {
		r.log.WarnContext(ctx, "recordercontrol: store.Remove failed",
			slog.String("camera_id", cameraID),
			slog.String("error", err.Error()))
	}
	if err := r.cap.Stop(cameraID); err != nil {
		r.log.WarnContext(ctx, "recordercontrol: Stop failed",
			slog.String("camera_id", cameraID),
			slog.String("error", err.Error()))
	}
}

// applyDiff is the core idempotent diff: given the authoritative camera set,
// start missing captures, stop extra ones, restart changed ones.
// Returns a reconcileResult string for metrics.
func (r *reconciler) applyDiff(ctx context.Context, cameras []Camera) string {
	// Build index of assigned cameras by ID.
	assigned := make(map[string]Camera, len(cameras))
	for _, c := range cameras {
		assigned[c.ID] = c
	}

	// Index of currently-running loops.
	running := make(map[string]struct{})
	for _, id := range r.cap.RunningCameras() {
		running[id] = struct{}{}
	}

	hadErrors := false

	// Start missing or restart changed.
	for id, cam := range assigned {
		if err := r.cap.EnsureRunning(cam); err != nil {
			r.log.WarnContext(ctx, "recordercontrol: EnsureRunning failed in reconcile",
				slog.String("camera_id", id),
				slog.String("error", err.Error()))
			hadErrors = true
			// recording-never-stops: do NOT stop other captures
		}
	}

	// Stop extra captures (cameras no longer in the assigned set).
	for id := range running {
		if _, ok := assigned[id]; ok {
			continue
		}
		if err := r.cap.Stop(id); err != nil {
			r.log.WarnContext(ctx, "recordercontrol: Stop failed in reconcile",
				slog.String("camera_id", id),
				slog.String("error", err.Error()))
			hadErrors = true
		}
	}

	if hadErrors {
		return reconcileResultPartial
	}
	return reconcileResultOK
}
