package audio

import (
	"math"
	"testing"
)

func TestClassifier_LoadAndClassify(t *testing.T) {
	c := NewClassifier("/tmp/models")

	// No model loaded yet.
	if c.HasModel(EventGunshot) {
		t.Fatal("expected no model loaded for gunshot")
	}

	// Load a mock gunshot model.
	c.LoadModel(&AudioModel{
		EventType: EventGunshot,
		ModelPath: "/tmp/models/gunshot.onnx",
		InferFunc: func(features []float32) (float32, error) {
			// Mock: return high confidence if features have high energy.
			if len(features) == 0 {
				return 0, nil
			}
			var sum float32
			for _, f := range features {
				sum += f * f
			}
			rms := float32(math.Sqrt(float64(sum / float32(len(features)))))
			if rms > 0.5 {
				return 0.95, nil
			}
			return 0.1, nil
		},
	})

	if !c.HasModel(EventGunshot) {
		t.Fatal("expected model loaded for gunshot")
	}

	// High energy features should trigger detection.
	highEnergy := make([]float32, 100)
	for i := range highEnergy {
		highEnergy[i] = 0.9
	}
	score, err := c.Classify(EventGunshot, highEnergy)
	if err != nil {
		t.Fatalf("classify error: %v", err)
	}
	if score < 0.9 {
		t.Errorf("expected high confidence for high energy, got %f", score)
	}

	// Low energy features should not trigger.
	lowEnergy := make([]float32, 100)
	for i := range lowEnergy {
		lowEnergy[i] = 0.01
	}
	score, err = c.Classify(EventGunshot, lowEnergy)
	if err != nil {
		t.Fatalf("classify error: %v", err)
	}
	if score > 0.5 {
		t.Errorf("expected low confidence for low energy, got %f", score)
	}
}

func TestClassifier_ClassifyAll(t *testing.T) {
	c := NewClassifier("/tmp/models")

	// Load two models.
	c.LoadModel(&AudioModel{
		EventType: EventGunshot,
		InferFunc: func(features []float32) (float32, error) {
			return 0.85, nil
		},
	})
	c.LoadModel(&AudioModel{
		EventType: EventGlassBreak,
		InferFunc: func(features []float32) (float32, error) {
			return 0.30, nil
		},
	})

	results := c.ClassifyAll(make([]float32, 10))
	if len(results) != 2 {
		t.Fatalf("expected 2 results, got %d", len(results))
	}
	if results[EventGunshot] != 0.85 {
		t.Errorf("gunshot score = %f, want 0.85", results[EventGunshot])
	}
	if results[EventGlassBreak] != 0.30 {
		t.Errorf("glass_break score = %f, want 0.30", results[EventGlassBreak])
	}
}

func TestClassifier_ClassifyNoModel(t *testing.T) {
	c := NewClassifier("/tmp/models")
	_, err := c.Classify(EventGunshot, make([]float32, 10))
	if err == nil {
		t.Fatal("expected error for missing model")
	}
}

func TestClassifier_Close(t *testing.T) {
	c := NewClassifier("/tmp/models")
	c.LoadModel(&AudioModel{
		EventType: EventGunshot,
		InferFunc: func(features []float32) (float32, error) { return 0.5, nil },
	})
	c.Close()
	if c.HasModel(EventGunshot) {
		t.Fatal("expected no models after Close")
	}
}

func TestExtractMelSpectrogram(t *testing.T) {
	// Generate a simple sine wave at 440 Hz, 16 kHz sample rate, 1 second.
	sampleRate := 16000
	duration := 1.0
	samples := make([]float32, int(float64(sampleRate)*duration))
	for i := range samples {
		samples[i] = float32(math.Sin(2 * math.Pi * 440 * float64(i) / float64(sampleRate)))
	}

	features := ExtractMelSpectrogram(samples, sampleRate, 64, 512, 160)
	if len(features) == 0 {
		t.Fatal("expected non-empty mel spectrogram")
	}

	// Should have 64 mel bins x numFrames values.
	numFrames := (len(samples) - 512) / 160
	expectedLen := 64 * numFrames
	if len(features) != expectedLen {
		t.Errorf("expected %d features, got %d", expectedLen, len(features))
	}

	// All values should be finite.
	for i, f := range features {
		if math.IsNaN(float64(f)) || math.IsInf(float64(f), 0) {
			t.Fatalf("non-finite value at index %d: %f", i, f)
		}
	}
}

func TestExtractMelSpectrogram_Empty(t *testing.T) {
	features := ExtractMelSpectrogram(nil, 16000, 64, 512, 160)
	if features != nil {
		t.Errorf("expected nil for empty input, got %d features", len(features))
	}
}

func TestMelConversion(t *testing.T) {
	// Round-trip test: hz -> mel -> hz should be approximately identity.
	testFreqs := []float64{0, 100, 440, 1000, 4000, 8000}
	for _, hz := range testFreqs {
		mel := hzToMel(hz)
		back := melToHz(mel)
		if math.Abs(back-hz) > 0.01 {
			t.Errorf("round-trip failed for %f Hz: got %f", hz, back)
		}
	}
}
