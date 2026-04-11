package clip

import (
	"context"
	"encoding/base64"
	"encoding/binary"
	"fmt"
	"math"
	"time"

	"github.com/bluenviron/mediamtx/internal/recorder/directoryingest"
)

// AIEventsPublisher is an EmbeddingSink that bridges CLIP embeddings to the
// DirectoryIngest AIEventsClient. Each embedding is published as an AIEvent
// with kind "clip_embedding" and the vector stored as a base64-encoded
// little-endian float32 array in the Attributes map.
type AIEventsPublisher struct {
	client *directoryingest.AIEventsClient
}

// NewAIEventsPublisher constructs a publisher that writes to the given
// AIEventsClient.
func NewAIEventsPublisher(client *directoryingest.AIEventsClient) *AIEventsPublisher {
	return &AIEventsPublisher{client: client}
}

// Publish implements EmbeddingSink. Each embedding becomes one AIEvent.
func (p *AIEventsPublisher) Publish(_ context.Context, embeddings []Embedding) error {
	for _, emb := range embeddings {
		encoded := encodeVector(emb.Vector)
		attrs := map[string]string{
			"embedding":     encoded,
			"embedding_dim": fmt.Sprintf("%d", len(emb.Vector)),
			"model_id":      emb.ModelID,
			"latency_ms":    fmt.Sprintf("%d", emb.Latency.Milliseconds()),
		}
		event := directoryingest.AIEvent{
			EventID:    generateEventID(emb.CameraID, emb.CapturedAt),
			CameraID:   emb.CameraID,
			Kind:       "clip_embedding",
			KindLabel:  "CLIP Image Embedding",
			ObservedAt: emb.CapturedAt,
			Attributes: attrs,
		}
		p.client.Publish(event)
	}
	return nil
}

// encodeVector serialises a float32 slice to base64-encoded little-endian
// bytes. This is compact and lossless.
func encodeVector(v []float32) string {
	buf := make([]byte, len(v)*4)
	for i, f := range v {
		binary.LittleEndian.PutUint32(buf[i*4:], math.Float32bits(f))
	}
	return base64.StdEncoding.EncodeToString(buf)
}

// DecodeVector deserialises a base64-encoded little-endian float32 vector.
// Exported for use by downstream consumers (forensic search, etc.).
func DecodeVector(s string) ([]float32, error) {
	buf, err := base64.StdEncoding.DecodeString(s)
	if err != nil {
		return nil, fmt.Errorf("clip: decode base64: %w", err)
	}
	if len(buf)%4 != 0 {
		return nil, fmt.Errorf("clip: decoded bytes length %d not divisible by 4", len(buf))
	}
	v := make([]float32, len(buf)/4)
	for i := range v {
		v[i] = math.Float32frombits(binary.LittleEndian.Uint32(buf[i*4:]))
	}
	return v, nil
}

// generateEventID produces a deterministic event id from camera + timestamp.
func generateEventID(cameraID string, t time.Time) string {
	return fmt.Sprintf("clip-%s-%d", cameraID, t.UnixNano())
}
