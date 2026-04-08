package cameras

import (
	"context"
	"errors"

	"github.com/bluenviron/mediamtx/internal/cloud/recordercontrol"
	"github.com/bluenviron/mediamtx/internal/cloud/streams"
)

// -----------------------------------------------------------------------
// CameraStoreBridge — adapts CameraRegistry to recordercontrol.CameraStore
// -----------------------------------------------------------------------

// CameraStoreBridge adapts CameraRegistry to the recordercontrol.CameraStore
// interface. This resolves TODO(KAI-249) in
// internal/cloud/recordercontrol/server.go.
//
// Usage in apiserver wiring:
//
//	cameraReg := cameras.NewCameraRegistry(db)
//	cameraStore := cameras.NewCameraStoreBridge(cameraReg)
//	handler, _ := recordercontrol.NewHandler(recordercontrol.Config{
//	    Cameras: cameraStore,
//	    ...
//	})
type CameraStoreBridge struct {
	reg CameraRegistry
}

// NewCameraStoreBridge constructs a CameraStoreBridge that satisfies
// recordercontrol.CameraStore using the real camera registry.
func NewCameraStoreBridge(reg CameraRegistry) *CameraStoreBridge {
	return &CameraStoreBridge{reg: reg}
}

// ListCamerasForRecorder implements recordercontrol.CameraStore.
// Multi-tenant invariant: tenantID is forwarded into every SQL predicate by
// the underlying CameraRegistry — cross-tenant cameras are impossible to return.
func (b *CameraStoreBridge) ListCamerasForRecorder(
	ctx context.Context,
	tenantID, recorderID string,
) ([]recordercontrol.CameraPayload, error) {
	cams, err := b.reg.ListByRecorder(ctx, tenantID, recorderID)
	if err != nil {
		return nil, err
	}

	out := make([]recordercontrol.CameraPayload, len(cams))
	for i, c := range cams {
		out[i] = recordercontrol.CameraPayload{
			ID:         c.ID,
			TenantID:   c.TenantID,
			RecorderID: recorderID,
			Name:       c.DisplayName,
			// CredentialRef is the opaque blob ref, not the plaintext.
			// The Recorder fetches the actual secret from cryptostore (KAI-251).
			CredentialRef: credentialRef(c.ID),
			ConfigJSON:    c.AIFeatures, // placeholder until CameraConfig proto lands (KAI-310)
		}
	}
	return out, nil
}

// credentialRef builds the opaque credential reference the Recorder uses to
// fetch RTSP secrets from cryptostore. Format: "cam:<camera_id>:rtsp".
// The Recorder derives the cryptostore subkey from this ref (KAI-251).
func credentialRef(cameraID string) string {
	return "cam:" + cameraID + ":rtsp"
}

// -----------------------------------------------------------------------
// StreamsCameraRegistry — adapts CameraRegistry to streams.CameraRegistry
// -----------------------------------------------------------------------

// StreamsCameraRegistry adapts CameraRegistry to the streams.CameraRegistry
// interface. This resolves TODO(KAI-249) in
// internal/cloud/streams/service.go.
//
// Usage:
//
//	cameraReg := cameras.NewCameraRegistry(db)
//	svc, _ := streams.NewService(streams.Config{
//	    CameraRegistry: cameras.NewStreamsCameraRegistry(cameraReg),
//	    ...
//	})
type StreamsCameraRegistry struct {
	reg CameraRegistry
}

// NewStreamsCameraRegistry constructs a StreamsCameraRegistry that satisfies
// streams.CameraRegistry using the real camera registry.
func NewStreamsCameraRegistry(reg CameraRegistry) *StreamsCameraRegistry {
	return &StreamsCameraRegistry{reg: reg}
}

// GetCamera implements streams.CameraRegistry. It returns ErrCameraNotFound
// (the streams package sentinel) when the camera does not exist in the tenant,
// so the service can map it to 404 without leaking cross-tenant existence.
func (r *StreamsCameraRegistry) GetCamera(
	ctx context.Context,
	tenantID, cameraID string,
) (streams.Camera, error) {
	cam, err := r.reg.GetCamera(ctx, tenantID, cameraID)
	if errors.Is(err, ErrNotFound) {
		return streams.Camera{}, streams.ErrCameraNotFound
	}
	if err != nil {
		return streams.Camera{}, err
	}
	return streams.Camera{
		ID:         cam.ID,
		RecorderID: cam.AssignedRecorderID,
		// RecorderLANSubnets and RecorderLANBaseURL come from the recorder row.
		// The full lookup (joining cameras → recorders) is deferred to KAI-258
		// when the full routing algorithm lands. For now the routing.go stub
		// falls back to the router's managed-relay URL.
	}, nil
}
