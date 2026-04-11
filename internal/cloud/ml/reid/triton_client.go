package reid

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"
)

// EmbeddingExtractor extracts re-id embeddings from person crop images.
// The real implementation calls Triton Inference Server; tests use the fake.
type EmbeddingExtractor interface {
	// Extract takes a cropped person image and returns a normalized embedding.
	Extract(ctx context.Context, imageData []byte, width, height int) ([]float32, error)

	// BatchExtract processes multiple crops in a single Triton call for
	// throughput. Returns one embedding per input, in order.
	BatchExtract(ctx context.Context, images []ImageInput) ([][]float32, error)

	// Close releases resources (HTTP client connections, etc.).
	Close() error
}

// ImageInput is a single image for batch extraction.
type ImageInput struct {
	Data   []byte
	Width  int
	Height int
}

// -----------------------------------------------------------------------
// Triton HTTP client
// -----------------------------------------------------------------------

// TritonConfig configures the Triton Inference Server connection.
type TritonConfig struct {
	// ServerURL is the base URL of the Triton HTTP endpoint (e.g. "http://localhost:8000").
	ServerURL string

	// ModelName is the name of the re-id model deployed on Triton (e.g. "osnet_x1_0").
	ModelName string

	// ModelVersion is the version string (empty for latest).
	ModelVersion string

	// Timeout per inference request.
	Timeout time.Duration

	// InputWidth and InputHeight are the model's expected input dimensions.
	InputWidth  int
	InputHeight int
}

// DefaultTritonConfig returns sensible defaults for OSNet on Triton.
func DefaultTritonConfig() TritonConfig {
	return TritonConfig{
		ServerURL:   "http://localhost:8000",
		ModelName:   "osnet_x1_0",
		Timeout:     5 * time.Second,
		InputWidth:  128,
		InputHeight: 256,
	}
}

// tritonClient implements EmbeddingExtractor via the Triton HTTP/REST API.
type tritonClient struct {
	cfg    TritonConfig
	client *http.Client
}

// NewTritonClient creates an EmbeddingExtractor backed by Triton Inference Server.
func NewTritonClient(cfg TritonConfig) EmbeddingExtractor {
	return &tritonClient{
		cfg: cfg,
		client: &http.Client{
			Timeout: cfg.Timeout,
		},
	}
}

// tritonInferRequest is the JSON body for Triton's v2 inference API.
type tritonInferRequest struct {
	Inputs  []tritonInput  `json:"inputs"`
	Outputs []tritonOutput `json:"outputs,omitempty"`
}

type tritonInput struct {
	Name     string    `json:"name"`
	Shape    []int     `json:"shape"`
	Datatype string    `json:"datatype"`
	Data     []float32 `json:"data"`
}

type tritonOutput struct {
	Name string `json:"name"`
}

// tritonInferResponse is the JSON body returned by Triton's v2 inference API.
type tritonInferResponse struct {
	Outputs []struct {
		Name     string    `json:"name"`
		Shape    []int     `json:"shape"`
		Datatype string    `json:"datatype"`
		Data     []float32 `json:"data"`
	} `json:"outputs"`
}

func (t *tritonClient) Extract(ctx context.Context, imageData []byte, width, height int) ([]float32, error) {
	results, err := t.BatchExtract(ctx, []ImageInput{{Data: imageData, Width: width, Height: height}})
	if err != nil {
		return nil, err
	}
	if len(results) == 0 {
		return nil, ErrNoEmbedding
	}
	return results[0], nil
}

func (t *tritonClient) BatchExtract(ctx context.Context, images []ImageInput) ([][]float32, error) {
	if len(images) == 0 {
		return nil, nil
	}

	batchSize := len(images)
	h, w := t.cfg.InputHeight, t.cfg.InputWidth
	if h == 0 {
		h = 256
	}
	if w == 0 {
		w = 128
	}
	channels := 3

	// Flatten all images into a single batch tensor [N, C, H, W].
	// The caller is expected to provide pre-processed RGB data, but we
	// accept raw bytes and do a simple normalization here.
	totalElements := batchSize * channels * h * w
	data := make([]float32, totalElements)

	for i, img := range images {
		offset := i * channels * h * w
		pixelCount := h * w
		imgLen := len(img.Data)

		for p := 0; p < pixelCount && p*3+2 < imgLen; p++ {
			r := float32(img.Data[p*3]) / 255.0
			g := float32(img.Data[p*3+1]) / 255.0
			b := float32(img.Data[p*3+2]) / 255.0
			data[offset+p] = r             // channel 0
			data[offset+pixelCount+p] = g   // channel 1
			data[offset+2*pixelCount+p] = b // channel 2
		}
	}

	req := tritonInferRequest{
		Inputs: []tritonInput{
			{
				Name:     "input",
				Shape:    []int{batchSize, channels, h, w},
				Datatype: "FP32",
				Data:     data,
			},
		},
		Outputs: []tritonOutput{
			{Name: "output"},
		},
	}

	body, err := json.Marshal(req)
	if err != nil {
		return nil, fmt.Errorf("reid: marshal triton request: %w", err)
	}

	url := fmt.Sprintf("%s/v2/models/%s/infer", t.cfg.ServerURL, t.cfg.ModelName)
	if t.cfg.ModelVersion != "" {
		url = fmt.Sprintf("%s/v2/models/%s/versions/%s/infer",
			t.cfg.ServerURL, t.cfg.ModelName, t.cfg.ModelVersion)
	}

	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("reid: create triton request: %w", err)
	}
	httpReq.Header.Set("Content-Type", "application/json")

	resp, err := t.client.Do(httpReq)
	if err != nil {
		return nil, fmt.Errorf("%w: %v", ErrTritonUnavail, err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		respBody, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("reid: triton returned %d: %s", resp.StatusCode, string(respBody))
	}

	var inferResp tritonInferResponse
	if err := json.NewDecoder(resp.Body).Decode(&inferResp); err != nil {
		return nil, fmt.Errorf("reid: decode triton response: %w", err)
	}

	if len(inferResp.Outputs) == 0 {
		return nil, ErrNoEmbedding
	}

	output := inferResp.Outputs[0]
	embDim := EmbeddingDim
	if len(output.Shape) >= 2 {
		embDim = output.Shape[len(output.Shape)-1]
	}

	results := make([][]float32, batchSize)
	for i := 0; i < batchSize; i++ {
		start := i * embDim
		end := start + embDim
		if end > len(output.Data) {
			return nil, fmt.Errorf("reid: triton output too short for batch item %d", i)
		}
		emb := make([]float32, embDim)
		copy(emb, output.Data[start:end])
		NormalizeEmbedding(emb)
		results[i] = emb
	}

	return results, nil
}

func (t *tritonClient) Close() error {
	t.client.CloseIdleConnections()
	return nil
}

// -----------------------------------------------------------------------
// Fake extractor for tests
// -----------------------------------------------------------------------

// FakeExtractor is a deterministic EmbeddingExtractor for unit tests.
// It produces a repeatable embedding derived from the image data hash.
type FakeExtractor struct {
	Dim int
}

// NewFakeExtractor creates a FakeExtractor with the given embedding dimension.
func NewFakeExtractor(dim int) *FakeExtractor {
	if dim <= 0 {
		dim = EmbeddingDim
	}
	return &FakeExtractor{Dim: dim}
}

func (f *FakeExtractor) Extract(_ context.Context, imageData []byte, _, _ int) ([]float32, error) {
	return f.deterministicEmbedding(imageData), nil
}

func (f *FakeExtractor) BatchExtract(_ context.Context, images []ImageInput) ([][]float32, error) {
	results := make([][]float32, len(images))
	for i, img := range images {
		results[i] = f.deterministicEmbedding(img.Data)
	}
	return results, nil
}

func (f *FakeExtractor) Close() error { return nil }

// deterministicEmbedding produces a normalized embedding derived from the
// input bytes. Same input always produces the same output. Different inputs
// produce substantially different embeddings so that cosine similarity
// distinguishes them even with high thresholds.
func (f *FakeExtractor) deterministicEmbedding(data []byte) []float32 {
	emb := make([]float32, f.Dim)

	// Use a simple FNV-like hash to seed each dimension differently.
	// This ensures that even small differences in data produce large
	// changes in the embedding vector.
	for i := range emb {
		h := uint32(2166136261) // FNV offset basis
		h ^= uint32(i * 7919)  // mix in dimension index with a prime
		for _, b := range data {
			h ^= uint32(b)
			h *= 16777619 // FNV prime
		}
		// Also mix dimension index into each byte iteration to spread values.
		h ^= uint32(i+1) * 2654435761 // Knuth multiplicative hash
		// Convert to a signed float in [-1, 1].
		emb[i] = float32(int32(h)) / float32(1<<31)
	}
	NormalizeEmbedding(emb)
	return emb
}
