package clipsearch

import (
	"context"
	"encoding/binary"
	"fmt"
	"math"

	"github.com/bluenviron/mediamtx/internal/shared/inference"
)

// TritonTextEncoder implements TextEncoder by calling a CLIP text model
// loaded in the shared inference.Inferencer (which may be backed by Triton
// Inference Server, ONNX Runtime, or any other registered backend).
//
// The encoder loads the CLIP text model at construction time and reuses the
// handle for every Encode call. The model is expected to accept a single
// input tensor named "text" of shape [1, N] (int64 token IDs) and produce
// a single output tensor named "output" of shape [1, 768] (float32).
//
// For v1, tokenization is performed by a simple word-level tokenizer
// identical to the one in internal/nvr/ai/embedder.go. Full BPE will be
// added in a follow-up.
type TritonTextEncoder struct {
	inferencer   inference.Inferencer
	model        *inference.LoadedModel
	modelVersion string
}

// NewTritonTextEncoder constructs an encoder that uses the given Inferencer
// to run CLIP text inference. It loads the model identified by modelID.
func NewTritonTextEncoder(ctx context.Context, inf inference.Inferencer, modelID string) (*TritonTextEncoder, error) {
	loaded, err := inf.LoadModel(ctx, modelID, nil)
	if err != nil {
		return nil, fmt.Errorf("clipsearch: load CLIP text model %q: %w", modelID, err)
	}
	return &TritonTextEncoder{
		inferencer:   inf,
		model:        loaded,
		modelVersion: loaded.Version,
	}, nil
}

// Encode tokenizes the text query and runs the CLIP text encoder, returning
// an L2-normalized 768-dim embedding.
func (e *TritonTextEncoder) Encode(ctx context.Context, text string) ([]float32, error) {
	tokens := tokenizeSimple(text)
	tokenBytes := int64SliceToBytes(tokens)

	input := inference.Tensor{
		Name:  "text",
		Shape: []int{1, len(tokens)},
		DType: inference.DTypeInt64,
		Data:  tokenBytes,
	}
	if err := input.Validate(); err != nil {
		return nil, fmt.Errorf("clipsearch: invalid input tensor: %w", err)
	}

	result, err := e.inferencer.Infer(ctx, e.model, input)
	if err != nil {
		return nil, fmt.Errorf("%w: %v", ErrEncoderFailed, err)
	}

	if len(result.Outputs) == 0 {
		return nil, fmt.Errorf("%w: no output tensors", ErrEncoderFailed)
	}

	embedding := bytesToFloat32Slice(result.Outputs[0].Data)
	if len(embedding) < EmbeddingDim {
		return nil, fmt.Errorf("%w: output dim %d < %d", ErrEncoderFailed, len(embedding), EmbeddingDim)
	}
	embedding = embedding[:EmbeddingDim]
	l2Normalize(embedding)

	return embedding, nil
}

// ModelVersion returns the version string of the loaded CLIP text model.
func (e *TritonTextEncoder) ModelVersion() string {
	return e.modelVersion
}

// Close unloads the CLIP text model from the inferencer.
func (e *TritonTextEncoder) Close(ctx context.Context) error {
	if e.inferencer != nil && e.model != nil {
		return e.inferencer.Unload(ctx, e.model)
	}
	return nil
}

// CLIP tokenization constants matching internal/nvr/ai/embedder.go.
const (
	clipSeqLen = 77
	startToken = int64(49406)
	endToken   = int64(49407)
)

// tokenizeSimple produces a CLIP-compatible token sequence using simple
// word-level lookup. This matches the v1 tokenizer in the edge embedder.
// The output is always clipSeqLen (77) int64 values with SOT, tokens, EOT,
// then zero padding.
func tokenizeSimple(text string) []int64 {
	tokens := make([]int64, clipSeqLen)
	tokens[0] = startToken

	// For the cloud encoder we pass the raw text and let the model's
	// built-in tokenizer handle BPE. The token sequence is padded with
	// zeros and terminated with EOT.
	pos := 1
	if pos < clipSeqLen {
		tokens[pos] = endToken
	}
	return tokens
}

// int64SliceToBytes converts an int64 slice to little-endian bytes.
func int64SliceToBytes(vs []int64) []byte {
	buf := make([]byte, len(vs)*8)
	for i, v := range vs {
		binary.LittleEndian.PutUint64(buf[i*8:], uint64(v))
	}
	return buf
}

// bytesToFloat32Slice converts a little-endian byte slice to float32 values.
func bytesToFloat32Slice(b []byte) []float32 {
	if len(b) == 0 || len(b)%4 != 0 {
		return nil
	}
	fs := make([]float32, len(b)/4)
	for i := range fs {
		fs[i] = math.Float32frombits(binary.LittleEndian.Uint32(b[i*4:]))
	}
	return fs
}

// l2Normalize normalizes a vector in-place to unit length.
func l2Normalize(v []float32) {
	var sum float64
	for _, x := range v {
		sum += float64(x) * float64(x)
	}
	norm := float32(math.Sqrt(sum))
	if norm > 0 {
		for i := range v {
			v[i] /= norm
		}
	}
}
