package behavioral_test

import (
	"context"
	"encoding/json"
	"fmt"
	"testing"
	"time"

	"github.com/bluenviron/mediamtx/internal/ai/behavioral"
)

// drainPipeline empties the events channel after the pipeline is closed.
// Called only from Cleanup; the empty loop is intentional.
func drainPipeline(p *behavioral.Pipeline) {
	for e := range p.Events() { //nolint:revive
		_ = e
	}
}

// BenchmarkPipeline_6Detectors_60FPS_32Cameras benchmarks the 6-detector
// pipeline at 60 FPS across 32 cameras.
//
// Target: total CPU time for a single frame across all cameras < 50ms.
// The benchmark reports ns/op for one frame across all 32 cameras; divide by
// 32 to get per-camera latency.  At 60 FPS the budget per frame is 16.7ms
// total; with 32 cameras sharing a single goroutine pool the per-camera budget
// is ~0.52ms.  The detectors are lightweight geometric computation so they
// comfortably fit this budget.
func BenchmarkPipeline_6Detectors_60FPS_32Cameras(b *testing.B) {
	const numCameras = 32
	const numDetectionsPerFrame = 10

	// Build 32 pipelines (one per camera).
	pipelines := make([]*behavioral.Pipeline, numCameras)
	for i := range numCameras {
		tenantID := fmt.Sprintf("tenant-%d", i%4) // 4 tenants, 8 cameras each
		cameraID := fmt.Sprintf("cam-%d", i)

		loiJSON, _ := json.Marshal(behavioral.LoiteringParams{
			ROI: square, ThresholdSeconds: 30.0,
		})
		lcJSON, _ := json.Marshal(behavioral.LineCrossingParams{Line: vertLine})
		roiJSON, _ := json.Marshal(behavioral.ROIParams{ROI: square})
		cdJSON, _ := json.Marshal(behavioral.CrowdDensityParams{
			ROI: square, ThresholdCount: 20,
		})
		tgJSON, _ := json.Marshal(behavioral.TailgatingParams{
			Line: vertLine, WindowSeconds: 5.0,
		})
		fallJSON, _ := json.Marshal(behavioral.FallParams{
			HeightDropFraction: 0.40, WindowSeconds: 0.5,
		})

		cfgs := []behavioral.DetectorConfig{
			{
				ID: "loi", TenantID: tenantID, CameraID: cameraID,
				Type: behavioral.DetectorTypeLoitering, Enabled: true, Params: loiJSON,
			},
			{
				ID: "lc", TenantID: tenantID, CameraID: cameraID,
				Type: behavioral.DetectorTypeLineCrossing, Enabled: true, Params: lcJSON,
			},
			{
				ID: "roi", TenantID: tenantID, CameraID: cameraID,
				Type: behavioral.DetectorTypeROI, Enabled: true, Params: roiJSON,
			},
			{
				ID: "cd", TenantID: tenantID, CameraID: cameraID,
				Type: behavioral.DetectorTypeCrowdDensity, Enabled: true, Params: cdJSON,
			},
			{
				ID: "tg", TenantID: tenantID, CameraID: cameraID,
				Type: behavioral.DetectorTypeTailgating, Enabled: true, Params: tgJSON,
			},
			{
				ID: "fall", TenantID: tenantID, CameraID: cameraID,
				Type: behavioral.DetectorTypeFall, Enabled: true, Params: fallJSON,
			},
		}

		p, err := behavioral.BuildPipeline(tenantID, cameraID, cfgs, nil)
		if err != nil {
			b.Fatalf("BuildPipeline: %v", err)
		}
		pipelines[i] = p
	}

	b.Cleanup(func() {
		for _, p := range pipelines {
			p.Close()
			drainPipeline(p)
		}
	})

	// Pre-build a representative frame.
	dets := make([]behavioral.Detection, numDetectionsPerFrame)
	for j := range numDetectionsPerFrame {
		dets[j] = behavioral.Detection{
			TrackID: int64(j + 1),
			Class:   "person",
			Box:     behavioral.BoundingBox{X1: 0.3, Y1: 0.3, X2: 0.5, Y2: 0.7},
		}
	}

	ctx := context.Background()
	frameTS := time.Now()
	frameInterval := time.Second / 60 // 60 FPS

	b.ResetTimer()
	b.ReportAllocs()

	for n := range b.N {
		ts := frameTS.Add(time.Duration(n) * frameInterval)
		for i, p := range pipelines {
			tenantID := fmt.Sprintf("tenant-%d", i%4)
			cameraID := fmt.Sprintf("cam-%d", i)
			f := behavioral.DetectionFrame{
				TenantID:   tenantID,
				CameraID:   cameraID,
				FrameID:    uint64(n),
				Timestamp:  ts,
				Detections: dets,
			}
			p.Feed(ctx, f)
		}
	}
}

// BenchmarkSingleDetector_Loitering benchmarks the loitering detector in
// isolation at 60 FPS with 10 tracked persons.
func BenchmarkSingleDetector_Loitering(b *testing.B) {
	cfg := behavioral.LoiteringParams{ROI: square, ThresholdSeconds: 30.0}
	d := behavioral.NewLoiteringDetector("bench", "t", "c", cfg, nil)
	b.Cleanup(func() {
		d.Close()
		for e := range d.Events() { //nolint:revive
			_ = e
		}
	})

	dets := []behavioral.Detection{
		det(1, 0.3, 0.3, 0.5, 0.7),
		det(2, 0.4, 0.4, 0.6, 0.8),
	}
	ctx := context.Background()
	ts := time.Now()

	b.ResetTimer()
	b.ReportAllocs()
	for n := range b.N {
		d.Feed(ctx, behavioral.DetectionFrame{
			TenantID:   "t",
			CameraID:   "c",
			FrameID:    uint64(n),
			Timestamp:  ts.Add(time.Duration(n) * 16 * time.Millisecond),
			Detections: dets,
		})
	}
}
