// internal/nvr/ai/pipeline.go
package ai

import (
	"context"
	"log"
	"sync"

	"github.com/bluenviron/mediamtx/internal/nvr/db"
)

// Pipeline orchestrates the four detection stages for a single camera.
type Pipeline struct {
	config            PipelineConfig
	detector          *Detector
	embedder          *Embedder
	database          *db.DB
	eventPub          EventPublisher
	detectionCallback DetectionCallback // optional callback for webhook dispatch

	cancel context.CancelFunc
	wg     sync.WaitGroup
}

// NewPipeline creates a new pipeline. Call Start to begin processing.
func NewPipeline(
	config PipelineConfig,
	detector *Detector,
	embedder *Embedder,
	database *db.DB,
	eventPub EventPublisher,
) *Pipeline {
	return &Pipeline{
		config:   config,
		detector: detector,
		embedder: embedder,
		database: database,
		eventPub: eventPub,
	}
}

// Start launches all pipeline stages as goroutines.
func (p *Pipeline) Start(parentCtx context.Context) {
	ctx, cancel := context.WithCancel(parentCtx)
	p.cancel = cancel

	// Probe resolution if not configured.
	width, height := p.config.StreamWidth, p.config.StreamHeight
	if width == 0 || height == 0 {
		var err error
		width, height, err = ProbeResolution(p.config.StreamURL)
		if err != nil {
			log.Printf("[ai][%s] ffprobe failed, using 640x480: %v", p.config.CameraName, err)
			width, height = 640, 480
		}
		log.Printf("[ai][%s] probed resolution: %dx%d", p.config.CameraName, width, height)
	}

	// Create channels between stages.
	frameCh := make(chan Frame, 1)
	detCh := make(chan DetectionFrame, 1)
	trackCh := make(chan TrackedFrame, 1)

	// Stage 1: FrameSrc
	frameSrc := NewFrameSrc(p.config.StreamURL, width, height, frameCh)
	p.wg.Add(1)
	go func() {
		defer p.wg.Done()
		frameSrc.Run(ctx)
	}()

	// Optional: ONVIF metadata source.
	var onvifSrc *ONVIFSrc
	if p.config.ONVIFMetadataURL != "" {
		onvifSrc = NewONVIFSrc(
			p.config.ONVIFMetadataURL,
			p.config.ONVIFUsername,
			p.config.ONVIFPassword,
		)
		p.wg.Add(1)
		go func() {
			defer p.wg.Done()
			onvifSrc.Run(ctx)
		}()
	}

	// Stage 2: Detector (reads frames, runs YOLO, merges ONVIF, emits DetectionFrame)
	confThresh := p.config.ConfidenceThresh
	if confThresh <= 0 {
		confThresh = 0.5
	}
	p.wg.Add(1)
	go func() {
		defer p.wg.Done()
		defer close(detCh)
		p.runDetector(ctx, frameCh, detCh, onvifSrc, confThresh)
	}()

	// Stage 3: Tracker
	tracker := NewTracker(detCh, trackCh, p.config.TrackTimeout)
	p.wg.Add(1)
	go func() {
		defer p.wg.Done()
		tracker.Run(ctx)
	}()

	// Stage 4: Publisher
	publisher := NewPublisher(trackCh, p.config.CameraID, p.config.CameraName, p.eventPub, p.database, p.embedder)
	if p.detectionCallback != nil {
		publisher.SetDetectionCallback(p.detectionCallback)
	}
	p.wg.Add(1)
	go func() {
		defer p.wg.Done()
		publisher.Run(ctx)
	}()

	log.Printf("[ai][%s] pipeline started (%dx%d, conf=%.2f, timeout=%ds)",
		p.config.CameraName, width, height, confThresh, p.config.TrackTimeout)
}

// SetDetectionCallback sets an optional callback for webhook dispatch.
// Must be called before Start.
func (p *Pipeline) SetDetectionCallback(cb DetectionCallback) {
	p.detectionCallback = cb
}

// Stop cancels the pipeline context and waits for all stages to exit.
func (p *Pipeline) Stop() {
	if p.cancel != nil {
		p.cancel()
	}
	p.wg.Wait()
	log.Printf("[ai][%s] pipeline stopped", p.config.CameraName)
}

func (p *Pipeline) runDetector(
	ctx context.Context,
	in <-chan Frame,
	out chan<- DetectionFrame,
	onvifSrc *ONVIFSrc,
	confThresh float32,
) {
	for {
		select {
		case <-ctx.Done():
			return
		case frame, ok := <-in:
			if !ok {
				return
			}

			yoloDets, err := p.detector.Detect(frame.Image, confThresh)
			if err != nil {
				log.Printf("[ai][%s] detect error: %v", p.config.CameraName, err)
				continue
			}

			// Convert YOLO detections to pipeline Detection type.
			dets := make([]Detection, len(yoloDets))
			for i, yd := range yoloDets {
				dets[i] = Detection{
					Class:      yd.ClassName,
					Confidence: yd.Confidence,
					Box:        BoundingBox{X: yd.X, Y: yd.Y, W: yd.W, H: yd.H},
					Source:     SourceYOLO,
				}
			}

			// Merge ONVIF detections if available.
			if onvifSrc != nil {
				onvifDets := onvifSrc.LatestDetections()
				dets = MergeDetections(dets, onvifDets)
			}

			df := DetectionFrame{
				Timestamp:  frame.Timestamp,
				Image:      frame.Image,
				Detections: dets,
			}
			select {
			case out <- df:
			case <-ctx.Done():
				return
			}
		}
	}
}

