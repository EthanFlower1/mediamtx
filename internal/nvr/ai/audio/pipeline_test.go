package audio

import (
	"context"
	"math"
	"sync"
	"testing"
	"time"
)

// mockEventPublisher collects published audio events for test assertions.
type mockEventPublisher struct {
	mu     sync.Mutex
	events []AudioEvent
}

func (m *mockEventPublisher) PublishAudioEvent(event AudioEvent) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.events = append(m.events, event)
}

func (m *mockEventPublisher) Events() []AudioEvent {
	m.mu.Lock()
	defer m.mu.Unlock()
	cp := make([]AudioEvent, len(m.events))
	copy(cp, m.events)
	return cp
}

func TestPipeline_DetectsGunshot(t *testing.T) {
	pub := &mockEventPublisher{}
	metrics := NewMetrics()

	classifier := NewClassifier("/tmp/models")
	classifier.LoadModel(&AudioModel{
		EventType: EventGunshot,
		InferFunc: func(features []float32) (float32, error) {
			return 0.92, nil // always high confidence
		},
	})

	// Generate synthetic audio: impulsive noise (simulates gunshot).
	sampleRate := 16000
	samples := generateImpulse(sampleRate, 1.0)

	// Create a capture that delivers one frame then blocks.
	frameDelivered := make(chan struct{})
	capture := NewAudioCaptureWithFunc(
		func(ctx context.Context) (AudioFrame, error) {
			select {
			case <-frameDelivered:
				// Block after first frame until context cancelled.
				<-ctx.Done()
				return AudioFrame{}, ctx.Err()
			default:
				close(frameDelivered)
				return AudioFrame{
					Samples:    samples,
					SampleRate: sampleRate,
					Timestamp:  time.Now(),
				}, nil
			}
		},
		nil,
	)

	config := Config{
		CameraID:   "cam-test-1",
		CameraName: "Test Camera",
		StreamURL:  "rtsp://test/stream",
		Enabled:    true,
	}

	pipeline := NewPipeline(config, classifier, pub, metrics)

	// Override the audio capture in the pipeline by running stages manually.
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()

	// Run the pipeline stages manually with the mock capture.
	audioCh := make(chan AudioFrame, 4)
	featureCh := make(chan classificationInput, 4)

	var wg sync.WaitGroup

	// Feed audio from mock capture.
	wg.Add(1)
	go func() {
		defer wg.Done()
		defer close(audioCh)
		for {
			frame, err := capture.ReadFrame(ctx)
			if err != nil {
				return
			}
			select {
			case audioCh <- frame:
			case <-ctx.Done():
				return
			}
		}
	}()

	// Feature extraction.
	wg.Add(1)
	go func() {
		defer wg.Done()
		defer close(featureCh)
		pipeline.runFeatureExtraction(ctx, audioCh, featureCh)
	}()

	// Classification.
	wg.Add(1)
	go func() {
		defer wg.Done()
		pipeline.runClassification(ctx, featureCh)
	}()

	// Wait for detection or timeout.
	deadline := time.After(2 * time.Second)
	for {
		select {
		case <-deadline:
			events := pub.Events()
			if len(events) == 0 {
				t.Fatal("expected gunshot detection within 2 seconds")
			}
			// Verify the event.
			evt := events[0]
			if evt.Type != EventGunshot {
				t.Errorf("expected gunshot event, got %s", evt.Type)
			}
			if evt.Confidence < 0.9 {
				t.Errorf("expected high confidence, got %f", evt.Confidence)
			}
			if evt.Latency > 2*time.Second {
				t.Errorf("latency %v exceeds 2s requirement", evt.Latency)
			}
			cancel()
			wg.Wait()
			return
		default:
			events := pub.Events()
			if len(events) > 0 {
				evt := events[0]
				if evt.Type != EventGunshot {
					t.Errorf("expected gunshot event, got %s", evt.Type)
				}
				if evt.Latency > 2*time.Second {
					t.Errorf("latency %v exceeds 2s requirement", evt.Latency)
				}
				cancel()
				wg.Wait()
				return
			}
			time.Sleep(10 * time.Millisecond)
		}
	}
}

func TestPipeline_RespectsConfidenceThreshold(t *testing.T) {
	pub := &mockEventPublisher{}
	metrics := NewMetrics()

	classifier := NewClassifier("/tmp/models")
	classifier.LoadModel(&AudioModel{
		EventType: EventGlassBreak,
		InferFunc: func(features []float32) (float32, error) {
			return 0.40, nil // below default threshold
		},
	})

	config := Config{
		CameraID:   "cam-test-2",
		CameraName: "Test Camera 2",
		StreamURL:  "rtsp://test/stream2",
		Enabled:    true,
	}

	pipeline := NewPipeline(config, classifier, pub, metrics)

	ctx, cancel := context.WithTimeout(context.Background(), 500*time.Millisecond)
	defer cancel()

	featureCh := make(chan classificationInput, 1)
	featureCh <- classificationInput{
		features:   make([]float32, 100),
		capturedAt: time.Now(),
	}
	close(featureCh)

	pipeline.runClassification(ctx, featureCh)

	events := pub.Events()
	if len(events) != 0 {
		t.Errorf("expected no events (below threshold), got %d", len(events))
	}
}

func TestPipeline_CooldownSuppression(t *testing.T) {
	pub := &mockEventPublisher{}
	metrics := NewMetrics()

	classifier := NewClassifier("/tmp/models")
	classifier.LoadModel(&AudioModel{
		EventType: EventSirenHorn,
		InferFunc: func(features []float32) (float32, error) {
			return 0.90, nil
		},
	})

	config := Config{
		CameraID:   "cam-test-3",
		CameraName: "Test Camera 3",
		StreamURL:  "rtsp://test/stream3",
		Enabled:    true,
	}

	pipeline := NewPipeline(config, classifier, pub, metrics)

	ctx, cancel := context.WithTimeout(context.Background(), 500*time.Millisecond)
	defer cancel()

	// Send multiple frames rapidly.
	featureCh := make(chan classificationInput, 10)
	for i := 0; i < 5; i++ {
		featureCh <- classificationInput{
			features:   make([]float32, 100),
			capturedAt: time.Now(),
		}
	}
	close(featureCh)

	pipeline.runClassification(ctx, featureCh)

	events := pub.Events()
	// Should only get 1 event due to cooldown.
	if len(events) != 1 {
		t.Errorf("expected 1 event (cooldown suppression), got %d", len(events))
	}
}

func TestPipeline_PerCameraDisable(t *testing.T) {
	pub := &mockEventPublisher{}
	metrics := NewMetrics()
	classifier := NewClassifier("/tmp/models")

	config := Config{
		CameraID:      "cam-disabled",
		CameraName:    "Disabled Camera",
		StreamURL:     "rtsp://test/disabled",
		Enabled:       true,
		EnabledEvents: []EventType{EventGunshot}, // only gunshot
	}

	pipeline := NewPipeline(config, classifier, pub, metrics)

	enabled := pipeline.enabledEvents()
	if len(enabled) != 1 || enabled[0] != EventGunshot {
		t.Errorf("expected only gunshot enabled, got %v", enabled)
	}
}

// generateImpulse creates a synthetic impulsive sound (like a gunshot).
func generateImpulse(sampleRate int, durationSec float64) []float32 {
	numSamples := int(float64(sampleRate) * durationSec)
	samples := make([]float32, numSamples)

	// Sharp impulse at 100ms.
	impulseStart := sampleRate / 10
	impulseDuration := sampleRate / 100 // 10ms

	for i := impulseStart; i < impulseStart+impulseDuration && i < numSamples; i++ {
		offset := float64(i - impulseStart)
		decay := math.Exp(-offset / float64(impulseDuration/3))
		samples[i] = float32(decay * 0.95)
	}

	return samples
}
