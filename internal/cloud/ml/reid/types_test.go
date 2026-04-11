package reid

import (
	"math"
	"testing"
)

func TestEmbeddingRoundTrip(t *testing.T) {
	original := []float32{1.0, -2.5, 3.14, 0.0, -0.001}
	bytes := EmbeddingToBytes(original)
	recovered := BytesToEmbedding(bytes)

	if len(recovered) != len(original) {
		t.Fatalf("length mismatch: got %d, want %d", len(recovered), len(original))
	}
	for i := range original {
		if recovered[i] != original[i] {
			t.Errorf("index %d: got %f, want %f", i, recovered[i], original[i])
		}
	}
}

func TestCosineSimilarityIdentical(t *testing.T) {
	a := []float32{1.0, 2.0, 3.0}
	sim := CosineSimilarity(a, a)
	if math.Abs(sim-1.0) > 1e-6 {
		t.Errorf("identical vectors: sim = %f, want 1.0", sim)
	}
}

func TestCosineSimilarityOrthogonal(t *testing.T) {
	a := []float32{1.0, 0.0}
	b := []float32{0.0, 1.0}
	sim := CosineSimilarity(a, b)
	if math.Abs(sim) > 1e-6 {
		t.Errorf("orthogonal vectors: sim = %f, want 0.0", sim)
	}
}

func TestCosineSimilarityOpposite(t *testing.T) {
	a := []float32{1.0, 2.0, 3.0}
	b := []float32{-1.0, -2.0, -3.0}
	sim := CosineSimilarity(a, b)
	if math.Abs(sim-(-1.0)) > 1e-6 {
		t.Errorf("opposite vectors: sim = %f, want -1.0", sim)
	}
}

func TestCosineSimilarityDifferentLengths(t *testing.T) {
	a := []float32{1.0, 2.0}
	b := []float32{1.0, 2.0, 3.0}
	sim := CosineSimilarity(a, b)
	if sim != 0 {
		t.Errorf("different lengths: sim = %f, want 0.0", sim)
	}
}

func TestCosineSimilarityZeroVector(t *testing.T) {
	a := []float32{0.0, 0.0, 0.0}
	b := []float32{1.0, 2.0, 3.0}
	sim := CosineSimilarity(a, b)
	if sim != 0 {
		t.Errorf("zero vector: sim = %f, want 0.0", sim)
	}
}

func TestNormalizeEmbedding(t *testing.T) {
	emb := []float32{3.0, 4.0}
	NormalizeEmbedding(emb)

	// Should be [0.6, 0.8] after normalization.
	if math.Abs(float64(emb[0])-0.6) > 1e-6 {
		t.Errorf("emb[0] = %f, want 0.6", emb[0])
	}
	if math.Abs(float64(emb[1])-0.8) > 1e-6 {
		t.Errorf("emb[1] = %f, want 0.8", emb[1])
	}

	// L2 norm should be 1.0.
	var norm float64
	for _, v := range emb {
		norm += float64(v) * float64(v)
	}
	if math.Abs(math.Sqrt(norm)-1.0) > 1e-6 {
		t.Errorf("L2 norm = %f, want 1.0", math.Sqrt(norm))
	}
}

func TestNormalizeEmbeddingZero(t *testing.T) {
	emb := []float32{0.0, 0.0, 0.0}
	NormalizeEmbedding(emb) // should not panic
	for i, v := range emb {
		if v != 0 {
			t.Errorf("index %d: got %f, want 0.0", i, v)
		}
	}
}
