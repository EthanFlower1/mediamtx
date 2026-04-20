// Package audio implements edge-only audio analytics for the NVR AI subsystem.
// It provides specialized detectors for gunshot, glass break, raised voices,
// and siren/horn events. All inference runs locally via ONNX Runtime with
// sub-2-second event-to-notification latency.
package audio

import (
	"time"
)

// EventType identifies the category of detected audio event.
type EventType string

const (
	EventGunshot      EventType = "gunshot"
	EventGlassBreak   EventType = "glass_break"
	EventRaisedVoices EventType = "raised_voices"
	EventSirenHorn    EventType = "siren_horn"
)

// AllEventTypes returns every supported audio event type.
func AllEventTypes() []EventType {
	return []EventType{
		EventGunshot,
		EventGlassBreak,
		EventRaisedVoices,
		EventSirenHorn,
	}
}

// AudioEvent represents a detected audio event from the classification pipeline.
type AudioEvent struct {
	Type       EventType `json:"type"`
	Confidence float32   `json:"confidence"`
	Timestamp  time.Time `json:"timestamp"`
	CameraID   string    `json:"camera_id"`
	CameraName string    `json:"camera_name"`
	// Latency is the time from audio capture to event emission.
	Latency time.Duration `json:"latency_ms"`
}

// AudioFrame is a chunk of decoded PCM audio ready for classification.
type AudioFrame struct {
	// Samples holds mono PCM float32 samples normalized to [-1, 1].
	Samples    []float32
	SampleRate int
	Timestamp  time.Time
}

// Config holds per-camera audio analytics configuration.
type Config struct {
	CameraID   string `json:"camera_id"`
	CameraName string `json:"camera_name"`
	StreamURL  string `json:"stream_url"` // RTSP URL for audio extraction

	// Enabled controls whether audio analytics runs for this camera.
	Enabled bool `json:"enabled"`

	// EnabledEvents selects which event types are active.
	// If empty and Enabled is true, all event types are active.
	EnabledEvents []EventType `json:"enabled_events,omitempty"`

	// ConfidenceThresholds sets per-event-type minimum confidence.
	// Missing entries use DefaultConfidenceThreshold.
	ConfidenceThresholds map[EventType]float32 `json:"confidence_thresholds,omitempty"`

	// ModelDir is the directory containing ONNX model files.
	// Defaults to the global models directory.
	ModelDir string `json:"model_dir,omitempty"`
}

// DefaultConfidenceThreshold is used when no per-event threshold is configured.
const DefaultConfidenceThreshold float32 = 0.60

// ConfidenceFor returns the confidence threshold for the given event type,
// falling back to DefaultConfidenceThreshold if not explicitly set.
func (c *Config) ConfidenceFor(evt EventType) float32 {
	if c.ConfidenceThresholds != nil {
		if t, ok := c.ConfidenceThresholds[evt]; ok {
			return t
		}
	}
	return DefaultConfidenceThreshold
}

// IsEventEnabled returns true if the given event type is enabled for this camera.
func (c *Config) IsEventEnabled(evt EventType) bool {
	if !c.Enabled {
		return false
	}
	if len(c.EnabledEvents) == 0 {
		return true // all enabled by default
	}
	for _, e := range c.EnabledEvents {
		if e == evt {
			return true
		}
	}
	return false
}

// AudioEventPublisher is the interface for emitting audio detection events
// to the notification pipeline. This mirrors the existing EventPublisher
// pattern from the video AI pipeline.
type AudioEventPublisher interface {
	PublishAudioEvent(event AudioEvent)
}
