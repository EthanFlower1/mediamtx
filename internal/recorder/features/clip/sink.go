package clip

import (
	"context"
	"log/slog"
	"sync"

	"github.com/bluenviron/mediamtx/internal/shared/logging"
)

// EmbeddingSink receives computed CLIP embeddings. Implementations MUST be
// safe for concurrent calls. The recorder wires the production sink to
// DirectoryIngest.PublishAIEvents; tests use InMemorySink.
type EmbeddingSink interface {
	Publish(ctx context.Context, embeddings []Embedding) error
}

// SinkFunc lets a plain function satisfy EmbeddingSink.
type SinkFunc func(ctx context.Context, embeddings []Embedding) error

// Publish implements EmbeddingSink.
func (f SinkFunc) Publish(ctx context.Context, embeddings []Embedding) error {
	if f == nil {
		return nil
	}
	return f(ctx, embeddings)
}

// InMemorySink accumulates embeddings in a slice. Intended for tests.
// Safe for concurrent use.
type InMemorySink struct {
	mu         sync.Mutex
	embeddings []Embedding
}

// NewInMemorySink constructs an empty InMemorySink.
func NewInMemorySink() *InMemorySink { return &InMemorySink{} }

// Publish implements EmbeddingSink.
func (s *InMemorySink) Publish(_ context.Context, embeddings []Embedding) error {
	if len(embeddings) == 0 {
		return nil
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	s.embeddings = append(s.embeddings, embeddings...)
	return nil
}

// Embeddings returns a snapshot of all embeddings the sink has received.
func (s *InMemorySink) Embeddings() []Embedding {
	s.mu.Lock()
	defer s.mu.Unlock()
	out := make([]Embedding, len(s.embeddings))
	copy(out, s.embeddings)
	return out
}

// Len returns the number of embeddings received.
func (s *InMemorySink) Len() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return len(s.embeddings)
}

// Reset clears the accumulated embeddings.
func (s *InMemorySink) Reset() {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.embeddings = s.embeddings[:0]
}

// LoggingSink logs embedding events as structured slog records. It stands in
// for the DirectoryIngest wiring until the full recorder integration lands.
type LoggingSink struct {
	logger *slog.Logger
}

// NewLoggingSink constructs a LoggingSink. If logger is nil, a component
// logger rooted at "clip" is created from the shared logging package.
func NewLoggingSink(logger *slog.Logger) *LoggingSink {
	if logger == nil {
		logger = logging.WithComponent(
			logging.New(logging.Options{Component: "clip"}),
			"clip",
		)
	}
	return &LoggingSink{logger: logger}
}

// Publish implements EmbeddingSink.
func (s *LoggingSink) Publish(ctx context.Context, embeddings []Embedding) error {
	logger := logging.LoggerFromContext(ctx, s.logger)
	for _, e := range embeddings {
		logger.LogAttrs(ctx, slog.LevelInfo, "clip_embedding",
			slog.String("camera_id", e.CameraID),
			slog.String("model_id", e.ModelID),
			slog.Int("vector_dim", len(e.Vector)),
			slog.Duration("latency", e.Latency),
			slog.Time("captured_at", e.CapturedAt),
		)
	}
	return nil
}
