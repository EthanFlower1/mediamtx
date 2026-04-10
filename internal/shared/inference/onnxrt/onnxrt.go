//go:build cgo

// Package onnxrt implements inference.Inferencer backed by ONNX Runtime
// via github.com/yalue/onnxruntime_go. It supports CPU and CUDA execution
// providers, concurrent Infer calls, and per-model latency tracking.
//
// This is the CGO build — the no-cgo stub lives in onnxrt_nocgo.go.
package onnxrt

import (
	"context"
	"crypto/sha256"
	"encoding/binary"
	"fmt"
	"log/slog"
	"math"
	"os"
	"path/filepath"
	"sort"
	"sync"
	"sync/atomic"
	"time"

	ort "github.com/yalue/onnxruntime_go"

	"github.com/bluenviron/mediamtx/internal/shared/inference"
)

// Option configures an Inferencer at construction time.
type Option func(*Inferencer)

// WithName sets the human-readable name returned by Name().
func WithName(n string) Option { return func(i *Inferencer) { i.name = n } }

// WithRegistry wires a ModelRegistry for LoadModel to consult when the
// caller passes nil bytes.
func WithRegistry(r inference.ModelRegistry) Option {
	return func(i *Inferencer) { i.registry = r }
}

// WithLibraryPath overrides the ONNX Runtime shared library search path.
// If empty, standard OS locations are probed.
func WithLibraryPath(p string) Option {
	return func(i *Inferencer) { i.libPath = p }
}

// WithGPU enables the CUDA execution provider. Falls back to CPU if
// CUDA is unavailable.
func WithGPU(enable bool) Option {
	return func(i *Inferencer) { i.useGPU = enable }
}

// WithLogger sets the slog.Logger.
func WithLogger(l *slog.Logger) Option {
	return func(i *Inferencer) { i.logger = l }
}

// Inferencer wraps ONNX Runtime sessions behind the inference.Inferencer
// interface. Thread-safe for concurrent Infer calls.
type Inferencer struct {
	name     string
	libPath  string
	useGPU   bool
	registry inference.ModelRegistry
	logger   *slog.Logger

	mu     sync.RWMutex
	closed bool
	models map[string]*modelEntry

	totalInf atomic.Uint64
	totalErr atomic.Uint64
}

type modelEntry struct {
	id          string
	version     string
	session     *ort.DynamicAdvancedSession
	sessionOpts *ort.SessionOptions
	loadedAt    time.Time
	tmpFile     string // on-disk temp file for the model bytes

	mu         sync.Mutex
	inferCount uint64
	errCount   uint64
	latencies  []time.Duration
}

const latencyWindow = 256

// New creates and initialises an ONNX Runtime Inferencer. The runtime
// library is loaded once per process (ort.SetSharedLibraryPath is
// idempotent).
func New(opts ...Option) (*Inferencer, error) {
	i := &Inferencer{
		name:   "onnxrt",
		models: make(map[string]*modelEntry),
		logger: slog.Default(),
	}
	for _, o := range opts {
		o(i)
	}

	if err := i.initRuntime(); err != nil {
		return nil, fmt.Errorf("onnxrt: init runtime: %w", err)
	}

	return i, nil
}

func (i *Inferencer) initRuntime() error {
	if i.libPath != "" {
		ort.SetSharedLibraryPath(i.libPath)
	} else {
		home, _ := os.UserHomeDir()
		candidates := []string{
			filepath.Join(home, "lib", "libonnxruntime.dylib"),
			"/usr/local/lib/libonnxruntime.dylib",
			"/usr/lib/libonnxruntime.so",
			"/usr/local/lib/libonnxruntime.so",
			"/usr/lib/x86_64-linux-gnu/libonnxruntime.so",
		}
		for _, p := range candidates {
			if _, err := os.Stat(p); err == nil {
				ort.SetSharedLibraryPath(p)
				break
			}
		}
	}
	return ort.InitializeEnvironment()
}

// Name implements inference.Inferencer.
func (i *Inferencer) Name() string { return i.name }

// Backend implements inference.Inferencer.
func (i *Inferencer) Backend() inference.BackendKind { return inference.BackendONNXRuntime }

// LoadModel implements inference.Inferencer. It writes model bytes to a
// temporary file (ONNX Runtime requires a file path), creates a session,
// and returns a LoadedModel handle.
func (i *Inferencer) LoadModel(ctx context.Context, modelID string, bytes []byte) (*inference.LoadedModel, error) {
	if modelID == "" {
		return nil, fmt.Errorf("onnxrt: empty model id")
	}

	i.mu.Lock()
	defer i.mu.Unlock()
	if i.closed {
		return nil, inference.ErrClosed
	}

	version := ""
	if bytes == nil {
		if i.registry == nil {
			return nil, fmt.Errorf("%w: no bytes and no registry", inference.ErrModelNotFound)
		}
		b, v, err := i.registry.Resolve(ctx, modelID)
		if err != nil {
			return nil, err
		}
		bytes = b
		version = v
	}

	// Write bytes to a temp file — ONNX Runtime needs a file path.
	tmpDir := os.TempDir()
	hash := sha256.Sum256(bytes)
	tmpPath := filepath.Join(tmpDir, fmt.Sprintf("onnxrt-%s-%x.onnx", modelID, hash[:8]))
	if err := os.WriteFile(tmpPath, bytes, 0600); err != nil {
		return nil, fmt.Errorf("onnxrt: write temp model: %w", err)
	}

	// Create ONNX Runtime dynamic session. DynamicAdvancedSession allows
	// running models without pre-allocated input/output tensors — shapes
	// are resolved at Run time.
	sessionOpts, err := ort.NewSessionOptions()
	if err != nil {
		_ = os.Remove(tmpPath)
		return nil, fmt.Errorf("onnxrt: session options: %w", err)
	}

	if i.useGPU {
		if cudaErr := sessionOpts.AppendExecutionProviderCUDA(nil); cudaErr != nil {
			i.logger.Warn("onnxrt: CUDA EP unavailable, falling back to CPU",
				"model_id", modelID,
				"error", cudaErr,
			)
		}
	}

	// Use standard single-input/single-output naming convention. Models
	// with different I/O names will need name discovery (KAI-278 part 2).
	inputNames := []string{"input"}
	outputNames := []string{"output"}
	session, err := ort.NewDynamicAdvancedSession(tmpPath,
		inputNames, outputNames, sessionOpts)
	if err != nil {
		sessionOpts.Destroy()
		_ = os.Remove(tmpPath)
		return nil, fmt.Errorf("onnxrt: create session for %s: %w", modelID, err)
	}

	handleID := fmt.Sprintf("%s#%d", modelID, len(i.models))
	entry := &modelEntry{
		id:         modelID,
		version:    version,
		session:    session,
		sessionOpts: sessionOpts,
		loadedAt:   time.Now(),
		tmpFile:    tmpPath,
	}
	i.models[handleID] = entry

	i.logger.Info("onnxrt: model loaded",
		"model_id", modelID,
		"handle", handleID,
		"gpu", i.useGPU,
	)

	return &inference.LoadedModel{
		ID:       modelID,
		Backend:  inference.BackendONNXRuntime,
		Version:  version,
		LoadedAt: entry.loadedAt,
		Handle:   handleID,
	}, nil
}

// Infer implements inference.Inferencer.
func (i *Inferencer) Infer(ctx context.Context, model *inference.LoadedModel, input inference.Tensor) (*inference.InferenceResult, error) {
	if model == nil {
		return nil, inference.ErrModelNotLoaded
	}
	if err := input.Validate(); err != nil {
		i.totalErr.Add(1)
		return nil, fmt.Errorf("%w: %v", inference.ErrInvalidTensor, err)
	}

	i.mu.RLock()
	closed := i.closed
	handleID, _ := model.Handle.(string)
	entry, ok := i.models[handleID]
	i.mu.RUnlock()

	if closed {
		return nil, inference.ErrClosed
	}
	if !ok {
		i.totalErr.Add(1)
		return nil, inference.ErrModelNotLoaded
	}

	start := time.Now()

	// Convert inference.Tensor to an ort.Value for the ONNX Runtime session.
	ortInput, err := tensorToOrtValue(input)
	if err != nil {
		i.totalErr.Add(1)
		entry.recordError()
		return nil, fmt.Errorf("onnxrt: convert input tensor: %w", err)
	}
	defer ortInput.Destroy()

	// Pass nil for output — ONNX Runtime auto-allocates and we extract
	// the result from the returned slice.
	outputs := []ort.Value{nil}
	if err := entry.session.Run([]ort.Value{ortInput}, outputs); err != nil {
		i.totalErr.Add(1)
		entry.recordError()
		return nil, fmt.Errorf("onnxrt: run %s: %w", model.ID, err)
	}

	latency := time.Since(start)
	entry.recordInference(latency)
	i.totalInf.Add(1)

	// Extract the output tensor. If ONNX Runtime auto-allocated it, we
	// must destroy it after copying the data out.
	result := &inference.InferenceResult{
		Latency: latency,
		ModelID: model.ID,
		Backend: inference.BackendONNXRuntime,
	}

	if outputs[0] != nil {
		outShape := outputs[0].GetShape()
		shape := make([]int, len(outShape))
		numElem := 1
		for idx, d := range outShape {
			shape[idx] = int(d)
			numElem *= int(d)
		}
		// Copy raw bytes out of the ORT value. For now assume float32.
		outData := make([]byte, numElem*4)
		if t, ok := outputs[0].(*ort.Tensor[float32]); ok {
			floats := t.GetData()
			for idx, f := range floats {
				binary.LittleEndian.PutUint32(outData[idx*4:], math.Float32bits(f))
			}
		}
		result.Outputs = []inference.Tensor{{
			Name:  "output",
			Shape: shape,
			DType: inference.DTypeFloat32,
			Data:  outData,
		}}
		outputs[0].Destroy()
	} else {
		result.Outputs = []inference.Tensor{{
			Name:  "output",
			Shape: []int{1},
			DType: inference.DTypeFloat32,
			Data:  make([]byte, 4),
		}}
	}

	return result, nil
}

// Unload implements inference.Inferencer.
func (i *Inferencer) Unload(ctx context.Context, model *inference.LoadedModel) error {
	if model == nil {
		return nil
	}

	i.mu.Lock()
	defer i.mu.Unlock()
	if i.closed {
		return inference.ErrClosed
	}

	handleID, _ := model.Handle.(string)
	entry, ok := i.models[handleID]
	if !ok {
		return nil
	}

	entry.session.Destroy()
	entry.sessionOpts.Destroy()
	_ = os.Remove(entry.tmpFile)
	delete(i.models, handleID)

	i.logger.Debug("onnxrt: model unloaded", "model_id", model.ID, "handle", handleID)
	return nil
}

// Stats implements inference.Inferencer.
func (i *Inferencer) Stats() inference.Stats {
	i.mu.RLock()
	defer i.mu.RUnlock()

	perModel := make(map[string]inference.ModelStats, len(i.models))
	for _, e := range i.models {
		e.mu.Lock()
		stats := inference.ModelStats{
			ModelID:        e.id,
			InferenceCount: e.inferCount,
			ErrorCount:     e.errCount,
			Latency:        percentiles(e.latencies),
		}
		e.mu.Unlock()

		if existing, ok := perModel[e.id]; ok {
			existing.InferenceCount += stats.InferenceCount
			existing.ErrorCount += stats.ErrorCount
			if stats.Latency.P99 > existing.Latency.P99 {
				existing.Latency = stats.Latency
			}
			perModel[e.id] = existing
		} else {
			perModel[e.id] = stats
		}
	}

	return inference.Stats{
		Backend:         inference.BackendONNXRuntime,
		ModelsLoaded:    len(i.models),
		TotalInferences: i.totalInf.Load(),
		TotalErrors:     i.totalErr.Load(),
		PerModel:        perModel,
	}
}

// Close implements inference.Inferencer.
func (i *Inferencer) Close() error {
	i.mu.Lock()
	defer i.mu.Unlock()
	if i.closed {
		return nil
	}
	i.closed = true

	for _, e := range i.models {
		e.session.Destroy()
		e.sessionOpts.Destroy()
		_ = os.Remove(e.tmpFile)
	}
	i.models = make(map[string]*modelEntry)

	i.logger.Info("onnxrt: closed")
	return nil
}

func (e *modelEntry) recordInference(latency time.Duration) {
	e.mu.Lock()
	defer e.mu.Unlock()
	e.inferCount++
	if len(e.latencies) >= latencyWindow {
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

// tensorToOrtValue converts an inference.Tensor (byte buffer) into an
// ort.Value suitable for passing to DynamicAdvancedSession.Run.
// Currently supports float32 only; other dtypes will be added in KAI-278 part 2.
func tensorToOrtValue(t inference.Tensor) (ort.Value, error) {
	shape := make(ort.Shape, len(t.Shape))
	for idx, d := range t.Shape {
		shape[idx] = int64(d)
	}

	switch t.DType {
	case inference.DTypeFloat32:
		numElem := t.NumElements()
		floats := make([]float32, numElem)
		for idx := 0; idx < numElem; idx++ {
			bits := binary.LittleEndian.Uint32(t.Data[idx*4:])
			floats[idx] = math.Float32frombits(bits)
		}
		tensor, err := ort.NewTensor(shape, floats)
		if err != nil {
			return nil, fmt.Errorf("create float32 tensor: %w", err)
		}
		return tensor, nil
	case inference.DTypeInt64:
		numElem := t.NumElements()
		vals := make([]int64, numElem)
		for idx := 0; idx < numElem; idx++ {
			vals[idx] = int64(binary.LittleEndian.Uint64(t.Data[idx*8:]))
		}
		tensor, err := ort.NewTensor(shape, vals)
		if err != nil {
			return nil, fmt.Errorf("create int64 tensor: %w", err)
		}
		return tensor, nil
	case inference.DTypeUint8:
		data := make([]uint8, len(t.Data))
		copy(data, t.Data)
		tensor, err := ort.NewTensor(shape, data)
		if err != nil {
			return nil, fmt.Errorf("create uint8 tensor: %w", err)
		}
		return tensor, nil
	default:
		return nil, fmt.Errorf("unsupported dtype %s for ONNX Runtime", t.DType)
	}
}

func percentiles(samples []time.Duration) inference.LatencyStats {
	if len(samples) == 0 {
		return inference.LatencyStats{}
	}
	sorted := make([]time.Duration, len(samples))
	copy(sorted, samples)
	sort.Slice(sorted, func(i, j int) bool { return sorted[i] < sorted[j] })
	pick := func(p float64) time.Duration {
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

// Compile-time assertions.
var _ inference.Inferencer = (*Inferencer)(nil)
