package audio

import (
	"context"
	"log"
	"sync"
	"time"
)

// Pipeline orchestrates the audio analytics stages for a single camera:
//   1. AudioCapture: extracts audio from RTSP stream
//   2. FeatureExtraction: converts PCM to mel-spectrogram
//   3. Classification: runs ONNX models for each enabled event type
//   4. Event Emission: publishes events above confidence threshold
//
// The pipeline targets <2s end-to-end latency from audio trigger to notification.
type Pipeline struct {
	config     Config
	classifier *Classifier
	eventPub   AudioEventPublisher
	metrics    *Metrics

	cancel context.CancelFunc
	wg     sync.WaitGroup
}

// NewPipeline creates a new audio analytics pipeline for a single camera.
func NewPipeline(
	config Config,
	classifier *Classifier,
	eventPub AudioEventPublisher,
	metrics *Metrics,
) *Pipeline {
	return &Pipeline{
		config:     config,
		classifier: classifier,
		eventPub:   eventPub,
		metrics:    metrics,
	}
}

// Start launches the audio analytics pipeline goroutines.
func (p *Pipeline) Start(parentCtx context.Context) {
	ctx, cancel := context.WithCancel(parentCtx)
	p.cancel = cancel

	audioCh := make(chan AudioFrame, 4)
	featureCh := make(chan classificationInput, 4)

	// Stage 1: Audio capture (reads from RTSP stream).
	p.wg.Add(1)
	go func() {
		defer p.wg.Done()
		defer close(audioCh)
		p.runAudioCapture(ctx, audioCh)
	}()

	// Stage 2: Feature extraction (PCM -> mel-spectrogram).
	p.wg.Add(1)
	go func() {
		defer p.wg.Done()
		defer close(featureCh)
		p.runFeatureExtraction(ctx, audioCh, featureCh)
	}()

	// Stage 3: Classification + event emission.
	p.wg.Add(1)
	go func() {
		defer p.wg.Done()
		p.runClassification(ctx, featureCh)
	}()

	log.Printf("[audio][%s] pipeline started (events: %v)",
		p.config.CameraName, p.enabledEvents())
}

// Stop cancels the pipeline and waits for all goroutines to finish.
func (p *Pipeline) Stop() {
	if p.cancel != nil {
		p.cancel()
	}
	p.wg.Wait()
	log.Printf("[audio][%s] pipeline stopped", p.config.CameraName)
}

// classificationInput pairs mel-spectrogram features with capture metadata.
type classificationInput struct {
	features   []float32
	capturedAt time.Time
}

// runAudioCapture extracts audio from the RTSP stream and sends PCM frames.
// In production this would use ffmpeg or a Go RTSP client to demux audio.
// For now it acts as the integration point — real capture requires an
// AudioCapture implementation (see capture.go).
func (p *Pipeline) runAudioCapture(ctx context.Context, out chan<- AudioFrame) {
	capture := NewAudioCapture(p.config.StreamURL)
	if err := capture.Open(); err != nil {
		log.Printf("[audio][%s] capture open failed: %v", p.config.CameraName, err)
		return
	}
	defer capture.Close()

	for {
		select {
		case <-ctx.Done():
			return
		default:
			frame, err := capture.ReadFrame(ctx)
			if err != nil {
				if ctx.Err() != nil {
					return
				}
				log.Printf("[audio][%s] capture read error: %v", p.config.CameraName, err)
				time.Sleep(100 * time.Millisecond)
				continue
			}
			select {
			case out <- frame:
			case <-ctx.Done():
				return
			}
		}
	}
}

// runFeatureExtraction converts raw PCM audio to mel-spectrogram features.
func (p *Pipeline) runFeatureExtraction(ctx context.Context, in <-chan AudioFrame, out chan<- classificationInput) {
	const (
		targetSampleRate = 16000
		nMels            = 64
		fftSize          = 512
		hopSize          = 160
	)

	for {
		select {
		case <-ctx.Done():
			return
		case frame, ok := <-in:
			if !ok {
				return
			}

			features := ExtractMelSpectrogram(frame.Samples, targetSampleRate, nMels, fftSize, hopSize)
			if len(features) == 0 {
				continue
			}

			select {
			case out <- classificationInput{
				features:   features,
				capturedAt: frame.Timestamp,
			}:
			case <-ctx.Done():
				return
			}
		}
	}
}

// runClassification runs all enabled classifiers on each feature frame
// and emits events that exceed the confidence threshold.
func (p *Pipeline) runClassification(ctx context.Context, in <-chan classificationInput) {
	// Cooldown tracking to suppress rapid-fire duplicate events.
	lastFired := make(map[EventType]time.Time)
	const cooldown = 3 * time.Second

	for {
		select {
		case <-ctx.Done():
			return
		case input, ok := <-in:
			if !ok {
				return
			}

			enabledTypes := p.enabledEvents()

			for _, evt := range enabledTypes {
				if !p.classifier.HasModel(evt) {
					continue
				}

				start := time.Now()
				score, err := p.classifier.Classify(evt, input.features)
				inferenceTime := time.Since(start)

				if err != nil {
					log.Printf("[audio][%s] classify %s error: %v",
						p.config.CameraName, evt, err)
					continue
				}

				threshold := p.config.ConfidenceFor(evt)

				// Record metrics regardless of threshold.
				if p.metrics != nil {
					p.metrics.RecordInference(p.config.CameraID, p.config.CameraName, evt, inferenceTime)
				}

				if score < threshold {
					continue
				}

				// Check cooldown.
				if last, ok := lastFired[evt]; ok && time.Since(last) < cooldown {
					continue
				}

				latency := time.Since(input.capturedAt)
				event := AudioEvent{
					Type:       evt,
					Confidence: score,
					Timestamp:  input.capturedAt,
					CameraID:   p.config.CameraID,
					CameraName: p.config.CameraName,
					Latency:    latency,
				}

				// Record detection metrics.
				if p.metrics != nil {
					p.metrics.RecordDetection(p.config.CameraID, p.config.CameraName, evt, score, latency)
				}

				log.Printf("[audio][%s] detected %s (conf=%.2f, latency=%v)",
					p.config.CameraName, evt, score, latency)

				p.eventPub.PublishAudioEvent(event)
				lastFired[evt] = time.Now()
			}
		}
	}
}

// enabledEvents returns the list of event types enabled for this camera.
func (p *Pipeline) enabledEvents() []EventType {
	if len(p.config.EnabledEvents) > 0 {
		return p.config.EnabledEvents
	}
	return AllEventTypes()
}
