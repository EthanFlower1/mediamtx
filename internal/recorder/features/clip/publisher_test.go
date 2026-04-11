package clip

import (
	"math"
	"testing"
	"time"
)

func TestEncodeDecodeVector(t *testing.T) {
	original := []float32{1.0, -0.5, 0.25, 3.14159, 0, -1e10}
	encoded := encodeVector(original)

	decoded, err := DecodeVector(encoded)
	if err != nil {
		t.Fatalf("DecodeVector: %v", err)
	}

	if len(decoded) != len(original) {
		t.Fatalf("expected len %d, got %d", len(original), len(decoded))
	}

	for i := range original {
		if decoded[i] != original[i] {
			t.Errorf("index %d: expected %f, got %f", i, original[i], decoded[i])
		}
	}
}

func TestDecodeVector_InvalidBase64(t *testing.T) {
	_, err := DecodeVector("not-valid-base64!!!")
	if err == nil {
		t.Fatal("expected error for invalid base64")
	}
}

func TestDecodeVector_BadLength(t *testing.T) {
	// 3 bytes is not divisible by 4.
	_, err := DecodeVector("AQID") // base64 of []byte{1,2,3}
	if err == nil {
		t.Fatal("expected error for non-divisible-by-4 length")
	}
}

func TestGenerateEventID(t *testing.T) {
	ts := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	id := generateEventID("cam-1", ts)
	if id == "" {
		t.Fatal("expected non-empty event id")
	}
	// Deterministic: same inputs produce same id.
	id2 := generateEventID("cam-1", ts)
	if id != id2 {
		t.Errorf("expected deterministic id, got %q and %q", id, id2)
	}
	// Different camera produces different id.
	id3 := generateEventID("cam-2", ts)
	if id == id3 {
		t.Error("expected different id for different camera")
	}
}

func TestEncodeVector_RoundTrip_Normalised(t *testing.T) {
	// Verify that encode/decode preserves normalised vectors exactly.
	v := []float32{0.6, 0.8}
	normalise(v)

	encoded := encodeVector(v)
	decoded, err := DecodeVector(encoded)
	if err != nil {
		t.Fatalf("DecodeVector: %v", err)
	}

	var norm float64
	for i, x := range decoded {
		if x != v[i] {
			t.Errorf("index %d: expected %f, got %f", i, v[i], x)
		}
		norm += float64(x) * float64(x)
	}
	if math.Abs(norm-1.0) > 1e-5 {
		t.Errorf("expected L2 norm ~1.0, got %f", norm)
	}
}
