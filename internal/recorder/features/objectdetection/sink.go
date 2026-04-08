package objectdetection

import (
	"context"
	"log/slog"
	"sync"

	"github.com/bluenviron/mediamtx/internal/shared/logging"
)

// DetectionEventSink receives post-filter detection events. Implementations
// MUST be safe for concurrent calls from multiple goroutines. The recorder
// wires the production sink up to DirectoryIngest.PublishAIEvents (the
// proto stream introduced in KAI-238); in tests and local runs, callers
// can use the InMemorySink or the LoggingSink below.
type DetectionEventSink interface {
	Publish(ctx context.Context, events []Detection) error
}

// SinkFunc lets a plain function satisfy DetectionEventSink without a
// wrapper type.
type SinkFunc func(ctx context.Context, events []Detection) error

// Publish implements DetectionEventSink.
func (f SinkFunc) Publish(ctx context.Context, events []Detection) error {
	if f == nil {
		return nil
	}
	return f(ctx, events)
}

// InMemorySink accumulates detection events in a slice. Intended for
// tests and for the local dev inspector. Safe for concurrent use.
type InMemorySink struct {
	mu     sync.Mutex
	events []Detection
}

// NewInMemorySink constructs an empty InMemorySink.
func NewInMemorySink() *InMemorySink { return &InMemorySink{} }

// Publish implements DetectionEventSink.
func (s *InMemorySink) Publish(_ context.Context, events []Detection) error {
	if len(events) == 0 {
		return nil
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	s.events = append(s.events, events...)
	return nil
}

// Events returns a snapshot of all events the sink has received.
func (s *InMemorySink) Events() []Detection {
	s.mu.Lock()
	defer s.mu.Unlock()
	out := make([]Detection, len(s.events))
	copy(out, s.events)
	return out
}

// Len returns the number of events received.
func (s *InMemorySink) Len() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return len(s.events)
}

// Reset clears the accumulated events.
func (s *InMemorySink) Reset() {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.events = s.events[:0]
}

// LoggingSink publishes detection events as structured slog records. It
// is a stub that stands in for the DirectoryIngest.PublishAIEvents wiring
// until the recorder integration lands.
type LoggingSink struct {
	logger *slog.Logger
}

// NewLoggingSink constructs a LoggingSink. If logger is nil, a component
// logger rooted at "objectdetection" is created from the shared logging
// package.
func NewLoggingSink(logger *slog.Logger) *LoggingSink {
	if logger == nil {
		logger = logging.WithComponent(
			logging.New(logging.Options{Component: "objectdetection"}),
			"objectdetection",
		)
	}
	return &LoggingSink{logger: logger}
}

// Publish implements DetectionEventSink.
func (s *LoggingSink) Publish(ctx context.Context, events []Detection) error {
	logger := logging.LoggerFromContext(ctx, s.logger)
	for _, e := range events {
		logger.LogAttrs(ctx, slog.LevelInfo, "object_detection_event",
			slog.String("camera_id", e.CameraID),
			slog.String("class", e.Class),
			slog.Int("class_id", e.ClassID),
			slog.Float64("confidence", e.Confidence),
			slog.Float64("bbox_x1", e.BoundingBox.X1),
			slog.Float64("bbox_y1", e.BoundingBox.Y1),
			slog.Float64("bbox_x2", e.BoundingBox.X2),
			slog.Float64("bbox_y2", e.BoundingBox.Y2),
			slog.String("track_id", e.TrackID),
			slog.Time("timestamp", e.Timestamp),
		)
	}
	return nil
}
