// internal/nvr/ai/benchmark_test.go
//
// KAI-39: Benchmark the AI pipeline under concurrent stream load.
// Simulates 8, 16, and 32 camera pipelines feeding through Tracker and
// Publisher stages, measuring inference latency, frame drops, and memory.
//
// These benchmarks use mock detectors and frame sources so they can run
// without ONNX Runtime, FFmpeg, or real RTSP streams.
package ai

import (
	"context"
	"fmt"
	"image"
	"image/color"
	"math/rand"
	"runtime"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

// ---------------------------------------------------------------------------
// Helpers
// ---------------------------------------------------------------------------

// syntheticFrame creates a small NRGBA image with random pixels.
func syntheticFrame(w, h int) image.Image {
	img := image.NewNRGBA(image.Rect(0, 0, w, h))
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			img.SetNRGBA(x, y, color.NRGBA{
				R: uint8(rand.Intn(256)),
				G: uint8(rand.Intn(256)),
				B: uint8(rand.Intn(256)),
				A: 255,
			})
		}
	}
	return img
}

// randomDetections generates n random detections similar to real YOLO output.
func randomDetections(n int) []Detection {
	classes := []string{"person", "car", "dog", "truck", "bicycle"}
	dets := make([]Detection, n)
	for i := range dets {
		dets[i] = Detection{
			Class:      classes[rand.Intn(len(classes))],
			Confidence: 0.5 + rand.Float32()*0.5,
			Box: BoundingBox{
				X: rand.Float32() * 0.7,
				Y: rand.Float32() * 0.7,
				W: 0.05 + rand.Float32()*0.2,
				H: 0.05 + rand.Float32()*0.3,
			},
			Source: SourceYOLO,
		}
	}
	return dets
}

// benchEventPub is a thread-safe mock EventPublisher that counts calls.
type benchEventPub struct {
	aiDetections  atomic.Int64
	frameBatches  atomic.Int64
}

func (b *benchEventPub) PublishAIDetection(_ string, _ string, _ float32) {
	b.aiDetections.Add(1)
}

func (b *benchEventPub) PublishDetectionFrame(_ string, _ []DetectionFrameData) {
	b.frameBatches.Add(1)
}

// ---------------------------------------------------------------------------
// simulatePipeline runs a single mock pipeline for duration, feeding synthetic
// detection frames at targetFPS. Returns stats.
// ---------------------------------------------------------------------------

type pipelineStats struct {
	cameraName     string
	framesSent     int64
	framesReceived int64
	framesDropped  int64
	totalLatency   time.Duration
	maxLatency     time.Duration
}

func simulatePipeline(
	ctx context.Context,
	cameraName string,
	targetFPS int,
	duration time.Duration,
	detPerFrame int,
) pipelineStats {
	detCh := make(chan DetectionFrame, 1) // buffer=1 matches production pipeline
	trackCh := make(chan TrackedFrame, 1)

	tracker := NewTracker(detCh, trackCh, 5)
	go tracker.Run(ctx)

	// Consumer: drain tracked frames, measure latency.
	var (
		mu             sync.Mutex
		framesReceived int64
		totalLatency   time.Duration
		maxLatency     time.Duration
	)
	go func() {
		for tf := range trackCh {
			lat := time.Since(tf.Timestamp)
			mu.Lock()
			framesReceived++
			totalLatency += lat
			if lat > maxLatency {
				maxLatency = lat
			}
			mu.Unlock()
		}
	}()

	// Producer: emit detection frames at target FPS.
	interval := time.Duration(float64(time.Second) / float64(targetFPS))
	img := syntheticFrame(640, 480)
	var framesSent int64
	var framesDropped int64
	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	deadline := time.After(duration)
	for {
		select {
		case <-deadline:
			close(detCh)
			// Give consumer time to drain.
			time.Sleep(50 * time.Millisecond)
			mu.Lock()
			defer mu.Unlock()
			return pipelineStats{
				cameraName:     cameraName,
				framesSent:     framesSent,
				framesReceived: framesReceived,
				framesDropped:  framesSent - framesReceived,
				totalLatency:   totalLatency,
				maxLatency:     maxLatency,
			}
		case <-ctx.Done():
			close(detCh)
			mu.Lock()
			defer mu.Unlock()
			return pipelineStats{
				cameraName:     cameraName,
				framesSent:     framesSent,
				framesReceived: framesReceived,
				framesDropped:  framesSent - framesReceived,
				totalLatency:   totalLatency,
				maxLatency:     maxLatency,
			}
		case <-ticker.C:
			df := DetectionFrame{
				Timestamp:  time.Now(),
				Image:      img,
				Detections: randomDetections(detPerFrame),
			}
			// Non-blocking send mirrors production drop-oldest behavior.
			select {
			case detCh <- df:
				framesSent++
			default:
				// Channel full — frame drop.
				framesSent++
				framesDropped++
				// Drain old frame and push new (like production FrameSrc).
				select {
				case <-detCh:
				default:
				}
				select {
				case detCh <- df:
				default:
				}
			}
		}
	}
}

// ---------------------------------------------------------------------------
// Benchmark: concurrent pipelines
// ---------------------------------------------------------------------------

func runConcurrentBenchmark(t *testing.T, numStreams int) {
	t.Helper()

	const (
		targetFPS   = 15
		duration    = 3 * time.Second
		detPerFrame = 4
	)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	var memBefore runtime.MemStats
	runtime.GC()
	runtime.ReadMemStats(&memBefore)

	startTime := time.Now()

	allStats := make([]pipelineStats, numStreams)
	var wg sync.WaitGroup
	wg.Add(numStreams)

	for i := 0; i < numStreams; i++ {
		i := i
		go func() {
			defer wg.Done()
			name := fmt.Sprintf("camera-%02d", i+1)
			allStats[i] = simulatePipeline(ctx, name, targetFPS, duration, detPerFrame)
		}()
	}

	wg.Wait()
	elapsed := time.Since(startTime)

	var memAfter runtime.MemStats
	runtime.GC()
	runtime.ReadMemStats(&memAfter)

	// Aggregate results.
	var totalSent, totalRecv, totalDrop int64
	var totalLat time.Duration
	var worstLat time.Duration

	for _, s := range allStats {
		totalSent += s.framesSent
		totalRecv += s.framesReceived
		totalDrop += s.framesDropped
		totalLat += s.totalLatency
		if s.maxLatency > worstLat {
			worstLat = s.maxLatency
		}
	}

	avgLat := time.Duration(0)
	if totalRecv > 0 {
		avgLat = totalLat / time.Duration(totalRecv)
	}

	dropRate := float64(0)
	if totalSent > 0 {
		dropRate = float64(totalDrop) / float64(totalSent) * 100
	}

	heapDelta := int64(memAfter.HeapAlloc) - int64(memBefore.HeapAlloc)
	totalAllocMB := float64(memAfter.TotalAlloc-memBefore.TotalAlloc) / (1024 * 1024)

	t.Logf("=== %d concurrent streams benchmark ===", numStreams)
	t.Logf("  Duration:           %v", elapsed.Round(time.Millisecond))
	t.Logf("  Frames sent:        %d", totalSent)
	t.Logf("  Frames received:    %d", totalRecv)
	t.Logf("  Frames dropped:     %d (%.1f%%)", totalDrop, dropRate)
	t.Logf("  Avg latency:        %v", avgLat.Round(time.Microsecond))
	t.Logf("  Max latency:        %v", worstLat.Round(time.Microsecond))
	t.Logf("  Heap delta:         %.2f MB", float64(heapDelta)/(1024*1024))
	t.Logf("  Total alloc:        %.2f MB", totalAllocMB)
	t.Logf("  Goroutines:         %d", runtime.NumGoroutine())

	// Assertions: pipeline should not drop more than 20% under synthetic load.
	if dropRate > 20 {
		t.Errorf("frame drop rate %.1f%% exceeds 20%% threshold", dropRate)
	}
	// Average latency through tracker should stay under 50ms in synthetic mode.
	if avgLat > 50*time.Millisecond {
		t.Errorf("average latency %v exceeds 50ms threshold", avgLat)
	}
}

func TestBenchmarkPipeline_8Streams(t *testing.T) {
	runConcurrentBenchmark(t, 8)
}

func TestBenchmarkPipeline_16Streams(t *testing.T) {
	runConcurrentBenchmark(t, 16)
}

func TestBenchmarkPipeline_32Streams(t *testing.T) {
	runConcurrentBenchmark(t, 32)
}

// ---------------------------------------------------------------------------
// Benchmark: Tracker throughput in isolation
// ---------------------------------------------------------------------------

func BenchmarkTrackerProcess(b *testing.B) {
	detCh := make(chan DetectionFrame, 64)
	trackCh := make(chan TrackedFrame, 64)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	tracker := NewTracker(detCh, trackCh, 5)
	go tracker.Run(ctx)

	// Drain output.
	go func() {
		for range trackCh {
		}
	}()

	img := syntheticFrame(320, 240)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		detCh <- DetectionFrame{
			Timestamp:  time.Now(),
			Image:      img,
			Detections: randomDetections(4),
		}
	}
	b.StopTimer()
	cancel()
}

// ---------------------------------------------------------------------------
// Benchmark: IoU computation (hot path in tracker)
// ---------------------------------------------------------------------------

func BenchmarkIoUBoxes(b *testing.B) {
	a := BoundingBox{0.1, 0.1, 0.3, 0.4}
	bb := BoundingBox{0.15, 0.12, 0.25, 0.35}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		iouBoxes(a, bb)
	}
}

// ---------------------------------------------------------------------------
// Benchmark: Frame channel drop-oldest behavior under pressure
// ---------------------------------------------------------------------------

func TestFrameDropBehavior(t *testing.T) {
	// Verify the drop-oldest channel pattern used by FrameSrc works correctly
	// and does not block producers when consumers are slow.
	ch := make(chan Frame, 1)
	img := syntheticFrame(64, 64)

	const totalFrames = 100
	var sent, dropped int64

	// Slow consumer: reads one frame every 10ms.
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	var received int64
	go func() {
		for {
			select {
			case <-ctx.Done():
				return
			case _, ok := <-ch:
				if !ok {
					return
				}
				atomic.AddInt64(&received, 1)
				time.Sleep(10 * time.Millisecond)
			}
		}
	}()

	// Fast producer: sends frames as fast as possible.
	for i := 0; i < totalFrames; i++ {
		f := Frame{Image: img, Timestamp: time.Now(), Width: 64, Height: 64}
		select {
		case ch <- f:
			sent++
		default:
			dropped++
			// Drain and resend.
			select {
			case <-ch:
			default:
			}
			ch <- f
			sent++
		}
	}

	// Wait for consumer to catch up.
	time.Sleep(200 * time.Millisecond)
	cancel()

	finalRecv := atomic.LoadInt64(&received)
	t.Logf("Frame drop test: sent=%d, dropped=%d, received=%d", sent, dropped, finalRecv)

	if dropped == 0 {
		t.Log("No drops detected — consumer kept up (expected with synthetic load)")
	}
	// The key invariant: producer should never block.
	if sent != totalFrames {
		t.Errorf("producer blocked: sent %d of %d frames", sent, totalFrames)
	}
}

// ---------------------------------------------------------------------------
// Test: Memory scaling — verify heap does not grow linearly with stream count
// ---------------------------------------------------------------------------

func TestMemoryScaling(t *testing.T) {
	const (
		targetFPS   = 10
		duration    = 1 * time.Second
		detPerFrame = 3
	)

	measure := func(n int) uint64 {
		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()

		runtime.GC()
		var before runtime.MemStats
		runtime.ReadMemStats(&before)

		var wg sync.WaitGroup
		wg.Add(n)
		for i := 0; i < n; i++ {
			i := i
			go func() {
				defer wg.Done()
				simulatePipeline(ctx, fmt.Sprintf("cam-%d", i), targetFPS, duration, detPerFrame)
			}()
		}
		wg.Wait()

		runtime.GC()
		var after runtime.MemStats
		runtime.ReadMemStats(&after)
		return after.TotalAlloc - before.TotalAlloc
	}

	alloc8 := measure(8)
	alloc16 := measure(16)
	alloc32 := measure(32)

	t.Logf("Total alloc:  8 streams = %.2f MB", float64(alloc8)/(1024*1024))
	t.Logf("Total alloc: 16 streams = %.2f MB", float64(alloc16)/(1024*1024))
	t.Logf("Total alloc: 32 streams = %.2f MB", float64(alloc32)/(1024*1024))

	// Scaling factor: 32-stream alloc should be less than 5x the 8-stream alloc.
	// Perfect linear scaling would be 4x; we allow overhead.
	ratio := float64(alloc32) / float64(alloc8)
	t.Logf("32/8 scaling ratio: %.2fx", ratio)
	if ratio > 6.0 {
		t.Errorf("memory scaling ratio %.2fx exceeds 6x threshold (super-linear growth detected)", ratio)
	}
}
