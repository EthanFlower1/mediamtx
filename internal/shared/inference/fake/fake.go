// Package fake implements a deterministic in-memory Inferencer used by
// tests and by feature tickets that want to exercise their AI pipeline
// without waiting for a real ONNX Runtime / TensorRT / Core ML / DirectML
// backend to ship.
//
// The fake is intentionally not a stub: it supports LoadModel, produces
// reproducible outputs seeded from modelID + input bytes, tracks stats, is
// safe for concurrent Infer, and honours Close. It does NOT do any real
// math; callers that need real predictions must use a real backend.
package fake

import (
	"context"
	"crypto/sha256"
	"encoding/binary"
	"fmt"
	"math"
	"sort"
	"sync"
	"sync/atomic"
	"time"

	"github.com/bluenviron/mediamtx/internal/shared/inference"
)

// OutputMode controls how Infer synthesises output bytes from inputs.
type OutputMode int

const (
	// OutputEcho returns the input bytes unchanged (shape-preserving).
	OutputEcho OutputMode = iota

	// OutputOnes returns a tensor of the same shape as the input, every
	// element set to 1 (in the input's dtype).
	OutputOnes

	// OutputDeterministicRandom returns a tensor of the same shape as the
	// input, filled with pseudo-random bytes seeded from the model id
	// and a hash of the input. Reproducible across runs.
	OutputDeterministicRandom
)

// Option customises an Inferencer at construction.
type Option func(*Inferencer)

// WithLatency injects a simulated latency into every Infer call. Useful
// for tests that want to exercise timeout / backpressure paths.
func WithLatency(d time.Duration) Option {
	return func(f *Inferencer) { f.latency = d }
}

// WithOutputMode overrides the default OutputEcho behaviour.
func WithOutputMode(m OutputMode) Option {
	return func(f *Inferencer) { f.outputMode = m }
}

// WithName sets the human-readable instance name returned by Name().
func WithName(n string) Option {
	return func(f *Inferencer) { f.name = n }
}

// WithRegistry wires up an inference.ModelRegistry that LoadModel will
// consult when the caller passes nil bytes.
func WithRegistry(r inference.ModelRegistry) Option {
	return func(f *Inferencer) { f.registry = r }
}

// Inferencer is the deterministic in-memory fake. It implements
// inference.Inferencer.
type Inferencer struct {
	name       string
	outputMode OutputMode
	latency    time.Duration
	registry   inference.ModelRegistry

	mu     sync.RWMutex
	closed bool
	models map[string]*modelEntry // keyed by handle-id

	totalInf atomic.Uint64
	totalErr atomic.Uint64
}

type modelEntry struct {
	id       string
	version  string
	bytes    []byte
	loadedAt time.Time

	mu         sync.Mutex
	inferCount uint64
	errCount   uint64
	// latencies is a bounded reservoir of observed latencies (ns). We
	// keep up to latencyWindow samples and compute percentiles by
	// sorting a copy.
	latencies []time.Duration
}

const latencyWindow = 256

// New constructs a fake Inferencer.
func New(opts ...Option) *Inferencer {
	f := &Inferencer{
		name:       "fake",
		outputMode: OutputEcho,
		models:     make(map[string]*modelEntry),
	}
	for _, o := range opts {
		o(f)
	}
	return f
}

// Name implements inference.Inferencer.
func (f *Inferencer) Name() string { return f.name }

// Backend implements inference.Inferencer.
func (f *Inferencer) Backend() inference.BackendKind { return inference.BackendFake }

// LoadModel implements inference.Inferencer. It accepts any bytes; if
// bytes is nil and a registry is configured, the registry is consulted.
func (f *Inferencer) LoadModel(ctx context.Context, modelID string, bytes []byte) (*inference.LoadedModel, error) {
	if modelID == "" {
		return nil, fmt.Errorf("fake: empty model id")
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.closed {
		return nil, inference.ErrClosed
	}

	version := ""
	if bytes == nil {
		if f.registry == nil {
			return nil, fmt.Errorf("%w: no bytes and no registry", inference.ErrModelNotFound)
		}
		b, v, err := f.registry.Resolve(ctx, modelID)
		if err != nil {
			return nil, err
		}
		bytes = b
		version = v
	}

	// The handle id embeds modelID plus a counter to allow the same
	// model id to be loaded multiple times concurrently.
	handleID := fmt.Sprintf("%s#%d", modelID, len(f.models))
	entry := &modelEntry{
		id:       modelID,
		version:  version,
		bytes:    append([]byte(nil), bytes...),
		loadedAt: time.Now(),
	}
	f.models[handleID] = entry

	return &inference.LoadedModel{
		ID:       modelID,
		Backend:  inference.BackendFake,
		Version:  version,
		LoadedAt: entry.loadedAt,
		Handle:   handleID,
	}, nil
}

// Infer implements inference.Inferencer.
func (f *Inferencer) Infer(ctx context.Context, model *inference.LoadedModel, input inference.Tensor) (*inference.InferenceResult, error) {
	if model == nil {
		return nil, inference.ErrModelNotLoaded
	}
	if err := input.Validate(); err != nil {
		f.totalErr.Add(1)
		return nil, fmt.Errorf("%w: %v", inference.ErrInvalidTensor, err)
	}

	f.mu.RLock()
	closed := f.closed
	handleID, _ := model.Handle.(string)
	entry, ok := f.models[handleID]
	f.mu.RUnlock()

	if closed {
		return nil, inference.ErrClosed
	}
	if !ok {
		f.totalErr.Add(1)
		return nil, inference.ErrModelNotLoaded
	}

	start := time.Now()
	if f.latency > 0 {
		select {
		case <-time.After(f.latency):
		case <-ctx.Done():
			f.totalErr.Add(1)
			entry.recordError()
			return nil, ctx.Err()
		}
	}

	out := f.synthesiseOutput(model.ID, input)
	latency := time.Since(start)

	entry.recordInference(latency)
	f.totalInf.Add(1)

	return &inference.InferenceResult{
		Outputs: []inference.Tensor{out},
		Latency: latency,
		ModelID: model.ID,
		Backend: inference.BackendFake,
	}, nil
}

// synthesiseOutput produces a deterministic output tensor of the same
// shape + dtype as the input, according to f.outputMode.
func (f *Inferencer) synthesiseOutput(modelID string, input inference.Tensor) inference.Tensor {
	out := inference.Tensor{
		Name:  "output",
		Shape: append([]int(nil), input.Shape...),
		DType: input.DType,
		Data:  make([]byte, len(input.Data)),
	}
	switch f.outputMode {
	case OutputEcho:
		copy(out.Data, input.Data)
	case OutputOnes:
		writeOnes(out.Data, input.DType, input.NumElements())
	case OutputDeterministicRandom:
		writeDeterministicRandom(modelID, input.Data, out.Data)
	default:
		copy(out.Data, input.Data)
	}
	return out
}

func writeOnes(buf []byte, dt inference.DType, n int) {
	sz := dt.ElementSize()
	if sz == 0 {
		return
	}
	for i := 0; i < n; i++ {
		off := i * sz
		switch dt {
		case inference.DTypeFloat32:
			binary.LittleEndian.PutUint32(buf[off:], math.Float32bits(1))
		case inference.DTypeInt32:
			binary.LittleEndian.PutUint32(buf[off:], 1)
		case inference.DTypeInt64:
			binary.LittleEndian.PutUint64(buf[off:], 1)
		case inference.DTypeFloat16:
			// 0x3C00 = 1.0 in IEEE 754 half precision.
			binary.LittleEndian.PutUint16(buf[off:], 0x3C00)
		case inference.DTypeBool, inference.DTypeInt8, inference.DTypeUint8:
			buf[off] = 1
		}
	}
}

// writeDeterministicRandom fills dst with bytes derived from SHA-256 of
// (modelID || inputBytes). The mapping is a fixed function so tests can
// assert reproducibility.
func writeDeterministicRandom(modelID string, input, dst []byte) {
	seed := sha256.New()
	_, _ = seed.Write([]byte(modelID))
	_, _ = seed.Write(input)
	block := seed.Sum(nil)

	// Stream block bytes, rehashing with a counter for buffers longer
	// than 32 bytes.
	var counter uint32
	off := 0
	for off < len(dst) {
		n := copy(dst[off:], block)
		off += n
		counter++
		h := sha256.New()
		_, _ = h.Write(block)
		var ctrBytes [4]byte
		binary.LittleEndian.PutUint32(ctrBytes[:], counter)
		_, _ = h.Write(ctrBytes[:])
		block = h.Sum(nil)
	}
}

// Unload implements inference.Inferencer.
func (f *Inferencer) Unload(ctx context.Context, model *inference.LoadedModel) error {
	if model == nil {
		return nil
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.closed {
		return inference.ErrClosed
	}
	handleID, _ := model.Handle.(string)
	delete(f.models, handleID)
	return nil
}

// Stats implements inference.Inferencer.
func (f *Inferencer) Stats() inference.Stats {
	f.mu.RLock()
	defer f.mu.RUnlock()

	perModel := make(map[string]inference.ModelStats, len(f.models))
	for _, e := range f.models {
		e.mu.Lock()
		stats := inference.ModelStats{
			ModelID:        e.id,
			InferenceCount: e.inferCount,
			ErrorCount:     e.errCount,
			Latency:        percentiles(e.latencies),
		}
		e.mu.Unlock()
		// If the same model id is loaded multiple times we aggregate
		// counts across handles.
		if existing, ok := perModel[e.id]; ok {
			existing.InferenceCount += stats.InferenceCount
			existing.ErrorCount += stats.ErrorCount
			// Keep the higher-percentile numbers as a simple merge.
			if stats.Latency.P99 > existing.Latency.P99 {
				existing.Latency = stats.Latency
			}
			perModel[e.id] = existing
		} else {
			perModel[e.id] = stats
		}
	}

	return inference.Stats{
		Backend:         inference.BackendFake,
		ModelsLoaded:    len(f.models),
		TotalInferences: f.totalInf.Load(),
		TotalErrors:     f.totalErr.Load(),
		PerModel:        perModel,
	}
}

// Close implements inference.Inferencer.
func (f *Inferencer) Close() error {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.closed {
		return nil
	}
	f.closed = true
	f.models = make(map[string]*modelEntry)
	return nil
}

func (e *modelEntry) recordInference(latency time.Duration) {
	e.mu.Lock()
	defer e.mu.Unlock()
	e.inferCount++
	if len(e.latencies) >= latencyWindow {
		// Drop the oldest sample (FIFO) to keep the window bounded.
		copy(e.latencies, e.latencies[1:])
		e.latencies = e.latencies[:len(e.latencies)-1]
	}
	e.latencies = append(e.latencies, latency)
}

func (e *modelEntry) recordError() {
	e.mu.Lock()
	defer e.mu.Unlock()
	e.errCount++
}

// percentiles computes a simple sorted-window P50/P95/P99. For empty input
// it returns zero values.
func percentiles(samples []time.Duration) inference.LatencyStats {
	if len(samples) == 0 {
		return inference.LatencyStats{}
	}
	sorted := make([]time.Duration, len(samples))
	copy(sorted, samples)
	sort.Slice(sorted, func(i, j int) bool { return sorted[i] < sorted[j] })
	pick := func(p float64) time.Duration {
		if len(sorted) == 0 {
			return 0
		}
		idx := int(math.Ceil(p*float64(len(sorted)))) - 1
		if idx < 0 {
			idx = 0
		}
		if idx >= len(sorted) {
			idx = len(sorted) - 1
		}
		return sorted[idx]
	}
	return inference.LatencyStats{
		P50: pick(0.50),
		P95: pick(0.95),
		P99: pick(0.99),
	}
}

// Compile-time assertion.
var _ inference.Inferencer = (*Inferencer)(nil)
