# AI Pipeline Benchmark Audit (KAI-39)

**Date:** 2026-04-03
**Machine:** Apple M1, 8 cores (GOMAXPROCS=8), macOS (Darwin 25.3.0)
**Go version:** 1.25.0

## Methodology

The benchmark simulates N concurrent camera pipelines, each generating
detection frames at 15 FPS with 3-4 synthetic YOLO detections per frame. Each
pipeline runs the real **Tracker** stage and a lightweight **Publisher** stub
(no database writes, no ONNX embedder). The **Detector** stage is replaced with
synthetic detections since ONNX Runtime is not available in CI/test environments.

This measures pipeline orchestration overhead, channel backpressure, tracker
throughput, memory scaling, and goroutine management -- everything except ONNX
inference latency.

Each concurrency level runs for 3 seconds. Memory scaling is verified separately.

## Load Test Results

| Cameras | Frames Sent | Frames Received | Drop Rate | Avg Latency | Max Latency | Total Alloc | Goroutines |
| ------- | ----------- | --------------- | --------- | ----------- | ----------- | ----------- | ---------- |
| 8       | 360         | 368             | 0%        | 36 us       | 880 us      | 10.4 MB     | 2          |
| 16      | 720         | 736             | 0%        | 64 us       | 10.2 ms     | 20.8 MB     | 2          |
| 32      | 1,440       | 1,472           | 0%        | 33 us       | 20.1 ms     | 41.6 MB     | 2          |

Note: "Frames Received" can exceed "Frames Sent" because the tracker emits
`ObjectLeft` events after the producer stops, generating additional tracked
frames from the drain phase.

### Key Observations

- **Zero frame drops** at all concurrency levels. The channel-based
  backpressure with drop-oldest strategy (buffer size 1) works effectively
  when the tracker can keep up.
- **Latency stays sub-millisecond on average.** Average latency is 33-64 us
  across all stream counts. Even the worst-case max latency at 32 cameras is
  only 20.1 ms, well within real-time requirements.
- **Memory scales linearly.** Total allocations grow from 10.4 MB (8 cameras) to
  41.6 MB (32 cameras) -- a 4.00x ratio for 4x the cameras, confirming
  O(n) scaling with no memory leaks.
- **Goroutine cleanup is correct.** After each benchmark level completes, only 2
  goroutines remain (the test goroutine and the runtime finalizer).

## Memory Scaling Verification

| Cameras | Total Alloc | Ratio vs 8-cam |
| ------- | ----------- | -------------- |
| 8       | 9.57 MB     | 1.00x          |
| 16      | 19.14 MB    | 2.00x          |
| 32      | 38.27 MB    | 4.00x          |

Scaling ratio 32/8 = **4.00x** (perfect linear scaling, well under the 6x
threshold for detecting super-linear growth).

## Frame Drop Behavior Test

Under extreme pressure (100 frames sent as fast as possible, consumer reading
at 100 FPS), the drop-oldest channel pattern works correctly:

- All 100 frames sent without producer blocking
- 99 frames dropped (expected -- consumer only has time to read 1)
- The key invariant holds: **the producer never blocks**

## Component Micro-Benchmarks

| Component           | Time/op | Allocs/op | Bytes/op | Notes                     |
| ------------------- | ------- | --------- | -------- | ------------------------- |
| Tracker (per frame) | 61.9 us | 22        | 11,610 B | IoU matching + track mgmt |
| IoU computation     | 4.1 ns  | 0         | 0 B      | Zero allocation hot path  |

## Bottleneck Analysis

1. **ONNX Inference (not measured -- production bottleneck):** In production,
   YOLO inference is the dominant cost. On CPU, YOLOv8n inference is typically
   30-80 ms per frame. This limits a single detector instance to ~12-33 FPS.
   With one detector per camera, 32 cameras at 15 FPS would require the ONNX
   runtime to handle 480 inferences/second.

2. **Image preprocessing (~12 ms/frame from previous profiling):** The
   `preprocess()` method in `detector.go` resizes input images to 640x640 and
   converts to CHW float32. Uses `image.At()` per pixel which causes interface
   dispatch overhead. Could be optimized 5-10x by working directly on
   `NRGBA.Pix` byte slices.

3. **Tracker (61.9 us/frame):** Low cost. The greedy IoU matching scales well.
   At 32 cameras and 15 FPS, tracker processing consumes only ~30 ms/second
   total across all cameras.

4. **Channel backpressure:** Buffer-1 channels with drop-oldest work correctly.
   No deadlocks, stalls, or producer blocking observed.

5. **Memory:** ~1.2 MB per camera for pipeline state is very manageable. The
   primary memory consumer in production will be the ONNX session tensors
   (~50-100 MB per session, not measured here).

## Maximum Sustainable Camera Count (Estimates)

| Scenario                        | Max Cameras at 15 FPS | Limiting Factor             |
| ------------------------------- | --------------------- | --------------------------- |
| Pipeline only (no ONNX)         | 100+                  | OS scheduler, GC pressure   |
| CPU ONNX (YOLOv8n, ~50ms/frame) | ~20                   | ONNX inference throughput   |
| CPU ONNX + shared detector pool | ~30-40                | Pool contention + CPU cores |
| GPU ONNX (if available)         | ~40-60                | GPU memory + batch size     |

## Recommendations

1. **Share the ONNX detector across cameras** with a worker pool (e.g., 2-4
   workers) rather than one session per camera. ONNX sessions are not
   thread-safe, but a pool of N sessions can serve M cameras where M >> N.

2. **Optimize image preprocessing** by replacing `image.At()` per-pixel access
   with direct `NRGBA.Pix` byte slice operations. Expected 5-10x improvement.

3. **Add adaptive frame skipping** based on inference backlog. If the detector
   queue depth exceeds a threshold, skip frames to maintain real-time tracking.

4. **Consider batched inference** if GPU is available. Batching 4-8 frames in a
   single ONNX call can improve GPU utilization significantly.

5. **Add runtime metrics** (Prometheus or internal counters) for: active
   pipelines, frames/sec, drop rate, inference latency percentiles. The
   benchmark test infrastructure can serve as the basis for these metrics.

## How to Run

```bash
# Full load test at 8, 16, 32 cameras (takes ~14 seconds)
go test -v -run 'TestBenchmarkPipeline|TestFrameDropBehavior|TestMemoryScaling' \
    -timeout 120s ./internal/nvr/ai/

# Component micro-benchmarks
go test -bench=. -benchmem -run='^$' ./internal/nvr/ai/
```
