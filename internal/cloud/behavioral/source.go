package behavioral

import (
	"context"
	"encoding/json"
)

// RecorderCameraConfig is the wire-compatible shape of a full behavioral config
// for one camera, sent to the recorder via the StreamAssignments message payload.
// Field names are intentionally identical to the recorder-side CameraConfig JSON
// tags (internal/recorder/features/behavioral/types.go) so deserialization is
// mechanical once KAI-310 lands.
type RecorderCameraConfig struct {
	CameraID  string                   `json:"camera_id"`
	Detectors []RecorderDetectorConfig `json:"detectors"`
}

// RecorderDetectorConfig is the per-detector slice of RecorderCameraConfig.
type RecorderDetectorConfig struct {
	DetectorType string `json:"detector_type"`
	Params       string `json:"params"`
	Enabled      bool   `json:"enabled"`
}

// BuildRecorderPayload converts a slice of Config rows (from Store.List) into
// the JSON string that is embedded in the wire camera message delivered to the
// recorder via recordercontrol (KAI-253).
//
// Returns "{}" on an empty or nil input so the recorder always sees a valid
// JSON object rather than null.
func BuildRecorderPayload(cameraID string, configs []Config) (string, error) {
	out := RecorderCameraConfig{
		CameraID:  cameraID,
		Detectors: make([]RecorderDetectorConfig, 0, len(configs)),
	}
	for _, c := range configs {
		params := c.Params
		if params == "" {
			params = "{}"
		}
		out.Detectors = append(out.Detectors, RecorderDetectorConfig{
			DetectorType: string(c.DetectorType),
			Params:       params,
			Enabled:      c.Enabled,
		})
	}
	b, err := json.Marshal(out)
	if err != nil {
		return "{}", err
	}
	return string(b), nil
}

// StoreBackedSource implements the recorder-side ConfigSource interface using
// the cloud behavioral Store. This is used by the cloud-side reconciler to
// answer GetCameraConfig calls for cameras whose configs have been mutated
// cloud-side but not yet pushed to the recorder.
//
// In production the reconciler calls BuildRecorderPayload and embeds the JSON
// in the wireCamera.BehavioralConfigJSON field (added to the wire message in
// this ticket). The StoreBackedSource is available for the cloud-side
// integration test path where the reconciler is not in the picture.
type StoreBackedSource struct {
	store Store
}

// NewStoreBackedSource wraps a Store as a GetCameraConfig source.
func NewStoreBackedSource(s Store) *StoreBackedSource {
	return &StoreBackedSource{store: s}
}

// GetCameraConfig lists all behavioral configs for the camera from the store
// and serializes them into the recorder wire format. Returns an empty
// RecorderCameraConfig (not an error) if nothing is configured.
func (s *StoreBackedSource) GetCameraConfig(ctx context.Context, tenantID, cameraID string) (string, error) {
	configs, err := s.store.List(ctx, tenantID, cameraID)
	if err != nil {
		return "{}", err
	}
	return BuildRecorderPayload(cameraID, configs)
}
