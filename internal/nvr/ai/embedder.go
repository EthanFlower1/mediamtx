package ai

import (
	"encoding/json"
	"fmt"
	"image"
	"math"
	"os"
	"strings"

	ort "github.com/yalue/onnxruntime_go"
	"golang.org/x/image/draw"
)

// Embedder wraps CLIP visual and text ONNX sessions for generating embeddings.
// The visual model outputs 768-dim vectors and the text model outputs 512-dim
// vectors. A learned projection matrix maps visual embeddings into the text
// embedding space so cosine similarity can be computed across modalities.
type Embedder struct {
	visualSession      *ort.AdvancedSession
	visualInputTensor  *ort.Tensor[float32]
	visualOutputTensor *ort.Tensor[float32]
	// We also allocate a tensor for last_hidden_state (required output)
	visualHiddenTensor *ort.Tensor[float32]
	visualDim          int

	textSession      *ort.AdvancedSession
	textIDsTensor    *ort.Tensor[int64]
	textMaskTensor   *ort.Tensor[int64]
	textOutputTensor *ort.Tensor[float32]
	textHiddenTensor *ort.Tensor[float32]
	textDim          int

	// visualProjection is a [textDim x visualDim] matrix that projects
	// 768-dim visual embeddings into the 512-dim text embedding space.
	visualProjection []float32

	vocab map[string]int64
}

const (
	clipImageSize = 224
	clipSeqLen    = 77
	startToken    = int64(49406)
	endToken      = int64(49407)

	// VisualEmbedDim is the output dimension of the CLIP ViT-B/32 visual encoder.
	VisualEmbedDim = 768
	// TextEmbedDim is the output dimension of the CLIP ViT-B/32 text encoder.
	TextEmbedDim = 512
)

// CLIP image normalization constants.
var (
	clipMean = [3]float32{0.48145466, 0.4578275, 0.40821073}
	clipStd  = [3]float32{0.26862954, 0.26130258, 0.27577711}
)

// NewEmbedder creates a CLIP embedder from visual and text ONNX model paths,
// a vocabulary file (clip-vocab.json), and an optional visual projection
// weights file that maps visual embeddings into the text embedding space.
// InitONNXRuntime must be called before creating an embedder.
func NewEmbedder(visualModelPath, textModelPath, vocabPath string, projectionPath ...string) (*Embedder, error) {
	for _, p := range []string{visualModelPath, textModelPath, vocabPath} {
		if _, err := os.Stat(p); err != nil {
			return nil, fmt.Errorf("file not found: %s: %w", p, err)
		}
	}

	// Load vocabulary.
	vocabData, err := os.ReadFile(vocabPath)
	if err != nil {
		return nil, fmt.Errorf("reading vocab file: %w", err)
	}
	var vocab map[string]int64
	if err := json.Unmarshal(vocabData, &vocab); err != nil {
		return nil, fmt.Errorf("parsing vocab file: %w", err)
	}

	e := &Embedder{
		vocab:     vocab,
		visualDim: VisualEmbedDim,
		textDim:   TextEmbedDim,
	}

	// --- Visual session ---
	visualInputShape := ort.NewShape(1, 3, clipImageSize, clipImageSize)
	e.visualInputTensor, err = ort.NewEmptyTensor[float32](visualInputShape)
	if err != nil {
		return nil, fmt.Errorf("creating visual input tensor: %w", err)
	}

	// last_hidden_state: [1, 50, 768]
	visualHiddenShape := ort.NewShape(1, 50, int64(VisualEmbedDim))
	e.visualHiddenTensor, err = ort.NewEmptyTensor[float32](visualHiddenShape)
	if err != nil {
		e.Close()
		return nil, fmt.Errorf("creating visual hidden tensor: %w", err)
	}

	// pooler_output: [1, 768]
	visualOutputShape := ort.NewShape(1, int64(VisualEmbedDim))
	e.visualOutputTensor, err = ort.NewEmptyTensor[float32](visualOutputShape)
	if err != nil {
		e.Close()
		return nil, fmt.Errorf("creating visual output tensor: %w", err)
	}

	e.visualSession, err = ort.NewAdvancedSession(
		visualModelPath,
		[]string{"pixel_values"},
		[]string{"last_hidden_state", "pooler_output"},
		[]ort.ArbitraryTensor{e.visualInputTensor},
		[]ort.ArbitraryTensor{e.visualHiddenTensor, e.visualOutputTensor},
		nil,
	)
	if err != nil {
		e.Close()
		return nil, fmt.Errorf("creating visual ONNX session: %w", err)
	}

	// --- Text session ---
	textInputShape := ort.NewShape(1, clipSeqLen)
	e.textIDsTensor, err = ort.NewEmptyTensor[int64](textInputShape)
	if err != nil {
		e.Close()
		return nil, fmt.Errorf("creating text IDs tensor: %w", err)
	}

	e.textMaskTensor, err = ort.NewEmptyTensor[int64](textInputShape)
	if err != nil {
		e.Close()
		return nil, fmt.Errorf("creating text mask tensor: %w", err)
	}

	// last_hidden_state: [1, 77, 512]
	textHiddenShape := ort.NewShape(1, clipSeqLen, int64(TextEmbedDim))
	e.textHiddenTensor, err = ort.NewEmptyTensor[float32](textHiddenShape)
	if err != nil {
		e.Close()
		return nil, fmt.Errorf("creating text hidden tensor: %w", err)
	}

	// pooler_output: [1, 512]
	textOutputShape := ort.NewShape(1, int64(TextEmbedDim))
	e.textOutputTensor, err = ort.NewEmptyTensor[float32](textOutputShape)
	if err != nil {
		e.Close()
		return nil, fmt.Errorf("creating text output tensor: %w", err)
	}

	e.textSession, err = ort.NewAdvancedSession(
		textModelPath,
		[]string{"input_ids", "attention_mask"},
		[]string{"last_hidden_state", "pooler_output"},
		[]ort.ArbitraryTensor{e.textIDsTensor, e.textMaskTensor},
		[]ort.ArbitraryTensor{e.textHiddenTensor, e.textOutputTensor},
		nil,
	)
	if err != nil {
		e.Close()
		return nil, fmt.Errorf("creating text ONNX session: %w", err)
	}

	// Load visual projection weights if provided.
	if len(projectionPath) > 0 && projectionPath[0] != "" {
		projData, err := os.ReadFile(projectionPath[0])
		if err != nil {
			e.Close()
			return nil, fmt.Errorf("reading visual projection: %w", err)
		}
		expected := e.textDim * e.visualDim * 4 // float32
		if len(projData) != expected {
			e.Close()
			return nil, fmt.Errorf("visual projection size mismatch: got %d bytes, want %d", len(projData), expected)
		}
		e.visualProjection = bytesToFloat32Slice(projData)
	}

	return e, nil
}

// EncodeImage runs the CLIP visual encoder on an image and returns an
// L2-normalized embedding vector (768-dim for ViT-B/32).
func (e *Embedder) EncodeImage(img image.Image) ([]float32, error) {
	e.preprocessImage(img)

	if err := e.visualSession.Run(); err != nil {
		return nil, fmt.Errorf("visual ONNX inference: %w", err)
	}

	output := e.visualOutputTensor.GetData()
	result := make([]float32, e.visualDim)
	copy(result, output[:e.visualDim])

	l2Normalize(result)
	return result, nil
}

// ProjectVisual maps a 768-dim visual embedding into the 512-dim text
// embedding space using the learned projection matrix. Returns nil if no
// projection weights are loaded. The result is L2-normalized.
func (e *Embedder) ProjectVisual(visual []float32) []float32 {
	if e.visualProjection == nil || len(visual) != e.visualDim {
		return nil
	}
	out := make([]float32, e.textDim)
	// Matrix multiply: out[i] = sum_j(projection[i*visualDim+j] * visual[j])
	for i := 0; i < e.textDim; i++ {
		var sum float32
		for j := 0; j < e.visualDim; j++ {
			sum += e.visualProjection[i*e.visualDim+j] * visual[j]
		}
		out[i] = sum
	}
	l2Normalize(out)
	return out
}

// EncodeText runs the CLIP text encoder on a text query and returns an
// L2-normalized embedding vector (512-dim for ViT-B/32).
//
// This uses a simple word-level tokenizer that splits on spaces and looks up
// each word (lowercased, with </w> suffix) in the CLIP vocabulary. This is
// sufficient for v1 search queries like "red car", "person at door".
// Full BPE tokenization can be added later for better accuracy.
func (e *Embedder) EncodeText(text string) ([]float32, error) {
	e.tokenize(text)

	if err := e.textSession.Run(); err != nil {
		return nil, fmt.Errorf("text ONNX inference: %w", err)
	}

	output := e.textOutputTensor.GetData()
	result := make([]float32, e.textDim)
	copy(result, output[:e.textDim])

	l2Normalize(result)
	return result, nil
}

// VisualDim returns the visual embedding dimension.
func (e *Embedder) VisualDim() int {
	return e.visualDim
}

// TextDim returns the text embedding dimension.
func (e *Embedder) TextDim() int {
	return e.textDim
}

// Close releases all ONNX Runtime resources.
func (e *Embedder) Close() {
	if e.visualSession != nil {
		e.visualSession.Destroy()
	}
	if e.visualInputTensor != nil {
		e.visualInputTensor.Destroy()
	}
	if e.visualOutputTensor != nil {
		e.visualOutputTensor.Destroy()
	}
	if e.visualHiddenTensor != nil {
		e.visualHiddenTensor.Destroy()
	}
	if e.textSession != nil {
		e.textSession.Destroy()
	}
	if e.textIDsTensor != nil {
		e.textIDsTensor.Destroy()
	}
	if e.textMaskTensor != nil {
		e.textMaskTensor.Destroy()
	}
	if e.textOutputTensor != nil {
		e.textOutputTensor.Destroy()
	}
	if e.textHiddenTensor != nil {
		e.textHiddenTensor.Destroy()
	}
}

// preprocessImage resizes img to 224x224 and fills the visual input tensor
// in CHW format with CLIP normalization.
func (e *Embedder) preprocessImage(img image.Image) {
	const size = clipImageSize

	dst := image.NewRGBA(image.Rect(0, 0, size, size))
	draw.BiLinear.Scale(dst, dst.Bounds(), img, img.Bounds(), draw.Over, nil)

	data := e.visualInputTensor.GetData()
	chSize := size * size
	for y := 0; y < size; y++ {
		for x := 0; x < size; x++ {
			r, g, b, _ := dst.At(x, y).RGBA()
			idx := y*size + x
			// Convert to [0,1] then normalize with CLIP mean/std.
			data[0*chSize+idx] = (float32(r>>8)/255.0 - clipMean[0]) / clipStd[0]
			data[1*chSize+idx] = (float32(g>>8)/255.0 - clipMean[1]) / clipStd[1]
			data[2*chSize+idx] = (float32(b>>8)/255.0 - clipMean[2]) / clipStd[2]
		}
	}
}

// tokenize fills the text input tensors using a simple word-level tokenizer.
// Format: [SOT, word1, word2, ..., EOT, 0, 0, ...]
// Attention mask: 1 for real tokens, 0 for padding.
func (e *Embedder) tokenize(text string) {
	ids := e.textIDsTensor.GetData()
	mask := e.textMaskTensor.GetData()

	// Clear tensors.
	for i := range ids {
		ids[i] = 0
		mask[i] = 0
	}

	// Start token.
	ids[0] = startToken
	mask[0] = 1

	// Tokenize: lowercase, split on spaces, look up each word with </w> suffix.
	text = strings.ToLower(strings.TrimSpace(text))
	words := strings.Fields(text)

	pos := 1
	for _, word := range words {
		if pos >= clipSeqLen-1 { // Leave room for end token.
			break
		}

		// Try the word with end-of-word suffix first.
		key := word + "</w>"
		if id, ok := e.vocab[key]; ok {
			ids[pos] = id
			mask[pos] = 1
			pos++
			continue
		}

		// If not found, try without suffix (subword token).
		if id, ok := e.vocab[word]; ok {
			ids[pos] = id
			mask[pos] = 1
			pos++
			continue
		}

		// If still not found, try character-by-character as a fallback.
		// For v1, skip unknown words.
	}

	// End token.
	if pos < clipSeqLen {
		ids[pos] = endToken
		mask[pos] = 1
	}
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

// CosineSimilarity computes the cosine similarity between two vectors.
// Both vectors must have the same length. Returns 0 if lengths differ.
func CosineSimilarity(a, b []float32) float64 {
	if len(a) != len(b) || len(a) == 0 {
		return 0
	}

	var dot, normA, normB float64
	for i := range a {
		dot += float64(a[i]) * float64(b[i])
		normA += float64(a[i]) * float64(a[i])
		normB += float64(b[i]) * float64(b[i])
	}

	denom := math.Sqrt(normA) * math.Sqrt(normB)
	if denom == 0 {
		return 0
	}
	return dot / denom
}
